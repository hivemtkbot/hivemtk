# HiveMtk 部署指南

> **版本**：v3.20.0（2026-08-16）
> **适用场景**：单机部署 / FRP 私域 / 反向代理

---

## 目录

1. [硬件需求](#一硬件需求)
2. [端口分配](#二端口分配)
3. [依赖服务](#三依赖服务)
4. [部署模式 A：单机部署](#四部署模式-a单机部署)
5. [部署模式 B：FRP 私域部署](#五部署模式-bfrp-私域部署)
6. [部署模式 C：反向代理部署](#六部署模式-c反向代理部署)
7. [备份与恢复](#七备份与恢复)
8. [常见问题](#八常见问题)

---

## 一、硬件需求

| 资源 | 最低配置 | 推荐配置 |
|------|---------|---------|
| CPU | 4 核 | 8 核 (Intel/AMD x86_64) |
| 内存 | 8 GB | 16 GB |
| 存储 | 100 GB SSD | 500 GB SSD |
| 网络 | 100 Mbps | 1 Gbps |
| GPU | 可选 | 可选 (推理加速 5-10x) |

## 二、端口分配

| 服务 | 端口 | 说明 |
|------|------|------|
| user-server | **8204** | 主 API（前端 + 后端）|
| user-server WS | 8205 | 客服 WebSocket |
| PostgreSQL | 5432 | 数据库 |
| Redis | 6379 | 缓存 |
| llama-server LLM | 8207 | 主对话模型 |
| llama-server Embedding | 8208 | 向量化 |

## 三、依赖服务

### 必装

| 服务 | 最低版本 | 用途 |
|------|----------|------|
| **PostgreSQL** | 14+ | 主数据库 |
| **Redis** | 6.2+ | 缓存 / 会话 |

### 可选

| 服务 | 用途 |
|------|------|
| **pgvector** | 知识库向量检索 |
| **MinIO** | 私有对象存储 |
| **llama.cpp** | 本地 LLM 推理（推荐）|
| **frpc/frps** | 内网穿透 |

### 系统包

```bash
# Ubuntu 22.04 / Debian 12
apt-get update && apt-get install -y \
  curl wget git make \
  postgresql-client redis-tools \
  nginx certbot python3-certbot-nginx
```

---

## 四、部署模式 A：单机部署

### 4.1 架构

```
┌──────────────────────────────────────┐
│   物理机 / 虚拟机 / WSL2             │
│                                      │
│  ┌────────────────────────────┐     │
│  │ user-server (8204)         │     │
│  └────────────┬───────────────┘     │
│               │                      │
│  ┌────────────┴───────────────┐     │
│  │ PostgreSQL (5432)          │     │
│  │ Redis (6379)               │     │
│  └────────────────────────────┘     │
│                                      │
│  ┌────────────────────────────┐     │
│  │ llama-server (8207-8208)   │     │
│  └────────────────────────────┘     │
└──────────────────────────────────────┘
```

### 4.2 部署步骤

```bash
# 1. 克隆代码
git clone https://github.com/your-org/hivemtk.git
cd hivemtk

# 2. 启动数据库
docker compose up -d postgres redis

# 3. 初始化数据库
PGPASSWORD=postgres psql -h 127.0.0.1 -U postgres -d hivemtk \
  -f migrations/init-db.sql

# 4. 启动后端
cd user-server
go build -o bin/user-server ./cmd/api
./bin/user-server -config config.yaml &

# 5. 启动 LLM
cd ../scripts/inference-host
./start-all.sh

# 6. 验证
curl http://localhost:8204/healthz
# 预期：{"status":"ok"}
```

---

## 五、部署模式 B：FRP 私域部署

> **完整指南**：[architecture/FRP私域部署指南.md](architecture/FRP私域部署指南.md)

### 5.1 架构

```
   访客（公网）                云端 VPS                内网（私域）
                                     ┌──────────┐
访客 →  https://chat.example.com    │  frps    │:7000
                                     │  :443    │
                                     └─────┬────┘
                                           │ (FRP 隧道)
                                  ┌────────┴────────┐
                                  │  frpc           │
                                  │  (内网机器)      │
                                  └────────┬────────┘
                                           │
       ┌─────────────────────────┐         │
       │ user-server :8204       │ ←───────┘
       │ PostgreSQL :5432 (本地) │
       └─────────────────────────┘
```

### 5.2 步骤

```bash
# 1. 云端 VPS 配置 frps
# /etc/frp/frps.ini
[common]
bind_port = 7000
vhost_https_port = 443
vhost_http_port = 80

systemctl start frps

# 2. 内网机器配置 frpc
# frpc.toml
serverAddr = "your-vps-ip"
serverPort = 7000

[[proxies]]
name = "user-server"
type = "http"
localPort = 8204
customDomains = ["chat.example.com"]

systemctl start frpc
```

---

## 六、部署模式 C：反向代理部署

### 6.1 Nginx 配置

```nginx
# /etc/nginx/sites-available/hivemtk
upstream user_server {
  server 127.0.0.1:8204;
}

server {
  listen 80;
  server_name chat.example.com;
  return 301 https://$host$request_uri;
}

server {
  listen 443 ssl http2;
  server_name chat.example.com;

  ssl_certificate /etc/letsencrypt/live/chat.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/chat.example.com/privkey.pem;

  # WebSocket
  location /ws/ {
    proxy_pass http://user_server;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_read_timeout 86400;
  }

  # API
  location / {
    proxy_pass http://user_server;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
  }
}
```

### 6.2 SSL 证书

```bash
apt install certbot python3-certbot-nginx
certbot --nginx -d chat.example.com
```

---

## 七、备份与恢复

### 7.1 备份策略

| 数据 | 频率 | 保留 | 工具 |
|------|------|------|------|
| PostgreSQL | 每日 02:00 | 7 天 | pg_dump |
| Redis | 每日 03:00 | 7 天 | RDB |

### 7.2 备份脚本

```bash
#!/bin/bash
# /opt/scripts/backup.sh

BACKUP_DIR=/var/backups/hivemtk/$(date +%Y%m%d)
mkdir -p $BACKUP_DIR

# PostgreSQL
pg_dump -U postgres -d hivemtk | gzip > $BACKUP_DIR/postgres.sql.gz

# Redis
redis-cli BGSAVE
cp /var/lib/redis/dump.rdb $BACKUP_DIR/

# 清理 7 天前的备份
find /var/backups/hivemtk/ -mtime +7 -delete
```

### 7.3 恢复

```bash
# PostgreSQL 恢复
gunzip -c backup.sql.gz | psql -U postgres -d hivemtk

# Redis 恢复
systemctl stop redis
cp backup/dump.rdb /var/lib/redis/
systemctl start redis
```

---

## 八、常见问题

### Q1: 前端白屏？

```yaml
# config.yaml
static:
  enabled: true
  user_web_path: /opt/hivemtk/user-web/dist
```

### Q2: AI 推理报 "model not found"？

```bash
./llama-server -m /models/qwen2.5-7b.gguf --port 8207
```

### Q3: WebSocket 频繁断连？

```nginx
proxy_read_timeout 86400;
proxy_send_timeout 86400;
```

### Q4: 数据库连接耗尽？

```yaml
# config.yaml
database:
  max_open_conns: 50
  max_idle_conns: 10
```

---

## 检查清单

- [ ] 核心端口（8204/5432/6379）可达
- [ ] `/healthz` 返回 200
- [ ] 数据库 migration 执行完成
- [ ] 备份脚本测试通过

---

## 参考链接

- [FRP 私域部署](architecture/FRP私域部署指南.md)
- [灾难恢复](operations/DR_RECOVERY.md)
- [高可用部署](operations/HA_DEPLOYMENT.md)
