package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// 「校验边界」：请求体上限（先看 Content-Length，再分块读兜底）。
// 贴图上限只有 32 KiB，1 MiB 的余量已经足够（base64 预检按 4/3 膨胀也远够）。
const MaxRequestBytes = 1 * 1024 * 1024

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Write(body []byte) (int, error) {
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	return rec.ResponseWriter.Write(body)
}

// 两半各有一个独立闸门，槽位数可被环境变量覆盖：
// API 那一半管住 goroutine / SQLite / Argon2，静态那一半只防止一次页面加载把内嵌文件
// 同时读爆 —— 两边各占各自的槽位，谁也不排谁的队。
const (
	DefaultAPIConcurrency    = 128
	DefaultStaticConcurrency = 64
)

// Concurrency 用一个固定容量的信号量把同时在处理的请求卡在 limit 以内。
// 排队的请求在客户端断开后直接放行，不再占用槽位。
//
// 这里不认路径：套在谁身上就卡谁。API 与静态各套一层、各用一份槽位，接线在 main.go
// （NewHandler 只服务接口，所以自己套 API 那一层默认值）。
func Concurrency(limit int, next http.Handler) http.Handler {
	slots := make(chan struct{}, limit)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
		case <-r.Context().Done():
			return
		}
		defer func() { <-slots }()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders 给所有响应补上三个静态安全头：nosniff 挡 MIME 嗅探，
// no-referrer 不外泄地址，DENY 禁止被 iframe 嵌套。CSP 暂不引入。
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// Logging 给每个请求打一条结构化日志（照 mini-yggdrasil 的中间件风格）。
// clientIP 由调用方给（见 httpx.Proxies）：访问日志与审计记录用的是同一个字段名与同一个值。
//
// 静态资源与 /livez 不记：一次页面加载就是十几个 /assets 与 /asset 请求，podman 的存活探针
// 每 30 秒又敲一次 /livez，全是噪音。/readyz 照记 —— 它带 token 门槛，401/503 值得留痕。
func Logging(clientIP func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skipAccessLog(r) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		level := slog.LevelInfo
		switch {
		case recorder.status >= 500:
			level = slog.LevelError
		case recorder.status >= 400:
			level = slog.LevelWarn
		}
		slog.Log(r.Context(), level, "HTTP Request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", recorder.status),
			slog.String("client_ip", clientIP(r)),
			slog.String("duration", time.Since(start).String()),
		)
	})
}

// skipAccessLog 判断这次请求要不要记访问日志：只放过构建产物与贴图字节的 GET，
// 以及 GET / HEAD 的存活探针（HEAD 由 GET 处理器应答，同样是探针噪音）。
func skipAccessLog(r *http.Request) bool {
	path := r.URL.Path
	switch r.Method {
	case http.MethodGet:
		return strings.HasPrefix(path, "/assets/") ||
			strings.HasPrefix(path, "/api/assets/") ||
			path == "/livez"
	case http.MethodHead:
		return path == "/livez"
	default:
		return false
	}
}

// 放在中间件里才有意义 —— 等到 handler 里再查，multipart 已经被解析完了。
func LimitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if length := r.Header.Get("Content-Length"); length != "" {
			if size, err := strconv.ParseInt(length, 10, 64); err == nil && size > MaxRequestBytes {
				fail(w, r, http.StatusRequestEntityTooLarge, "request.too-big")
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
		next.ServeHTTP(w, r)
	})
}
