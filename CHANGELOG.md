# 变更日志 (Changelog)

本仓库（hivemtk 用户端）的所有重要变更都会记录在此文件。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/) 规范。

---

## [Unreleased]

### Added
- **2026-07-22 全方位审计整改（Phase 1）**：
  - 新增 `user-web/src/components/PageState.vue`：统一 loading / error / empty 三态展示组件。
  - 新增 `user-web/scripts/i18n-merge-fills.cjs`：将 `phrases_fill1/2/3.js` 合并到 `phrases.js`。
- **2026-07-22 全方位审计整改（M3 重命名）**：
  - `internal/controller/upgrade.go` → `migration_controller.go`，结构体 `UpgradeController` → `MigrationController`。
  - 路由 `/upgrade/*` → `/migration/*`，保留 `setupUpgradeRoutes` 别名以兼容旧调用方。
- **2026-07-22 全方位审计整改（CI/CD）**：
  - 新增 `.github/workflows/user-server-ci.yml`：user-server / user-web / embed-sdk 的 go vet / go build / 单元测试自动化。
- **2026-07-22 全方位审计整改（embed-sdk 边界测试 L3）**：
  - 新增 `embed-sdk/test/boundary.test.mjs`：62 个边界用例覆盖非法 origin / 畸形消息体 / 超大 payload / allowedOrigins 配置异常 / 多次 mount/destroy 循环 / 事件回调异常 / URL 解析 / SSR 守卫 / query 非法值 / 多次 init / 多种消息类型 / 50 次 open/close / 危险协议 origin。
  - `package.json` 新增 `test:boundary` / `test:all` 脚本。
- **2026-07-22 全方位审计整改（数据库迁移文档 L5）**：
  - 新增 `migrations/README.md`：说明 `init-user-db.sql` 与 001-017 迁移文件执行顺序与幂等性约束。

### Changed
- **2026-07-22 全方位审计整改**：
  - `phrases_fill1.js / phrases_fill2.js / phrases_fill3.js` 已合并至 `phrases.js` 并删除，词条数 322 → 1319（净增 997 条）。
  - 控制器文件 `upgrade_controller_test.go` 同步更新为 `MigrationController` 与 `/migration/*` 路径。

### Security
- **审计整改 (M6 - JWT 存储安全)**：`user-web/index.html` 新增 `Content-Security-Policy`（script-src 限 `self`、禁 inline/eval、frame-ancestors 'none'、connect-src 限同源+https），缓解 XSS 窃取 localStorage 中的 JWT。彻底迁移 HttpOnly Cookie 需后端 Session 支持，列为后续增强。

### Changed
- **审计整改 (M7 - 依赖构造集中化)**：`internal/router/admin_routes.go` 抽取 `wirePublicDependencies(db)`，路由层只消费依赖，降低与具体实现的耦合，便于后续接入 DI 容器。
- **审计整改 (L6 - 热点接口缓存)**：`internal/controller/platform.go` 的 `platformCall` 对幂等 `GET` 代理请求增加全局 Redis 短时缓存（TTL 20s），降低对平台 API 的重复调用。
- **审计整改 (M8 - PageState 落地)**：`user-web/src/views/shortLink/List.vue`、`teamUser/List.vue` 接入统一 `PageState` 组件（loading/error/empty 三态，含错误重试），作为全量列表页标准化的参考实现。
- **审计整改 (M9 - API 一致性比对)**：新增 `scripts/api-inventory.sh` 生成后端路由/前端调用比对报告；`.github/workflows/user-ci.yml` 增加非阻塞 `api-inventory` 步骤产出 artifact。

### Known Limitations
以下 TODO 经评估为非阻塞核心功能的占位实现，将在后续版本中补全：
- `internal/controller/trace_controller.go:11` — 全链路 trace 查询/聚合：当前返回 501，路由可达，build 通过。
- `internal/controller/llm_provider_controller.go:11` — LLM Provider health/circuit/policy 字段：当前返回 501，路由可达，build 通过。
- `internal/service/sop_dispatcher.go:173` — WSHub 外部注入（避免循环依赖）。
- `internal/service/sop_node_executors.go:240` — `ScriptTemplateService` 关键词匹配集成（P0-2 待集成）。

---

## [1.0.0] 2026-07-21

### Changed
- **容器统一 `mtk-` 前缀**：所有容器服务名即容器名 —— `postgres-user`→`mtk-postgres`、`redis`→`mtk-redis`、`user-server`→`mtk-user-server`，推理栈保持 `mtk-llm` / `mtk-embedding` / `mtk-rerank`。`docker-compose.yml` 服务键、`depends_on`、`DB_HOST`/`REDIS_HOST`、网络别名同步改为 `mtk-` 前缀。
- **本地推理模型轻量化（当前默认 dev 档）**：LLM 由 `Qwen2.5-3B-Instruct` 降为 `Qwen2.5-1.5B-Instruct`（Q4）；Embedding 由 `BAAI/bge-m3` 换为 `Qwen3-Embedding-0.6B`（TEI Candle 后端，1024 维）；Rerank 由 `bge-reranker-v2-m3` 换为 `bge-reranker-base`。
- **TEI embedding 防 OOM**：增加 `--max-concurrent-requests=1 --max-batch-tokens=512`，docker-compose 内存上限调整为 embedding 8G / llm 1.5G / rerank 1G / user-server 1G / redis·pg 512M。
- **PostgreSQL 宿主机端口 `8202`→`8232`**（避免与 OrbStack 默认占用冲突）；`USER_POSTGRES_HOST_PORT` 默认值同步更新。
- 配套更新 `README.md` / `docs/architecture/部署方案_用户端.md` / `docs/architecture/LOCAL_INFERENCE_OPTIMIZATION.md` / `models/README.md` / `docker-compose-example.yml` 的模型与端口说明。
