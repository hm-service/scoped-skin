package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"scoped-skin/core"
)

// expectedPerm 是 DATA_DIR 里各文件应有的权限。
// `-wal` / `-shm` 是 SQLite 自己建的临时文件，开关库时可能不在，所以只收紧、不参与断言。
var expectedPerm = map[string]fs.FileMode{
	"private.pem": core.SecretPerm,
	"public.pem":  core.PublicPerm,
	"app.db":      core.SecretPerm,
}

// hardenDataDir 把数据目录与库文件收紧到约定权限；密钥文件由 core 自己收紧。
func hardenDataDir(dir string) error {
	if dir == "" {
		return nil
	}
	// "." 和相对路径指向的是真实目录，先归成绝对路径，免得被当成无需处理而漏收紧。
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	if err := os.Chmod(abs, core.DataDirPerm); err != nil {
		return fmt.Errorf("tighten data dir permissions: %w", err)
	}
	for name, mode := range expectedPerm {
		if err := core.TightenPerm(filepath.Join(abs, name), mode); err != nil {
			return fmt.Errorf("tighten permissions on %s: %w", name, err)
		}
	}
	for _, name := range []string{"app.db-wal", "app.db-shm"} {
		if err := core.TightenPerm(filepath.Join(abs, name), core.SecretPerm); err != nil {
			return fmt.Errorf("tighten permissions on %s: %w", name, err)
		}
	}
	return nil
}

// checkDataDir 断言权限已经是约定值，给 /readyz 用。
func checkDataDir(dir string) error {
	if dir == "" {
		return nil
	}
	// 断言的是真实目录：DATA_DIR 写成 "." 或相对路径时同样要查。
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("data dir unusable: %w", err)
	}
	if perm := info.Mode().Perm(); perm != core.DataDirPerm {
		return fmt.Errorf("data dir mode is %o, want %o", perm, core.DataDirPerm)
	}
	for name, want := range expectedPerm {
		info, err := os.Stat(filepath.Join(abs, name))
		if err != nil {
			return fmt.Errorf("%s unusable: %w", name, err)
		}
		if perm := info.Mode().Perm(); perm != want {
			return fmt.Errorf("%s mode is %o, want %o", name, perm, want)
		}
	}
	return nil
}
