package db

import (
	"database/sql"
	"time"

	"qingzhang/internal/apperr"
)

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
	RecorderName string `json:"recorderName"` // 记账人昵称（共享账本下区分是谁记的）
	ExternalID   string `json:"-"`            // 账单导入的交易单号，用于去重；手记记录为空
}

// Book 账本
type Book struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	OwnerID int64  `json:"ownerId"`
}

// FindOrCreateUser：按 openid 找用户，不存在则创建个人账本并设为当前账本
func (s *Store) FindOrCreateUser(openid, nickname string) (*User, error) {
	u := &User{}
	err := s.DB.QueryRow(`SELECT id, openid, nickname, current_book_id FROM user WHERE openid=?`, openid).
		Scan(&u.ID, &u.Openid, &u.Nickname, &u.BookID)
	if err == nil {
		return u, nil
	}
	res, err := s.DB.Exec(`INSERT INTO user(openid, nickname, book_id) VALUES(?,?,0)`, openid, nickname)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	// 个人账本：book.id = user.id，建立归属与可访问关系，设为当前账本
	if _, err := s.DB.Exec(`INSERT OR IGNORE INTO book(id,name,owner_id) VALUES(?,?,?)`, id, "我的账本", id); err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`INSERT OR IGNORE INTO user_book(user_id,book_id) VALUES(?,?)`, id, id); err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`UPDATE user SET book_id=?, current_book_id=? WHERE id=?`, id, id, id); err != nil {
		return nil, err
	}
	return &User{ID: id, Openid: openid, Nickname: nickname, BookID: id}, nil
}

// BookIDOf 返回用户当前账本 id
func (s *Store) BookIDOf(userID int64) (int64, error) {
	var bid int64
	err := s.DB.QueryRow(`SELECT current_book_id FROM user WHERE id=?`, userID).Scan(&bid)
	return bid, err
}

// ListBooks 返回用户可访问的账本（带名称、归属）
func (s *Store) ListBooks(userID int64) ([]Book, error) {
	rows, err := s.DB.Query(`
		SELECT b.id, b.name, b.owner_id
		FROM user_book ub JOIN book b ON b.id = ub.book_id
		WHERE ub.user_id=? ORDER BY b.id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Name, &b.OwnerID); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// CreateBook 新建账本，归属当前用户，加入可访问列表并切为当前账本
func (s *Store) CreateBook(userID int64, name string) (*Book, error) {
	res, err := s.DB.Exec(`INSERT INTO book(name, owner_id) VALUES(?,?)`, name, userID)
	if err != nil {
		return nil, err
	}
	bid, _ := res.LastInsertId()
	if _, err := s.DB.Exec(`INSERT OR IGNORE INTO user_book(user_id,book_id) VALUES(?,?)`, userID, bid); err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`UPDATE user SET current_book_id=? WHERE id=?`, bid, userID); err != nil {
		return nil, err
	}
	return &Book{ID: bid, Name: name, OwnerID: userID}, nil
}

// SwitchBook 切换当前账本（必须在用户可访问列表内）
func (s *Store) SwitchBook(userID, bookID int64) error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM user_book WHERE user_id=? AND book_id=?`, userID, bookID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return apperr.Param("无权访问该账本")
	}
	_, err := s.DB.Exec(`UPDATE user SET current_book_id=? WHERE id=?`, bookID, userID)
	return err
}

// Pull：拉取该账本在 since 之后变更的记录（含软删，供客户端同步删除）
func (s *Store) Pull(bookID int64, since string) ([]Record, error) {
	rows, err := s.DB.Query(`
		SELECT r.id, r.book_id, r.user_id, r.type, r.amount, r.category,
		       COALESCE(r.note,''), COALESCE(r.account,''), COALESCE(r.counterparty,''),
		       r.happened_at, r.is_deleted, r.updated_at, COALESCE(u.nickname,'')
		FROM record r LEFT JOIN user u ON u.id = r.user_id
		WHERE r.book_id=? AND r.updated_at > ? ORDER BY r.updated_at ASC`,
		bookID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.BookID, &r.UserID, &r.Type, &r.Amount, &r.Category,
			&r.Note, &r.Account, &r.Counterparty, &r.HappenedAt, &r.IsDeleted, &r.UpdatedAt,
			&r.RecorderName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// JoinBook：邀请码=账本 id。校验账本存在后加入可访问列表，并切为当前账本（不再覆盖原账本）。
func (s *Store) JoinBook(userID, bookID int64) error {
	var exists int
	if err := s.DB.QueryRow(`SELECT 1 FROM book WHERE id=?`, bookID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return apperr.Invite("邀请码无效")
		}
		return err
	}
	if _, err := s.DB.Exec(`INSERT OR IGNORE INTO user_book(user_id,book_id) VALUES(?,?)`, userID, bookID); err != nil {
		return err
	}
	_, err := s.DB.Exec(`UPDATE user SET current_book_id=? WHERE id=?`, bookID, userID)
	return err
}

// ImportRecords：账单导入专用。按 external_id 去重，已存在则跳过。
// 返回 (新增数, 跳过数)。整批一个事务。
func (s *Store) ImportRecords(bookID, userID int64, recs []Record) (int, int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	imported, skipped := 0, 0
	for _, r := range recs {
		if r.ExternalID != "" {
			var n int
			if err := tx.QueryRow(`SELECT COUNT(1) FROM record WHERE book_id=? AND external_id=?`,
				bookID, r.ExternalID).Scan(&n); err != nil {
				return 0, 0, err
			}
			if n > 0 {
				skipped++
				continue
			}
		}
		if _, err := tx.Exec(`INSERT INTO record
			(book_id,user_id,type,amount,category,note,account,counterparty,happened_at,is_deleted,updated_at,external_id)
			VALUES(?,?,?,?,?,?,?,?,?,0,?,?)`,
			bookID, userID, r.Type, r.Amount, r.Category, r.Note, r.Account, r.Counterparty,
			r.HappenedAt, now, r.ExternalID); err != nil {
			return 0, 0, err
		}
		imported++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return imported, skipped, nil
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
