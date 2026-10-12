package yggdrasil

import (
	"net"
	"net/http"
	"slices"
	"strings"

	"scoped-skin/httpx"
)

// AliasPrefix 是挂在站点根上的 Yggdrasil 前缀（DESIGN.md 3.1 的发现头指向它）。
// 站点外壳（根目录 web.go）给首页发 X-Authlib-Injector-API-Location 时也用它。
const AliasPrefix = "/api/yggdrasil"

// 每个端点除了「METHOD /path」还额外注册一条同路径、不限方法的规则：Go 1.22 起
// ServeMux 会挑更具体的那条，于是方法不匹配时能按规范回 JSON 的 405，而不是
// ServeMux 默认的纯文本。
func (s *Server) Register(mux *http.ServeMux) {
	handle := func(method, path string, handler http.HandlerFunc) {
		mux.HandleFunc(method+" "+AliasPrefix+path, handler)
		mux.HandleFunc(AliasPrefix+path, methodNotAllowed(method))
	}

	// 根接口：元数据。带不带结尾斜杠都认。
	mux.HandleFunc("GET "+AliasPrefix, s.handleMeta)
	mux.HandleFunc("GET "+AliasPrefix+"/{$}", s.handleMeta)
	mux.HandleFunc(AliasPrefix, methodNotAllowed(http.MethodGet))
	mux.HandleFunc(AliasPrefix+"/{$}", methodNotAllowed(http.MethodGet))

	handle(http.MethodPost, "/authserver/authenticate", s.handleAuthenticate)
	handle(http.MethodPost, "/authserver/refresh", s.handleRefresh)
	handle(http.MethodPost, "/authserver/validate", s.handleValidate)
	handle(http.MethodPost, "/authserver/invalidate", s.handleInvalidate)
	handle(http.MethodPost, "/authserver/signout", s.handleSignout)

	handle(http.MethodPost, "/sessionserver/session/minecraft/join", s.handleJoin)
	handle(http.MethodGet, "/sessionserver/session/minecraft/hasJoined", s.handleHasJoined)
	handle(http.MethodGet, "/sessionserver/session/minecraft/profile/{uuid}", s.handleProfile)

	handle(http.MethodPost, "/api/profiles/minecraft", s.handleProfiles)

	// 前缀下认不出的路径：按规范回 JSON 404
	mux.HandleFunc(AliasPrefix+"/", notFound)
}

// handleMeta 是规范的「API 元数据获取」，也就是启动器最先拉的那份元数据。
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	httpx.Write(w, http.StatusOK, map[string]any{
		"meta": map[string]any{
			"serverName":            "Scoped Skin",
			"implementationName":    "scoped-skin",
			"implementationVersion": s.App.Config.Version,
			// 本站用的是账号名（不是邮箱）登录，据此告诉启动器可以填非邮箱凭证（DESIGN.md 3.6）
			"feature.non_email_login": true,
		},
		"skinDomains":        skinDomains(r),
		"signaturePublickey": s.App.Keys.PublicPEM(),
	})
}

// 规范：不以点开头的规则要求域名完全相同，含 * 的规则按通配匹配；白名单里没有的域名
// 会让客户端报 "non-whitelisted domain"。贴图是和站点同源分发的，所以把请求用到的
// 主机名（含带端口写法，不同实现的比较口径不一样）连同本机名一起放进去。
func skinDomains(r *http.Request) []string {
	host := r.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
		if host == "" {
			host = "127.0.0.1"
		}
	}
	domains := []string{}
	for _, candidate := range []string{host, r.Host, "127.0.0.1", "localhost"} {
		if candidate == "" || slices.Contains(domains, candidate) {
			continue
		}
		domains = append(domains, candidate)
	}
	return domains
}

// assetURL 拼贴图的绝对地址；客户端会直接来拉，所以必须是绝对 URL。
func assetURL(r *http.Request, assetID string) string {
	return scheme(r) + "://" + r.Host + "/asset/" + assetID
}

// scheme 认 TLS 连接与反向代理给的 X-Forwarded-Proto（DESIGN.md：公网部署需套 TLS 反代）。
func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return "http"
}
