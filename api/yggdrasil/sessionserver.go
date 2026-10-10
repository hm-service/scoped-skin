package yggdrasil

import (
	"net/http"
	"strings"

	"scoped-skin/core"
	"scoped-skin/httpx"
)

// maxProfileLookup 是单次批量查询的角色数上限（规范：必须设上限，且至少为 2）。
const maxProfileLookup = 10

type joinRequest struct {
	AccessToken     string `json:"accessToken"`
	SelectedProfile string `json:"selectedProfile"`
	ServerID        string `json:"serverId"`
}

// 规范要求记下 serverId、accessToken 与发请求的客户端 IP，并设置过期时间
// （core.Joins 用的是 30 秒）。令牌有效且 selectedProfile 与令牌绑定的角色一致才算成功。
func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	payload, ok := decode[joinRequest](w, r)
	if !ok {
		return
	}
	session, ok := s.App.Sessions.Get(payload.AccessToken)
	if !ok ||
		core.DashlessUUID(session.PlayerUUID) != core.DashlessUUID(payload.SelectedProfile) {
		forbidden(w, errInvalidToken)
		return
	}
	s.App.Joins.Record(session.PlayerUUID, payload.ServerID, s.App.ClientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

// username 要与暗号所绑定的角色名一致；ip 是可选参数（Minecraft 服务端开了
// prevent-proxy-connections 才会带上），带上时会和 join 时记下的客户端 IP 比对。
// 核对不上一律回 204（规范：操作失败返回 204），不泄露是哪一步不对。
func (s *Server) handleHasJoined(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	username := strings.TrimSpace(query.Get("username"))
	serverID := query.Get("serverId")
	if username == "" || serverID == "" {
		badRequest(w, "Missing username or serverId")
		return
	}
	user := s.App.Store.UserByPlayerName(username)
	if user == nil || !s.App.Joins.Consume(user.UUID, serverID, query.Get("ip")) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	entry, err := s.fullProfile(r, user, true)
	if err != nil {
		internalError(w, err)
		return
	}
	httpx.Write(w, http.StatusOK, entry)
}

// unsigned 默认为 true（不带签名）；只有显式 unsigned=false 才签名。角色不存在回 204。
func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	user := s.App.Store.User(r.PathValue("uuid"))
	if user == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	entry, err := s.fullProfile(r, user, r.URL.Query().Get("unsigned") == "false")
	if err != nil {
		internalError(w, err)
		return
	}
	httpx.Write(w, http.StatusOK, entry)
}

// handleProfiles 是 POST /api/profiles/minecraft：按角色名批量查询（不返回角色属性）。
func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	names, ok := decode[[]string](w, r)
	if !ok {
		return
	}
	if len(names) > maxProfileLookup {
		badRequest(w, "Too many profiles requested")
		return
	}
	seen := map[string]bool{}
	found := []profile{}
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if user := s.App.Store.UserByPlayerName(name); user != nil {
			found = append(found, profileOf(user))
		}
	}
	httpx.Write(w, http.StatusOK, found)
}
