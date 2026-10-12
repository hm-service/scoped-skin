package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"scoped-skin/api"
	"scoped-skin/app"
	"scoped-skin/builtin"
)

// version 是根元数据里的 implementationVersion，也是启动日志里的一条。
const version = "0.2.0"

func initLogger() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
}

// concurrencyLimits 是两个独立闸门的槽位数：API 那一半（含探针与贴图字节）与静态那一半。
type concurrencyLimits struct {
	api    int
	static int
}

// readConcurrencyLimits 读两个槽位数（AGENTS.md 的环境变量约定），没设就用默认值。
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
		return 0, fmt.Errorf("%s 必须是 >= 1 的整数，实际是 %q", name, raw)
	}
	return value, nil
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
	for _, prefix := range []string{"/api/", "/asset/", "/livez", "/readyz"} {
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

	instance, err := app.Open(app.Config{
		DataDir:        os.Getenv("DATA_DIR"),
		Version:        version,
		BuiltinFS:      builtin.Files,
		AdminUsername:  os.Getenv("ADMIN_USERNAME"),
		AdminPassword:  os.Getenv("ADMIN_PASSWORD"),
		TrustedProxies: os.Getenv("TRUSTED_PROXIES"),
		ReadyzToken:    os.Getenv("READYZ_TOKEN"),
	})
	if err != nil {
		slog.Error("init failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer instance.Close()

	// 路由与监听（DESIGN.md 1.1：HOST / PORT）
	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:              host + ":" + port,
		Handler:           newRootHandler(instance, site, limits),
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("server started",
		slog.String("addr", server.Addr),
		slog.String("dataDir", instance.Config.DataDir),
		slog.String("version", version),
	)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}
