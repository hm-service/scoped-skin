// Package yggdrasil_test 是对着《Yggdrasil 服务端技术规范》写的接口测试：
//
//	https://yushijinhun.github.io/authlib-injector/zh/Yggdrasil-服务端技术规范.html
//
// 每条用例都尽量落在规范原文上（状态码、错误码、错误文案、字段名、签名算法），
// 而不是落在实现细节上。
package yggdrasil_test

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"scoped-skin/api/yggdrasil"
	"scoped-skin/app"
	"scoped-skin/core"
	"scoped-skin/db"
	"scoped-skin/internal/testenv"
)

// ── 测试脚手架 ──────────────────────────────────────────────────────────

type harness struct {
	t   *testing.T
	app *app.App
	mux *http.ServeMux
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, nil)
}

// newHarnessWith 同上，但允许先改一下实例配置（例如 TrustedProxies）。
func newHarnessWith(t *testing.T, mutate func(*app.Config)) *harness {
	t.Helper()
	instance := testenv.NewWith(t, mutate)
	mux := http.NewServeMux()
	yggdrasil.New(instance).Register(mux)
	return &harness{t: t, app: instance, mux: mux}
}

// do 发一个请求；remoteAddr 为空时用 httptest 的默认值 192.0.2.1:1234。
func (h *harness) do(method, path string, body any, remoteAddr string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.doWith(method, path, body, remoteAddr, nil)
}

// doWith 同上，额外塞几个请求头（例如反代给的 X-Forwarded-For）。
func (h *harness) doWith(method, path string, body any, remoteAddr string, headers map[string]string) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader *strings.Reader
	if body != nil {
		switch typed := body.(type) {
		case string:
			reader = strings.NewReader(typed)
		default:
			encoded, err := json.Marshal(body)
			if err != nil {
				h.t.Fatalf("序列化请求体失败: %v", err)
			}
			reader = strings.NewReader(string(encoded))
		}
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if remoteAddr != "" {
		request.RemoteAddr = remoteAddr
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	h.mux.ServeHTTP(recorder, request)
	return recorder
}

type profileJSON struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Properties []struct {
		Name      string `json:"name"`
		Value     string `json:"value"`
		Signature string `json:"signature"`
	} `json:"properties"`
}

type authenticateJSON struct {
	AccessToken       string        `json:"accessToken"`
	ClientToken       string        `json:"clientToken"`
	AvailableProfiles []profileJSON `json:"availableProfiles"`
	SelectedProfile   *profileJSON  `json:"selectedProfile"`
	User              *struct {
		ID         string `json:"id"`
		Properties []any  `json:"properties"`
	} `json:"user"`
}

type errorJSON struct {
	Error        string `json:"error"`
	ErrorMessage string `json:"errorMessage"`
}

// authenticate 走一遍登录并断言成功。
func (h *harness) authenticate(username, password string, extra map[string]any) authenticateJSON {
	h.t.Helper()
	payload := map[string]any{"username": username, "password": password}
	for key, value := range extra {
		payload[key] = value
	}
	recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/authenticate", payload, "")
	if recorder.Code != http.StatusOK {
		h.t.Fatalf("认证失败: %d %s", recorder.Code, recorder.Body.String())
	}
	var response authenticateJSON
	testenv.DecodeInto(h.t, recorder, &response)
	return response
}

func (h *harness) wantsError(recorder *httptest.ResponseRecorder, status int, kind, message string) {
	h.t.Helper()
	if recorder.Code != status {
		h.t.Fatalf("状态码 = %d，想要 %d（%s）", recorder.Code, status, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		h.t.Fatalf("Content-Type = %q，规范要求 application/json; charset=utf-8", contentType)
	}
	var payload errorJSON
	testenv.DecodeInto(h.t, recorder, &payload)
	if payload.Error != kind {
		h.t.Fatalf("error = %q，想要 %q", payload.Error, kind)
	}
	if message != "" && payload.ErrorMessage != message {
		h.t.Fatalf("errorMessage = %q，想要 %q", payload.ErrorMessage, message)
	}
}

// texturePayload 解开 textures 属性里那串 base64。
type texturePayload struct {
	Timestamp   int64  `json:"timestamp"`
	ProfileID   string `json:"profileId"`
	ProfileName string `json:"profileName"`
	Textures    map[string]struct {
		URL      string `json:"url"`
		Metadata *struct {
			Model string `json:"model"`
		} `json:"metadata"`
	} `json:"textures"`
}

func decodeTextureProperty(t *testing.T, value string) texturePayload {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("textures 属性不是合法 base64: %v", err)
	}
	var payload texturePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("textures 属性不是合法 JSON: %v (%s)", err, raw)
	}
	return payload
}

// verifySignature 按规范验签：SHA1withRSA，签的对象是属性值（那串 base64）本身。
func verifySignature(t *testing.T, publicPEM, value, signature string) {
	t.Helper()
	block, _ := pem.Decode([]byte(publicPEM))
	if block == nil {
		t.Fatalf("公钥不是合法 PEM: %q", publicPEM)
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("公钥解析失败: %v", err)
	}
	public, ok := parsed.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("公钥不是 RSA 公钥")
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

func metaOf(t *testing.T, h *harness) map[string]any {
	t.Helper()
	recorder := h.do(http.MethodGet, "/api/yggdrasil", nil, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("根接口状态码 = %d", recorder.Code)
	}
	return testenv.Decode(t, recorder)
}

// ── 根接口（API 元数据获取） ────────────────────────────────────────────

func TestMetadataRoot(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	for _, path := range []string{"/api/yggdrasil", "/api/yggdrasil/"} {
		recorder := h.do(http.MethodGet, path, nil, "")
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, recorder.Code)
		}
		if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
			t.Fatalf("GET %s Content-Type = %q", path, contentType)
		}
	}

	body := metaOf(t, h)
	// 规范：字段名就是 signaturePublickey（小写 k）
	publicPEM, ok := body["signaturePublickey"].(string)
	if !ok {
		t.Fatalf("缺少 signaturePublickey: %v", body)
	}
	if _, exists := body["signaturePublicKey"]; exists {
		t.Fatalf("不该出现驼峰写法的 signaturePublicKey")
	}
	if !strings.HasPrefix(publicPEM, "-----BEGIN PUBLIC KEY-----") ||
		!strings.HasSuffix(publicPEM, "-----END PUBLIC KEY-----\n") {
		t.Fatalf("公钥 PEM 格式不对: %q", publicPEM)
	}
	if publicPEM != h.app.Keys.PublicPEM() {
		t.Fatalf("元数据里的公钥与实例密钥不一致")
	}

	meta, ok := body["meta"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 meta: %v", body)
	}
	if meta["implementationName"] != "scoped-skin" {
		t.Fatalf("implementationName = %v", meta["implementationName"])
	}
	if meta["feature.non_email_login"] != true {
		t.Fatalf("feature.non_email_login = %v，本站用账号名登录，应为 true", meta["feature.non_email_login"])
	}

	domains, ok := body["skinDomains"].([]any)
	if !ok || len(domains) == 0 {
		t.Fatalf("skinDomains 缺失: %v", body["skinDomains"])
	}
	// 请求的 Host 是 example.com（httptest.NewRequest 的默认值），应当被带进白名单。
	seen := map[string]bool{}
	for _, domain := range domains {
		seen[domain.(string)] = true
	}
	for _, wanted := range []string{"example.com", "127.0.0.1", "localhost"} {
		if !seen[wanted] {
			t.Fatalf("skinDomains 缺少 %q: %v", wanted, domains)
		}
	}
}

// ── 登录 / 刷新 / 校验 / 吊销 / 登出 ───────────────────────────────────

func TestAuthenticateSuccess(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	response := h.authenticate("steve", "steve123", nil)
	if len(response.AccessToken) != 32 || len(response.ClientToken) != 32 {
		t.Fatalf("令牌应当是 32 位无横线 hex: %q / %q", response.AccessToken, response.ClientToken)
	}
	if _, err := hex.DecodeString(response.AccessToken); err != nil {
		t.Fatalf("accessToken 不是十六进制: %v", err)
	}
	// 规范：客户端没给 clientToken 时服务端随机生成一个
	if response.AccessToken == response.ClientToken {
		t.Fatalf("accessToken 与 clientToken 不该相同")
	}
	wantUUID := core.OfflineUUID("steve")
	if len(response.AvailableProfiles) != 1 || response.AvailableProfiles[0].ID != wantUUID ||
		response.AvailableProfiles[0].Name != "steve" {
		t.Fatalf("availableProfiles 不对: %+v", response.AvailableProfiles)
	}
	if response.SelectedProfile == nil || response.SelectedProfile.ID != wantUUID {
		t.Fatalf("selectedProfile 不对: %+v", response.SelectedProfile)
	}
	// 规范：user 仅在 requestUser 为 true 时包含
	if response.User != nil {
		t.Fatalf("没要 requestUser 却回了 user: %+v", response.User)
	}

	// 客户端指定的 clientToken 要原样回显（规范：可以是任意字符串）
	given := h.authenticate("steve", "steve123", map[string]any{"clientToken": "我的启动器-1"})
	if given.ClientToken != "我的启动器-1" {
		t.Fatalf("clientToken = %q，应当原样回显", given.ClientToken)
	}

	withUser := h.authenticate("steve", "steve123", map[string]any{"requestUser": true})
	if withUser.User == nil || withUser.User.ID != wantUUID {
		t.Fatalf("requestUser=true 时应当回 user: %+v", withUser.User)
	}
}

func TestAuthenticateRejectsBadCredentials(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	cases := []struct {
		name     string
		username string
		password string
	}{
		{"密码错误", "steve", "nope"},
		{"账号不存在", "nobody", "nope"},
		{"空账号", "", ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			start := time.Now()
			recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/authenticate", map[string]any{
				"username": testCase.username, "password": testCase.password,
			}, "")
			h.wantsError(recorder, http.StatusForbidden,
				"ForbiddenOperationException", "Invalid credentials. Invalid username or password.")
			// DESIGN.md 5.3：账号存不存在都要花掉至少 1 秒
			if elapsed := time.Since(start); elapsed < core.LoginMinSeconds {
				t.Fatalf("校验只花了 %v，应当恒定 ≥ %v", elapsed, core.LoginMinSeconds)
			}
		})
	}
}

func TestAuthenticateRejectsMalformedBody(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/authenticate", "{不是 JSON", "")
	h.wantsError(recorder, http.StatusBadRequest, "Bad Request", "")
}

// DESIGN.md 3.5：防爆破不靠限流，靠恒定请求时间 —— 连打多次错的也不会被拦，
// 但每次都至少花 1 秒；账号存不存在都一样。
func TestAuthenticateHasNoThrottleButConstantTime(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	for attempt := 1; attempt <= 6; attempt++ {
		start := time.Now()
		recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/authenticate", map[string]any{
			"username": "alex", "password": "wrong",
		}, "")
		h.wantsError(recorder, http.StatusForbidden,
			"ForbiddenOperationException", "Invalid credentials. Invalid username or password.")
		if elapsed := time.Since(start); elapsed < core.LoginMinSeconds {
			t.Fatalf("第 %d 次只花了 %v，应当至少 %v", attempt, elapsed, core.LoginMinSeconds)
		}
	}

	// 不存在的账号也走同一条恒定耗时路径
	start := time.Now()
	recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/authenticate", map[string]any{
		"username": "查无此人", "password": "whatever",
	}, "")
	h.wantsError(recorder, http.StatusForbidden,
		"ForbiddenOperationException", "Invalid credentials. Invalid username or password.")
	if elapsed := time.Since(start); elapsed < core.LoginMinSeconds {
		t.Fatalf("不存在的账号只花了 %v，应当至少 %v", elapsed, core.LoginMinSeconds)
	}

	// 挨了这么多下之后，正确密码照样能登录（说明确实没在限流）
	response := h.authenticate("alex", "alex12345", nil)
	if response.SelectedProfile == nil || response.SelectedProfile.Name != "alex" {
		t.Fatalf("被限流了: %+v", response.SelectedProfile)
	}
}

func TestRefreshRotatesToken(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	first := h.authenticate("steve", "steve123", nil)
	recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/refresh", map[string]any{
		"accessToken": first.AccessToken, "clientToken": first.ClientToken,
	}, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("刷新失败: %d %s", recorder.Code, recorder.Body.String())
	}
	var second authenticateJSON
	testenv.DecodeInto(t, recorder, &second)
	if second.AccessToken == first.AccessToken {
		t.Fatalf("刷新应当换一个新令牌")
	}
	// 规范：新令牌的 clientToken 应与原令牌的相同
	if second.ClientToken != first.ClientToken {
		t.Fatalf("clientToken 变了: %q → %q", first.ClientToken, second.ClientToken)
	}
	if second.SelectedProfile == nil || second.SelectedProfile.Name != "steve" {
		t.Fatalf("selectedProfile 不对: %+v", second.SelectedProfile)
	}

	// 旧令牌被吊销，新令牌可用
	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/authserver/validate",
		map[string]any{"accessToken": first.AccessToken}, ""),
		http.StatusForbidden, "ForbiddenOperationException", "Invalid token.")
	if recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/validate",
		map[string]any{"accessToken": second.AccessToken}, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("新令牌校验 = %d", recorder.Code)
	}
}

func TestRefreshFailuresKeepOldToken(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	first := h.authenticate("steve", "steve123", nil)

	// clientToken 对不上
	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/authserver/refresh", map[string]any{
		"accessToken": first.AccessToken, "clientToken": "别的客户端",
	}, ""), http.StatusForbidden, "ForbiddenOperationException", "Invalid token.")

	// 令牌根本不存在
	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/authserver/refresh", map[string]any{
		"accessToken": "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f",
	}, ""), http.StatusForbidden, "ForbiddenOperationException", "Invalid token.")

	// 规范：若请求失败，原令牌依然有效
	if recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/validate",
		map[string]any{"accessToken": first.AccessToken}, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("刷新失败后原令牌被误伤: %d", recorder.Code)
	}
}

// 规范：试图向一个已经绑定了角色的令牌指定其要绑定的角色 → 400 IllegalArgumentException。
func TestRefreshSelectedProfileIsRejected(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	first := h.authenticate("steve", "steve123", nil)
	recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/refresh", map[string]any{
		"accessToken":     first.AccessToken,
		"selectedProfile": map[string]any{"id": core.OfflineUUID("steve"), "name": "steve"},
	}, "")
	h.wantsError(recorder, http.StatusBadRequest, "IllegalArgumentException",
		"Access token already has a profile assigned.")
}

func TestValidateAndInvalidate(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	session := h.authenticate("steve", "steve123", nil)

	// clientToken 给对了才放行，给了错的也一样是「令牌无效」
	if recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/validate", map[string]any{
		"accessToken": session.AccessToken, "clientToken": session.ClientToken,
	}, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("校验 = %d", recorder.Code)
	}
	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/authserver/validate", map[string]any{
		"accessToken": session.AccessToken, "clientToken": "别的客户端",
	}, ""), http.StatusForbidden, "ForbiddenOperationException", "Invalid token.")
	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/authserver/validate",
		map[string]any{"accessToken": "deadbeef"}, ""),
		http.StatusForbidden, "ForbiddenOperationException", "Invalid token.")

	// 规范：无论操作是否成功，吊销都回 204
	for _, token := range []string{session.AccessToken, "deadbeef"} {
		recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/invalidate",
			map[string]any{"accessToken": token}, "")
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("吊销 %q = %d", token, recorder.Code)
		}
		if body := recorder.Body.String(); body != "" {
			t.Fatalf("204 不该带响应体: %q", body)
		}
	}
	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/authserver/validate",
		map[string]any{"accessToken": session.AccessToken}, ""),
		http.StatusForbidden, "ForbiddenOperationException", "Invalid token.")
}

func TestSignoutRevokesEveryToken(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	first := h.authenticate("steve", "steve123", nil)
	second := h.authenticate("steve", "steve123", nil)

	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/authserver/signout", map[string]any{
		"username": "steve", "password": "错的",
	}, ""), http.StatusForbidden,
		"ForbiddenOperationException", "Invalid credentials. Invalid username or password.")

	recorder := h.do(http.MethodPost, "/api/yggdrasil/authserver/signout", map[string]any{
		"username": "steve", "password": "steve123",
	}, "")
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("登出 = %d %s", recorder.Code, recorder.Body.String())
	}
	for name, token := range map[string]string{"第一个": first.AccessToken, "第二个": second.AccessToken} {
		h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/authserver/validate",
			map[string]any{"accessToken": token}, ""),
			http.StatusForbidden, "ForbiddenOperationException", "Invalid token.")
		_ = name
	}
}

// ── 会话部分 ────────────────────────────────────────────────────────────

func TestJoinAndHasJoined(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	session := h.authenticate("steve", "steve123", nil)
	playerUUID := core.OfflineUUID("steve")

	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/sessionserver/session/minecraft/join",
		map[string]any{"accessToken": "bad", "selectedProfile": playerUUID, "serverId": "s1"}, ""),
		http.StatusForbidden, "ForbiddenOperationException", "Invalid token.")
	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/sessionserver/session/minecraft/join",
		map[string]any{"accessToken": session.AccessToken, "selectedProfile": core.OfflineUUID("alex"), "serverId": "s1"}, ""),
		http.StatusForbidden, "ForbiddenOperationException", "Invalid token.")

	if recorder := h.do(http.MethodPost, "/api/yggdrasil/sessionserver/session/minecraft/join",
		map[string]any{"accessToken": session.AccessToken, "selectedProfile": playerUUID, "serverId": "s1"}, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("进服登记 = %d %s", recorder.Code, recorder.Body.String())
	}

	recorder := h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/hasJoined?username=steve&serverId=s1", nil, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("hasJoined = %d", recorder.Code)
	}
	var profile profileJSON
	testenv.DecodeInto(t, recorder, &profile)
	if profile.ID != playerUUID || profile.Name != "steve" {
		t.Fatalf("hasJoined 的角色不对: %+v", profile)
	}
	if len(profile.Properties) != 1 || profile.Properties[0].Name != "textures" {
		t.Fatalf("hasJoined 应当带 textures 属性: %+v", profile.Properties)
	}
	// 规范：hasJoined 返回的是「完整信息（包含角色属性及数字签名）」
	if profile.Properties[0].Signature == "" {
		t.Fatalf("hasJoined 的 textures 属性必须带签名")
	}
	verifySignature(t, h.app.Keys.PublicPEM(), profile.Properties[0].Value, profile.Properties[0].Signature)

	// 核对一次即作废（防重放）
	replay := h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/hasJoined?username=steve&serverId=s1", nil, "")
	if replay.Code != http.StatusNoContent {
		t.Fatalf("重放 hasJoined 应当 204，实际 %d", replay.Code)
	}

	// serverId 不对 / 用户不存在 / 少参数
	if recorder := h.do(http.MethodPost, "/api/yggdrasil/sessionserver/session/minecraft/join",
		map[string]any{"accessToken": session.AccessToken, "selectedProfile": playerUUID, "serverId": "s2"}, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("第二次进服登记 = %d", recorder.Code)
	}
	for _, query := range []string{"?username=steve&serverId=s3", "?username=nobody&serverId=s2"} {
		if recorder := h.do(http.MethodGet,
			"/api/yggdrasil/sessionserver/session/minecraft/hasJoined"+query, nil, ""); recorder.Code != http.StatusNoContent {
			t.Fatalf("hasJoined%s 应当 204，实际 %d", query, recorder.Code)
		}
	}
	h.wantsError(h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/hasJoined?username=steve", nil, ""),
		http.StatusBadRequest, "Bad Request", "")
}

// 规范：ip 是可选参数；带上时应当核对是不是 join 时那个客户端。
func TestHasJoinedChecksClientIP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	session := h.authenticate("steve", "steve123", nil)
	playerUUID := core.OfflineUUID("steve")

	if recorder := h.do(http.MethodPost, "/api/yggdrasil/sessionserver/session/minecraft/join",
		map[string]any{"accessToken": session.AccessToken, "selectedProfile": playerUUID, "serverId": "s9"},
		"192.0.2.7:5555"); recorder.Code != http.StatusNoContent {
		t.Fatalf("进服登记 = %d", recorder.Code)
	}

	// IP 对不上：204，而且不消费掉暗号
	if recorder := h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/hasJoined?username=steve&serverId=s9&ip=10.0.0.1",
		nil, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("IP 对不上应当 204，实际 %d", recorder.Code)
	}
	// IP 对得上：放行
	if recorder := h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/hasJoined?username=steve&serverId=s9&ip=192.0.2.7",
		nil, ""); recorder.Code != http.StatusOK {
		t.Fatalf("IP 对得上应当 200，实际 %d", recorder.Code)
	}
}

// DESIGN.md 5.5：站在可信反代后面时，join 记下的是反代头里的真实客户端 IP，
// hasJoined 的 ip 比对才有意义（不然所有玩家都会被记成反代自己）。
func TestHasJoinedUsesRealClientIPBehindTrustedProxy(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, func(cfg *app.Config) { cfg.TrustedProxies = "192.0.2.1/32" })

	session := h.authenticate("steve", "steve123", nil)
	playerUUID := core.OfflineUUID("steve")
	joined := map[string]any{"accessToken": session.AccessToken, "selectedProfile": playerUUID, "serverId": "s10"}

	// 反代的地址是 192.0.2.1（可信），真实客户端在 X-Forwarded-For 里
	if recorder := h.doWith(http.MethodPost, "/api/yggdrasil/sessionserver/session/minecraft/join",
		joined, "192.0.2.1:5555", map[string]string{"X-Forwarded-For": "198.51.100.7"}); recorder.Code != http.StatusNoContent {
		t.Fatalf("进服登记 = %d", recorder.Code)
	}

	// MC 服务端拿来的是反代地址 → 对不上 → 204
	if recorder := h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/hasJoined?username=steve&serverId=s10&ip=192.0.2.1",
		nil, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("拿反代地址来比对应当 204，实际 %d", recorder.Code)
	}
	// 真实客户端 IP → 放行
	if recorder := h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/hasJoined?username=steve&serverId=s10&ip=198.51.100.7",
		nil, ""); recorder.Code != http.StatusOK {
		t.Fatalf("真实客户端 IP 应当 200，实际 %d", recorder.Code)
	}

	// 对端不可信时（默认配置），伪造的 XFF 换不来身份
	plain := newHarness(t)
	session = plain.authenticate("steve", "steve123", nil)
	joined = map[string]any{"accessToken": session.AccessToken, "selectedProfile": playerUUID, "serverId": "s11"}
	if recorder := plain.doWith(http.MethodPost, "/api/yggdrasil/sessionserver/session/minecraft/join",
		joined, "203.0.113.9:5555", map[string]string{"X-Forwarded-For": "198.51.100.7"}); recorder.Code != http.StatusNoContent {
		t.Fatalf("进服登记 = %d", recorder.Code)
	}
	if recorder := plain.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/hasJoined?username=steve&serverId=s11&ip=198.51.100.7",
		nil, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("伪造的 XFF 不该被认，实际 %d", recorder.Code)
	}
}

// ── 角色部分 ────────────────────────────────────────────────────────────

func TestProfile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	playerUUID := core.OfflineUUID("steve")

	// unsigned 默认 true（不带签名）
	recorder := h.do(http.MethodGet, "/api/yggdrasil/sessionserver/session/minecraft/profile/"+playerUUID, nil, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("查询角色 = %d", recorder.Code)
	}
	var profile profileJSON
	testenv.DecodeInto(t, recorder, &profile)
	if profile.ID != playerUUID {
		t.Fatalf("角色 UUID = %q，想要 %q", profile.ID, playerUUID)
	}
	if len(profile.Properties) != 1 {
		t.Fatalf("角色属性不对: %+v", profile.Properties)
	}
	if profile.Properties[0].Signature != "" {
		t.Fatalf("unsigned 默认 true，不该带签名")
	}
	payload := decodeTextureProperty(t, profile.Properties[0].Value)
	if payload.ProfileID != playerUUID || payload.ProfileName != "steve" {
		t.Fatalf("textures 载荷不对: %+v", payload)
	}
	if payload.Timestamp <= 0 {
		t.Fatalf("timestamp 应当是 Java 毫秒时间戳: %d", payload.Timestamp)
	}
	// 没装备皮肤时 textures 是空对象（而不是 null）
	if payload.Textures == nil || len(payload.Textures) != 0 {
		t.Fatalf("没装备皮肤时 textures 应当是空对象: %v", payload.Textures)
	}

	// unsigned=false → 带签名
	recorder = h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/profile/"+playerUUID+"?unsigned=false", nil, "")
	testenv.DecodeInto(t, recorder, &profile)
	if profile.Properties[0].Signature == "" {
		t.Fatalf("unsigned=false 时必须带签名")
	}
	verifySignature(t, h.app.Keys.PublicPEM(), profile.Properties[0].Value, profile.Properties[0].Signature)

	// 带横线的 UUID 也认
	dashed := playerUUID[0:8] + "-" + playerUUID[8:12] + "-" + playerUUID[12:16] + "-" + playerUUID[16:20] + "-" + playerUUID[20:32]
	if recorder := h.do(http.MethodGet, "/api/yggdrasil/sessionserver/session/minecraft/profile/"+dashed, nil, ""); recorder.Code != http.StatusOK {
		t.Fatalf("带横线的 UUID 应当也认，实际 %d", recorder.Code)
	}
	// 查无此人 → 204
	if recorder := h.do(http.MethodGet, "/api/yggdrasil/sessionserver/session/minecraft/profile/00000000000000000000000000000000", nil, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("查无此人应当 204，实际 %d", recorder.Code)
	}
}

// 装备皮肤后，textures 里要出现指向 /asset/{sha256} 的 SKIN，模型按库里的 model_type 映射。
func TestProfileCarriesEquippedSkin(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	items, err := h.app.Store.List("skin", "all", "", "")
	if err != nil || len(items) == 0 {
		t.Fatalf("内置资源没灌进来: %v %d", err, len(items))
	}
	slimID := pickModel(t, items, "alex")
	wideID := pickModel(t, items, "steve")

	steve := h.app.Store.UserByUsername("steve")
	if status, err := h.app.Store.Equip(wideID, steve.UUID); err != nil || status != "ok" {
		t.Fatalf("装备失败: %v %v", status, err)
	}
	playerUUID := core.OfflineUUID("steve")
	recorder := h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/profile/"+playerUUID+"?unsigned=false", nil, "")
	var profile profileJSON
	testenv.DecodeInto(t, recorder, &profile)
	payload := decodeTextureProperty(t, profile.Properties[0].Value)
	skin, ok := payload.Textures["SKIN"]
	if !ok {
		t.Fatalf("装备后应当有 SKIN: %+v", payload.Textures)
	}
	wantURL := "http://example.com/asset/" + wideID
	if skin.URL != wantURL {
		t.Fatalf("SKIN.url = %q，想要 %q", skin.URL, wantURL)
	}
	// steve → default
	if skin.Metadata == nil || skin.Metadata.Model != "default" {
		t.Fatalf("steve 模型的 metadata 应当是 default: %+v", skin.Metadata)
	}

	// alex → slim
	if status, err := h.app.Store.Equip(slimID, steve.UUID); err != nil || status != "ok" {
		t.Fatalf("装备失败: %v %v", status, err)
	}
	recorder = h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/profile/"+playerUUID, nil, "")
	testenv.DecodeInto(t, recorder, &profile)
	payload = decodeTextureProperty(t, profile.Properties[0].Value)
	if payload.Textures["SKIN"].Metadata == nil || payload.Textures["SKIN"].Metadata.Model != "slim" {
		t.Fatalf("alex 模型的 metadata 应当是 slim: %+v", payload.Textures["SKIN"].Metadata)
	}
	// 材质 URL 的文件名就是材质 hash（规范：客户端直接拿它当 hash 用）
	if !strings.HasSuffix(payload.Textures["SKIN"].URL, "/"+slimID) {
		t.Fatalf("URL 末段应当是 sha256: %q", payload.Textures["SKIN"].URL)
	}
}

func pickModel(t *testing.T, items []db.Item, model string) string {
	t.Helper()
	for _, item := range items {
		if item.Model == model {
			return item.ID
		}
	}
	t.Fatalf("内置资源里没有 model=%s 的皮肤", model)
	return ""
}

func TestProfilesByNames(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	recorder := h.do(http.MethodPost, "/api/yggdrasil/api/profiles/minecraft",
		[]string{"steve", "alex", "STEVE", "nobody"}, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("批量查询 = %d %s", recorder.Code, recorder.Body.String())
	}
	var found []map[string]any
	testenv.DecodeInto(t, recorder, &found)
	if len(found) != 2 {
		t.Fatalf("应当查到 2 个角色（去重、忽略不存在）: %v", found)
	}
	for _, entry := range found {
		// 规范：这一份不含角色属性
		if _, exists := entry["properties"]; exists {
			t.Fatalf("批量查询不该返回 properties: %v", entry)
		}
		if entry["id"] == nil || entry["name"] == nil {
			t.Fatalf("角色信息不全: %v", entry)
		}
	}

	// 规范的安全提示：单次查询的角色数要有上限
	tooMany := make([]string, 0, 32)
	for index := range 32 {
		tooMany = append(tooMany, "name"+string(rune('a'+index)))
	}
	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/api/profiles/minecraft", tooMany, ""),
		http.StatusBadRequest, "Bad Request", "")
	// 不是数组
	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/api/profiles/minecraft", map[string]any{}, ""),
		http.StatusBadRequest, "Bad Request", "")
}

// ── 一般 HTTP 异常 ──────────────────────────────────────────────────────

func TestMethodAndPathErrors(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// 规范：一般 HTTP 异常的 error 取该状态码的 Reason Phrase
	h.wantsError(h.do(http.MethodGet, "/api/yggdrasil/authserver/authenticate", nil, ""),
		http.StatusMethodNotAllowed, "Method Not Allowed", "")
	h.wantsError(h.do(http.MethodPost, "/api/yggdrasil/sessionserver/session/minecraft/hasJoined", nil, ""),
		http.StatusMethodNotAllowed, "Method Not Allowed", "")
	h.wantsError(h.do(http.MethodGet, "/api/yggdrasil/nope", nil, ""),
		http.StatusNotFound, "Not Found", "")
}
