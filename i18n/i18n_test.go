package i18n_test

import (
	"strings"
	"testing"

	"scoped-skin/i18n"
)

func TestParsePicksFirstSupportedLanguage(t *testing.T) {
	t.Parallel()

	cases := map[string]i18n.Lang{
		"":                        i18n.ZhHans,
		"en":                      i18n.En,
		"en-US,en;q=0.9":          i18n.En,
		"zh-CN,zh;q=0.9,en;q=0.8": i18n.ZhHans,
		"en-GB,zh;q=0.5":          i18n.En,
		"fr-FR,de;q=0.8,en;q=0.7": i18n.En,
		"zh-Hant":                 i18n.ZhHans,
		"fr-FR,de;q=0.8":          i18n.ZhHans,
		"  EN-us  ":               i18n.En,
		"*":                       i18n.ZhHans,
	}
	for header, want := range cases {
		if got := i18n.Parse(header); got != want {
			t.Errorf("Parse(%q) = %q，想要 %q", header, got, want)
		}
	}

	if i18n.Normalize("EN") != i18n.En || i18n.Normalize("zh_TW") != i18n.ZhHans || i18n.Normalize("nope") != i18n.ZhHans {
		t.Errorf("Normalize 没归一好")
	}
	if got := i18n.Supported(); len(got) != 2 || got[0] != i18n.ZhHans || got[1] != i18n.En {
		t.Errorf("Supported() = %v", got)
	}
}

// 每条文案两种语言都要有，且不能留占位符 %s / %d 之外的半成品。
func TestCatalogHasEveryLanguage(t *testing.T) {
	t.Parallel()

	keys := i18n.Keys()
	if len(keys) < 40 {
		t.Fatalf("文案表只有 %d 条，像是漏了", len(keys))
	}
	for _, lang := range i18n.Supported() {
		for _, key := range keys {
			text := i18n.T(lang, key)
			if text == "" || text == key {
				t.Errorf("[%s] %s 没有文案", lang, key)
			}
			if strings.ContainsAny(text, "\n\t") {
				t.Errorf("[%s] %s 里有换行/制表符", lang, key)
			}
		}
	}
}

func TestTranslateRendersArgsAndNestedKeys(t *testing.T) {
	t.Parallel()

	if got := i18n.T(i18n.ZhHans, "valid.tag-count", 16); got != "标签最多 16 个" {
		t.Errorf("zh = %q", got)
	}
	if got := i18n.T(i18n.En, "valid.tag-count", 16); got != "At most 16 tags" {
		t.Errorf("en = %q", got)
	}
	// 参数里包成 i18n.Key 的会先按同一语言翻一遍
	if got := i18n.T(i18n.En, "valid.conflict", i18n.Key("field.player_name"), "steve"); got != "Player name “steve” is taken" {
		t.Errorf("嵌套键 = %q", got)
	}
	if got := i18n.T(i18n.ZhHans, "valid.conflict", i18n.Key("field.player_name"), "steve"); got != "游戏内名称「steve」已被占用" {
		t.Errorf("嵌套键 = %q", got)
	}
	// 认不出的键原样返回，方便一眼看出漏配
	if got := i18n.T(i18n.En, "nope.nope"); got != "nope.nope" {
		t.Errorf("未知键 = %q", got)
	}
	if i18n.Has("nope.nope") || !i18n.Has("auth.required") {
		t.Errorf("Has 判断不对")
	}
}
