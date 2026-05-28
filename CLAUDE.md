# 开发规范（清账后端）

## 沟通规范
- **必须使用中文进行思考、回答。** 所有思考过程、回复、提交说明、代码注释一律用中文。

## 工程约定
- 后端：Go + 标准库 `net/http`（Go 1.22 方法路由，无框架）+ SQLite（纯 Go 驱动 modernc.org/sqlite）。
- 统一响应：`{code,msg,data}`，code=0 成功；业务错误用 `internal/apperr` 的错误码。
- 错误处理：业务层抛 `*apperr.Error`，出口 `handler.writeErr` 统一翻译；未归类错误兜底为 internal。
- 密钥（WX_APPID/WX_SECRET/JWT_SECRET）只放服务器 systemd 环境变量，绝不入仓库/CI。
- 部署：push 到 main → GitHub Actions 编译静态二进制 → 传服务器 → 原子替换 → 重启 systemd。
