#!/usr/bin/env bash
# 在你本机/CI 上交叉编译出 Linux 静态二进制，扔到服务器即可，服务器无需装任何东西
set -e
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o qingzhang .
echo "产物: ./qingzhang  （上传到 /opt/qingzhang/ 并 systemctl restart qingzhang）"
