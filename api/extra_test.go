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
		if got := detail(t, recorder); got != "UUID 必须是 32 位十六进制字符" {
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
