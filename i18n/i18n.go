// Package i18n 是后端文案表：站点 API 里所有用户可见的文案都从这里取。
//
// 默认与源语言都是**简体中文**；英文是另一栏。Yggdrasil 那套错误文案由规范定死（英文），
// 不走这里。语言由请求的 Accept-Language 决定（见 api.WithLanguage），选择不落库、不跟账号。
package i18n

import (
	"fmt"
	"strings"
)

type Lang string

const (
	ZhHans Lang = "zh-Hans"
	En     Lang = "en"
)

// Default 是认不出请求语言时的兜底（也是源语言）。
const Default = ZhHans

// Key 包起来的参数表示「这个参数本身也是一个文案键」，渲染时先翻译它。
type Key string

type entry struct {
	zh string
	en string
}

// catalog 是全部文案。键用点号分层，改动时记得两边都写。
var catalog = map[string]entry{
	// ── 鉴权与身份组 ─────────────────────────────────────────────
	"auth.required":     {"未登录", "Not signed in"},
	"auth.stale":        {"登录状态无效，请重新登录", "Session is stale, sign in again"},
	"auth.admin":        {"需要管理员身份", "Admin only"},
	"auth.write":        {"当前身份组是只读的，不能修改任何资源", "This role is read-only and cannot modify anything"},
	"auth.login.failed": {"用户名或密码不正确", "Wrong account or password"},

	// ── 请求与字段 ──────────────────────────────────────────────
	"request.json":    {"请求体不是合法的 JSON", "Malformed JSON body"},
	"request.too-big": {"请求体不能超过 1 MB", "Request body is over 1 MB"},

	// ── 贴图 ───────────────────────────────────────────────────
	"upload.missing":   {"缺少上传文件", "Missing uploaded file"},
	"upload.too-big":   {"图片不能超过 32 KB", "Image is over 32 KB"},
	"upload.base64":    {"图片数据不是合法的 base64", "Image data is not valid base64"},
	"asset.unknown":    {"找不到该贴图", "Texture not found"},
	"asset.cannot-del": {"只读资源不能删除", "Read-only textures cannot be deleted"},
	"library.category": {"category 只能是 skin 或 cape", "category must be skin or cape"},
	"library.tab":      {"tab 只能是 all 或 favorites", "tab must be all or favorites"},

	// ── 账号字段校验（core / db 抛的键） ─────────────────────────
	"field.uuid":           {"UUID", "UUID"},
	"field.username":       {"账号名", "Account name"},
	"field.player_name":    {"游戏内名称", "Player name"},
	"valid.uuid":           {"UUID 必须是 32 位十六进制字符", "UUID must be 32 hex characters"},
	"valid.username.empty": {"账号名不能为空", "Account name is required"},
	"valid.username.long":  {"账号名最长 %d 个字符", "Account name is at most %d characters"},
	"valid.username.space": {"账号名不能包含空白字符", "Account name cannot contain spaces"},
	"valid.player_name":    {"游戏内名称只能是小写字母、数字或下划线，长度 1-16", "Player name takes lowercase letters, digits and underscore, 1–16 characters"},
	"valid.role":           {"身份组只能是 %s", "Role must be one of %s"},
	"valid.password.empty": {"密码不能为空", "Password is required"},
	"valid.password.short": {"密码至少 %d 个字符", "Password is at least %d characters"},
	"valid.password.long":  {"密码最长 %d 个字符", "Password is at most %d characters"},
	"valid.conflict":       {"%s「%s」已被占用", "%s “%s” is taken"},
	"valid.readonly":       {"只读资源不能修改", "Read-only textures cannot be modified"},
	"valid.last-admin":     {"这是最后一个管理员，不能降级", "The last admin cannot be demoted"},
	"valid.tag-count":      {"标签最多 %d 个", "At most %d tags"},
	"valid.tag-long":       {"单个标签最长 %d 个字符", "Each tag is at most %d characters"},

	// ── 贴图校验 ───────────────────────────────────────────────
	"valid.kind":     {"kind 只能是 %s", "kind must be one of %s"},
	"texture.png":    {"不是有效的 PNG 图片", "Not a valid PNG image"},
	"texture.size":   {"图片尺寸无效", "Invalid image dimensions"},
	"texture.skin":   {"皮肤必须是 64×64", "Skin must be 64×64"},
	"texture.cape":   {"披风必须是 64×32（宽 64、高 32）", "Cape must be 64×32 (64 wide, 32 tall)"},
	"texture.decode": {"PNG 数据已损坏，无法解码", "PNG data is corrupted and cannot be decoded"},

	// ── 账号与用户管理 ──────────────────────────────────────────
	"user.unknown":    {"找不到该用户", "Account not found"},
	"user.last-admin": {"这是最后一个管理员，不能删除", "The last admin cannot be deleted"},

	// ── 服务端内部错误（500） ────────────────────────────────────
	"store.tag-read":     {"读取标签失败", "Failed to read tags"},
	"store.list-read":    {"读取贴图列表失败", "Failed to list textures"},
	"store.stats-read":   {"读取统计失败", "Failed to read stats"},
	"store.texture-read": {"读取贴图失败", "Failed to read texture"},
	"store.texture-gone": {"贴图不存在", "Texture not found"},
	"store.upload":       {"上传贴图失败", "Upload failed"},
	"store.favorite":     {"切换收藏失败", "Failed to toggle favorite"},
	"store.delete":       {"删除贴图失败", "Failed to delete texture"},
	"store.equip":        {"装备贴图失败", "Failed to equip texture"},
	"store.user-read":    {"读取用户列表失败", "Failed to read accounts"},
	"store.user-write":   {"用户操作失败", "Account operation failed"},
	"store.user-delete":  {"删除用户失败", "Failed to delete account"},
	"store.token":        {"刷新令牌失败", "Failed to refresh tokens"},
}

// Parse 从 Accept-Language 里挑一档支持的语言：按出现顺序找第一个认得出来的，
// 一个都不认得就用默认（也是源语言）。
func Parse(header string) Lang {
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if tag == "" {
			continue
		}
		switch lower := strings.ToLower(tag); {
		case strings.HasPrefix(lower, "en"):
			return En
		case strings.HasPrefix(lower, "zh"):
			return ZhHans
		}
	}
	return Default
}

// Supported 返回支持的语言（按偏好顺序，第一档是源语言）。
func Supported() []Lang { return []Lang{ZhHans, En} }

func Normalize(value string) Lang {
	switch lower := strings.ToLower(strings.TrimSpace(value)); {
	case strings.HasPrefix(lower, "en"):
		return En
	case strings.HasPrefix(lower, "zh"):
		return ZhHans
	default:
		return Default
	}
}

func Has(key string) bool {
	_, ok := catalog[key]
	return ok
}

func Keys() []string {
	keys := make([]string, 0, len(catalog))
	for key := range catalog {
		keys = append(keys, key)
	}
	return keys
}

// T 取文案并按参数填占位符。键不在表里就原样返回键（便于一眼看出漏配）。
// 参数里包了 Key 的会先按同一语言翻译一遍。
func T(lang Lang, key string, args ...any) string {
	item, ok := catalog[key]
	if !ok {
		return key
	}
	template := item.zh
	if Normalize(string(lang)) == En {
		template = item.en
	}
	if len(args) == 0 {
		return template
	}
	resolved := make([]any, len(args))
	for index, arg := range args {
		if nested, ok := arg.(Key); ok {
			resolved[index] = T(lang, string(nested))
			continue
		}
		resolved[index] = arg
	}
	return fmt.Sprintf(template, resolved...)
}
