package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
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

func main() {
	initLogger()

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
		Handler:           api.NewHandler(instance),
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
