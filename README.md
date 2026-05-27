# 清账 · Go + SQLite 极简记账后端

项目前端：https://github.com/ZZQ323/qingzhang-miniapp

## 项目启动

如果是没有注册小程序，那么设置环境变量：
- DEV_MODE=1
- JWT_SECRET=本地随便一串至少32字节xxxxxxxxxx 

然后
```bash
go mod tidy          # 首次：联网拉 modernc.org/sqlite，生成 go.sum
go run .
```

没有注册小程序可以测试：

```bash
# 登录拿 token
curl -s -X POST localhost:8080/api/auth/login -d "{\"code\":\"test1\"}"
# 用返回的 token 拉/推
curl -s localhost:8080/api/sync/pull -H "Authorization: Bearer <token>"
```

如果注册了小程序：
```bash
cd qingzhang-go
set WX_APPID=你的appid           # Windows cmd
set WX_SECRET=你的secret
set JWT_SECRET=本地32字节随机串
go run .
```

`JWT_SECRET` 可以使用 `openssl` 生成 `openssl rand -base64 64`。

## 项目部署


```bash
sudo tee /etc/systemd/system/qingzhang.service >/dev/null <<'EOF'
[Unit]
Description=Qingzhang Ledger (Go)
After=network.target

[Service]
User=root
WorkingDirectory=/opt/qingzhang
ExecStart=/opt/qingzhang/qingzhang
Environment=PORT=8080
Environment=DB_PATH=file:/opt/qingzhang/qingzhang.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)
Environment=JWT_SECRET=换成真实的32字节随机串
Environment=WX_APPID=你的appid
Environment=WX_SECRET=你的secret
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
# 等上传代码后再
#部署成功后第一次要 enable
sudo systemctl enable --now qingzhang
sudo systemctl status qingzhang
```

## 项目背景

为 2 核 2G 服务器设计，单个静态二进制 + 一个 SQLite 文件，常驻内存约 50–80MB。零中间件，与你机器上已有的 VitePress 博客（GitHub 构建，静态托管）、frpc 共存毫无压力。

## 架构

```
小程序(uniapp) --HTTPS--> Nginx(443) --> qingzhang 二进制(:8080) --> qingzhang.db (SQLite, WAL)
```

只有两个新进程：nginx（你本来就有，还托管着博客）+ qingzhang 二进制。数据库不是独立进程，就是一个文件。

## 关于 SQLite 并发（你问到的）

SQLite 写操作串行：同一时刻只允许一个写，但 WAL 模式下读写可并行。对 2–3 用户的记账场景毫无压力。

代码里做了两层保险：
1. 连接串开 WAL + busy_timeout：`journal_mode(WAL)`、`busy_timeout(5000)`
2. `db.SetMaxOpenConns(1)`：把 Go 连接池设为 1，从应用层就让写串行，根除 `database is busy`。

驱动用 `modernc.org/sqlite`（纯 Go，无 cgo），所以可以 `CGO_ENABLED=0` 交叉编译出零依赖静态二进制，服务器上什么都不用装。

## 目录

```
.
├── main.go                      # 入口，net/http ServeMux（Go 1.22 方法路由，无框架）
├── go.mod
├── internal/
│   ├── db/      db.go           # 连接、WAL、单写连接、建表迁移
│   │           dao.go          # User/Record 模型 + Pull/Upsert/FindOrCreateUser
│   ├── middleware/ auth.go      # JWT 签发与校验中间件
│   ├── wx/      wx.go           # code 换 openid
│   └── handler/ handler.go      # login / pull / push 三个接口
├── miniapp/                     # 前端（uniapp，接口契约与后端一致，无需改）
└── deploy/  build.sh nginx.conf qingzhang.service
```

## API（与原 Spring 版完全一致，前端不用改）

| 方法 | 路径 | 说明 | 鉴权 |
|---|---|---|---|
| POST | /api/auth/login | body {code}，返回 {token,userId,nickname,bookId} | 否 |
| GET  | /api/sync/pull?since= | 拉 since 之后的变更；不传则全量 | 是 |
| POST | /api/sync/push | body {records:[...]}，upsert（含软删） | 是 |

响应统一 {code,msg,data}，code=0 成功。鉴权 `Authorization: Bearer <token>`。

## 跑起来

本地开发（会自动建 qingzhang.db）：
```bash
export WX_APPID=xxx WX_SECRET=xxx JWT_SECRET=至少32字节随机串
go run .
```

部署到服务器：
```bash
# 1. 本机/CI 交叉编译（不依赖服务器环境）
bash deploy/build.sh                    # 产出 ./qingzhang

# 2. 上传二进制到服务器
scp qingzhang user@server:/opt/qingzhang/

# 3. 配 systemd（填好环境变量），启动
sudo cp deploy/qingzhang.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now qingzhang

# 4. nginx 反代 + HTTPS（小程序强制），证书用 Let's Encrypt
sudo cp deploy/nginx.conf /etc/nginx/conf.d/qingzhang.conf
sudo nginx -t && sudo systemctl reload nginx
```

## 备份

数据全在 `qingzhang.db` 一个文件。已提供 `deploy/backup.sh`（WAL 安全的 `.backup` 快照 + gzip + 自动清理过期）：
```bash
bash deploy/backup.sh                    # 手动备份一次
# 每天 03:30 自动备份，crontab -e 加：
# 30 3 * * * /usr/bin/bash /opt/qingzhang/deploy/backup.sh >> /var/log/qingzhang-backup.log 2>&1
```
可用 `DB_FILE` / `BACKUP_DIR` / `KEEP_DAYS` 环境变量覆盖路径与保留天数。

## 注意

- 本沙箱无法访问 modernc.org，未能在此处替你跑通最终二进制；但全部业务代码已通过 Go 编译类型检查（db/wx/middleware/handler/main 均无报错），只有 sqlite 驱动那行空导入需你在本机首次 `go build` 时联网拉取。
- 首次 build 若提示校验，执行 `go mod tidy` 生成 go.sum。
- JWT_SECRET 必须换成强随机串；SQLite 文件权限设好，别让 nginx 能直接读。
- 共享账本（和朋友共记一本）：把对方 user 的 book_id 改成同一个值即可，record 表已按 book_id 隔离。
