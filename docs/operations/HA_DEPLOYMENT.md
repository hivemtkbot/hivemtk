# HiveMtk 高可用 (HA) 部署指南（2026-08-15 M3-P1-E6）

> 私域部署单租户场景下的高可用部署方案。涵盖：多副本部署 / 负载均衡 / 健康检查 / 自动故障转移 / 数据零丢失 / 滚动升级。

---

## 1. HA 架构总览

```
                    ┌──────────────────┐
                    │  Nginx / HAProxy │  ← L4/L7 LB（4 副本，最低配置）
                    │  (Keepalived VIP)│
                    └────────┬─────────┘
                             │
            ┌────────────────┼────────────────┐
            │                │                │
     ┌──────▼─────┐  ┌──────▼─────┐  ┌──────▼─────┐
     │ user-server│  │ user-server│  │ user-server│  ← 3 副本（最低）
     │  实例 1    │  │  实例 2    │  │  实例 3    │
     └──────┬─────┘  └──────┬─────┘  └──────┬─────┘
            │                │                │
            └────────────────┼────────────────┘
                             │
       ┌─────────────────────┼─────────────────────┐
       │                     │                     │
  ┌────▼────┐          ┌─────▼────┐          ┌─────▼────┐
  │PostgreSQL│         │PostgreSQL│         │PostgreSQL│  ← 主从流复制
  │  Primary │         │ Replica  │         │ Replica  │     （含 HAProxy pgpool）
  └──────────┘         └──────────┘         └──────────┘
                             │
                       ┌─────▼─────┐
                       │   Redis   │  ← Sentinel（3 节点）或 Cluster
                       │  Sentinel │
                       └───────────┘
                             │
                       ┌─────▼─────┐
                       │  llama.cpp│  ← 双机热备：主备切换（10s 内）
                       │  推理节点  │
                       └───────────┘
```

---

## 2. 最低 HA 配置

### 2.1 硬件清单（最低）

| 角色 | 数量 | 规格 | 备注 |
|------|------|------|------|
| 负载均衡 (Nginx) | 2 | 2C4G | Keepalived VIP 漂移 |
| user-server | 3 | 4C8G | 无状态，水平扩展 |
| PostgreSQL | 1 + 2 | 4C16G | 主从流复制 |
| Redis Sentinel | 3 | 2C4G | 1 主 2 从 + 3 sentinel |
| llama.cpp 推理 | 2 | 8C16G + GPU | 主备切换 |
| 监控/Prometheus | 1 | 2C4G | 可选 |

### 2.2 软件版本

| 组件 | 版本 | 说明 |
|------|------|------|
| PostgreSQL | 15+ | 流复制 + logical replication |
| Redis | 7+ | Sentinel 自动故障转移 |
| Nginx | 1.24+ | stream + http |
| Keepalived | 2.2+ | VIP 漂移 |
| Docker | 24+ | 容器化部署 |
| Kubernetes | 1.28+ | 可选（裸机部署用 Docker Swarm） |

---

## 3. 部署模式

### 3.1 模式 A：Docker Compose 多机（推荐入门）

```yaml
# docker-compose-ha.yml（3 节点示例）
version: '3.9'

x-common-env: &common-env
  POSTGRES_HOST: ${POSTGRES_HOST:-pg-primary}
  POSTGRES_PORT: 5432
  REDIS_ADDR: ${REDIS_ADDR:-redis-sentinel:26379}

services:
  user-server-1:
    image: ghcr.io/xiaofang142/hivemtk/user-server:latest
    hostname: user-server-1
    environment:
      <<: *common-env
      SERVER_ID: 1
    ports:
      - "8204:8204"
    healthcheck:
      test: ["CMD", "curl", "-fsS", "http://localhost:8204/healthz"]
      interval: 10s
      timeout: 3s
      retries: 3

  user-server-2:
    image: ghcr.io/xiaofang142/hivemtk/user-server:latest
    hostname: user-server-2
    environment:
      <<: *common-env
      SERVER_ID: 2
    ports:
      - "8205:8204"

  user-server-3:
    image: ghcr.io/xiaofang142/hivemtk/user-server:latest
    hostname: user-server-3
    environment:
      <<: *common-env
      SERVER_ID: 3
    ports:
      - "8206:8204"

  nginx:
    image: nginx:1.25-alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    depends_on:
      - user-server-1
      - user-server-2
      - user-server-3
```

### 3.2 模式 B：Kubernetes 部署（生产推荐）

```yaml
# k8s/user-server.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: user-server
  labels:
    app: user-server
spec:
  replicas: 3
  selector:
    matchLabels:
      app: user-server
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  template:
    metadata:
      labels:
        app: user-server
    spec:
      containers:
        - name: user-server
          image: ghcr.io/xiaofang142/hivemtk/user-server:v1.0.0
          ports:
            - containerPort: 8204
          env:
            - name: POSTGRES_HOST
              value: pg-primary
            - name: REDIS_ADDR
              value: redis-sentinel:26379
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8204
            initialDelaySeconds: 30
            periodSeconds: 10
            timeoutSeconds: 3
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8204
            initialDelaySeconds: 5
            periodSeconds: 5
            timeoutSeconds: 2
          resources:
            requests:
              cpu: 500m
              memory: 1Gi
            limits:
              cpu: 2000m
              memory: 4Gi
---
apiVersion: v1
kind: Service
metadata:
  name: user-server
spec:
  selector:
    app: user-server
  ports:
    - port: 80
      targetPort: 8204
  type: ClusterIP
```

### 3.3 模式 C：裸机 + systemd（最大性能）

```bash
# /etc/systemd/system/hivemtk-user-server@.service
[Unit]
Description=HiveMtk User Server (instance %i)
After=network.target postgresql.service redis.service

[Service]
Type=notify
ExecStart=/opt/hivemtk/bin/user-server --config=/etc/hivemtk/config-%i.yaml
EnvironmentFile=/etc/hivemtk/user-server.env
User=hivemtk
Group=hivemtk
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

```bash
# 启动 3 副本
systemctl enable hivemtk-user-server@{1,2,3}
systemctl start hivemtk-user-server@{1,2,3}
```

---

## 4. 健康检查

### 4.1 /healthz（Liveness）

```go
// 进程存活检查（轻量，不查 DB）
// 返回 200 → 健康，503 → 重启
r.GET("/healthz", func(c *gin.Context) {
    c.JSON(200, gin.H{"status": "alive"})
})
```

### 4.2 /readyz（Readiness）

```go
// 完整健康检查（含 DB / Redis / 推理服务）
// 返回 200 → 接流量，503 → 摘除流量
r.GET("/readyz", func(c *gin.Context) {
    checks := map[string]string{}
    if err := db.Ping(); err != nil {
        checks["postgres"] = err.Error()
    } else {
        checks["postgres"] = "ok"
    }
    if err := redis.Ping(); err != nil {
        checks["redis"] = err.Error()
    } else {
        checks["redis"] = "ok"
    }
    healthy := true
    for _, v := range checks {
        if v != "ok" {
            healthy = false
        }
    }
    if !healthy {
        c.JSON(503, checks)
        return
    }
    c.JSON(200, checks)
})
```

### 4.3 健康检查时序

| 阶段 | 探针 | 失败行为 |
|------|------|---------|
| 启动 0-5s | 不探针 | - |
| 启动 5s+ | readiness | 503，LB 不转发 |
| 启动 30s+ | liveness + readiness | 都返回 200，正常服务 |
| 运行中 | 每 10s 一次 | 任一失败 → 摘除流量或重启 |

---

## 5. 负载均衡

### 5.1 Nginx upstream 配置

```nginx
upstream user_server {
    least_conn;
    server user-server-1:8204 max_fails=3 fail_timeout=30s;
    server user-server-2:8204 max_fails=3 fail_timeout=30s;
    server user-server-3:8204 max_fails=3 fail_timeout=30s;
    keepalive 32;
    keepalive_requests 1000;
    keepalive_timeout 60s;
}

server {
    listen 80;
    server_name hivemtk.example.com;

    location / {
        proxy_pass http://user_server;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_connect_timeout 3s;
        proxy_read_timeout 30s;
        proxy_send_timeout 30s;
        # WebSocket 支持
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### 5.2 Keepalived VIP 漂移

```bash
# /etc/keepalived/keepalived.conf
vrrp_script check_nginx {
    script "killall -0 nginx"
    interval 2
    weight -2
    fall 3
    rise 2
}

vrrp_instance VI_1 {
    state MASTER          # 备份机写 BACKUP
    interface eth0
    virtual_router_id 51
    priority 100          # 备份机 < 100
    advert_int 1
    authentication {
        auth_type PASS
        auth_pass hivemtk
    }
    virtual_ipaddress {
        192.168.1.100/24  # VIP
    }
    track_script {
        check_nginx
    }
}
```

---

## 6. 故障转移策略

### 6.1 user-server 实例故障

| 检测 | 触发 | 恢复时间 | 数据丢失 |
|------|------|---------|---------|
| /healthz 3 次失败 | Docker/K8s 重启 | < 30s | 0（无状态） |
| /readyz 3 次失败 | LB 摘除 | < 30s | 0（无状态） |
| 进程崩溃 | 进程守护重启 | < 5s | 0（无状态） |
| 节点宕机 | 健康检查 | < 60s | 0（流量切到其他节点） |

### 6.2 PostgreSQL 主库故障

| 检测 | 触发 | 恢复时间 | 数据丢失 |
|------|------|---------|---------|
| Primary OOM / 宕机 | Patroni 自动切换 | < 30s | 0（同步复制） |
| Primary 网络分区 | Sentinel 心跳超时 | < 60s | 0（同步复制） |
| Primary 磁盘损坏 | 切换 + 从备份恢复 | 5-30min | 取决于最近一次备份 |

### 6.3 Redis 主节点故障

| 检测 | 触发 | 恢复时间 | 数据丢失 |
|------|------|---------|---------|
| Master 宕机 | Sentinel 选举新主 | < 30s | 0（持久化开启） |
| 网络分区 | 多数派选举 | < 60s | 0（脑裂保护） |

---

## 7. 滚动升级

```bash
# 1. 拉取新版本镜像
docker pull ghcr.io/xiaofang142/hivemtk/user-server:v1.1.0

# 2. 逐个升级（先升级 1，验证健康，再升级 2、3）
for instance in 1 2 3; do
    docker compose up -d --no-deps user-server-$instance
    # 等待健康检查通过
    until curl -fsS http://user-server-$instance:8204/healthz; do
        sleep 2
    done
    echo "Instance $instance healthy"
done
```

```bash
# Kubernetes 滚动升级
kubectl set image deployment/user-server \
    user-server=ghcr.io/xiaofang142/hivemtk/user-server:v1.1.0
kubectl rollout status deployment/user-server
```

---

## 8. 容量规划

| 并发用户 | user-server 副本 | CPU/副本 | 内存/副本 | DB 连接数 | Redis 内存 |
|---------|----------------|---------|---------|----------|----------|
| < 100 | 2 | 2C | 4G | 100 | 1G |
| 100-1000 | 3 | 4C | 8G | 200 | 2G |
| 1000-10000 | 5+ | 8C | 16G | 500 | 4G |
| 10000+ | 10+ | 16C | 32G | 1000 | 8G |

---

## 9. 验收清单

- [ ] 3 副本 user-server 启动正常
- [ ] Nginx upstream 健康检查通过
- [ ] /healthz 和 /readyz 正常响应
- [ ] PG 主从流复制延迟 < 5s
- [ ] Redis Sentinel 1 主 2 从
- [ ] 故障转移测试：手动 kill 1 个 user-server，LB 自动摘除
- [ ] 故障转移测试：手动 stop PG primary，从库自动提升
- [ ] 故障转移测试：手动 stop Redis master，sentinel 选举新主
- [ ] 滚动升级测试：1 副本 1 副本升级不中断
- [ ] 监控告警：SLO breach 触发回调
- [ ] 数据零丢失验证：模拟主库宕机后写入仍能从从库读到

---

> 配套文档：[灾难恢复手册](DR_RECOVERY.md) · [SLA/SLO 承诺](SLA_SLO.md)
