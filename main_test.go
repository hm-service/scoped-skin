package main

import "testing"

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
