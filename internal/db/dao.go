package db

import "time"

// Data Access Object 层
type User struct {
	ID       int64  `json:"id"`
	Openid   string `json:"-"`
	Nickname string `json:"nickname"`
	BookID   int64  `json:"bookId"`
}

type Record struct {
	ID           int64  `json:"id"`
	BookID       int64  `json:"bookId"`
	UserID       int64  `json:"userId"`
	Type         int    `json:"type"`   // 1支出 2收入
	Amount       int64  `json:"amount"` // 分
	Category     string `json:"category"`
	Note         string `json:"note"`
	Account      string `json:"account"`
	Counterparty string `json:"counterparty"`
	HappenedAt   string `json:"happenedAt"` // YYYY-MM-DD
	IsDeleted    int    `json:"isDeleted"`
	UpdatedAt    string `json:"updatedAt"`
}

// FindOrCreateUser：按 openid 找用户，不存在则创建并把 book_id 设为自身 id（个人账本）
func (s *Store) FindOrCreateUser(openid, nickname string) (*User, error) {
	u := &User{}
	err := s.DB.QueryRow(`SELECT id, openid, nickname, book_id FROM user WHERE openid=?`, openid).
		Scan(&u.ID, &u.Openid, &u.Nickname, &u.BookID)
	if err == nil {
		return u, nil
	}
	res, err := s.DB.Exec(`INSERT INTO user(openid, nickname, book_id) VALUES(?,?,0)`, openid, nickname)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if _, err := s.DB.Exec(`UPDATE user SET book_id=? WHERE id=?`, id, id); err != nil {
		return nil, err
	}
	return &User{ID: id, Openid: openid, Nickname: nickname, BookID: id}, nil
}

func (s *Store) BookIDOf(userID int64) (int64, error) {
	var bid int64
	err := s.DB.QueryRow(`SELECT book_id FROM user WHERE id=?`, userID).Scan(&bid)
	return bid, err
}

// Pull：拉取该账本在 since 之后变更的记录（含软删，供客户端同步删除）
func (s *Store) Pull(bookID int64, since string) ([]Record, error) {
	rows, err := s.DB.Query(`
		SELECT id, book_id, user_id, type, amount, category,
		       COALESCE(note,''), COALESCE(account,''), COALESCE(counterparty,''),
		       happened_at, is_deleted, updated_at
		FROM record WHERE book_id=? AND updated_at > ? ORDER BY updated_at ASC`,
		bookID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.BookID, &r.UserID, &r.Type, &r.Amount, &r.Category,
			&r.Note, &r.Account, &r.Counterparty, &r.HappenedAt, &r.IsDeleted, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Upsert：id<=0 视为新增；否则按 id 更新。统一在一个事务里写，配合单连接保证串行。
func (s *Store) Upsert(bookID, userID int64, recs []Record) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for _, r := range recs {
		if r.ID <= 0 {
			_, err = tx.Exec(`INSERT INTO record
				(book_id,user_id,type,amount,category,note,account,counterparty,happened_at,is_deleted,updated_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
				bookID, userID, r.Type, r.Amount, r.Category, r.Note, r.Account, r.Counterparty,
				r.HappenedAt, r.IsDeleted, now)
		} else {
			_, err = tx.Exec(`UPDATE record SET
				type=?,amount=?,category=?,note=?,account=?,counterparty=?,happened_at=?,is_deleted=?,updated_at=?
				WHERE id=? AND book_id=?`,
				r.Type, r.Amount, r.Category, r.Note, r.Account, r.Counterparty,
				r.HappenedAt, r.IsDeleted, now, r.ID, bookID)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
