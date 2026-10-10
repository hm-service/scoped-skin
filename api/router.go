// Package api 是站点的 HTTP 层：页面、静态资源、探针、/api/* ，以及挂在其下的
// Yggdrasil（authlib-injector 兼容）接口。
package api

import (
	"net/http"
	"strings"

	"scoped-skin/api/yggdrasil"
	"scoped-skin/app"
	"scoped-skin/httpx"
)

type Server struct {
	App *app.App
}

// NewHandler 返回带中间件的完整处理器：访问日志 + 并发闸门 + 安全响应头 + 语言协商 +
// 请求体上限。并发闸门排在日志里头，所以访问日志里的耗时含排队时间；语言要排在上限前头，
// 那道闸回的错误文案也得跟着请求语言走。
func NewHandler(instance *app.App) http.Handler {
	return Logging(instance.ClientIP,
		Concurrency(SecurityHeaders(WithLanguage(LimitBody(NewMux(instance))))))
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
		value := strings.Join(methods, ", ")
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

// NewMux 挂载全部路由。Go 1.22 起 ServeMux 支持方法与通配符，所以这里按
// 「METHOD /path」声明，比在 handler 里自己判方法清楚。
func NewMux(instance *app.App) *http.ServeMux {
	server := &Server{App: instance}
	mux := http.NewServeMux()

	register(mux, []route{
		{http.MethodGet, "/{$}", server.handleIndex},
		{http.MethodGet, "/auth", server.handleAuthPage},
		{http.MethodGet, "/maintain", server.handleMaintainPage},
		{http.MethodGet, "/static/{path...}", server.handleStatic},

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
