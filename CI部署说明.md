# 后端自动部署配置说明

推到 `main` 分支后，GitHub Actions 自动编译静态二进制 → scp 到服务器 → 原子替换 → 重启 systemd。

## 一、放入仓库的文件

```
后端仓库根目录/
├── .github/workflows/deploy.yml   # 流水线
├── .gitignore                     # 排除密钥/数据库/二进制
├── main.go  go.mod  go.sum  internal/...
```

## 二、要在 GitHub 填的 Secrets

仓库页面 → Settings → Secrets and variables → Actions → New repository secret，逐个添加：

| Secret 名 | 值 | 说明 |
|---|---|---|
| `SERVER_HOST` | 你服务器的 IP 或域名 | |
| `SERVER_USER` | 部署用的 Linux 用户 | 建议专用账号，非 root |
| `SERVER_SSH_KEY` | SSH **私钥**全文 | 见下方生成方法 |
| `SERVER_PATH` | 部署目录，如 `/opt/qingzhang` | 二进制与 _staging 都在此目录下 |

> SSH 端口固定走 22（deploy.yml 里写死）。要改端口就在 deploy.yml 把 `port: 22` 改掉或新增 Secret。

注意：`WX_SECRET`、`JWT_SECRET`、`WX_APPID` 这些**不放这里**——它们是运行时配置，写在服务器的 systemd 文件里（见下），不经过 CI。CI 只负责传二进制 + 重启，碰不到业务密钥。

## 三、SSH 密钥生成（一次性）

在本机生成一对**专用于部署**的密钥（别用你日常登录的那把）：

```bash
ssh-keygen -t ed25519 -C "github-deploy-qingzhang" -f ~/.ssh/qz_deploy
```

- 公钥 `~/.ssh/qz_deploy.pub` 内容追加到服务器的 `~/.ssh/authorized_keys`（对应 `SSH_USER` 那个账号）
- 私钥 `~/.ssh/qz_deploy` 的**全部内容**（含 `-----BEGIN/END-----` 行）粘贴到 GitHub 的 `SSH_KEY` Secret

## 四、服务器一次性准备

```bash
# 1. 部署目录
sudo mkdir -p /opt/qingzhang/_staging
sudo chown -R $SSH_USER:$SSH_USER /opt/qingzhang

# 2. systemd 服务文件（业务密钥写在这里，不进 git/CI）
sudo tee /etc/systemd/system/qingzhang.service >/dev/null <<'EOF'
[Unit]
Description=Qingzhang Ledger (Go)
After=network.target

[Service]
User=部署用户名
WorkingDirectory=/opt/qingzhang
ExecStart=/opt/qingzhang/qingzhang
Environment=PORT=8080
Environment=DB_PATH=file:/opt/qingzhang/qingzhang.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)
Environment=JWT_SECRET=换成32字节以上强随机串
Environment=WX_APPID=你的小程序appid
Environment=WX_SECRET=你的小程序secret
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now qingzhang
```

## 五、让部署用户能免密重启服务

deploy.yml 里用了 `sudo systemctl restart`。给部署账号一条**仅限该命令**的免密 sudo（最小授权，别给全量 sudo）：

```bash
echo '部署用户名 ALL=(ALL) NOPASSWD: /bin/systemctl restart qingzhang, /bin/systemctl is-active qingzhang' \
  | sudo tee /etc/sudoers.d/qingzhang
sudo chmod 440 /etc/sudoers.d/qingzhang
```

## 六、验证

1. 改点代码，推到 `main`
2. GitHub 仓库 Actions 页看流水线绿灯
3. 服务器 `systemctl status qingzhang` 确认在新进程上运行
4. `curl https://你的域名/api/sync/pull` 返回 401（没带 token，符合预期）即通

## 安全小结

- 密钥分两类：**部署密钥**（SSH）放 GitHub Secrets；**业务密钥**（JWT/微信）放服务器 systemd。两者都不进 git。
- 部署账号用专用密钥 + 最小 sudo 授权，泄露影响面可控。
- 仓库即使私有，也靠 `.gitignore` 确保 `.env`/`*.db`/二进制不会被误提交。
- 想日后转公开：先用 `gitleaks detect` 或 GitHub secret scanning 扫一遍历史，确认无残留密钥再转。
