package core

import (
	"context"
	"log/slog"
)

// Audit 是审计日志：一条事件一行结构化日志，带 client_ip 与
// 时间戳（slog 的 JSON handler 会带上，时区取进程的 TZ）。
type Audit struct{}

func NewAudit() *Audit { return &Audit{} }

// AuditNameLimit 是审计里记录请求方自报名字（username 等）时的字符上限。这类值来自
// 未鉴权的请求体，原样入账会被一条几百 KiB 的 username 撑爆，也会淹没真正的记录。
const AuditNameLimit = 64

// Clip 把 value 裁到最多 limit 个字符（按 rune 计），超出时以「…」结尾，让截断在日志里一眼可见。
func Clip(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func (a *Audit) Write(event, ip string, extra map[string]any) {
	attrs := make([]slog.Attr, 0, len(extra)+2)
	attrs = append(attrs, slog.String("event", event), slog.String("client_ip", ip))
	for key, value := range extra {
		attrs = append(attrs, slog.Any(key, value))
	}
	slog.LogAttrs(context.Background(), slog.LevelInfo, "audit", attrs...)
}
