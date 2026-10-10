package core_test

import (
	"testing"

	"scoped-skin/core"
)

// 回归（令牌表逐出 + 吊销）：Create 里 `tokens := append(s.byUser[uuid], ...)`
// 与 drop 的就地压缩可能共用同一块底层数组（append 复用了上一轮逐出后留下的空余容量），
// 压完再 `tokens = tokens[1:]` 会把列表中间那一项挤掉、末尾那项重复一次 ——
// 被挤掉的那条不在 byUser 里、却还留在 byToken 里，DeleteUser 于是吊销不掉它。
//
// 上限必须 ≥ 3 才复现：上限 2 时被挤掉的正好是表头，`tokens[1:]` 之后结果碰巧正确。
// 这里用上限 3 发 5 个令牌：正确结果只该留最后 3 个（下标 2/3/4），前两个都必须已经
// 查不到。修复前下标 1 会活下来（甚至在 DeleteUser 之后还挺着）。
func TestSessionsEvictionRevokesEveryIssuedToken(t *testing.T) {
	original := core.MaxTokensPerUser
	core.MaxTokensPerUser = 3
	t.Cleanup(func() { core.MaxTokensPerUser = original })

	store := core.NewSessions()
	const total = 5 // > 上限，保证至少逐出两次、能撞上就地压缩
	tokens := make([]string, 0, total)
	seen := map[string]bool{}
	for index := range total {
		accessToken, clientToken, err := store.Create("uuid-steve", "")
		if err != nil {
			t.Fatalf("颁发第 %d 个令牌失败: %v", index, err)
		}
		if accessToken == "" || clientToken == "" {
			t.Fatalf("第 %d 个令牌为空: %q / %q", index, accessToken, clientToken)
		}
		if seen[accessToken] {
			t.Fatalf("颁出了重复的 accessToken: %q", accessToken)
		}
		seen[accessToken] = true
		tokens = append(tokens, accessToken)
	}

	if count := store.Count("uuid-steve"); count != core.MaxTokensPerUser {
		t.Errorf("Count = %d，想要 %d", count, core.MaxTokensPerUser)
	}

	// 上限 3、发了 5 个：只有最后 3 个该活着，前两个必须已经被逐出。
	for index, accessToken := range tokens {
		_, live := store.Get(accessToken)
		wantLive := index >= total-core.MaxTokensPerUser
		if live != wantLive {
			t.Errorf("第 %d 个令牌 live = %v，想要 %v（上限 %d 时应当只留最后 %d 个）",
				index, live, wantLive, core.MaxTokensPerUser, core.MaxTokensPerUser)
		}
	}

	// DeleteUser 必须把**每一个**发出去的令牌都吊销掉。被逐出却没从 byToken 删干净
	// 的那条正是修复前漏网的那条。
	if revoked := store.DeleteUser("uuid-steve"); revoked != core.MaxTokensPerUser {
		t.Errorf("DeleteUser 返回 %d，想要 %d", revoked, core.MaxTokensPerUser)
	}
	for index, accessToken := range tokens {
		if session, ok := store.Get(accessToken); ok {
			t.Errorf("DeleteUser 之后第 %d 个令牌还能查到: %+v", index, session)
		}
	}
	if count := store.Count("uuid-steve"); count != 0 {
		t.Errorf("DeleteUser 之后 Count = %d，想要 0", count)
	}
	if again := store.DeleteUser("uuid-steve"); again != 0 {
		t.Errorf("重复 DeleteUser 返回 %d，想要 0", again)
	}
}
