package api

import (
	"net/http"
	"sort"

	"scoped-skin/core"
	"scoped-skin/db"
	"scoped-skin/httpx"
)

// adminSnapshot 是维护页用的整份快照：用户列表 + 调用者自己的 uuid
// （前端据此禁掉「删掉自己」这类自杀操作）。
func (s *Server) adminSnapshot(requesting *db.User) (map[string]any, error) {
	users, err := s.App.Store.Users()
	if err != nil {
		return nil, err
	}
	list := make([]userView, 0, len(users))
	for index := range users {
		list = append(list, publicUser(&users[index]))
	}
	return map[string]any{"users": list, "me": requesting.UUID}, nil
}

// writeUserError 把业务层的字段校验错误翻成 HTTP：撞唯一约束 → 409，其余 → 400，
// detail 按请求语言渲染（前端直接展示）。
func writeUserError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadRequest
	if isConflict(err) {
		status = http.StatusConflict
	}
	if text := businessMessage(r, err); text != "" {
		httpx.Fail(w, status, text)
		return
	}
	fail(w, r, http.StatusInternalServerError, "store.user-write")
}

// handleListUsers 是维护页的数据源：整份用户列表（不含密码哈希）。
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	admin := s.currentAdmin(w, r)
	if admin == nil {
		return
	}
	snapshot, err := s.adminSnapshot(admin)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.user-read")
		return
	}
	httpx.Write(w, http.StatusOK, snapshot)
}

// handleCreateUser 新建账号。uuid 默认按离线模式规则由游戏内名称推导，之后再改也行。
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	admin := s.currentAdmin(w, r)
	if admin == nil {
		return
	}
	var payload struct {
		Username   string  `json:"username"`
		Password   string  `json:"password"`
		PlayerName string  `json:"player_name"`
		Role       *string `json:"role"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	// role 不出现时用默认身份组；出现了（哪怕是空串）就按提交的值校验
	role := core.DefaultRole
	if payload.Role != nil {
		role = *payload.Role
	}

	created, err := s.App.Store.CreateUser(payload.Username, payload.Password, payload.PlayerName, role)
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	if err := s.App.RefreshTokens(); err != nil {
		fail(w, r, http.StatusInternalServerError, "store.token")
		return
	}
	s.App.Audit.Write("user.create", s.App.ClientIP(r), map[string]any{
		"actor":       admin.UUID,
		"uuid":        created.UUID,
		"username":    created.Username,
		"player_name": created.PlayerName,
		"role":        created.Role,
	})

	snapshot, err := s.adminSnapshot(admin)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.user-read")
		return
	}
	snapshot["user"] = publicUser(created)
	httpx.Write(w, http.StatusCreated, snapshot)
}

// handleUpdateUser 改指定账号的 uuid / 账号名 / 密码 / 游戏内名称 / 身份组。
//
// 请求体里只有出现过的字段才算改动（指针为 nil 就是没出现）；改完按 uuid 作废
// 该账号的 Yggdrasil 内存会话，站点旧 token 也在 RefreshTokens 里一并失效。
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	admin := s.currentAdmin(w, r)
	if admin == nil {
		return
	}
	target, err := core.NormalizeUUID(r.PathValue("uuid"))
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	var payload struct {
		UUID       *string `json:"uuid"`
		Username   *string `json:"username"`
		Password   *string `json:"password"`
		PlayerName *string `json:"player_name"`
		Role       *string `json:"role"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	changes := map[string]string{}
	if payload.UUID != nil {
		changes["uuid"] = *payload.UUID
	}
	if payload.Username != nil {
		changes["username"] = *payload.Username
	}
	if payload.Password != nil {
		changes["password"] = *payload.Password
	}
	if payload.PlayerName != nil {
		changes["player_name"] = *payload.PlayerName
	}
	if payload.Role != nil {
		changes["role"] = *payload.Role
	}

	updated, err := s.App.Store.UpdateUser(target, changes)
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	if updated == nil {
		fail(w, r, http.StatusNotFound, "user.unknown")
		return
	}
	if err := s.App.RefreshTokens(target); err != nil {
		fail(w, r, http.StatusInternalServerError, "store.token")
		return
	}

	// 审计只记改了哪些字段名，绝不记密码本身
	changed := make([]string, 0, len(changes))
	for field := range changes {
		changed = append(changed, field)
	}
	sort.Strings(changed)
	s.App.Audit.Write("user.update", s.App.ClientIP(r), map[string]any{
		"actor":    admin.UUID,
		"uuid":     updated.UUID,
		"was_uuid": target,
		"username": updated.Username,
		"changed":  changed,
	})

	snapshot, err := s.adminSnapshot(admin)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.user-read")
		return
	}
	snapshot["user"] = publicUser(updated)
	httpx.Write(w, http.StatusOK, snapshot)
}

// handleDeleteUser 删除账号：最后一个管理员不允许删（避免把管理员锁在门外）。
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	admin := s.currentAdmin(w, r)
	if admin == nil {
		return
	}
	target, err := core.NormalizeUUID(r.PathValue("uuid"))
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	// 删除前先把账号取出来，审计日志要记它的账号名
	removed := s.App.Store.User(target)

	status, err := s.App.Store.DeleteUser(target)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.user-delete")
		return
	}
	if status == "missing" {
		fail(w, r, http.StatusNotFound, "user.unknown")
		return
	}
	if status == "last_admin" {
		fail(w, r, http.StatusBadRequest, "user.last-admin")
		return
	}
	if err := s.App.RefreshTokens(); err != nil {
		fail(w, r, http.StatusInternalServerError, "store.token")
		return
	}

	username := ""
	if removed != nil {
		username = removed.Username
	}
	s.App.Audit.Write("user.delete", s.App.ClientIP(r), map[string]any{
		"actor":    admin.UUID,
		"uuid":     target,
		"username": username,
	})

	snapshot, err := s.adminSnapshot(admin)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.user-read")
		return
	}
	snapshot["deleted"] = target
	httpx.Write(w, http.StatusOK, snapshot)
}
