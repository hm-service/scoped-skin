package core

import (
	"errors"
	"io/fs"
	"os"
)

// DATA_DIR 与其中文件的权限约定。
const (
	DataDirPerm = 0o700 // 数据目录：只有属主能进
	SecretPerm  = 0o600 // 私钥、数据库（含 -wal / -shm）
	PublicPerm  = 0o644 // 公钥：它本身就是公开信息
)

// TightenPerm 把文件或目录的权限改成 mode；路径不存在就跳过（返回值只反映真的改不动）。
func TightenPerm(path string, mode fs.FileMode) error {
	if err := os.Chmod(path, mode); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}
