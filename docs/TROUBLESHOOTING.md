# HiveMTK 故障排查手册（TROUBLESHOOTING）

> 本文档汇总 HiveMTK 私域独立部署常见故障的定位路径、临时缓解、根因修复方案。
> 配合 `INCIDENT_RUNBOOK.md`（按告警维度拆分）使用。

---

## 1. 启动类故障

### 1.1 user-server 启动 panic：`USER_JWT_SECRET 未配置或长度不足 32 字符`
**症状**：`go run` 或 `docker run` 进程 panic 退出，stderr 含 `panic: [SECURITY] USER_JWT_SECRET ...`
**根因**：JWT 签名密钥未配置或长度不足 32 字符；生产模式下 `loadJWTSecret` 主动 panic 防止使用硬编码密钥。
**修复**：
```bash
# 在 .env 中设置至少 32 字符的随机密钥
export USER_JWT_SECRET="$(openssl rand -hex 32)"
# 或 Docker 环境变量
docker run -e USER_JWT_SECRET="$(openssl rand -hex 32)" ...
```
**避免**：
- 不要使用 `test-jwt-secret-do-not-use-in-prod-32+chars` 这类固定测试值上线。

### 1.2 数据库连接失败：`failed to connect to ...: dial tcp 127.0.0.1:5432: connect: connection refused`
**症状**：启动日志报 PostgreSQL 连不上，进程退出。
**根因**：
- PostgreSQL 未启动或端口未监听；
- `.env` 中 `POSTGRES_HOST` / `POSTGRES_PORT` 错误；
- Docker 容器内 `localhost` 指向容器自身而非宿主机。
**修复**：
```bash
# 1. 检查端口监听
ss -tlnp | grep 5432
# 2. Docker 场景：用 host.docker.internal 替代 localhost
POSTGRES_HOST=host.docker.internal
# 3. 验证连通
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB -c "SELECT 1"
```

---

## 2. 鉴权 / CORS 类

### 2.1 浏览器控制台：`CORS policy: ... No 'Access-Control-Allow-Origin' header`
**症状**：前端跨域请求被浏览器拦截；返回 403。
**根因**：请求 Origin 不在 `CORS_ALLOW_ORIGINS` 白名单；新版 CORS 中间件（白名单模式）拒绝反射任意 Origin。
**修复**：
```bash
# 在 .env 中追加允许的来源（逗号分隔）
CORS_ALLOW_ORIGINS="https://app.example.com,https://admin.example.com"
# 重启 user-server 生效
```
**避免**：
- 不要写 `*` 与 `Allow-Credentials: true` 共存，浏览器会拒绝。

### 2.2 WebSocket 连接 401 / 403
**症状**：`new WebSocket('wss://...')` 立即被关闭；服务端日志 `[ws upgrade failed]`。
**根因**：
- token 缺失或已过期；
- token 中的 `user_id` 与 query `agent_id` 不一致（新鉴权强制对齐）；
- 浏览器 WebSocket API 不支持自定义 header，token 必须走 query。
**修复**：
```js
// 前端：登录后用 user_id 替换 agent_id，并附 token
const ws = new WebSocket(`wss://app.example.com/ws?agent_id=${userId}&token=${token}`)
```

---

## 3. 性能 / 可靠性

### 3.1 审计日志偶发丢失
**症状**：审计查询页某些时间窗内操作记录缺失。
**根因**：P0-7 已修复——`saveAuditBatch` 重试逻辑曾永远不触发；当前已收集 `failedLogs` 并按指数退避重试 3 次。
**缓解**：
- 服务端日志中查找 `[audit] N 条审计日志在 3 次重试后仍写入失败`，定位具体失败原因（DB 抖动 / 磁盘满 / 死锁）。
- 临时方案：在 `audit.go` 中把 `maxRetries` 提升到 5-10，但需配套监控告警，避免无限堆积。

### 3.2 批量发送 WhatsApp 任务卡住
**症状**：`/whatsapp/job/{id}` 状态长时间 `running` 但 `success + failed < total`。
**根因**：
- 目标账号 `conns` 失效，`ensureConn` 失败未释放资源；
- 调度 goroutine 内部 panic。
**修复**：
```bash
# 1. 查看相关日志
journalctl -u hivemtk-user | grep -E 'whatsapp|panic'
# 2. P0-6 已通过 utils.SafeGo 包装所有后台 goroutine，确保 panic 不再拖垮进程
# 3. 手动重置该 job 状态
psql -c "UPDATE whatsapp_jobs SET status='pending' WHERE id=123"
```

### 3.3 Outbox 多实例重复派发
**症状**：SOP 节点被同一实例多次触发，导致重复执行。
**根因**：未使用 `SELECT ... FOR UPDATE SKIP LOCKED` 时多实例并发查询。
**修复**：
- P1-21 已修复——`processDueTimers` 启用 GORM `clause.Locking{Strength:"UPDATE", Options:"SKIP LOCKED"}`。
- 验证：开启 2 个 user-server 实例，查看日志 `[outbox] processed due timers` 的 `fired_count` 总和 == DB 中实际触发的行数。

---

## 4. 数据迁移 / 模型

### 4.1 AutoMigrate 与手写 SQL 冲突
**症状**：`pq: column "xxx" does not exist` 或 `pq: relation "xxx" already exists`。
**根因**：
- `internal/migration/migrations/*.go` 手动创建表与 GORM AutoMigrate 重复；
- 索引与字段在两边都被声明。
**修复**：
```bash
# 1. 临时禁用 AutoMigrate 启动，看 migrations 是否完整
USER_AUTO_MIGRATE=false ./user-server
# 2. 单独跑一次手动迁移
go run ./cmd/migrate up
```

### 4.2 025 迁移 DROP TABLE 误删数据
**症状**：`025_unify_system_users.sql` 执行后 `system_users` 表数据丢失。
**修复**（一次性预防，非已发生补救）：
- 所有 DROP TABLE 之前必须先 `CREATE TABLE system_users_backup_2026XXXX AS SELECT * FROM system_users`；
- 跑完冒烟验证后再 `DROP`；
- 出问题用 `psql -c "INSERT INTO system_users SELECT * FROM system_users_backup_2026XXXX"` 还原。

---

## 5. 前端 / 前端构建

### 5.1 `npm run build` 报 `JavaScript heap out of memory`
**症状**：CI 构建 OOM，进程退出码非零。
**根因**：Element Plus 全量引入 + Vite 默认 chunk 拆分过粗。
**修复**：
```bash
# 1. 增加 Node 堆内存
NODE_OPTIONS=--max-old-space-size=4096 npm run build
# 2. 长期方案：P1-10 改为按需引入（unplugin-vue-components + unplugin-auto-import）
```

### 5.2 浏览器控制台 `localStorage quota exceeded`
**症状**：登录态丢失、菜单折叠态丢失。
**根因**：i18n / 主题 / 路由历史全部塞 localStorage，单域超过 5-10MB 上限。
**修复**：
- 清理浏览器 localStorage；
- 长期方案：i18n 包体拆分懒加载，路由历史用 IndexedDB 替代。

---

## 6. 运维 / 监控

### 6.1 Prometheus `/metrics` 返回 404
**症状**：Grafana 面板无数据。
**根因**：路由未挂载或中间件顺序错误。
**修复**：
```go
// router 中确保
r.GET("/metrics", gin.WrapH(promhttp.Handler()))
// 且放在鉴权中间件之前（metrics 端点通常无需鉴权或单独 IP 白名单）
```

### 6.2 容器 OOMKilled
**症状**：`docker ps -a` 看到 `Exited (137)`，内核日志含 `Memory cgroup out of memory: Killed process`。
**根因**：单容器内存超过 `memory.limit_in_bytes`；常见于 `embedding-server` 大模型加载后未释放。
**修复**：
```yaml
# docker-compose.yml
services:
  user-server:
    deploy:
      resources:
        limits:
          memory: 4G
    # embedding-server 单独跑 + GPU/分页加载
  embedding-server:
    deploy:
      resources:
        limits:
          memory: 8G
```

---

## 7. 常见问题快速索引

| 现象 | 速查 |
|------|------|
| 启动 panic 含 `SECURITY` | 1.1 |
| CORS 跨域 403 | 2.1 |
| WebSocket 401/403 | 2.2 |
| 审计日志丢失 | 3.1 |
| WhatsApp job 卡住 | 3.2 |
| Outbox 重复派发 | 3.3 |
| 迁移冲突 | 4.1 |
| DROP TABLE 误删 | 4.2 |
| npm build OOM | 5.1 |
| `/metrics` 404 | 6.1 |
| 容器 OOMKilled | 6.2 |

---

> 更多排障细节请配合 `INCIDENT_RUNBOOK.md`（按告警维度拆分）。
