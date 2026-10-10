package yggdrasil_test

import (
	"bytes"
	"crypto/tls"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"scoped-skin/core"
	"scoped-skin/internal/testenv"
)

// 批量查询的边界：正好 10 个（规范给的实现上限）要放行，11 个才拒；
// 空名 / 全空白项被跳过；JSON null 当成空数组。
func TestProfilesBatchBoundaries(t *testing.T) {
	h := newHarness(t)
	const endpoint = "/api/yggdrasil/api/profiles/minecraft"

	// 正好 10 个：含重复（大小写/空白不同）与查不到的
	names := []string{"steve", "alex", "notch", "dinnerbone", "guest", "STEVE", "  alex  ", "", "   ", "nobody"}
	if len(names) != 10 {
		t.Fatalf("测试样本应当是 10 个名字，实际 %d", len(names))
	}
	recorder := h.do(http.MethodPost, endpoint, names, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("正好 10 个状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	var found []map[string]any
	testenv.DecodeInto(t, recorder, &found)
	if len(found) != 5 {
		t.Fatalf("应当查到 5 个去重后的角色（steve/alex/notch/dinnerbone/guest），实际 %d: %v",
			len(found), found)
	}
	for _, entry := range found {
		if entry["id"] == nil || entry["name"] == nil {
			t.Errorf("角色信息不全: %v", entry)
		}
	}

	// 11 个 → 400
	tooMany := append(append([]string{}, names...), "one-more")
	h.wantsError(h.do(http.MethodPost, endpoint, tooMany, ""),
		http.StatusBadRequest, "Bad Request", "Too many profiles requested")

	// JSON null → 200 空数组
	recorder = h.do(http.MethodPost, endpoint, "null", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("null 状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	if strings.TrimSpace(recorder.Body.String()) != "[]" {
		t.Errorf("null 应当回空数组，实际 %s", recorder.Body.String())
	}

	// 全是空白 → 200 空数组
	recorder = h.do(http.MethodPost, endpoint, []string{"", "   ", "\t"}, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("全空白状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	if strings.TrimSpace(recorder.Body.String()) != "[]" {
		t.Errorf("全空白应当回空数组，实际 %s", recorder.Body.String())
	}
}

// 请求体撞上中间件 MaxBytesReader 的上限时，decode 要认出 MaxBytesError 并回
// 规范格式的 400（而不是把「JSON 坏了」和「体太大」混为一谈）。
func TestAuthenticateOverBodyLimitIsBadRequest(t *testing.T) {
	h := newHarness(t)

	const limit = 4 << 20
	// 内容是一个合法 JSON 串的开头，长到超过上限；包一层 MaxBytesReader 模拟中间件
	body := `"` + strings.Repeat("a", limit+1024) + `"`
	recorder := httptest.NewRecorder()
	reader := http.MaxBytesReader(recorder, io.NopCloser(strings.NewReader(body)), limit)

	request := httptest.NewRequest(http.MethodPost, "/api/yggdrasil/authserver/authenticate", nil)
	request.Body = reader
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.mux.ServeHTTP(response, request)

	h.wantsError(response, http.StatusBadRequest, "Bad Request", "Request body too large")
}

// join 的请求体坏了要回规范格式的 400，不能当成「令牌无效」的 403。
func TestJoinRejectsMalformedBody(t *testing.T) {
	h := newHarness(t)
	h.wantsError(
		h.do(http.MethodPost, "/api/yggdrasil/sessionserver/session/minecraft/join", "{不是 JSON", ""),
		http.StatusBadRequest, "Bad Request", "Malformed JSON body")
}

// 其余几个 POST 端点的坏请求体也都要回规范格式的 400。
func TestAuthserverEndpointsRejectMalformedBodies(t *testing.T) {
	h := newHarness(t)
	for _, path := range []string{
		"/api/yggdrasil/authserver/authenticate",
		"/api/yggdrasil/authserver/refresh",
		"/api/yggdrasil/authserver/validate",
		"/api/yggdrasil/authserver/invalidate",
		"/api/yggdrasil/authserver/signout",
		"/api/yggdrasil/api/profiles/minecraft",
	} {
		h.wantsError(h.do(http.MethodPost, path, "{不是 JSON", ""),
			http.StatusBadRequest, "Bad Request", "Malformed JSON body")
	}
}

// refresh 的三条边角：requestUser=true 要带 user；令牌还在但角色已经被删掉
// （后台删号）必须按令牌无效处理。
func TestRefreshRequestUserAndDeletedProfile(t *testing.T) {
	h := newHarness(t)
	const refreshPath = "/api/yggdrasil/authserver/refresh"

	steveSession := h.authenticate("steve", "steve123", nil)
	recorder := h.do(http.MethodPost, refreshPath, map[string]any{
		"accessToken": steveSession.AccessToken,
		"clientToken": steveSession.ClientToken,
		"requestUser": true,
	}, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("refresh 状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	var refreshed struct {
		AccessToken string `json:"accessToken"`
		ClientToken string `json:"clientToken"`
		User        *struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	testenv.DecodeInto(t, recorder, &refreshed)
	if refreshed.User == nil || refreshed.User.ID != core.OfflineUUID("steve") {
		t.Errorf("requestUser=true 时应当回 user: %+v", refreshed.User)
	}
	if refreshed.AccessToken == "" || refreshed.AccessToken == steveSession.AccessToken {
		t.Errorf("refresh 应当换一个新 accessToken: %q", refreshed.AccessToken)
	}

	// 令牌在内存里，但角色已经从库里删掉：refresh 必须失败
	alexSession := h.authenticate("alex", "alex12345", nil)
	if status, err := h.app.Store.DeleteUser(core.OfflineUUID("alex")); err != nil || status != "deleted" {
		t.Fatalf("删角色失败: %q (%v)", status, err)
	}
	h.wantsError(h.do(http.MethodPost, refreshPath, map[string]any{
		"accessToken": alexSession.AccessToken,
		"clientToken": alexSession.ClientToken,
	}, ""), http.StatusForbidden, "ForbiddenOperationException", "Invalid token.")
}

// 同时装了皮肤与披风时，textures 里要有 SKIN 与 CAPE 两项，且都指向 /asset/{sha256}。
func TestProfileCarriesCape(t *testing.T) {
	h := newHarness(t)

	// 造一张 64×32 的合法披风并入库
	canvas := image.NewRGBA(image.Rect(0, 0, 64, 32))
	canvas.Set(0, 0, color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("编码披风 PNG 失败: %v", err)
	}
	cape, status, err := h.app.Store.AddUpload("cape", []string{"披风"}, buffer.Bytes())
	if err != nil || status != "created" {
		t.Fatalf("入库披风失败: %q (%v)", status, err)
	}
	items, err := h.app.Store.List("skin", "all", "", "")
	if err != nil || len(items) == 0 {
		t.Fatalf("内置皮肤没灌进来: %v %d", err, len(items))
	}
	skinID := pickKind(t, items, "skin.steve")

	steve := h.app.Store.UserByUsername("steve")
	if status, err := h.app.Store.Equip(skinID, steve.UUID); err != nil || status != "ok" {
		t.Fatalf("装备皮肤失败: %q (%v)", status, err)
	}
	if status, err := h.app.Store.Equip(cape.ID, steve.UUID); err != nil || status != "ok" {
		t.Fatalf("装备披风失败: %q (%v)", status, err)
	}

	recorder := h.do(http.MethodGet,
		"/api/yggdrasil/sessionserver/session/minecraft/profile/"+core.OfflineUUID("steve"), nil, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("查询角色 = %d（%s）", recorder.Code, recorder.Body.String())
	}
	var entry profileJSON
	testenv.DecodeInto(t, recorder, &entry)
	payload := decodeTextureProperty(t, entry.Properties[0].Value)

	skin, ok := payload.Textures["SKIN"]
	if !ok || skin.URL != "http://example.com/asset/"+skinID {
		t.Errorf("SKIN = %+v，想要 http://example.com/asset/%s", skin, skinID)
	}
	wornCape, ok := payload.Textures["CAPE"]
	if !ok {
		t.Fatalf("装备的披风没出现在 textures 里: %+v", payload.Textures)
	}
	if want := "http://example.com/asset/" + cape.ID; wornCape.URL != want {
		t.Errorf("CAPE.url = %q，想要 %q", wornCape.URL, want)
	}
	// 披风没有模型元数据（只有皮肤才有 default / slim）
	if wornCape.Metadata != nil {
		t.Errorf("披风不该带 model metadata: %+v", wornCape.Metadata)
	}
}

// 材质 URL 必须是绝对地址：认 TLS 连接与反代的 X-Forwarded-Proto（取第一段），
// Host 用请求本身带的。
func TestTextureURLSchemeAndHost(t *testing.T) {
	h := newHarness(t)

	items, err := h.app.Store.List("skin", "all", "", "")
	if err != nil || len(items) == 0 {
		t.Fatalf("内置资源没灌进来: %v %d", err, len(items))
	}
	skinID := pickKind(t, items, "skin.steve")
	steve := h.app.Store.UserByUsername("steve")
	if status, err := h.app.Store.Equip(skinID, steve.UUID); err != nil || status != "ok" {
		t.Fatalf("装备失败: %q (%v)", status, err)
	}
	playerUUID := core.OfflineUUID("steve")

	profileURL := func(host string, headers map[string]string, useTLS bool) string {
		request := httptest.NewRequest(http.MethodGet,
			"/api/yggdrasil/sessionserver/session/minecraft/profile/"+playerUUID, nil)
		request.Host = host
		if useTLS {
			request.TLS = &tls.ConnectionState{}
		}
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		recorder := httptest.NewRecorder()
		h.mux.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET profile（Host=%s）= %d（%s）", host, recorder.Code, recorder.Body.String())
		}
		var entry profileJSON
		testenv.DecodeInto(t, recorder, &entry)
		if len(entry.Properties) != 1 {
			t.Fatalf("角色属性不对: %+v", entry.Properties)
		}
		payload := decodeTextureProperty(t, entry.Properties[0].Value)
		skin, ok := payload.Textures["SKIN"]
		if !ok {
			t.Fatalf("装备的 SKIN 没出现在 textures 里: %+v", payload.Textures)
		}
		return skin.URL
	}

	cases := []struct {
		name    string
		host    string
		headers map[string]string
		tls     bool
		want    string
	}{
		{"明文直连", "plain.example.com", nil, false, "http://plain.example.com/asset/" + skinID},
		{"反代给 X-Forwarded-Proto", "proxy.example.com",
			map[string]string{"X-Forwarded-Proto": "https, http"}, false,
			"https://proxy.example.com/asset/" + skinID},
		{"直接 TLS", "tls.example.com", nil, true, "https://tls.example.com/asset/" + skinID},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := profileURL(testCase.host, testCase.headers, testCase.tls); got != testCase.want {
				t.Errorf("SKIN.url = %q，想要 %q", got, testCase.want)
			}
		})
	}
}

// Host 只有端口（`:8080`）时主机名解出来是空，skinDomains 要退回 127.0.0.1，
// 且不能出现空串或重复项。
func TestMetadataSkinDomainsWithPortOnlyHost(t *testing.T) {
	h := newHarness(t)

	request := httptest.NewRequest(http.MethodGet, "/api/yggdrasil", nil)
	request.Host = ":8080"
	recorder := httptest.NewRecorder()
	h.mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("元数据状态码 = %d", recorder.Code)
	}
	var payload map[string]any
	testenv.DecodeInto(t, recorder, &payload)
	domains, ok := payload["skinDomains"].([]any)
	if !ok || len(domains) == 0 {
		t.Fatalf("skinDomains 缺失: %v", payload["skinDomains"])
	}
	seen := map[string]bool{}
	for _, raw := range domains {
		domain, _ := raw.(string)
		if domain == "" {
			t.Errorf("skinDomains 里不该有空串: %v", domains)
		}
		if seen[domain] {
			t.Errorf("skinDomains 里有重复项 %q: %v", domain, domains)
		}
		seen[domain] = true
	}
	for _, wanted := range []string{"127.0.0.1", ":8080", "localhost"} {
		if !seen[wanted] {
			t.Errorf("skinDomains 缺少 %q: %v", wanted, domains)
		}
	}
}
