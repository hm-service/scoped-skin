package i18n_test

import (
	"testing"

	"scoped-skin/i18n"
)

// 语言协商的边角：按出现顺序取第一个认得出来的（q 值不参与排序，这是文档化的约定），
// 一个都不认得 / 全是空项时回默认（简体中文）。
func TestParseQualityValuesAndUnknownFallback(t *testing.T) {
	cases := map[string]i18n.Lang{
		"en;q=0.1,zh;q=0.9": i18n.En,     // 先出现的 en 赢，尽管 zh 的 q 更高
		"zh;q=0.1,en;q=0.9": i18n.ZhHans, // 反过来同理
		"zh-Hans-CN,en":     i18n.ZhHans,
		"EN_us":             i18n.En,
		",,;q=0.9":          i18n.ZhHans, // 全是空项
		";q=0.9":            i18n.ZhHans,
		"de,fr,ja":          i18n.ZhHans, // 一个都不认得
		"*":                 i18n.ZhHans,
	}
	for header, want := range cases {
		t.Run(header, func(t *testing.T) {
			if got := i18n.Parse(header); got != want {
				t.Errorf("Parse(%q) = %q，想要 %q", header, got, want)
			}
		})
	}

	for input, want := range map[string]i18n.Lang{
		"":        i18n.ZhHans,
		"   ":     i18n.ZhHans,
		"ZH_hant": i18n.ZhHans,
		" en-GB ": i18n.En,
		"不认识的输入":  i18n.ZhHans,
	} {
		t.Run(input, func(t *testing.T) {
			if got := i18n.Normalize(input); got != want {
				t.Errorf("Normalize(%q) = %q，想要 %q", input, got, want)
			}
		})
	}
}
