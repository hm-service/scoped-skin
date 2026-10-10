package core_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"scoped-skin/core"
)

// 回归（私钥存在但读不动）：LoadSigningKeys 曾把任何 os.ReadFile 错误都当成
// 「密钥不存在」，于是静默生成一对新密钥并覆盖 private.pem —— 已签发的 textures
// 签名全部失效、公钥也跟着变。正确的是：读不动 ≠ 不存在，直接报错。
//
// 主用例用 0o200（只写不读）：读权限没了但写权限还在，修复前会**真的**把私钥覆盖掉。
// 补一个 0o000（完全不可读写）：那条路径修复前会因写入也失败而侥幸报错，所以它只能
// 证明「不能覆盖」，区分新旧代码的是 0o200 那条。
func TestLoadSigningKeysFailsOnUnreadablePrivateKey(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 无视文件权限位，构造不出「读不动」")
	}

	for _, mode := range []os.FileMode{0o200, 0o000} {
		t.Run(mode.String(), func(t *testing.T) {
			dataDir := t.TempDir()
			privatePath := filepath.Join(dataDir, "private.pem")

			// 先正常生成一对，拿到磁盘上私钥的原始字节与公钥
			original, err := core.LoadSigningKeys(dataDir)
			if err != nil {
				t.Fatalf("首次加载失败: %v", err)
			}
			before, err := os.ReadFile(privatePath)
			if err != nil {
				t.Fatalf("读取私钥失败: %v", err)
			}

			if err := os.Chmod(privatePath, mode); err != nil {
				t.Fatalf("改权限失败: %v", err)
			}
			t.Cleanup(func() { os.Chmod(privatePath, core.SecretPerm) })

			keys, err := core.LoadSigningKeys(dataDir)
			if err == nil {
				t.Fatalf("私钥读不动时必须报错，不能静默换一对新密钥（实际拿到 %v）", keys != nil)
			}
			if keys != nil {
				t.Fatalf("报错时返回的 keys 必须是 nil，实际 %+v", keys)
			}

			// 文件字节不能被覆盖
			if err := os.Chmod(privatePath, core.SecretPerm); err != nil {
				t.Fatalf("恢复权限失败: %v", err)
			}
			after, err := os.ReadFile(privatePath)
			if err != nil {
				t.Fatalf("重读私钥失败: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("读不动的私钥被覆盖了：%d 字节 → %d 字节", len(before), len(after))
			}

			// 恢复权限后应当还是原来那把（没有重新生成）
			reloaded, err := core.LoadSigningKeys(dataDir)
			if err != nil {
				t.Fatalf("恢复权限后加载失败: %v", err)
			}
			if reloaded.PublicPEM() != original.PublicPEM() {
				t.Fatalf("重新加载后公钥变了，说明期间重新生成过密钥")
			}
		})
	}
}

// 缺 private.pem 时才生成；生成后再次加载必须拿到同一把公钥（不重新生成）。
// 另外 public.pem 不作数：磁盘上那份公钥是坏的 / 是别的密钥也不影响启动，
// 因为公钥总是从私钥现推、并顺手修回去。
func TestLoadSigningKeysGeneratesOnlyWhenMissing(t *testing.T) {
	dataDir := t.TempDir()
	privatePath := filepath.Join(dataDir, "private.pem")
	publicPath := filepath.Join(dataDir, "public.pem")

	if _, err := os.Stat(privatePath); !os.IsNotExist(err) {
		t.Fatalf("前置条件：private.pem 本不该存在")
	}
	first, err := core.LoadSigningKeys(dataDir)
	if err != nil {
		t.Fatalf("首次加载失败: %v", err)
	}
	if _, err := os.Stat(privatePath); err != nil {
		t.Fatalf("缺文件时应当生成 private.pem: %v", err)
	}
	if _, err := os.Stat(publicPath); err != nil {
		t.Fatalf("缺文件时应当落盘 public.pem: %v", err)
	}

	reloaded, err := core.LoadSigningKeys(dataDir)
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if reloaded.PublicPEM() != first.PublicPEM() {
		t.Fatalf("重新加载拿到了另一把公钥，说明重新生成了密钥")
	}

	// 把 public.pem 换成别的密钥的公钥（磁盘上那份只是给运维看的，不作数）
	otherDir := t.TempDir()
	other, err := core.LoadSigningKeys(otherDir)
	if err != nil {
		t.Fatalf("生成第二对密钥失败: %v", err)
	}
	if err := os.WriteFile(publicPath, []byte(other.PublicPEM()), core.PublicPerm); err != nil {
		t.Fatalf("覆盖 public.pem 失败: %v", err)
	}
	after, err := core.LoadSigningKeys(dataDir)
	if err != nil {
		t.Fatalf("public.pem 被换掉后仍应能从私钥启动: %v", err)
	}
	if after.PublicPEM() != first.PublicPEM() {
		t.Fatalf("公钥应当从私钥现推，不该采信磁盘上的 public.pem")
	}
	onDisk, err := os.ReadFile(publicPath)
	if err != nil {
		t.Fatalf("重读 public.pem 失败: %v", err)
	}
	if string(onDisk) != first.PublicPEM() {
		t.Fatalf("被改坏的 public.pem 应当被修回去")
	}
}

// private.pem 是个目录（不是缺失）也必须报错，不能当成「不存在」去生成覆盖。
func TestLoadSigningKeysRejectsDirectoryAsPrivateKey(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dataDir, "private.pem"), 0o700); err != nil {
		t.Fatalf("造目录失败: %v", err)
	}
	if _, err := core.LoadSigningKeys(dataDir); err == nil {
		t.Fatalf("private.pem 是目录时应当报错")
	}
}
