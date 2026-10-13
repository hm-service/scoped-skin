package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"scoped-skin/core"
	"scoped-skin/db"
	"scoped-skin/httpx"
)

// userView 是站点 API 回给前端的账号信息：uuid / role /
// player_name / username / skin / cape —— 绝不带 password_hash。
type userView struct {
	UUID       string `json:"uuid"`
	Role       string `json:"role"`
	PlayerName string `json:"player_name"`
	Username   string `json:"username"`
	Skin       string `json:"skin"`
	Cape       string `json:"cape"`
}

// publicUser 把库里的 user 行裁成给前端的字段，密码哈希在这里被丢掉。
func publicUser(user *db.User) userView {
	return userView{
		UUID:       user.UUID,
		Role:       user.Role,
		PlayerName: user.PlayerName,
		Username:   user.Username,
		Skin:       user.Skin,
		Cape:       user.Cape,
	}
}

// decodeJSON 解析 JSON 请求体；失败时写 400 并返回 false。
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		fail(w, r, http.StatusBadRequest, "request.json")
		return false
	}
	return true
}

// currentUser 把 `Authorization: Bearer <token>` 换成库里的 user 行。
//
// 头缺失 / 不是 Bearer / 没有 token → 401「未登录」；token 不认识，或认识的
// token 对应的账号已经不在库里 → 401「登录状态无效，请重新登录」。
func (s *Server) currentUser(w http.ResponseWriter, r *http.Request) *db.User {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "bearer") || token == "" {
		unauthorized(w, r, "auth.required")
		return nil
	}
	user := s.App.Store.User(s.App.Tokens.Resolve(strings.TrimSpace(token)))
	if user == nil {
		unauthorized(w, r, "auth.stale")
		return nil
	}
	return user
}

// currentAdmin 是身份组闸门：里管理用户 / 删除资源只有 role = admin 能做。
func (s *Server) currentAdmin(w http.ResponseWriter, r *http.Request) *db.User {
	user := s.currentUser(w, r)
	if user == nil {
		return nil
	}
	if user.Role != "admin" {
		fail(w, r, http.StatusForbidden, "auth.admin")
		return nil
	}
	return user
}

// currentWriter 是写闸门：里 guest 是只读身份组 —— 上传、收藏、换装这些
// 会改持久化资源的操作都要求 admin / normal。
func (s *Server) currentWriter(w http.ResponseWriter, r *http.Request) *db.User {
	user := s.currentUser(w, r)
	if user == nil {
		return nil
	}
	if user.Role != "admin" && user.Role != "normal" {
		fail(w, r, http.StatusForbidden, "auth.write")
		return nil
	}
	return user
}

// handleLogin 校验账号密码，按固定算法算出 token 返回（与启动时同一套规则）。
//
// 不做限流：按 IP 的限流交给反向代理，应用侧只保证 Argon2id 的
// 并发上限与恒定耗时。每次尝试都写一条审计记录，成功时额外记下 uuid。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}

	ip := s.App.ClientIP(r)
	user := s.App.VerifyCredentials(payload.Username, payload.Password)
	if user == nil {
		s.App.Audit.Write("login", ip, map[string]any{
			"username": payload.Username,
			"result":   "denied",
		})
		unauthorized(w, r, "auth.login.failed")
		return
	}

	s.App.Audit.Write("login", ip, map[string]any{
		"username": user.Username,
		"result":   "ok",
		"uuid":     user.UUID,
	})

	token := s.App.Tokens.Register(core.Credential{
		UUID: user.UUID, Username: user.Username, PasswordHash: user.PasswordHash,
	})
	httpx.Write(w, http.StatusOK, map[string]any{
		"token":  token,
		"player": publicUser(user),
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	user := s.currentUser(w, r)
	if user == nil {
		return
	}
	httpx.Write(w, http.StatusOK, map[string]any{"player": publicUser(user)})
}
