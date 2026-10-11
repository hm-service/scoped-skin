package db_test

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"scoped-skin/builtin"
	"scoped-skin/core"
	"scoped-skin/db"
)

func openStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	seedUsers(t, store)
	if err := store.EnsureBuiltins(builtin.Files); err != nil {
		t.Fatalf("灌内置资源失败: %v", err)
	}
	return store
}

// seedUsers 播种与 internal/testenv 同一套账号（生产代码不再内置任何账号）。
func seedUsers(t *testing.T, store *db.Store) {
	t.Helper()
	for _, account := range []struct {
		username, password, playerName, role string
	}{
		{"admin", "admin123", "admin", "admin"},
		{"steve", "steve123", "steve", "normal"},
		{"alex", "alex12345", "alex", "normal"},
		{"notch", "notch123", "notch", "normal"},
		{"dinnerbone", "dinnerbone123", "dinnerbone", "normal"},
		{"guest", "guest123", "guest", "guest"},
	} {
		if _, err := store.CreateUser(account.username, account.password, account.playerName, account.role); err != nil {
			t.Fatalf("建测试账号 %s 失败: %v", account.username, err)
		}
	}
}

func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	canvas.Set(0, 0, color.RGBA{R: 0x2f, G: 0xbf, B: 0x71, A: 0xff})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("编码 PNG 失败: %v", err)
	}
	return buffer.Bytes()
}

// 内容寻址的库要靠不同像素才能建出多条不同的贴图。
func pngWithPixel(t *testing.T, seed byte) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, core.SkinSize, core.SkinSize))
	canvas.Set(int(seed)%core.SkinSize, 0, color.RGBA{R: seed, G: 0x40, B: 0xff, A: 0xff})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("编码 PNG 失败: %v", err)
	}
	return buffer.Bytes()
}

func TestEnsureBuiltinsIsIdempotent(t *testing.T) {
	t.Parallel()
	store := openStore(t)

	items, err := store.List("skin", "all", "", "")
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if len(items) != 9 {
		t.Fatalf("内置皮肤应当有 9 张，实际 %d", len(items))
	}
	// 按上传时间倒序（upload_at 相同则按自增 id 倒序），而且全是只读的内置资源
	for _, item := range items {
		if !item.Readonly {
			t.Fatalf("只读标记丢了: %+v", item)
		}
		if item.Kind != "skin.steve" && item.Kind != "skin.alex" {
			t.Fatalf("内置皮肤的 kind 不对: %+v", item)
		}
		if item.Category != "skin" || item.Width != 64 || item.Height != 64 {
			t.Fatalf("条目字段不对: %+v", item)
		}
		if item.TextureURL != "/asset/"+item.ID || len(item.ID) != 64 {
			t.Fatalf("内容寻址字段不对: %+v", item)
		}
		if len(item.Tags) == 0 {
			t.Fatalf("内置资源应当带标签: %+v", item)
		}
	}
	if items[0].UploadAt < items[len(items)-1].UploadAt {
		t.Fatalf("没有按上传时间倒序")
	}

	// 再灌一次不该重复
	if err := store.EnsureBuiltins(builtin.Files); err != nil {
		t.Fatalf("重复灌内置资源失败: %v", err)
	}
	items, err = store.List("skin", "all", "", "")
	if err != nil || len(items) != 9 {
		t.Fatalf("重复灌之后条数 = %d (%v)", len(items), err)
	}
	stats, err := store.Stats("")
	if err != nil || stats.Skins != 9 || stats.Capes != 0 {
		t.Fatalf("统计不对: %+v (%v)", stats, err)
	}
}

func TestCreateUserStoresArgon2idHash(t *testing.T) {
	t.Parallel()
	store := openStore(t)

	users, err := store.Users()
	if err != nil || len(users) != 6 {
		t.Fatalf("测试账号应当有 6 个，实际 %d (%v)", len(users), err)
	}
	roles := map[string]string{}
	for _, user := range users {
		roles[user.Username] = user.Role
		if user.UUID != core.OfflineUUID(user.PlayerName) {
			t.Fatalf("UUID 应当按离线模式规则推导: %+v", user)
		}
	}
	if roles["admin"] != "admin" || roles["guest"] != "guest" || roles["steve"] != "normal" {
		t.Fatalf("身份组不对: %v", roles)
	}

	// 密码是 Argon2id 哈希，且能校验通过
	steve := store.UserByUsername("steve")
	if steve == nil || !core.VerifyPassword("steve123", steve.PasswordHash) {
		t.Fatalf("密码校验失败")
	}
	if store.Authenticate("steve", "steve123") == nil {
		t.Fatalf("Authenticate 应当通过")
	}
	if store.Authenticate("steve", "错密码") != nil || store.Authenticate("不存在", "x") != nil {
		t.Fatalf("错误凭据不该通过")
	}

	// 账号名唯一：重复建号会被拒，且不动已有的行
	if _, err := store.CreateUser("steve", "hunter2x", "steve2", "normal"); err == nil {
		t.Fatalf("重复的账号名应当被拒")
	}
	if again := store.UserByUsername("steve"); again.PasswordHash != steve.PasswordHash {
		t.Fatalf("重复建号把密码哈希改掉了")
	}
}

func TestUserCRUD(t *testing.T) {
	t.Parallel()
	store := openStore(t)

	created, err := store.CreateUser("newbie@example.com", "hunter2x", "Newbie", "normal")
	if err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	// 游戏内名称大小写原样保留，离线 UUID 按存下来的名字推
	if created.PlayerName != "Newbie" || created.UUID != core.OfflineUUID("Newbie") ||
		created.Role != "normal" || created.Skin != "" {
		t.Fatalf("新账号字段不对: %+v", created)
	}
	// 大小写不敏感的唯一性：Newbie 已存在，newbie 也要撞（建表那条 UNIQUE 是二进制比较，拦不住）
	if _, err := store.CreateUser("newbie2", "hunter2x", "newbie", "normal"); err == nil {
		t.Fatalf("只差大小写的重名账号不该建得出来")
	}
	// 唯一约束（改自己的 uuid 是空操作，不算冲突，所以拿别人的来撞）
	for _, clash := range []map[string]string{
		{"username": "steve"},
		{"player_name": "Steve"},
		{"uuid": store.UserByUsername("alex").UUID},
	} {
		_, err := store.UpdateUser(created.UUID, clash)
		if _, ok := errors.AsType[*core.ConflictError](err); !ok {
			t.Fatalf("%v 应当撞唯一约束，实际 %v", clash, err)
		}
	}
	if _, err := store.CreateUser("steve", "x", "steve2", "normal"); err == nil {
		t.Fatalf("重名账号不该建得出来")
	}
	if _, err := store.CreateUser("bad name", "x", "ok", "normal"); err == nil {
		t.Fatalf("非法账号名不该建得出来")
	}

	updated, err := store.UpdateUser(created.UUID, map[string]string{
		"username": "renamed@example.com", "player_name": "renamed", "role": "guest",
	})
	if err != nil || updated == nil {
		t.Fatalf("改账号失败: %v", err)
	}
	if updated.Username != "renamed@example.com" || updated.PlayerName != "renamed" || updated.Role != "guest" {
		t.Fatalf("改完的字段不对: %+v", updated)
	}
	// 换成新 UUID 时收藏要跟着走（无外键，靠代码保证）
	moved, err := store.UpdateUser(updated.UUID, map[string]string{"uuid": "0123456789abcdef0123456789abcdef"})
	if err != nil || moved.UUID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("改 UUID 失败: %+v (%v)", moved, err)
	}
	if store.User(updated.UUID) != nil {
		t.Fatalf("旧 UUID 应当查不到了")
	}
	if gone, err := store.UpdateUser("ffffffffffffffffffffffffffffffff", map[string]string{"role": "guest"}); err != nil || gone != nil {
		t.Fatalf("改不存在的账号应当回 (nil, nil): %+v (%v)", gone, err)
	}

	if _, err := store.UpdateUser(moved.UUID, map[string]string{"password": "changed_12345"}); err != nil {
		t.Fatalf("改密码失败: %v", err)
	}
	if store.Authenticate("renamed@example.com", "hunter2x") != nil {
		t.Fatalf("旧密码还能用")
	}
	if store.Authenticate("renamed@example.com", "changed_12345") == nil {
		t.Fatalf("新密码用不了")
	}

	if status, err := store.DeleteUser(moved.UUID); err != nil || status != "deleted" {
		t.Fatalf("删号失败: %q (%v)", status, err)
	}
	if store.User(moved.UUID) != nil {
		t.Fatalf("删完还能查到")
	}
	if status, err := store.DeleteUser(moved.UUID); err != nil || status != "missing" {
		t.Fatalf("删不存在的账号 = %q (%v)", status, err)
	}
}

// DESIGN.md 5：最后一个管理员不允许删除或降级，避免把自己锁在门外。
func TestLastAdminIsProtected(t *testing.T) {
	t.Parallel()
	store := openStore(t)

	if count := store.AdminCount(); count != 1 {
		t.Fatalf("初始管理员数 = %d", count)
	}
	admin := store.UserByUsername("admin")
	if _, err := store.UpdateUser(admin.UUID, map[string]string{"role": "normal"}); err == nil {
		t.Fatalf("最后一个管理员不该能降级")
	}
	if status, err := store.DeleteUser(admin.UUID); err != nil || status != "last_admin" {
		t.Fatalf("最后一个管理员不该能删除: %q (%v)", status, err)
	}

	// 先提拔一个，再降级/删除原来的就没问题了
	promoted, err := store.UpdateUser(core.OfflineUUID("steve"), map[string]string{"role": "admin"})
	if err != nil || promoted.Role != "admin" {
		t.Fatalf("提拔管理员失败: %+v (%v)", promoted, err)
	}
	if _, err := store.UpdateUser(admin.UUID, map[string]string{"role": "normal"}); err != nil {
		t.Fatalf("还有别的管理员时应当能降级: %v", err)
	}
}

func TestUploadAndUpdateTexture(t *testing.T) {
	t.Parallel()
	store := openStore(t)

	skin := pngBytes(t, 64, 64)
	item, status, err := store.AddUpload("skin.steve", []string{"自建", "蓝色"}, skin)
	if err != nil || status != "created" {
		t.Fatalf("上传失败: %q (%v)", status, err)
	}
	// 入库的是规范化后的字节，id 是它的 sha256（Go 编出来的 PNG 本来就是不动点）
	normalized, _, _, err := core.NormalizeTexture("skin.steve", skin)
	if err != nil {
		t.Fatalf("规范化失败: %v", err)
	}
	if wantID := digestHex(normalized); item.ID != wantID {
		t.Fatalf("id = %s，想要 sha256(入库字节) = %s", item.ID, wantID)
	}
	if item.Readonly || item.Kind != "skin.steve" || item.Category != "skin" || item.Width != 64 ||
		item.Height != 64 || item.Size != len(normalized) || item.Favorite || len(item.Tags) != 2 {
		t.Fatalf("新条目字段不对: %+v", item)
	}
	if data, err := store.Texture(item.ID); err != nil || !bytes.Equal(data, normalized) {
		t.Fatalf("取回的字节不一致: %v", err)
	}

	// 夹带元数据的 PNG：入库字节里那些 chunk 要被剥掉，于是和同一张干净的图落到同一条资源上
	dirty := injectChunk(skin, "tEXt", []byte("Comment\x00夹带一点元数据"))
	dirty = injectChunk(dirty, "tIME", []byte{0x07, 0xEA, 1, 1, 0, 0, 0})
	if types := chunkTypes(dirty); len(types) != 5 || types[0] != "IHDR" {
		t.Fatalf("测试样本没造对: %v", types)
	}
	stripped, status, err := store.AddUpload("skin.alex", []string{"干净"}, dirty)
	if err != nil || status != "updated" {
		t.Fatalf("带元数据上传应当落到已有那条上: %q (%v)", status, err)
	}
	if stripped.ID != item.ID {
		t.Fatalf("剥掉元数据后应当和同一张图同一条资源: %s vs %s", stripped.ID, item.ID)
	}
	stored, err := store.Texture(stripped.ID)
	if err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if types := chunkTypes(stored); len(types) != 3 || types[0] != "IHDR" || types[1] != "IDAT" || types[2] != "IEND" {
		t.Fatalf("入库字节里不该有辅助 chunk: %v", types)
	}
	if bytes.Contains(stored, []byte("夹带一点元数据")) {
		t.Fatalf("夹带的元数据还在")
	}

	// 内容重复：标签 / kind 按本次提交的值覆盖
	updated, status, err := store.AddUpload("skin.alex", []string{"自建"}, skin)
	if err != nil || status != "updated" {
		t.Fatalf("覆盖失败: %q (%v)", status, err)
	}
	if updated.Kind != "skin.alex" || updated.Category != "skin" || len(updated.Tags) != 1 || updated.ID != item.ID {
		t.Fatalf("覆盖后的字段不对: %+v", updated)
	}
	// 没有变化就是 unchanged
	if _, status, err := store.AddUpload("skin.alex", []string{"自建"}, skin); err != nil || status != "unchanged" {
		t.Fatalf("重复上传 = %q (%v)", status, err)
	}

	// 不是 PNG / 尺寸不合法
	if _, _, err := store.AddUpload("skin.steve", nil, []byte("hello")); err == nil {
		t.Fatalf("非 PNG 应当报错")
	}
	// 只收默认尺寸：皮肤必须正好 64×64，披风必须正好 64×32
	for _, size := range [][2]int{{32, 32}, {64, 32}, {128, 128}, {512, 512}} {
		if _, _, err := store.AddUpload("skin.steve", nil, pngBytes(t, size[0], size[1])); err == nil {
			t.Fatalf("皮肤 %d×%d 不该通过", size[0], size[1])
		}
	}
	for _, size := range [][2]int{{64, 64}, {128, 64}, {32, 64}} {
		if _, _, err := store.AddUpload("cape", nil, pngBytes(t, size[0], size[1])); err == nil {
			t.Fatalf("披风 %d×%d 不该通过", size[0], size[1])
		}
	}
	// 只有 IHDR、没有图像数据（头合法但解不开）
	var headerOnly bytes.Buffer
	headerOnly.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13, 'I', 'H', 'D', 'R'})
	headerOnly.Write([]byte{0, 0, 0, 64, 0, 0, 0, 64})
	if _, _, err := store.AddUpload("skin.steve", nil, headerOnly.Bytes()); err == nil {
		t.Fatalf("解不开的 PNG 不该通过")
	}
	// 内置资源不能改（拿它的内容重传）
	builtin := builtinItem(t, store)
	builtinData, err := store.Texture(builtin.ID)
	if err != nil {
		t.Fatalf("读内置资源失败: %v", err)
	}
	if _, _, err := store.AddUpload("skin.steve", nil, builtinData); err == nil {
		t.Fatalf("内置资源不该能改")
	}

	// 披风的 kind 就是 cape、category 是 cape
	cape, status, err := store.AddUpload("cape", nil, pngBytes(t, 64, 32))
	if err != nil || status != "created" || cape.Kind != "cape" || cape.Category != "cape" {
		t.Fatalf("传披风失败: %+v (%q, %v)", cape, status, err)
	}
	stats, _ := store.Stats("")
	if stats.Skins != 10 || stats.Capes != 1 {
		t.Fatalf("统计不对: %+v", stats)
	}
}

func digestHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func chunkTypes(data []byte) []string {
	types := []string{}
	offset := 8
	for offset+8 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		kind := string(data[offset+4 : offset+8])
		types = append(types, kind)
		offset += 12 + length
		if kind == "IEND" {
			break
		}
	}
	return types
}

// injectChunk 在 IHDR 之后插一个 chunk（例如 tEXt），模拟「带元数据的上传文件」。
func injectChunk(data []byte, kind string, payload []byte) []byte {
	out := append([]byte{}, data[:8]...)
	offset := 8
	for offset+8 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		current := string(data[offset+4 : offset+8])
		out = append(out, data[offset:offset+12+length]...)
		offset += 12 + length
		if current == "IHDR" {
			head := make([]byte, 8)
			binary.BigEndian.PutUint32(head[0:4], uint32(len(payload)))
			copy(head[4:8], kind)
			sum := crc32.NewIEEE()
			sum.Write([]byte(kind))
			sum.Write(payload)
			tail := make([]byte, 4)
			binary.BigEndian.PutUint32(tail, sum.Sum32())
			out = append(out, head...)
			out = append(out, payload...)
			out = append(out, tail...)
		}
		if current == "IEND" {
			break
		}
	}
	return out
}

// 创建时指定 uuid：空串仍按离线规则推导；指定的要归一化并校验。
func TestCreateUserWithUUID(t *testing.T) {
	t.Parallel()
	store := openStore(t)

	explicit := "0123456789abcdef0123456789abcdef"
	user, err := store.CreateUserWithUUID("anchored@example.com", "hunter2x", "Anchored", "normal",
		"01234567-89AB-CDEF-0123-456789ABCDEF")
	if err != nil || user.UUID != explicit {
		t.Fatalf("指定 uuid 没生效: %+v (%v)", user, err)
	}
	if user.PlayerName != "Anchored" {
		t.Errorf("角色名 = %q，想要 Anchored", user.PlayerName)
	}

	derived, err := store.CreateUserWithUUID("derived@example.com", "hunter2x", "Derived2", "normal", "")
	if err != nil || derived.UUID != core.OfflineUUID("Derived2") {
		t.Fatalf("空 uuid 应当按角色名推导: %+v (%v)", derived, err)
	}

	for _, bad := range []string{"not-a-uuid", strings.Repeat("0", 31), strings.Repeat("g", 32)} {
		if _, err := store.CreateUserWithUUID("bad@example.com", "hunter2x", "BadUuid", "normal", bad); err == nil {
			t.Errorf("非法 uuid %q 不该通过", bad)
		}
	}

	_, err = store.CreateUserWithUUID("clash@example.com", "hunter2x", "ClashUuid", "normal", explicit)
	var conflict *core.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("uuid 撞车应当是冲突: %v", err)
	}
}

func TestListFavoriteSearchEquipDelete(t *testing.T) {
	t.Parallel()
	store := openStore(t)

	skin := pngBytes(t, 64, 64)
	item, _, err := store.AddUpload("skin.steve", []string{"自建", "蓝色外套"}, skin)
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	steve := store.UserByUsername("steve")
	alex := store.UserByUsername("alex")

	// 刚上传的排最前（upload_at 最新）
	items, err := store.List("skin", "all", "", steve.UUID)
	if err != nil || len(items) != 10 || items[0].ID != item.ID || items[0].Favorite {
		t.Fatalf("列表不对: %d (%v)", len(items), err)
	}
	// 标签交集查询
	if found, _ := store.List("skin", "all", "自建", steve.UUID); len(found) != 1 || found[0].ID != item.ID {
		t.Fatalf("标签查询不对: %v", found)
	}
	if found, _ := store.List("skin", "all", "自建 蓝色外套", steve.UUID); len(found) != 1 {
		t.Fatalf("多标签交集查询不对: %v", found)
	}
	if found, _ := store.List("skin", "all", "自建 不存在", steve.UUID); len(found) != 0 {
		t.Fatalf("交集应当为空: %v", found)
	}
	if found, _ := store.List("cape", "all", "", steve.UUID); len(found) != 0 {
		t.Fatalf("披风库应当是空的: %v", found)
	}

	// 收藏是「每个人自己的」
	toggled, err := store.ToggleFavorite(item.ID, steve.UUID)
	if err != nil || toggled == nil || !toggled.Favorite {
		t.Fatalf("收藏失败: %+v (%v)", toggled, err)
	}
	if favorites, _ := store.List("skin", "favorites", "", steve.UUID); len(favorites) != 1 {
		t.Fatalf("我的收藏应当是 1 条: %v", favorites)
	}
	if favorites, _ := store.List("skin", "favorites", "", alex.UUID); len(favorites) != 0 {
		t.Fatalf("别人的收藏不该混进来: %v", favorites)
	}
	if stats, _ := store.Stats(steve.UUID); stats.Favorites != 1 {
		t.Fatalf("收藏统计不对: %+v", stats)
	}
	if again, _ := store.ToggleFavorite(item.ID, steve.UUID); again.Favorite {
		t.Fatalf("再点一次应当取消收藏")
	}
	if missing, _ := store.ToggleFavorite("0000000000000000000000000000000000000000000000000000000000000000", steve.UUID); missing != nil {
		t.Fatalf("收藏不存在的贴图应当回 nil")
	}

	// 装备：按 category 写对应列
	if status, err := store.Equip(item.ID, steve.UUID); err != nil || status != "ok" {
		t.Fatalf("装备失败: %q (%v)", status, err)
	}
	if equipped := store.User(steve.UUID); equipped.Skin != item.ID || equipped.Cape != "" {
		t.Fatalf("装备状态没写对: %+v", equipped)
	}
	if status, _ := store.Equip("0000000000000000000000000000000000000000000000000000000000000000", steve.UUID); status != "missing" {
		t.Fatalf("装备不存在的贴图 = %q", status)
	}
	if got, _ := store.Item(item.ID, steve.UUID); got.Width != 64 || got.Height != 64 {
		t.Fatalf("单条查询的尺寸不对: %+v", got)
	}
	if none, _ := store.Item("0000000000000000000000000000000000000000000000000000000000000000", steve.UUID); none != nil {
		t.Fatalf("不存在的贴图应当回 nil")
	}

	// 删除前的「谁在穿这张」名单：steve 穿成皮肤；披风那条走另一列
	cape, _, err := store.AddUpload("cape", []string{"自建披风"}, pngBytes(t, 64, 32))
	if err != nil {
		t.Fatalf("传披风失败: %v", err)
	}
	if status, err := store.Equip(cape.ID, alex.UUID); err != nil || status != "ok" {
		t.Fatalf("给 alex 装备披风失败: %q (%v)", status, err)
	}
	wearers, err := store.AssetUsers(item.ID)
	if err != nil || len(wearers) != 1 {
		t.Fatalf("皮肤使用名单 = %+v (%v)", wearers, err)
	}
	if wearers[0].UUID != steve.UUID || wearers[0].PlayerName != "steve" || wearers[0].Slot != "skin" {
		t.Fatalf("皮肤使用名单内容不对: %+v", wearers[0])
	}
	if capeWearers, _ := store.AssetUsers(cape.ID); len(capeWearers) != 1 ||
		capeWearers[0].UUID != alex.UUID || capeWearers[0].Slot != "cape" {
		t.Fatalf("披风使用名单不对: %+v", capeWearers)
	}
	if none, _ := store.AssetUsers("0000000000000000000000000000000000000000000000000000000000000000"); len(none) != 0 {
		t.Fatalf("没人用的贴图应当回空名单: %+v", none)
	}

	// 只读的内置资源不能删
	if status, _ := store.Delete(builtinItem(t, store).ID); status != "readonly" {
		t.Fatalf("删内置资源 = %q", status)
	}
	// 删自己传的：收藏与装备状态一起清掉
	store.ToggleFavorite(item.ID, steve.UUID)
	if status, err := store.Delete(item.ID); err != nil || status != "deleted" {
		t.Fatalf("删贴图 = %q (%v)", status, err)
	}
	if equipped := store.User(steve.UUID); equipped.Skin != "" {
		t.Fatalf("删掉后装备状态没清: %+v", equipped)
	}
	if favorites, _ := store.List("skin", "favorites", "", steve.UUID); len(favorites) != 0 {
		t.Fatalf("删掉后收藏没清: %v", favorites)
	}
	if status, _ := store.Delete(item.ID); status != "missing" {
		t.Fatalf("重复删除 = %q", status)
	}
	// 删干净之后名单也要空（引用已经跟着清掉了）
	if wearers, _ := store.AssetUsers(item.ID); len(wearers) != 0 {
		t.Fatalf("删掉后名单没清: %+v", wearers)
	}
	if capeWearers, _ := store.AssetUsers(cape.ID); len(capeWearers) != 1 {
		t.Fatalf("删别的贴图不该动披风的使用者: %+v", capeWearers)
	}
}

// 搜索是子串（模糊）匹配，排序按命中档位：前缀 → 中缀 → 后缀，同档才轮到上传时间。
func TestListSearchRanksByMatchPosition(t *testing.T) {
	t.Parallel()
	store := openStore(t)

	upload := func(tag string, seed byte) db.Item {
		t.Helper()
		item, _, err := store.AddUpload("skin.steve", []string{tag}, pngWithPixel(t, seed))
		if err != nil {
			t.Fatalf("上传 %q 失败: %v", tag, err)
		}
		return item
	}
	// 上传顺序故意与「靠前」相反：最新传的反而是后缀命中
	prefix := upload("蓝色外套", 1)
	infix := upload("浅蓝披风", 2)
	suffix := upload("深蓝", 3)

	found, err := store.List("skin", "all", "蓝", "")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	// 精确交集的话「蓝」一条都搜不到，子串匹配才能命中这三条
	if len(found) != 3 {
		t.Fatalf("「蓝」应当命中 3 条: %v", found)
	}
	for index, want := range []db.Item{prefix, infix, suffix} {
		if found[index].ID != want.ID {
			t.Fatalf("第 %d 条 = %v，想要 %v", index+1, found[index].Tags, want.Tags)
		}
	}

	// 每一项查询词都要命中：这个条目是「前缀 + 后缀」，取最差档
	if hit, _ := store.List("skin", "all", "蓝 外套", ""); len(hit) != 1 || hit[0].ID != prefix.ID {
		t.Fatalf("多查询词的交集不对: %v", hit)
	}

	// 大小写不敏感
	mixed := upload("BlueCape", 4)
	if hit, _ := store.List("skin", "all", "blue", ""); len(hit) != 1 || hit[0].ID != mixed.ID {
		t.Fatalf("大小写不敏感的子串匹配不对: %v", hit)
	}
}

func TestTagCounts(t *testing.T) {
	t.Parallel()
	store := openStore(t)

	tags, err := store.TagCounts()
	if err != nil || len(tags) == 0 {
		t.Fatalf("标签统计失败: %v", err)
	}
	if tags[0].Tag != "官方" || tags[0].Count != 9 {
		t.Fatalf("内置资源都带「官方」标签，应当排第一: %+v", tags[0])
	}
	// 按次数倒序
	for index := 1; index < len(tags); index++ {
		if tags[index-1].Count < tags[index].Count {
			t.Fatalf("没有按使用次数倒序: %+v", tags)
		}
	}
}

func TestTextureLookupGuards(t *testing.T) {
	t.Parallel()
	store := openStore(t)

	// 长度不对直接不看库
	if data, err := store.Texture("太短"); err != nil || data != nil {
		t.Fatalf("短 id 应当回 nil: %v", err)
	}
	if data, err := store.Texture("0000000000000000000000000000000000000000000000000000000000000000"); err != nil || data != nil {
		t.Fatalf("不存在的 id 应当回 nil: %v", err)
	}
}

func TestReady(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "app.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	if err := store.Ready(); err != nil {
		t.Fatalf("就绪探针失败: %v", err)
	}
	// 探针表不该留在库里
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("直连数据库失败: %v", err)
	}
	defer handle.Close()
	var count int
	if err := handle.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = '__probe'").Scan(&count); err != nil {
		t.Fatalf("查表失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("探针表没清干净")
	}

	store.Close()
	if err := store.Ready(); err == nil {
		t.Fatalf("库关了之后就绪探针应当失败")
	}
}

// 原型的规矩：表结构和 DESIGN.md 对不上就直接重建，不做迁移。
func TestSchemaMismatchRebuilds(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "app.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	seedUsers(t, store)
	store.Close()

	// 把 user 表改成旧版本的样子
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("直连数据库失败: %v", err)
	}
	if _, err := handle.Exec("DROP TABLE user"); err != nil {
		t.Fatalf("改表失败: %v", err)
	}
	if _, err := handle.Exec("CREATE TABLE user (id INTEGER PRIMARY KEY, username TEXT)"); err != nil {
		t.Fatalf("改表失败: %v", err)
	}
	handle.Close()

	reopened, err := db.Open(path)
	if err != nil {
		t.Fatalf("重开数据库失败: %v", err)
	}
	defer reopened.Close()
	columns := map[string]bool{}
	handle, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("直连数据库失败: %v", err)
	}
	defer handle.Close()
	rows, err := handle.Query("PRAGMA table_info(user)")
	if err != nil {
		t.Fatalf("查表结构失败: %v", err)
	}
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             any
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("读表结构失败: %v", err)
		}
		columns[name] = true
	}
	rows.Close()
	for _, wanted := range []string{"uuid", "role", "player_name", "username", "password_hash", "skin", "cape"} {
		if !columns[wanted] {
			t.Fatalf("重建后的 user 表缺少 %s: %v", wanted, columns)
		}
	}
	// 重建之后账号是空的（数据不保留），需要重新灌
	if users, _ := reopened.Users(); len(users) != 0 {
		t.Fatalf("重建后不该保留旧数据: %d", len(users))
	}
	seedUsers(t, reopened)
	if len(mustUsers(t, reopened)) != 6 {
		t.Fatalf("重建后应当能重新灌进 6 个账号")
	}
}

func mustUsers(t *testing.T, store *db.Store) []db.User {
	t.Helper()
	users, err := store.Users()
	if err != nil {
		t.Fatalf("列账号失败: %v", err)
	}
	return users
}

func builtinItem(t *testing.T, store *db.Store) db.Item {
	t.Helper()
	items, err := store.List("skin", "all", "", "")
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	for _, item := range items {
		if item.Readonly {
			return item
		}
	}
	t.Fatalf("库里没有只读的内置资源")
	return db.Item{}
}

// DESIGN.md 1.5：建库时就把目录与库文件收到 0700 / 0600。
func TestOpenHardensPermissions(t *testing.T) {
	t.Parallel()

	// 目录由 db.Open 自己建（DATA_DIR 的样子）；t.TempDir() 本身是调用方给的，权限不归它管
	dir := filepath.Join(t.TempDir(), "data")
	store, err := db.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer store.Close()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat 目录失败: %v", err)
	}
	if got := info.Mode().Perm(); got != core.DataDirPerm {
		t.Fatalf("目录权限 = %o，想要 %o", got, core.DataDirPerm)
	}
	for _, name := range []string{"app.db", "app.db-wal", "app.db-shm"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s 失败: %v", name, err)
		}
		if got := info.Mode().Perm(); got != core.SecretPerm {
			t.Fatalf("%s 权限 = %o，想要 %o", name, got, core.SecretPerm)
		}
	}
}
