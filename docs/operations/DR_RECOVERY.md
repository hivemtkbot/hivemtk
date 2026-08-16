# HiveMtk 灾难恢复指南

> 单商户本地部署的备份与恢复方案。

---

## 1. 备份策略

### 1.1 备份频率

| 类型 | 频率 | 保留期 | 说明 |
|------|------|--------|------|
| 全量备份 | 每日 02:00 | 7 天 | 所有数据完整备份 |
| 增量备份 | 每小时 | 24 小时 | 仅备份变化的数据 |
| 日志归档 | 每日 03:00 | 30 天 | WAL 日志归档 |

### 1.2 备份内容

| 组件 | 备份方式 | 存储位置 |
|------|---------|---------|
| PostgreSQL | pg_dump / pg_basebackup | `/var/backups/hivemtk/` |
| Redis | RDB 快照 | `/var/backups/hivemtk/` |
| 上传文件 | tar + gzip | `/var/backups/hivemtk/` |
| 配置文件 | 直接复制 | `/var/backups/hivemtk/config/` |

### 1.3 备份脚本

```bash
#!/bin/bash
# /opt/hivemtk/scripts/backup.sh

set -euo pipefail

BACKUP_ROOT="/var/backups/hivemtk"
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="$BACKUP_ROOT/$DATE"

echo "[$(date)] Starting backup to $BACKUP_DIR"

mkdir -p "$BACKUP_DIR"

# PostgreSQL 全量备份
echo "Backing up PostgreSQL..."
docker compose -f /opt/hivemtk/docker-compose.yml exec -T postgres \
  pg_dump -U hivemtk -d hivemtk -Fc > "$BACKUP_DIR/postgres.dump"

# Redis 备份
echo "Backing up Redis..."
docker compose -f /opt/hivemtk/docker-compose.yml exec -T redis \
  redis-cli --rdb /tmp/dump.rdb
docker compose -f /opt/hivemtk/docker-compose.yml cp \
  redis:/tmp/dump.rdb "$BACKUP_DIR/redis.rdb"

# 上传文件备份
echo "Backing up uploads..."
tar czf "$BACKUP_DIR/uploads.tar.gz" /opt/hivemtk/uploads/ 2>/dev/null || true

# 配置文件备份
echo "Backing up config..."
cp /opt/hivemtk/.env "$BACKUP_DIR/"
cp /opt/hivemtk/docker-compose.yml "$BACKUP_DIR/"

# 生成校验
md5sum "$BACKUP_DIR"/* > "$BACKUP_DIR/checksums.md5"

# 清理超过 7 天的备份
find "$BACKUP_ROOT" -maxdepth 1 -type d -mtime +7 -exec rm -rf {} +

echo "[$(date)] Backup completed: $BACKUP_DIR"
echo "[$(date)] Backup size: $(du -sh "$BACKUP_DIR" | cut -f1)"
```

### 1.4 定时任务

```cron
# /etc/cron.d/hivemtk-backup
0 2 * * * root /opt/hivemtk/scripts/backup.sh >> /var/log/hivemtk-backup.log 2>&1
```

## 2. 恢复流程

### 2.1 恢复步骤

```bash
# 1. 停止服务
cd /opt/hivemtk
docker compose down

# 2. 选择要恢复的备份
BACKUP_DATE="20260815_020000"  # 改为实际日期

# 3. 恢复 PostgreSQL
echo "Restoring PostgreSQL from $BACKUP_DATE..."
cat "/var/backups/hivemtk/$BACKUP_DATE/postgres.dump" | \
  docker compose exec -T postgres pg_restore -U hivemtk -d hivemtk

# 4. 恢复 Redis
echo "Restoring Redis..."
docker compose cp "/var/backups/hivemtk/$BACKUP_DATE/redis.rdb" redis:/tmp/dump.rdb

# 5. 恢复上传文件
echo "Restoring uploads..."
tar xzf "/var/backups/hivemtk/$BACKUP_DATE/uploads.tar.gz" -C /

# 6. 启动服务
echo "Starting services..."
docker compose up -d

# 7. 验证恢复
echo "Verifying restoration..."
sleep 10
curl -s http://localhost:8204/healthz
docker compose exec -T postgres pg_isready -U hivemtk
docker compose exec -T redis redis-cli ping

echo "Restoration completed from $BACKUP_DATE"
```

### 2.2 恢复验证

```bash
# 检查数据库完整性
docker compose exec -T postgres psql -U hivemtk -c "SELECT count(*) FROM users;"

# 检查功能完整性
curl -s http://localhost:8204/api/v1/teams | python3 -m json.tool

# 检查日志
tail -f /var/log/hivemtk/app.log
```

## 3. 故障场景处理

### 3.1 主机故障

```
1. 在新主机上安装依赖
2. 克隆项目代码
3. 复制备份文件
4. 执行恢复流程
5. 启动服务
6. 验证功能
```

### 3.2 数据误删

```
1. 停止应用写入
2. 从最近备份恢复到临时数据库
3. 导出丢失的数据
4. 合回生产数据库
5. 验证数据完整性
```

### 3.3 数据库损坏

```
1. 停止服务
2. 使用 pg_resetwal 修复
3. 从备份恢复
4. 启动服务
5. 验证数据完整性
```

## 4. 备份验证

### 4.1 每周验证

```bash
#!/bin/bash
# /opt/hivemtk/scripts/verify-backup.sh
# 每周执行一次

set -euo pipefail

BACKUP_ROOT="/var/backups/hivemtk"
LATEST=$(ls -td "$BACKUP_ROOT"/*/ | head -1)

echo "Verifying backup: $LATEST"

# 恢复到临时数据库
docker compose -f /opt/hivemtk/docker-compose.yml up -d postgres

# 等待 PostgreSQL 就绪
sleep 10

# 尝试恢复
cat "$LATEST/postgres.dump" | \
  docker compose -f /opt/hivemtk/docker-compose.yml exec -T postgres \
  pg_restore -U hivemtk -d hivemtk_clean 2>&1 || true

# 检查数据量
COUNT=$(docker compose -f /opt/hivemtk/docker-compose.yml exec -T postgres \
  psql -U hivemtk -t -c "SELECT count(*) FROM users;")

echo "Users count after restore: $COUNT"

if [ "$COUNT" -gt "0" ]; then
  echo "✅ Backup verification PASSED"
else
  echo "❌ Backup verification FAILED"
  exit 1
fi

# 清理
docker compose -f /opt/hivemtk/docker-compose.yml exec -T postgres \
  psql -U hivemtk -c "DROP DATABASE IF EXISTS hivemtk_clean;"

echo "[$(date)] Backup verification completed"
```

### 4.2 定时任务

```cron
# 每周日 03:00 执行
0 3 * * 0 root /opt/hivemtk/scripts/verify-backup.sh >> /var/log/hivemtk-verify.log 2>&1
```

---

*最后更新: 2026-08-16*
