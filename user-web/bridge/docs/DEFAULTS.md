# Bridge 默认值清单（DEFAULTS）

> 本文档是 `user-web/bridge` 全部默认值的**唯一文档源**。  
> 代码侧单一源：`user-web/bridge/src/core/constants.js`（前端）+ `user-server/internal/bridge/handler.go` / `hub.go` / `cmd/api/main.go`（后端）。  
> 任何调整必须 **同步** 三处：本文档 + 代码常量 + 单元测试。

## 1. 设计原则（项目硬约束）

1. **不允许"软启动"**——任何默认值都必须能从文档溯源。
2. **不允许在多个位置重复硬编码同一个值**——所有常量集中在 constants.js / handler.go / main.go。
3. **不允许"兜底成空值"**——若 config 缺失，明确报错而非静默回退到默认值。
4. **新默认值必须加测试**——`test/constants.test.js` 验证与本清单字面一致。

## 2. 默认值清单

### 2.1 服务端连接（user-server）

| 字段 | 默认值 | 文档源 |
| --- | --- | --- |
| `host` | `localhost` | DEVELOPMENT.md §2.4 端口对照表（dev 本机） |
| `port` | `8204` | `user-server/Dockerfile:57 ENV SERVER_PORT=8204` + `cmd/api/main.go DefaultListenPort` |
| `baseUrl` | `http://localhost:8204` | 上述两者组合 |
| `healthPaths` | `['/health', '/healthz', '/readyz', '/api/health']` | `user-server/internal/router/router.go` 实际注册顺序（优先 /health 含依赖检查） |
| `wsPath` | `/api/ws/bridge` | `user-server/internal/router/service_routes.go:90 auth.GET("/ws/bridge", ...)` |
| `profile` | `dev` | `user-server/config.yaml inference.profile: dev` |

### 2.2 端口兜底（cmd/api/main.go）

| 常量 | 默认值 | 文档源 |
| --- | --- | --- |
| `DefaultListenPort` | `"8204"` | DEVELOPMENT.md §2.4 端口对照表 \| 8204 \| user-server \| Gin HTTP |
| `DefaultRedisPort` | `"8203"` | DEVELOPMENT.md §2.4 端口对照表 \| 8203 \| Redis |

### 2.3 限速/风控（防封号三层，详见 bridge.md §17.3）

| 字段 | 默认值 | 设计动机 |
| --- | --- | --- |
| `accountCapacity` | 12 | 单账号每分钟 Token 桶容量（抖音 IM 风控安全阈值上限） |
| `accountRefillPerMin` | 12 | 单账号每分钟补充速率（与容量相同 → 稳态 12/min） |
| `minIntervalMs` | 1500 | 任意两次下行最小间隔（拟人节奏） |
| `jitterMinMs` | 800 | 发送前随机延迟下限 |
| `jitterMaxMs` | 2600 | 发送前随机延迟上限 |
| `conversationCooldownMs` | 3000 | 同会话两次回复冷却 |
| `conversationPerHour` | 40 | 同会话每小时最多 40 条回复 |
| `dedupWindowMs` | 60000 | 相同文案 60s 去重 |

**前端**：12/min（更严格）  
**后端兜底**：60/min（handler.go `DeliverRateLimitPerMin`）—— 仅防 AI 失控洪泛

### 2.4 WS 客户端（bridge-client.js）

| 字段 | 默认值 | 与服务端对齐 |
| --- | --- | --- |
| `serverIdleTimeoutMs` | 25_000 | < 服务端 `pongWait=60s` 留 35s 缓冲 |
| `reconnectBaseMs` | 1_000 | 指数退避起始 |
| `reconnectMaxMs` | 30_000 | 退避封顶 |
| `reconnectJitterMs` | 500 | 防雪崩抖动 |

### 2.5 UI / 测试

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `healthCheckTimeoutMs` | 3_500 | popup「测试连接」单次 fetch 超时 |
| `metaReportIntervalMs` | 5_000 | content script 周期性上报当前 meta |

### 2.6 协议常量

| 字段 | 值 | 文档源 |
| --- | --- | --- |
| 协议帧名 | `register` / `inbound_message` / `history` / `outbound_reply` / `pong` / `ping` / `ack` / `error` | `user-server/internal/bridge/frames.go` |
| 渠道常量 | `douyin_web` / `xhs_web` / `tiktok_web` | `user-server/internal/bridge/channel.go` |
| 发送方类型 | `customer` / `agent` / `self` | MessageEvent.SenderType |
| 消息方向 | `inbound` / `outbound` | history 帧专用 |

### 2.7 平台入口（仅 popup 快捷按钮）

| 渠道 | URL |
| --- | --- |
| 抖音 | `https://www.douyin.com/` |
| 小红书 | `https://www.xiaohongshu.com/` |
| TikTok | `https://www.tiktok.com/` |

### 2.8 安全护栏

| 字段 | 默认值 | 文档源 |
| --- | --- | --- |
| `maxReplyContentBytes` | 4_096 (4KB) | `handler.go:65` + 前端 constants.SECURITY 同值（避免服务端截断后用户看到残缺） |
| `logMaskMaxChars` | 24 | 超过此长度的字符串在 console 截断（防 PII 泄露） |

## 3. 调整流程

1. **改一处**：`constants.js` 或 `handler.go` / `hub.go` / `main.go` 的常量
2. **改两处**：本文档（数值 + 文档源）
3. **改三处**：相关单元测试（`test/constants.test.js` / `hub_test.go` / `cmd/api/defaults_test.go`）
4. **跑测试**：`cd user-web/bridge && npx vitest run` + `cd user-server && go test -race ./internal/bridge/... ./cmd/api/...`

## 4. 反模式（禁止）

| 反模式 | 后果 | 正确做法 |
| --- | --- | --- |
| `serverUrl \|\| 'http://localhost:8204'` | 静默兜底，掩盖配置错误 | 缺失时明确报错 |
| 多处硬编码 `1024` | 改一处忘另一处 | 全部从 const 取 |
| 文档和代码不一致 | 排查时无法信任文档 | 测试验证一致性 |
| 加默认值不写文档源 | 后续维护者无法调整 | 每个常量都附"文档源"注释 |
| HTML 中写死端口/URL（如 `placeholder="http://localhost:8204"`） | 改 constants.js 后 UI 不变 | 由 JS 加载时从 constants 设置 placeholder |
| `setInterval(fn, 5000)` 直接写 5000 | 改 constants.js 后行为不变 | 从 `UI_DEFAULTS.metaReportIntervalMs` 取 |

## 5. 测试矩阵

| 测试文件 | 验证 |
| --- | --- |
| `test/constants.test.js` | 所有前端常量与本文档字面一致 |
| `test/popup.test.js` | 默认值在 popup 行为中正确应用 |
| `internal/bridge/hub_test.go` | 60/min 兜底限速 |
| `internal/bridge/handler_test.go` | 4KB content 截断 |
| `internal/pkg/utils/config/inference_load_test.go` | config.yaml 与 DefaultInferenceConfig 一致（参照此模式） |
| `cmd/api/defaults_test.go`（v1.3 新增） | `DefaultListenPort` / `DefaultRedisPort` 与 DEVELOPMENT.md / Dockerfile 对齐 |

## 6. 全局审计清单（2026-07-28 重构）

按项目硬约束"默认值单一源 / 禁软启动"全面扫描后的所有调试点：

### 6.1 服务端（user-server）

| 位置 | 调整 | 文档源 |
| --- | --- | --- |
| `cmd/api/main.go:43` | `DefaultListenPort = "8204"` | DEVELOPMENT.md §2.4 端口对照表 |
| `cmd/api/main.go:46` | `DefaultRedisPort = "8203"` | DEVELOPMENT.md §2.4 端口对照表 |
| `internal/bridge/handler.go:51-73` | `writeWait` / `pongWait` / `pingPeriod` / `maxMessageSize` / `maxReplyContentBytes` / `sendBufferSize` / `readBufferSize` / `writeBufferSize` | gorilla/websocket 推荐 + 经验值 |
| `internal/bridge/hub.go:30-38` | `DeliverRateLimitPerMin` / `JanitorInterval` / `JanitorIdleTTL` | 业务经验值 |

### 6.2 扩展（user-web/bridge）

| 位置 | 调整 | 文档源 |
| --- | --- | --- |
| `src/core/constants.js:30-40` | `DEFAULT_USER_SERVER` | DEVELOPMENT.md §2.4 + Dockerfile ENV |
| `src/core/constants.js:50-53` | `PLATFORM_ENTRY_URLS` | 平台官网 |
| `src/core/constants.js:60-79` | `RATE_LIMIT_DEFAULTS` | bridge.md §17.3 + 经验值 |
| `src/core/constants.js:89-97` | `WS_CLIENT_DEFAULTS` | handler.go pongWait/pingPeriod |
| `src/core/constants.js:102-108` | `UI_DEFAULTS` | 业务经验值 |
| `src/core/constants.js:115-129` | `PROTOCOL` | frames.go 协议契约 |
| `src/core/constants.js:136-140` | `SECURITY` | handler.go + 隐私合规 |
| `src/popup/index.js:18-26` | popup 默认值全部从 constants.js 导入（修复前硬编码 8204/HEALTH_PATHS/UI_DEFAULTS） | DEFAULTS.md |
| `src/popup/index.html:97` | placeholder 改为空，由 JS 动态从 constants 设置 | DEFAULTS.md |
| `src/background/index.js:13` | `DEFAULT_USER_SERVER` 已导入（无新硬编码） | DEFAULTS.md |
| `src/core/bridge-client.js:4` | 引入 `DEFAULT_USER_SERVER` / `WS_CLIENT_DEFAULTS`，消除 `/api/ws/bridge` 与 25000 魔数 | constants.js |
| `src/core/sanitize.js:35` | `MAX_BODY_BYTES = SECURITY.maxReplyContentBytes`（修复前硬编码 4*1024） | constants.SECURITY |
| `src/core/logger.js:19` | `MAX_LOG_CHARS = SECURITY.logMaskMaxChars`（修复前硬编码 24） | constants.SECURITY |
| `src/content/common.js:122` | `setInterval(report, UI_DEFAULTS.metaReportIntervalMs)`（修复前硬编码 5000） | constants.UI_DEFAULTS |

## 7. 同步检查（grep audit）

```bash
# 扩展端：禁止在 src/ 出现裸端口字面量（除 constants.js 注释外）
cd hivemtk/user-web/bridge
grep -rn "8204\|8207\|8208\|8209\|8232" src/ --include="*.js" | grep -v "constants.js"

# 服务端：禁止在非 ports.go / config 出现裸端口字面量
cd hivemtk/user-server
grep -rn '"8204"\|"8203"\|"8205"\|"8206"\|"8207"\|"8208"\|"8209"\|"8232"\|8202' cmd/ internal/ --include="*.go" \
  | grep -v "ports.go" \
  | grep -v "inference_load_test.go" \
  | grep -v "main.go" \
  | grep -v "//.*端口" \
  | grep -v "DEVELOPMENT.md" \
  | grep -v "testdb.go"     # 测试默认值，与 CI 一致
# 应只返回端口字面量在 ports.go / 注释 / 文档字符串
```

## 8. 跨包对齐表（v1.4 全局审计，2026-07-28）

为避免"文档说 8204，代码里又写 8204"的双重维护，**所有端口 / URL 默认值必须能溯源到本表**。表中"代码侧常量"列是直接 grep 可得的位置；"审计测试"列是跑通即认为对齐的测试。

| 端口/URL | 文档源 | user-server 代码常量 | bridge 代码常量 | 审计测试 |
| --- | --- | --- | --- | --- |
| user-server Gin HTTP `8204` | DEVELOPMENT.md §2.4 | `config.DefaultListenPort` = `"8204"` | `DEFAULT_USER_SERVER.port` = `8204` | `cmd/api/defaults_test.go` + `test/constants.test.js` |
| PostgreSQL dev `8232` | DEVELOPMENT.md §2.4 | `config.DefaultDBPortDev` = `8232` | — | `pkg/utils/config/ports_test.go` |
| PostgreSQL docker `8202` | DEVELOPMENT.md §2.4 | `config.DefaultDBPortDocker` = `8202` | — | `pkg/utils/config/ports_test.go` |
| Redis `8203` | DEVELOPMENT.md §2.4 | `config.DefaultRedisPort` = `"8203"` | — | `cmd/api/defaults_test.go` |
| platform-server `8205` | DEVELOPMENT.md §2.4 | `config.DefaultPlatformPort` = `"8205"` | — | `pkg/utils/config/ports_test.go` |
| Chromium CDP `8206` | DEVELOPMENT.md §2.4 | `config.DefaultChromiumCDPPort` = `"8206"` | — | `pkg/utils/config/ports_test.go` |
| LLM `8207` | DEVELOPMENT.md §2.4 | `config.DefaultLLMPort` = `8207` | — | `pkg/utils/config/ports_test.go` |
| Embedding `8208` | DEVELOPMENT.md §2.4 | `config.DefaultEmbeddingPort` = `8208` | — | `pkg/utils/config/ports_test.go` |
| Rerank `8209` | DEVELOPMENT.md §2.4 | `config.DefaultRerankPort` = `8209` | — | `pkg/utils/config/ports_test.go` |
| baseURL `http://localhost:8204` | ports.go | `config.DefaultUserServerBaseURL` | `DEFAULT_USER_SERVER.baseUrl` | `pkg/utils/config/ports_test.go` |
| platform baseURL `http://localhost:8205` | ports.go | `config.DefaultPlatformBaseURL` | — | `pkg/utils/config/ports_test.go` |
| LLM baseURL `http://127.0.0.1:8207/v1` | ports.go + config.yaml | `config.DefaultLLMBaseURLDev` | — | `inference_load_test.go` |
| Embedding baseURL `http://127.0.0.1:8208/v1` | ports.go + config.yaml | `config.DefaultEmbeddingBaseURLDev` | — | `inference_load_test.go` |
| Rerank baseURL `http://127.0.0.1:8209/v1` | ports.go + config.yaml | `config.DefaultRerankBaseURLDev` | — | `inference_load_test.go` |
| Chromium CDP URL `http://localhost:8206` | ports.go | `config.DefaultRemoteDebugURL` | — | `pkg/utils/config/ports_test.go` |

## 9. 软启动禁用清单（v1.4 新增）

下列行为是典型"软启动"，**全部禁止**：

| 反模式位置（修复前） | 修复后 |
| --- | --- |
| `cmd/reset-admin/main.go:143-148` 兜底 `pg.Host=localhost/pg.Port=8202/pg.Password=password123` | `log.Fatalf` 明确报错，强制 `POSTGRES_PASSWORD` 注入 |
| `cmd/seed/main.go:191-198` 同样兜底 | 同上 |
| `cmd/perf/main.go:22` `baseURL = "http://localhost:8080"` | 改为 `config.DefaultUserServerBaseURL`（= `http://localhost:8204`） |
| `cmd/embedding-server/main.go:92` `port := flag.Int("port", 80, ...)` | 改为 `config.DefaultEmbeddingPort`（= `8208`） |
| `cmd/routeinspect/main.go:28` `middleware.InitLicenseChecker("http://localhost:8205", "")` | 改为 `config.DefaultPlatformBaseURL` |
| `internal/pkg/utils/config/server.go:323` `return "http://127.0.0.1:8205"` | 改为 `config.DefaultPlatformBaseURL` |
| `internal/pkg/utils/config/server.go:338` `Port = 8202` | 改为 `config.DefaultDBPortDocker` |
| `internal/controller/auto_reply.go:717` `remoteDebugging = "http://localhost:8206"` | 改为 `config.DefaultRemoteDebugURL` |
| `internal/service/short_link.go:549` `http://localhost:8204` | 改为 `config.DefaultUserServerBaseURL` |
| `internal/pkg/testutil/testdb.go:138` `password123` | 加文档源注释；测试专用，与 CI 一致，**不**用于生产 |
