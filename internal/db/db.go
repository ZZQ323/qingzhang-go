package db

import (
	"database/sql"
	"strings"

	_ "modernc.org/sqlite" // 纯 Go 驱动，无需 cgo / C 工具链
)

type Store struct{ DB *sql.DB }

func Open(dsn string) (*Store, error) {
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// 关键：SQLite 写操作串行，把连接池设为 1，从 Go 层避免 "database is busy"。
	// 2-3 用户量级，单连接完全够用，且最省心。
	d.SetMaxOpenConns(1)
	if err := d.Ping(); err != nil {
		return nil, err
	}
	return &Store{DB: d}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) Migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS user (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			openid    TEXT NOT NULL UNIQUE,
			nickname  TEXT,
			book_id   INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS record (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id      INTEGER NOT NULL,
			user_id      INTEGER NOT NULL,
			type         INTEGER NOT NULL,
			amount       INTEGER NOT NULL,
			category     TEXT NOT NULL,
			note         TEXT,
			account      TEXT,
			counterparty TEXT,
			happened_at  TEXT NOT NULL,
			is_deleted   INTEGER NOT NULL DEFAULT 0,
			updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
			created_at   TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sync  ON record(book_id, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_query ON record(book_id, happened_at)`,
		// 账单导入去重：external_id 为交易单号，TEXT 不限长（可容纳 56+ 位）。
		// 老库升级用 ALTER 补列（已存在会报错，忽略）。
		`ALTER TABLE record ADD COLUMN external_id TEXT NOT NULL DEFAULT ''`,
		// 同一账本内交易单号唯一；空串不参与去重（手记记录无单号）。
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_extid ON record(book_id, external_id) WHERE external_id <> ''`,

		// ── 多账本 ──
		// book：账本（id 沿用旧 book_id 体系，个人账本 id=user.id，保证历史 record.book_id 不失效）
		`CREATE TABLE IF NOT EXISTS book (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			name      TEXT NOT NULL,
			owner_id  INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// user_book：用户可访问哪些账本（多对多）
		`CREATE TABLE IF NOT EXISTS user_book (
			user_id INTEGER NOT NULL,
			book_id INTEGER NOT NULL,
			PRIMARY KEY(user_id, book_id)
		)`,
		// user 增加「当前账本」
		`ALTER TABLE user ADD COLUMN current_book_id INTEGER NOT NULL DEFAULT 0`,
		// 个人账本 id 沿用 user.id（小整数）；新建账本走 book 自增，
		// 把自增起点抬到 100000 以上，与 user.id 空间隔离，避免撞车。
		`UPDATE sqlite_sequence SET seq=100000 WHERE name='book' AND seq<100000`,
		`INSERT INTO sqlite_sequence(name,seq) SELECT 'book',100000 WHERE NOT EXISTS(SELECT 1 FROM sqlite_sequence WHERE name='book')`,
		// ── 历史数据迁移（幂等）──
		// 每个用户的个人账本
		`INSERT OR IGNORE INTO book(id, name, owner_id) SELECT id, '我的账本', id FROM user`,
		// 自己能访问自己的个人账本
		`INSERT OR IGNORE INTO user_book(user_id, book_id) SELECT id, id FROM user`,
		// 曾加入过别人账本的，补进 user_book
		`INSERT OR IGNORE INTO user_book(user_id, book_id) SELECT id, book_id FROM user WHERE book_id<>0 AND book_id<>id`,
		// 设定当前账本：原 book_id 有效则用它，否则用个人账本
		`UPDATE user SET current_book_id = CASE WHEN book_id<>0 THEN book_id ELSE id END WHERE current_book_id=0`,
	}
	for _, q := range stmts {
		if _, err := s.DB.Exec(q); err != nil {
			// ALTER ADD COLUMN 在列已存在时会报错，属预期，跳过
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return err
		}
	}
	return nil
}
