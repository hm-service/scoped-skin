package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DESIGN.md 5.2「校验边界」：请求体上限（先看 Content-Length，再分块读兜底）。
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

// MaxConcurrentRequests 是 `/api/*` 的并发上限（DESIGN.md 5.3）：超出的请求在闸门外排队，
// 免得 goroutine、SQLite 连接与 Argon2 一起被放开。
const MaxConcurrentRequests = 128

// Concurrency 用一个固定容量的信号量把同时在处理的请求卡在上限以内。
// 排队的请求在客户端断开后直接放行，不再占用槽位。
//
// 只管 `/api/*`：页面、`/static/*` 与 `/asset/*` 都不吃 Argon2，也没有每请求的稀缺资源，
// 让它们直接走 —— 否则浏览器并发拉几张图就能把 128 个槽位占满。
func Concurrency(next http.Handler) http.Handler {
	slots := make(chan struct{}, MaxConcurrentRequests)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		select {
		case slots <- struct{}{}:
		case <-r.Context().Done():
			return
		}
		defer func() { <-slots }()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders 给所有响应补上三个静态安全头（DESIGN.md 5.3）：nosniff 挡 MIME 嗅探，
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
// 静态资源与 /livez 不记：一次页面加载就是十几个 /static 与 /asset 请求，podman 的存活探针
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

// skipAccessLog 判断这次请求要不要记访问日志：只放过纯静态的 GET 与存活探针。
func skipAccessLog(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	path := r.URL.Path
	return strings.HasPrefix(path, "/static/") ||
		strings.HasPrefix(path, "/asset/") ||
		path == "/livez"
}

// 放在中间件里才有意义 —— 等到 handler 里再查，multipart 已经被解析完了。
func LimitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if length := r.Header.Get("Content-Length"); length != "" {
			if size, err := strconv.ParseInt(length, 10, 64); err == nil && size > MaxRequestBytes {
				fail(w, r, http.StatusBadRequest, "request.too-big")
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
		next.ServeHTTP(w, r)
	})
}
