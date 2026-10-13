package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"sync"
)

type Credential struct {
	UUID         string
	Username     string
	PasswordHash string
}

// 站点 token 的拼接分隔符：用不可见字符，避免字段内容里的冒号造成歧义。
const tokenSeparator = "\x1f"

const TokenSecretBytes = 32

// token = HMAC-SHA256(secret, uuid + "\x1f" + username + "\x1f" + password_hash)，secret 只活在内存里、
// 每次启动重新随机：所以光有库（uuid / 账号名 / password_hash 都在里头）
// 推不出别人的 token，而重启会让所有人的站点 token 一起失效。
type TokenStore struct {
	secret []byte
	mu     sync.RWMutex
	// 键是 token 的定长摘要：明文当键会让「长度 / 匹配了多少」从查找本身漏成时间差。
	byToken map[string]tokenEntry
}

// tokenEntry 的 digest 与键同源，取出后再恒时比较一次，避免把安全性建立在 map 的内部实现上。
type tokenEntry struct {
	digest [32]byte
	uuid   string
}

// tokenDigest 把任意长度的令牌折成固定长度，供查找与恒时比较共用。
func tokenDigest(token string) [32]byte {
	return sha256.Sum256([]byte(token))
}

func tokenKey(token string) string {
	digest := tokenDigest(token)
	return string(digest[:])
}

func NewTokenStore() (*TokenStore, error) {
	secret, err := RandomBytes(TokenSecretBytes)
	if err != nil {
		return nil, err
	}
	return &TokenStore{secret: secret, byToken: make(map[string]tokenEntry)}, nil
}

// TokenFor 算出某个账号的 token，但不改令牌表。
func (t *TokenStore) TokenFor(credential Credential) string {
	mac := hmac.New(sha256.New, t.secret)
	mac.Write([]byte(strings.Join([]string{credential.UUID, credential.Username, credential.PasswordHash}, tokenSeparator)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (t *TokenStore) Register(credential Credential) string {
	token := t.TokenFor(credential)
	digest := tokenDigest(token)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.byToken[string(digest[:])] = tokenEntry{digest: digest, uuid: credential.UUID}
	return token
}

// 密钥在本次进程里是固定的，所以没被改动的账号 token 不变、依旧有效；被改过 uuid /
// 账号名 / 密码的账号，旧 token 会在这里被丢掉 —— 也就是「改凭据即踢下线」。
func (t *TokenStore) Rebuild(credentials []Credential) {
	tokens := make(map[string]tokenEntry, len(credentials))
	for _, credential := range credentials {
		token := t.TokenFor(credential)
		digest := tokenDigest(token)
		tokens[string(digest[:])] = tokenEntry{digest: digest, uuid: credential.UUID}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.byToken = tokens
}

// Resolve 把 token 换成 uuid；不认识就返回空串。
func (t *TokenStore) Resolve(token string) string {
	if token == "" {
		return ""
	}
	digest := tokenDigest(token)
	t.mu.RLock()
	defer t.mu.RUnlock()
	entry, ok := t.byToken[string(digest[:])]
	if !ok || subtle.ConstantTimeCompare(entry.digest[:], digest[:]) != 1 {
		return ""
	}
	return entry.uuid
}
