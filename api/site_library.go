package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"

	"scoped-skin/core"
	"scoped-skin/db"
	"scoped-skin/httpx"
)

// 分块读，块大小照 Python 版的 READ_CHUNK_BYTES 来。
const uploadChunkBytes = 64 * 1024

// uploadMemoryBytes 交给 ParseMultipartForm 的内存阈值：请求体本身已被中间件的
// MaxBytesReader 卡在 MaxRequestBytes，所以整个 multipart 都能留在内存里。
const uploadMemoryBytes = MaxRequestBytes

type itemWithStatus struct {
	db.Item
	Created bool `json:"created"`
	Updated bool `json:"updated"`
}

type itemWithStats struct {
	db.Item
	Stats db.Stats `json:"stats"`
}

// formValue 取表单 / 查询参数：参数出现过就用它的值（空串也算出现过），
// 没出现才用默认值 —— 与 FastAPI 的 `Form(default)` / 查询默认参数一致。
func formValue(values url.Values, name, fallback string) string {
	if list, ok := values[name]; ok && len(list) > 0 {
		return list[0]
	}
	return fallback
}

// 不能一把 io.ReadAll：那样是先把整个请求体吃进内存再校验，上限就只是个摆设。
func readUpload(w http.ResponseWriter, r *http.Request, reader io.Reader) ([]byte, bool) {
	var buffer bytes.Buffer
	chunk := make([]byte, uploadChunkBytes)
	for {
		count, err := reader.Read(chunk)
		if count > 0 {
			if buffer.Len()+count > core.MaxUploadBytes {
				fail(w, r, http.StatusBadRequest, "upload.too-big")
				return nil, false
			}
			buffer.Write(chunk[:count])
		}
		if errors.Is(err, io.EOF) {
			return buffer.Bytes(), true
		}
		if err != nil {
			// 读请求体时撞上中间件 MaxBytesReader 的上限（Content-Length 缺失、
			// 靠分块传超大体的情况），错误体与中间件那道闸保持一致。
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				fail(w, r, http.StatusRequestEntityTooLarge, "request.too-big")
			} else {
				fail(w, r, http.StatusBadRequest, "upload.missing")
			}
			return nil, false
		}
	}
}

// handleTags 返回库里已有的标签（含使用次数）：没有固定词表，前端据此做
// 「已有 + 手动输入」。
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	user := s.currentUser(w, r)
	if user == nil {
		return
	}
	tags, err := s.App.Store.TagCounts()
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.tag-read")
		return
	}
	httpx.Write(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	user := s.currentUser(w, r)
	if user == nil {
		return
	}
	query := r.URL.Query()
	category := formValue(query, "category", "skin")
	if !slices.Contains(core.Categories, category) {
		fail(w, r, http.StatusBadRequest, "library.category")
		return
	}
	tab := formValue(query, "tab", "all")
	if tab != "all" && tab != "favorites" {
		fail(w, r, http.StatusBadRequest, "library.tab")
		return
	}
	q := formValue(query, "q", "")
	// 搜索串本身不落库、也不回显，这里只防「一个超长 q 拼出几万条 LIKE」——
	// 长度按 core.MaxQueryLength（标签数量 × 单条长度）算，正常用永远碰不到。
	if len([]rune(q)) > core.MaxQueryLength {
		fail(w, r, http.StatusBadRequest, "library.query-long", core.MaxQueryLength)
		return
	}

	items, err := s.App.Store.List(category, tab, q, user.UUID)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.list-read")
		return
	}
	stats, err := s.App.Store.Stats(user.UUID)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.stats-read")
		return
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"category": category,
		"tab":      tab,
		"q":        q,
		"tokens":   core.SplitQuery(q),
		"items":    items,
		"stats":    stats,
	})
}

// precheckRequest 是上传前预检的请求体。前端把文件内容（base64）一起带上来，
// 由服务端自己验一遍、自己算 asset_id，而不是只信前端算好的那个值。
type precheckRequest struct {
	Kind string `json:"kind"`
	Data string `json:"data"`
}

// 尺寸已经限死在 64×64 / 64×32，把原图带上来一起验没什么负担：
// 服务端跑规范化（PNG 头 + 真解码 + 尺寸 + 重新编码）并算 asset_id，顺手把这个 ID 回给前端
// —— 入库字节是重编码过的，前端自己算不出这个 ID。新皮肤也返回 200（带 exists: false），
// 避免浏览器控制台里出现 404。
func (s *Server) handlePrecheck(w http.ResponseWriter, r *http.Request) {
	user := s.currentUser(w, r)
	if user == nil {
		return
	}
	var payload precheckRequest
	if !decodeJSON(w, r, &payload) {
		return
	}
	kind, err := core.NormalizeKind(payload.Kind)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, businessMessage(r, err))
		return
	}
	data, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		fail(w, r, http.StatusBadRequest, "upload.base64")
		return
	}
	normalized, _, _, err := core.NormalizeTexture(kind, data)
	if err != nil {
		if text := businessMessage(r, err); text != "" {
			httpx.Fail(w, http.StatusBadRequest, text)
			return
		}
		fail(w, r, http.StatusInternalServerError, "store.upload")
		return
	}
	digest := sha256Hex(normalized)
	item, err := s.App.Store.Item(digest, user.UUID)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.texture-read")
		return
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"asset_id": digest,
		"exists":   item != nil,
		"item":     item,
	})
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// 前端不再算 asset_id：服务端把原图规范化（解码 → 重新编码，剥掉多余的 PNG chunk）后
// 取这串入库字节的 sha256 作为 ID。文件分块读，一超过贴图上限就 400 —— 请求体那道闸
// 在中间件里。
func (s *Server) handleUploadSkin(w http.ResponseWriter, r *http.Request) {
	user := s.currentWriter(w, r)
	if user == nil {
		return
	}

	if err := r.ParseMultipartForm(uploadMemoryBytes); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			fail(w, r, http.StatusRequestEntityTooLarge, "request.too-big")
			return
		}
		fail(w, r, http.StatusBadRequest, "upload.missing")
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		fail(w, r, http.StatusBadRequest, "upload.missing")
		return
	}
	handle, err := files[0].Open()
	if err != nil {
		fail(w, r, http.StatusBadRequest, "upload.missing")
		return
	}
	defer handle.Close()

	data, ok := readUpload(w, r, handle)
	if !ok {
		return
	}

	form := r.MultipartForm.Value
	item, status, err := s.App.Store.AddUpload(
		formValue(form, "kind", ""),
		form["tags"],
		data,
	)
	if err != nil {
		if text := businessMessage(r, err); text != "" {
			httpx.Fail(w, http.StatusBadRequest, text)
			return
		}
		fail(w, r, http.StatusInternalServerError, "store.upload")
		return
	}
	s.App.Audit.Write("asset.upload", s.App.ClientIP(r), map[string]any{
		"actor":    user.UUID,
		"asset_id": item.ID,
		"kind":     item.Kind,
		"size":     item.Size,
		"result":   status,
	})
	httpx.Write(w, http.StatusCreated, itemWithStatus{
		Item:    item,
		Created: status == "created",
		Updated: status == "updated",
	})
}

// handleFavorite 切换收藏，顺带回一份最新统计。收藏是一条持久化记录，所以走写闸门。
func (s *Server) handleFavorite(w http.ResponseWriter, r *http.Request) {
	user := s.currentWriter(w, r)
	if user == nil {
		return
	}
	item, err := s.App.Store.ToggleFavorite(r.PathValue("asset_id"), user.UUID)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.favorite")
		return
	}
	if item == nil {
		fail(w, r, http.StatusNotFound, "asset.unknown")
		return
	}
	stats, err := s.App.Store.Stats(user.UUID)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.stats-read")
		return
	}
	httpx.Write(w, http.StatusOK, itemWithStats{Item: *item, Stats: stats})
}

// handleAssetUsers 列出正在穿这张贴图的账号：删除前给管理员看的。
//
// 只给 admin：这份名单是「谁装备了什么」，和 /api/admin/users 一样属于管理面。
func (s *Server) handleAssetUsers(w http.ResponseWriter, r *http.Request) {
	if s.currentAdmin(w, r) == nil {
		return
	}
	assetID := r.PathValue("asset_id")
	if item, err := s.App.Store.Item(assetID, ""); err != nil {
		fail(w, r, http.StatusInternalServerError, "store.texture-read")
		return
	} else if item == nil {
		fail(w, r, http.StatusNotFound, "asset.unknown")
		return
	}
	users, err := s.App.Store.AssetUsers(assetID)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.users-read")
		return
	}
	httpx.Write(w, http.StatusOK, struct {
		Users []db.AssetUser `json:"users"`
	}{Users: users})
}

// handleDeleteItem 删除贴图：里「删除资源」只给 admin。
//
// 删除本身会把使用者的 skin / cape 清空（db.dropAssetLocked），这里不做拦截 ——
// 「有谁在用」由 GET /api/items/{asset_id}/users 提前告诉前端，让管理员自己决定。
func (s *Server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	admin := s.currentAdmin(w, r)
	if admin == nil {
		return
	}
	assetID := r.PathValue("asset_id")
	status, err := s.App.Store.Delete(assetID)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.delete")
		return
	}
	if status == "missing" {
		fail(w, r, http.StatusNotFound, "asset.unknown")
		return
	}
	if status == "readonly" {
		fail(w, r, http.StatusBadRequest, "asset.cannot-del")
		return
	}
	s.App.Audit.Write("asset.delete", s.App.ClientIP(r), map[string]any{
		"actor":    admin.UUID,
		"asset_id": assetID,
	})
	stats, err := s.App.Store.Stats(admin.UUID)
	if err != nil {
		fail(w, r, http.StatusInternalServerError, "store.stats-read")
		return
	}
	httpx.Write(w, http.StatusOK, struct {
		Deleted string   `json:"deleted"`
		Stats   db.Stats `json:"stats"`
	}{Deleted: assetID, Stats: stats})
}
