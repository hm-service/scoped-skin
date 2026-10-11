package core_test

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"scoped-skin/core"
)

// 入库字节是「解码 → 重新编码」的结果，必须是不动点：否则同一张图两次上传会得到
// 两个 asset_id，内容寻址与去重都会失效（db.AddUpload 用它算 sha256）。
func TestNormalizeTextureIsFixedPoint(t *testing.T) {
	original := pngBytes(t, core.SkinSize, core.SkinSize)

	first, width, height, err := core.NormalizeTexture("skin.steve", original)
	if err != nil {
		t.Fatalf("规范化失败: %v", err)
	}
	if width != core.SkinSize || height != core.SkinSize {
		t.Fatalf("尺寸 = %d×%d，想要 %d×%d", width, height, core.SkinSize, core.SkinSize)
	}
	second, _, _, err := core.NormalizeTexture("skin.steve", first)
	if err != nil {
		t.Fatalf("二次规范化失败: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("规范化不是不动点：%d 字节 → %d 字节，asset_id 会飘",
			len(first), len(second))
	}
	// 重新规范化出来的必须还是一张能解开的 64×64 PNG
	decoded, err := png.Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("规范化后的字节解不开: %v", err)
	}
	if bounds := decoded.Bounds(); bounds.Dx() != core.SkinSize || bounds.Dy() != core.SkinSize {
		t.Errorf("规范化后的尺寸 = %d×%d", bounds.Dx(), bounds.Dy())
	}
}

// 上限按**字符**算而不是字节：多字节字符正好到上限要能过，多一个字符才被拒。
func TestFieldLimitsCountRunesNotBytes(t *testing.T) {
	if _, err := core.NormalizeUsername(strings.Repeat("a", core.MaxUsernameLength)); err != nil {
		t.Errorf("%d 个字符的账号名应当通过: %v", core.MaxUsernameLength, err)
	}
	if _, err := core.NormalizeUsername(strings.Repeat("a", core.MaxUsernameLength+1)); err == nil {
		t.Errorf("%d 个字符的账号名应当被拒", core.MaxUsernameLength+1)
	}
	// 密码现在是白名单字符集（只收 ASCII），「按字符不按字节」由下面的标签来证
	if _, err := core.NormalizePassword(strings.Repeat("a", core.MaxPasswordLength)); err != nil {
		t.Errorf("%d 个字符的密码应当通过: %v", core.MaxPasswordLength, err)
	}
	if _, err := core.NormalizePassword(strings.Repeat("a", core.MaxPasswordLength+1)); err == nil {
		t.Errorf("%d 个字符的密码应当被拒", core.MaxPasswordLength+1)
	}
	if tags, err := core.NormalizeTags(nil); err != nil || len(tags) != 0 {
		t.Errorf("0 个标签应当通过且回空: %v (%v)", tags, err)
	}
	if got := core.CleanTags(nil); len(got) != 0 {
		t.Errorf("CleanTags(nil) = %v，想要空", got)
	}
	if _, err := core.NormalizeTags([]string{strings.Repeat("标", core.MaxTagLength)}); err != nil {
		t.Errorf("%d 个字符的标签应当通过: %v", core.MaxTagLength, err)
	}
	if _, err := core.NormalizeTags([]string{strings.Repeat("标", core.MaxTagLength+1)}); err == nil {
		t.Errorf("%d 个字符的标签应当被拒", core.MaxTagLength+1)
	}

	// HeaderSize 的 24 字节边界：23 字节读不出、24 字节正好读出
	header := make([]byte, 24)
	header[16], header[19] = 0, 64
	header[20], header[23] = 0, 32
	if width, height := core.HeaderSize(header); width != 64 || height != 32 {
		t.Errorf("正好 24 字节应当读出 64×32，实际 %d×%d", width, height)
	}
	if width, height := core.HeaderSize(header[:23]); width != 0 || height != 0 {
		t.Errorf("23 字节应当回 0×0，实际 %d×%d", width, height)
	}
}
