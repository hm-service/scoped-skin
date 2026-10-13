package core_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"scoped-skin/core"
)

// password_hash 使用 Argon2id，参数 m=19456、t=2、p=1。
func TestHashPasswordIsArgon2idPHC(t *testing.T) {
	t.Parallel()

	hash, err := core.HashPassword("steve123")
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("哈希串前缀不对: %q", hash)
	}
	if strings.Contains(hash, "steve123") {
		t.Fatalf("哈希串里不该出现明文")
	}
	// 哈希串自带随机 salt
	again, err := core.HashPassword("steve123")
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	if again == hash {
		t.Fatalf("两次哈希结果相同，salt 没起作用")
	}

	if !core.VerifyPassword("steve123", hash) {
		t.Fatalf("正确密码没通过")
	}
	if core.VerifyPassword("steve124", hash) {
		t.Fatalf("错误密码通过了")
	}
	for index, broken := range []string{"", "plain", "$argon2id$v=19$m=19456,t=2,p=1$onlyfour", "$argon2i$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA"} {
		t.Run(fmt.Sprintf("%d_%s", index, broken), func(t *testing.T) {
			if core.VerifyPassword("steve123", broken) {
				t.Fatalf("损坏的哈希串 %q 不该通过", broken)
			}
		})
	}
}

// 账号验证恒定至少 1 秒。
func TestSleepUntilPadsSlowAndFastPaths(t *testing.T) {
	t.Parallel()

	start := time.Now()
	core.SleepUntil(start, 60*time.Millisecond)
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Fatalf("只等了 %v", elapsed)
	}

	// 已经花掉的时间要算进去，不能无脑再睡一遍
	start = time.Now()
	time.Sleep(80 * time.Millisecond)
	core.SleepUntil(start, 60*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("已经够久了还在睡: %v", elapsed)
	}
}

// 站点 token = HMAC-SHA256(进程密钥, uuid + "\x1f" + username + "\x1f" + password_hash)。
// 同一个进程里稳定；换一次启动（换密钥）就整批不一样 —— 库泄露也推不出 token。
func TestTokenIsStableWithinProcessAndBoundToSecret(t *testing.T) {
	t.Parallel()

	store, err := core.NewTokenStore()
	if err != nil {
		t.Fatalf("NewTokenStore: %v", err)
	}
	steve := core.Credential{UUID: "uuid-1", Username: "steve", PasswordHash: "$argon2id$..."}
	first := store.TokenFor(steve)
	if len(first) != 64 {
		t.Fatalf("token 应当是 64 位 hex: %q", first)
	}
	if first != store.TokenFor(steve) {
		t.Fatalf("同一进程里同样的输入要算出同样的 token")
	}
	// 分隔符要挡得住「字段内容里塞分隔符」这种拼接歧义
	for index, changed := range []core.Credential{
		{UUID: "uuid-2", Username: "steve", PasswordHash: "$argon2id$..."},
		{UUID: "uuid-1", Username: "alex", PasswordHash: "$argon2id$..."},
		{UUID: "uuid-1", Username: "steve", PasswordHash: "$argon2id$..!"},
	} {
		t.Run(fmt.Sprintf("%d_%s_%s", index, changed.UUID, changed.Username), func(t *testing.T) {
			if store.TokenFor(changed) == first {
				t.Fatalf("改一个字段就该换一个 token: %+v", changed)
			}
		})
	}

	restarted, err := core.NewTokenStore()
	if err != nil {
		t.Fatalf("NewTokenStore: %v", err)
	}
	if restarted.TokenFor(steve) == first {
		t.Fatalf("换一次启动就该换一批 token")
	}
}

func TestTokenStoreRebuildDropsChangedCredentials(t *testing.T) {
	t.Parallel()

	store, err := core.NewTokenStore()
	if err != nil {
		t.Fatalf("NewTokenStore: %v", err)
	}
	steve := core.Credential{UUID: "uuid-steve", Username: "steve", PasswordHash: "hash-steve"}
	alex := core.Credential{UUID: "uuid-alex", Username: "alex", PasswordHash: "hash-alex"}
	steveToken := store.Register(steve)
	alexToken := store.Register(alex)

	if resolved := store.Resolve(steveToken); resolved != "uuid-steve" {
		t.Fatalf("token 换 uuid 失败: %q", resolved)
	}
	if store.Resolve("不认识") != "" || store.Resolve("") != "" {
		t.Fatalf("未知 token 应当解析成空")
	}

	// 改了 steve 的密码：他旧 token 失效，alex 的照旧
	store.Rebuild([]core.Credential{
		{UUID: "uuid-steve", Username: "steve", PasswordHash: "hash-steve-v2"},
		alex,
	})
	if store.Resolve(steveToken) != "" {
		t.Fatalf("改过凭据的账号旧 token 应当失效")
	}
	if store.Resolve(alexToken) != "uuid-alex" {
		t.Fatalf("没改动的账号 token 不应当受影响")
	}
}

// token 查找走定长摘要：同长度、只差最后一位的近似 token 也要被拒。
func TestTokenResolveRejectsLookalike(t *testing.T) {
	t.Parallel()

	store, err := core.NewTokenStore()
	if err != nil {
		t.Fatalf("NewTokenStore: %v", err)
	}
	credential := core.Credential{UUID: "uuid-steve", Username: "steve", PasswordHash: "hash-steve"}
	token := store.Register(credential)
	if got := store.Resolve(token); got != credential.UUID {
		t.Fatalf("Resolve(正确 token) = %q，想要 %q", got, credential.UUID)
	}

	lookalike := token[:len(token)-1]
	if token[len(token)-1] == '0' {
		lookalike += "1"
	} else {
		lookalike += "0"
	}
	for name, candidate := range map[string]string{
		"同长度近似": lookalike,
		"截短":    token[:len(token)-1],
		"加长":    token + "0",
	} {
		t.Run(name, func(t *testing.T) {
			if got := store.Resolve(candidate); got != "" {
				t.Errorf("Resolve(%s) = %q，想要空", name, got)
			}
		})
	}
}

// Yggdrasil 规范的「兼容离线验证」：UUID 是角色名的一元函数。
func TestOfflineUUIDMatchesJava(t *testing.T) {
	t.Parallel()

	// Java: UUID.nameUUIDFromBytes(("OfflinePlayer:Notch").getBytes(UTF_8))
	cases := map[string]string{
		"Notch":      "b50ad385829d3141a2167e7d7539ba7f",
		"steve":      core.OfflineUUID("steve"),
		"dinnerbone": core.OfflineUUID("dinnerbone"),
	}
	if got := core.OfflineUUID("Notch"); got != cases["Notch"] {
		t.Fatalf("OfflineUUID(Notch) = %q，想要 %q", got, cases["Notch"])
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if len(value) != 32 || strings.Contains(value, "-") {
				t.Fatalf("%s 的 UUID 应当是 32 位无横线: %q", name, value)
			}
			if value != strings.ToLower(value) {
				t.Fatalf("%s 的 UUID 应当是小写: %q", name, value)
			}
		})
	}
	if core.OfflineUUID("steve") == core.OfflineUUID("alex") {
		t.Fatalf("不同角色名不该撞 UUID")
	}
	// 版本 3 / variant 标记
	if core.OfflineUUID("steve")[12] != '3' {
		t.Fatalf("应当是 version 3 UUID: %q", core.OfflineUUID("steve"))
	}
	if !strings.ContainsRune("89ab", rune(core.OfflineUUID("steve")[16])) {
		t.Fatalf("RFC 4122 variant 标记不对: %q", core.OfflineUUID("steve"))
	}
}

func TestDashlessUUID(t *testing.T) {
	t.Parallel()
	got := core.DashlessUUID("B50AD385-829D-3141-A216-7E7D7539BA7F")
	if got != "b50ad385829d3141a2167e7d7539ba7f" {
		t.Fatalf("DashlessUUID = %q", got)
	}
}

func TestRandomHex(t *testing.T) {
	t.Parallel()
	first, err := core.RandomHex(16)
	if err != nil {
		t.Fatalf("生成随机数失败: %v", err)
	}
	second, err := core.RandomHex(16)
	if err != nil {
		t.Fatalf("生成随机数失败: %v", err)
	}
	if len(first) != 32 || first == second {
		t.Fatalf("16 字节应当是 32 位 hex 且每次不同: %q / %q", first, second)
	}
}
