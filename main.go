package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"scoped-skin/api"
	"scoped-skin/app"
	"scoped-skin/builtin"
)

// version 是根元数据里的 implementationVersion，也是启动日志里的一条。
const version = "0.2.0"

// shutdownTimeout 是优雅关闭的时限：等在手请求最多等这么久，
// 超时就强制关掉，进程以 254 退出。
const shutdownTimeout = 5 * time.Second

func initLogger() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		// 日志约定用 timestamp 字段，覆盖 handler 默认的 time。
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				attr.Key = "timestamp"
			}
			return attr
		},
	})))
}

// concurrencyLimits 是两个独立闸门的槽位数：API 那一半（含探针与贴图字节）与静态那一半。
type concurrencyLimits struct {
	api    int
	static int
}

// readConcurrencyLimits 读两个槽位数，没设就用默认值。
func readConcurrencyLimits() (concurrencyLimits, error) {
	apiLimit, err := readLimit("MAX_API_CONCURRENCY", api.DefaultAPIConcurrency)
	if err != nil {
		return concurrencyLimits{}, err
	}
	staticLimit, err := readLimit("MAX_STATIC_CONCURRENCY", api.DefaultStaticConcurrency)
	if err != nil {
		return concurrencyLimits{}, err
	}
	return concurrencyLimits{api: apiLimit, static: staticLimit}, nil
}

// readLimit 读一个并发上限：没设就用默认值，设了就必须是 >= 1 的整数 —— 写错了直接起不来，
// 免得信号量容量为 0 把服务变成永久排队。
func readLimit(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("%s must be an integer >= 1, got %q", name, raw)
	}
	return value, nil
}

// readPort 读 PORT：没设就用 8080，设了就必须是 1-65535 的整数。
func readPort() (int, error) {
	raw := os.Getenv("PORT")
	if raw == "" {
		return 8080, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 65535 {
		return 0, fmt.Errorf("PORT must be an integer between 1 and 65535, got %q", raw)
	}
	return value, nil
}

// readHost 读 HOST：没设就用 127.0.0.1，设了就必须是 IP 或合法的 RFC 1123 主机名 ——
// 带空格、斜杠的值会让 net.JoinHostPort 拼成另一个地址，宁可在启动时拦下。
func readHost() (string, error) {
	host := os.Getenv("HOST")
	if host == "" {
		return "127.0.0.1", nil
	}
	if net.ParseIP(host) == nil && !validHostname(host) {
		return "", fmt.Errorf("HOST must be an IP address or a valid hostname, got %q", host)
	}
	return host, nil
}

// validHostname 按 RFC 1123 校验：点分标签各 1-63 字符 [A-Za-z0-9-]、不以连字符起止，总长 <= 253。
func validHostname(host string) bool {
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for i, r := range label {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			case r == '-' && i > 0 && i < len(label)-1:
			default:
				return false
			}
		}
	}
	return true
}

// readSecret 读一个凭据类环境变量：长度与字符集都对得上才收，读完立刻从进程环境表里删掉 ——
// 免得子进程把它继承过去。留空返回空串，默认行为由调用方决定。
func readSecret(name string, minLength, maxLength int) (string, error) {
	value := os.Getenv(name)
	_ = os.Unsetenv(name)
	if value == "" {
		return "", nil
	}
	if len(value) < minLength || len(value) > maxLength {
		return "", fmt.Errorf("%s must be %d-%d characters long, got %d", name, minLength, maxLength, len(value))
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return "", fmt.Errorf("%s may only contain letters, digits and underscores, got %q", name, r)
		}
	}
	return value, nil
}

// readReadyzToken 读 READYZ_TOKEN：留空表示探针不设门槛。
func readReadyzToken() (string, error) { return readSecret("READYZ_TOKEN", 16, 128) }

// readSessionSigningKey 读 SESSION_SIGNING_KEY：留空表示本次进程随机生成一把，
// 那样重启之后已签发的凭据全部失效。
func readSessionSigningKey() (string, error) { return readSecret("SESSION_SIGNING_KEY", 32, 128) }

// readAdminPassword 读 ADMIN_PASSWORD：只在首次启动建号时用得上，读完同样从环境表里删掉。
// 长度与字符集由建号那一步按账号规则校验，不在这里重复一遍。
func readAdminPassword() string {
	password := os.Getenv("ADMIN_PASSWORD")
	_ = os.Unsetenv("ADMIN_PASSWORD")
	return password
}

// checkTimezone 读 TZ：设了就必须是 Go 认得的地名。
// 名字写错时 Go 会静默退回 UTC，日志时间戳跟着错 —— 宁可在启动时报出来。
func checkTimezone() error {
	name := os.Getenv("TZ")
	if name == "" {
		return nil
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("TZ is not a valid time zone name %q: %w", name, err)
	}
	return nil
}

// newRootHandler 把两半接起来：站点外壳（web.go）管页面与静态资源，api 包管接口、
// 探针与贴图字节；两半各套一层并发闸门、各用一份槽位，外面再套共用的那几层中间件
// （访问日志、安全头、语言协商、体积上限），站点外壳因此和接口一样有安全头与访问日志。
//
// files 就是站点外壳要发的构建产物：正常启动传内嵌的 site，测试传内存文件系统。
func newRootHandler(instance *app.App, files fs.FS, limits concurrencyLimits) http.Handler {
	apiHandler := api.Concurrency(limits.api, api.NewMux(instance))
	siteShell := api.Concurrency(limits.static, siteHandler(files))
	root := http.NewServeMux()
	// api 包认领的前缀：新增的接口如果不落在这些前缀下，得在这里补一条，
	// 否则请求会落到站点外壳上变成 404。
	for _, prefix := range []string{"/api/", "/livez", "/readyz"} {
		root.Handle(prefix, apiHandler)
	}
	root.Handle("/", siteShell)
	return api.Middleware(instance, root)
}

func main() {
	initLogger()

	limits, err := readConcurrencyLimits()
	if err != nil {
		slog.Error("init failed", slog.Any("error", err))
		os.Exit(1)
	}
	port, err := readPort()
	if err != nil {
		slog.Error("init failed", slog.Any("error", err))
		os.Exit(1)
	}
	host, err := readHost()
	if err != nil {
		slog.Error("init failed", slog.Any("error", err))
		os.Exit(1)
	}
	readyzToken, err := readReadyzToken()
	if err != nil {
		slog.Error("init failed", slog.Any("error", err))
		os.Exit(1)
	}
	sessionKey, err := readSessionSigningKey()
	if err != nil {
		slog.Error("init failed", slog.Any("error", err))
		os.Exit(1)
	}
	if sessionKey == "" {
		slog.Warn("SESSION_SIGNING_KEY is not set; issued credentials will not survive a restart")
	}
	// 管理员密码也是凭据：读完就删，别让它留在环境表里。
	adminPassword := readAdminPassword()
	if err := checkTimezone(); err != nil {
		slog.Error("init failed", slog.Any("error", err))
		os.Exit(1)
	}

	instance, err := app.Open(app.Config{
		DataDir:           os.Getenv("DATA_DIR"),
		Version:           version,
		BuiltinFS:         builtin.Files,
		AdminUsername:     os.Getenv("ADMIN_USERNAME"),
		AdminPassword:     adminPassword,
		TrustedProxies:    os.Getenv("TRUSTED_PROXIES"),
		SessionSigningKey: sessionKey,
		ReadyzToken:       readyzToken,
	})
	if err != nil {
		slog.Error("init failed", slog.Any("error", err))
		os.Exit(1)
	}

	// 路由与监听。自己 Listen 而不是 ListenAndServe，
	// 是为了先拿到内核给的地址再打启动日志，端口写 0 时也只有这里看得到实际端口。
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		slog.Error("init failed", slog.Any("error", err))
		instance.Close()
		os.Exit(1)
	}
	server := &http.Server{
		Handler:           newRootHandler(instance, site, limits),
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("server started",
		slog.String("addr", listener.Addr().String()),
		slog.String("data_dir", instance.Config.DataDir),
		slog.String("start_at", time.Now().Format(time.RFC3339)),
		slog.String("version", version),
	)
	os.Exit(serve(server, listener, instance))
}

// serve 跑起服务并处理停止信号：第一次 SIGINT / SIGTERM 走优雅关闭，
// 之后的信号交回默认处置 —— 卡在等待里时，再按一次 Ctrl-C 就该立刻结束。
//
// 返回值即进程退出码：正常关闭 0；Shutdown 超时被强制关闭 254；关库失败 255。
func serve(server *http.Server, listener net.Listener, instance *app.App) int {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	var received os.Signal
	select {
	case received = <-signals:
		signal.Stop(signals)
		slog.Info("shutdown signal received", slog.String("signal", signalName(received)))
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server exited with error", slog.Any("error", err))
			instance.Close()
			return 1
		}
		instance.Close()
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown timed out", slog.Any("error", err))
		server.Close()
		instance.Close()
		return 254
	}
	if err := instance.Close(); err != nil {
		slog.Error("closing data store failed", slog.Any("error", err))
		return 255
	}
	slog.Info("server shutdown",
		slog.String("signal", signalName(received)),
		slog.String("stop_at", time.Now().Format(time.RFC3339)),
	)
	return 0
}

// signalName 把信号写成 SIGINT / SIGTERM 这样的规范名：Go 默认的 "interrupt" / "terminated"
// 在运维侧不直观。
func signalName(sig os.Signal) string {
	switch sig {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	}
	return sig.String()
}
