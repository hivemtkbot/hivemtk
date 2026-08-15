# HiveMtk 灾难恢复 (DR) 手册（2026-08-15 M3-P1-E7）

> 私域部署场景下的灾难恢复手册。涵盖：备份策略 / 恢复流程 / RTO / RPO / 演练。

---

## 1. 灾难等级定义

| 等级 | 场景 | RTO | RPO | 影响范围 |
|------|------|-----|-----|---------|
| **L1** | 进程崩溃 / 服务异常 | < 5min | 0 | 单实例 |
| **L2** | 单节点宕机 | < 30min | 0 | 单机 |
| **L3** | 数据库主库故障 | < 30min | 0（同步复制）| 全平台 |
| **L4** | 数据中心故障 | < 4h | < 1min（同步复制）| 全平台 |
| **L5** | 数据损坏 / 误删 | < 24h | 取决于最近备份 | 取决于损坏范围 |
| **L6** | 区域级灾难 | < 24h | < 5min | 全平台 |

---

## 2. 备份策略

### 2.1 备份对象

| 数据 | 频率 | 保留期 | 存储位置 |
|------|------|--------|---------|
| PostgreSQL（pg_dump） | 每日 02:00 | 30 天 | 异地 OSS / S3 |
| PostgreSQL（WAL 归档） | 实时 | 7 天 | 异地 OSS / S3 |
| Redis RDB | 每 6h | 7 天 | 异地 OSS / S3 |
| Redis AOF | 实时 | 24h | 异地 OSS / S3 |
| 上传文件 | 每日 | 永久 | 异地 OSS / S3 |
| 配置文件 | 每次变更 | 永久 | Git 仓库 |
| LLM 模型 | 不备份（重下载） | - | - |
| 审计日志 | 每日 | 1 年 | 异地 OSS / S3 |

### 2.2 自动备份脚本

```bash
#!/bin/bash
# scripts/backup.sh（每日 02:00 cron 执行）
set -euo pipefail

BACKUP_DIR="/var/backups/hivemtk/$(date +%Y%m%d_%H%M%S)"
S3_BUCKET="s3://hivemtk-backups/$(date +%Y/%m/%d)/"
POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
POSTGRES_USER="${POSTGRES_USER:-hivemtk}"
POSTGRES_DB="${POSTGRES_DB:-hivemtk}"

mkdir -p "$BACKUP_DIR"

# 1. PostgreSQL 全量
echo "[$(date)] Backup PostgreSQL..."
pg_dump -h "$POSTGRES_HOST" -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
    -Fc --no-owner --no-acl \
    -f "$BACKUP_DIR/postgres.dump"

# 2. Redis RDB
echo "[$(date)] Backup Redis..."
redis-cli --rdb "$BACKUP_DIR/redis.rdb"

# 3. 上传文件
echo "[$(date)] Backup uploads..."
tar czf "$BACKUP_DIR/uploads.tar.gz" /app/uploads/

# 4. 审计日志
echo "[$(date)] Backup audit logs..."
pg_dump -h "$POSTGRES_HOST" -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
    -t audit_logs -Fc \
    -f "$BACKUP_DIR/audit_logs.dump"

# 5. 上传到 S3
echo "[$(date)] Upload to S3..."
aws s3 sync "$BACKUP_DIR" "$S3_BUCKET" --storage-class STANDARD_IA

# 6. 清理本地旧备份（保留 7 天）
find /var/backups/hivemtk -type d -mtime +7 -exec rm -rf {} +

echo "[$(date)] Backup completed: $BACKUP_DIR"
```

### 2.3 WAL 归档（流复制 + 归档）

```ini
# postgresql.conf
wal_level = replica
archive_mode = on
archive_command = 'aws s3 cp %p s3://hivemtk-backups/wal/%f'
archive_timeout = 60
max_wal_senders = 5
wal_keep_size = '1GB'
```

---

## 3. 恢复流程

### 3.1 L1：进程崩溃（user-server）

**症状**：user-server 进程消失，但节点存活。

**自动恢复**：
```bash
# Docker 自动重启
docker ps -a | grep user-server
docker restart user-server-1
```

**手动恢复**（如自动重启失败）：
```bash
docker logs user-server-1 --tail 50
docker compose up -d user-server-1
```

**RTO**：< 5min
**RPO**：0

### 3.2 L2：单节点宕机

**症状**：物理机 / 虚拟机宕机。

**恢复**：
```bash
# 1. 从 LB 摘除故障节点
# nginx upstream 中注释掉故障节点
# vim /etc/nginx/conf.d/hivemtk.conf
nginx -s reload

# 2. 启动新节点（按 HA 部署指南）
ssh new-node
docker compose up -d

# 3. 等待健康检查通过
until curl -fsS http://new-node:8204/healthz; do sleep 5; done

# 4. 重新加入 LB
# 取消注释，nginx -s reload
```

**RTO**：< 30min
**RPO**：0

### 3.3 L3：PostgreSQL 主库故障

**症状**：PG primary 不可达，sentinel / patroni 自动切换。

**自动切换**（如使用 Patroni）：
```bash
# Patroni 自动选主
patronictl -c /etc/patroni.yml list
```

**手动切换**：
```bash
# 1. 确认主库故障
psql -h pg-primary -U hivemtk -c "SELECT 1"  # 超时

# 2. 提升从库
ssh pg-replica-1
sudo -u postgres pg_ctl promote -D /var/lib/postgresql/15/main

# 3. 验证
psql -h pg-replica-1 -U hivemtk -c "SELECT pg_is_in_recovery()"
# 期望：f（不再是 replica）

# 4. 切换应用配置
vim /etc/hivemtk/user-server.env
# POSTGRES_HOST=pg-replica-1  # 临时
# 或使用 pgpool 重定向

# 5. 重建原主库（修复后）
ssh pg-primary-old
sudo -u postgres pg_ctl initdb -D /var/lib/postgresql/15/main
# 从新主库 basebackup
pg_basebackup -h pg-replica-1 -D /var/lib/postgresql/15/main -U replicator -P -Xs -R
sudo systemctl start postgresql
# 现在是 replica
```

**RTO**：< 30min
**RPO**：0（同步复制）

### 3.4 L4：数据中心故障

**症状**：整个 DC 不可用（断电 / 网络隔离 / 洪水）。

**跨 DC 恢复**：
```bash
# 1. 启动备用 DC（异地灾备）
ssh dc-backup-admin@backup-dc
cd /opt/hivemtk
docker compose up -d

# 2. 切换 DNS 解析到备用 DC
# 减少 TTL 提前（建议 TTL < 300s）
vim /etc/bind/db.hivemtk.example.com
# hivemtk.example.com. 300 IN A backup-dc-public-ip
rndc reload

# 3. 验证服务
curl -fsS https://hivemtk.example.com/healthz
```

**RTO**：< 4h
**RPO**：< 1min（同步复制）

### 3.5 L5：数据损坏 / 误删

**症状**：用户误操作删表 / 应用 bug 导致脏数据。

**PITR（Point-In-Time Recovery）**：
```bash
# 1. 停止应用
docker compose stop user-server

# 2. 恢复最近一次全量备份
pg_restore -h pg-primary -U hivemtk -d hivemtk -c \
    /var/backups/hivemtk/20260815_020000/postgres.dump

# 3. 应用 WAL 归档（恢复到误删之前）
# 设置 recovery.conf
cat > /var/lib/postgresql/15/main/recovery.signal << EOF
recovery_target_time = '2026-08-15 14:30:00'
recovery_target_action = 'promote'
restore_command = 'aws s3 cp s3://hivemtk-backups/wal/%f %p'
EOF

# 4. 启动 PG（进入 recovery 模式）
sudo systemctl start postgresql

# 5. 验证数据
psql -h pg-primary -U hivemtk -c "SELECT count(*) FROM critical_table"

# 6. 启动应用
docker compose start user-server
```

**RTO**：< 24h
**RPO**：取决于发现时间（建议每日全量 + WAL 归档）

### 3.6 L6：区域级灾难

**场景**：地震 / 海啸 / 大规模断电导致整个区域不可用。

**恢复**：
```bash
# 1. 启动异地灾备集群（按 HA 部署指南）
# 2. 恢复最近一次跨区域备份
# 3. 切换 DNS 到新区域
# 4. 通知用户
```

**RTO**：< 24h
**RPO**：< 5min

---

## 4. 演练计划

### 4.1 季度演练

| 季度 | 演练内容 | 负责人 | 验收 |
|------|---------|--------|------|
| Q1 | L1/L2 演练（重启 / 节点宕机） | SRE | RTO < 30min |
| Q2 | L3 演练（PG 主从切换） | DBA | RTO < 30min |
| Q3 | L4 演练（DC 切换） | SRE + 业务 | RTO < 4h |
| Q4 | L5 演练（PITR） | DBA | 数据可恢复到误删前 |

### 4.2 演练脚本

```bash
# scripts/dr-drill.sh（季度演练）
#!/bin/bash
set -euo pipefail

DRILL_TYPE="${1:?usage: $0 <L1|L2|L3|L4|L5>}"

case "$DRILL_TYPE" in
    L1)
        echo "[L1] 重启 user-server 副本"
        docker restart user-server-2
        sleep 30
        curl -fsS http://user-server-2:8204/healthz
        ;;
    L2)
        echo "[L2] 模拟节点宕机（停机 5 分钟后启动）"
        ssh node-2 "sudo shutdown -h +5"
        sleep 300
        ssh node-2 "sudo systemctl start docker"
        ;;
    L3)
        echo "[L3] 模拟 PG primary 故障"
        ssh pg-primary "sudo systemctl stop postgresql"
        sleep 60
        # 验证自动切换
        patronictl -c /etc/patroni.yml list
        ;;
    L4)
        echo "[L4] 启动备用 DC"
        # 人工执行
        echo "请手动执行 DR_L4 流程"
        ;;
    L5)
        echo "[L5] 模拟数据误删 + PITR"
        # 1. 创建测试表
        psql -c "CREATE TABLE drill_test (id int); INSERT INTO drill_test VALUES (1)"
        # 2. 记录当前时间
        date +%Y-%m-%d_%H:%M:%S
        sleep 5
        # 3. 误删
        psql -c "DROP TABLE drill_test"
        # 4. 启动恢复流程（人工确认）
        echo "请手动执行 PITR 恢复"
        ;;
esac
```

---

## 5. 备份验证

```bash
# scripts/verify-backup.sh（每周自动跑）
#!/bin/bash
set -euo pipefail

# 1. 下载最新备份
LATEST=$(aws s3 ls s3://hivemtk-backups/ --recursive | sort | tail -1 | awk '{print $4}')
aws s3 cp "s3://hivemtk-backups/$LATEST" /tmp/verify.dump

# 2. 创建临时数据库
TEST_DB="hivemtk_verify_$(date +%s)"
createdb -h 127.0.0.1 -U hivemtk "$TEST_DB"

# 3. 恢复
pg_restore -h 127.0.0.1 -U hivemtk -d "$TEST_DB" /tmp/verify.dump

# 4. 验证表数
COUNT=$(psql -h 127.0.0.1 -U hivemtk -d "$TEST_DB" -tAc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")
echo "Tables: $COUNT"
if [ "$COUNT" -lt 50 ]; then
    echo "ERROR: backup seems incomplete"
    exit 1
fi

# 5. 清理
dropdb -h 127.0.0.1 -U hivemtk "$TEST_DB"
rm /tmp/verify.dump

echo "Backup verification OK"
```

---

## 6. 联系人 / 升级路径

| 等级 | 第一响应 | 升级 | 决策方 |
|------|---------|------|--------|
| L1-L2 | SRE on-call | - | - |
| L3 | DBA | SRE Lead | - |
| L4-L6 | SRE Lead | 技术 VP | CEO / 客户方 |

---

## 7. 文档维护

- 每次演练后更新 RTO / RPO 实测数据
- 每次架构变更后更新恢复流程
- 每季度 review 一次（与 SRE 周会同步）

---

> 配套：[高可用部署](HA_DEPLOYMENT.md) · [SLA/SLO](SLA_SLO.md) · [等级保护合规](等保2.0三级合规.md)
