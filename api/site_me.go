package api

import (
	"net/http"

	"scoped-skin/db"
	"scoped-skin/httpx"
)

// mePayload 组 /api/me 与 /api/me/equip 共用的响应体：当前账号 + 装备中的皮肤 / 披风。
//
// skin / cape 是完整贴图条目，没装备就是 null（Python 版同理：只有装了才去查库）。
func (s *Server) mePayload(user *db.User) (map[string]any, error) {
	payload := map[string]any{"player": publicUser(user)}

	var skin *db.Item
	if user.Skin != "" {
		item, err := s.App.Store.Item(user.Skin, user.UUID)
		if err != nil {
			return nil, err
		}
		skin = item
	}
	var cape *db.Item
	if user.Cape != "" {
		item, err := s.App.Store.Item(user.Cape, user.UUID)
		if err != nil {
			return nil, err
		}
		cape = item
	}
	payload["skin"] = skin
	payload["cape"] = cape
	return payload, nil
}

// handleMe 返回当前登录用户 + 他装备中的皮肤 / 披风。
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := s.currentUser(w, r)
	if user == nil {
		return
	}
	payload, err := s.mePayload(user)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.texture-read")
		return
	}
	httpx.Write(w, http.StatusOK, payload)
}

// handleEquip 把一张贴图设为当前装备的皮肤 / 披风。
func (s *Server) handleEquip(w http.ResponseWriter, r *http.Request) {
	user := s.currentUser(w, r)
	if user == nil {
		return
	}
	var payload struct {
		AssetID string `json:"asset_id"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}

	status, err := s.App.Store.Equip(payload.AssetID, user.UUID)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.equip")
		return
	}
	if status == "missing" {
		fail(w, r, http.StatusNotFound, "asset.unknown")
		return
	}

	// 装备成功后回的就是 /api/me 的那份结构；这里重查一次 user 行，
	// 让响应里的 skin / cape 就是刚写进去的值（账号刚被并发删掉则按未登录处理）。
	updated := s.App.Store.User(user.UUID)
	if updated == nil {
		unauthorized(w, r, "auth.stale")
		return
	}
	body, err := s.mePayload(updated)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.texture-read")
		return
	}
	httpx.Write(w, http.StatusOK, body)
}
