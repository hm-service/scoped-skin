package api

import (
	"log/slog"
	"net/http"
)

// handleLivez 只要服务在运行就 200，不碰数据库等任何依赖；无响应体。
func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleReadyz 数据库可读可写才 200，否则 503。
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
		slog.ErrorContext(r.Context(), "readiness probe failed", slog.Any("error", err))
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}
