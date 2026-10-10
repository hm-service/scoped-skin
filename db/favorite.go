package db

// ToggleFavorite 切换收藏；贴图不存在返回 nil。
func (s *Store) ToggleFavorite(assetID, playerUUID string) (*Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, err := s.itemLocked(assetID, playerUUID)
	if err != nil || item == nil {
		return nil, err
	}
	if item.Favorite {
		_, err = s.db.Exec("DELETE FROM favorite WHERE player = ? AND asset = ?", playerUUID, assetID)
	} else {
		_, err = s.db.Exec("INSERT OR IGNORE INTO favorite (player, asset) VALUES (?, ?)",
			playerUUID, assetID)
	}
	if err != nil {
		return nil, err
	}
	return s.itemLocked(assetID, playerUUID)
}
