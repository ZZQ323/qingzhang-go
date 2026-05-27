#!/usr/bin/env bash
# 每日备份 SQLite 数据库。WAL 模式下用 .backup 命令做一致性快照（比 cp 安全）。
# 用法：bash deploy/backup.sh        （或丢进 crontab，见文件末尾）
set -euo pipefail

DB="${DB_FILE:-/opt/qingzhang/qingzhang.db}"
DEST="${BACKUP_DIR:-/opt/qingzhang/backup}"
KEEP_DAYS="${KEEP_DAYS:-14}"   # 保留天数，超期自动清理

mkdir -p "$DEST"
STAMP="$(date +%F_%H%M%S)"
OUT="$DEST/qingzhang-$STAMP.db"

# .backup 会处理 WAL，产出单文件一致快照
sqlite3 "$DB" ".backup '$OUT'"
gzip -f "$OUT"
echo "[backup] $OUT.gz"

# 清理过期备份
find "$DEST" -name 'qingzhang-*.db.gz' -type f -mtime "+$KEEP_DAYS" -delete

# ── 配置每天 03:30 自动备份（在服务器上执行一次）──
#   crontab -e
#   30 3 * * * /usr/bin/bash /opt/qingzhang/deploy/backup.sh >> /var/log/qingzhang-backup.log 2>&1
