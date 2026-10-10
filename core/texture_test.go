package core_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"
	"testing"

	"scoped-skin/core"
	"scoped-skin/i18n"
)

// pngHeader 造一段「头部合法」的 PNG 字节：只有 IHDR、没有图像数据，
// 用来验证「光有头不算数，还得真解得开」。
func pngHeader(width, height uint32) []byte {
	body := make([]byte, 24)
	copy(body, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	copy(body[12:], "IHDR")
	binary.BigEndian.PutUint32(body[16:], width)
	binary.BigEndian.PutUint32(body[20:], height)
	return body
}

// pngBytes 造一张真 PNG。
func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	canvas.Set(0, 0, color.RGBA{R: 0x2f, G: 0xbf, B: 0x71, A: 0xff})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("编码 PNG 失败: %v", err)
	}
	return buffer.Bytes()
}

// DESIGN.md 5.2：只接受默认尺寸 —— 皮肤正好 64×64，披风正好 64×32。
func TestValidateTexture(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		category      string
		width, height int
		data          []byte
		wantErr       string
	}{
		{"皮肤 64×64", "skin", 64, 64, nil, ""},
		{"披风 64×32", "cape", 64, 32, nil, ""},
		{"皮肤 128×128（HD 不收）", "skin", 128, 128, nil, "皮肤必须是 64×64"},
		{"皮肤 512×512（HD 不收）", "skin", 512, 512, nil, "皮肤必须是 64×64"},
		{"皮肤 64×32", "skin", 64, 32, nil, "皮肤必须是 64×64"},
		{"皮肤 32×32", "skin", 32, 32, nil, "皮肤必须是 64×64"},
		{"披风 32×64（宽高写反了）", "cape", 32, 64, nil, "披风必须是 64×32"},
		{"披风 64×64", "cape", 64, 64, nil, "披风必须是 64×32"},
		{"披风 128×64", "cape", 128, 64, nil, "披风必须是 64×32"},
		{"不是 PNG", "skin", 0, 0, []byte("GIF89a...................."), "不是有效的 PNG 图片"},
		{"太短", "skin", 0, 0, []byte{0x89, 'P'}, "不是有效的 PNG 图片"},
		{"只有 IHDR、没有图像数据", "skin", 0, 0, pngHeader(64, 64), "PNG 数据已损坏，无法解码"},
		{"声明尺寸为 0", "skin", 0, 0, pngHeader(0, 0), "图片尺寸无效"},
		{"未知类别", "hat", 64, 64, nil, "未知类别"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			data := testCase.data
			if data == nil {
				data = pngBytes(t, testCase.width, testCase.height)
			}
			width, height, err := core.ValidateTexture(testCase.category, data)
			if testCase.wantErr == "" {
				if err != nil {
					t.Fatalf("不该报错: %v", err)
				}
				if width == 0 || height == 0 {
					t.Fatalf("尺寸应当是正数: %d×%d", width, height)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("错误 = %v，想要包含 %q", err, testCase.wantErr)
			}
			var invalid *core.ValidationError
			if !asValidation(err, &invalid) {
				t.Fatalf("应当是 ValidationError: %T", err)
			}
		})
	}

	// 上限：32 KiB（尺寸检查再严，体积闸门也还在）
	oversized := append(pngHeader(core.SkinSize, core.SkinSize), make([]byte, core.MaxUploadBytes)...)
	if _, _, err := core.ValidateTexture("skin", oversized); err == nil ||
		!strings.Contains(err.Error(), "图片不能超过 32 KB") {
		t.Fatalf("超过 32 KiB 应当被拒: %v", err)
	}
}

func TestHeaderSize(t *testing.T) {
	t.Parallel()
	width, height := core.HeaderSize(pngHeader(128, 64))
	if width != 128 || height != 64 {
		t.Fatalf("HeaderSize = %d×%d", width, height)
	}
	if width, height := core.HeaderSize([]byte{1, 2, 3}); width != 0 || height != 0 {
		t.Fatalf("数据太短应当回 0×0，实际 %d×%d", width, height)
	}
}

// DESIGN.md 4.1 的字段约束。
func TestNormalizeUserFields(t *testing.T) {
	t.Parallel()

	if got, err := core.NormalizeUUID("B50AD385-829D-3141-A216-7E7D7539BA7F"); err != nil ||
		got != "b50ad385829d3141a2167e7d7539ba7f" {
		t.Fatalf("NormalizeUUID = %q, %v", got, err)
	}
	for _, bad := range []string{"", "xyz", "b50ad385829d3141a2167e7d7539ba7", "b50ad385829d3141a2167e7d7539ba7f0"} {
		if _, err := core.NormalizeUUID(bad); err == nil {
			t.Fatalf("UUID %q 不该通过", bad)
		}
	}

	if got, err := core.NormalizeUsername("  steve  "); err != nil || got != "steve" {
		t.Fatalf("NormalizeUsername = %q, %v", got, err)
	}
	if _, err := core.NormalizeUsername("   "); err == nil {
		t.Fatalf("空账号名不该通过")
	}
	if _, err := core.NormalizeUsername(strings.Repeat("a", core.MaxUsernameLength+1)); err == nil {
		t.Fatalf("超长账号名不该通过")
	}
	if _, err := core.NormalizeUsername("ste ve"); err == nil {
		t.Fatalf("账号名里的空白不该通过")
	}

	if got, err := core.NormalizePlayerName("  Steve_01 "); err != nil || got != "steve_01" {
		t.Fatalf("NormalizePlayerName = %q, %v", got, err)
	}
	for _, bad := range []string{"", "Steve!", "中文", "a2345678901234567"} {
		if _, err := core.NormalizePlayerName(bad); err == nil {
			t.Fatalf("角色名 %q 不该通过", bad)
		}
	}

	for _, role := range []string{"admin", "normal", "guest"} {
		if got, err := core.NormalizeRole(strings.ToUpper(role)); err != nil || got != role {
			t.Fatalf("NormalizeRole(%s) = %q, %v", role, got, err)
		}
	}
	if _, err := core.NormalizeRole("root"); err == nil {
		t.Fatalf("未知身份组不该通过")
	}

	if _, err := core.NormalizePassword(""); err == nil {
		t.Fatalf("空密码不该通过")
	}
	// DESIGN.md 4.1：至少 8 个字符
	if _, err := core.NormalizePassword(strings.Repeat("x", core.MinPasswordLength-1)); err == nil ||
		!strings.Contains(err.Error(), "密码至少 8 个字符") {
		t.Fatalf("短密码不该通过: %v", err)
	}
	if _, err := core.NormalizePassword(strings.Repeat("x", core.MinPasswordLength)); err != nil {
		t.Fatalf("正好 8 个字符应当通过: %v", err)
	}
	if _, err := core.NormalizePassword(strings.Repeat("x", core.MaxPasswordLength+1)); err == nil {
		t.Fatalf("超长密码不该通过")
	}
	if got, err := core.NormalizePassword(" 空格也算密码 "); err != nil || got != " 空格也算密码 " {
		t.Fatalf("密码不该被 trim: %q, %v", got, err)
	}
}

func TestNormalizeTagsAndModel(t *testing.T) {
	t.Parallel()

	tags, err := core.NormalizeTags([]string{" 官方 ", "官方", "Steve", "", "  ", "蓝色  外套"})
	if err != nil {
		t.Fatalf("NormalizeTags 报错: %v", err)
	}
	want := []string{"官方", "Steve", "蓝色 外套"}
	if len(tags) != len(want) {
		t.Fatalf("NormalizeTags = %v，想要 %v", tags, want)
	}
	for index := range want {
		if tags[index] != want[index] {
			t.Fatalf("NormalizeTags = %v，想要 %v", tags, want)
		}
	}

	// DESIGN.md 4.2：标签最多 16 个、单条最长 64 字符
	many := make([]string, core.MaxTagsPerAsset+1)
	for index := range many {
		many[index] = "标签" + strconv.Itoa(index)
	}
	if _, err := core.NormalizeTags(many); err == nil ||
		!strings.Contains(err.Error(), "标签最多 16 个") {
		t.Fatalf("超过 16 个标签应当被拒: %v", err)
	}
	if _, err := core.NormalizeTags(many[:core.MaxTagsPerAsset]); err != nil {
		t.Fatalf("正好 16 个标签应当通过: %v", err)
	}
	long := strings.Repeat("长", core.MaxTagLength+1)
	if _, err := core.NormalizeTags([]string{long}); err == nil ||
		!strings.Contains(err.Error(), "单个标签最长 64 个字符") {
		t.Fatalf("超长标签应当被拒: %v", err)
	}
	if _, err := core.NormalizeTags([]string{strings.Repeat("长", core.MaxTagLength)}); err != nil {
		t.Fatalf("正好 64 字符的标签应当通过: %v", err)
	}
	// 读取路径用的宽松版本：截断而不报错
	if got := core.CleanTags([]string{long}); len([]rune(got[0])) != core.MaxTagLength {
		t.Fatalf("CleanTags 应当截断到 %d 字符，实际 %q", core.MaxTagLength, got[0])
	}
	if got := core.CleanTags(many); len(got) != core.MaxTagsPerAsset {
		t.Fatalf("CleanTags 应当截到 %d 个，实际 %d", core.MaxTagsPerAsset, len(got))
	}

	cases := map[string]string{
		"steve": "steve", "classic": "steve", "default": "steve",
		"alex": "alex", "slim": "alex",
		"share": "share", "auto-detect": "share", "": "share", "乱写": "share", "STEVE": "steve",
	}
	for input, want := range cases {
		if got := core.NormalizeModel("skin", input); got != want {
			t.Fatalf("NormalizeModel(skin, %q) = %q，想要 %q", input, got, want)
		}
	}
	// DESIGN.md 4.2：category 为 cape 时 model_type 取 share
	for _, input := range []string{"steve", "alex", "", "乱写"} {
		if got := core.NormalizeModel("cape", input); got != core.CapeModel {
			t.Fatalf("NormalizeModel(cape, %q) = %q，应当是 share", input, got)
		}
	}
}

func TestQueryTokensAndMatching(t *testing.T) {
	t.Parallel()

	tokens := core.SplitQuery("  官方   官方  Steve ")
	want := []string{"官方", "Steve"}
	if len(tokens) != len(want) || tokens[0] != want[0] || tokens[1] != want[1] {
		t.Fatalf("SplitQuery = %v，想要 %v", tokens, want)
	}
	if tokens := core.SplitQuery(""); len(tokens) != 0 {
		t.Fatalf("空查询应当切成空切片: %v", tokens)
	}

	// 标签交集：每一项查询标签都要能在条目标签里找到（大小写不敏感）
	if !core.MatchesQuery([]string{"官方", "Steve"}, []string{"steve"}) {
		t.Fatalf("大小写不敏感的交集没匹配上")
	}
	if !core.MatchesQuery([]string{"官方", "Steve"}, []string{"官方", "STeve"}) {
		t.Fatalf("多标签交集没匹配上")
	}
	if core.MatchesQuery([]string{"官方"}, []string{"官方", "蓝色"}) {
		t.Fatalf("缺一项标签不该匹配")
	}
	if !core.MatchesQuery([]string{"官方"}, nil) {
		t.Fatalf("空查询应当匹配一切")
	}
}

func TestValidationErrorTypes(t *testing.T) {
	t.Parallel()

	var validation *core.ValidationError
	var conflict *core.ConflictError
	// 错误只带文案键 + 参数；Error() 渲染成源语言（简体中文），方便看日志
	err := error(&core.ValidationError{Key: "valid.uuid"})
	if !asValidation(err, &validation) || validation.Key != "valid.uuid" {
		t.Fatalf("ValidationError 断言失败")
	}
	if got := validation.Error(); got != "UUID 必须是 32 位十六进制字符" {
		t.Fatalf("Error() = %q，应当渲染成源语言文案", got)
	}
	conflictErr := error(&core.ConflictError{Key: "valid.conflict", Args: []any{i18n.Key("field.username"), "steve"}})
	if asValidation(conflictErr, &validation) {
		t.Fatalf("ConflictError 不该被断言成 ValidationError（两者是并列的具体类型）")
	}
	if !asConflict(conflictErr, &conflict) || conflict.Key != "valid.conflict" {
		t.Fatalf("ConflictError 断言失败")
	}
	if got := conflict.Error(); got != "账号名「steve」已被占用" {
		t.Fatalf("Error() = %q，参数里的文案键应当被翻译", got)
	}
}

// 校验函数抛出来的键必须都在文案表里，否则界面上会直接显示键名。
func TestValidationKeysAreInCatalog(t *testing.T) {
	t.Parallel()

	cases := []func() error{
		func() error { _, err := core.NormalizeUUID("nope"); return err },
		func() error { _, err := core.NormalizeUsername(""); return err },
		func() error {
			_, err := core.NormalizeUsername(strings.Repeat("a", core.MaxUsernameLength+1))
			return err
		},
		func() error { _, err := core.NormalizeUsername("a b"); return err },
		func() error { _, err := core.NormalizePlayerName("Bad!"); return err },
		func() error { _, err := core.NormalizeRole("root"); return err },
		func() error { _, err := core.NormalizePassword(""); return err },
		func() error { _, err := core.NormalizePassword("短"); return err },
		func() error {
			_, err := core.NormalizePassword(strings.Repeat("x", core.MaxPasswordLength+1))
			return err
		},
		func() error { _, _, _, err := core.NormalizeTexture("hat", pngBytes(t, 64, 64)); return err },
		func() error { _, _, _, err := core.NormalizeTexture("skin", []byte("junk")); return err },
		func() error { _, _, _, err := core.NormalizeTexture("skin", pngHeader(64, 64)); return err },
		func() error { _, _, _, err := core.NormalizeTexture("skin", pngHeader(0, 0)); return err },
		func() error { _, _, _, err := core.NormalizeTexture("skin", pngBytes(t, 128, 128)); return err },
		func() error { _, _, _, err := core.NormalizeTexture("cape", pngBytes(t, 64, 64)); return err },
		func() error {
			data := append(pngHeader(64, 64), make([]byte, core.MaxUploadBytes)...)
			_, _, _, err := core.NormalizeTexture("skin", data)
			return err
		},
	}
	for index, build := range cases {
		err := build()
		if err == nil {
			t.Fatalf("第 %d 个用例应当报错", index+1)
		}
		var invalid *core.ValidationError
		if !asValidation(err, &invalid) {
			t.Fatalf("第 %d 个用例应当是 ValidationError: %T", index+1, err)
		}
		if !i18n.Has(invalid.Key) {
			t.Fatalf("文案表里没有 %q（校验错误会原样把它显示给用户）", invalid.Key)
		}
	}
}
