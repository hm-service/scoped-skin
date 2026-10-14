package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"scoped-skin/api"
	"scoped-skin/api/yggdrasil"
	"scoped-skin/internal/testenv"
)

// testSite 是一份最小的构建产物：三个页面、一个带内容哈希的资源、一个固定名字的图标，
// 外加一个空目录（目录列表必须给不出来）。
func testSite() fs.FS {
	return fstest.MapFS{
		"index.html":            {Data: []byte("<!doctype html><title>皮肤库</title>")},
		"auth.html":             {Data: []byte("<!doctype html><title>登录</title>")},
		"maintain.html":         {Data: []byte("<!doctype html><title>维护</title>")},
		"favicon.svg":           {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)},
		"assets/app-abc123.js":  {Data: []byte("console.log(1)")},
		"assets/app-abc123.css": {Data: []byte("body{}")},
		"assets/nested":         {Mode: fs.ModeDir},
	}
}

// 页面按 URL 表分发：/ → index.html，/auth → auth.html，/maintain → maintain.html。
// 首页带 authlib-injector 的发现头，三个页面都 no-store。
func TestSitePages(t *testing.T) {
	handler := siteHandler(testSite())

	cases := []struct {
		path    string
		file    string
		aliPath string // 首页才有 X-Authlib-Injector-API-Location
	}{
		{"/", "皮肤库", yggdrasil.AliasPrefix},
		{"/auth", "登录", ""},
		{"/maintain", "维护", ""},
	}
	for i, item := range cases {
		t.Run(fmt.Sprintf("%d_%s", i, item.path), func(t *testing.T) {
			recorder := testenv.Do(handler, http.MethodGet, item.path, nil, "")
			if recorder.Code != http.StatusOK {
				t.Fatalf("%s 状态码 = %d，想要 200", item.path, recorder.Code)
			}
			if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
				t.Errorf("%s Content-Type = %q", item.path, got)
			}
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("%s Cache-Control = %q，想要 no-store", item.path, got)
			}
			if !strings.Contains(recorder.Body.String(), item.file) {
				t.Errorf("%s 响应体 = %q，不含 %q", item.path, recorder.Body.String(), item.file)
			}
			if got := recorder.Header().Get("X-Authlib-Injector-API-Location"); got != item.aliPath {
				t.Errorf("%s X-Authlib-Injector-API-Location = %q，想要 %q", item.path, got, item.aliPath)
			}
		})
	}
}

// 产物里的其余文件按原样发：带哈希的 /assets/* 一年 immutable，固定名字的每次回源确认。
func TestSiteServesBuiltFiles(t *testing.T) {
	handler := siteHandler(testSite())

	script := testenv.Do(handler, http.MethodGet, "/assets/app-abc123.js", nil, "")
	if script.Code != http.StatusOK {
		t.Fatalf("/assets/app-abc123.js 状态码 = %d", script.Code)
	}
	if got := script.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Errorf("Content-Type = %q", got)
	}
	if got := script.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("/assets/* Cache-Control = %q", got)
	}

	style := testenv.Do(handler, http.MethodGet, "/assets/app-abc123.css", nil, "")
	if got := style.Header().Get("Content-Type"); !strings.Contains(got, "text/css") {
		t.Errorf("CSS Content-Type = %q", got)
	}

	icon := testenv.Do(handler, http.MethodGet, "/favicon.svg", nil, "")
	if icon.Code != http.StatusOK {
		t.Fatalf("/favicon.svg 状态码 = %d", icon.Code)
	}
	if got := icon.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("/favicon.svg Cache-Control = %q，想要 no-cache", got)
	}

	// HEAD 走 GET 的处理器，但不回体
	head := testenv.Do(handler, http.MethodHead, "/assets/app-abc123.js", nil, "")
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Errorf("HEAD /assets/app-abc123.js = %d，体长 %d", head.Code, head.Body.Len())
	}
}

// 只认具体文件：目录不给列表，产物里的 *.html 不能绕过页面表直接访问，穿越也读不出东西。
func TestSiteHasNoDirectoryListing(t *testing.T) {
	handler := siteHandler(testSite())

	for i, path := range []string{"/assets/", "/assets/nested", "/assets/nested/", "/assets/nope.js",
		"/index.html", "/auth.html", "/maintain.html", "/nope", "/nope/deep.js"} {
		t.Run(fmt.Sprintf("%d_%s", i, path), func(t *testing.T) {
			recorder := testenv.Do(handler, http.MethodGet, path, nil, "")
			if recorder.Code != http.StatusNotFound {
				t.Errorf("%s 状态码 = %d，想要 404", path, recorder.Code)
			}
			if strings.Contains(recorder.Body.String(), "app-abc123") {
				t.Errorf("%s 回了个目录列表: %s", path, recorder.Body.String())
			}
		})
	}

	// 路径穿越：ServeMux 先按清理后的路径回 301，跟过去也只会落到普通路径；这里只要求
	// 「不是带着内容 200」，不会从内嵌文件系统里读出别的东西。
	recorder := testenv.Do(handler, http.MethodGet, "/assets/../../etc/passwd", nil, "")
	if recorder.Code == http.StatusOK && recorder.Body.Len() > 0 {
		t.Fatalf("路径穿越回了个 200：%s", recorder.Body.String())
	}
}

// 方法不对：405 + Allow（站点外壳直接判，不走 ServeMux 的默认 405）。
func TestSiteMethodNotAllowed(t *testing.T) {
	handler := siteHandler(testSite())

	for i, path := range []string{"/", "/auth", "/assets/app-abc123.js", "/favicon.svg"} {
		t.Run(fmt.Sprintf("%d_%s", i, path), func(t *testing.T) {
			recorder := testenv.Do(handler, http.MethodPost, path, nil, "")
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Errorf("POST %s 状态码 = %d，想要 405", path, recorder.Code)
			}
			if got := recorder.Header().Get("Allow"); got != allowMethods {
				t.Errorf("POST %s Allow = %q，想要 %q", path, got, allowMethods)
			}
		})
	}
}

// 产物里少了页面说明这个二进制本身是坏的：500，不假装 404。
func TestSiteMissingPageIsServerError(t *testing.T) {
	handler := siteHandler(fstest.MapFS{"favicon.svg": {Data: []byte("<svg/>")}})

	recorder := testenv.Do(handler, http.MethodGet, "/", nil, "")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("页面缺失时状态码 = %d，想要 500", recorder.Code)
	}
}

// defaultLimits 是正常启动时的槽位数；要验闸门本身时才另外造一份。
func defaultLimits() concurrencyLimits {
	return concurrencyLimits{
		auth:   api.DefaultAuthConcurrency,
		api:    api.DefaultAPIConcurrency,
		static: api.DefaultStaticConcurrency,
	}
}

// 静态那半给 0 个槽位时页面一直排队，其余路由组各有各的槽位、照常应答。
func TestRootHandlerGivesEachRouteGroupItsOwnGate(t *testing.T) {
	instance := testenv.New(t)
	handler := newRootHandler(instance, testSite(), concurrencyLimits{auth: 1, api: 1, static: 0})

	page := make(chan struct{})
	go func() {
		defer close(page)
		testenv.Do(handler, http.MethodGet, "/", nil, "")
	}()
	select {
	case <-page:
		t.Fatal("静态那半是 0 个槽位，却把页面放过去了")
	case <-time.After(200 * time.Millisecond):
	}

	for name, item := range map[string]struct {
		target string
		want   int
	}{
		"存活探针不设闸门":   {"/livez", http.StatusOK},
		"就绪探针有自己的槽位": {"/readyz", http.StatusOK},
		"认证那半有自己的槽位": {"/api/auth/session", http.StatusUnauthorized},
	} {
		t.Run(name, func(t *testing.T) {
			if recorder := testenv.Do(handler, http.MethodGet, item.target, nil, ""); recorder.Code != item.want {
				t.Fatalf("%s = %d，想要 %d", item.target, recorder.Code, item.want)
			}
		})
	}
}

// 认证与业务各一组：业务那半给 0 个槽位时它一直排队，认证那半照常应答。
func TestRootHandlerKeepsAuthGateSeparate(t *testing.T) {
	instance := testenv.New(t)
	handler := newRootHandler(instance, testSite(), concurrencyLimits{auth: 1, api: 0, static: 1})

	blocked := make(chan struct{})
	go func() {
		defer close(blocked)
		testenv.Do(handler, http.MethodGet, "/api/library", nil, "")
	}()
	select {
	case <-blocked:
		t.Fatal("业务那半是 0 个槽位，却把请求放过去了")
	case <-time.After(200 * time.Millisecond):
	}

	if recorder := testenv.Do(handler, http.MethodGet, "/api/auth/session", nil, ""); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("/api/auth/session = %d，想要 401（认证那半不该被业务闸门挡住）", recorder.Code)
	}
}

// 接线：页面归站点外壳、/api/* 与探针归 api 包，共用中间件套在最外层。
func TestRootHandlerWiresSiteAndApi(t *testing.T) {
	instance := testenv.New(t)
	handler := newRootHandler(instance, testSite(), defaultLimits())

	page := testenv.Do(handler, http.MethodGet, "/", nil, "")
	if page.Code != http.StatusOK {
		t.Fatalf("主页状态码 = %d", page.Code)
	}
	if got := page.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("主页缺安全头: X-Content-Type-Options = %q", got)
	}
	if got := page.Header().Get("X-Authlib-Injector-API-Location"); got != yggdrasil.AliasPrefix {
		t.Errorf("主页发现头 = %q，想要 %q", got, yggdrasil.AliasPrefix)
	}

	// 未登录的接口：401 + JSON 错误体（说明确实交给了 api 包）
	unauthorized := testenv.Do(handler, http.MethodGet, "/api/tags", nil, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("/api/tags 状态码 = %d，想要 401（%s）", unauthorized.Code, unauthorized.Body.String())
	}
	if !strings.Contains(unauthorized.Body.String(), `"detail"`) {
		t.Errorf("/api/tags 的错误体不是 JSON: %s", unauthorized.Body.String())
	}

	// 探针与未知路径各归各家
	if recorder := testenv.Do(handler, http.MethodGet, "/readyz", nil, ""); recorder.Code != http.StatusOK {
		t.Errorf("/readyz 状态码 = %d，想要 200", recorder.Code)
	}
	if recorder := testenv.Do(handler, http.MethodGet, "/nope", nil, ""); recorder.Code != http.StatusNotFound {
		t.Errorf("/nope 状态码 = %d，想要 404", recorder.Code)
	}
}

// 访问日志在接线后的行为：页面照记，/assets/* 与 /assets/* 不记（与中间件的跳过规则一致）。
func TestRootHandlerAccessLog(t *testing.T) {
	instance := testenv.New(t)
	handler := newRootHandler(instance, testSite(), defaultLimits())

	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	for _, path := range []string{"/", "/auth", "/assets/app-abc123.js", "/livez"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	logged := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(buffer.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("日志不是一行 JSON: %q", line)
		}
		if record["msg"] == "HTTP Request" {
			if path, ok := record["path"].(string); ok {
				logged[path] = true
			}
		}
	}
	for i, path := range []string{"/", "/auth"} {
		t.Run(fmt.Sprintf("logged_%d_%s", i, path), func(t *testing.T) {
			if !logged[path] {
				t.Errorf("%s 应当写访问日志", path)
			}
		})
	}
	for i, path := range []string{"/assets/app-abc123.js", "/livez"} {
		t.Run(fmt.Sprintf("silent_%d_%s", i, path), func(t *testing.T) {
			if logged[path] {
				t.Errorf("%s 不该写访问日志", path)
			}
		})
	}
}
