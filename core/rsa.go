package core

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// RSAKeyBits 取规范建议的 4096 位（参考实现同样是 4096）。
const RSAKeyBits = 4096

// SigningKeys 是给 Yggdrasil 的 textures 属性签名的密钥对，明文存在 DATA_DIR 下：
// private.pem（PKCS#8）、public.pem（PKIX）。公钥同时是根元数据里的 signaturePublickey。
type SigningKeys struct {
	private *rsa.PrivateKey
	public  string
}

func LoadSigningKeys(dataDir string) (*SigningKeys, error) {
	if err := os.MkdirAll(dataDir, DataDirPerm); err != nil {
		return nil, err
	}
	privatePath := filepath.Join(dataDir, "private.pem")
	publicPath := filepath.Join(dataDir, "public.pem")

	keys := &SigningKeys{}
	switch raw, err := os.ReadFile(privatePath); {
	case err == nil:
		block, _ := pem.Decode(raw)
		if block == nil {
			return nil, fmt.Errorf("private key %s is not valid PEM", privatePath)
		}
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse private key %s: %w", privatePath, err)
		}
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key %s is not an RSA key", privatePath)
		}
		keys.private = key
	case errors.Is(err, fs.ErrNotExist):
		key, err := rsa.GenerateKey(rand.Reader, RSAKeyBits)
		if err != nil {
			return nil, err
		}
		derived, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, err
		}
		if err := writePEM(privatePath, "PRIVATE KEY", derived); err != nil {
			return nil, err
		}
		keys.private = key
	default:
		// 读不动不等于没有：私钥存在却读不了时若静默换一对新密钥，已签发的 textures
		// 签名会全部失效，公钥也会跟着变，所以这里直接失败。
		return nil, fmt.Errorf("read private key %s: %w", privatePath, err)
	}

	// 公钥总是从私钥现推：磁盘上的那份只是给运维看的，不作数、也不会和私钥走岔。
	publicDER, err := x509.MarshalPKIXPublicKey(&keys.private.PublicKey)
	if err != nil {
		return nil, err
	}
	publicBlock := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	keys.public = string(publicBlock)
	if err := os.WriteFile(publicPath, publicBlock, PublicPerm); err != nil {
		return nil, err
	}
	// 读进来的旧文件也顺手收紧：私钥只有属主能读
	if err := TightenPerm(privatePath, SecretPerm); err != nil {
		return nil, err
	}
	if err := TightenPerm(publicPath, PublicPerm); err != nil {
		return nil, err
	}
	return keys, nil
}

// PublicPEM 返回 PKIX 格式的公钥 PEM（根元数据的 signaturePublickey 字段）。
func (k *SigningKeys) PublicPEM() string { return k.public }

// 规范原文：签名算法为 SHA1withRSA；签的对象就是 textures 属性的值（那串 base64）。
func (k *SigningKeys) Sign(value string) (string, error) {
	digest := sha1.Sum([]byte(value))
	signature, err := rsa.SignPKCS1v15(rand.Reader, k.private, crypto.SHA1, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func writePEM(path, kind string, der []byte) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), SecretPerm)
}
