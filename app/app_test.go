package app

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"scoped-skin/builtin"
	"scoped-skin/core"
)

// openInstance 起一个真实实例（内置资源照灌，不播种任何测试账号）。
func openInstance(t *testing.T, mutate func(*Config)) *App {
	t.Helper()

	cfg := Config{
		DataDir:   filepath.Join(t.TempDir(), "data"),
		Version:   "test",
		BuiltinFS: builtin.Files,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	instance, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(instance.Close)
	return instance
}

// DESIGN.md 1.2 / 1.1：首次启动只建一个管理员账号，密码取自 ADMIN_PASSWORD。
func TestFirstStartCreatesSingleAdmin(t *testing.T) {
	t.Parallel()

	instance := openInstance(t, func(cfg *Config) {
		cfg.AdminUsername = "root"
		cfg.AdminPassword = "s3cret-password"
	})

	users, err := instance.Store.Users()
	if err != nil || len(users) != 1 {
		t.Fatalf("首次启动应当只有 1 个账号，实际 %d (%v)", len(users), err)
	}
	admin := users[0]
	if admin.Username != "root" || admin.Role != "admin" || admin.PlayerName != "root" {
		t.Fatalf("管理员账号不对: %+v", admin)
	}
	if instance.Store.Authenticate("root", "s3cret-password") == nil {
		t.Fatalf("ADMIN_PASSWORD 没生效")
	}
	if instance.Store.Authenticate("root", "admin") != nil {
		t.Fatalf("不该存在默认密码")
	}
}

// 密码留空就随机生成一个，并把明文打进日志（只此一次）。
// 不用 t.Parallel：slog.SetDefault 是全局的，捕获日志的用例必须独占跑，
// 否则同包里其它并行用例的启动日志会串进这个 buffer。
func TestFirstStartGeneratesPasswordWhenUnset(t *testing.T) {
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	instance := openInstance(t, nil)
	users, err := instance.Store.Users()
	if err != nil || len(users) != 1 || users[0].Username != DefaultAdminUsername {
		t.Fatalf("默认账号名应当是 %s: %v (%v)", DefaultAdminUsername, users, err)
	}

	var password string
	for _, line := range strings.Split(strings.TrimSpace(buffer.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		message, _ := record["msg"].(string)
		// 只认「随机生成」那条：设了 ADMIN_PASSWORD 的用例也会打 username=admin，但没有密码字段
		if strings.Contains(message, "已生成管理员账号") {
			password, _ = record["password"].(string)
		}
	}
	if len(password) != 32 {
		t.Fatalf("日志里的密码应当是一串 32 位 hex，实际 %q", password)
	}
	if instance.Store.Authenticate(DefaultAdminUsername, password) == nil {
		t.Fatalf("日志里打出来的密码应当能登录")
	}
}

// 已有账号时不再动账号表：重开一次不该多出账号，也不该改掉密码。
func TestReopenKeepsExistingAccounts(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "data")
	open := func() *App {
		instance, err := Open(Config{
			DataDir:       dir,
			Version:       "test",
			BuiltinFS:     builtin.Files,
			AdminUsername: "root",
			AdminPassword: "first-password",
		})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(instance.Close)
		return instance
	}

	first := open()
	// 管理员改过密码 / 改过账号名之后重启，不该被重新种回来
	if _, err := first.Store.CreateUser("steve", "steve123", "steve", "normal"); err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	before := first.Store.UserByUsername("root")
	first.Close()

	second := open()
	users, err := second.Store.Users()
	if err != nil || len(users) != 2 {
		t.Fatalf("重启后账号数 = %d (%v)，想要 2", len(users), err)
	}
	after := second.Store.UserByUsername("root")
	if after == nil || after.PasswordHash != before.PasswordHash {
		t.Fatalf("重启不该改掉已有账号的密码哈希")
	}
}

// DESIGN.md 5.3：Argon2id 的并发上限是固定的 4。
func TestArgonGateSize(t *testing.T) {
	t.Parallel()

	instance := openInstance(t, nil)
	if ArgonConcurrency != 4 {
		t.Fatalf("ArgonConcurrency = %d，DESIGN.md 要求 4", ArgonConcurrency)
	}
	if got := cap(instance.argon); got != 4 {
		t.Fatalf("信号量容量 = %d，想要 4", got)
	}
}

// 并发登录不该在闸门上卡死：把并发开到 8 也都能正常返回。
func TestVerifyCredentialsUnderConcurrency(t *testing.T) {
	t.Parallel()

	instance := openInstance(t, func(cfg *Config) {
		cfg.AdminPassword = "password-1234"
	})

	var group sync.WaitGroup
	results := make([]bool, 8)
	for index := range results {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index] = instance.VerifyCredentials("admin", "password-1234") != nil
		}()
	}
	group.Wait()
	for index, ok := range results {
		if !ok {
			t.Fatalf("第 %d 个并发校验没通过", index+1)
		}
	}
}

// DESIGN.md 3.2 / 1.5：启动时把权限收紧，readyz 持续断言；谁把权限放开就立刻不认。
func TestReadyzGuaranteesDataDirPermissions(t *testing.T) {
	t.Parallel()

	instance := openInstance(t, nil)
	dir := instance.Config.DataDir

	permOf := func(name string) fs.FileMode {
		t.Helper()
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		return info.Mode().Perm()
	}

	if got := permOf("."); got != core.DataDirPerm {
		t.Fatalf("数据目录权限 = %o，想要 %o", got, core.DataDirPerm)
	}
	if got := permOf("app.db"); got != core.SecretPerm {
		t.Fatalf("app.db 权限 = %o，想要 %o", got, core.SecretPerm)
	}
	if got := permOf("private.pem"); got != core.SecretPerm {
		t.Fatalf("private.pem 权限 = %o，想要 %o", got, core.SecretPerm)
	}
	if got := permOf("public.pem"); got != core.PublicPerm {
		t.Fatalf("public.pem 权限 = %o，想要 %o", got, core.PublicPerm)
	}
	if err := instance.Ready(); err != nil {
		t.Fatalf("就绪探针不该报错: %v", err)
	}

	// 库文件被放开 → 不认
	if err := os.Chmod(filepath.Join(dir, "app.db"), 0o644); err != nil {
		t.Fatalf("改权限失败: %v", err)
	}
	if err := instance.Ready(); err == nil {
		t.Fatalf("app.db 变成 0644 之后 readyz 还报就绪")
	}
	// 私钥被放开 → 不认
	if err := os.Chmod(filepath.Join(dir, "app.db"), core.SecretPerm); err != nil {
		t.Fatalf("改权限失败: %v", err)
	}
	if err := os.Chmod(filepath.Join(dir, "private.pem"), 0o644); err != nil {
		t.Fatalf("改权限失败: %v", err)
	}
	if err := instance.Ready(); err == nil {
		t.Fatalf("private.pem 变成 0644 之后 readyz 还报就绪")
	}
	// 目录被放开 → 不认
	if err := os.Chmod(filepath.Join(dir, "private.pem"), core.SecretPerm); err != nil {
		t.Fatalf("改权限失败: %v", err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("改权限失败: %v", err)
	}
	if err := instance.Ready(); err == nil {
		t.Fatalf("数据目录变成 0755 之后 readyz 还报就绪")
	}
}
