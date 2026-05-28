package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"qingzhang/internal/apperr"
	"qingzhang/internal/bill"
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
// writeErr 统一错误出口：业务错误(*apperr.Error)按其 code/msg 返回；
// 其余未归类错误一律包成 CodeInternal，避免把底层细节暴露成裸字符串。
func writeErr(w http.ResponseWriter, err error) {
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		ae = apperr.Internal(err.Error())
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp{Code: ae.Code, Msg: ae.Msg})
}

// POST /api/auth/login  body: {code}
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		writeErr(w, apperr.Param("缺少登录 code"))
		return
	}
	var openid string
	if h.DevMode {
		// 开发态：用 code 派生固定 openid，免接微信也能联调；不同 code 视为不同用户
		openid = "dev_" + body.Code
	} else {
		sess, err := wx.Code2Session(h.WxAppID, h.WxSecret, body.Code)
		if err != nil {
			writeErr(w, apperr.WxLogin("微信登录失败："+err.Error()))
			return
		}
		openid = sess.Openid
	}
	nickname := "用户" + openid[max(0, len(openid)-4):]
	u, err := h.Store.FindOrCreateUser(openid, nickname)
	if err != nil {
		writeErr(w, err)
		return
	}
	token, err := middleware.Issue(h.JWTSecret, u.ID)
	if err != nil {
		writeErr(w, err)
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
		writeErr(w, err)
		return
	}
	since := r.URL.Query().Get("since")
	if since == "" {
		since = "1970-01-01 00:00:00"
	}
	recs, err := h.Store.Pull(bookID, since)
	if err != nil {
		writeErr(w, err)
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
		writeErr(w, err)
		return
	}
	var body struct {
		Records []db.Record `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, apperr.Param("请求体解析失败"))
		return
	}
	if err := h.Store.Upsert(bookID, uid, body.Records); err != nil {
		writeErr(w, err)
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
		writeErr(w, apperr.Param("缺少邀请码"))
		return
	}
	if err := h.Store.JoinBook(uid, body.BookID); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]interface{}{"bookId": body.BookID})
}

// POST /api/import  body: {source:"wx"|"alipay", content:"<CSV原文>"}
// 登录后使用（走 Auth 中间件）。解析账单 -> 按交易单号去重入库。
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	bookID, err := h.Store.BookIDOf(uid)
	if err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Source  string `json:"source"`
		Content string `json:"content"` // 文件原始字节的 base64（前端 readFile base64 直传，保留 GBK）
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
		writeErr(w, apperr.Param("缺少账单内容"))
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Content)
	if err != nil {
		writeErr(w, apperr.Param("账单内容需为 base64"))
		return
	}
	recs, err := bill.Parse(body.Source, data)
	if err != nil {
		writeErr(w, err)
		return
	}
	imported, skipped, err := h.Store.ImportRecords(bookID, uid, recs)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]interface{}{
		"imported": imported, // 新增条数
		"skipped":  skipped,  // 重复跳过条数
		"parsed":   len(recs),
	})
}

// GET /api/books  我的账本列表（带当前账本标记）
func (h *Handler) ListBooks(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	books, err := h.Store.ListBooks(uid)
	if err != nil {
		writeErr(w, err)
		return
	}
	cur, _ := h.Store.BookIDOf(uid)
	writeOK(w, map[string]interface{}{"books": books, "currentBookId": cur})
}

// POST /api/books  {name}  新建账本并切为当前
func (h *Handler) CreateBook(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeErr(w, apperr.Param("请输入账本名称"))
		return
	}
	b, err := h.Store.CreateBook(uid, strings.TrimSpace(body.Name))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, b)
}

// POST /api/books/switch  {bookId}  切换当前账本
func (h *Handler) SwitchBook(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var body struct {
		BookID int64 `json:"bookId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.BookID <= 0 {
		writeErr(w, apperr.Param("缺少 bookId"))
		return
	}
	if err := h.Store.SwitchBook(uid, body.BookID); err != nil {
		writeErr(w, err)
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
