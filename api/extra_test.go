package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"scoped-skin/api"
	"scoped-skin/core"
	"scoped-skin/internal/testenv"
)

// 直接把原始字符串当请求体发出去（testenv.Do 会把 body 序列化成 JSON，
// 发不出「坏 JSON」）。
func rawRequest(handler http.Handler, method, target, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// 每个需要登录 / 管理员身份的入口在缺 token 时都要 401（而不是 403 或 500），
// 并且把语言协商的 401 文案走通。
func TestUnauthenticatedRoutesRequireToken(t *testing.T) {
	instance, handler := newSite(t)
	_ = instance

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/tags", nil},
		{http.MethodGet, "/api/me", nil},
		{http.MethodPost, "/api/me/equip", map[string]any{"asset_id": "x"}},
		{http.MethodPost, "/api/skins", nil},
		{http.MethodGet, "/api/admin/users", nil},
		{http.MethodPost, "/api/admin/users", map[string]any{}},
	}
	for _, testCase := range cases {
		recorder := testenv.Do(handler, testCase.method, testCase.path, testCase.body, "")
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s 状态码 = %d，想要 401（%s）",
				testCase.method, testCase.path, recorder.Code, recorder.Body.String())
			continue
		}
		if got := detail(t, recorder); got != "未登录" {
			t.Errorf("%s %s detail = %q，想要 %q", testCase.method, testCase.path, got, "未登录")
		}
		if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
			t.Errorf("%s %s WWW-Authenticate = %q", testCase.method, testCase.path, got)
		}
	}
}

// 坏 JSON 请求体一律回 400 request.json（不是 500，也不是把字段当空处理）。
func TestMalformedJSONBodies(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")
	steveUUID := core.OfflineUUID("steve")
	const broken = "{这不是合法的 JSON"

	cases := []struct {
		method, path string
		token        string
	}{
		{http.MethodPost, "/api/auth/login", ""},
		{http.MethodPost, "/api/precheck", token},
		{http.MethodPost, "/api/me/equip", token},
		{http.MethodPost, "/api/admin/users", token},
		{http.MethodPatch, "/api/admin/users/" + steveUUID, token},
	}
	for _, testCase := range cases {
		recorder := rawRequest(handler, testCase.method, testCase.path, broken, testCase.token)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s %s 状态码 = %d，想要 400（%s）",
				testCase.method, testCase.path, recorder.Code, recorder.Body.String())
			continue
		}
		if got := detail(t, recorder); got != "请求体不是合法的 JSON" {
			t.Errorf("%s %s detail = %q", testCase.method, testCase.path, got)
		}
	}
}

// /api/tags 是前端标签候选的数据源：登录后返回库里在用的标签与次数。
func TestTagsEndpoint(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")

	recorder := testenv.Do(handler, http.MethodGet, "/api/tags", nil, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/tags 状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	tags, ok := testenv.Decode(t, recorder)["tags"].([]any)
	if !ok || len(tags) == 0 {
		t.Fatalf("tags 应当是数组: %s", recorder.Body.String())
	}
	found := false
	for _, raw := range tags {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("标签条目不是对象: %v", raw)
		}
		// 内置资源都带「官方」，一共 9 张皮肤
		if entry["tag"] == "官方" && entry["count"] == float64(9) {
			found = true
		}
	}
	if !found {
		t.Errorf("标签统计里应当有「官方」×9: %s", recorder.Body.String())
	}
}

// 管理入口的路径参数与目标不存在时的状态码：非法 uuid → 400，合法但查无此人 → 404。
func TestAdminRouteTargetValidation(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")
	missingUUID := strings.Repeat("f", 32)

	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		recorder := testenv.Do(handler, method, "/api/admin/users/not-a-uuid", nil, token)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s 非法 uuid 状态码 = %d，想要 400（%s）", method, recorder.Code, recorder.Body.String())
		}
		if got := detail(t, recorder); got != "UUID 必须是 32 位小写十六进制字符" {
			t.Errorf("%s 非法 uuid detail = %q", method, got)
		}

		recorder = testenv.Do(handler, method, "/api/admin/users/"+missingUUID,
			map[string]any{"role": "guest"}, token)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s 查无此人状态码 = %d，想要 404（%s）", method, recorder.Code, recorder.Body.String())
		}
		if got := detail(t, recorder); got != "找不到该用户" {
			t.Errorf("%s 查无此人 detail = %q", method, got)
		}
	}

	// 删一条不存在的贴图 → 404（不是 400 / 500）
	recorder := testenv.Do(handler, http.MethodDelete, "/api/items/"+strings.Repeat("0", 64), nil, token)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("删不存在的贴图状态码 = %d，想要 404（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "找不到该贴图" {
		t.Errorf("删不存在的贴图 detail = %q", got)
	}
}

// 数据库挂掉时读贴图字节要回 500（而不是把「库里读不到」当成 404 骗客户端说没有）。
func TestAssetReturns500WhenDatabaseIsDown(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")
	assetID := firstItem(t, handler, token, "skin")["id"].(string)

	if err := instance.Store.Close(); err != nil {
		t.Fatalf("关库失败: %v", err)
	}
	recorder := testenv.Do(handler, http.MethodGet, "/asset/"+assetID, nil, "")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("库挂了之后取贴图状态码 = %d，想要 500（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "读取贴图失败" {
		t.Errorf("库挂了之后取贴图 detail = %q", got)
	}
}

// 处理器不经过 WithLanguage 中间件时也必须能跑：language() 兜底成源语言，
// 且不该凭空写出 Content-Language 头。
func TestHandlersWithoutLanguageMiddleware(t *testing.T) {
	instance := testenv.New(t)
	mux := api.NewMux(instance)

	recorder := testenv.Do(mux, http.MethodGet, "/api/library", nil, "")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("裸 mux 状态码 = %d，想要 401（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "未登录" {
		t.Errorf("裸 mux detail = %q，想要源语言文案 %q", got, "未登录")
	}
	if got := recorder.Header().Get("Content-Language"); got != "" {
		t.Errorf("没有中间件时不该有 Content-Language，实际 %q", got)
	}
}

// 「谁在穿这张贴图」：删除前给管理员看的名单（DESIGN.md 3.6）。
func TestAssetUsersEndpoint(t *testing.T) {
	instance, handler := newSite(t)
	adminToken := testenv.Token(t, instance, "admin")
	steveToken := testenv.Token(t, instance, "steve")
	guestToken := testenv.Token(t, instance, "guest")

	recorder := doUpload(t, handler, adminToken, [][2]string{{"kind", "skin.steve"}}, "skin.png", encodePNG(t, 64, 64))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("上传状态码 = %d（%s）", recorder.Code, recorder.Body.String())
	}
	assetID, _ := testenv.Decode(t, recorder)["id"].(string)

	// 没人用时是空名单，不是 404
	recorder = testenv.Do(handler, http.MethodGet, "/api/items/"+assetID+"/users", nil, adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("查名单状态码 = %d（%s）", recorder.Code, recorder.Body.String())
	}
	if users, _ := testenv.Decode(t, recorder)["users"].([]any); len(users) != 0 {
		t.Fatalf("还没人装备，名单该是空的: %v", users)
	}

	// steve 装备上之后，名单里应当有他，并且标出是 skin 那一列
	if recorder := testenv.Do(handler, http.MethodPost, "/api/me/equip",
		map[string]any{"asset_id": assetID}, steveToken); recorder.Code != http.StatusOK {
		t.Fatalf("装备状态码 = %d（%s）", recorder.Code, recorder.Body.String())
	}
	recorder = testenv.Do(handler, http.MethodGet, "/api/items/"+assetID+"/users", nil, adminToken)
	users, _ := testenv.Decode(t, recorder)["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("名单 = %v，想要 1 条", users)
	}
	first, _ := users[0].(map[string]any)
	if first["player_name"] != "steve" || first["slot"] != "skin" {
		t.Errorf("名单内容 = %v，想要 steve / skin", first)
	}

	// 名单属于管理面：guest 与普通用户都看不到
	for name, token := range map[string]string{"guest": guestToken, "steve": steveToken} {
		recorder := testenv.Do(handler, http.MethodGet, "/api/items/"+assetID+"/users", nil, token)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s 查名单 = %d，想要 403", name, recorder.Code)
		}
	}
	// 不存在的贴图 → 404（与删除一致）
	recorder = testenv.Do(handler, http.MethodGet,
		"/api/items/"+strings.Repeat("0", 64)+"/users", nil, adminToken)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("不存在的贴图 = %d，想要 404", recorder.Code)
	}

	// 真删了之后：使用者的装备被清空，名单也空了
	if recorder := testenv.Do(handler, http.MethodDelete, "/api/items/"+assetID, nil, adminToken); recorder.Code != http.StatusOK {
		t.Fatalf("删除状态码 = %d（%s）", recorder.Code, recorder.Body.String())
	}
	if skin, ok := testenv.Decode(t, testenv.Do(handler, http.MethodGet, "/api/me", nil, steveToken))["skin"]; ok && skin != nil {
		t.Errorf("删掉后 steve 的皮肤该是 null，实际 %v", skin)
	}
}
