package db_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"scoped-skin/builtin"
	"scoped-skin/core"
	"scoped-skin/db"
)

// 回归（最后一个管理员不可降级）：库里只剩 1 个 admin 时，改成 guest 和 normal
// 都必须被拒。修复前只挡了 normal，改成 guest 会把管理员清零，等于把自己锁在门外。
func TestLastAdminCannotBeDemotedToGuest(t *testing.T) {
	store := openStore(t)
	admin := store.UserByUsername("admin")
	if admin == nil || admin.Role != "admin" {
		t.Fatalf("测试账号里应当有一个 admin: %+v", admin)
	}
	if count := store.AdminCount(); count != 1 {
		t.Fatalf("初始管理员数 = %d，想要 1", count)
	}

	for _, role := range []string{"guest", "normal"} {
		updated, err := store.UpdateUser(admin.UUID, map[string]string{"role": role})
		var invalid *core.ValidationError
		if !errors.As(err, &invalid) || invalid.Key != "valid.last-admin" {
			t.Errorf("把最后一个管理员降成 %s: err = %v，想要 *core.ValidationError{valid.last-admin}", role, err)
		}
		if updated != nil {
			t.Errorf("降成 %s 被拒时不该返回改过的用户: %+v", role, updated)
		}
	}
	// 被拒之后库里状态不能被动过
	if after := store.User(admin.UUID); after == nil || after.Role != "admin" {
		t.Fatalf("被拒的降级却改动了库里状态: %+v", after)
	}
	if count := store.AdminCount(); count != 1 {
		t.Fatalf("被拒之后管理员数 = %d，想要 1", count)
	}

	// 有第二个管理员之后就能降级
	steve := store.UserByUsername("steve")
	if _, err := store.UpdateUser(steve.UUID, map[string]string{"role": "admin"}); err != nil {
		t.Fatalf("提拔第二个管理员失败: %v", err)
	}
	if count := store.AdminCount(); count != 2 {
		t.Fatalf("提拔之后管理员数 = %d，想要 2", count)
	}
	demoted, err := store.UpdateUser(admin.UUID, map[string]string{"role": "guest"})
	if err != nil || demoted == nil || demoted.Role != "guest" {
		t.Fatalf("还有别的管理员时应当能降成 guest: %+v (%v)", demoted, err)
	}
	if count := store.AdminCount(); count != 1 {
		t.Errorf("降级之后管理员数 = %d，想要 1", count)
	}
}

// 账号查找的边界：空名 / 空白 / 带横线的 uuid / 非法 uuid。
func TestUserLookupEdges(t *testing.T) {
	store := openStore(t)

	if count, err := store.UserCount(); err != nil || count != 6 {
		t.Errorf("UserCount = %d (%v)，想要 6", count, err)
	}
	if store.UserByPlayerName("") != nil || store.UserByPlayerName("   ") != nil {
		t.Errorf("空角色名不该查到任何账号")
	}
	if store.UserByPlayerName("nobody") != nil {
		t.Errorf("不存在的角色名不该查到账号")
	}
	// 大小写不敏感
	if store.UserByPlayerName("  STEVE  ") == nil {
		t.Errorf("按角色名查询应当大小写不敏感且 trim")
	}
	steve := store.UserByUsername("steve")
	if steve == nil {
		t.Fatalf("steve 不存在")
	}
	// 带横线的 uuid 也认
	dashed := steve.UUID[0:8] + "-" + steve.UUID[8:12] + "-" + steve.UUID[12:16] + "-" +
		steve.UUID[16:20] + "-" + steve.UUID[20:32]
	if got := store.User(dashed); got == nil || got.UUID != steve.UUID {
		t.Errorf("带横线的 UUID 应当能查到: %+v", got)
	}
	for _, bad := range []string{"", "   ", "not-a-uuid", "0123456789abcdef"} {
		if got := store.User(bad); got != nil {
			t.Errorf("非法 UUID %q 不该查到: %+v", bad, got)
		}
	}
}

// UpdateUser 的空操作语义：没有可改的键就原样返回；password 为空串表示「不改密码」。
func TestUpdateUserNoOpChanges(t *testing.T) {
	store := openStore(t)
	steve := store.UserByUsername("steve")

	same, err := store.UpdateUser(steve.UUID, map[string]string{})
	if err != nil || same == nil || same.UUID != steve.UUID || same.PasswordHash != steve.PasswordHash {
		t.Fatalf("空 changes 应当原样返回当前用户: %+v (%v)", same, err)
	}
	if _, err := store.UpdateUser(steve.UUID, map[string]string{"password": ""}); err != nil {
		t.Fatalf("password 为空串应当按「不改」处理，不该报错: %v", err)
	}
	if store.Authenticate("steve", "steve123") == nil {
		t.Errorf("password 为空串把原密码清掉了")
	}
	// 其余字段给空串就是非法输入，不是空操作
	if _, err := store.UpdateUser(steve.UUID, map[string]string{"role": ""}); err == nil {
		t.Errorf("空身份组应当被拒")
	}
	if _, err := store.UpdateUser(steve.UUID, map[string]string{"username": ""}); err == nil {
		t.Errorf("空账号名应当被拒")
	}
}

// 装备披风要写 cape 列、不碰 skin 列（Equip 按 asset 的 category 选列）。
func TestEquipCapeGoesToCapeColumn(t *testing.T) {
	store := openStore(t)
	cape, status, err := store.AddUpload("cape", "share", nil, pngBytes(t, 64, 32))
	if err != nil || status != "created" {
		t.Fatalf("传披风失败: %q (%v)", status, err)
	}
	steve := store.UserByUsername("steve")

	if status, err := store.Equip(cape.ID, steve.UUID); err != nil || status != "ok" {
		t.Fatalf("装备披风失败: %q (%v)", status, err)
	}
	after := store.User(steve.UUID)
	if after.Cape != cape.ID {
		t.Errorf("cape 列 = %q，想要 %q", after.Cape, cape.ID)
	}
	if after.Skin != "" {
		t.Errorf("装备披风不该动 skin 列: %q", after.Skin)
	}
}

// 手工把 tags 改成非法 JSON（模拟改库 / 旧版本残留）之后，读取路径必须退化成空标签
// 而不是让整个列表 / 统计接口失败。
func TestMalformedTagJSONDegradesGracefully(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	if err := store.EnsureBuiltins(builtin.Files); err != nil {
		t.Fatalf("灌内置资源失败: %v", err)
	}
	items, err := store.List("skin", "all", "", "")
	if err != nil || len(items) < 2 {
		t.Fatalf("内置资源没灌进来: %d (%v)", len(items), err)
	}
	assetID := items[0].ID
	emptyTagAssetID := items[1].ID
	if err := store.Close(); err != nil {
		t.Fatalf("关库失败: %v", err)
	}

	const broken = "{这显然不是 JSON"
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("直连数据库失败: %v", err)
	}
	if _, err := handle.Exec("UPDATE asset SET tags = ? WHERE asset_id = ?", broken, assetID); err != nil {
		handle.Close()
		t.Fatalf("改 tags 失败: %v", err)
	}
	// 空串也是一种「手工改库」留下的状态：读取路径同样要退化成空标签
	if _, err := handle.Exec("UPDATE asset SET tags = '' WHERE asset_id = ?", emptyTagAssetID); err != nil {
		handle.Close()
		t.Fatalf("清空 tags 失败: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("关直连失败: %v", err)
	}

	reopened, err := db.Open(path)
	if err != nil {
		t.Fatalf("重开数据库失败: %v", err)
	}
	defer reopened.Close()

	item, err := reopened.Item(assetID, "")
	if err != nil {
		t.Fatalf("坏 tags 不该让单条查询失败: %v", err)
	}
	if item == nil {
		t.Fatalf("条目不该消失")
	}
	if len(item.Tags) != 0 {
		t.Errorf("坏 tags 应当退化成空标签，实际 %v", item.Tags)
	}
	emptyItem, err := reopened.Item(emptyTagAssetID, "")
	if err != nil {
		t.Fatalf("空 tags 不该让单条查询失败: %v", err)
	}
	if emptyItem == nil || len(emptyItem.Tags) != 0 {
		t.Errorf("空串 tags 应当退化成空标签，实际 %+v", emptyItem)
	}

	listed, err := reopened.List("skin", "all", "", "")
	if err != nil || len(listed) != len(items) {
		t.Fatalf("坏 tags 不该让列表失败: %d (%v)", len(listed), err)
	}
	counts, err := reopened.TagCounts()
	if err != nil {
		t.Fatalf("坏 tags 不该让标签统计失败: %v", err)
	}
	for _, count := range counts {
		if count.Tag == broken {
			t.Errorf("非法 tags 的字面量不该出现在标签统计里: %+v", count)
		}
	}
	if _, err := reopened.Texture(assetID); err != nil {
		t.Errorf("坏 tags 不该影响贴图字节读取: %v", err)
	}
}
