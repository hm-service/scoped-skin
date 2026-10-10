// Package db 是 SQLite 存储层：单库、无外键约束，一致性由代码保证（DESIGN.md 4）。
// asset 里 is_builtin = 1 的是随包发行的官方默认皮肤（用户不能编辑 / 删除）。
package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"

	_ "modernc.org/sqlite"

	"scoped-skin/core"
)

const schema = `
CREATE TABLE IF NOT EXISTS user (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid          TEXT NOT NULL UNIQUE,   -- 用户 UUID，32 位无横线小写
    role          TEXT NOT NULL,          -- 身份组：admin / normal / guest
    player_name   TEXT NOT NULL UNIQUE,   -- 游戏内角色名，小写字母 + 数字 + 下划线
    username      TEXT NOT NULL UNIQUE,   -- 账号登录名
    password_hash TEXT NOT NULL UNIQUE,   -- 账号登录密码的 Argon2id 哈希串
    skin          TEXT NOT NULL,          -- 空串表示未装备
    cape          TEXT NOT NULL           -- 空串表示未装备
);

CREATE TABLE IF NOT EXISTS asset (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    asset_id   TEXT NOT NULL UNIQUE,   -- 资源 ID，实则为文件数据的 sha256
    category   TEXT NOT NULL,          -- skin / cape
    model_type TEXT NOT NULL,          -- share / steve / alex（cape 固定 share）
    is_builtin INTEGER NOT NULL,       -- 1 = 内置资源（用户无法编辑）
    upload_at  INTEGER NOT NULL,       -- UNIX 秒级时间戳
    tags       TEXT NOT NULL,          -- json 字符串列表
    data       BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS favorite (
    id     INTEGER PRIMARY KEY AUTOINCREMENT,
    player TEXT NOT NULL,   -- user.uuid
    asset  TEXT NOT NULL    -- asset.asset_id
);

-- asset 上不建索引（DESIGN.md 4.2）：库就那么大，列表查询本来就要全表扫一遍再按
-- upload_at 排序，按 category / model_type 建索引既省不下多少，还白搭写放大。
CREATE UNIQUE INDEX IF NOT EXISTS idx_favorite_player_asset ON favorite (player, asset);
`

// expected 是期望的列集合；启动时校验，不一致就直接重建（原型不做迁移）。
var expected = map[string][]string{
	"user":     {"id", "uuid", "role", "player_name", "username", "password_hash", "skin", "cape"},
	"asset":    {"id", "asset_id", "category", "model_type", "is_builtin", "upload_at", "tags", "data"},
	"favorite": {"id", "player", "asset"},
}

// Store 包住 database/sql，并在多步写操作上加一把锁（SQLite 单写者）。
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// Open 打开（必要时创建）数据库，保证表结构是当前版本。
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		// DESIGN.md 1.5：数据目录只有属主能进
		if err := os.MkdirAll(dir, core.DataDirPerm); err != nil {
			return nil, err
		}
	}
	// 先把库文件按私密权限建出来：SQLite 的 -wal / -shm 会沿用这个文件的权限
	if err := createPrivateFile(path); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	handle, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	handle.SetMaxOpenConns(1)
	handle.SetMaxIdleConns(1)
	store := &Store{db: handle}
	if err := store.migrate(); err != nil {
		handle.Close()
		return nil, err
	}
	// migrate 之后 -wal / -shm 已经出现，一起收紧（老库也顺带修正）
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		if err := core.TightenPerm(name, core.SecretPerm); err != nil {
			handle.Close()
			return nil, err
		}
	}
	return store, nil
}

// createPrivateFile 把库文件按 0600 建出来；已存在就只收紧权限。
func createPrivateFile(path string) error {
	handle, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, core.SecretPerm)
	if err != nil {
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}
	return core.TightenPerm(path, core.SecretPerm)
}

// Close 关掉数据库连接。
func (s *Store) Close() error { return s.db.Close() }

// Ready 是 /readyz 的探针：__probe 建得出来又删得掉，就说明库可读可写。
//
// 探针表不在 DESIGN.md 的三张表里、schema 校验也不看它，所以不会干扰业务。
func (s *Store) Ready() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec("DROP TABLE IF EXISTS __probe"); err != nil {
		return err
	}
	if _, err := s.db.Exec("CREATE TABLE __probe (id INTEGER)"); err != nil {
		return err
	}
	if _, err := s.db.Exec("DROP TABLE __probe"); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	for table, columns := range expected {
		actual, err := s.columns(table)
		if err != nil {
			return err
		}
		if slices.Compare(slices.Sorted(slices.Values(actual)), slices.Sorted(slices.Values(columns))) == 0 {
			continue
		}
		slog.Warn("表结构与 DESIGN.md 不一致，重建数据库", slog.String("table", table))
		for _, name := range []string{"favorite", "asset", "user"} {
			if _, err := s.db.Exec("DROP TABLE IF EXISTS " + name); err != nil {
				return err
			}
		}
		if _, err := s.db.Exec(schema); err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (s *Store) columns(table string) ([]string, error) {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    any
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
