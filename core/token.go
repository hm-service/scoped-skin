package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// 站点会话凭据是无状态的：服务端不存会话表，凭据自带账号与过期时间，用
// SESSION_SIGNING_KEY 签名。代价是签出去的凭据撤不回来 —— 只能轮换 key 把所有人下线。
const (
	SessionTokenTTL  = 30 * 24 * time.Hour
	SessionNonceSize = 16
	sessionVersion   = "v1"
)

// 载荷与签名都用不带填充的 base64url：凭据要原样塞进 Authorization 头。
var sessionBase64 = base64.RawURLEncoding

// SiteTokens 签发与校验站点凭据：
//
//	base64url(uuid|exp|nonce) + "." + base64url(HMAC-SHA256(key, "v1|" + 载荷))
type SiteTokens struct {
	key []byte
}

func NewSiteTokens(key []byte) *SiteTokens { return &SiteTokens{key: key} }

// SessionKey 生成一把能直接填进 SESSION_SIGNING_KEY 的随机密钥（32 字节 = 64 位 hex）。
func SessionKey() (string, error) { return RandomHex(32) }

// Issue 给某个账号签一张凭据，过期时间为 now + SessionTokenTTL。
func (t *SiteTokens) Issue(uuid string, now time.Time) (string, error) {
	nonce, err := RandomBytes(SessionNonceSize)
	if err != nil {
		return "", err
	}
	payload := sessionBase64.EncodeToString([]byte(strings.Join([]string{
		uuid,
		strconv.FormatInt(now.Add(SessionTokenTTL).Unix(), 10),
		sessionBase64.EncodeToString(nonce),
	}, "|")))
	return payload + "." + sessionBase64.EncodeToString(t.sign(payload)), nil
}

// Verify 把凭据换成 uuid。先验签、再看过期：任一不过都返回空串。
func (t *SiteTokens) Verify(token string, now time.Time) string {
	payload, signature, found := strings.Cut(token, ".")
	if !found || payload == "" || signature == "" {
		return ""
	}
	got, err := sessionBase64.DecodeString(signature)
	if err != nil || subtle.ConstantTimeCompare(got, t.sign(payload)) != 1 {
		return ""
	}
	raw, err := sessionBase64.DecodeString(payload)
	if err != nil {
		return ""
	}
	exp, uuid, ok := parseSessionPayload(string(raw))
	if !ok || now.Unix() >= exp {
		return ""
	}
	return uuid
}

func (t *SiteTokens) sign(payload string) []byte {
	mac := hmac.New(sha256.New, t.key)
	mac.Write([]byte(sessionVersion + "|" + payload))
	return mac.Sum(nil)
}

// parseSessionPayload 拆开 uuid|exp|nonce。
func parseSessionPayload(raw string) (int64, string, bool) {
	uuid, rest, found := strings.Cut(raw, "|")
	if !found || uuid == "" {
		return 0, "", false
	}
	exp, _, found := strings.Cut(rest, "|")
	if !found {
		return 0, "", false
	}
	seconds, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return seconds, uuid, true
}
