package core_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
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

// 只接受默认尺寸 —— 皮肤正好 64×64，披风正好 64×32。
func TestValidateTexture(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		kind          string
		width, height int
		data          []byte
		wantErr       string
	}{
		{"皮肤 64×64（steve）", "skin.steve", 64, 64, nil, ""},
		{"皮肤 64×64（alex）", "skin.alex", 64, 64, nil, ""},
		{"披风 64×32", "cape", 64, 32, nil, ""},
		{"皮肤 128×128（HD 不收）", "skin.steve", 128, 128, nil, "texture.skin"},
		{"皮肤 512×512（HD 不收）", "skin.alex", 512, 512, nil, "texture.skin"},
		{"皮肤 64×32", "skin.steve", 64, 32, nil, "texture.skin"},
		{"皮肤 32×32", "skin.alex", 32, 32, nil, "texture.skin"},
		{"披风 32×64（宽高写反了）", "cape", 32, 64, nil, "texture.cape"},
		{"披风 64×64", "cape", 64, 64, nil, "texture.cape"},
		{"披风 128×64", "cape", 128, 64, nil, "texture.cape"},
		{"不是 PNG", "skin.steve", 0, 0, []byte("GIF89a...................."), "texture.png"},
		{"太短", "skin.steve", 0, 0, []byte{0x89, 'P'}, "texture.png"},
		{"只有 IHDR、没有图像数据", "skin.steve", 0, 0, pngHeader(64, 64), "texture.decode"},
		{"声明尺寸为 0", "skin.steve", 0, 0, pngHeader(0, 0), "texture.size"},
		{"未知 kind", "hat", 64, 64, nil, "valid.kind"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			data := testCase.data
			if data == nil {
				data = pngBytes(t, testCase.width, testCase.height)
			}
			width, height, err := core.ValidateTexture(testCase.kind, data)
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
	if _, _, err := core.ValidateTexture("skin.steve", oversized); err == nil ||
		!strings.Contains(err.Error(), "upload.too-big") {
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

// 账号字段的约束。
func TestNormalizeUserFields(t *testing.T) {
	t.Parallel()

	// 写入路径只收小写（大写要在前端折下来，后端严格）
	if got, err := core.NormalizeUUID("b50ad385-829d-3141-a216-7e7d7539ba7f"); err != nil ||
		got != "b50ad385829d3141a2167e7d7539ba7f" {
		t.Fatalf("NormalizeUUID = %q, %v", got, err)
	}
	for index, bad := range []string{"", "xyz", "B50AD385-829D-3141-A216-7E7D7539BA7F",
		"b50ad385829d3141a2167e7d7539ba7", "b50ad385829d3141a2167e7d7539ba7f0"} {
		t.Run(fmt.Sprintf("%d_%s", index, bad), func(t *testing.T) {
			if _, err := core.NormalizeUUID(bad); err == nil {
				t.Fatalf("UUID %q 不该通过", bad)
			}
		})
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
	// 账号名只收小写字母 + 数字 + @ + .（邮箱形状够用）
	// 规则是「字符集」而不是邮箱语法：@ 与 . 可以出现在任何位置，下划线不在集合里
	for index, bad := range []string{"ste ve", "Steve", "steve!", "中文", "steve-bot", "a/b"} {
		t.Run(fmt.Sprintf("%d_%s", index, bad), func(t *testing.T) {
			if _, err := core.NormalizeUsername(bad); err == nil {
				t.Errorf("账号名 %q 不该通过", bad)
			}
		})
	}
	for index, good := range []string{"steve", "steve_bot", "steve.bot", "a.b@c.d", "user123@example.com", "@"} {
		t.Run(fmt.Sprintf("%d_%s", index, good), func(t *testing.T) {
			if _, err := core.NormalizeUsername(good); err != nil {
				t.Errorf("账号名 %q 应当通过: %v", good, err)
			}
		})
	}

	// 大小写原样保留（正版也是显示按输入、唯一性不敏感）
	for input, want := range map[string]string{
		"  Steve_01 ": "Steve_01",
		"steve_01":    "steve_01",
		"STEVE_01":    "STEVE_01",
		"abc":         "abc", // 下限 3
		strings.Repeat("a", core.MaxPlayerNameLength): strings.Repeat("a", core.MaxPlayerNameLength),
	} {
		t.Run(input, func(t *testing.T) {
			if got, err := core.NormalizePlayerName(input); err != nil || got != want {
				t.Fatalf("NormalizePlayerName(%q) = %q, %v（想要 %q）", input, got, err, want)
			}
		})
	}
	for index, bad := range []string{
		"", "ab", // 短于下限
		"Steve!", "中文", "ste ve", // 字符集
		strings.Repeat("a", core.MaxPlayerNameLength+1), // 超上限
	} {
		t.Run(fmt.Sprintf("%d_%s", index, bad), func(t *testing.T) {
			if _, err := core.NormalizePlayerName(bad); err == nil {
				t.Fatalf("角色名 %q 不该通过", bad)
			}
		})
	}

	for _, role := range []string{"admin", "normal", "guest"} {
		t.Run(role, func(t *testing.T) {
			if got, err := core.NormalizeRole(strings.ToUpper(role)); err != nil || got != role {
				t.Fatalf("NormalizeRole(%s) = %q, %v", role, got, err)
			}
		})
	}
	if _, err := core.NormalizeRole("root"); err == nil {
		t.Fatalf("未知身份组不该通过")
	}

	if _, err := core.NormalizePassword(""); err == nil {
		t.Fatalf("空密码不该通过")
	}
	// 至少 8 个字符
	if _, err := core.NormalizePassword(strings.Repeat("x", core.MinPasswordLength-1)); err == nil ||
		!strings.Contains(err.Error(), "valid.password.short") {
		t.Fatalf("短密码不该通过: %v", err)
	}
	if _, err := core.NormalizePassword(strings.Repeat("x", core.MinPasswordLength)); err != nil {
		t.Fatalf("正好 8 个字符应当通过: %v", err)
	}
	if _, err := core.NormalizePassword(strings.Repeat("x", core.MaxPasswordLength+1)); err == nil {
		t.Fatalf("超长密码不该通过")
	}
	// 密码走白名单字符集：空白/符号（以及复制粘贴带进来的零宽字符）一律拒，
	// 不做 trim —— 不然后端会悄悄改掉用户输入的密码。
	if _, err := core.NormalizePassword(" hunter2x"); err == nil {
		t.Fatalf("带空格的密码不该通过")
	}
	if got, err := core.NormalizePassword("hunter_2x"); err != nil || got != "hunter_2x" {
		t.Fatalf("合法密码应当原样返回: %q, %v", got, err)
	}
}

func TestNormalizeTags(t *testing.T) {
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

	// 标签最多 16 个、单条最长 64 字符
	many := make([]string, core.MaxTagsPerAsset+1)
	for index := range many {
		many[index] = "标签" + strconv.Itoa(index)
	}
	if _, err := core.NormalizeTags(many); err == nil ||
		!strings.Contains(err.Error(), "valid.tag-count") {
		t.Fatalf("超过 16 个标签应当被拒: %v", err)
	}
	if _, err := core.NormalizeTags(many[:core.MaxTagsPerAsset]); err != nil {
		t.Fatalf("正好 16 个标签应当通过: %v", err)
	}
	long := strings.Repeat("长", core.MaxTagLength+1)
	if _, err := core.NormalizeTags([]string{long}); err == nil ||
		!strings.Contains(err.Error(), "valid.tag-long") {
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
}

// kind 是库里存的那一列（已含模型），只认 skin.steve / skin.alex / cape。
// 旧的 model（share / steve / alex）与粗分类（skin）都不再是合法取值。
func TestNormalizeKind(t *testing.T) {
	t.Parallel()

	valid := map[string]string{
		"skin.steve":    "skin.steve",
		"skin.alex":     "skin.alex",
		"cape":          "cape",
		"SKIN.STEVE":    "skin.steve",
		"  Skin.Alex  ": "skin.alex",
		"\tcape\n":      "cape",
	}
	for input, want := range valid {
		t.Run(input, func(t *testing.T) {
			got, err := core.NormalizeKind(input)
			if err != nil || got != want {
				t.Fatalf("NormalizeKind(%q) = %q, %v，想要 %q", input, got, err, want)
			}
		})
	}

	// 非法值 / 旧结构的枚举值 / 大小写与空白之外的花样，都要回 valid.kind
	for index, bad := range []string{
		"", "   ", "skin", "steve", "alex", "share", "default", "slim", "classic",
		"skin.steve.x", "skin-steve", "skin steve", "capes", "SKIN", "乱写",
	} {
		t.Run(fmt.Sprintf("%d_%s", index, bad), func(t *testing.T) {
			got, err := core.NormalizeKind(bad)
			if err == nil {
				t.Fatalf("NormalizeKind(%q) 不该通过（回 %q）", bad, got)
			}
			var invalid *core.ValidationError
			if !asValidation(err, &invalid) || invalid.Key != "valid.kind" {
				t.Fatalf("NormalizeKind(%q) 的错误 = %v，想要 *core.ValidationError{valid.kind}", bad, err)
			}
		})
	}
}

// kind ↔ category 的推导关系：皮肤类两个 kind 都归 skin，cape 归 cape。
func TestCategoryOfAndKindsOf(t *testing.T) {
	t.Parallel()

	for _, kind := range core.Kinds {
		t.Run(kind, func(t *testing.T) {
			category := core.CategoryOf(kind)
			if category != "skin" && category != "cape" {
				t.Fatalf("CategoryOf(%q) = %q，不在 Categories 里", kind, category)
			}
		})
	}
	if core.CategoryOf("skin.steve") != "skin" || core.CategoryOf("skin.alex") != "skin" ||
		core.CategoryOf("cape") != "cape" {
		t.Fatalf("CategoryOf 的映射不对")
	}

	skins := core.KindsOf("skin")
	if len(skins) != 2 || skins[0] != "skin.steve" || skins[1] != "skin.alex" {
		t.Fatalf("KindsOf(skin) = %v，想要 [skin.steve skin.alex]", skins)
	}
	if capes := core.KindsOf("cape"); len(capes) != 1 || capes[0] != "cape" {
		t.Fatalf("KindsOf(cape) = %v，想要 [cape]", capes)
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

	// 匹配档位：前缀(0) → 中缀(1) → 后缀(2)，大小写不敏感，认不出回 -1
	ranks := []struct {
		tag, token string
		want       int
	}{
		{"Steve", "steve", core.MatchPrefix},
		{"Steve", "ST", core.MatchPrefix},
		{"浅蓝披风", "蓝", core.MatchInfix},
		{"浅蓝披风", "披风", core.MatchSuffix},
		{"浅蓝披风", "蓝披", core.MatchInfix},
		{"浅蓝披风", "绿", -1},
		{"abc", "abcd", -1},
	}
	for index, testCase := range ranks {
		t.Run(fmt.Sprintf("%d_%s_%s", index, testCase.tag, testCase.token), func(t *testing.T) {
			if got := core.MatchRank(testCase.tag, testCase.token); got != testCase.want {
				t.Errorf("MatchRank(%q, %q) = %d，想要 %d", testCase.tag, testCase.token, got, testCase.want)
			}
		})
	}

	// 标签交集：每一项查询词都要能在条目标签里找到；档位取各项里最差的那一档
	rank, ok := core.QueryRank([]string{"官方", "Steve"}, []string{"steve"})
	if !ok || rank != core.MatchPrefix {
		t.Fatalf("大小写不敏感的前缀命中没匹配上: %d, %v", rank, ok)
	}
	if rank, ok := core.QueryRank([]string{"官方", "Steve"}, []string{"官方", "STeve"}); !ok || rank != core.MatchPrefix {
		t.Fatalf("多标签交集没匹配上: %d, %v", rank, ok)
	}
	if rank, ok := core.QueryRank([]string{"官方皮肤"}, []string{"官方", "皮肤"}); !ok || rank != core.MatchSuffix {
		t.Fatalf("最差档位应当是后缀: %d, %v", rank, ok)
	}
	if _, ok := core.QueryRank([]string{"官方"}, []string{"官方", "蓝色"}); ok {
		t.Fatalf("缺一项查询词不该匹配")
	}
	if rank, ok := core.QueryRank([]string{"官方"}, nil); !ok || rank != core.MatchPrefix {
		t.Fatalf("空查询应当匹配一切: %d, %v", rank, ok)
	}
}

func TestValidationErrorTypes(t *testing.T) {
	t.Parallel()

	var validation *core.ValidationError
	var conflict *core.ConflictError
	// 错误只带文案键 + 参数；Error() 是给日志看的，所以给的是键而不是译文
	err := error(&core.ValidationError{Key: "valid.uuid"})
	if !asValidation(err, &validation) || validation.Key != "valid.uuid" {
		t.Fatalf("ValidationError 断言失败")
	}
	if got := validation.Error(); got != "valid.uuid" {
		t.Fatalf("Error() = %q，想要文案键", got)
	}
	if got := (error(&core.ValidationError{Key: "valid.tag-long", Args: []any{64}})).Error(); got != "valid.tag-long(64)" {
		t.Fatalf("带参数的 Error() = %q", got)
	}
	conflictErr := error(&core.ConflictError{Key: "valid.conflict", Args: []any{i18n.Key("field.username"), "steve"}})
	if asValidation(conflictErr, &validation) {
		t.Fatalf("ConflictError 不该被断言成 ValidationError（两者是并列的具体类型）")
	}
	if !asConflict(conflictErr, &conflict) || conflict.Key != "valid.conflict" {
		t.Fatalf("ConflictError 断言失败")
	}
	if got := conflict.Error(); got != "valid.conflict(field.username, steve)" {
		t.Fatalf("Error() = %q，参数里的文案键也应当原样给出", got)
	}
	// 用户可见的文案仍然按语言渲染（HTTP 层走这条）
	if got := i18n.T(i18n.ZhHans, conflict.Key, conflict.Args...); got != "账号名「steve」已被占用" {
		t.Fatalf("渲染文案 = %q", got)
	}
	if got := i18n.T(i18n.En, conflict.Key, conflict.Args...); got != "Account name “steve” is taken" {
		t.Fatalf("英文文案 = %q", got)
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
		func() error { _, _, _, err := core.NormalizeTexture("skin.steve", []byte("junk")); return err },
		func() error { _, _, _, err := core.NormalizeTexture("skin.steve", pngHeader(64, 64)); return err },
		func() error { _, _, _, err := core.NormalizeTexture("skin.steve", pngHeader(0, 0)); return err },
		func() error { _, _, _, err := core.NormalizeTexture("skin.steve", pngBytes(t, 128, 128)); return err },
		func() error { _, _, _, err := core.NormalizeTexture("cape", pngBytes(t, 64, 64)); return err },
		func() error {
			data := append(pngHeader(64, 64), make([]byte, core.MaxUploadBytes)...)
			_, _, _, err := core.NormalizeTexture("skin.steve", data)
			return err
		},
	}
	for index, build := range cases {
		t.Run(fmt.Sprintf("%d", index+1), func(t *testing.T) {
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
		})
	}
}
