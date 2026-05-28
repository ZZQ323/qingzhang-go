package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"qingzhang/internal/apperr"
)

// 401 也返回统一 {code,msg}，前端能拿到「登录失效」提示，而非空响应
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": apperr.CodeUnauthorized,
		"msg":  "登录已失效，请重新登录",
	})
}

type ctxKey string

const UserIDKey ctxKey = "uid"

// Issue 签发 30 天有效的 JWT
func Issue(secret []byte, userID int64) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// Auth 返回一个中间件：校验 Authorization: Bearer <token>，注入 userID 到 context
func Auth(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				writeUnauthorized(w)
				return
			}
			tokenStr := strings.TrimPrefix(auth, "Bearer ")
			claims := &jwt.RegisteredClaims{}
			tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				return secret, nil
			})
			if err != nil || !tok.Valid {
				writeUnauthorized(w)
				return
			}
			uid, _ := strconv.ParseInt(claims.Subject, 10, 64)
			ctx := context.WithValue(r.Context(), UserIDKey, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserID(r *http.Request) int64 {
	if v, ok := r.Context().Value(UserIDKey).(int64); ok {
		return v
	}
	return 0
}
