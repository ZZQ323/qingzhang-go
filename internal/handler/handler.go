package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"qingzhang/internal/db"
	"qingzhang/internal/middleware"
	"qingzhang/internal/wx"
)

type Handler struct {
	Store     *db.Store
	JWTSecret []byte
	WxAppID   string
	WxSecret  string
	DevMode   bool // 开发态跳过微信换 openid，用固定 openid 直接签 token
}

// 统一响应包装 {code,msg,data}
type resp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

func writeOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp{Code: 0, Msg: "ok", Data: data})
}
func writeErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp{Code: 1, Msg: msg})
}

// POST /api/auth/login  body: {code}
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		writeErr(w, "缺少 code")
		return
	}
	var openid string
	if h.DevMode {
		// 开发态：用 code 派生固定 openid，免接微信也能联调；不同 code 视为不同用户
		openid = "dev_" + body.Code
	} else {
		sess, err := wx.Code2Session(h.WxAppID, h.WxSecret, body.Code)
		if err != nil {
			writeErr(w, err.Error())
			return
		}
		openid = sess.Openid
	}
	nickname := "用户" + openid[max(0, len(openid)-4):]
	u, err := h.Store.FindOrCreateUser(openid, nickname)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	token, err := middleware.Issue(h.JWTSecret, u.ID)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	writeOK(w, map[string]interface{}{
		"token": token, "userId": u.ID, "nickname": u.Nickname, "bookId": u.BookID,
	})
}

// GET /api/sync/pull?since=YYYY-MM-DD HH:MM:SS
func (h *Handler) Pull(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	bookID, err := h.Store.BookIDOf(uid)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	since := r.URL.Query().Get("since")
	if since == "" {
		since = "1970-01-01 00:00:00"
	}
	recs, err := h.Store.Pull(bookID, since)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	writeOK(w, map[string]interface{}{
		"records":    recs,
		"serverTime": time.Now().UTC().Format("2006-01-02 15:04:05"),
	})
}

// POST /api/sync/push  body: {records:[...]}
func (h *Handler) Push(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	bookID, err := h.Store.BookIDOf(uid)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	var body struct {
		Records []db.Record `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, "请求体解析失败")
		return
	}
	if err := h.Store.Upsert(bookID, uid, body.Records); err != nil {
		writeErr(w, err.Error())
		return
	}
	writeOK(w, map[string]interface{}{
		"serverTime": time.Now().UTC().Format("2006-01-02 15:04:05"),
	})
}

// POST /api/book/join  body: {bookId}  邀请码 = 账本主人的 userId
func (h *Handler) JoinBook(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var body struct {
		BookID int64 `json:"bookId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.BookID <= 0 {
		writeErr(w, "缺少 bookId")
		return
	}
	if err := h.Store.JoinBook(uid, body.BookID); err != nil {
		writeErr(w, err.Error())
		return
	}
	writeOK(w, map[string]interface{}{"bookId": body.BookID})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
