package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"regexp"
	"strings"

	"scoped-skin/i18n"
)

// DESIGN.md 4.1 / 4.2 里的枚举量与字段上限。
const (
	MaxUsernameLength = 32
	MaxPasswordLength = 256

	// DESIGN.md 5.2：分块读（或 base64 解码后）超过这个上限就拒。哪怕 64×64 RGBA
	// 满熵（压不动）也才 16.5 KiB，32 KiB 留了一倍余量。
	MaxUploadBytes = 32 * 1024

	// DESIGN.md 4.1：密码长度下限，只起个「别用太短的」提醒作用。
	MinPasswordLength = 8

	// DESIGN.md 4.2：贴图用标签代替自由命名（库里已有的标签即词表，也可随时手动输入），
	// 但数量与单条长度都要有上限，否则标签栏就能拿来塞存储。
	MaxTagsPerAsset = 16
	MaxTagLength    = 64
)

// Kinds / Categories / Roles 是 DESIGN.md 4.2 与 5.1 的枚举量。
//
// `kind` 是库里存的那一列，把「是皮肤还是披风」与「皮肤的模型」合成一个值；
// `category` 是由它推出的粗分类（皮肤 / 披风），列表筛选与页面顶部的分类选择器用它。
var (
	Kinds      = []string{"skin.steve", "skin.alex", "cape"}
	Categories = []string{"skin", "cape"}
	Roles      = []string{"admin", "normal", "guest"}
)

func CategoryOf(kind string) string {
	if strings.HasPrefix(kind, "skin") {
		return "skin"
	}
	return "cape"
}

func KindsOf(category string) []string {
	if category == "cape" {
		return []string{"cape"}
	}
	return []string{"skin.steve", "skin.alex"}
}

// NormalizeKind 只接受 DESIGN.md 4.2 里那几个枚举值。
func NormalizeKind(value string) (string, error) {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	for _, known := range Kinds {
		if cleaned == known {
			return cleaned, nil
		}
	}
	return "", invalid("valid.kind", strings.Join(Kinds, " / "))
}

// DESIGN.md 5.2：只接受默认尺寸的贴图 —— 皮肤 64×64、披风 64×32（宽 64、高 32）。
// 只支持默认尺寸（不收 HD 贴图）顺带也就堵掉了「IHDR 里声明一个巨大尺寸」这类
// PNG 炸弹 —— 规范专门警告过「不先检查图像大小即可导致拒绝服务攻击」。
const (
	SkinSize   = 64
	CapeWidth  = 64
	CapeHeight = 32
)

const DefaultRole = "normal"

var (
	uuidPattern       = regexp.MustCompile(`^[0-9a-f]{32}$`)
	playerNamePattern = regexp.MustCompile(`^[a-z0-9_]{1,16}$`)
	pngMagic          = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
)

// ValidationError 输入不合法（贴图不是可用的 MC 贴图 / 账号字段不符合 DESIGN.md 的约束）。
//
// 只带**文案键 + 参数**，不带成品文案：渲染成哪种语言是 HTTP 层的事（见 i18n 包）。
type ValidationError struct {
	Key  string
	Args []any
}

func (e *ValidationError) Error() string { return keyText(e.Key, e.Args) }

// ConflictError 账号字段撞上唯一约束（uuid / player_name / username 已被占用）。
type ConflictError struct {
	Key  string
	Args []any
}

func (e *ConflictError) Error() string { return keyText(e.Key, e.Args) }

// keyText 是这两个错误的 Error()：它只进 slog（DESIGN.md 1.3 要求日志一律英文），
// 所以给的是文案键与参数而不是渲染后的文案 —— 译文由 HTTP 层按 Accept-Language 出。
func keyText(key string, args []any) string {
	if len(args) == 0 {
		return key
	}
	parts := make([]string, len(args))
	for index, arg := range args {
		if nested, ok := arg.(i18n.Key); ok {
			parts[index] = string(nested)
			continue
		}
		parts[index] = fmt.Sprint(arg)
	}
	return key + "(" + strings.Join(parts, ", ") + ")"
}

func invalid(key string, args ...any) *ValidationError {
	return &ValidationError{Key: key, Args: args}
}

// NormalizeUUID 统一成 32 位无横线小写；带横线的输入也接受。
func NormalizeUUID(value string) (string, error) {
	cleaned := DashlessUUID(strings.TrimSpace(value))
	if !uuidPattern.MatchString(cleaned) {
		return "", invalid("valid.uuid")
	}
	return cleaned, nil
}

func NormalizeUsername(value string) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return "", invalid("valid.username.empty")
	}
	if len([]rune(cleaned)) > MaxUsernameLength {
		return "", invalid("valid.username.long", MaxUsernameLength)
	}
	if strings.ContainsAny(cleaned, " \t\r\n\v\f") {
		return "", invalid("valid.username.space")
	}
	return cleaned, nil
}

// NormalizePlayerName 游戏内角色名：DESIGN.md 规定只能是小写字母 + 数字 + 下划线，最长 16。
func NormalizePlayerName(value string) (string, error) {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	if !playerNamePattern.MatchString(cleaned) {
		return "", invalid("valid.player_name")
	}
	return cleaned, nil
}

func NormalizeRole(value string) (string, error) {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	for _, role := range Roles {
		if cleaned == role {
			return cleaned, nil
		}
	}
	return "", invalid("valid.role", strings.Join(Roles, " / "))
}

func NormalizePassword(value string) (string, error) {
	if value == "" {
		return "", invalid("valid.password.empty")
	}
	if length := len([]rune(value)); length < MinPasswordLength {
		return "", invalid("valid.password.short", MinPasswordLength)
	} else if length > MaxPasswordLength {
		return "", invalid("valid.password.long", MaxPasswordLength)
	}
	return value, nil
}

func NormalizeTags(tags []string) ([]string, error) {
	result := cleanTags(tags)
	if len(result) > MaxTagsPerAsset {
		return nil, invalid("valid.tag-count", MaxTagsPerAsset)
	}
	for _, tag := range result {
		if len([]rune(tag)) > MaxTagLength {
			return nil, invalid("valid.tag-long", MaxTagLength)
		}
	}
	return result, nil
}

// CleanTags 是只规整不报错的版本：库里读出来的标签按入库时的约束本就是合法的，
// 这里再规整一遍只是兜住手工改库之类的意外，不该因此让整个列表接口失败。
func CleanTags(tags []string) []string {
	result := cleanTags(tags)
	if len(result) > MaxTagsPerAsset {
		result = result[:MaxTagsPerAsset]
	}
	kept := make([]string, 0, len(result))
	for _, tag := range result {
		if runes := []rune(tag); len(runes) > MaxTagLength {
			tag = string(runes[:MaxTagLength])
		}
		kept = append(kept, tag)
	}
	return kept
}

func cleanTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, raw := range tags {
		tag := strings.Join(strings.Fields(raw), " ")
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, tag)
	}
	return result
}

func SplitQuery(query string) []string {
	tokens := []string{}
	seen := map[string]bool{}
	for _, token := range strings.Fields(query) {
		key := strings.ToLower(token)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		tokens = append(tokens, token)
	}
	return tokens
}

// 匹配档位（越小越靠前），与前端 tags.js 的 matchTags 同一套定义。
const (
	MatchPrefix = iota
	MatchInfix
	MatchSuffix
)

// MatchRank 一个查询词与一个标签的匹配档位：前缀 / 中缀 / 后缀，不匹配返回 -1。
// 大小写不敏感；比较与「后缀」判定都在小写化后的串上做，免得 Unicode 折叠改变字节长度。
func MatchRank(tag, token string) int {
	haystack := strings.ToLower(tag)
	needle := strings.ToLower(token)
	at := strings.Index(haystack, needle)
	if at < 0 {
		return -1
	}
	if at == 0 {
		return MatchPrefix
	}
	if at+len(needle) == len(haystack) {
		return MatchSuffix
	}
	return MatchInfix
}

// QueryRank 标签交集：每一项查询词都要能命中条目的某个标签（子串匹配），
// 返回「各项里最差的那一档」；一项都没命中就返回 ok=false。
// 取最差档是为了让「每个词都靠前」的条目排在前面，而不是靠某一个词拉分。
func QueryRank(tags, tokens []string) (int, bool) {
	if len(tokens) == 0 {
		return MatchPrefix, true
	}
	worst := MatchPrefix
	for _, token := range tokens {
		best := -1
		for _, tag := range tags {
			if rank := MatchRank(tag, token); rank >= 0 && (best < 0 || rank < best) {
				best = rank
			}
		}
		if best < 0 {
			return 0, false
		}
		if best > worst {
			worst = best
		}
	}
	return worst, true
}

// HeaderSize 从 BLOB 前 24 字节里读 PNG 尺寸（列表查询用，避免把整张图读进内存）。
func HeaderSize(header []byte) (int, int) {
	if len(header) < 24 {
		return 0, 0
	}
	return int(binary.BigEndian.Uint32(header[16:20])), int(binary.BigEndian.Uint32(header[20:24]))
}

// DESIGN.md「资源信任边界」：上传只校验 PNG 头、尺寸与「能不能真的解码」，不做内容审核。
// 尺寸收得很紧：皮肤必须正好 64×64，披风必须正好 64×32，HD 贴图一律拒绝。
func ValidateTexture(kind string, data []byte) (int, int, error) {
	_, width, height, err := decodeTexture(kind, data)
	return width, height, err
}

// NormalizeTexture 校验后**解码再重新编码**，返回可以入库的字节与尺寸。
//
// DESIGN.md 4.2：入库的字节是重新编码过的，asset_id 取这串字节的 sha256 —— 所以它不再是
// 「上传文件本身的 sha256」。这么做的理由是规范要求的那句「除去其中任何与位图无关的数据」：
// Go 的 PNG 编码器不写任何辅助 chunk（gAMA / tIME / tEXt … 全没了），像素一个不动，而且
// 是个不动点（同一张图解码重编两轮字节相同），所以 asset_id 稳定、可去重。
func NormalizeTexture(kind string, data []byte) ([]byte, int, int, error) {
	img, width, height, err := decodeTexture(kind, data)
	if err != nil {
		return nil, 0, 0, err
	}
	var out bytes.Buffer
	if err := (&png.Encoder{}).Encode(&out, img); err != nil {
		return nil, 0, 0, err
	}
	return out.Bytes(), width, height, nil
}

// 头部看完还要真解：只信 IHDR 的话，一段伪造的头部后面跟一堆垃圾字节也能进库、再原样分发
// 给客户端；解码这一步把「声明自己 64×64 但其实不是图片」的输入挡在门外。
func decodeTexture(kind string, data []byte) (image.Image, int, int, error) {
	if _, err := NormalizeKind(kind); err != nil {
		return nil, 0, 0, err
	}
	if len(data) > MaxUploadBytes {
		return nil, 0, 0, invalid("upload.too-big")
	}
	if len(data) < 24 || !bytes.HasPrefix(data, pngMagic) || string(data[12:16]) != "IHDR" {
		return nil, 0, 0, invalid("texture.png")
	}
	width, height := HeaderSize(data)
	if width <= 0 || height <= 0 {
		return nil, 0, 0, invalid("texture.size")
	}
	if CategoryOf(kind) == "skin" && (width != SkinSize || height != SkinSize) {
		return nil, 0, 0, invalid("texture.skin")
	}
	if kind == "cape" && (width != CapeWidth || height != CapeHeight) {
		return nil, 0, 0, invalid("texture.cape")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, invalid("texture.decode")
	}
	return img, width, height, nil
}
