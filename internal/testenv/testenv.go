// Package testenv 给测试用：一套独立的应用实例（全新临时 DATA_DIR + 内置资源 +
// mock 账号），以及几个发请求的小工具。RSA 密钥只生成一次，之后各用例复制复用 ——
// 4096 位密钥生成一次要 ~1 秒，每个用例都生成太慢。
package testenv

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"scoped-skin/app"
	"scoped-skin/builtin"
	"scoped-skin/core"
)

var (
	keysOnce sync.Once
	keysDir  string
	keysErr  error
)

func sharedKeys() (string, error) {
	keysOnce.Do(func() {
		keysDir, keysErr = os.MkdirTemp("", "sms-test-keys-")
		if keysErr != nil {
			return
		}
		_, keysErr = core.LoadSigningKeys(keysDir)
	})
	return keysDir, keysErr
}

// Accounts 是测试用的固定账号：生产代码不再内置任何账号，测试自己播种一套。
var Accounts = []struct {
	Username   string
	Password   string
	PlayerName string
	Role       string
}{
	{Username: "admin", Password: "admin123", PlayerName: "admin", Role: "admin"},
	{Username: "steve", Password: "steve123", PlayerName: "steve", Role: "normal"},
	{Username: "alex", Password: "alex12345", PlayerName: "alex", Role: "normal"},
	{Username: "notch", Password: "notch123", PlayerName: "notch", Role: "normal"},
	{Username: "dinnerbone", Password: "dinnerbone123", PlayerName: "dinnerbone", Role: "normal"},
	{Username: "guest", Password: "guest123", PlayerName: "guest", Role: "guest"},
}

// New 起一套独立的应用实例，用完自动清理。
func New(t *testing.T) *app.App {
	t.Helper()
	return NewWith(t, nil)
}

// NewWith 同上，但允许先改一下 Config（例如设置 TrustedProxies）。
func NewWith(t *testing.T, mutate func(*app.Config)) *app.App {
	t.Helper()

	dataDir := t.TempDir()
	source, err := sharedKeys()
	if err != nil {
		t.Fatalf("生成测试密钥失败: %v", err)
	}
	for _, name := range []string{"private.pem", "public.pem"} {
		body, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatalf("读取测试密钥失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dataDir, name), body, 0o600); err != nil {
			t.Fatalf("写入测试密钥失败: %v", err)
		}
	}

	cfg := app.Config{
		DataDir:       dataDir,
		Version:       "test",
		BuiltinFS:     builtin.Files,
		AdminUsername: Accounts[0].Username,
		AdminPassword: Accounts[0].Password,
	}
	if mutate != nil {
		mutate(&cfg)
	}

	instance, err := app.Open(cfg)
	if err != nil {
		t.Fatalf("打开测试实例失败: %v", err)
	}
	t.Cleanup(instance.Close)
	Seed(t, instance)
	return instance
}

// Seed 往实例里播种测试账号（app.Open 自己只建一个管理员）。
func Seed(t *testing.T, instance *app.App) {
	t.Helper()
	for _, account := range Accounts {
		if instance.Store.UserByUsername(account.Username) != nil {
			continue
		}
		if _, err := instance.Store.CreateUser(account.Username, account.Password, account.PlayerName, account.Role); err != nil {
			t.Fatalf("播种测试账号 %s 失败: %v", account.Username, err)
		}
	}
	if err := instance.RefreshTokens(); err != nil {
		t.Fatalf("重建 token 表失败: %v", err)
	}
}

// Token 直接按固定算法算出某个账号的站点 token（不走登录接口，免得撞限流）。
func Token(t *testing.T, instance *app.App, username string) string {
	t.Helper()
	user := instance.Store.UserByUsername(username)
	if user == nil {
		t.Fatalf("测试账号不存在: %s", username)
	}
	return instance.Tokens.TokenFor(core.Credential{
		UUID: user.UUID, Username: user.Username, PasswordHash: user.PasswordHash,
	})
}

// Do 发一个请求。body 为 nil 时不带请求体，否则序列化成 JSON；token 非空时带 Bearer 头。
func Do(handler http.Handler, method, target string, body any, token string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, target, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// Decode 把响应体解析成 map，方便断言。
func Decode(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	payload := map[string]any{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("响应不是合法的 JSON: %v (%s)", err, recorder.Body.String())
	}
	return payload
}

// DecodeInto 把响应体解析到给定结构里。
func DecodeInto(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("响应不是合法的 JSON: %v (%s)", err, recorder.Body.String())
	}
}
