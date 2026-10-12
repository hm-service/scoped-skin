package api

import (
	"net/http"
)

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
