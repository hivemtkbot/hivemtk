# 代码 vs 文档 交叉对比报告

> **报告级别**：⭐⭐⭐ 项目级硬约束文档
> **生成时间**：2026-07-22
> **对比范围**：用户端（`hivemtk/`）+ 平台端（`hivemtk-platform/`）
> **对比方法**：代码 → 文档（按路由/视图清单反查文档）；文档 → 代码（按文档引用反查代码实现）

---

## 一、对比方法学

| 维度 | 用户端 | 平台端 |
|------|--------|--------|
| 后端路由入口 | `user-server/internal/router/router.go` | `platform-server/internal/router/router.go` |
| 前端路由模块 | `user-web/src/router/modules/*.js`（59 个） | `platform-web/src/router/modules/*.js`（3 个） |
| 官网页面 | — | `website/src/views/*.vue`（7 个） |
| 营销功能文档 | `docs/marketing-features/*.md`（44 个） | `docs/platform-features/*.md`（**0 个，目录不存在**） |
| 架构图文档 | `docs/architecture/ARCHITECTURE_DIAGRAM.md` | **无平台端架构图** |
| 部署文档 | `docs/architecture/部署方案_用户端.md` 等 | `docs/architecture/部署方案_平台端与用户端.md` |

---

## 二、用户端交叉对比

### 2.1 实际代码功能清单（来自路由 + 视图）

> 共 59 个前端模块 + 60+ 后端路由组（`setup*Routes`）+ 多 AI 智能体三套独立路由 + 公开 Webhook/Card/Embed 路由。

| # | 模块 slug | 前端路由 | 后端路由组 | 业务域 |
|---|-----------|----------|------------|--------|
| 1 | `abExperiment` | ✅ | `setupABTestRoutes` | 营销自动化 |
| 2 | `aiAgent` | ✅ | `AIAgentController.RegisterRoutes` | 多 AI 智能体 |
| 3 | `aiContent` | ✅ | `setupAIContentRoutes` | 内容创作 |
| 4 | `aiProductivity` | ✅ | `setupAnalyticsRoutes` | 数据分析 |
| 5 | `backup` | ✅ | `setupBackupRoutes` | 系统管理 |
| 6 | `batchOperation` | ✅ | `setupBatchRoutes` | 营销自动化 |
| 7 | `chatChannel` | ✅ | `setupChatChannelAdminRoutes` | 客服 Web Widget |
| 8 | `churnPrediction` | ✅ | `setupChurnRoutes` | 营销自动化 |
| 9 | `clue` | ✅ | `setupClueRoutes` | 线索与客户 |
| 10 | `community` | ✅ | `setupCommunityRoutes` | 社群管理 |
| 11 | `conversionFunnel` | ✅ | `setupAnalyticsRoutes` | 数据分析 |
| 12 | `customReport` | ✅ | `setupCustomReportRoutes` | 营销自动化 |
| 13 | `customer360` | ✅ | (customer.go) | 线索与客户 |
| 14 | `customerEvent` | ✅ | `setupEventRoutes` | CDP 客户事件 |
| 15 | `customerJourney` | ✅ | `setupCustomerJourneyRoutes` | 数据分析 |
| 16 | `customerService` | ✅ | `setupCustomerServiceRoutes` | 客服子功能 |
| 17 | `customerSession` | ✅ | `setupCustomerServiceRoutes` | 客服会话 |
| 18 | `dashboardScreen` | ✅ | `setupDashboardRoutes` | 数据大屏 |
| 19 | `dialogueMemory` | ✅ | `setupDialogueMemoryRoutes` | 对话记忆中心 |
| 20 | `domainPool` | ✅ | `setupDomainPoolRoutes` | 域名池 |
| 21 | `douyinCard` | ✅ | `setupCardRoutes` | 多平台卡片 |
| 22 | `email` | ✅ | `setupEmailRoutes` | 邮件营销 |
| 23 | `feishu` | ✅ | `setupFeishuRoutes` | 飞书账号 |
| 24 | `integration` | ✅ | `setupIntegrationRoutes` | 第三方对接 |
| 25 | `intentRecognition` | ✅ | `setupIntentRoutes` | 意图识别中心 |
| 26 | `knowledge` | ✅ | `setupKnowledgeBaseRoutes` | 知识库内容层 |
| 27 | `kuaishouCard` | ✅ | `setupCardRoutes` | 多平台卡片 |
| 28 | `livecode` | ✅ | `setupLiveCodeRoutes` | 活码 |
| 29 | `llmRouting` | ✅ | `setupLLMRoutingRoutes` | LLM 多模型路由 |
| 30 | `marketingFlow` | ✅ | `setupMarketingFlowRoutes` | 营销自动化 |
| 31 | `messageHub` | ✅ | `setupMessageRoutes` | 消息中心 |
| 32 | `objection` | ✅ | `setupObjectionHandlerRoutes` | 异议处理 |
| 33 | `oneid` | ✅ | (customer_oneid_controller.go) | OneID 身份统一 |
| 34 | `operationLog` | ✅ | (operation_log.go) | 操作日志 |
| 37 | `persona` | ✅ | (sales_persona_controller.go) | 销冠画像 |
| 38 | `platformAccount` | ✅ | `setupPlatformAccountRoutes` | 平台账号 |
| 39 | `ragProductConfig` | ✅ | `setupRagRoutes` | RAG 产品配置 |
| 40 | `reachPipeline` | ✅ | `setupReachPipelineRoutes` | 触达 Pipeline |
| 41 | `scriptTemplate` | ✅ | `setupScriptRoutes` | 话术库 |
| 42 | `securityAudit` | ✅ | `setupQualityRoutes` | 安全审计 |
| 43 | `shortLink` | ✅ | `setupShortLinkRoutes` | 短链管理 |
| 44 | `sms` | ✅ | `setupSmsRoutes` | 短信营销 |
| 45 | `sopAgent` | ✅ | `setupSOPRoutes` | SOP 智能体 |
| 46 | `system` | ✅ | `setupSystemRoutes` | 系统管理 |
| 47 | `tagSegmentation` | ✅ | (user_segment.go) | 标签分层 |
| 48 | `teamUser` | ✅ | `setupTeamRoutes` | 团队用户 |
| 49 | `telegram` | ✅ | `setupTelegramRoutes` | Telegram 账号 |
| 50 | `templateMarket` | ✅ | `setupTemplateRoutes` | 模板市场 |
| 51 | `tiktok` | ✅ | `setupTiktokRoutes` | TikTok 卡片 |
| 52 | `tuning` | ✅ | `setupTuningRoutes` | 置信度/拟人度/反馈学习 |
| 53 | `unifiedInbox` | ✅ | (inbox_controller.go) | 统一收件箱 |
| 54 | `unifiedMessage` | ✅ | `setupMessageRoutes` | 统一消息 |
| 55 | `userSegment` | ✅ | `setupUserSegmentRoutes` | 用户分层 RFM |
| 56 | `wecomAccount` | ✅ | `setupWeComRoutes` + `setupWeComHealthRoutes` | 企微账号 |
| 57 | `whatsapp` | ✅ | `setupWhatsappRoutes` + `setupWhatsAppCloudRoutes` | WhatsApp |
| 58 | `xianyuCard` | ✅ | `setupCardRoutes` | 多平台卡片 |
| 59 | `xiaohongshuCard` | ✅ | `setupCardRoutes` | 多平台卡片 |

**额外后端路由（无独立前端模块但有后端实现）**：
- `setupAccountRoutes` — 账户管理（账号挂载）
- `setupCardStatsRoutes` — 卡片统计（多平台共用）
- `setupAutoReplyRoutes` — 自动回复（通用 + 闲鱼 + TikTok + 小红书）
- `setupMaterialRoutes` — 素材管理
- `setupCustomerRFMRoutes` — 客户 RFM 联动分层
- `setupRecoveryQueueRoutes` — 流失挽回队列
- `setupLLMProviderRoutes` — LLM Provider 降级管理
- `setupTraceRoutes` — 全链路追踪
- `setupSSEDashboardRoutes` — SSE 实时驾驶舱
- `setupUpgradeRoutes` — 版本升级
- `ChannelAgentBindingController` — 渠道账号绑定智能体
- `CustomerServiceAgentController` — 客服座席挂载智能体
- 文件上传：`POST /upload`
- 公开 Webhook/Card Share/Embed Static/Chat Public WebSocket

### 2.2 已有功能文档清单（44 份）

> 来源：`docs/marketing-features/`（见 [README.md](marketing-features/README.md)）

详见 [marketing-features/README.md](marketing-features/README.md) 二级表格。

### 2.3 文档缺口矩阵（代码已有，文档缺失）

> 共 **28 项核心功能**已实现但无独立功能文档。

| # | 模块 slug | 功能名称 | 业务域 | 优先级 |
|---|-----------|----------|--------|--------|
| 1 | `aiAgent` | 多 AI 智能体管理（CRUD/测试/上下文加载） | 多 AI 智能体 | P0 |
| 2 | `channelAgentBinding` | 渠道账号绑定智能体 | 多 AI 智能体 | P0 |
| 3 | `csAgentMount` | 客服座席挂载智能体 | 多 AI 智能体 | P0 |
| 4 | `customerJourney` | 客户旅程大屏（9 阶段监控） | 数据分析 | P1 |
| 5 | `dialogueMemory` | 对话记忆中心（短期/长期/RAG） | AI 销冠 | P0 |
| 6 | `intentRecognition` | 意图识别中心（12 意图分类） | AI 销冠 | P0 |
| 7 | `reachPipeline` | 触达 Pipeline 框架（9 步执行） | 触达中心 | P0 |
| 8 | `sopAgent` | SOP 智能体（DAG 流转） | AI 销冠 | P0 |
| 9 | `llmRouting` | LLM 多模型路由（6 厂商/8 场景） | AI 销冠 | P0 |
| 10 | `llmProvider` | LLM Provider 降级管理 | AI 销冠 | P1 |
| 11 | `traceDashboard` | 全链路追踪驾驶舱 | 系统管理 | P1 |
| 12 | `sseDashboard` | SSE 实时驾驶舱 | 系统管理 | P1 |
| 13 | `objection` | 异议处理 | AI 销冠 | P1 |
| 14 | `persona` | 销冠画像独立 UI | AI 销冠 | P1 |
| 15 | `oneid` | OneID 身份统一（归一化/冲突解决） | CDP | P0 |
| 16 | `tagSegmentation` | 标签分层 | CDP | P1 |
| 17 | `conversionFunnel` | 转化漏斗 | 数据分析 | P1 |
| 18 | `aiProductivity` | 智能体产能 | 数据分析 | P1 |
| 19 | `knowledge` | 知识库内容管理（导入/统计/OpenAPI） | RAG | P0 |
| 20 | `ragProductConfig` | RAG 产品配置 | RAG | P0 |
| 21 | `chatChannel` | 客服 Web Widget 渠道管理 | 客服 | P0 |
| 22 | `unifiedInbox` | 统一收件箱 | 触达中心 | P0 |
| 23 | `wecomAccount` | 企微账号管理（含健康度） | 社群管理 | P0 |
| 24 | `feishu` | 飞书账号管理 | 社群管理 | P1 |
| 25 | `telegram` | Telegram 账号管理 | 社群管理 | P1 |
| 26 | `teamUser` | 团队用户与角色管理 | 认证与用户 | P0 |
| 27 | `batchOperation` | 批量操作 | 营销自动化 | P1 |
| 28 | `operationLog` | 操作日志（事件总线订阅） | 系统管理 | P1 |
| 29 | `securityAudit` | 安全审计 | 系统管理 | P1 |
| 30 | `tuning` | 置信度/拟人度/反馈学习面板 | AI 销冠 | P1 |
| 31 | `livecode` | 活码管理 | 短链与活码 | P0 |
| 32 | `shortlink` | 短链管理（含统计） | 短链与活码 | P0 |
| 33 | `messageHub` | 消息中心 | 统一消息 | P1 |
| 34 | `recoveryQueue` | 流失挽回队列 | 营销自动化 | P1 |
| 35 | `material` | 素材管理 | 内容创作 | P1 |

### 2.4 文档 README 引用但实际不存在的文档（断链）

> 来源：[marketing-features/README.md](marketing-features/README.md) 中带超链接但点击 404 的条目。

| 引用文档 | 实际状态 | 替代方案 |
|----------|----------|----------|
| `merchant-initialization.md` | ❌ 不存在 | 已实现，需新建 |
| `team-user-management.md` | ❌ 不存在 | 已实现，需新建（对齐 `teamUser` 模块） |
| `auto-reply-universal.md` | ❌ 不存在 | 已实现，需新建 |
| `email-list-management.md` | ❌ 不存在 | 已实现，需新建 |
| `email-draft-management.md` | ❌ 不存在 | 已实现，需新建 |
| `email-jobs-management.md` | ❌ 不存在 | 已实现，需新建 |
| `email-send-execution.md` | ❌ 不存在 | 已实现，需新建 |
| `sms-list-management.md` | ❌ 不存在 | 已实现，需新建 |
| `sms-draft-management.md` | ❌ 不存在 | 已实现，需新建 |
| `sms-jobs-management.md` | ❌ 不存在 | 已实现，需新建 |
| `agent-telegram-automation.md` | ❌ 不存在 | 已实现，需新建 |
| `community-management.md` | ❌ 不存在 | 已实现，需新建 |
| `shortlink-management.md` | ❌ 不存在 | 已实现，需新建 |
| `livecode-management.md` | ❌ 不存在 | 已实现，需新建 |
| `knowledge-management.md` | ❌ 不存在 | 已实现，需新建 |
| `material-management.md` | ❌ 不存在 | 已实现，需新建 |
| `integration-account.md` | ❌ 不存在 | 已实现，需新建 |

### 2.5 文档存在但 README 中无超链接（仅文本）

| 文档 | 状态 |
|------|------|
| `email-smtp-config.md` | 仅文本，建议补链接 |
| `sms-config.md` | 仅文本，建议补链接 |
| `system-config.md` | 仅文本，建议补链接 |
| `obs-config.md` | 仅文本，建议补链接 |

### 2.6 用户端架构图现状

[ARCHITECTURE_DIAGRAM.md](architecture/ARCHITECTURE_DIAGRAM.md) v1.3 已涵盖：
- ✅ C4 上下文 + 容器图
- ✅ 五层架构（L1 表现 / L2 网关 / L3 业务 / L4 aiAgent 能力 / L5 数据）
- ✅ 模块依赖关系图（含 aiagent 合并层）
- ✅ AI 销冠架构（Engine / Tool-Use / Pipeline / SOP 状态机）
- ✅ 数据流图（客户消息处理 / 单实例隔离）
- ✅ 部署架构（单服务器 / 离线）
- ✅ 安全架构
- ✅ 关键流程时序图（登录 / AI 销冠谈单）
- ✅ ADR-001 ~ ADR-005

**待补强**：
- 多 AI 智能体架构（MULTI_AI_AGENT_DESIGN）章节可补图
- OneID 身份统一数据流图可补
- 触达 Pipeline 9 步与 Webhook 入站 9 步对比图可补

---

## 三、平台端交叉对比

### 3.1 实际代码功能清单（来自路由 + 视图）

#### 3.1.1 platform-server 后端 API

| 分组 | 路径 | 方法 | 鉴权 | 功能 |
|------|------|------|------|------|
| 健康检查 | `/health`、`/api/health` | GET | 公开 | Docker / K8s 探针 |
| 商户端 API | `/merchant-api/merchant/register` | POST | 签名 | 商户注册（用户端调用） |
| 商户端 API | `/merchant-api/logs/report` | POST | 签名 | 用户端日志上报 |
| 公开 API | `/public/site/contact` | GET | 公开 | 官网联系信息读取 |
| 平台公开 | `/api/platform/heartbeat` | POST | 公开 | 用户端心跳上报 |
| 平台公开 | `/api/platform/install` | POST | 公开 | 用户端安装信息上报 |
| 平台认证 | `/api/auth/login` | POST | 公开 | 平台管理员登录 |
| 平台认证 | `/api/user` | GET | JWT | 当前用户信息 |
| 平台认证 | `/api/users/:id` | PUT | JWT | 更新用户 |
| 平台认证 | `/api/change-password` | POST | JWT | 修改密码 |
| 驾驶舱 | `/platform/dashboard` | GET | JWT | 驾驶舱统计 |
| 平台统计 | `/platform/stats/system` | GET | JWT | 系统信息 |
| 平台统计 | `/platform/stats/overview` | GET | JWT | 总览 |
| 平台统计 | `/platform/stats/merchant` | GET | JWT | 商户统计 |
| 商户管理 | `/platform/merchants` | GET/POST | JWT | 商户列表/创建 |
| 商户管理 | `/platform/merchants/:key` | GET/PUT/DELETE | JWT | 商户详情/更新/删除 |
| 商户管理 | `/platform/merchants/:key/approve` | POST | JWT | 商户审批 |
| 商户管理 | `/platform/merchants/statistics` | GET | JWT | 商户统计聚合 |
| 商户管理 | `/platform/merchants/:key/statistics` | GET | JWT | 单商户统计 |
| 商户管理 | `/platform/merchants/:key/api-trend` | GET | JWT | API 趋势 |
| 系统监控 | `/platform/monitoring/health` | GET | JWT | 系统健康 |
| 系统监控 | `/platform/monitoring/api-metrics` | GET | JWT | API 指标 |
| 系统监控 | `/platform/monitoring/performance` | GET | JWT | 性能指标 |
| 心跳查询 | `/platform/installs/list` | GET | JWT | 安装列表 |
| 心跳查询 | `/platform/heartbeats/list` | GET | JWT | 心跳列表 |
| 心跳查询 | `/platform/heartbeats/stats` | GET | JWT | 心跳统计 |
| 站点联系 | `/platform/site/contact` | GET/PUT | JWT | 站点联系管理 |

#### 3.1.2 platform-web 前端页面（3 个路由模块）

| 路由模块 | 路由 | 视图 | 功能 |
|----------|------|------|------|
| `dashboard` | `/` | `dashboard/Index.vue` | 驾驶舱 |
| `platform` | `/platform/merchant/list` | `platform/merchant/List.vue` | 商户管理 |
| `platform` | `/platform/heartbeat` | `platform/Heartbeat.vue` | 心跳监控 |
| `platform` | `/platform/stats` | `platform/Stats.vue` | 平台统计 |
| `platform` | `/platform/monitoring` | `monitoring/Index.vue` | 系统监控 |
| `system` | `/system/stats` | `system/Stats.vue` | 系统统计 |
| — | `/login` | `Login.vue` | 登录 |
| — | `/profile` | `Profile.vue` | 个人资料 |

> 注：`router/modules/merchant.js` 存在但未被 `index.js` 引入（重复定义于 `platform.js`），属冗余文件。

#### 3.1.3 website 官网页面（7 个）

| 页面 | 路由 | 视图 | 功能 |
|------|------|------|------|
| 首页 | `/` | `HomePage.vue` | 产品介绍、核心卖点 |
| 功能 | `/features` | `FeaturesPage.vue` | 功能详情 |
| 工具链 | `/toolchain` | `ToolchainPage.vue` | 架构图、工具链 |
| 工作流 | `/workflow` | `WorkflowPage.vue` | 业务工作流 |
| 文档 | `/docs` | `DocsPage.vue` | 文档入口 |
| 下载 | `/download` | `DownloadPage.vue` | 下载源码 |
| FAQ | `/faq` | `FaqPage.vue` | 常见问题 |

### 3.2 已有平台端文档清单

| 文档 | 状态 |
|------|------|
| [docs/INDEX.md](../hivemtk-platform/docs/INDEX.md) | ✅ 存在（但引用大量断链） |
| [docs/architecture/部署方案_平台端与用户端.md](../hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md) | ✅ 存在 |

### 3.3 平台端文档缺口矩阵

> 平台端文档严重缺失：用户端文档反复引用的 `hivemtk-platform/docs/platform-features/` 目录**完全不存在**。

#### 3.3.1 INDEX.md 引用但实际不存在的文档

| 引用文档 | 实际状态 | 必要性 |
|----------|----------|--------|
| `operations/DEPLOY_PLATFORM.md` | ❌ 不存在 | 高 |
| `operations/MONITORING.md` | ❌ 不存在 | 高 |
| `architecture/PLATFORM_DEPLOYMENT.md` | ❌ 不存在 | 高 |
| `architecture/INSTALLATION_ARCHITECTURE.md` | ❌ 不存在 | 中 |
| `scripts/release.sh` | ❌ 不存在（已删除） | — |
| `scripts/publish-user.sh` | ❌ 不存在（已删除） | — |

#### 3.3.2 用户端文档引用的平台端文档（不存在）

> 用户端 [marketing-features/README.md](marketing-features/README.md) §十六声明「平台端 18 个模块见 `hivemtk-platform/docs/platform-features/`」，但该目录**不存在**。

| 用户端引用 | 实际状态 |
|------------|----------|
| `hivemtk-platform/docs/platform-features/README.md` | ❌ 不存在 |
| `hivemtk-platform/docs/platform-features/platform-merchant.md` | ❌ 不存在 |
| `hivemtk-platform/docs/platform-features/platform-license.md` | ❌ 不存在（License 已下线） |
| `hivemtk-platform/docs/platform-features/merchant-api.md` | ❌ 不存在 |
| `hivemtk-platform/docs/platform-features/public-api.md` | ❌ 不存在 |

#### 3.3.3 平台端架构图完全缺失

| 应有架构图 | 现状 |
|------------|------|
| 平台端 C4 上下文图 | ❌ |
| 平台端容器图 | ❌ |
| 平台端组件图（platform-server 内部） | ❌ |
| 平台端数据流图（用户端 → 平台端心跳/安装） | ❌ |
| 平台端部署架构图 | 仅在「部署方案_平台端与用户端.md」内有简图 |
| 平台端与用户端通信矩阵 | ✅ 已有（部署方案文档内） |

### 3.4 平台端功能模块清单（按实际代码归并）

| # | 模块 slug | 功能名称 | 后端控制器 | 前端页面 | 文档状态 |
|---|-----------|----------|------------|----------|----------|
| 1 | `platform-auth` | 平台登录与用户管理 | `auth.go` | `Login.vue`、`Profile.vue` | ❌ 缺 |
| 2 | `platform-dashboard` | 驾驶舱 | `dashboard.go` | `dashboard/Index.vue` | ❌ 缺 |
| 3 | `platform-merchant` | 商户管理 CRUD/审批/统计 | `merchant/management.go` | `platform/merchant/List.vue` | ❌ 缺 |
| 4 | `platform-heartbeat` | 心跳接收与监控 | `heartbeat.go` | `platform/Heartbeat.vue` | ❌ 缺 |
| 5 | `platform-install` | 安装信息上报与查询 | `install.go` | （融在心跳页） | ❌ 缺 |
| 6 | `platform-stats` | 平台统计（系统/总览/商户） | `platform_stats.go` | `platform/Stats.vue` | ❌ 缺 |
| 7 | `platform-monitoring` | 系统监控（健康/API指标/性能） | `monitoring/system.go` | `monitoring/Index.vue`、`system/Stats.vue` | ❌ 缺 |
| 8 | `merchant-api` | 商户端 API（注册/日志上报） | `merchant_api.go` | — | ❌ 缺 |
| 9 | `site-contact` | 官网联系信息管理 | `site_contact.go` | （website 调用） | ❌ 缺 |

---

## 四、汇总统计

| 维度 | 用户端 | 平台端 |
|------|--------|--------|
| 实际代码功能数 | 59 前端模块 + 60+ 后端路由组 | 9 个功能模块（含 1 个商户端 API） |
| 已有功能文档数 | 44 份 | 0 份（目录不存在） |
| 文档缺口数 | 28+ 项 | 9 项（全部缺失） |
| 架构图文档 | 1 份（v1.3 较完整） | 0 份（完全缺失） |
| INDEX 引用断链数 | 17 项 | 6 项 |
| 跨仓库引用断链 | 引用平台端 `platform-features/` 不存在 | — |

---

## 五、修复优先级

> **进度更新（2026-07-22）**：P0/P1/P2 全部完成。所有断链清除、缺失文档补全、架构图补强、冗余文件清理均已完成。

### P0 必修（影响文档可用性）
1. ✅ **平台端创建 `docs/platform-features/` 目录与 README**（用户端反复引用）— 已创建 9 份功能文档
2. ✅ **平台端创建架构图文档**（`docs/architecture/PLATFORM_ARCHITECTURE.md`）— 已创建 9 节内容
3. ✅ **平台端 INDEX.md 清除断链**（移除或补建引用文档）— 已清除 6 项断链，移除 OTA 引用
4. ✅ **用户端 marketing-features/README.md 清除断链**（17 项）— 已重写，模块数更新为 92
5. ✅ **用户端补 P0 级缺失功能文档**（aiAgent / channelAgentBinding / csAgentMount / dialogueMemory / intentRecognition / reachPipeline / sopAgent / llmRouting / oneid / knowledge / ragProductConfig / chatChannel / unifiedInbox / wecomAccount / teamUser / livecode / shortlink）— 16 份已创建（livecode/shortlink/teamUser 合并到 *-management.md）

### P1 应修（提升文档完整性）
6. ✅ 用户端补 P1 级缺失功能文档（customerJourney / objection / persona / tagSegmentation / conversionFunnel / aiProductivity / feishu / telegram / batchOperation / operationLog / securityAudit / tuning / messageHub / recoveryQueue / material / llmProvider / traceDashboard / sseDashboard）— 17 份已创建
7. ⏸️ 平台端补 operations/ 部署/监控文档 — 改为从 INDEX.md 移除断链引用（部署内容已由 PLATFORM_ARCHITECTURE.md + 部署方案_平台端与用户端.md + 发布流程.md 覆盖）
8. ✅ 用户端架构图补强多 AI 智能体章节 — 已在 ARCHITECTURE_DIAGRAM.md §4.5 新增多智能体编排层拓扑（v1.4）

### P2 可选
9. ✅ 删除 `platform-web/src/router/modules/merchant.js` 冗余文件 — 已删除（router/index.js 未引用，路由已在 platform.js 中定义）
10. ✅ 统一两端 README 中的「模块数」表述（用户端 92 + 平台端 9 = 101）

---

## 六、相关文档

- 用户端入口：[docs/INDEX.md](INDEX.md)
- 用户端架构：[architecture/ARCHITECTURE_DIAGRAM.md](architecture/ARCHITECTURE_DIAGRAM.md)
- 用户端功能索引：[marketing-features/README.md](marketing-features/README.md)
- 平台端入口：[../hivemtk-platform/docs/INDEX.md](../hivemtk-platform/docs/INDEX.md)
- 平台端部署：[../hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md](../hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md)
- 跨端分工论证：[architecture/部署方案_平台端与用户端.md](architecture/部署方案_平台端与用户端.md)

---

*本报告由代码扫描 + 文档反查交叉生成，作为后续文档补全工作的依据。*
