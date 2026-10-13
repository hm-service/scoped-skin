package db

import (
	"database/sql"
	"errors"
	"strings"

	"scoped-skin/core"
	"scoped-skin/i18n"
)

const userColumns = "uuid, role, player_name, username, password_hash, skin, cape"

// UserCount 返回账号总数（首次启动判定用）。
func (s *Store) UserCount() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM user").Scan(&count)
	return count, err
}

// Authenticate 账号密码校验：通过返回 user 行，否则返回 nil（站点登录与 Yggdrasil 共用）。
func (s *Store) Authenticate(username, password string) *User {
	user := s.UserByUsername(strings.TrimSpace(username))
	if user == nil || !core.VerifyPassword(password, user.PasswordHash) {
		return nil
	}
	return user
}

// Users 按入库顺序返回全部账号。
func (s *Store) Users() ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usersLocked()
}

// User 按 UUID 查账号（带横线的输入也接受），没有返回 nil。
func (s *Store) User(playerUUID string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.userLocked(core.DashlessUUID(playerUUID))
}

// UserByUsername 按账号登录名查账号，没有返回 nil。
func (s *Store) UserByUsername(username string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.userByUsernameLocked(username)
}

// UserByPlayerName 按游戏内角色名查账号（大小写不敏感），没有返回 nil。
func (s *Store) UserByPlayerName(playerName string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	wanted := strings.ToLower(strings.TrimSpace(playerName))
	if wanted == "" {
		return nil
	}
	users, err := s.usersLocked()
	if err != nil {
		return nil
	}
	for index := range users {
		if strings.ToLower(users[index].PlayerName) == wanted {
			return &users[index]
		}
	}
	return nil
}

func (s *Store) AdminCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.adminCountLocked()
}

// CreateUser 新建账号；uuid 按离线模式规则从游戏内名称推导（管理员可再改）。
func (s *Store) CreateUser(username, password, playerName, role string) (*User, error) {
	return s.CreateUserWithUUID(username, password, playerName, role, "")
}

// CreateUserWithUUID 新建账号。uuid 传空串 = 按离线模式规则从游戏内名称推导；
// 传了就用它 —— 例如对齐正版账号、或接上某个世界里已经存在的玩家数据。
//
// 格式在这里再校验一遍：这条路径不该假定调用方已经过了 HTTP 层的 NormalizeUUID。
func (s *Store) CreateUserWithUUID(username, password, playerName, role, uuid string) (*User, error) {
	name, err := core.NormalizePlayerName(playerName)
	if err != nil {
		return nil, err
	}
	login, err := core.NormalizeUsername(username)
	if err != nil {
		return nil, err
	}
	group, err := core.NormalizeRole(role)
	if err != nil {
		return nil, err
	}
	secret, err := core.NormalizePassword(password)
	if err != nil {
		return nil, err
	}
	hash, err := core.HashPassword(secret)
	if err != nil {
		return nil, err
	}

	playerUUID := core.OfflineUUID(name)
	if uuid != "" {
		if playerUUID, err = core.NormalizeUUID(uuid); err != nil {
			return nil, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.uniqueConflictLocked(map[string]string{
		"uuid": playerUUID, "player_name": name, "username": login,
	}, "")
	if err != nil {
		return nil, err
	}
	if err := s.insertUserLocked(playerUUID, login, name, group, hash); err != nil {
		return nil, err
	}
	return s.userLocked(playerUUID), nil
}

// changes 里没出现的键表示不改；uuid 变更时顺手把 favorite 里指向旧 uuid 的行一起
// 改掉（无外键，一致性靠代码）。最后一个管理员不允许被降级。账号不存在返回 (nil, nil)。
func (s *Store) UpdateUser(playerUUID string, changes map[string]string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.userLocked(core.DashlessUUID(playerUUID))
	if current == nil {
		return nil, nil
	}

	assignments := []string{}
	values := []any{}
	fields := map[string]string{}
	for _, field := range []string{"uuid", "username", "player_name", "role"} {
		raw, ok := changes[field]
		if !ok {
			continue
		}
		var (
			normalized string
			err        error
		)
		switch field {
		case "uuid":
			normalized, err = core.NormalizeUUID(raw)
		case "username":
			normalized, err = core.NormalizeUsername(raw)
		case "player_name":
			normalized, err = core.NormalizePlayerName(raw)
		case "role":
			normalized, err = core.NormalizeRole(raw)
		}
		if err != nil {
			return nil, err
		}
		fields[field] = normalized
		assignments = append(assignments, field+" = ?")
		values = append(values, normalized)
	}
	if password, ok := changes["password"]; ok && password != "" {
		secret, err := core.NormalizePassword(password)
		if err != nil {
			return nil, err
		}
		hash, err := core.HashPassword(secret)
		if err != nil {
			return nil, err
		}
		fields["password_hash"] = hash
		assignments = append(assignments, "password_hash = ?")
		values = append(values, hash)
	}
	if len(assignments) == 0 {
		return current, nil
	}
	// 最后一个管理员不可降级 —— guest 同样是降级（只挡 normal 会把管理员清零）。
	if role, ok := fields["role"]; ok && role != "admin" && current.Role == "admin" && s.adminCountLocked() <= 1 {
		return nil, &core.ValidationError{Key: "valid.last-admin"}
	}
	if err := s.uniqueConflictLocked(fields, current.UUID); err != nil {
		return nil, err
	}

	setClause := strings.Join(assignments, ", ")
	if _, err := s.db.Exec("UPDATE user SET "+setClause+" WHERE uuid = ?",
		append(values, current.UUID)...); err != nil {
		return nil, err
	}
	newUUID := current.UUID
	if value, ok := fields["uuid"]; ok {
		newUUID = value
	}
	if newUUID != current.UUID {
		if _, err := s.db.Exec("UPDATE favorite SET player = ? WHERE player = ?",
			newUUID, current.UUID); err != nil {
			return nil, err
		}
	}
	return s.userLocked(newUUID), nil
}

// DeleteUser 删除账号 + 它的收藏。返回 "deleted" / "missing" / "last_admin"。
func (s *Store) DeleteUser(playerUUID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	target := core.DashlessUUID(playerUUID)
	user := s.userLocked(target)
	if user == nil {
		return "missing", nil
	}
	if user.Role == "admin" && s.adminCountLocked() <= 1 {
		return "last_admin", nil
	}
	if _, err := s.db.Exec("DELETE FROM user WHERE uuid = ?", target); err != nil {
		return "", err
	}
	if _, err := s.db.Exec("DELETE FROM favorite WHERE player = ?", target); err != nil {
		return "", err
	}
	return "deleted", nil
}

// Equip 把某个资产设为该用户当前装备的皮肤 / 披风（按 category 写对应列）。
// 返回 "ok" / "missing"。
func (s *Store) Equip(assetID, playerUUID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var kind string
	err := s.db.QueryRow("SELECT kind FROM asset WHERE asset_id = ?", assetID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	column := "skin"
	if core.CategoryOf(kind) == "cape" {
		column = "cape"
	}
	_, err = s.db.Exec("UPDATE user SET "+column+" = ? WHERE uuid = ?",
		assetID, core.DashlessUUID(playerUUID))
	return "ok", err
}

// ---------------------------------------------------------------- 内部实现

func (s *Store) insertUserLocked(playerUUID, username, playerName, role, hash string) error {
	_, err := s.db.Exec(
		"INSERT INTO user (uuid, role, player_name, username, password_hash, skin, cape)"+
			" VALUES (?, ?, ?, ?, ?, '', '')",
		playerUUID, role, playerName, username, hash)
	return err
}

func (s *Store) userLocked(playerUUID string) *User {
	row := s.db.QueryRow("SELECT "+userColumns+" FROM user WHERE uuid = ?", playerUUID)
	user, err := scanUser(row)
	if err != nil {
		return nil
	}
	return user
}

func (s *Store) userByUsernameLocked(username string) *User {
	row := s.db.QueryRow("SELECT "+userColumns+" FROM user WHERE username = ?", strings.TrimSpace(username))
	user, err := scanUser(row)
	if err != nil {
		return nil
	}
	return user
}

func (s *Store) usersLocked() ([]User, error) {
	rows, err := s.db.Query("SELECT " + userColumns + " FROM user ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.UUID, &user.Role, &user.PlayerName, &user.Username,
			&user.PasswordHash, &user.Skin, &user.Cape); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) adminCountLocked() int {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM user WHERE role = 'admin'").Scan(&count); err != nil {
		return 0
	}
	return count
}

// uniqueConflictLocked 做唯一约束预检；fields 的键只可能是 uuid / player_name / username。
func (s *Store) uniqueConflictLocked(fields map[string]string, excludeUUID string) error {
	for _, column := range []string{"uuid", "player_name", "username"} {
		value, ok := fields[column]
		if !ok {
			continue
		}
		query := "SELECT uuid FROM user WHERE " + column + " = ?"
		if column == "player_name" {
			// 名字大小写原样保存，但唯一性大小写不敏感（正版也是「Steve 与 steve 算同一个」）。
			// 建表那条 UNIQUE 索引是二进制比较，不认大小写，所以这一层预检才是闸门。
			query += " COLLATE NOCASE"
		}
		args := []any{value}
		if excludeUUID != "" {
			query += " AND uuid <> ?"
			args = append(args, excludeUUID)
		}
		var found string
		err := s.db.QueryRow(query, args...).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		return &core.ConflictError{Key: "valid.conflict", Args: []any{i18n.Key("field." + column), value}}
	}
	return nil
}

func scanUser(row *sql.Row) (*User, error) {
	var user User
	err := row.Scan(&user.UUID, &user.Role, &user.PlayerName, &user.Username,
		&user.PasswordHash, &user.Skin, &user.Cape)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
