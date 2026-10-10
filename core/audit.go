package core

import (
	"context"
	"log/slog"
)

// Audit 是 DESIGN.md 5.4 要求的审计日志：一条事件一行结构化日志，带远程 IP 与
// 时间戳（slog 的 JSON handler 会带上，时区取进程的 TZ）。
//
//	{"time":"2026-10-10T21:18:35+08:00","level":"INFO","msg":"audit","event":"login","ip":"127.0.0.1","username":"steve"}
//
// 不落任何文件：全部交给 stdout，由容器运行时去收集与轮转（DESIGN.md 1.1）。
type Audit struct{}

// NewAudit 造一个审计记录器。
func NewAudit() *Audit { return &Audit{} }

// Write 记一条审计事件；extra 里的字段直接并进这条日志。
func (a *Audit) Write(event, ip string, extra map[string]any) {
	attrs := make([]slog.Attr, 0, len(extra)+2)
	attrs = append(attrs, slog.String("event", event), slog.String("ip", ip))
	for key, value := range extra {
		attrs = append(attrs, slog.Any(key, value))
	}
	slog.LogAttrs(context.Background(), slog.LevelInfo, "audit", attrs...)
}
