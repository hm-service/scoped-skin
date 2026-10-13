package main

import (
	"strings"
	"testing"
)

// 端口没设用默认值，设了就必须是 1-65535 的整数（AGENTS.md 的环境变量约定）。
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

// 探针 token 留空表示不设门槛，设了就必须是 16-128 位的小写字母 / 数字 / 下划线。
func TestReadReadyzToken(t *testing.T) {
	cases := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{name: "留空不校验"},
		{name: "正常 token", token: "your_readyz_token"},
		{name: "16 位下界", token: "0123456789abcdef"},
		{name: "太短", token: "short_token", wantErr: true},
		{name: "太长", token: strings.Repeat("a", 129), wantErr: true},
		{name: "含大写", token: "Your_readyz_token", wantErr: true},
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
		})
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

// 两个槽位数都可以用环境变量覆盖（AGENTS.md 的环境变量约定）；没设用默认值，
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
