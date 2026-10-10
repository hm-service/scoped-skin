package core_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"scoped-skin/core"
)

// captureLog 把默认 slog 输出接到内存里，返回读日志的函数。
func captureLog(t *testing.T) func() []map[string]any {
	t.Helper()

	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return func() []map[string]any {
		records := []map[string]any{}
		for _, line := range strings.Split(strings.TrimSpace(buffer.String()), "\n") {
			if line == "" {
				continue
			}
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("不是合法 JSON: %v (%s)", err, line)
			}
			records = append(records, record)
		}
		return records
	}
}

// DESIGN.md 5.4：每条审计记录一行结构化日志，带事件名、远程 IP 与 RFC 3339 时间戳。
//
// 不用 t.Parallel：captureLog 会改全局的 slog 默认 handler，并行跑会互相串台。
func TestAuditWritesStructuredRecords(t *testing.T) {
	read := captureLog(t)
	audit := core.NewAudit()
	audit.Write("login", "203.0.113.9", map[string]any{"username": "steve", "result": "ok"})
	audit.Write("user.create", "203.0.113.9", map[string]any{"actor": "uuid-admin", "uuid": "uuid-steve"})

	records := read()
	if len(records) != 2 {
		t.Fatalf("应当写下 2 条，实际 %d: %v", len(records), records)
	}
	first := records[0]
	if first["msg"] != "audit" || first["event"] != "login" || first["ip"] != "203.0.113.9" || first["username"] != "steve" {
		t.Fatalf("审计内容不对: %v", first)
	}
	stamp, ok := first["time"].(string)
	if !ok {
		t.Fatalf("缺少 time 字段: %v", first)
	}
	if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
		t.Fatalf("time 不是 RFC 3339: %q", stamp)
	}
	if second := records[1]; second["event"] != "user.create" || second["actor"] != "uuid-admin" {
		t.Fatalf("审计内容不对: %v", second)
	}
}

// 不传额外字段也要能写，且不落任何文件（审计只流向 stdout）。
func TestAuditWithoutExtraFields(t *testing.T) {
	read := captureLog(t)
	core.NewAudit().Write("login", "127.0.0.1", nil)

	records := read()
	if len(records) != 1 || records[0]["ip"] != "127.0.0.1" {
		t.Fatalf("审计记录不对: %v", records)
	}
}
