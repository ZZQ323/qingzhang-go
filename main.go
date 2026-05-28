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
		DevMode:   env("DEV_MODE", "") != "",
	}
	if h.DevMode {
		log.Print("DEV_MODE 已开启：登录将跳过微信换 openid，用 code 派生 openid 直接签发 token")
	} else if h.WxAppID == "" || h.WxSecret == "" {
		// 生产态必须接入真实小程序凭证，否则 wx.login 换 openid 无从谈起
		log.Fatal("未配置 WX_APPID/WX_SECRET：生产态需设这两个环境变量，本地联调可设 DEV_MODE=1")
	}

	mux := http.NewServeMux()
	// 公开接口
	mux.HandleFunc("POST /api/auth/login", h.Login)
	// 鉴权接口（middleware.Auth 包裹）
	auth := middleware.Auth(h.JWTSecret)
	mux.Handle("GET /api/sync/pull", auth(http.HandlerFunc(h.Pull)))
	mux.Handle("POST /api/sync/push", auth(http.HandlerFunc(h.Push)))
	mux.Handle("POST /api/book/join", auth(http.HandlerFunc(h.JoinBook)))
	mux.Handle("POST /api/import", auth(http.HandlerFunc(h.Import)))

	addr := ":" + env("PORT", "8080")
	log.Printf("qingzhang listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func env(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
