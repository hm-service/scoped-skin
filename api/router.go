// Package api 是站点的 HTTP 层：探针、/asset/* 的贴图字节、/api/* ，以及挂在其下的
// Yggdrasil（authlib-injector 兼容）接口。页面与静态资源不在这里 —— 那是站点外壳
// （根目录 web.go）的事，main.go 负责把两者接到一棵路由树上。
package api

import (
	"net/http"
	"slices"
	"strings"

	"scoped-skin/api/yggdrasil"
	"scoped-skin/app"
	"scoped-skin/httpx"
)

type Server struct {
	App *app.App
}

// NewHandler 是只含接口的完整处理器：中间件 + NewMux，供只关心接口的调用方使用
// （测试、或者将来换一个站点外壳）。页面与静态资源不在这里。
func NewHandler(instance *app.App) http.Handler {
	return Middleware(instance, NewMux(instance))
}

// Middleware 给任意处理器套上站点的通用中间件：访问日志 + 并发闸门 + 安全响应头 +
// 语言协商 + 请求体上限。并发闸门排在日志里头，所以访问日志里的耗时含排队时间；语言要排在
// 上限前头，那道闸回的错误文案也得跟着请求语言走。
//
// 站点外壳（根目录 web.go）与接口共用这一套，顺序只在这里定义一次。
func Middleware(instance *app.App, next http.Handler) http.Handler {
	return Logging(instance.ClientIP,
		Concurrency(SecurityHeaders(WithLanguage(LimitBody(next)))))
}

type route struct {
	method  string
	pattern string
	handler http.HandlerFunc
}

// register 按「METHOD 路径」注册，并给每条路径再补一条不限方法的规则，用来回 405。
//
// 为什么不靠 ServeMux 自己回 405：它只在**没有任何模式匹配**时才这么做，而底部那条 `/`
// 兜底匹配一切，于是方法不对会被兜成 404。给每条路径补一条不限方法的规则后，路径匹配、
// 方法不匹配就落到这里，带上 Allow 回 405（/api/* 下仍是 JSON 错误体）。
func register(mux *http.ServeMux, routes []route) {
	allowed := make(map[string][]string, len(routes))
	for _, item := range routes {
		allowed[item.pattern] = append(allowed[item.pattern], item.method)
		mux.HandleFunc(item.method+" "+item.pattern, item.handler)
	}
	for pattern, methods := range allowed {
		value := allowValue(methods)
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", value)
			if strings.HasPrefix(r.URL.Path, "/api/") {
				httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
				return
			}
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		})
	}
}

// allowValue 拼 Allow 头。Go 的 "GET" 模式同时也接 HEAD，所以 GET 要连 HEAD 一起写出来
// —— 站点外壳（web.go）与 Yggdrasil 的 405 用的是同一条规则。
func allowValue(methods []string) string {
	if slices.Contains(methods, http.MethodGet) && !slices.Contains(methods, http.MethodHead) {
		methods = append(methods, http.MethodHead)
	}
	return strings.Join(methods, ", ")
}

// NewMux 挂载 /api/*、Yggdrasil、探针与贴图字节。Go 1.22 起 ServeMux 支持方法与通配符，
// 所以这里按「METHOD /path」声明，比在 handler 里自己判方法清楚。
//
// 页面与静态资源不在这里：那些是站点外壳的事（根目录 web.go 把构建产物发出去），
// 由 main.go 按前缀把两者接起来。
func NewMux(instance *app.App) *http.ServeMux {
	server := &Server{App: instance}
	mux := http.NewServeMux()

	register(mux, []route{
		{http.MethodGet, "/livez", server.handleLivez},
		{http.MethodGet, "/readyz", server.handleReadyz},

		{http.MethodGet, "/asset/{asset_id}", server.handleAsset},

		{http.MethodPost, "/api/auth/login", server.handleLogin},
		{http.MethodGet, "/api/auth/session", server.handleSession},
		{http.MethodGet, "/api/tags", server.handleTags},
		{http.MethodGet, "/api/library", server.handleLibrary},
		{http.MethodPost, "/api/precheck", server.handlePrecheck},
		{http.MethodPost, "/api/skins", server.handleUploadSkin},
		{http.MethodPost, "/api/items/{asset_id}/favorite", server.handleFavorite},
		{http.MethodGet, "/api/items/{asset_id}/users", server.handleAssetUsers},
		{http.MethodDelete, "/api/items/{asset_id}", server.handleDeleteItem},
		{http.MethodGet, "/api/me", server.handleMe},
		{http.MethodPost, "/api/me/equip", server.handleEquip},
		{http.MethodGet, "/api/admin/users", server.handleListUsers},
		{http.MethodPost, "/api/admin/users", server.handleCreateUser},
		{http.MethodPatch, "/api/admin/users/{uuid}", server.handleUpdateUser},
		{http.MethodDelete, "/api/admin/users/{uuid}", server.handleDeleteUser},
	})

	yggdrasil.New(instance).Register(mux)

	mux.HandleFunc("/", server.handleNotFound)
	return mux
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	http.NotFound(w, r)
}
