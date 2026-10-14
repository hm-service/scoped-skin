package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// 端口没设用默认值，设了就必须是 1-65535 的整数。
func TestReadPort(t *testing.T) {
	cases := []struct {
		name    string
		port    string
		want    int
		wantErr bool
	}{
		{name: "没设用默认值", want: 8080},
		{name: "正常端口", port: "10080", want: 10080},
		{name: "下界", port: "1", want: 1},
		{name: "上界", port: "65535", want: 65535},
		{name: "0", port: "0", wantErr: true},
		{name: "超出上界", port: "65536", wantErr: true},
		{name: "负数", port: "-1", wantErr: true},
		{name: "不是数字", port: "http", wantErr: true},
		{name: "带空格", port: " 8080", wantErr: true},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv("PORT", item.port)

			got, err := readPort()
			if item.wantErr {
				if err == nil {
					t.Fatalf("readPort() = %d，想要报错", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readPort() 报错: %v", err)
			}
			if got != item.want {
				t.Errorf("readPort() = %d，想要 %d", got, item.want)
			}
		})
	}
}

// HOST 没设用默认值，设了就必须是 IP 或合法的 RFC 1123 主机名。
func TestReadHost(t *testing.T) {
	longest := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)
	cases := []struct {
		name    string
		host    string
		want    string
		wantErr bool
	}{
		{name: "没设用默认值", want: "127.0.0.1"},
		{name: "IPv4", host: "0.0.0.0", want: "0.0.0.0"},
		{name: "IPv6", host: "::1", want: "::1"},
		{name: "主机名", host: "api.example.com", want: "api.example.com"},
		{name: "单标签主机名", host: "localhost", want: "localhost"},
		{name: "大写与连字符", host: "My-Host.example", want: "My-Host.example"},
		{name: "63 字符标签", host: strings.Repeat("a", 63), want: strings.Repeat("a", 63)},
		{name: "253 字符名", host: longest, want: longest},
		{name: "254 字符名", host: longest + "a", wantErr: true},
		{name: "64 字符标签", host: strings.Repeat("a", 64), wantErr: true},
		{name: "带空格", host: " 127.0.0.1", wantErr: true},
		{name: "带斜杠", host: "localhost/", wantErr: true},
		{name: "带端口", host: "localhost:8080", wantErr: true},
		{name: "空标签", host: "a..b", wantErr: true},
		{name: "结尾点", host: "example.com.", wantErr: true},
		{name: "连字符开头", host: "-host", wantErr: true},
		{name: "连字符结尾", host: "host-", wantErr: true},
		{name: "下划线", host: "my_host", wantErr: true},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv("HOST", item.host)

			got, err := readHost()
			if item.wantErr {
				if err == nil {
					t.Fatalf("readHost() = %q，想要报错", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readHost() 报错: %v", err)
			}
			if got != item.want {
				t.Errorf("readHost() = %q，想要 %q", got, item.want)
			}
		})
	}
}

// 探针 token 留空表示不设门槛，设了就必须是 16-128 位的字母 / 数字 / 下划线。
func TestReadReadyzToken(t *testing.T) {
	cases := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{name: "留空不校验"},
		{name: "正常 token", token: "your_readyz_token"},
		{name: "16 位下界", token: "0123456789abcdef"},
		{name: "含大写", token: "Your_readyz_token"},
		{name: "太短", token: "short_token", wantErr: true},
		{name: "太长", token: strings.Repeat("a", 129), wantErr: true},
		{name: "含连字符", token: "your-readyz-token", wantErr: true},
		{name: "含空格", token: "your readyz token", wantErr: true},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv("READYZ_TOKEN", item.token)

			got, err := readReadyzToken()
			if item.wantErr {
				if err == nil {
					t.Fatalf("readReadyzToken() = %q，想要报错", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readReadyzToken() 报错: %v", err)
			}
			if got != item.token {
				t.Errorf("readReadyzToken() = %q，想要 %q", got, item.token)
			}
			if left := os.Getenv("READYZ_TOKEN"); left != "" {
				t.Errorf("读完之后不该还留在环境表里: %q", left)
			}
		})
	}
}

// 签名密钥 32-128 位，留空表示本次进程随机生成一把。
func TestReadSessionSigningKey(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "留空表示随机生成"},
		{name: "32 位下界", key: strings.Repeat("a", 32)},
		{name: "太长", key: strings.Repeat("a", 129), wantErr: true},
		{name: "含连字符", key: "someone-very-safe-signing-key-32", wantErr: true},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv("SESSION_SIGNING_KEY", item.key)

			got, err := readSessionSigningKey()
			if item.wantErr {
				if err == nil {
					t.Fatalf("readSessionSigningKey() = %q，想要报错", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readSessionSigningKey() 报错: %v", err)
			}
			if got != item.key {
				t.Errorf("readSessionSigningKey() = %q，想要 %q", got, item.key)
			}
			if left := os.Getenv("SESSION_SIGNING_KEY"); left != "" {
				t.Errorf("读完之后不该还留在环境表里: %q", left)
			}
		})
	}
}

// 管理员密码读完同样从环境表里删掉。
func TestReadAdminPasswordUnsetEnv(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "verify_pass_123")

	if got := readAdminPassword(); got != "verify_pass_123" {
		t.Errorf("readAdminPassword() = %q", got)
	}
	if left := os.Getenv("ADMIN_PASSWORD"); left != "" {
		t.Errorf("读完之后不该还留在环境表里: %q", left)
	}
}

// TZ 没设就用系统时区，设了就必须是 Go 认得的地名（退成 UTC 就是静默降级）。
func TestCheckTimezone(t *testing.T) {
	cases := []struct {
		name    string
		tz      string
		wantErr bool
	}{
		{name: "没设"},
		{name: "UTC", tz: "UTC"},
		{name: "Asia/Shanghai", tz: "Asia/Shanghai"},
		{name: "地名写错", tz: "Asia/Shanghi", wantErr: true},
		{name: "不是地名", tz: "Shanghai", wantErr: true},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv("TZ", item.tz)

			if err := checkTimezone(); (err != nil) != item.wantErr {
				t.Fatalf("checkTimezone() 报错 = %v，想要报错 = %v", err, item.wantErr)
			}
		})
	}
}

// 两个槽位数都可以用环境变量覆盖；没设用默认值，
// 设了但写错直接失败 —— 信号量容量为 0 会让服务永久排队，宁可在启动时就报错。
func TestReadConcurrencyLimits(t *testing.T) {
	cases := []struct {
		name    string
		api     string
		static  string
		want    concurrencyLimits
		wantErr bool
	}{
		{name: "都没设用默认值", want: concurrencyLimits{api: 128, static: 64}},
		{name: "都设了", api: "256", static: "8", want: concurrencyLimits{api: 256, static: 8}},
		{name: "只设了一个", api: "1", want: concurrencyLimits{api: 1, static: 64}},
		{name: "API 不是数字", api: "many", wantErr: true},
		{name: "API 是 0", api: "0", wantErr: true},
		{name: "API 是负数", api: "-3", wantErr: true},
		{name: "静态是 0", static: "0", wantErr: true},
		{name: "静态带空格", static: " 8", wantErr: true},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv("MAX_API_CONCURRENCY", item.api)
			t.Setenv("MAX_STATIC_CONCURRENCY", item.static)

			got, err := readConcurrencyLimits()
			if item.wantErr {
				if err == nil {
					t.Fatalf("readConcurrencyLimits() = %+v，想要报错", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readConcurrencyLimits() 报错: %v", err)
			}
			if got != item.want {
				t.Errorf("readConcurrencyLimits() = %+v，想要 %+v", got, item.want)
			}
		})
	}
}

// 每条日志都要带 RFC 3339 的 timestamp 字段，而不是 handler 默认的 time。
func TestInitLoggerTimestampField(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建管道失败: %v", err)
	}
	t.Cleanup(func() { reader.Close() })
	previousOut := os.Stdout
	os.Stdout = writer
	previousLogger := slog.Default()
	t.Cleanup(func() {
		os.Stdout = previousOut
		slog.SetDefault(previousLogger)
	})

	initLogger()
	slog.Info("probe")
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭管道失败: %v", err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &record); err != nil {
		t.Fatalf("不是合法 JSON: %v (%s)", err, raw)
	}
	stamp, ok := record["timestamp"].(string)
	if !ok {
		t.Fatalf("缺少 timestamp 字段: %v", record)
	}
	if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
		t.Fatalf("timestamp 不是 RFC 3339: %q", stamp)
	}
	if _, ok := record["time"]; ok {
		t.Fatalf("不应再有 time 字段: %v", record)
	}
}
