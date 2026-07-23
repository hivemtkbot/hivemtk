# HiveMtk 用户端 - 营销功能模块索引

> **目录定位**: 本目录存放用户端（商户端）私域部署下**所有营销工具既有功能**的独立详细文档。
> **适用范围**: 用户端 **91 个核心业务模块**（认证/卡片/自动回复/RAG/邮件/短信/社群/短链/线索/营销自动化/内容/系统/集成/AI 销冠/多 AI 智能体等）。平台端 9 个模块见 [`hivemtk-platform/docs/platform-features/`](../../hivemtk-platform/docs/platform-features/README.md)（独立仓库）。
> **文档规范**: 每份文档独立成文,严格按统一功能文档模板（背景/数据模型/API/业务流/测试/版本历史）编写。
> **最后更新**: 2026-07-22（全面补全 28+ 项缺失功能文档，对齐实际代码）

---

## 一、文档总览

| 业务域 | 子模块数 | 子目录前缀 |
|--------|---------|-----------|
| 认证与用户管理 | 5 | `auth-*` / `user-*` / `team-*` / `merchant-init-*` |
| 多平台卡片 | 5 | `card-*` |
| 自动回复与 RAG | 8 | `auto-reply-*` / `rag-*` / `knowledge-*` |
| 邮件营销 | 5 | `email-*` |
| 短信营销 | 4 | `sms-*` |
| 社群管理 | 6 | `community-*` / `wecom-*` / `feishu-*` / `telegram-*` |
| 短链与活码 | 3 | `shortlink-*` / `livecode-*` / `domain-*` |
| 线索与客户 | 11 | `clue-*` / `customer-*` / `cs-*` / `websocket-*` / `oneid-*` / `tag-*` |
| 营销自动化 | 8 | `marketing-*` / `ab-*` / `rfm-*` / `churn-*` / `report-*` / `dashboard-*` / `batch-*` / `recovery-*` |
| 内容创作 | 5 | `content-*` / `script-*` / `template-*` / `material-*` |
| 系统管理 | 11 | `system-*` / `obs-*` / `upgrade-*` / `backup-*` / `upload-*` / `operation-log-*` / `security-audit-*` / `trace-*` / `sse-*` |
| 第三方对接 | 2 | `integration-*` / `sync-*` |
| 统一消息 | 3 | `unified-message-*` / `unified-inbox-*` / `message-hub-*` / `platform-account-*` |
| AI 销冠核心 | 9 | `dialogue-memory-*` / `intent-*` / `sop-*` / `llm-routing-*` / `llm-provider-*` / `objection-*` / `persona-*` / `tuning-*` / `reach-pipeline-*` |
| 多 AI 智能体 | 3 | `ai-agent-*` / `channel-agent-binding-*` / `cs-agent-mount-*` |
| 数据分析 | 4 | `customer-journey-*` / `conversion-funnel-*` / `ai-productivity-*` / `analytics-*` |
| 客服 Web Widget | 1 | `chat-channel-*` |
| 平台端 | 9 | 见 [`hivemtk-platform/docs/platform-features/`](../../hivemtk-platform/docs/platform-features/README.md) |
| **合计** | **101** | - |

> **统计说明**: 用户端 92 份 + 平台端 9 份 = 101 份功能文档。用户端实际代码包含 59 个前端路由模块 + 60+ 后端路由组，部分后端路由（如 `setupAccountRoutes`/`setupCardStatsRoutes`）共用文档。

---

## 二、认证与用户管理域

> ⚠️ `merchant-initialization.md` 虽归类在"认证与用户管理",但实际属于**商户四文档**之一(商户首次入驻),与"平台端域"的 `platform-merchant` / `merchant-api` 协作,详见 [INDEX.md 边界关系](../INDEX.md)。

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [auth-login-jwt.md](auth-login-jwt.md) | 登录认证与 JWT 鉴权 | ✅ 已实现 |
| [user-management.md](user-management.md) | 用户管理 CRUD | ✅ 已实现 |
| [team-user-management.md](team-user-management.md) | 团队用户与角色管理 | ✅ 已实现 |
| [merchant-initialization.md](merchant-initialization.md) | 商户初始化向导(**商户四文档之一**) | ✅ 已实现 |
| [websocket-realtime.md](websocket-realtime.md) | WebSocket 实时通信 | ✅ 已实现 |

## 三、多平台卡片域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [card-douyin.md](card-douyin.md) | 抖音卡片生成 | ✅ 已实现 |
| [card-kuaishou.md](card-kuaishou.md) | 快手卡片生成 | ✅ 已实现 |
| [card-xiaohongshu.md](card-xiaohongshu.md) | 小红书卡片生成 | ✅ 已实现 |
| [card-xianyu.md](card-xianyu.md) | 闲鱼卡片生成 | ✅ 已实现 |
| [card-tiktok.md](card-tiktok.md) | TikTok 卡片生成 | ✅ 已实现 |

## 四、自动回复与 RAG 域

> 📌 **RAG 三文档边界关系**:`rag-knowledge-base`(配置) → `knowledge-management`(内容入库) → `agent-rag-qa`(应用调用)。三者构成 RAG 完整链路,各司其职,详见各文档顶部"边界说明"。

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [auto-reply-universal.md](auto-reply-universal.md) | 通用自动回复（chromedp） | ✅ 已实现 |
| [auto-reply-xianyu.md](auto-reply-xianyu.md) | 闲鱼自动回复 | ✅ 已实现 |
| [auto-reply-tiktok.md](auto-reply-tiktok.md) | TikTok 自动回复 | ✅ 已实现 |
| [rag-knowledge-base.md](rag-knowledge-base.md) | RAG 知识库配置(**配置层**:LLM/Embedding/pgvector) | ✅ 已实现 |
| [rag-product-config.md](rag-product-config.md) | RAG 产品配置（多产品绑定） | ✅ 已实现 |
| [knowledge-management.md](knowledge-management.md) | 知识库文档管理(**内容层**:文档导入/分段/向量化) | ✅ 已实现 |
| [agent-rag-qa.md](agent-rag-qa.md) | RAG 智能客服(**应用层**:智能客服场景) | ✅ 已实现 |
| [script-library.md](script-library.md) | 话术库 | ✅ 已实现 |

## 五、邮件营销域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [email-list-management.md](email-list-management.md) | 邮件列表与收件人 | ✅ 已实现 |
| [email-smtp-config.md](email-smtp-config.md) | SMTP 配置管理 | ✅ 已实现 |
| [email-draft-management.md](email-draft-management.md) | 邮件草稿 | ✅ 已实现 |
| [email-jobs-management.md](email-jobs-management.md) | 邮件任务 | ✅ 已实现 |
| [email-send-execution.md](email-send-execution.md) | 邮件发送执行 | ✅ 已实现 |

## 六、短信营销域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [sms-config.md](sms-config.md) | 短信配置（阿里云/腾讯云/华为云） | ✅ 已实现 |
| [sms-list-management.md](sms-list-management.md) | 短信列表与发送 | ✅ 已实现 |
| [sms-draft-management.md](sms-draft-management.md) | 短信草稿 | ✅ 已实现 |
| [sms-jobs-management.md](sms-jobs-management.md) | 短信任务调度 | ✅ 已实现 |

## 七、社群管理域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [community-whatsapp.md](community-whatsapp.md) | WhatsApp 营销 | ✅ 已实现 |
| [agent-telegram-automation.md](agent-telegram-automation.md) | Telegram AI 销售自动化 | ✅ 已实现 |
| [telegram-account.md](telegram-account.md) | Telegram 账号管理 | ✅ 已实现 |
| [community-wecom.md](community-wecom.md) | 企业微信 | ✅ 已实现 |
| [wecom-account.md](wecom-account.md) | 企微账号管理（含健康度） | ✅ 已实现 |
| [feishu-account.md](feishu-account.md) | 飞书账号管理 | ✅ 已实现 |
| [community-management.md](community-management.md) | 通用社群管理 | ✅ 已实现 |

## 八、短链与活码域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [shortlink-management.md](shortlink-management.md) | 短链管理（含统计） | ✅ 已实现 |
| [livecode-management.md](livecode-management.md) | 活码管理 | ✅ 已实现 |
| [domain-pool.md](domain-pool.md) | 域名池管理 | ✅ 已实现 |

## 九、线索与客户管理域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [clue-management.md](clue-management.md) | 线索管理 | ✅ 已实现 |
| [customer-360.md](customer-360.md) | 客户 360 视图 | ✅ 已实现 |
| [cdp-event-tracking.md](cdp-event-tracking.md) | 客户事件追踪 CDP | ✅ 已实现 |
| [oneid.md](oneid.md) | OneID 身份统一（归一化/冲突解决） | ✅ 已实现 |
| [tag-segmentation.md](tag-segmentation.md) | 标签分层 | ✅ 已实现 |
| [cs-session.md](cs-session.md) | 客服会话 | ✅ 已实现 |
| [cs-agent.md](cs-agent.md) | 客服代理 | ✅ 已实现 |
| [cs-quick-reply.md](cs-quick-reply.md) | 快捷回复 | ✅ 已实现 |
| [cs-session-tag.md](cs-session-tag.md) | 会话标签 | ✅ 已实现 |
| [cs-ai-suggest.md](cs-ai-suggest.md) | AI 建议 | ✅ 已实现 |
| [chat-channel.md](chat-channel.md) | 客服 Web Widget 渠道管理 | ✅ 已实现 |

## 十、营销自动化域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [marketing-flow.md](marketing-flow.md) | 营销流程编排 | ✅ 已实现 |
| [ab-test.md](ab-test.md) | A/B 测试 | ✅ 已实现 |
| [rfm-segment.md](rfm-segment.md) | 用户分层 RFM | ✅ 已实现 |
| [churn-prediction.md](churn-prediction.md) | 流失预警 | ✅ 已实现 |
| [recovery-queue.md](recovery-queue.md) | 流失挽回队列 | ✅ 已实现 |
| [custom-report.md](custom-report.md) | 自定义报表 | ✅ 已实现 |
| [dashboard.md](dashboard.md) | 数据大屏 | ✅ 已实现 |
| [batch-operation.md](batch-operation.md) | 批量操作 | ✅ 已实现 |

## 十一、内容创作域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [ai-content.md](ai-content.md) | AI 内容创作 | ✅ 已实现 |
| [script-library.md](script-library.md) | 话术库 | ✅ 已实现 |
| [template-market.md](template-market.md) | 模板市场 | ✅ 已实现 |
| [material-management.md](material-management.md) | 素材管理 | ✅ 已实现 |
| [file-upload.md](file-upload.md) | 文件上传 | ✅ 已实现 |

## 十二、系统管理域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [system-config.md](system-config.md) | 系统配置 | ✅ 已实现 |
| [system-ops.md](system-ops.md) | 系统运维 | ✅ 已实现 |
| [obs-config.md](obs-config.md) | OBS 对象存储配置 | ✅ 已实现 |
| [upgrade.md](upgrade.md) | 版本升级 | ✅ 已实现 |
| [backup-recovery.md](backup-recovery.md) | 备份恢复 | ✅ 已实现 |
| [operation-log.md](operation-log.md) | 操作日志（事件总线订阅） | ✅ 已实现 |
| [security-audit.md](security-audit.md) | 安全审计 | ✅ 已实现 |
| [trace-dashboard.md](trace-dashboard.md) | 全链路追踪驾驶舱 | ✅ 已实现 |
| [sse-dashboard.md](sse-dashboard.md) | SSE 实时驾驶舱 | ✅ 已实现 |
| [llm-provider.md](llm-provider.md) | LLM Provider 降级管理 | ✅ 已实现 |
| [tuning-panel.md](tuning-panel.md) | 置信度/拟人度/反馈学习面板 | ✅ 已实现 |

## 十三、第三方对接域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [integration-account.md](integration-account.md) | 集成账号管理 | ✅ 已实现 |
| [sync-log.md](sync-log.md) | 同步日志 | ✅ 已实现 |

## 十四、统一消息域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [unified-message.md](unified-message.md) | 统一消息 | ✅ 已实现 |
| [unified-inbox.md](unified-inbox.md) | 统一收件箱 | ✅ 已实现 |
| [message-hub.md](message-hub.md) | 消息中心 | ✅ 已实现 |
| [platform-account.md](platform-account.md) | 平台账号管理 | ✅ 已实现 |

## 十六、AI 销冠核心域

> 📌 **AI 销冠引擎核心模块**: 围绕 `sales_engine.Engine.Handle()` 主流程（感知 → 决策 → 行动 → 记忆）构建。详见 [ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md) §四 AI 销冠架构。

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [dialogue-memory.md](dialogue-memory.md) | 对话记忆中心（短期/长期/RAG） | ✅ 已实现 |
| [intent-recognition.md](intent-recognition.md) | 意图识别中心（12 意图分类） | ✅ 已实现 |
| [sop-agent.md](sop-agent.md) | SOP 智能体（DAG 流转） | ✅ 已实现 |
| [llm-routing.md](llm-routing.md) | LLM 多模型路由（6 厂商/8 场景） | ✅ 已实现 |
| [objection-handler.md](objection-handler.md) | 异议处理 | ✅ 已实现 |
| [sales-persona.md](sales-persona.md) | 销冠画像独立 UI | ✅ 已实现 |
| [reach-pipeline.md](reach-pipeline.md) | 触达 Pipeline 框架（9 步执行） | ✅ 已实现 |
| [ai-content.md](ai-content.md) | AI 内容创作 | ✅ 已实现 |
| [ab-test.md](ab-test.md) | A/B 测试（话术/触达实验） | ✅ 已实现 |

## 十七、多 AI 智能体域

> 📌 **多 AI 智能体架构**（MULTI_AI_AGENT_DESIGN）: 一个商户可配置多个独立智能体（销售型/客服型/混合型），每个智能体可绑定不同 LLM、SOP、知识库。详见 [ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)。

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [ai-agent.md](ai-agent.md) | 多 AI 智能体管理（CRUD/测试/上下文加载） | ✅ 已实现 |
| [channel-agent-binding.md](channel-agent-binding.md) | 渠道账号绑定智能体 | ✅ 已实现 |
| [cs-agent-mount.md](cs-agent-mount.md) | 客服座席挂载智能体 | ✅ 已实现 |

## 十八、数据分析域

| 文档 | 功能名称 | 状态 |
|------|---------|------|
| [customer-journey.md](customer-journey.md) | 客户旅程大屏（9 阶段监控） | ✅ 已实现 |
| [conversion-funnel.md](conversion-funnel.md) | 转化漏斗 | ✅ 已实现 |
| [ai-productivity.md](ai-productivity.md) | 智能体产能 | ✅ 已实现 |
| [custom-report.md](custom-report.md) | 自定义报表 | ✅ 已实现 |

## 十九、平台端域（跳转至独立仓库）

> 📌 **平台端 9 份功能模块**已拆分到独立仓库,不存放于本目录。
>
> 📌 **商户相关四文档边界关系**（跨仓库协作）:
> - [`platform-merchant.md`](../../hivemtk-platform/docs/platform-features/platform-merchant.md) = 平台对**商户主体**的 CRUD/审批（平台端）
> - [`merchant-api.md`](../../hivemtk-platform/docs/platform-features/merchant-api.md) = 商户**运行时**与平台通信的 `/merchant-api` 接口集（平台端）
> - [`merchant-initialization.md`](merchant-initialization.md) = 商户端**首次入驻**的多步骤配置向导（归类在"认证与用户管理"，本目录）
> - 平台端开源版已下线 License 流程，`platform-license.md` 不再存在
>
> 完整平台端功能清单见 [`hivemtk-platform/docs/platform-features/`](../../hivemtk-platform/docs/platform-features/README.md)。

## 二十、文档使用说明

1. **每个文档独立成文**：不允许多个功能合并到同一份文档。
2. **结构遵循模板**：所有文档严格按统一功能文档模板编写。
3. **状态实时更新**：文档末尾的"版本历史"与"功能完成状态"必须随代码变更同步更新。
4. **交叉引用**：被引用的子模块在"被依赖模块"中清晰列出。

---

## 二十一、相关文档

- [../INDEX.md](../INDEX.md) — 用户端文档总索引
- [../CROSS_COMPARISON_REPORT.md](../CROSS_COMPARISON_REPORT.md) — 代码 vs 文档交叉对比报告
- [../architecture/部署方案_平台端与用户端.md](../architecture/部署方案_平台端与用户端.md) — 平台端/用户端分工论证
- [../architecture/ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md) — 系统架构图（C4 / 五层 / 模块依赖）
- 平台端 9 个模块：[`hivemtk-platform/docs/platform-features/`](../../hivemtk-platform/docs/platform-features/README.md)

---

*最后更新: 2026-07-22（全面补全 28+ 项缺失功能文档，对齐实际代码；平台端模块数从 18 修正为 9）*
