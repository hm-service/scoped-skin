package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"

	"scoped-skin/core"
)

// builtinEpoch 是内置资源的固定「上传时间」基准，保证排序稳定。
const builtinEpoch = 1_700_000_000

// itemColumns 是 DESIGN.md 3.6「贴图条目」的字段来源；head 只取 BLOB 前 24 字节，
// 列表查询不必把整张图读进内存。
const itemColumns = "a.asset_id, a.category, a.model_type, a.tags, a.upload_at, a.is_builtin," +
	" length(a.data) AS size, substr(a.data, 1, 24) AS head, (f.id IS NOT NULL) AS favorite"

// EnsureBuiltins 把内置资源对齐到 manifest：缺的补上，manifest 里没有的（旧版本残留）删掉。
//
// 每个角色只带官方默认模型那一份（wide=steve / slim=alex）。用户上传的数据不受影响；
// 被移除的内置资源会顺手清掉引用它的收藏与装备状态。
func (s *Store) EnsureBuiltins(fsys fs.FS) error {
	raw, err := fs.ReadFile(fsys, "manifest.json")
	if err != nil {
		return fmt.Errorf("读取内置资源清单失败: %w", err)
	}
	var manifest struct {
		Builtin []builtinEntry `json:"builtin"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("解析内置资源清单失败: %w", err)
	}
	total := len(manifest.Builtin)

	type wanted struct {
		category string
		model    string
		tags     []string
		data     []byte
		uploadAt int64
	}
	assets := map[string]wanted{}
	for index, entry := range manifest.Builtin {
		data, err := fs.ReadFile(fsys, entry.File)
		if err != nil {
			return fmt.Errorf("读取内置资源 %s 失败: %w", entry.File, err)
		}
		category := entry.Category
		if category == "" {
			category = "skin"
		}
		// 内置资源也走同一条规范化路径：id 同样是「入库字节」的 sha256
		normalized, _, _, err := core.NormalizeTexture(category, data)
		if err != nil {
			return fmt.Errorf("内置资源 %s 不合规: %w", entry.File, err)
		}
		tags, err := core.NormalizeTags(entry.Tags)
		if err != nil {
			return fmt.Errorf("内置资源 %s 的标签不合规: %w", entry.File, err)
		}
		digest := sha256.Sum256(normalized)
		assets[hex.EncodeToString(digest[:])] = wanted{
			category: category,
			model:    core.NormalizeModel(category, entry.Model),
			tags:     tags,
			data:     normalized,
			uploadAt: int64(builtinEpoch + total - index),
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.builtinIDsLocked()
	if err != nil {
		return err
	}
	for assetID, item := range assets {
		if slices.Contains(existing, assetID) {
			continue
		}
		tags, err := json.Marshal(item.tags)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(
			"INSERT OR IGNORE INTO asset (asset_id, category, model_type, is_builtin, upload_at, tags, data)"+
				" VALUES (?, ?, ?, 1, ?, ?, ?)",
			assetID, item.category, item.model, item.uploadAt, string(tags), item.data); err != nil {
			return err
		}
	}
	for _, assetID := range existing {
		if _, keep := assets[assetID]; keep {
			continue
		}
		if err := s.dropAssetLocked(assetID); err != nil {
			return err
		}
	}
	return nil
}

// List 按上传时间倒序返回某个类别下的条目；query 按空白切成标签取交集。
func (s *Store) List(category, tab, query, playerUUID string) ([]Item, error) {
	sqlText := "SELECT " + itemColumns + " FROM asset a" +
		" LEFT JOIN favorite f ON f.asset = a.asset_id AND f.player = ?" +
		" WHERE a.category = ?"
	if tab == "favorites" {
		sqlText += " AND f.id IS NOT NULL"
	}
	sqlText += " ORDER BY a.upload_at DESC, a.id DESC"

	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(sqlText, playerUUID, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tokens := core.SplitQuery(query)
	if len(tokens) == 0 {
		return items, nil
	}
	kept := items[:0]
	for _, item := range items {
		if core.MatchesQuery(item.Tags, tokens) {
			kept = append(kept, item)
		}
	}
	return kept, nil
}

// Item 取一条贴图（顺带带上该用户是否收藏过），没有返回 nil。
func (s *Store) Item(assetID, playerUUID string) (*Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.itemLocked(assetID, playerUUID)
}

// Texture 取贴图字节流；asset_id 长度不对或查无此图都返回 nil。
func (s *Store) Texture(assetID string) ([]byte, error) {
	if len(assetID) != 64 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var data []byte
	err := s.db.QueryRow("SELECT data FROM asset WHERE asset_id = ?", assetID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// TagCounts 返回库里正在使用的标签 + 使用次数，按热度排序（前端据此做候选）。
func (s *Store) TagCounts() ([]TagCount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query("SELECT tags FROM asset")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		for _, tag := range decodeTags(raw) {
			counts[tag]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	tags := make([]TagCount, 0, len(counts))
	for tag, count := range counts {
		tags = append(tags, TagCount{Tag: tag, Count: count})
	}
	slices.SortFunc(tags, func(left, right TagCount) int {
		if left.Count != right.Count {
			return right.Count - left.Count // 次数多的排前面
		}
		return strings.Compare(left.Tag, right.Tag)
	})
	return tags, nil
}

// Stats 返回皮肤 / 披风总数与该用户的收藏数。
func (s *Store) Stats(playerUUID string) (Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statsLocked(playerUUID)
}

// AddUpload 写入或更新一张贴图，返回 (条目, 状态)，状态是 created / updated / unchanged。
//
// 入库前先 core.NormalizeTexture（解码 → 重新编码，剥掉 PNG 里与位图无关的 chunk），
// asset_id 就是这串入库字节的 sha256 —— 客户端不参与算这个 ID（预检会把它回给前端）。
// 内容已存在时不会新建、也不会合并标签：标签与模型按本次提交的值覆盖（没有变化就原样
// 返回）；内置资源不允许改，直接报错。
func (s *Store) AddUpload(category, model string, tags []string, data []byte) (Item, string, error) {
	normalized, _, _, err := core.NormalizeTexture(category, data)
	if err != nil {
		return Item{}, "", err
	}
	wantedTags, err := core.NormalizeTags(tags)
	if err != nil {
		return Item{}, "", err
	}
	digest := sha256.Sum256(normalized)
	hexDigest := hex.EncodeToString(digest[:])

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.itemLocked(hexDigest, "")
	if err != nil {
		return Item{}, "", err
	}
	if existing != nil {
		if existing.Builtin {
			return Item{}, "", &core.ValidationError{Key: "valid.builtin"}
		}
		wantedModel := core.NormalizeModel(category, model)
		if slices.Equal(existing.Tags, wantedTags) && existing.Model == wantedModel {
			return *existing, "unchanged", nil
		}
		encoded, err := json.Marshal(wantedTags)
		if err != nil {
			return Item{}, "", err
		}
		if _, err := s.db.Exec("UPDATE asset SET tags = ?, model_type = ? WHERE asset_id = ?",
			string(encoded), wantedModel, hexDigest); err != nil {
			return Item{}, "", err
		}
		updated, err := s.itemLocked(hexDigest, "")
		if err != nil {
			return Item{}, "", err
		}
		return *updated, "updated", nil
	}

	encoded, err := json.Marshal(wantedTags)
	if err != nil {
		return Item{}, "", err
	}
	if _, err := s.db.Exec(
		"INSERT INTO asset (asset_id, model_type, category, data, tags, upload_at, is_builtin)"+
			" VALUES (?, ?, ?, ?, ?, ?, 0)",
		hexDigest, core.NormalizeModel(category, model), category, normalized, string(encoded),
		time.Now().Unix()); err != nil {
		return Item{}, "", err
	}
	created, err := s.itemLocked(hexDigest, "")
	if err != nil {
		return Item{}, "", err
	}
	return *created, "created", nil
}

// Delete 删除贴图。返回 "deleted" / "builtin" / "missing"。
func (s *Store) Delete(assetID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var builtin bool
	err := s.db.QueryRow("SELECT is_builtin FROM asset WHERE asset_id = ?", assetID).Scan(&builtin)
	if errors.Is(err, sql.ErrNoRows) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	if builtin {
		return "builtin", nil
	}
	if err := s.dropAssetLocked(assetID); err != nil {
		return "", err
	}
	return "deleted", nil
}

// ---------------------------------------------------------------- 内部实现

func (s *Store) itemLocked(assetID, playerUUID string) (*Item, error) {
	row := s.db.QueryRow(
		"SELECT "+itemColumns+" FROM asset a"+
			" LEFT JOIN favorite f ON f.asset = a.asset_id AND f.player = ?"+
			" WHERE a.asset_id = ?", playerUUID, assetID)
	item, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) statsLocked(playerUUID string) (Stats, error) {
	stats := Stats{}
	rows, err := s.db.Query("SELECT category, COUNT(*) FROM asset GROUP BY category")
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var (
			category string
			count    int
		)
		if err := rows.Scan(&category, &count); err != nil {
			rows.Close()
			return stats, err
		}
		if category == "cape" {
			stats.Capes = count
		} else {
			stats.Skins = count
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return stats, err
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM favorite WHERE player = ?", playerUUID).
		Scan(&stats.Favorites); err != nil {
		return stats, err
	}
	return stats, nil
}

// dropAssetLocked 删掉一条资源，并清掉引用它的收藏与装备状态（无外键，靠代码保证）。
func (s *Store) dropAssetLocked(assetID string) error {
	for _, statement := range []string{
		"DELETE FROM asset WHERE asset_id = ?",
		"DELETE FROM favorite WHERE asset = ?",
		"UPDATE user SET skin = '' WHERE skin = ?",
		"UPDATE user SET cape = '' WHERE cape = ?",
	} {
		if _, err := s.db.Exec(statement, assetID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) builtinIDsLocked() ([]string, error) {
	rows, err := s.db.Query("SELECT asset_id FROM asset WHERE is_builtin = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var assetID string
		if err := rows.Scan(&assetID); err != nil {
			return nil, err
		}
		ids = append(ids, assetID)
	}
	return ids, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanItem(row scanner) (*Item, error) {
	var (
		assetID, category, model, tags string
		uploadAt                       int64
		builtin, favorite              bool
		size                           int
		head                           []byte
	)
	if err := row.Scan(&assetID, &category, &model, &tags, &uploadAt, &builtin, &size, &head, &favorite); err != nil {
		return nil, err
	}
	width, height := core.HeaderSize(head)
	return &Item{
		ID:         assetID,
		Kind:       category,
		Model:      model,
		Tags:       decodeTags(tags),
		UploadAt:   uploadAt,
		Builtin:    builtin,
		Width:      width,
		Height:     height,
		Size:       size,
		Favorite:   favorite,
		TextureURL: "/asset/" + assetID,
	}, nil
}

func decodeTags(raw string) []string {
	tags := []string{}
	if raw == "" {
		return tags
	}
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return []string{}
	}
	return core.CleanTags(tags)
}
