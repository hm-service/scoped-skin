package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"scoped-skin/core"
)

// expectedPerm 是 DATA_DIR 里各文件应有的权限（DESIGN.md 1.5）。
// `-wal` / `-shm` 是 SQLite 自己建的临时文件，开关库时可能不在，所以只收紧、不参与断言。
var expectedPerm = map[string]fs.FileMode{
	"private.pem": core.SecretPerm,
	"public.pem":  core.PublicPerm,
	"app.db":      core.SecretPerm,
}

// hardenDataDir 把数据目录与库文件收紧到约定权限；密钥文件由 core 自己收紧。
func hardenDataDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.Chmod(dir, core.DataDirPerm); err != nil {
		return fmt.Errorf("收紧数据目录权限失败: %w", err)
	}
	for name, mode := range expectedPerm {
		if err := core.TightenPerm(filepath.Join(dir, name), mode); err != nil {
			return fmt.Errorf("收紧 %s 权限失败: %w", name, err)
		}
	}
	for _, name := range []string{"app.db-wal", "app.db-shm"} {
		if err := core.TightenPerm(filepath.Join(dir, name), core.SecretPerm); err != nil {
			return fmt.Errorf("收紧 %s 权限失败: %w", name, err)
		}
	}
	return nil
}

// checkDataDir 断言权限已经是约定值，给 /readyz 用（DESIGN.md 3.2）。
func checkDataDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("数据目录不可用: %w", err)
	}
	if perm := info.Mode().Perm(); perm != core.DataDirPerm {
		return fmt.Errorf("数据目录权限是 %o，应当是 %o", perm, core.DataDirPerm)
	}
	for name, want := range expectedPerm {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("%s 不可用: %w", name, err)
		}
		if perm := info.Mode().Perm(); perm != want {
			return fmt.Errorf("%s 权限是 %o，应当是 %o", name, perm, want)
		}
	}
	return nil
}
