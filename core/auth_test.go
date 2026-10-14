package core_test

import (
	"encoding/base64"
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

// 站点凭据 = base64url(exp|uuid|nonce) + "." + base64url(HMAC-SHA256(key, "v1|" + 载荷))。
func TestSiteTokenRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	tokens := core.NewSiteTokens([]byte("key-a"))
	first, err := tokens.Issue("uuid-steve", now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if got := tokens.Verify(first, now); got != "uuid-steve" {
		t.Fatalf("Verify(刚签发) = %q，想要 uuid-steve", got)
	}

	payload, signature, found := strings.Cut(first, ".")
	if !found || payload == "" || signature == "" {
		t.Fatalf("凭据形状应当是 载荷.签名: %q", first)
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("载荷不是 base64url: %v", err)
	}
	fields := strings.Split(string(raw), "|")
	if len(fields) != 3 || fields[1] != "uuid-steve" {
		t.Fatalf("载荷 = %q，想要 exp|uuid|nonce", raw)
	}
	if want := fmt.Sprint(now.Add(core.SessionTokenTTL).Unix()); fields[0] != want {
		t.Fatalf("过期时间 = %q，想要 %q（30 天后）", fields[0], want)
	}
	nonce, err := base64.RawURLEncoding.DecodeString(fields[2])
	if err != nil || len(nonce) != core.SessionNonceSize {
		t.Fatalf("nonce 应当是 %d 字节的 base64url: %v", core.SessionNonceSize, err)
	}

	second, err := tokens.Issue("uuid-steve", now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if second == first {
		t.Fatalf("两次签发不能重复")
	}
}

// 先验签、再看过期：形状不对、签名对不上、载荷被动过，一律解析成空。
func TestSiteTokenRejectsTampered(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	tokens := core.NewSiteTokens([]byte("key-a"))
	token, err := tokens.Issue("uuid-steve", now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	payload, signature, _ := strings.Cut(token, ".")
	other, err := core.NewSiteTokens([]byte("key-b")).Issue("uuid-steve", now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	flipped := "A" + signature[1:]
	if signature[0] == 'A' {
		flipped = "B" + signature[1:]
	}
	for name, candidate := range map[string]string{
		"空串":             "",
		"没有签名":           payload,
		"签名被改":           payload + "." + flipped,
		"换成别人的凭据":        other,
		"载荷加料":           payload + "A." + signature,
		"签名不是 base64url": payload + ".!!!",
	} {
		t.Run(name, func(t *testing.T) {
			if got := tokens.Verify(candidate, now); got != "" {
				t.Errorf("Verify(%s) = %q，想要空", name, got)
			}
		})
	}
}

func TestSiteTokenExpiryAndSigningKey(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	tokens := core.NewSiteTokens([]byte("key-a"))
	token, err := tokens.Issue("uuid-steve", now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if got := tokens.Verify(token, now.Add(core.SessionTokenTTL-time.Second)); got != "uuid-steve" {
		t.Fatalf("过期前应当有效，得到 %q", got)
	}
	if got := tokens.Verify(token, now.Add(core.SessionTokenTTL)); got != "" {
		t.Fatalf("到点就应当失效，得到 %q", got)
	}
	// 同一把密钥（部署方填的 SESSION_SIGNING_KEY）在另一个实例里照样认这张凭据
	if got := core.NewSiteTokens([]byte("key-a")).Verify(token, now); got != "uuid-steve" {
		t.Fatalf("同一把密钥应当互相认账，得到 %q", got)
	}
	// 凭据是无状态的：轮换 SESSION_SIGNING_KEY 是唯一能把所有人下线的手段
	if got := core.NewSiteTokens([]byte("key-b")).Verify(token, now); got != "" {
		t.Fatalf("换一把密钥之后旧凭据必须失效，得到 %q", got)
	}
}

// 最短校验时间是 1 秒再加 0~200ms 抖动：固定值本身也是一种可观测的指纹。
func TestLoginFloorIsJittered(t *testing.T) {
	t.Parallel()

	seen := make(map[time.Duration]bool)
	for range 64 {
		floor := core.LoginFloor()
		if floor < core.LoginMinSeconds || floor > core.LoginMinSeconds+200*time.Millisecond {
			t.Fatalf("LoginFloor = %v，超出 [1s, 1.2s]", floor)
		}
		seen[floor] = true
	}
	if len(seen) < 2 {
		t.Fatalf("LoginFloor 64 次只给出 %d 个取值，抖动没生效", len(seen))
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
