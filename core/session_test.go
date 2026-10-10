package core_test

import (
	"testing"
	"time"

	"scoped-skin/core"
)

func TestSessionsLifecycle(t *testing.T) {
	store := core.NewSessions()

	accessToken, clientToken, err := store.Create("uuid-steve", "")
	if err != nil {
		t.Fatalf("颁发令牌失败: %v", err)
	}
	if len(accessToken) != 32 || len(clientToken) != 32 {
		t.Fatalf("令牌应当是 32 位无横线 hex: %q / %q", accessToken, clientToken)
	}
	session, ok := store.Get(accessToken)
	if !ok || session.PlayerUUID != "uuid-steve" || session.ClientToken != clientToken {
		t.Fatalf("会话内容不对: %+v", session)
	}
	if session.ExpiresAt.Before(time.Now().Add(core.SessionTTL - time.Minute)) {
		t.Fatalf("过期时间不对: %v", session.ExpiresAt)
	}

	// 客户端给的 clientToken 原样保留（规范：可以是任意字符串）
	_, given, err := store.Create("uuid-steve", "我的启动器")
	if err != nil {
		t.Fatalf("颁发令牌失败: %v", err)
	}
	if given != "我的启动器" {
		t.Fatalf("clientToken = %q", given)
	}

	store.Delete(accessToken)
	if _, ok := store.Get(accessToken); ok {
		t.Fatalf("吊销后还能查到")
	}
	store.Delete("本来就不存在") // 不该 panic

	if revoked := store.DeleteUser("uuid-steve"); revoked != 1 {
		t.Fatalf("应当吊销 1 条，实际 %d", revoked)
	}
	if store.Count("uuid-steve") != 0 {
		t.Fatalf("登出后不该还有令牌")
	}
}

// 规范：服务端也应对令牌数量加以限制，超出时先吊销最旧的。
func TestSessionsCapTokensPerUser(t *testing.T) {
	store := core.NewSessions()

	tokens := make([]string, 0, core.MaxTokensPerUser+1)
	for range core.MaxTokensPerUser + 1 {
		accessToken, _, err := store.Create("uuid-steve", "")
		if err != nil {
			t.Fatalf("颁发令牌失败: %v", err)
		}
		tokens = append(tokens, accessToken)
	}
	if count := store.Count("uuid-steve"); count != core.MaxTokensPerUser {
		t.Fatalf("令牌数 = %d，应当被压到 %d", count, core.MaxTokensPerUser)
	}
	// 最旧的那条被挤掉了，最新的还在
	if _, ok := store.Get(tokens[0]); ok {
		t.Fatalf("最旧的令牌应当被吊销")
	}
	if _, ok := store.Get(tokens[len(tokens)-1]); !ok {
		t.Fatalf("最新的令牌不该被吊销")
	}
}

func TestSessionsExpire(t *testing.T) {
	original := core.SessionTTL
	core.SessionTTL = 20 * time.Millisecond
	t.Cleanup(func() { core.SessionTTL = original })

	store := core.NewSessions()
	accessToken, _, err := store.Create("uuid-steve", "")
	if err != nil {
		t.Fatalf("颁发令牌失败: %v", err)
	}
	if _, ok := store.Get(accessToken); !ok {
		t.Fatalf("刚发的令牌就过期了")
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := store.Get(accessToken); ok {
		t.Fatalf("过期令牌应当查不到")
	}
	// 过期即顺手删掉，不留垃圾
	if count := store.Count("uuid-steve"); count != 0 {
		t.Fatalf("过期令牌没被清掉")
	}
}

func TestJoinsConsumeOnce(t *testing.T) {
	store := core.NewJoins()
	store.Record("uuid-steve", "serverId-1", "192.0.2.1")

	if !store.Consume("uuid-steve", "serverId-1", "") {
		t.Fatalf("暗号对不上")
	}
	// 核对一次即作废（防重放）
	if store.Consume("uuid-steve", "serverId-1", "") {
		t.Fatalf("重放不该通过")
	}

	store.Record("uuid-steve", "serverId-2", "192.0.2.1")
	if store.Consume("uuid-steve", "别的暗号", "") {
		t.Fatalf("serverId 不对却通过了")
	}
	if store.Consume("别的角色", "serverId-2", "") {
		t.Fatalf("角色不对却通过了")
	}
	// 服务端给了 ip 就核对；对不上时不消费掉暗号
	if store.Consume("uuid-steve", "serverId-2", "10.0.0.1") {
		t.Fatalf("IP 对不上却通过了")
	}
	if !store.Consume("uuid-steve", "serverId-2", "192.0.2.1") {
		t.Fatalf("IP 对得上应当通过")
	}
}

func TestJoinsExpire(t *testing.T) {
	original := core.JoinsTTL
	core.JoinsTTL = 20 * time.Millisecond
	t.Cleanup(func() { core.JoinsTTL = original })

	store := core.NewJoins()
	store.Record("uuid-steve", "serverId", "192.0.2.1")
	time.Sleep(30 * time.Millisecond)
	if store.Consume("uuid-steve", "serverId", "") {
		t.Fatalf("过期暗号不该通过")
	}
}
