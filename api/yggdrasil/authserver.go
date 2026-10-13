package yggdrasil

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"scoped-skin/httpx"
)

// equalToken 用定长摘要恒时比较：直接比字符串会把长度与「前几位对了」漏成时间差。
func equalToken(left, right string) bool {
	leftDigest := sha256.Sum256([]byte(left))
	rightDigest := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftDigest[:], rightDigest[:]) == 1
}

// clientTokenMatches 报告请求带的 clientToken 是否对得上；空表示这次不校验（规范允许）。
func clientTokenMatches(stored, presented string) bool {
	return presented == "" || equalToken(stored, presented)
}

// 规范的请求体。没有列进来的字段（agent 等）按规范不需要校验，直接忽略。

type authenticateRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	ClientToken string `json:"clientToken"`
	RequestUser bool   `json:"requestUser"`
}

type refreshRequest struct {
	AccessToken     string   `json:"accessToken"`
	ClientToken     string   `json:"clientToken"`
	RequestUser     bool     `json:"requestUser"`
	SelectedProfile *profile `json:"selectedProfile"`
}

type tokenRequest struct {
	AccessToken string `json:"accessToken"`
	ClientToken string `json:"clientToken"`
}

type signoutRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionResponse struct {
	AccessToken       string    `json:"accessToken"`
	ClientToken       string    `json:"clientToken"`
	AvailableProfiles []profile `json:"availableProfiles"`
	SelectedProfile   *profile  `json:"selectedProfile,omitempty"`
	User              *userInfo `json:"user,omitempty"`
}

// refreshResponse 是 refresh 的响应：规范里没有 availableProfiles 这一项。
type refreshResponse struct {
	AccessToken     string    `json:"accessToken"`
	ClientToken     string    `json:"clientToken"`
	SelectedProfile *profile  `json:"selectedProfile,omitempty"`
	User            *userInfo `json:"user,omitempty"`
}

// 防爆破不做限流，靠的是恒定请求时间：无论账号存不存在，这次校验都至少
// 花掉 1 秒才返回，于是单个客户端的请求频率天然被压住。限流交给反向代理。
// 这是和站点登录同一道密码闸门，成功与失败都要留审计。
func (s *Server) handleAuthenticate(w http.ResponseWriter, r *http.Request) {
	payload, ok := decode[authenticateRequest](w, r)
	if !ok {
		return
	}
	if tooLong(payload.ClientToken) {
		badRequest(w, "Invalid clientToken")
		return
	}
	ip := s.App.ClientIP(r)
	user := s.App.VerifyCredentials(payload.Username, payload.Password)
	if user == nil {
		s.App.Audit.Write("login", ip, map[string]any{
			"username": payload.Username,
			"result":   "denied",
		})
		forbidden(w, errInvalidCredentials)
		return
	}
	s.App.Audit.Write("login", ip, map[string]any{
		"username": user.Username,
		"result":   "ok",
		"uuid":     user.UUID,
	})

	accessToken, clientToken, err := s.App.Sessions.Create(user.UUID, payload.ClientToken)
	if err != nil {
		internalError(w, err)
		return
	}
	// 每个账号只有一角色，所以令牌总是直接绑上它（规范：仅有一个角色时通常绑定到该角色）。
	entry := profileOf(user)
	response := sessionResponse{
		AccessToken:       accessToken,
		ClientToken:       clientToken,
		AvailableProfiles: []profile{entry},
		SelectedProfile:   &entry,
	}
	if payload.RequestUser {
		response.User = userInfoOf(user)
	}
	httpx.Write(w, http.StatusOK, response)
}

// 请求失败时原令牌依然有效，所以这里先发新令牌、再吊销旧的。
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	payload, ok := decode[refreshRequest](w, r)
	if !ok {
		return
	}
	if tooLong(payload.AccessToken, payload.ClientToken) {
		forbidden(w, errInvalidToken)
		return
	}
	session, ok := s.App.Sessions.Get(payload.AccessToken)
	if !ok || !clientTokenMatches(session.ClientToken, payload.ClientToken) {
		forbidden(w, errInvalidToken)
		return
	}
	// 规范：向一个已经绑定了角色的令牌指定其要绑定的角色 → 400 IllegalArgumentException。
	// 本站每个账号只有一个角色，令牌永远是绑好的，所以这条路径必然是这个错。
	if payload.SelectedProfile != nil {
		sendError(w, http.StatusBadRequest, "IllegalArgumentException",
			"Access token already has a profile assigned.")
		return
	}
	user := s.App.Store.User(session.PlayerUUID)
	if user == nil {
		forbidden(w, errInvalidToken)
		return
	}

	accessToken, clientToken, err := s.App.Sessions.Create(user.UUID, session.ClientToken)
	if err != nil {
		internalError(w, err)
		return
	}
	s.App.Sessions.Delete(payload.AccessToken)

	entry := profileOf(user)
	response := refreshResponse{
		AccessToken:     accessToken,
		ClientToken:     clientToken,
		SelectedProfile: &entry,
	}
	if payload.RequestUser {
		response.User = userInfoOf(user)
	}
	httpx.Write(w, http.StatusOK, response)
}

// handleValidate 是 POST /authserver/validate：有效回 204，否则按令牌无效处理。
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	payload, ok := decode[tokenRequest](w, r)
	if !ok {
		return
	}
	if tooLong(payload.AccessToken, payload.ClientToken) {
		forbidden(w, errInvalidToken)
		return
	}
	session, ok := s.App.Sessions.Get(payload.AccessToken)
	if !ok || !clientTokenMatches(session.ClientToken, payload.ClientToken) {
		forbidden(w, errInvalidToken)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// 规范：服务端只需要检查 accessToken，clientToken 不影响结果；无论操作是否成功都回 204。
func (s *Server) handleInvalidate(w http.ResponseWriter, r *http.Request) {
	payload, ok := decode[tokenRequest](w, r)
	if !ok {
		return
	}
	s.App.Sessions.Delete(payload.AccessToken)
	w.WriteHeader(http.StatusNoContent)
}

// 规范的安全提示：该 API 同样可用于判断密码的正确性，所以和 authenticate 一样走恒定耗时的
// 校验入口，也一样要留审计。
func (s *Server) handleSignout(w http.ResponseWriter, r *http.Request) {
	payload, ok := decode[signoutRequest](w, r)
	if !ok {
		return
	}
	ip := s.App.ClientIP(r)
	user := s.App.VerifyCredentials(payload.Username, payload.Password)
	if user == nil {
		s.App.Audit.Write("login", ip, map[string]any{
			"username": payload.Username,
			"result":   "denied",
		})
		forbidden(w, errInvalidCredentials)
		return
	}
	s.App.Audit.Write("login", ip, map[string]any{
		"username": user.Username,
		"result":   "ok",
		"uuid":     user.UUID,
	})
	s.App.Sessions.DeleteUser(user.UUID)
	w.WriteHeader(http.StatusNoContent)
}
