package api_test

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"scoped-skin/api"
	"scoped-skin/app"
	"scoped-skin/core"
	"scoped-skin/internal/testenv"
)

// 回归（最后一个管理员不可降级）：HTTP 层要把 core 的 valid.last-admin 翻成 400，
// 而且 guest 和 normal 一视同仁。修复前只挡了 normal，改成 guest 会 200。
func TestAdminCannotDemoteLastAdminOverHTTP(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")
	adminUUID := instance.Store.UserByUsername("admin").UUID

	for _, role := range []string{"guest", "normal"} {
		recorder := testenv.Do(handler, http.MethodPatch, "/api/admin/users/"+adminUUID,
			map[string]any{"role": role}, token)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("把最后一个管理员降成 %s 的状态码 = %d，想要 400（%s）",
				role, recorder.Code, recorder.Body.String())
		}
		if got := detail(t, recorder); got != "这是最后一个管理员，不能降级" {
			t.Errorf("降成 %s 的 detail = %q，想要 %q", role, got, "这是最后一个管理员，不能降级")
		}
	}
	if after := instance.Store.User(adminUUID); after == nil || after.Role != "admin" {
		t.Fatalf("被拒的降级却改动了库里状态: %+v", after)
	}

	// 有第二个管理员之后就能降级
	steveUUID := core.OfflineUUID("steve")
	if recorder := testenv.Do(handler, http.MethodPatch, "/api/admin/users/"+steveUUID,
		map[string]any{"role": "admin"}, token); recorder.Code != http.StatusOK {
		t.Fatalf("提拔第二个管理员状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	recorder := testenv.Do(handler, http.MethodPatch, "/api/admin/users/"+adminUUID,
		map[string]any{"role": "guest"}, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("还有别的管理员时降级状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	if role := object(t, testenv.Decode(t, recorder), "user")["role"]; role != "guest" {
		t.Errorf("降级后的 role = %v，想要 guest", role)
	}
}

// DESIGN.md 5.1：/api/admin/* 只给 admin；normal / guest 都要 403。
func TestAdminRoutesRejectNonAdmin(t *testing.T) {
	instance, handler := newSite(t)

	for _, account := range []string{"guest", "steve"} {
		token := testenv.Token(t, instance, account)
		for _, call := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, "/api/admin/users", nil},
			{http.MethodPost, "/api/admin/users", map[string]any{
				"username": "sneaky", "password": "sneaky123", "player_name": "sneaky",
			}},
			{http.MethodPatch, "/api/admin/users/" + core.OfflineUUID("steve"),
				map[string]any{"role": "guest"}},
			{http.MethodDelete, "/api/admin/users/" + core.OfflineUUID("steve"), nil},
		} {
			recorder := testenv.Do(handler, call.method, call.path, call.body, token)
			if recorder.Code != http.StatusForbidden {
				t.Errorf("%s %s（%s）状态码 = %d，想要 403（%s）",
					call.method, call.path, account, recorder.Code, recorder.Body.String())
			}
			if got := detail(t, recorder); got != "需要管理员身份" {
				t.Errorf("%s %s（%s）detail = %q", call.method, call.path, account, got)
			}
		}
	}
}

// 建号时的字段校验：撞唯一约束回 409（不是 400），非法字段回 400，且文案带字段名。
func TestAdminCreateUserConflictAndValidation(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")

	// 账号名与已有账号撞了 → 409
	recorder := testenv.Do(handler, http.MethodPost, "/api/admin/users", map[string]any{
		"username": "steve", "password": "steve123", "player_name": "steve2",
	}, token)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("重名账号状态码 = %d，想要 409（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "账号名「steve」已被占用" {
		t.Errorf("冲突 detail = %q", got)
	}

	// 游戏内名称非法 → 400
	recorder = testenv.Do(handler, http.MethodPost, "/api/admin/users", map[string]any{
		"username": "fresh", "password": "fresh123", "player_name": "Bad Name!",
	}, token)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("非法角色名状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); !strings.Contains(got, "游戏内名称") {
		t.Errorf("校验 detail = %q", got)
	}

	// role 显式给空串也算「提交了非法值」，而不是默默用默认身份组
	recorder = testenv.Do(handler, http.MethodPost, "/api/admin/users", map[string]any{
		"username": "fresh2", "password": "fresh123", "player_name": "fresh2", "role": "",
	}, token)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("空 role 状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
}

// 路由兜底：/api/* 的 404 是 JSON，其余路径是纯文本；方法不对回 405 且带 Allow；
// 这些响应上语言协商头也必须在。
func TestRoutingErrorsAndNegotiationHeaders(t *testing.T) {
	_, handler := newSite(t)

	// /api/ 下认不出的路径 → JSON 404
	recorder := testenv.Do(handler, http.MethodGet, "/api/does-not-exist", nil, "")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("未知 API 路径状态码 = %d，想要 404", recorder.Code)
	}
	if got := detail(t, recorder); got != "Not Found" {
		t.Errorf("未知 API 路径 detail = %q，想要 %q", got, "Not Found")
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("未知 API 路径 Content-Type = %q", got)
	}

	// 非 API 的未知路径 → 纯文本 404，不该是 JSON
	recorder = testenv.Do(handler, http.MethodGet, "/does-not-exist", nil, "")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("未知路径状态码 = %d，想要 404", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), `"detail"`) {
		t.Errorf("非 API 的 404 不该是 JSON 错误体: %q", recorder.Body.String())
	}

	// 站点路由上方法不对：405 + Allow（register 给每条路径补了不限方法的规则，
	// 否则会被底部那条 `/` 兜底吞成 404）。/api/* 下错误体仍是 JSON。
	recorder = testenv.Do(handler, http.MethodPost, "/api/library", nil, "")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/library 状态码 = %d，想要 405（%s）",
			recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Allow"); got != "GET" {
		t.Errorf("POST /api/library Allow = %q，想要 GET", got)
	}
	if got := detail(t, recorder); got != "Method Not Allowed" {
		t.Errorf("POST /api/library detail = %q，想要 %q", got, "Method Not Allowed")
	}

	// 非 API 路径上方法不对同样是 405，但错误体是纯文本
	recorder = testenv.Do(handler, http.MethodPost, "/livez", nil, "")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /livez 状态码 = %d，想要 405（%s）", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Allow"); got != "GET" {
		t.Errorf("POST /livez Allow = %q，想要 GET", got)
	}
	if strings.Contains(recorder.Body.String(), `"detail"`) {
		t.Errorf("非 API 的 405 不该是 JSON 错误体: %q", recorder.Body.String())
	}

	// Yggdrasil 那套也是 405（由它自己注册的「不限方法」规则给出）
	recorder = testenv.Do(handler, http.MethodGet, "/api/yggdrasil/authserver/authenticate", nil, "")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/yggdrasil/authserver/authenticate 状态码 = %d，想要 405（%s）",
			recorder.Code, recorder.Body.String())
	}

	// 语言协商头在错误响应上也在
	recorder = testenv.Do(handler, http.MethodGet, "/api/does-not-exist", nil, "")
	if got := recorder.Header().Get("Vary"); !strings.Contains(got, "Accept-Language") {
		t.Errorf("Vary = %q，应当带上 Accept-Language", got)
	}
	if got := recorder.Header().Get("Content-Language"); got != "zh-Hans" {
		t.Errorf("Content-Language = %q，想要 zh-Hans", got)
	}
	// 英文请求下 404 的 detail 也走同一套（404 文案目前两语言同形，这里只确认头）
	request := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	english := httptest.NewRecorder()
	handler.ServeHTTP(english, request)
	if got := english.Header().Get("Content-Language"); got != "en" {
		t.Errorf("英文请求 Content-Language = %q，想要 en", got)
	}
}

// 上传缺文件：不是 multipart、或 multipart 里没有 file 字段，都按「缺少上传文件」处理。
func TestUploadRejectsMissingFile(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")

	recorder := testenv.Do(handler, http.MethodPost, "/api/skins", map[string]any{"kind": "skin.steve"}, token)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("非 multipart 上传状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "缺少上传文件" {
		t.Errorf("非 multipart 上传 detail = %q", got)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("kind", "skin.steve"); err != nil {
		t.Fatalf("写表单失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关 multipart 失败: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/skins", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("没有 file 字段状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "缺少上传文件" {
		t.Errorf("没有 file 字段 detail = %q", got)
	}
}

// 请求体超过中间件的 1 MB 上限、又没有 Content-Length 时，multipart 解析会撞上
// MaxBytesReader，错误体要和中间件那道闸一致（不是「缺少上传文件」）。
func TestUploadOverBodyLimitIsRejected(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")

	recorder := doUpload(t, handler, token, [][2]string{{"kind", "skin.steve"}},
		"huge.bin", make([]byte, api.MaxRequestBytes+512*1024))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("超过请求体上限状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "请求体不能超过 1 MB" {
		t.Errorf("超过请求体上限 detail = %q，想要 %q", got, "请求体不能超过 1 MB")
	}
}

// /api/library 的查询参数边界：参数出现过但为空按非法处理；q 切成 token 回给前端。
func TestLibraryQueryParameterEdges(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")

	recorder := testenv.Do(handler, http.MethodGet, "/api/library?category=", nil, token)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("空 category 状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "category 只能是 skin 或 cape" {
		t.Errorf("空 category detail = %q", got)
	}

	recorder = testenv.Do(handler, http.MethodGet, "/api/library?category=skin&tab=weird", nil, token)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("非法 tab 状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "tab 只能是 all 或 favorites" {
		t.Errorf("非法 tab detail = %q", got)
	}

	// 空 q → tokens 是空数组（不是 null）
	// 搜索串长度上限：正常用碰不到，超了直接 400，不去拼 SQL
	recorder = testenv.Do(handler, http.MethodGet,
		"/api/library?category=skin&q="+url.QueryEscape(strings.Repeat("标", core.MaxQueryLength+1)), nil, token)
	if recorder.Code != http.StatusBadRequest || detail(t, recorder) != "搜索串最长 256 个字符" {
		t.Fatalf("超长搜索串 = %d（%s）", recorder.Code, recorder.Body.String())
	}

	recorder = testenv.Do(handler, http.MethodGet, "/api/library?category=skin&q=", nil, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("空 q 状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	payload := testenv.Decode(t, recorder)
	tokens, ok := payload["tokens"].([]any)
	if !ok || len(tokens) != 0 {
		t.Errorf("空 q 的 tokens = %v，想要空数组", payload["tokens"])
	}

	// 多个查询标签：去重 + 保留大小写，交给前端显示
	recorder = testenv.Do(handler, http.MethodGet,
		"/api/library?category=skin&q="+url.QueryEscape("官方  官方  Steve"), nil, token)
	payload = testenv.Decode(t, recorder)
	tokens, _ = payload["tokens"].([]any)
	if len(tokens) != 2 || tokens[0] != "官方" || tokens[1] != "Steve" {
		t.Errorf("tokens = %v，想要 [官方 Steve]", payload["tokens"])
	}

	// 找不到的贴图字节 → JSON 404
	recorder = testenv.Do(handler, http.MethodGet, "/asset/"+strings.Repeat("0", 64), nil, "")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("不存在的贴图状态码 = %d，想要 404（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "贴图不存在" {
		t.Errorf("不存在的贴图 detail = %q", got)
	}

	for _, path := range []string{"/auth", "/maintain"} {
		recorder = testenv.Do(handler, http.MethodGet, path, nil, "")
		if recorder.Code != http.StatusOK {
			t.Errorf("GET %s 状态码 = %d，想要 200", path, recorder.Code)
			continue
		}
		if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Errorf("GET %s Content-Type = %q", path, got)
		}
		if recorder.Body.Len() == 0 {
			t.Errorf("GET %s 回了空页面", path)
		}
	}
}

// DESIGN.md 5.1 的回归：guest 收紧成只读身份组 —— 任何会改持久化资源的写操作都要 403
// （文案键 auth.write），但读接口（列表 / 标签 / me / 预检 / 贴图字节）照常可用。
func TestGuestIsReadOnly(t *testing.T) {
	instance, handler := newSite(t)
	guestToken := testenv.Token(t, instance, "guest")
	normalToken := testenv.Token(t, instance, "steve")

	// 挑一条真实存在的贴图：写闸门必须排在资源查找前面，403 不能是 404 的副作用
	assetID := firstItem(t, handler, guestToken, "skin")["id"].(string)

	writes := []struct {
		name, method, path string
		body               any
	}{
		{"收藏", http.MethodPost, "/api/items/" + assetID + "/favorite", nil},
		{"换装", http.MethodPost, "/api/me/equip", map[string]any{"asset_id": assetID}},
		{"上传", http.MethodPost, "/api/skins", map[string]any{}},
	}
	for _, call := range writes {
		recorder := testenv.Do(handler, call.method, call.path, call.body, guestToken)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("guest %s 状态码 = %d，想要 403（%s）", call.name, recorder.Code, recorder.Body.String())
			continue
		}
		if got := detail(t, recorder); got != "当前身份组是只读的，不能修改任何资源" {
			t.Errorf("guest %s detail = %q，想要只读身份组文案", call.name, got)
		}
	}

	// 被拒的写操作不能留下任何痕迹：既没收藏，也没换装
	guest := instance.Store.UserByUsername("guest")
	item, err := instance.Store.Item(assetID, guest.UUID)
	if err != nil {
		t.Fatalf("查条目失败: %v", err)
	}
	if item == nil || item.Favorite {
		t.Errorf("guest 的收藏被写进去了: %+v", item)
	}
	if after := instance.Store.User(guest.UUID); after.Skin != "" || after.Cape != "" {
		t.Errorf("guest 的装备状态被写进去了: %+v", after)
	}

	// 只读接口对 guest 一律放行
	skinPNG := encodePNG(t, core.SkinSize, core.SkinSize)
	reads := []struct {
		name, method, path string
		body               any
	}{
		{"皮肤列表", http.MethodGet, "/api/library?category=skin", nil},
		{"披风列表", http.MethodGet, "/api/library?category=cape", nil},
		{"标签", http.MethodGet, "/api/tags", nil},
		{"账号", http.MethodGet, "/api/me", nil},
		{"贴图字节", http.MethodGet, "/asset/" + assetID, nil},
		{"预检", http.MethodPost, "/api/precheck", map[string]any{
			"kind": "skin.steve",
			"data": base64.StdEncoding.EncodeToString(skinPNG),
		}},
	}
	for _, call := range reads {
		recorder := testenv.Do(handler, call.method, call.path, call.body, guestToken)
		if recorder.Code != http.StatusOK {
			t.Errorf("guest %s 状态码 = %d，想要 200（%s）", call.name, recorder.Code, recorder.Body.String())
		}
	}

	// 删除仍走 admin 闸门：guest 403，但文案是 auth.admin 而不是只读身份组
	recorder := testenv.Do(handler, http.MethodDelete, "/api/items/"+assetID, nil, guestToken)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("guest 删除状态码 = %d，想要 403（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "需要管理员身份" {
		t.Errorf("guest 删除 detail = %q，想要 %q", got, "需要管理员身份")
	}

	// normal 身份组不受影响：收藏 / 换装都能成，证明 403 是冲 guest 来的而不是全线封死
	if recorder := testenv.Do(handler, http.MethodPost, "/api/items/"+assetID+"/favorite", nil, normalToken); recorder.Code != http.StatusOK {
		t.Fatalf("normal 收藏状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	if recorder := testenv.Do(handler, http.MethodPost, "/api/me/equip",
		map[string]any{"asset_id": assetID}, normalToken); recorder.Code != http.StatusOK {
		t.Fatalf("normal 换装状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
}

// /readyz 的 token 门槛（DESIGN.md 3.2）：配了 READYZ_TOKEN 就只认对得上的 ?token=，
// 其余一律 401 且无响应体；没配则维持公开。/livez 永远不需要 token。
func TestReadyzTokenGate(t *testing.T) {
	instance := testenv.NewWith(t, func(cfg *app.Config) { cfg.ReadyzToken = "s3cret-token" })
	handler := api.NewHandler(instance)

	for _, target := range []string{"/readyz", "/readyz?token=", "/readyz?token=wrong", "/readyz?token=s3cret-toke"} {
		recorder := testenv.Do(handler, http.MethodGet, target, nil, "")
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s 状态码 = %d，想要 401", target, recorder.Code)
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("%s 的 401 不该带响应体: %q", target, recorder.Body.String())
		}
	}

	if recorder := testenv.Do(handler, http.MethodGet, "/readyz?token=s3cret-token", nil, ""); recorder.Code != http.StatusOK {
		t.Errorf("带对 token 的状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	if recorder := testenv.Do(handler, http.MethodGet, "/livez", nil, ""); recorder.Code != http.StatusOK {
		t.Errorf("/livez 状态码 = %d，想要 200（存活探针不该被 token 挡住）", recorder.Code)
	}

	// 没配 READYZ_TOKEN：维持公开，带不带 token 都 200
	open := api.NewHandler(testenv.New(t))
	for _, target := range []string{"/readyz", "/readyz?token=whatever"} {
		if recorder := testenv.Do(open, http.MethodGet, target, nil, ""); recorder.Code != http.StatusOK {
			t.Errorf("未配 READYZ_TOKEN 时 %s 状态码 = %d，想要 200", target, recorder.Code)
		}
	}
}
