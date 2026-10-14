package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"scoped-skin/builtin"
)

// TRUSTED_PROXIES 写错时启动必须失败，不能默默退回默认值（那会让公网部署以为
// 反代头被认了，实际上全按对端地址记 IP）。
func TestOpenRejectsInvalidTrustedProxies(t *testing.T) {
	_, err := Open(Config{
		DataDir:        filepath.Join(t.TempDir(), "data"),
		Version:        "test",
		BuiltinFS:      builtin.Files,
		TrustedProxies: "不是网段",
	})
	if err == nil {
		t.Fatalf("非法的 TRUSTED_PROXIES 应当让 Open 失败")
	}
}

// ArgonConcurrency 被配成 < 1 时至少留 1，否则信号量容量为 0、登录会永久阻塞。
func TestArgonConcurrencyClampedToAtLeastOne(t *testing.T) {
	original := ArgonConcurrency
	ArgonConcurrency = 0
	t.Cleanup(func() { ArgonConcurrency = original })

	instance := openInstance(t, func(cfg *Config) { cfg.AdminPassword = "password_1234" })
	if got := cap(instance.argon); got != 1 {
		t.Fatalf("信号量容量 = %d，想要 1", got)
	}

	done := make(chan bool, 1)
	go func() { done <- instance.VerifyCredentials("admin", "password_1234") != nil }()
	select {
	case ok := <-done:
		if !ok {
			t.Errorf("密码正确却没通过")
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("ArgonConcurrency < 1 时校验卡死了")
	}
}

// 首次启动：ADMIN_USERNAME 只影响账号登录名，角色名恒为 admin ——
// 所以像邮箱这种过不了角色名规则（有 @ 与 .）的账号名也能起号。
func TestFirstAdminPlayerNameIsFixed(t *testing.T) {
	instance := openInstance(t, func(cfg *Config) {
		cfg.AdminUsername = "first.admin@example.com"
		cfg.AdminPassword = "s3cret_password"
	})
	user := instance.Store.UserByUsername("first.admin@example.com")
	if user == nil {
		t.Fatalf("管理员账号没建出来")
	}
	if user.PlayerName != DefaultAdminUsername {
		t.Errorf("游戏内名称 = %q，想要 %q", user.PlayerName, DefaultAdminUsername)
	}
	if user.Role != "admin" {
		t.Errorf("role = %q，想要 admin", user.Role)
	}
}

// /readyz 的签名密钥断言：密钥没了就不能算就绪。
func TestReadyFailsWithoutSigningKeys(t *testing.T) {
	instance := openInstance(t, nil)
	if err := instance.Ready(); err != nil {
		t.Fatalf("初始就绪探针不该报错: %v", err)
	}
	keys := instance.Keys
	instance.Keys = nil
	if err := instance.Ready(); err == nil {
		t.Errorf("签名密钥没了，就绪探针还报就绪")
	}
	instance.Keys = keys
	if err := instance.Ready(); err != nil {
		t.Errorf("把密钥放回去之后应当恢复就绪: %v", err)
	}
}

// ResetSessions 要把「uuid 从库里消失」的账号的 Yggdrasil 内存会话一并清掉：
// accessToken 只活在内存里，不显式作废就会一直有效。
func TestResetSessionsRevokesSessionsOfVanishedUsers(t *testing.T) {
	instance := openInstance(t, func(cfg *Config) { cfg.AdminPassword = "password_1234" })

	steve, err := instance.Store.CreateUser("steve", "steve123", "steve", "normal")
	if err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	// 先重建一次，让 knownUUIDs 里记上 steve
	if err := instance.ResetSessions(); err != nil {
		t.Fatalf("重建 token 表失败: %v", err)
	}
	accessToken, _, err := instance.Sessions.Create(steve.UUID, "")
	if err != nil {
		t.Fatalf("造会话失败: %v", err)
	}
	if _, ok := instance.Sessions.Get(accessToken); !ok {
		t.Fatalf("刚造的会话就查不到")
	}

	// 绕过 HTTP 层直接从库里删号（模拟维护脚本 / 直接改库）
	if status, err := instance.Store.DeleteUser(steve.UUID); err != nil || status != "deleted" {
		t.Fatalf("删号失败: %q (%v)", status, err)
	}
	if err := instance.ResetSessions(); err != nil {
		t.Fatalf("重建 token 表失败: %v", err)
	}
	if session, ok := instance.Sessions.Get(accessToken); ok {
		t.Errorf("账号已从库里消失，会话却还活着: %+v", session)
	}
}

// App.ClientIP 把可信代理规则接出去给 HTTP 层用（api 与 yggdrasil 共用同一个入口）。
func TestClientIPUsesTrustedProxies(t *testing.T) {
	instance := openInstance(t, func(cfg *Config) { cfg.TrustedProxies = "192.0.2.1/32" })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.1:5555"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := instance.ClientIP(request); got != "198.51.100.7" {
		t.Errorf("可信代理下 ClientIP = %q，想要 198.51.100.7", got)
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "203.0.113.9:5555"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := instance.ClientIP(request); got != "203.0.113.9" {
		t.Errorf("不可信对端下 ClientIP = %q，想要 203.0.113.9", got)
	}
}
