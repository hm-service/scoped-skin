package core_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"scoped-skin/core"
)

// TightenPerm 只把「路径不存在」当成可以忽略；其它错误（例如路径中间隔着普通文件，
// 返回 ENOTDIR）必须原样报出来，不能和「不存在」混为一谈。
func TestTightenPermDistinguishesBrokenPathFromMissing(t *testing.T) {
	// 不存在的路径：忽略
	if err := core.TightenPerm(filepath.Join(t.TempDir(), "不存在"), core.SecretPerm); err != nil {
		t.Errorf("路径不存在不该报错: %v", err)
	}

	// 把普通文件当目录用：错误不是 ErrNotExist，应当报出来
	file := filepath.Join(t.TempDir(), "regular")
	if err := os.WriteFile(file, []byte("x"), core.SecretPerm); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	err := core.TightenPerm(filepath.Join(file, "child"), core.SecretPerm)
	if err == nil {
		t.Fatalf("路径中间隔着普通文件时应当报错")
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ENOTDIR 不该被当成 ErrNotExist: %v", err)
	}
}
