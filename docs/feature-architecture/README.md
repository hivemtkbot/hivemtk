# HiveMtk 功能架构图 · 交互参数论证 · 优化头脑风暴总览

> 本目录是对 [`../marketing-features/`](../marketing-features/README.md)（用户端 94 个模块）与 [`../../../hivemtk-platform/docs/platform-features/`](../../../hivemtk-platform/docs/platform-features/README.md)（平台端 10 个模块）的**第二层分析**：
> 在既有「背景/数据模型/API/业务流/测试」之上，补充三件事——
> 1. **每个功能的架构图**（Mermaid，组件交互 / 数据流 / 调用链）；
> 2. **详细交互参数 + 逐参论证**（字段为何必填、类型取舍、边界、校验）；
> 3. **头脑风暴论证与优化改进**（权衡、风险、可落地的改进项）。
>
> 覆盖 **19 个业务域 / 104 个功能**。每个域一个文件，域内按功能分节。

---

## 一、文件索引（19 域）

| # | 业务域 | 文件 | 功能数 |
|---|--------|------|-------|
| 1 | 认证与用户管理 | [01-auth-user.md](01-auth-user.md) | 4 |
| 2 | 多平台卡片 | [02-card.md](02-card.md) | 5 |
| 3 | 自动回复与 RAG | [03-auto-reply-rag.md](03-auto-reply-rag.md) | 8 |
| 4 | 邮件营销 | [04-email.md](04-email.md) | 7 |
| 5 | 短信营销 | [05-sms.md](05-sms.md) | 4 |
| 6 | 社群管理 | [06-community.md](06-community.md) | 8 |
| 7 | 短链与活码 | [07-shortlink-livecode.md](07-shortlink-livecode.md) | 3 |
| 8 | 线索与客户 | [08-clue-customer.md](08-clue-customer.md) | 10 |
| 9 | 营销自动化 | [09-marketing-automation.md](09-marketing-automation.md) | 8 |
| 10 | 内容创作 | [10-content.md](10-content.md) | 4 |
| 11 | 系统管理 | [11-system.md](11-system.md) | 11 |
| 12 | 安全与权限 | [12-security.md](12-security.md) | 2 |
| 13 | 第三方对接 | [13-integration.md](13-integration.md) | 2 |
| 14 | 统一消息 | [14-unified-message.md](14-unified-message.md) | 4 |
| 15 | AI 销冠核心 | [15-ai-agent-core.md](15-ai-agent-core.md) | 7 |
| 16 | 多 AI 智能体 | [16-multi-agent.md](16-multi-agent.md) | 3 |
| 17 | 数据分析 | [17-data-analysis.md](17-data-analysis.md) | 3 |
| 18 | 客服 Web Widget | [18-chat-widget.md](18-chat-widget.md) | 1 |
| 19 | 平台端 | [19-platform.md](19-platform.md) | 10 |
| **合计** | | | **104** |

---

## 二、跨功能的关键架构约束（论证基线）

以下约束在多个功能中反复出现，作为统一论证基线（详见工作记忆与各功能节）：

1. **桥接三通道闭环**：上报 `/api/bridge/ingest`、状态 `/api/bridge/outbox/ack`、下发 `/api/bridge/outbox`。
   下发采用「服务端权威 + 原子认领（`FOR UPDATE SKIP LOCKED`）+ at-least-once 重下发 + claim/reclaim 两步走」。论证：该设计根除重复转发，是消息可靠性的锚点，优化时**严禁**在哈希输入加 `conversationID`（跨语言哈希契约：`channel|trim(content)` → `mh:${8位hex}`）。
2. **单租户私域**：`AppKeyResolve` 不强鉴权、`customer_session` 无归属校验(IDOR)、`chat_ws` 空 Origin 放行——均为**设计内预期**，仅文档记录，勿当 bug 改。
3. **生产聊天主路径**：`SmartCSOrchestrator → SalesEngine` → 工厂注入 `ToolExecutor` → `runAgentLoop`（非 `buildPrompt` 兜底）。多轮记忆从 `session_messages` 取最近 ≤20 条注入（剔除本轮）。
4. **LLM 路由兜底**：首选+备选全跳过时自动兜底任意已启用且过质量门禁 provider；`reasoning` 模型 `max_tokens` 基线 ≥2048，否则截断空回复。
5. **全链路追踪 6 节点**：`ingest→ai_dispatch→outbound_enqueue→inbox_sync→downlink_fetch→delivered_ack`，异步非阻塞 sink（缓冲 8192，300ms/200 条批量落库）。`message_hub`/`inbox_conversations` **无 channel 列**，monitor 严禁 `SELECT channel`。

---

## 三、使用说明

- 架构图使用 Mermaid（GitHub / VS Code / 多数 Markdown 渲染器原生支持）。
- 每个功能节结构固定：`架构图` → `交互参数（含逐参论证）` → `头脑风暴与优化论证`。
- 参数论证聚焦「为什么这样设计 / 边界 / 风险」；优化论证聚焦「可落地的具体改进」。
