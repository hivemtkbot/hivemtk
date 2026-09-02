# HiveMTK user-server 代码审计报告

> **审计日期**: 2026-09-02  
> **审计范围**: `hivemtk/user-server/internal/`（2095 个 Go 文件）  
> **审计维度**: 架构规范 / 编码规范 / 代码质量 / 功能完整度 / 业务链连贯性  
> **审计标准**: Router → Handler(Controller) → Service → Repository → Model 五层架构铁律

---

## 📊 审计总览

| 维度 | 合规率 | 评分 | 核心问题 |
|------|--------|------|----------|
| **架构规范（五层）** | ~90% | ⭐⭐⭐⭐ | 6 个 Service 绕过 Repository 直连 DB，7 个 Controller 直连 DB |
| **编码规范** | ~75% | ⭐⭐⭐ | 455 处 Controller 用 `context.Background()` 丢失 trace，17 处静默吞 panic |
| **功能完整度** | ~95% | ⭐⭐⭐⭐⭐ | ~850+ 端点覆盖，权限有 6 个 P0 级漏挂 |
| **业务链连贯** | **94.6%** | ⭐⭐⭐⭐⭐ | 7 大核心链路生产可用，无阻断性 stub |
| **测试覆盖** | ~27% | ⭐⭐ | geo/controller 零测试，controller 层仅 24% |

---

## 一、🏛️ 架构规范审计（五层架构）

### 1.1 违规统计

| 违规类型 | 严重度 | 文件数 | 典型位置 |
|----------|--------|--------|----------|
| Service 直接 `db.GetDB()` | 🔴 P0 | **6 个** | `r44_gap_services.go`(12+处), `help_center.go`(8处) |
| Controller 直接 GORM 操作 | 🔴 P0 | **7 个** | `prompt.go`, `customer_service.go`, `r44_gap_endpoints.go` |
| Controller 持有 `*gorm.DB` 字段 | 🔴 P0 | **3 个** | `prompt.go`, `customer_service.go` |
| Router 内联 handler 含业务逻辑 | 🟠 P1 | **4 处** | `admin_routes.go:63-83`, `router.go:213-233` |
| DI 注入位置不当 | 🟡 P2 | **2 处** | `script_library.go`, `smart_cs_orchestrator.go` |
| Controller 有但 Repo 缺失的域 | 🟡 P2 | **~15 个** | `help_center`, `handoff_chain`, `r44_gap_*` |

### 1.2 P0 详细违规清单

#### Service 层绕过 Repository 直连 DB

| # | 文件 | 违规次数 | 修复建议 |
|---|------|----------|----------|
| P0-1 | `service/r44_gap_services.go` | **12+** | 拆分为 6 个独立 Repository（Backup/RagEval/Cohort/Email/Clue/DLQ） |
| P0-2 | `service/help_center.go` | **8** | 创建 `repository/help_center_repo.go` 封装 knowledge_documents/chunks 查询 |
| P0-3 | `service/session_ai.go` | **2** | 创建 Repository 封装 session_messages 查询 + upsert |
| P0-4 | `service/office_hours.go` | **1** | 创建 Repository 封装 MessageHub 检查 + Create |
| P0-5 | `service/message_trace_cleanup_cron.go` | **1** | 创建 Repository 封装 PII 打码 + TTL |
| P0-6 | `service/smart_cs_orchestrator.go` | **1** | 从注入层传入 Repository，Service 不应关心 `db.GetDB()` |

#### Controller 层直连 DB

| # | 文件 | 违规描述 |
|---|------|----------|
| P0-7 | `controller/prompt.go` | 直接持有 `db *gorm.DB` 字段，完全跳过 Service/Repository |
| P0-8 | `controller/customer_service.go` | 直接持有 `db *gorm.DB` 字段 |
| P0-9 | `controller/r44_gap_endpoints.go` | 6 处直接 `db.GetDB()` + GORM，最严重的 Controller 违规 |
| P0-10 | `controller/r39_extras.go` | WebVitals / MarketingFlowSync 直接 GORM |
| P0-11 | `controller/dnc_controller.go` | 直接 `db.GetDB().Table(...).Scan(...)`，注释暗示"知道应该用 Repo 但没改" |
| P0-12 | `controller/message_hub.go` | goroutine 中直接 GORM 查询 |
| P0-13 | `controller/rule_engine_controller.go` | Fire 方法中直接查 CustomerSession |

### 1.3 Router 内联 handler 违规

| # | 文件:行号 | 路由 | 问题 |
|---|-----------|------|------|
| P1-1 | `admin_routes.go:63-69` | `POST /api/system/init-admin` | 含业务判断（检查 Initialized 状态） |
| P1-2 | `admin_routes.go:73-83` | `GET /api/license/status`, `GET /api/license/features` | 返回硬编码 JSON，应抽 LicenseController |
| P1-3 | `frontend_aliases.go:578` | `GET /system/menus` | 返回硬编码空数组 |
| P1-4 | `router.go:213-233` | `GET /__debug__/routes` | 完整业务逻辑（遍历去重导出），应放 debug Controller |

### 1.4 合规的好榜样

成熟业务域基本遵守五层架构：
- `auth` / `user` / `message_hub` / `ai_agent` / `clue` / `customer_session`
- geo 子域独立 controller/service/repository/model，不污染主域
- Repository 层基本完善（193 个文件）

---

## 二、💻 编码规范与代码质量审计

### 2.1 错误处理：忽略 error

| 类别 | 数量 | 典型位置 | 风险 |
|------|------|----------|------|
| `_ = ctx.ShouldBindJSON(&req)` | **21 处** | `ai_agent.go:377`, `auth.go:491`, `xianyu_card.go:168,200,232` | 非法参数静默通过 |
| `strconv.Atoi` 忽略 error | **42+ 处** | `inbox.go:53`, `alert_rule.go:144` | 非法值默认 0 → 第一页/空数据 |
| GORM `.Error` 忽略 | **20+ 处** | `session_chain.go:301,429,445,450,454,459`, `whatsapp.go:269-315` | 写库失败静默丢失 |
| `json.Unmarshal` 忽略 error | **20+ 处** | `memory_system.go:741,808`, `dialogue_memory.go:103,174,214,254,272` | 脏数据导致 nil pointer |
| `Close()` 忽略 error | **10+ 处** | `chat_ws.go:190,196,198,249,257,258,261,267` | WS 资源泄漏 |
| 业务 Service 调用忽略 | **4 处** | `auth.go:181`, `notification.go:21`, `agent_status.go:193` | 日志丢漏 |

### 2.2 Context 传递：P0 级架构问题

| 指标 | 数量 | 说明 |
|------|------|------|
| ✅ 正确使用 `ctx.Request.Context()` | **520 处** | — |
| ❌ 错误使用 `context.Background()` | **455 处** | 丢失链路追踪 + 超时感知取消 |

**问题最重的文件**（全部使用 `context.Background()`）：
- `controller/agent_status.go` — 12 处
- `controller/alert_rule.go` — 11 处
- `controller/account.go` — 5 处
- `controller/auth.go` — 7 处

> **正确做法**: Controller 层应全部使用 `ctx.Request.Context()`。后台 goroutine 用 `context.WithoutCancel(ctx)` 或 `context.Background() + WithTimeout`。

### 2.3 Panic 恢复：静默吞 panic

| 类型 | 数量 | 位置 | 正确做法参考 |
|------|------|------|-------------|
| `_ = recover()` 无日志 | **17 处** | `smart_cs_orchestrator.go:354,384,406`, `session_chain.go:39,270`, `r44_gap_services.go:201`, `whatsapp.go` 多处 | `auth.go:82-86` — recover 后 `log.Printf("[...] panic recovered: %v", r)` |

### 2.4 硬编码魔法数字

| 类别 | 数量 | 分布 |
|------|------|------|
| `WithTimeout` 超时值 | **25+ 处** | `dingtalk.go:15s`, `integration.go:30s ×4`, `feedback_loop_cron.go:5-60min ×4` |
| 分页参数解析模板 | **73 处** | 30+ 个 Controller 文件重复 `page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))` |
| 端口默认值 | 少数 | `model/account.go:19` ProxyPort 默认 `1080` |

> **注意**: `pkg/utils/pagination` 已存在统一分页工具，但 Controller 层未充分使用。

### 2.5 测试覆盖

| 目录 | Go 文件 | 测试文件 | 测试率 | 评估 |
|------|---------|----------|--------|------|
| `internal/service` | 484 | 204 | 42% | ⚠️ 中等偏上 |
| `internal/controller` | 208 | 50 | **24%** | 🚨 偏低 |
| `internal/repository` | 193 | 53 | 27% | ⚠️ 偏低 |
| `internal/model` | 173 | 36 | 21% | 🚨 偏低 |
| `internal/middleware` | 30 | 7 | 23% | ⚠️ 偏低 |
| `internal/geo/service` | 30 | 4 | **13%** | 🚨 严重不足 |
| `internal/geo/controller` | 13 | **0** | **0%** | 🚨 完全缺失 |
| `internal/aiagent/llm` | 42 | 22 | 52% | ✅ 良好 |

**完全无测试的目录**：`geo/controller`, `geo/dto`, `integration/templates`, `domain/errors`, `pkg/errhttp`, `pkg/httpclient`, `pkg/messageid`, `pkg/testutil`, `pkg/textutil`, `repository/scope`, `system/install`

### 2.6 已知 Bug（来自测试文件）

| 位置 | 内容 |
|------|------|
| `upload_test.go:507` | `.docx` ZIP magic 被拒 |
| `upload_test.go:525` | `video/mp4` MIME 不在白名单但 `.mp4` 扩展名在 |
| `upload_test.go:534` | `image/svg+xml` MIME 不在白名单但 `.svg` 扩展名在 |
| `upload_test.go:543` | `.rar` MIME vs 扩展名不一致 |

---

## 三、📦 功能完整度审计

### 3.1 工程规模

| 维度 | 数量 |
|------|------|
| Model 文件 | 137（主域）+ 73（子域）≈ 210 表结构 |
| Controller 文件 | 208 |
| Service 文件 | 484 |
| Repository 文件 | 193 |
| DTO 文件 | 46（主域）+ 8（子域） |
| 路由注册调用 | ~1012 处 |
| API 端点总数 | **~850+** |
| *routes.go 文件 | 19 |
| Feature Flag | 6 env + DB 级 API |

### 3.2 路由模块清单（19 个路由文件）

| 路由文件 | 主要职责 | 端点级 |
|----------|----------|--------|
| `router.go` | 总装配 + `/health` + `/__debug__/routes` | 根级 |
| `health.go` | `/health` `/healthz` `/readyz` | 根级 |
| `platform_routes.go` | 登录/SSO/init/system/短链/活码/share | public + auth |
| `admin_routes.go` | license/status/features（内联） | public |
| `auth_routes.go` | refresh-token/logout/current-user/MFA/security | auth |
| `chat_routes.go` | 访客聊天 WS/Sessions/Messages | chat/public |
| `business_routes.go` | feature-flags/rag/sop/faq/kb/intent/memory | auth |
| `service_routes.go` | customer-sessions/agents/quick-replies/session-tags | auth |
| `content_routes.go` | domainpool/material/asset-bundle | auth |
| `channel_overview_routes.go` | channels/overview/platform-accounts/wecom-health | auth |
| `card_routes.go` | 五平台卡片 CRUD + stats + cross-publish | auth |
| `system_routes.go` | system/config/restart/logs/stats/backup/restore | admin |
| `admin_routes.go` | 旧 admin 别名路由 | — |
| `geo_routes.go` | GEO 生成式引擎 20+ 路由 | auth + admin |
| `knowledge_ports.go` | KB 连接器/外部导入 | auth |
| `self_service_routes.go` | 公开注册/忘记密码 | public |
| `sso_routes.go` | SSO providers/login/callback | public |
| `embed_static_routes.go` | embed SDK SPA 静态资源 | 根级 |
| `frontend_aliases.go` | 前端别名路由（含内联空数据） | 多组 |
| `ws.go` | Agent WebSocket `/ws/agent` | auth |
| `tool_debug_routes.go` | AI 工具调试 | admin |
| `tool_providers_route_test.go` | — | — |

### 3.3 权限覆盖审计

#### 中间件层次

```
gin.Engine (r)
├── 根级公开: /health, /healthz, /readyz, /__debug__/routes, /share/*, /chat/embed/*, /ws/visitor, /s/:code, /l/:code, webhook
├── /api public (无 JWT): login/SSO/init/public register
├── /api chat/public (AppKeyResolve + VisitorRateLimit + Sanitize)
├── /api bridgeWS (InitGuard + BridgeIngressGuard — X-Bridge-Token)
├── /api (JWTAuthMiddleware + InitGuard)
│   ├── AdminAuthMiddleware 子组: /system/admin/*, /manage, 各渠道写操作
│   └── 普通 auth: 业务读操作 + 部分写操作
└── /api/platform (InitGuard + JWT + AdminAuthMiddleware)
```

#### ⚠️ P0 级权限漏挂（写操作未 admin 化）

| 模块 | 端点 | 风险 |
|------|------|------|
| **GEO 竞品** | `POST/PUT/DELETE /geo/competitors/*` | 所有 auth 用户可 CRUD 竞品 |
| **主动触达** | `POST /reach/proactive/send|quick|batch` | staff 可触发批量短信/邮件外发 |
| **SOP 执行** | `POST /sop/execute`, `POST /sop/step` | 可绕过 admin 执行 SOP |
| **资产包写** | `POST/PUT/DELETE /asset-bundle/*`, `/asset-market/*`, `/local-assets/*` | staff 可编辑/删除资产包 |
| **Reach Pipeline** | `POST /reach/pipelines` | 创建 pipeline 漏挂 admin |
| **营销流程** | `POST /marketing-flows` | auth 组可创建 |

#### ⚠️ P1 级权限漏挂

| 模块 | 端点 |
|------|------|
| 活码 | `/live-code` 完整 CRUD 在 auth 组 |
| 触达模板 | `POST /templates` |
| 一键竞品 | `PUT /agent/co-pilot/config`, `POST /agent/co-pilot/evaluate` |
| GEO 任务 | `POST /geo/crawler/run`, `POST /geo/inaccurate-claims` |
| 客户旅程 | `POST /customer-journey/transition|touch` |
| Web Vitals | `POST /monitor/web-vitals` |

### 3.4 Feature Flag 体系

| Flag | 默认 | 说明 |
|------|------|------|
| `parallel` | false | 并行化处理 |
| `stream` | false | 流式输出 |
| `layer1` | false | Layer1 决策层（FAQ/SOP SkipLLM） |
| `fallback_chain` | false | 4 级降级链 |
| `debug_log` | false | 调试日志 |
| `sse_bridge` | **true** | Bridge 出站 SSE（唯一默认开启） |

两套机制并存：env 驱动（5s 热加载）+ DB 级 Flag API（admin only）。

### 3.5 健康检查完备性

| 端点 | 鉴权 | 检测范围 |
|------|------|----------|
| `/health` | 无 | DB + Redis + LLM Inference Failover 快照 + Embedding TCP Dial |
| `/healthz` | 无 | **仅存活**，不检查依赖 |
| `/readyz` | 无 | DB + Redis + LLM + Embedding |

状态分级：`ok` / `degraded` / 各组件 `ok` / `down` / `not_configured`，错误码 50301~50306 独立。

---

## 四、🔗 业务链连贯性审计

### 4.1 七大核心链路总览

| 链路 | 完整度 | 入口 | 出口 | 错误处理 | Stub 风险 |
|------|--------|------|------|----------|-----------|
| 1. 消息入库 | **95%** | Bridge HTTP + Webhook | SalesEngine + 规则引擎 | 幂等去重 + echo 自环 | 🟢 无 |
| 2. 消息出库 | **90%** | DeliverBridgeOutbound | 14 渠道 adapter | 重试 + 备用渠道 | 🟡 网页类依赖外发 |
| 3. 会话管理 | **92%** | CreateSession | TTL 自动关闭 | 状态机 + CSAT + 规则引擎 | 🟢 无 |
| 4. RAG 知识库 | **96%** | 多源导入（文件/URL/API） | pgvector + BM25 融合 | 每步 markFailed + panic recover | 🟢 无 |
| 5. SOP 执行 | **98%** | Execute + Step | 19 种节点全实现 | 副作用幂等 + LLM 降级 Next[0] | 🟢 NoopExecutor 兜底 |
| 6. Sales Engine | **97%** | Handle + HandleStream + AgentLoop | 9 步全链路 | 每步独立降级 + 7 维护栏 | 🟢 nil 注入都有 fallback |
| 7. 降级链 | **94%** | LLM Dispatcher 拦截点 | 模板回复 | 4 级链 + 熔断器 + 预算检测 | 🟢 无 |

**综合完整度**: **94.6%** — 生产可用级，无阻断性 stub。

### 4.2 消息入库链路详解

```
渠道消息 → Webhook/Bridge 接收
  → inbox_ingress 幂等去重（canonical_dedup + recent_echo + exact_echo）
  → inbox_ingress_persist 落库 message_hub + session_messages
  → triggerSalesEngine 渠道绑定 Agent 自动执行
  → DispatchSessionEvent 规则引擎
  → ReopenOnInboundMessage closed/resolved → waiting
```

覆盖渠道：WhatsApp / Telegram（原生最完善），WeCom / WeChat / DingTalk / Feishu / Douyin（主要走 bridge 桥接）

### 4.3 消息出库链路详解

```
AI 生成回复 → DeliverBridgeOutbound 落 outbox
  → reach_send_pipeline 8 步管道:
    permission → rate_limit → retry → fallback → audit → cost → journey → send
  → Bridge 外发 / 原生 adapter HTTP 调用
  → BridgeOutbound SSE + WS 双通道通知
```

覆盖渠道：14 个（Douyin / Kuaishou / XHS / TikTok / Xianyu / WeCom / WeChat / DingTalk / Telegram / WhatsApp / Feishu / SMS / Email / WebCard）

### 4.4 会话管理链详解

**状态机**: `pending → ai_handling → waiting → human_handling → resolved → closed`

关键特性：
- TTL Cron：每小时扫描，24h 无活动自动关闭
- resolved → 自动下发 CSAT 调研
- 访客新消息 → resolved/closed 自动 reopen 回 waiting
- SLA auto_resolve：可配置 N 小时超时打标关闭
- 轻量规则引擎：3 事件 + 7 动作 + 延迟执行

### 4.5 RAG 知识库链详解（最完整子系统）

**入库流水线**：
```
mark processing → etl.ExtractText → ProcessDocument 分片
  → chunkRepo.BatchCreate → embeddingService.Embed → persistChunkEmbeddings
  → (contextualEnhance) → BuildIndex → UpdateStatusIndexed
```

**三层检索**：
```
Tier1: FAQ/SOP SkipLLM 精确匹配
Tier2: pgvector 向量检索
Tier3: BM25 关键词检索 + RRF Reciprocal Rank Fusion 融合
```

**高级特性**：HyDE 假设性文档嵌入 / Contextual Retrieval 入库期上下文前缀 / 向量缓存 / 离线 eval

### 4.6 SOP 执行链详解（节点类型 19 种全实现）

**节点执行策略**：
| 类型 | 执行器 | 特性 |
|------|--------|------|
| start / end | 生命周期边界 | — |
| greeting/inquire/introduce/handle/close/invite/follow_up/activate/nurture | MessageNodeBase ×9 | Prompt → Config → LLM → 11 种默认话术 |
| condition / branch | ConditionExecutor | SOPEvaluateConditionBranches → Next[0] 回退 |
| llm / ai_decide | LLMNodeExecutor | JSONMode → 失败降级 Next[0] |
| wait | WaitExecutor | sop_timer 表，默认 24h |
| action / send_offer | MessageNodeBase | — |

**关键特性**：节点幂等性（`_side_effects` + `sideEffectKey`）、llmSem 信号量限流、entry_policy 防重复进入（once/cooldown/always）、DFS 三色环检测

### 4.7 Sales Engine 详解（9 步串行 + AgentLoop 护栏）

```
1. resolve_customer → CustomerLookup
2. recall_memory → DialogueMemoryInterface
3. recognize_intent → IntentRecognizerInterface（nil → unknown 0.3）
4. match_sop → SOPMatcherInterface
5. recall_rag → RAGSearcher
6. generate_candidate → LayerRouter.SkipLLM → AgentLoop（ReAct 模式）
7. polish → PolisherInterface（文本拟人）
8. humanize_eval → HumanizeEvalService（行为拟人）
```

**Agent Loop 7 维护栏**：
wall_clock_timeout / token_budget_exhausted / cost_budget_exhausted / cost_drift_detected / llm_error / empty_final_content / max_iterations_exhausted

**降级链**：intent nil → fallback；LLM 失败 → script → RAG → 默认话术；Agent Loop 终极兜底友好话术

### 4.8 降级链详解（覆盖 LLM 调用 / Agent Loop / SOP / 发送管道）

```
ProviderFailover:
  30s 健康检查 + 5 次失败阈值 + 60s 熔断周期
  场景策略: intent_recognize / sop_reply / objection / friendly_chat / long_summary / high_quality / low_cost

Fallback Tree 4 级:
  LevelPrimary (7B) → LevelSecondary (3B) → LevelCache (Redis) → LevelTemplate
  永久性错误甄别（content_policy / context_length_exceeded → 跳过 Secondary）
  降级响应标记 provider=degraded / model=template 可监控
```

---

## 五、📈 综合评估与修复优先级

### 5.1 优先级矩阵

#### 🔴 P0 严重（必须修，影响生产稳定性/架构根基）

| # | 问题 | 规模 | 影响 | 修复工作量 |
|---|------|------|------|-----------|
| 1 | Controller `context.Background()` | 455 处 | 丢失 trace + 请求取消感知 | 中等（批量替换 ctx.Request.Context()） |
| 2 | `_ = recover()` 无日志 | 17 处 | 生产 panic 不可观测 | 小（加一行 log.Printf） |
| 3 | Service 绕过 Repository 直连 DB | 6 文件 | 架构根基破坏 | 大（r44_gap_services + help_center 最重） |
| 4 | Controller 直连 DB / 持有 DB 字段 | 7 文件 | 架构根基破坏 | 大 |
| 5 | `_ = ctx.ShouldBindJSON` | 21 处 | 非法参数静默通过 | 小（加 3 行 error check） |
| 6 | P0 级权限漏挂 | 6 端点 | 安全风险（主动触达/资产包/SOP执行） | 小（router 加 AdminAuthMiddleware） |

#### 🟡 P1 中等（应该修，影响可维护性/正确性）

| # | 问题 | 规模 | 影响 |
|---|------|------|------|
| 7 | `strconv.Atoi` 忽略 error | 42+ 处 | 非法分页参数默认 0 |
| 8 | GORM `.Error` 忽略 | 20+ 处 | 写库失败静默丢失 |
| 9 | `json.Unmarshal` 忽略 error | 20+ 处 | 脏数据 → nil pointer |
| 10 | geo/controller 零测试 | 13 文件 | 无回归保护 |
| 11 | Router 内联 handler 含业务逻辑 | 4 处 | 违反铁律 |
| 12 | `defer func() { _ = recover() }()` | 17 处 | panic 不可观测 |
| 13 | Controller 测试率仅 24% | 158 文件 | 无回归保护 |

#### 🟢 P2 建议（可选，改善工程质量）

| # | 问题 | 规模 | 建议 |
|---|------|------|------|
| 14 | 分页模板重复 | 73 处 | 用已有 `pkg/utils/pagination` 统一 |
| 15 | WithTimeout 硬编码 | 25+ 处 | 抽常量或配置化 |
| 16 | upload KNOWN BUG | 4 处 | 按测试暴露问题修复 |
| 17 | Router alias 权限绕过 | 多端点 | 检查 `frontend_aliases.go` 中 doReg 注册的别名路由 |
| 18 | 大 Controller 无测试 Top 10 | 10 文件（300+ 行） | 优先 `ai_agent.go` / `chat_public.go` / `webhook.go` |
| 19 | Cron 任务无 Repository | 3 文件 | 创建 Repository 封装批量清理逻辑 |

### 5.2 修复建议顺序

```
阶段 1 — 快速止血（2-3 天）:
  P0-2: 所有 _ = recover() 加日志（17 处，一行一行改）
  P0-5: 所有 _ = ctx.ShouldBindJSON 加 error check（21 处）
  P0-6: 6 个 P0 权限漏挂端点加 admin 中间件

阶段 2 — 架构修复（5-7 天）:
  P0-3: r44_gap_services.go + help_center.go 下沉 Repository（最重的 2 个文件）
  P0-4: prompt.go / customer_service.go / r44_gap_endpoints.go 拆 Service + Repository

阶段 3 — 编码规范统一（并行）:
  P0-1: Controller context.Background() → ctx.Request.Context()（455 处 grep 批量替换）
  P1-7/8/9: strconv.Atoi / GORM Error / json.Unmarshal 忽略 error（各 20-40 处）
```

### 5.3 值得保留的亮点

1. **五层架构主体完整** — 成熟业务域基本遵守铁律，问题集中在"快速补齐"的 gap 功能
2. **RAG 知识库最完善** — 三层检索 + 向量/关键词融合 + HyDE/Contextual Retrieval
3. **SOP 执行链零 stub** — 19 种节点类型全实现，副作用幂等，LLM 降级 Next[0] 不 panic
4. **Sales Engine 9 步链路** — AgentLoop 7 维护栏，每步独立降级，成本漂移熔断
5. **降级覆盖面极广** — LLM/SOP/发送/RAG 四级降级 + 熔断器 + 预算检测
6. **健康检查完备** — 四层探测 + 业务错误码分级
7. **Feature Flag 热加载** — env 驱动 + 5s 轮询，生产零重启
8. **Bridge 三通道分离** — BridgeIngress / Bridge WS / Webhook 职责清晰

---

## 📎 附录：关键文件索引

### 架构违规代表文件
| 文件 | 违规类型 |
|------|----------|
| `service/r44_gap_services.go` | Service 绕过 Repository（12+ 处） |
| `service/help_center.go` | Service 绕过 Repository（8 处） |
| `controller/prompt.go` | Controller 直接持有 `*gorm.DB` |
| `controller/customer_service.go` | Controller 直接持有 `*gorm.DB` |
| `controller/r44_gap_endpoints.go` | Controller 6 处直接 GORM |
| `router/admin_routes.go` | Router 内联 handler 含业务逻辑 |

### 错误处理反模式代表
| 文件 | 问题 |
|------|------|
| `controller/agent_status.go` | context.Background() 12 处 |
| `service/smart_cs_orchestrator.go` | _ = recover() 无日志 3 处 |
| `service/whatsapp.go` | _ = 忽略 GORM error 10 处 |
| `service/session_chain.go` | _ = 忽略 GORM error 6 处 |

### 正确做法参考
| 文件 | 做法 |
|------|------|
| `controller/auth.go:82-86` | recover 后 `log.Printf` 记录 |
| `pkg/utils/pagination.go` | 已存在但未被充分使用的分页工具 |
| `controller/clue.go` | 成熟业务域 Controller 规范榜样 |
| `service/ai_agent.go` | 成熟业务域 Service 规范榜样 |

---

*审计完成。建议从 P0-2（recover 加日志）和 P0-6（权限漏挂）开始快速止血，再推进架构修复。*
