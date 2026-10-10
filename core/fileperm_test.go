package core_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"scoped-skin/core"
)

// DESIGN.md 1.5：权限收紧只改权限，不碰内容；路径不存在也不算错。
func TestTightenPerm(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "secret.pem")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	if err := core.TightenPerm(path, core.SecretPerm); err != nil {
		t.Fatalf("收紧权限失败: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat 失败: %v", err)
	}
	if got := info.Mode().Perm(); got != core.SecretPerm {
		t.Fatalf("权限 = %o，想要 %o", got, core.SecretPerm)
	}

	if err := core.TightenPerm(filepath.Join(t.TempDir(), "不存在"), fs.FileMode(0o600)); err != nil {
		t.Fatalf("路径不存在不该报错: %v", err)
	}
}
