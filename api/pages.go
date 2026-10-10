package api

import (
	"bytes"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"time"

	"scoped-skin/web"
)

// DESIGN.md 3.1：让启动器从站点地址发现 Yggdrasil 服务。
const YggdrasilLocation = "/api/yggdrasil"

// servePage 把内嵌的 HTML 页面写出去：页面一律 no-store，免得改完前端还看旧的。
func servePage(w http.ResponseWriter, name string, headers map[string]string) {
	body, err := fs.ReadFile(web.Files, name)
	if err != nil {
		http.Error(w, "页面缺失", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	for key, value := range headers {
		w.Header().Set(key, value)
	}
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// handleIndex 是主页，顺带带上 ALI 响应头（API 地址指示）。
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	servePage(w, "index.html", map[string]string{
		"X-Authlib-Injector-API-Location": YggdrasilLocation,
	})
}

func (s *Server) handleAuthPage(w http.ResponseWriter, r *http.Request) {
	servePage(w, "auth.html", nil)
}

// handleMaintainPage 是管理员维护页；页面本身公开，真正的闸门在 /api/admin/*。
func (s *Server) handleMaintainPage(w http.ResponseWriter, r *http.Request) {
	servePage(w, "maintain.html", nil)
}

// handleStatic 分发内嵌的静态资源（DESIGN.md 3.7）。
//
// 不用 http.FileServerFS 就是为了不给目录列表：只认具体文件，目录（含 /static/ 本身）
// 一律 404，免得把内嵌文件的清单摊开给人看。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	if !fs.ValidPath(name) || name == "." {
		http.NotFound(w, r)
		return
	}
	info, err := fs.Stat(web.Files, name)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	body, err := fs.ReadFile(web.Files, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// ServeContent 按扩展名定 Content-Type，顺手处理 Range 与条件请求。
	http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(body))
}

// handleLivez 只要服务在运行就 200，不碰数据库等任何依赖；无响应体。
func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleReadyz 数据库可读可写才 200，否则 503（DESIGN.md 3.2）。
// 失败原因只进日志，不写响应体 —— 探针是公开的，没必要把内部错误摊出去。
//
// 配了 READYZ_TOKEN 就先核对 ?token=，不对回 401（同样无响应体）：探针对外是个稳定的
// 探测点，公网部署时给它一道门槛，免得谁都能拿它当靶子。
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !s.App.ReadyAuthorized(r.URL.Query().Get("token")) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if err := s.App.Ready(); err != nil {
		slog.ErrorContext(r.Context(), "就绪探针失败", slog.Any("error", err))
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleAsset 返回贴图字节：不带鉴权（<img> 没法带头），URL 本身就是内容的 sha256。
//
// 规范的安全警告：材质响应的 Content-Type 必须是 image/png，否则有 MIME 嗅探风险。
func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	data, err := s.App.Store.Texture(r.PathValue("asset_id"))
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.texture-read")
		return
	}
	if data == nil {
		fail(w, r, http.StatusNotFound, "store.texture-gone")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
