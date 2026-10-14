package core

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	randv2 "math/rand/v2"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// password_hash 使用 Argon2id 算法进行哈希，默认取
// m=19456 (19 MiB), t=2, p=1。
const (
	argonMemory  uint32 = 19456
	argonTime    uint32 = 2
	argonThreads uint8  = 1
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// 账号验证的最短耗时：1 秒再加 0~200ms 的抖动 —— 固定值本身也是一种可观测的指纹。
const (
	LoginMinSeconds = time.Second
	loginJitterMax  = 200 * time.Millisecond
)

// LoginFloor 给出本次校验该等多久。
func LoginFloor() time.Duration {
	return LoginMinSeconds + time.Duration(randv2.Int64N(int64(loginJitterMax)+1))
}

// PHC 字符串里的 base64 不带填充（与 argon2-cffi 一致）。
var phcBase64 = base64.RawStdEncoding

// HashPassword 明文 -> Argon2id PHC 哈希串，形如
// `$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>`。
func HashPassword(password string) (string, error) {
	salt, err := RandomBytes(argonSaltLen)
	if err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		phcBase64.EncodeToString(salt), phcBase64.EncodeToString(key)), nil
}

func VerifyPassword(password, passwordHash string) bool {
	parts := strings.Split(passwordHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory, timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return false
	}
	if memory == 0 || timeCost == 0 || threads == 0 {
		return false
	}
	salt, err := phcBase64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := phcBase64.DecodeString(parts[5])
	if err != nil {
		return false
	}
	// 哈希段为空（被截断的 PHC 串）必须当成损坏：argon2.IDKey 的 keyLen 为 0 会 panic，
	// 就算不 panic，ConstantTimeCompare 对两个空切片也返回相等 —— 等于放行任何密码。
	if len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// SleepUntil 把一次校验补到 `min` 那么长，用来抹掉「不存在的账号秒回」这个时序侧信道。
func SleepUntil(start time.Time, min time.Duration) {
	if remaining := min - time.Since(start); remaining > 0 {
		time.Sleep(remaining)
	}
}

func RandomBytes(size int) ([]byte, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return nil, fmt.Errorf("random source unavailable: %w", err)
	}
	return buffer, nil
}

// RandomHex 生成 size 字节的随机数，返回 2×size 位小写 hex 串
// （Yggdrasil 的 accessToken / clientToken 就是 16 字节 = 32 位无横线 hex）。
func RandomHex(size int) (string, error) {
	buffer, err := RandomBytes(size)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
