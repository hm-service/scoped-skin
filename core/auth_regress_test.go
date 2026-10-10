package core_test

import (
	"strings"
	"testing"

	"scoped-skin/core"
)

// 回归（被截断的 PHC 哈希串）：`$argon2id$v=19$m=19456,t=2,p=1$<salt>$`（哈希段为空）
// 曾让 VerifyPassword 走进 argon2.IDKey(..., keyLen=0)：
// x/crypto/argon2 里 `blake2b.New(0, nil)` 返回 nil，紧接着 `b2.Write` 就是 nil 解引用 panic。
// 即便不 panic，两个空切片 ConstantTimeCompare 也相等 —— 等于任何密码都能进。
func TestVerifyPasswordRejectsTruncatedPHC(t *testing.T) {
	hash, err := core.HashPassword("steve123")
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Fatalf("PHC 串应当是 6 段: %q", hash)
	}
	// 保留 salt 段，砍掉哈希段 —— 正是「被截断」的样子
	salt := parts[4]
	truncated := "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$"

	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("VerifyPassword 对截断的 PHC 串 panic 了: %v", recovered)
			}
		}()
		if core.VerifyPassword("steve123", truncated) {
			t.Errorf("哈希段为空时必须返回 false，不能放行正确密码")
		}
		if core.VerifyPassword("任何别的密码", truncated) {
			t.Errorf("哈希段为空时任何密码都不该通过")
		}
		if core.VerifyPassword("", truncated) {
			t.Errorf("哈希段为空时空密码也不该通过")
		}
	}()
}

// VerifyPassword 的其余边界：段数、版本、参数、base64 都要挡在 argon2 之前/之后，
// 一律返回 false 而不是 panic。
func TestVerifyPasswordMalformedHashes(t *testing.T) {
	hash, err := core.HashPassword("steve123")
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	parts := strings.Split(hash, "$")
	salt := parts[4]

	cases := []struct {
		name     string
		password string
		phc      string
		want     bool
	}{
		{"正确密码往返", "steve123", hash, true},
		{"错误密码", "steve124", hash, false},
		{"空密码", "", hash, false},
		{"空串", "steve123", "", false},
		{"段数少一段", "steve123", "$argon2id$v=19$m=19456,t=2,p=1$" + salt, false},
		{"段数多一段", "steve123", hash + "$多余", false},
		{"算法不是 argon2id", "steve123", strings.Replace(hash, "argon2id", "argon2i", 1), false},
		{"首段非空", "steve123", "x" + hash, false},
		{"版本不对", "steve123", "$argon2id$v=16$m=19456,t=2,p=1$" + salt + "$aGFzaA", false},
		{"版本段不是数字", "steve123", "$argon2id$v=十九$m=19456,t=2,p=1$" + salt + "$aGFzaA", false},
		{"参数段残缺", "steve123", "$argon2id$v=19$m=19456$" + salt + "$aGFzaA", false},
		{"m=0", "steve123", "$argon2id$v=19$m=0,t=2,p=1$" + salt + "$aGFzaA", false},
		{"t=0", "steve123", "$argon2id$v=19$m=19456,t=0,p=1$" + salt + "$aGFzaA", false},
		{"p=0", "steve123", "$argon2id$v=19$m=19456,t=2,p=0$" + salt + "$aGFzaA", false},
		{"salt 非法 base64", "steve123", "$argon2id$v=19$m=19456,t=2,p=1$不是 base64$aGFzaA", false},
		{"哈希段非法 base64", "steve123", "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$!!!", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("VerifyPassword(%q) panic 了: %v", testCase.phc, recovered)
				}
			}()
			if got := core.VerifyPassword(testCase.password, testCase.phc); got != testCase.want {
				t.Errorf("VerifyPassword(%q, %q) = %v，想要 %v", testCase.password, testCase.phc, got, testCase.want)
			}
		})
	}
}
