package main

import (
	"log"
	"net/http"
	"os"
	"qingzhang/internal/db"
	"qingzhang/internal/handler"
	"qingzhang/internal/middleware"
)

func main() {
	// dsn - Data Source Name
	dsn := env("DB_PATH", "file:qingzhang.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	db, err := db.Open(dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	defer db.Close()
	if err := db.Migrate(); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	h := &handler.Handler{
		Store:     db,
		JWTSecret: []byte(env("JWT_SECRET", "change_me_to_a_long_random_secret_32b+")),
		WxAppID:   env("WX_APPID", ""),
		WxSecret:  env("WX_SECRET", ""),
	}

	mux := http.NewServeMux()
	// 公开接口
	mux.HandleFunc("POST /api/auth/login", h.Login)
	// 鉴权接口（middleware.Auth 包裹）
	auth := middleware.Auth(h.JWTSecret)
	mux.Handle("GET /api/sync/pull", auth(http.HandlerFunc(h.Pull)))
	mux.Handle("POST /api/sync/push", auth(http.HandlerFunc(h.Push)))

	addr := ":" + env("PORT", "8080")
	log.Printf("qingzhang listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// 读取环境变量的，并且在环境变量不存在或为空时，提供一个默认值
func env(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
