// Package api_test 从外部视角打整条 HTTP 链路（真实处理器 + 独立临时实例）。
package api_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"scoped-skin/api"
	"scoped-skin/app"
	"scoped-skin/core"
	"scoped-skin/internal/testenv"
)

func newSite(t *testing.T) (*app.App, http.Handler) {
	t.Helper()
	instance := testenv.New(t)
	return instance, api.NewHandler(instance)
}

// captureLogs 把默认 slog 输出接到内存里，返回读日志的函数（每条一行 JSON）。
func captureLogs(t *testing.T) func() []map[string]any {
	t.Helper()

	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return func() []map[string]any {
		records := []map[string]any{}
		for _, line := range strings.Split(strings.TrimSpace(buffer.String()), "\n") {
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err == nil {
				records = append(records, record)
			}
		}
		return records
	}
}

func loggedIP(t *testing.T, records []map[string]any, path string) string {
	t.Helper()
	for index := len(records) - 1; index >= 0; index-- {
		if records[index]["msg"] == "HTTP Request" && records[index]["path"] == path {
			ip, _ := records[index]["ip"].(string)
			return ip
		}
	}
	t.Fatalf("日志里没有 %s 的记录: %v", path, records)
	return ""
}

// doUpload 发一次 multipart 的 POST /api/skins；fields 允许重复键（tags 就会出现多次）。
func doUpload(t *testing.T, handler http.Handler, token string, fields [][2]string, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("构造 multipart 失败: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("写入 multipart 失败: %v", err)
	}
	for _, field := range fields {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			t.Fatalf("写入表单字段失败: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 multipart 失败: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/skins", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// encodePNG 现场编码一张 width×height 的 PNG（DESIGN.md 的最小角色皮肤是 64×64）。
func encodePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 5), B: 0x40, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatalf("编码 PNG 失败: %v", err)
	}
	return buffer.Bytes()
}

// oversizedPNG 造一个头部合法（64×64）、但体积超过上限的伪 PNG，用来验证分块读上限。
func oversizedPNG() []byte {
	blob := []byte{
		0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', // PNG 魔数
		0, 0, 0, 13, 'I', 'H', 'D', 'R', // IHDR 长度与类型
		0, 0, 0, 64, // 宽度
		0, 0, 0, 64, // 高度
	}
	return append(blob, make([]byte, core.MaxUploadBytes+1024)...)
}

func digestHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// detail 取响应体里的 detail 字段（前端就是读这个来显示错误的）。
func detail(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	value, _ := testenv.Decode(t, recorder)["detail"].(string)
	return value
}

func object(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	child, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("响应里没有对象字段 %q: %v", key, parent)
	}
	return child
}

func firstItem(t *testing.T, handler http.Handler, token, category string) map[string]any {
	t.Helper()
	recorder := testenv.Do(handler, http.MethodGet, "/api/library?category="+category, nil, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/library 状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	items, ok := testenv.Decode(t, recorder)["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("library 里没有条目: %s", recorder.Body.String())
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("条目不是 JSON 对象: %v", items[0])
	}
	return item
}

func builtinItem(t *testing.T, handler http.Handler, token, category string) map[string]any {
	t.Helper()
	recorder := testenv.Do(handler, http.MethodGet, "/api/library?category="+category, nil, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/library 状态码 = %d，想要 200", recorder.Code)
	}
	items, _ := testenv.Decode(t, recorder)["items"].([]any)
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok && item["readonly"] == true {
			return item
		}
	}
	t.Fatalf("库里没有只读的内置资源: %s", recorder.Body.String())
	return nil
}

func TestLogin(t *testing.T) {
	t.Run("正确凭据返回可用 token", func(t *testing.T) {
		_, handler := newSite(t)
		recorder := testenv.Do(handler, http.MethodPost, "/api/auth/login",
			map[string]any{"username": "admin", "password": "admin123"}, "")
		if recorder.Code != http.StatusOK {
			t.Fatalf("登录状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
		}
		payload := testenv.Decode(t, recorder)
		token, _ := payload["token"].(string)
		if token == "" {
			t.Fatalf("登录没有返回 token: %s", recorder.Body.String())
		}
		player := object(t, payload, "player")
		if player["username"] != "admin" {
			t.Errorf("player.username = %v，想要 admin", player["username"])
		}
		if _, leaked := player["password_hash"]; leaked {
			t.Error("响应里泄露了 password_hash")
		}

		session := testenv.Do(handler, http.MethodGet, "/api/auth/session", nil, token)
		if session.Code != http.StatusOK {
			t.Fatalf("/api/auth/session 状态码 = %d，想要 200（%s）", session.Code, session.Body.String())
		}
		if object(t, testenv.Decode(t, session), "player")["username"] != "admin" {
			t.Errorf("session 里的账号不对: %s", session.Body.String())
		}
	})

	t.Run("密码错误文案精确", func(t *testing.T) {
		_, handler := newSite(t)
		recorder := testenv.Do(handler, http.MethodPost, "/api/auth/login",
			map[string]any{"username": "admin", "password": "not-the-password"}, "")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("状态码 = %d，想要 401", recorder.Code)
		}
		if got := detail(t, recorder); got != "用户名或密码不正确" {
			t.Errorf("detail = %q，想要 %q", got, "用户名或密码不正确")
		}
	})

	// DESIGN.md 5.3：应用不做限流（连按 IP 的也没有），这活交给反向代理；连打多次也不该出 429。
	t.Run("不限流", func(t *testing.T) {
		_, handler := newSite(t)
		for attempt := 1; attempt <= 6; attempt++ {
			recorder := testenv.Do(handler, http.MethodPost, "/api/auth/login",
				map[string]any{"username": "admin", "password": "wrong"}, "")
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("第 %d 次登录状态码 = %d，想要 401（%s）",
					attempt, recorder.Code, recorder.Body.String())
			}
		}
	})
}

func TestAuthGate(t *testing.T) {
	instance, handler := newSite(t)
	adminToken := testenv.Token(t, instance, "admin")
	guestToken := testenv.Token(t, instance, "guest")

	t.Run("没有 token", func(t *testing.T) {
		recorder := testenv.Do(handler, http.MethodGet, "/api/auth/session", nil, "")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("状态码 = %d，想要 401", recorder.Code)
		}
		if got := detail(t, recorder); got != "未登录" {
			t.Errorf("detail = %q，想要 %q", got, "未登录")
		}
		if recorder.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("WWW-Authenticate = %q，想要 Bearer", recorder.Header().Get("WWW-Authenticate"))
		}
	})

	t.Run("token 不认识", func(t *testing.T) {
		recorder := testenv.Do(handler, http.MethodGet, "/api/auth/session", nil, strings.Repeat("0", 64))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("状态码 = %d，想要 401", recorder.Code)
		}
		if got := detail(t, recorder); got != "登录状态无效，请重新登录" {
			t.Errorf("detail = %q，想要 %q", got, "登录状态无效，请重新登录")
		}
	})

	t.Run("guest 不能上传", func(t *testing.T) {
		recorder := testenv.Do(handler, http.MethodPost, "/api/skins", map[string]any{}, guestToken)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("状态码 = %d，想要 403（%s）", recorder.Code, recorder.Body.String())
		}
		if got := detail(t, recorder); got != "当前身份组是只读的，不能修改任何资源" {
			t.Errorf("detail = %q，想要 %q", got, "当前身份组是只读的，不能修改任何资源")
		}
	})

	t.Run("guest 不能删图", func(t *testing.T) {
		recorder := testenv.Do(handler, http.MethodDelete,
			"/api/items/"+strings.Repeat("a", 64), nil, guestToken)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("状态码 = %d，想要 403（%s）", recorder.Code, recorder.Body.String())
		}
		if got := detail(t, recorder); got != "需要管理员身份" {
			t.Errorf("detail = %q，想要 %q", got, "需要管理员身份")
		}
	})

	t.Run("admin 不能删内置资源", func(t *testing.T) {
		item := builtinItem(t, handler, adminToken, "skin")
		recorder := testenv.Do(handler, http.MethodDelete, "/api/items/"+item["id"].(string), nil, adminToken)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
		}
		if got := detail(t, recorder); got != "只读资源不能删除" {
			t.Errorf("detail = %q，想要 %q", got, "只读资源不能删除")
		}
	})
}

func TestUploadSkin(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")

	data := encodePNG(t, 64, 64)

	// 首次上传：201 + created
	recorder := doUpload(t, handler, token, [][2]string{
		{"kind", "skin.steve"},
		{"tags", "冒烟"},
		{"tags", "64x64"},
	}, "skin.png", data)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("上传状态码 = %d，想要 201（%s）", recorder.Code, recorder.Body.String())
	}
	payload := testenv.Decode(t, recorder)
	if payload["created"] != true || payload["updated"] != false {
		t.Errorf("created/updated = %v/%v，想要 true/false", payload["created"], payload["updated"])
	}
	// 内容寻址：URL 末段就是所服务字节的 sha256（id 由服务端按入库字节算）
	assetID, _ := payload["id"].(string)
	if len(assetID) != 64 {
		t.Fatalf("id = %q，想要 64 位 hex", assetID)
	}
	served := testenv.Do(handler, http.MethodGet, "/asset/"+assetID, nil, "")
	if got := digestHex(served.Body.Bytes()); got != assetID {
		t.Errorf("/asset/%s 的字节 sha256 = %s", assetID, got)
	}

	// 同一份内容改标签与 kind：只覆盖那一条，updated
	recorder = doUpload(t, handler, token, [][2]string{
		{"kind", "skin.alex"},
		{"tags", "改过的标签"},
	}, "skin-again.png", data)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("二次上传状态码 = %d，想要 201（%s）", recorder.Code, recorder.Body.String())
	}
	payload = testenv.Decode(t, recorder)
	if payload["created"] != false || payload["updated"] != true {
		t.Errorf("created/updated = %v/%v，想要 false/true", payload["created"], payload["updated"])
	}
	if payload["kind"] != "skin.alex" || payload["category"] != "skin" {
		t.Errorf("kind/category = %v/%v，想要 skin.alex/skin", payload["kind"], payload["category"])
	}
	tags, _ := payload["tags"].([]any)
	if len(tags) != 1 || tags[0] != "改过的标签" {
		t.Errorf("tags = %v，想要 [改过的标签]", payload["tags"])
	}

	// 不是 PNG
	recorder = doUpload(t, handler, token, [][2]string{{"kind", "skin.steve"}},
		"bad.png", []byte("这显然不是一张 PNG 图片"))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("非 PNG 状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "不是有效的 PNG 图片" {
		t.Errorf("detail = %q", got)
	}

	// 头合法但体积超过上限：分块读到上限立刻 400
	recorder = doUpload(t, handler, token, [][2]string{{"kind", "skin.steve"}},
		"huge.png", oversizedPNG())
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("超大图片状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "图片不能超过 32 KB" {
		t.Errorf("detail = %q，想要 %q", got, "图片不能超过 32 KB")
	}
}

// DESIGN.md 3.6：预检把原图（base64）一起带上来，由服务端自己校验并自己算 asset_id，
// 再把这个 ID 回给前端 —— 入库字节是重编码过的，前端算不出它。
func TestPrecheckValidatesPayload(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")

	data := encodePNG(t, 64, 64)
	precheck := func(body any) *httptest.ResponseRecorder {
		t.Helper()
		return testenv.Do(handler, http.MethodPost, "/api/precheck", body, token)
	}
	payloadFor := func(kind string, raw []byte) map[string]any {
		return map[string]any{"kind": kind, "data": base64.StdEncoding.EncodeToString(raw)}
	}

	// 没上传过：200 + exists=false，并回服务端算的 asset_id
	recorder := precheck(payloadFor("skin.steve", data))
	if recorder.Code != http.StatusOK {
		t.Fatalf("预检状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	payload := testenv.Decode(t, recorder)
	assetID, _ := payload["asset_id"].(string)
	if payload["exists"] != false || len(assetID) != 64 {
		t.Fatalf("exists/asset_id = %v/%q，想要 false/64 位 hex", payload["exists"], assetID)
	}

	// 上传之后同内容再预检：exists=true，并把库里那条带回来；两边的 ID 必须一致
	recorder = doUpload(t, handler, token, [][2]string{{"kind", "skin.steve"}}, "skin.png", data)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("上传状态码 = %d，想要 201（%s）", recorder.Code, recorder.Body.String())
	}
	if uploaded := testenv.Decode(t, recorder); uploaded["id"] != assetID {
		t.Errorf("上传得到的 id = %v，预检给的是 %s，两边必须一致", uploaded["id"], assetID)
	}
	recorder = precheck(payloadFor("skin.steve", data))
	payload = testenv.Decode(t, recorder)
	if payload["exists"] != true {
		t.Fatalf("上传后预检 exists = %v，想要 true", payload["exists"])
	}
	if item := object(t, payload, "item"); item["id"] != assetID {
		t.Errorf("预检带回来的条目 id = %v，想要 %s", item["id"], assetID)
	}

	// 尺寸不对：服务端自己按规则拒（HD 皮肤）
	hd := encodePNG(t, 128, 128)
	recorder = precheck(payloadFor("skin.steve", hd))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("HD 皮肤预检状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "皮肤必须是 64×64" {
		t.Errorf("detail = %q，想要 %q", got, "皮肤必须是 64×64")
	}

	// 拿皮肤去当披风：按披风的规则拒
	recorder = precheck(payloadFor("cape", data))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("皮肤当披风状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); !strings.Contains(got, "披风必须是 64×32") {
		t.Errorf("detail = %q", got)
	}

	// 披风 64×32 是合法的
	cape := encodePNG(t, 64, 32)
	if recorder := precheck(payloadFor("cape", cape)); recorder.Code != http.StatusOK {
		t.Errorf("披风预检状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}

	// 不是 PNG / 头合法但数据坏了 / base64 坏了 / kind 不对 / 内容为空
	cases := []struct {
		name string
		body any
		want string
	}{
		{"不是 PNG", payloadFor("skin.steve", []byte("junk")), "不是有效的 PNG 图片"},
		{"头合法但数据坏了", payloadFor("skin.steve", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13, 'I', 'H', 'D', 'R', 0, 0, 0, 64, 0, 0, 0, 64}), "PNG 数据已损坏，无法解码"},
		{"base64 坏了", map[string]any{"kind": "skin.steve", "data": "这显然不是 base64"}, "图片数据不是合法的 base64"},
		{"kind 不对", payloadFor("hat", data), "kind 只能是 skin.steve / skin.alex / cape"},
		{"内容为空", payloadFor("skin.steve", nil), "不是有效的 PNG 图片"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := precheck(testCase.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
			}
			if got := detail(t, recorder); got != testCase.want {
				t.Errorf("detail = %q，想要 %q", got, testCase.want)
			}
		})
	}

	// 没登录不允许预检
	recorder = testenv.Do(handler, http.MethodPost, "/api/precheck", payloadFor("skin.steve", data), "")
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("未登录预检状态码 = %d，想要 401", recorder.Code)
	}
}

func TestFavoriteToggle(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")
	assetID := firstItem(t, handler, token, "skin")["id"].(string)

	favorite := func() (bool, float64) {
		t.Helper()
		recorder := testenv.Do(handler, http.MethodPost, "/api/items/"+assetID+"/favorite", nil, token)
		if recorder.Code != http.StatusOK {
			t.Fatalf("收藏状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
		}
		payload := testenv.Decode(t, recorder)
		stats := object(t, payload, "stats")
		count, _ := stats["favorites"].(float64)
		return payload["favorite"] == true, count
	}

	if on, count := favorite(); !on || count != 1 {
		t.Errorf("第一次切换后 favorite=%v favorites=%v，想要 true/1", on, count)
	}
	if on, count := favorite(); on || count != 0 {
		t.Errorf("第二次切换后 favorite=%v favorites=%v，想要 false/0", on, count)
	}

	// 不存在的贴图 → 404
	recorder := testenv.Do(handler, http.MethodPost,
		"/api/items/"+strings.Repeat("f", 64)+"/favorite", nil, token)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("收藏不存在贴图状态码 = %d，想要 404", recorder.Code)
	}
	if got := detail(t, recorder); got != "找不到该贴图" {
		t.Errorf("detail = %q，想要 %q", got, "找不到该贴图")
	}
}

func TestEquip(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")
	assetID := firstItem(t, handler, token, "skin")["id"].(string)

	recorder := testenv.Do(handler, http.MethodPost, "/api/me/equip",
		map[string]any{"asset_id": assetID}, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("装备状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}

	recorder = testenv.Do(handler, http.MethodGet, "/api/me", nil, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("/api/me 状态码 = %d，想要 200", recorder.Code)
	}
	payload := testenv.Decode(t, recorder)
	if object(t, payload, "skin")["id"] != assetID {
		t.Errorf("装备后的 skin = %v，想要 %s", payload["skin"], assetID)
	}
	if payload["cape"] != nil {
		t.Errorf("没装备披风时 cape = %v，想要 null", payload["cape"])
	}

	recorder = testenv.Do(handler, http.MethodPost, "/api/me/equip",
		map[string]any{"asset_id": strings.Repeat("f", 64)}, token)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("装备不存在贴图状态码 = %d，想要 404（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "找不到该贴图" {
		t.Errorf("detail = %q，想要 %q", got, "找不到该贴图")
	}
}

func TestAdminUsers(t *testing.T) {
	instance, handler := newSite(t)
	adminToken := testenv.Token(t, instance, "admin")
	adminUUID := instance.Store.UserByUsername("admin").UUID

	// 新建 → 201，列表里能看到
	recorder := testenv.Do(handler, http.MethodPost, "/api/admin/users", map[string]any{
		"username":    "tester",
		"password":    "tester123",
		"player_name": "tester",
		"role":        "normal",
	}, adminToken)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("创建用户状态码 = %d，想要 201（%s）", recorder.Code, recorder.Body.String())
	}
	payload := testenv.Decode(t, recorder)
	created := object(t, payload, "user")
	uuid, _ := created["uuid"].(string)
	if uuid == "" || created["username"] != "tester" || created["role"] != "normal" {
		t.Fatalf("创建出来的用户不对: %v", created)
	}
	if payload["me"] != adminUUID {
		t.Errorf("me = %v，想要 %s", payload["me"], adminUUID)
	}

	list := testenv.Do(handler, http.MethodGet, "/api/admin/users", nil, adminToken)
	if list.Code != http.StatusOK {
		t.Fatalf("用户列表状态码 = %d，想要 200", list.Code)
	}
	found := false
	for _, raw := range testenv.Decode(t, list)["users"].([]any) {
		if entry, ok := raw.(map[string]any); ok && entry["username"] == "tester" {
			found = true
		}
	}
	if !found {
		t.Fatalf("新建的用户没出现在列表里: %s", list.Body.String())
	}

	// 改账号名：旧 token 立刻 401，新账号名算出来的 token 可用
	oldToken := testenv.Token(t, instance, "tester")
	recorder = testenv.Do(handler, http.MethodPatch, "/api/admin/users/"+uuid,
		map[string]any{"username": "tester2"}, adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("改用户状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	if object(t, testenv.Decode(t, recorder), "user")["username"] != "tester2" {
		t.Errorf("改完的账号名不对: %s", recorder.Body.String())
	}
	recorder = testenv.Do(handler, http.MethodGet, "/api/auth/session", nil, oldToken)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("旧 token 状态码 = %d，想要 401", recorder.Code)
	}
	newToken := testenv.Token(t, instance, "tester2")
	if recorder = testenv.Do(handler, http.MethodGet, "/api/auth/session", nil, newToken); recorder.Code != http.StatusOK {
		t.Fatalf("新 token 状态码 = %d，想要 200", recorder.Code)
	}

	// 最后一个管理员不能删
	recorder = testenv.Do(handler, http.MethodDelete, "/api/admin/users/"+adminUUID, nil, adminToken)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("删最后管理员状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "这是最后一个管理员，不能删除" {
		t.Errorf("detail = %q，想要 %q", got, "这是最后一个管理员，不能删除")
	}

	// 删普通账号 → 200，它的 token 立刻 401
	recorder = testenv.Do(handler, http.MethodDelete, "/api/admin/users/"+uuid, nil, adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("删用户状态码 = %d，想要 200（%s）", recorder.Code, recorder.Body.String())
	}
	if testenv.Decode(t, recorder)["deleted"] != uuid {
		t.Errorf("deleted = %v，想要 %s", testenv.Decode(t, recorder)["deleted"], uuid)
	}
	if recorder = testenv.Do(handler, http.MethodGet, "/api/auth/session", nil, newToken); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("被删用户的 token 状态码 = %d，想要 401", recorder.Code)
	}
}

func TestPagesAndProbes(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")

	index := testenv.Do(handler, http.MethodGet, "/", nil, "")
	if index.Code != http.StatusOK {
		t.Fatalf("主页状态码 = %d，想要 200", index.Code)
	}
	if got := index.Header().Get("X-Authlib-Injector-API-Location"); got != api.YggdrasilLocation {
		t.Errorf("X-Authlib-Injector-API-Location = %q，想要 %q", got, api.YggdrasilLocation)
	}

	if recorder := testenv.Do(handler, http.MethodGet, "/livez", nil, ""); recorder.Code != http.StatusOK {
		t.Errorf("/livez 状态码 = %d，想要 200", recorder.Code)
	}
	if recorder := testenv.Do(handler, http.MethodGet, "/readyz", nil, ""); recorder.Code != http.StatusOK {
		t.Errorf("/readyz 状态码 = %d，想要 200", recorder.Code)
	}

	assetID := firstItem(t, handler, token, "skin")["id"].(string)
	asset := testenv.Do(handler, http.MethodGet, "/asset/"+assetID, nil, "")
	if asset.Code != http.StatusOK {
		t.Fatalf("/asset 状态码 = %d，想要 200", asset.Code)
	}
	if got := asset.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q，想要 image/png", got)
	}
	if got := asset.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", got)
	}
	if asset.Body.Len() == 0 {
		t.Error("/asset 返回了空内容")
	}
}

func TestRequestSizeLimit(t *testing.T) {
	_, handler := newSite(t)

	request := httptest.NewRequest(http.MethodPost, "/api/skins", strings.NewReader("x"))
	request.Header.Set("Content-Length", strconv.Itoa(api.MaxRequestBytes+1))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "请求体不能超过 1 MB" {
		t.Errorf("detail = %q，想要 %q", got, "请求体不能超过 1 MB")
	}
}

// DESIGN.md 2.4：站点 API 的用户可见文案按请求的 Accept-Language 走，认不出来就用简体中文。
func TestLanguageNegotiation(t *testing.T) {
	instance := testenv.New(t)
	handler := api.NewHandler(instance)
	token := testenv.Token(t, instance, "admin")

	// 自定义头的请求（testenv.Do 固定不带 Accept-Language）
	call := func(method, target, accept string, body any, bearer string) *httptest.ResponseRecorder {
		t.Helper()
		var reader io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("序列化失败: %v", err)
			}
			reader = bytes.NewReader(encoded)
		}
		request := httptest.NewRequest(method, target, reader)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if accept != "" {
			request.Header.Set("Accept-Language", accept)
		}
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	// 默认（不带头）= 源语言：简体中文
	recorder := call(http.MethodGet, "/api/library?category=hat", "", nil, token)
	if recorder.Code != http.StatusBadRequest || detail(t, recorder) != "category 只能是 skin 或 cape" {
		t.Fatalf("默认语言 = %d %q", recorder.Code, detail(t, recorder))
	}
	if got := recorder.Header().Get("Content-Language"); got != "zh-Hans" {
		t.Errorf("Content-Language = %q", got)
	}
	if got := recorder.Header().Get("Vary"); !strings.Contains(got, "Accept-Language") {
		t.Errorf("Vary = %q，应当带上 Accept-Language", got)
	}

	// 英文
	recorder = call(http.MethodGet, "/api/library?category=hat", "en-US,en;q=0.9", nil, token)
	if recorder.Code != http.StatusBadRequest || detail(t, recorder) != "category must be skin or cape" {
		t.Fatalf("英文 = %d %q", recorder.Code, detail(t, recorder))
	}
	if got := recorder.Header().Get("Content-Language"); got != "en" {
		t.Errorf("Content-Language = %q", got)
	}

	// 401 的文案也跟着语言走
	recorder = call(http.MethodGet, "/api/library", "en", nil, "")
	if recorder.Code != http.StatusUnauthorized || detail(t, recorder) != "Not signed in" {
		t.Errorf("未登录（en）= %d %q", recorder.Code, detail(t, recorder))
	}

	// 账号字段校验：core 抛的是文案键，参数里还嵌着「字段名」这个键，两层都要翻译
	steveUUID := core.OfflineUUID("steve")
	recorder = call(http.MethodPatch, "/api/admin/users/"+steveUUID, "en",
		map[string]any{"player_name": "Bad!"}, token)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("非法游戏内名称 = %d（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "Player name takes lowercase letters, digits and underscore, 1–16 characters" {
		t.Errorf("en 校验文案 = %q", got)
	}
	recorder = call(http.MethodPatch, "/api/admin/users/"+steveUUID, "zh-Hans",
		map[string]any{"player_name": "Bad!"}, token)
	if got := detail(t, recorder); got != "游戏内名称只能是小写字母、数字或下划线，长度 1-16" {
		t.Errorf("zh 校验文案 = %q", got)
	}

	// 撞唯一约束 → 409，字段名也要翻译
	recorder = call(http.MethodPatch, "/api/admin/users/"+steveUUID, "en",
		map[string]any{"username": "alex"}, token)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("重名账号 = %d（%s）", recorder.Code, recorder.Body.String())
	}
	if got := detail(t, recorder); got != "Account name “alex” is taken" {
		t.Errorf("en 冲突文案 = %q", got)
	}
	recorder = call(http.MethodPatch, "/api/admin/users/"+steveUUID, "zh-Hans",
		map[string]any{"username": "alex"}, token)
	if got := detail(t, recorder); got != "账号名「alex」已被占用" {
		t.Errorf("zh 冲突文案 = %q", got)
	}
}

// ── 公网部署相关的加固点（见 DESIGN.md 第 5 节） ────────────────────────────────

// 三个静态安全头要在所有响应上都出现（CSS/JS、JSON、图片一视同仁）。
func TestSecurityHeaders(t *testing.T) {
	_, handler := newSite(t)

	for _, path := range []string{"/", "/livez", "/static/js/app.js"} {
		recorder := testenv.Do(handler, http.MethodGet, path, nil, "")
		header := recorder.Header()
		if header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q", path, header.Get("X-Content-Type-Options"))
		}
		if header.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: Referrer-Policy = %q", path, header.Get("Referrer-Policy"))
		}
		if header.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: X-Frame-Options = %q", path, header.Get("X-Frame-Options"))
		}
	}
}

// 静态资源只认具体文件：目录不给列表（不然内嵌文件的清单就摊开了）。
func TestStaticFilesHaveNoDirectoryListing(t *testing.T) {
	_, handler := newSite(t)

	recorder := testenv.Do(handler, http.MethodGet, "/static/js/app.js", nil, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("取具体文件状态码 = %d", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "javascript") {
		t.Errorf("Content-Type = %q", contentType)
	}

	for _, path := range []string{"/static/", "/static/js/", "/static/js", "/static/nope.js"} {
		recorder := testenv.Do(handler, http.MethodGet, path, nil, "")
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s 状态码 = %d，想要 404", path, recorder.Code)
		}
		if strings.Contains(recorder.Body.String(), "app.js") {
			t.Errorf("%s 回了个目录列表: %s", path, recorder.Body.String())
		}
	}

	// 路径穿越：ServeMux 先按清理后的路径回 307，跟过去也只会落到普通页面；这里只要求
	// 「不是带着内容 200」，不会从内嵌文件系统里读出别的东西。
	recorder = testenv.Do(handler, http.MethodGet, "/static/../../etc/passwd", nil, "")
	if recorder.Code == http.StatusOK && recorder.Body.Len() > 0 {
		t.Fatalf("路径穿越回了个 200：%s", recorder.Body.String())
	}
}

// 探针无响应体；数据库不可用时 /readyz 回 503，失败原因只进日志。
func TestProbes(t *testing.T) {
	instance, handler := newSite(t)

	for _, path := range []string{"/livez", "/readyz"} {
		recorder := testenv.Do(handler, http.MethodGet, path, nil, "")
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s 状态码 = %d", path, recorder.Code)
		}
		if body := recorder.Body.String(); body != "" {
			t.Errorf("%s 响应体 = %q，想要空", path, body)
		}
	}

	if err := instance.Store.Close(); err != nil {
		t.Fatalf("关库失败: %v", err)
	}
	recorder := testenv.Do(handler, http.MethodGet, "/readyz", nil, "")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("库挂了之后 /readyz 状态码 = %d，想要 503", recorder.Code)
	}
	if body := recorder.Body.String(); body != "" {
		t.Errorf("503 响应体 = %q，想要空", body)
	}
}

// DESIGN.md 5.5：只有对端在 TRUSTED_PROXIES 里时才认反代头，访问日志里的 IP 跟着变。
// 这里用 /auth 而不是 /livez —— 后者与静态资源刻意不写访问日志（DESIGN.md 1.3）。
func TestClientIPFromTrustedProxy(t *testing.T) {
	instance := testenv.NewWith(t, func(cfg *app.Config) { cfg.TrustedProxies = "192.0.2.1/32" })
	handler := api.NewHandler(instance)

	read := captureLogs(t)
	request := httptest.NewRequest(http.MethodGet, "/auth", nil)
	request.RemoteAddr = "192.0.2.1:54321" // httptest 的默认对端，正好被标成可信
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if ip := loggedIP(t, read(), "/auth"); ip != "198.51.100.7" {
		t.Fatalf("可信代理下日志里的 ip = %q，想要 198.51.100.7", ip)
	}

	// 对端不可信（默认配置）时，伪造的 XFF 一律不算数
	_, plain := newSite(t)
	read = captureLogs(t)
	request = httptest.NewRequest(http.MethodGet, "/auth", nil)
	request.RemoteAddr = "203.0.113.9:54321"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	plain.ServeHTTP(httptest.NewRecorder(), request)

	if ip := loggedIP(t, read(), "/auth"); ip != "203.0.113.9" {
		t.Fatalf("不可信对端下日志里的 ip = %q，想要 203.0.113.9", ip)
	}
}

// DESIGN.md 4.2：标签数量与单条长度都有上限，文案里的数字直接跟着常量走。
func TestTagLimits(t *testing.T) {
	instance, handler := newSite(t)
	token := testenv.Token(t, instance, "admin")
	png := encodePNG(t, core.SkinSize, core.SkinSize)

	fields := [][2]string{{"kind", "skin.steve"}}
	for index := range core.MaxTagsPerAsset + 1 {
		fields = append(fields, [2]string{"tags", "标签" + strconv.Itoa(index)})
	}
	recorder := doUpload(t, handler, token, fields, "too-many.png", png)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("超过标签数上限的状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got, want := detail(t, recorder), "标签最多 "+strconv.Itoa(core.MaxTagsPerAsset)+" 个"; got != want {
		t.Errorf("detail = %q，想要 %q", got, want)
	}

	recorder = doUpload(t, handler, token,
		[][2]string{{"kind", "skin.steve"}, {"tags", strings.Repeat("长", core.MaxTagLength+1)}},
		"tag-too-long.png", png)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("超长标签状态码 = %d，想要 400（%s）", recorder.Code, recorder.Body.String())
	}
	if got, want := detail(t, recorder), "单个标签最长 "+strconv.Itoa(core.MaxTagLength)+" 个字符"; got != want {
		t.Errorf("detail = %q，想要 %q", got, want)
	}

	// 正好顶到两个上限应当能过
	fields = [][2]string{{"kind", "skin.steve"}, {"tags", strings.Repeat("长", core.MaxTagLength)}}
	for index := 1; index < core.MaxTagsPerAsset; index++ {
		fields = append(fields, [2]string{"tags", "标签" + strconv.Itoa(index)})
	}
	recorder = doUpload(t, handler, token, fields, "at-limit.png", png)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("正好到上限的状态码 = %d，想要 201（%s）", recorder.Code, recorder.Body.String())
	}
}

// DESIGN.md 5.4：上传与删除贴图要留审计，带上操作者 uuid。
func TestAssetChangesAreAudited(t *testing.T) {
	instance, handler := newSite(t)
	admin := instance.Store.UserByUsername("admin")
	token := testenv.Token(t, instance, "admin")
	read := captureLogs(t)

	recorder := doUpload(t, handler, token, [][2]string{{"kind", "skin.steve"}, {"tags", "审计"}},
		"audited.png", encodePNG(t, core.SkinSize, core.SkinSize))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("上传状态码 = %d（%s）", recorder.Code, recorder.Body.String())
	}
	var uploaded struct {
		ID   string `json:"id"`
		Size int    `json:"size"`
		Kind string `json:"kind"`
	}
	testenv.DecodeInto(t, recorder, &uploaded)
	if uploaded.Kind != "skin.steve" {
		t.Errorf("上传响应的 kind = %q，想要 skin.steve", uploaded.Kind)
	}

	recorder = testenv.Do(handler, http.MethodDelete, "/api/items/"+uploaded.ID, nil, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("删除状态码 = %d（%s）", recorder.Code, recorder.Body.String())
	}

	events := map[string]map[string]any{}
	for _, record := range read() {
		if name, ok := record["event"].(string); ok {
			events[name] = record
		}
	}
	upload, ok := events["asset.upload"]
	if !ok {
		t.Fatalf("没有 asset.upload 审计记录: %v", events)
	}
	if upload["actor"] != admin.UUID || upload["asset_id"] != uploaded.ID ||
		upload["kind"] != "skin.steve" || upload["result"] != "created" {
		t.Errorf("asset.upload 内容不对: %v", upload)
	}
	removed, ok := events["asset.delete"]
	if !ok {
		t.Fatalf("没有 asset.delete 审计记录: %v", events)
	}
	if removed["actor"] != admin.UUID || removed["asset_id"] != uploaded.ID {
		t.Errorf("asset.delete 内容不对: %v", removed)
	}
}
