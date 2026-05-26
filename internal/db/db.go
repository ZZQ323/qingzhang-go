package db

import (
	"database/sql"

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
	}
	for _, q := range stmts {
		if _, err := s.DB.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
