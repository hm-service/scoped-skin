// 端到端：把站点 API 与 Yggdrasil 接口放在同一个 mux 上跑，验证两套 token 之间的联动。
package api_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"scoped-skin/api"
	"scoped-skin/api/yggdrasil"
	"scoped-skin/core"
	"scoped-skin/internal/testenv"
)

// DESIGN.md 3.5：管理员动过某个账号（删号 / 改 UUID / 改凭据）时，会按 uuid 把这个账号
// 的 Yggdrasil 会话一并清掉。这里把「站点侧改凭据 → 启动器侧令牌失效」串起来验一遍。
func TestAdminCredentialChangeKillsYggdrasilSession(t *testing.T) {
	instance := testenv.New(t)
	handler := api.NewHandler(instance)
	playerUUID := core.OfflineUUID("steve")

	// 1. 启动器侧先登录一次，拿一个 accessToken
	recorder := testenv.Do(handler, http.MethodPost, "/api/yggdrasil/authserver/authenticate",
		map[string]any{"username": "steve", "password": "steve123"}, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("认证失败: %d %s", recorder.Code, recorder.Body.String())
	}
	accessToken := testenv.Decode(t, recorder)["accessToken"].(string)

	// 2. 浏览器侧也拿一个站点 token
	siteToken := testenv.Token(t, instance, "steve")
	if recorder := testenv.Do(handler, http.MethodGet, "/api/auth/session", nil, siteToken); recorder.Code != http.StatusOK {
		t.Fatalf("站点 token 一开始就该有效: %d", recorder.Code)
	}

	// 3. 管理员改掉 steve 的密码
	adminToken := testenv.Token(t, instance, "admin")
	recorder = testenv.Do(handler, http.MethodPatch, "/api/admin/users/"+playerUUID,
		map[string]any{"password": "changed_12345"}, adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("改密码失败: %d %s", recorder.Code, recorder.Body.String())
	}

	// 4. 站点旧 token 立刻失效（确定性哈希按库里的字段整表重算）
	if recorder := testenv.Do(handler, http.MethodGet, "/api/auth/session", nil, siteToken); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("改凭据后站点旧 token 应当 401，实际 %d", recorder.Code)
	}
	// 5. Yggdrasil 的内存会话也被清掉
	recorder = testenv.Do(handler, http.MethodPost, "/api/yggdrasil/authserver/validate",
		map[string]any{"accessToken": accessToken}, "")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("改凭据后 Yggdrasil 令牌应当 403，实际 %d", recorder.Code)
	}
	// 6. 新密码能用新令牌换出来
	recorder = testenv.Do(handler, http.MethodPost, "/api/yggdrasil/authserver/authenticate",
		map[string]any{"username": "steve", "password": "changed_12345"}, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("新密码应当能登录: %d %s", recorder.Code, recorder.Body.String())
	}
}

// 删号也要连带清掉 Yggdrasil 会话。
func TestAdminDeleteUserKillsYggdrasilSession(t *testing.T) {
	instance := testenv.New(t)
	handler := api.NewHandler(instance)

	recorder := testenv.Do(handler, http.MethodPost, "/api/yggdrasil/authserver/authenticate",
		map[string]any{"username": "alex", "password": "alex12345"}, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("认证失败: %d", recorder.Code)
	}
	accessToken := testenv.Decode(t, recorder)["accessToken"].(string)

	adminToken := testenv.Token(t, instance, "admin")
	recorder = testenv.Do(handler, http.MethodDelete, "/api/admin/users/"+core.OfflineUUID("alex"), nil, adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("删号失败: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = testenv.Do(handler, http.MethodPost, "/api/yggdrasil/authserver/validate",
		map[string]any{"accessToken": accessToken}, "")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("删号后 Yggdrasil 令牌应当 403，实际 %d", recorder.Code)
	}
}

// 启动器视角的一条龙：从站点地址发现 Yggdrasil 服务，再走完进服流程。
func TestLauncherDiscoveryThenJoin(t *testing.T) {
	instance := testenv.New(t)
	handler := api.NewHandler(instance)

	// DESIGN.md 3.1：站点首页用 ALI 头指出 Yggdrasil 服务在哪（首页由站点外壳负责发出，
	// 那条在根目录 web_test.go 里测）。启动器顺着这个地址走完下面的流程。
	location := yggdrasil.AliasPrefix

	// 顺着 ALI 摸到根元数据，拿到签名公钥
	recorder := testenv.Do(handler, http.MethodGet, location, nil, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("根元数据 = %d", recorder.Code)
	}
	meta := testenv.Decode(t, recorder)
	if _, ok := meta["signaturePublickey"].(string); !ok {
		t.Fatalf("根元数据里没有 signaturePublickey: %v", meta)
	}

	// 登录 → 进服登记 → 服务端核对
	recorder = testenv.Do(handler, http.MethodPost, location+"/authserver/authenticate",
		map[string]any{"username": "steve", "password": "steve123"}, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("认证失败: %d %s", recorder.Code, recorder.Body.String())
	}
	accessToken := testenv.Decode(t, recorder)["accessToken"].(string)
	playerUUID := core.OfflineUUID("steve")

	recorder = testenv.Do(handler, http.MethodPost, location+"/sessionserver/session/minecraft/join",
		map[string]any{"accessToken": accessToken, "selectedProfile": playerUUID, "serverId": "端到端"}, "")
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("进服登记 = %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = testenv.Do(handler, http.MethodGet,
		location+"/sessionserver/session/minecraft/hasJoined?username=steve&serverId=端到端", nil, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("hasJoined = %d", recorder.Code)
	}
	profile := testenv.Decode(t, recorder)
	if profile["id"] != playerUUID || profile["name"] != "steve" {
		t.Fatalf("角色信息不对: %v", profile)
	}
	properties, ok := profile["properties"].([]any)
	if !ok || len(properties) != 1 {
		t.Fatalf("应当带一条 textures 属性: %v", profile["properties"])
	}
	entry := properties[0].(map[string]any)
	if entry["name"] != "textures" || entry["signature"] == nil || entry["signature"] == "" {
		t.Fatalf("textures 属性应当带签名: %v", entry)
	}
}

// 规范：材质 URL 的文件名就是材质 hash（客户端直接拿它当缓存键用）。
// 入库的字节能被重新编码过，所以这个「文件名 = 所服务字节的 sha256」必须真的成立。
func TestTextureURLNameIsTheHashOfServedBytes(t *testing.T) {
	instance := testenv.New(t)
	handler := api.NewHandler(instance)

	// 给 steve 装备一张内置皮肤
	items, err := instance.Store.List("skin", "all", "", "")
	if err != nil || len(items) == 0 {
		t.Fatalf("内置资源没灌进来: %v %d", err, len(items))
	}
	steve := instance.Store.UserByUsername("steve")
	if status, err := instance.Store.Equip(items[0].ID, steve.UUID); err != nil || status != "ok" {
		t.Fatalf("装备失败: %v %v", status, err)
	}

	// 从 profile 里拿 textures.SKIN.url
	playerUUID := core.OfflineUUID("steve")
	recorder := testenv.Do(handler, http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/profile/"+playerUUID+"?unsigned=false", nil, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("查询角色 = %d", recorder.Code)
	}
	body := testenv.Decode(t, recorder)
	properties, _ := body["properties"].([]any)
	if len(properties) != 1 {
		t.Fatalf("角色属性不对: %v", body["properties"])
	}
	value := properties[0].(map[string]any)["value"].(string)
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("textures 属性不是合法 base64: %v", err)
	}
	var payload struct {
		Textures map[string]struct {
			URL string `json:"url"`
		} `json:"textures"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("textures 属性不是合法 JSON: %v", err)
	}
	url := payload.Textures["SKIN"].URL
	if !strings.HasPrefix(url, "http://example.com/asset/") {
		t.Fatalf("材质 URL 不对: %q", url)
	}

	// 顺着这个 URL 拉回来，末段必须等于字节的 sha256
	recorder = testenv.Do(handler, http.MethodGet, strings.TrimPrefix(url, "http://example.com"), nil, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("拉材质 = %d", recorder.Code)
	}
	sum := sha256.Sum256(recorder.Body.Bytes())
	if want := url[strings.LastIndex(url, "/")+1:]; want != hex.EncodeToString(sum[:]) {
		t.Fatalf("URL 末段 %s != 服务字节的 sha256 %s", want, hex.EncodeToString(sum[:]))
	}
}
