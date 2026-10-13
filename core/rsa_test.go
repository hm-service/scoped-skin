package core_test

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scoped-skin/core"
)

// 用给定 PEM 公钥按规范（SHA1withRSA）验签。
func verifyWithPEM(t *testing.T, publicPEM, value, signature string) {
	t.Helper()
	block, _ := pem.Decode([]byte(publicPEM))
	if block == nil {
		t.Fatalf("公钥不是合法 PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("公钥解析失败: %v", err)
	}
	public, ok := parsed.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("不是 RSA 公钥")
	}
	raw, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		t.Fatalf("签名不是合法 base64: %v", err)
	}
	digest := sha1.Sum([]byte(value))
	if err := rsa.VerifyPKCS1v15(public, crypto.SHA1, digest[:], raw); err != nil {
		t.Fatalf("验签失败: %v", err)
	}
}

func TestSigningKeysGenerateAndReload(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	keys, err := core.LoadSigningKeys(dataDir)
	if err != nil {
		t.Fatalf("加载密钥失败: %v", err)
	}
	for _, name := range []string{"private.pem", "public.pem"} {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
				t.Fatalf("没有落盘 %s: %v", name, err)
			}
		})
	}
	publicPEM := keys.PublicPEM()
	if !strings.HasPrefix(publicPEM, "-----BEGIN PUBLIC KEY-----") {
		t.Fatalf("公钥应当是 PKIX PEM: %q", publicPEM)
	}
	block, _ := pem.Decode([]byte(publicPEM))
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("公钥解析失败: %v", err)
	}
	if bits := parsed.(*rsa.PublicKey).N.BitLen(); bits != core.RSAKeyBits {
		t.Fatalf("密钥长度 = %d，规范建议 %d 位", bits, core.RSAKeyBits)
	}

	// 签名 → 用公钥验签；内容变一个字节就不该通过
	value := base64.StdEncoding.EncodeToString([]byte(`{"profileName":"steve"}`))
	signature, err := keys.Sign(value)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}
	verifyWithPEM(t, publicPEM, value, signature)

	// 内容改一个字节就不该验得过
	other := value + "x"
	block, _ = pem.Decode([]byte(publicPEM))
	public := parsed.(*rsa.PublicKey)
	raw, _ := base64.StdEncoding.DecodeString(signature)
	digest := sha1.Sum([]byte(other))
	if err := rsa.VerifyPKCS1v15(public, crypto.SHA1, digest[:], raw); err == nil {
		t.Fatalf("改过内容还验签通过")
	}

	// 重新加载同一目录：公钥不变，旧签名照样能验
	reloaded, err := core.LoadSigningKeys(dataDir)
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if reloaded.PublicPEM() != publicPEM {
		t.Fatalf("重新加载后公钥变了")
	}
	verifyWithPEM(t, reloaded.PublicPEM(), value, signature)
}

func TestSigningKeysRejectBrokenPEM(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "private.pem"), []byte("不是 PEM"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := core.LoadSigningKeys(dataDir); err == nil {
		t.Fatalf("坏掉的私钥应当报错")
	}
}
