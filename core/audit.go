package core

import (
	"context"
	"log/slog"
)

// Audit 是 DESIGN.md 5.4 要求的审计日志：一条事件一行结构化日志，带远程 IP 与
// 时间戳（slog 的 JSON handler 会带上，时区取进程的 TZ）。
type Audit struct{}

func NewAudit() *Audit { return &Audit{} }

func (a *Audit) Write(event, ip string, extra map[string]any) {
	attrs := make([]slog.Attr, 0, len(extra)+2)
	attrs = append(attrs, slog.String("event", event), slog.String("ip", ip))
	for key, value := range extra {
		attrs = append(attrs, slog.Any(key, value))
	}
	slog.LogAttrs(context.Background(), slog.LevelInfo, "audit", attrs...)
}
