# 渠道 Ingress 收敛 (Channel Ingress)

> **所属模块**: 统一消息 / 多渠道接入
> **功能 slug**: `channel-ingress`
> **文档定位**: 7 大社媒渠道统一接入消息中台的总览文档，遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。
> **设计依据**: [企业级架构优化/渠道接入消息中台.md](../企业级架构优化/渠道接入消息中台.md)
> **代码位置**: `user-server/internal/controller/*_account_controller.go` + `internal/controller/message_hub_controller.go` + `internal/controller/inbox_controller.go`

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 渠道 Ingress 收敛 |
| 功能名称(英文) | Channel Ingress Convergence |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | 统一消息 / 多渠道接入 |
| 优先级 | P1 |
| 实际完成时间 | 2026-07 |
| 最后更新 | 2026-07-24 |

### 1.1 已完成内容

- [x] 7 大渠道账号管理（独立 controller / CRUD / 测试）
- [x] 统一消息事件结构（MessageEvent）
- [x] 统一收件箱（InboxService）
- [x] 公开 Webhook 入站路由（无 JWT，按渠道签名鉴权）
- [x] AI 智能体绑定（每个渠道账号可绑 ai_agent_id）
- [x] 人工接管锁（Redis 串行化排队防抖）
- [x] 高并发上下文追加（Redis pending 队列）

### 1.2 待完成内容

- [ ] 渠道健康度统一看板（部分渠道已有，如 wecom_health）

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

HiveMtk 定位为"七端打透"的私域营销 OS，需把抖音 / 快手 / 小红书 / 闲鱼 / TikTok / 企微 / 微信公众号 / Telegram / 钉钉 / 飞书 / WhatsApp 等多渠道流量统一接入消息中台，避免每个渠道一套独立收发链路导致的运维灾难。

### 2.2 解决思路

**渠道收敛三步走**：
1. **账号层**：每个渠道独立 controller 维护账号 CRUD 与凭据加密存储
2. **入站层**：每个渠道通过 Webhook / WebSocket 把消息推到统一 `MessageEvent` 结构
3. **消费层**：`InboxService.HandleIngressMessage` 统一处理（人工锁优先 → AI 处理锁 → 上下文追加队列）

### 2.3 关键算法或模型

#### 2.3.1 统一消息事件结构（MessageEvent）

```go
type MessageEvent struct {
    EventID   string    // 全局唯一事件 ID
    SessionID string    // 系统内部映射的唯一会话 ID
    Channel   string    // 渠道来源: "douyin" / "kuaishou" / "xhs" / "xianyu" / "tiktok" / "wecom" / "telegram" / "dingtalk" / "feishu" / "whatsapp"
    SenderID  string    // 最终客户的唯一物理标识
    MsgType   string    // "text" / "image" / "file"
    Content   string    // 纯文本消息内容
    Timestamp time.Time
}
```

#### 2.3.2 双锁机制

- **人工接管锁**：`hivemtk:lock:human:{session_id}` → 命中后仅入库不路由 AI
- **AI 处理锁**：`hivemtk:lock:ai_processing:{session_id}` → SetNX 15 秒，未抢到则推入 `hivemtk:pending:{session_id}` 上下文追加队列

#### 2.3.3 7 大渠道接入矩阵

| 渠道 | 入站方式 | 账号管理 controller | 智能卡片 | 自动回复 |
|---|---|---|---|---|
| 抖音 | Webhook | `douyin_card.go` | ✅ | ✅ |
| 快手 | Webhook | `kuaishou_card.go` | ✅ | ✅ |
| 小红书 | Webhook | `xiaohongshu_card.go` | ✅ | ✅（`xiaohongshu_auto_reply.go`） |
| 闲鱼 | Webhook | `xianyu_card.go` | ✅ | ✅（`xianyu_auto_reply.go`） |
| TikTok | Webhook | `tiktok_card_controller.go` | ✅ | ✅（`tiktok_auto_reply_controller.go`） |
| 企微 | 回调 | `wecom.go` + `wecom_health_controller.go` | - | ✅ |
| 微信公众号 | 回调 | `wecom.go`（共用） | - | ✅ |
| Telegram | Webhook | `telegram_account_controller.go` | - | ✅（`agent-telegram-automation`） |
| 钉钉 | 回调 | `dingtalk_app_account_controller.go` | - | ✅ |
| 飞书 | 回调 | `feishu_account_controller.go` | - | ✅ |
| WhatsApp | Webhook | `whatsapp.go` + `whatsapp_cloud_account_controller.go` | - | ✅ |

> 注：HiveMtk README 主推"七端打通"，实际覆盖更多渠道；本表反映真实代码实现，便于运维排障。

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入(Webhook) | 渠道签名 / token | header/query | 是 | 各渠道鉴权字段 |
| 输入(Webhook) | 原始事件体 | body | 是 | 各渠道事件结构 |
| 输出(Webhook) | challenge / 200 | - | - | 渠道要求响应 |
| 输出(消息事件) | MessageEvent | - | - | 内部统一结构 |

---

## 三、设计标准

### 3.1 渠道账号管理 API 契约（典型）

| Method | URL 前缀 | 鉴权 | 说明 |
|---|---|---|---|
| GET | /api/{channel}-account/accounts | JWT | 列表 |
| POST | /api/{channel}-account/accounts | JWT | 创建 |
| PUT | /api/{channel}-account/accounts/:id | JWT | 更新 |
| DELETE | /api/{channel}-account/accounts/:id | JWT | 删除 |
| POST | /api/{channel}-account/accounts/:id/test | JWT | 测试 |

具体前缀见各渠道独立文档（feishu-account / telegram-account / wecom-account / dingtalk-app-account 等）。

### 3.2 入站 Webhook 路由

| Method | URL 前缀 | 鉴权 | 说明 |
|---|---|---|---|
| POST | /api/webhook/:channel | 渠道签名 | 入站消息统一入口 |
| GET | /api/webhook/:channel | 渠道签名 | 渠道 challenge 校验 |
| WS | /ws/chat | JWT | 客服端 WebSocket（坐席侧） |
| WS | /ws/chat-public | 公开 | 嵌入式 Chat Widget（访客侧） |

### 3.3 安全与合规

- 凭据（AppSecret / Token / AESKey）AES-256-GCM 加密存储
- 返回时统一掩码
- Webhook 入站按渠道签名校验，不依赖 JWT
- 人工接管锁优先于 AI 处理锁
- 每条入站消息落 PostgreSQL + 审计日志
- 主动触达发送前打印 `[COMPLIANCE]` 合规提示（不可关闭）

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| Webhook 入站 P99 | < 500ms（含入库） |
| 锁抢成功率 | > 95% |
| 上下文追加队列延迟 | < 100ms |
| 单实例并发会话 | 1000+ |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| L2 网关 | 中间件 + 公开路由 | 渠道签名校验 |
| L3 业务 | controller/*_account_controller.go | 渠道账号 CRUD |
| L3 业务 | controller/message_hub_controller.go | 消息中台 |
| L3 业务 | controller/inbox_controller.go | 统一收件箱 |
| L3 业务 | controller/webhook_controller.go | Webhook 入站 |
| L4 aiAgent | service.InboxService | 双锁 + AI 路由 |
| L5 数据 | PostgreSQL / Redis | 持久化 + 锁 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 多 AI 智能体（ai-agent） | ai_agent_id 绑定 |
| 统一消息（unified-message） | MessageEvent 结构 |
| 统一收件箱（unified-inbox） | InboxService 消费 |
| 客服会话（cs-session） | session_id 映射 |
| 多平台卡片（card-*） | 渠道卡片共用入站 |

### 4.3 数据流向

```text
[渠道客户端] → [Webhook 入站] → [渠道签名校验]
                                  │
                                  ▼
                       [转换为 MessageEvent]
                                  │
                                  ▼
                       [InboxService.HandleIngressMessage]
                                  │
                ┌─────────────────┴──────────────────┐
                ▼                                   ▼
        [人工锁命中?]                          [AI 锁抢成功?]
        是 → 仅入库                              是 → 触发 AgentRuntime
        否 → 检查 AI 锁                          否 → 推入 pending 队列（上下文追加）
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 商户在「社群管理 → 各渠道账号」添加账号
2. 在渠道开放平台配置回调地址指向 `/api/webhook/:channel`
3. 在「多 AI 智能体」配置智能体
4. 在「渠道账号」绑定智能体
5. 渠道用户发消息 → 自动进入消息中台 → 路由到智能体 / 人工

### 5.2 系统处理流程

**入站**：
1. Webhook 接收 → 渠道签名校验
2. 转换为 `MessageEvent`（含 event_id / session_id / channel / sender_id / content）
3. `InboxService.HandleIngressMessage(event)`：
   - 检查人工接管锁 → 命中则仅入库
   - 否则 SetNX AI 锁（15s）→ 成功触发 AgentRuntime / 失败推入 pending 队列
4. AgentRuntime 调用 AI 智能体 → 生成回复 → 渠道 adapter 发送

**出站（主动触达）**：
1. 营销人员在触达 Pipeline 配置任务
2. Service 调用渠道 adapter（如 WhatsApp Cloud API / Telegram Bot API）
3. 发送前打印 `[COMPLIANCE]` 提示
4. 失败重试 + 错误记录到 `last_error_at` / `last_error_msg`

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 签名校验失败 | 401 | 渠道拒绝 |
| 消息体格式错误 | 400 | 入库为 invalid 事件 |
| AI 锁超时 | - | 自动释放，下条消息可抢锁 |
| 渠道 adapter 调用失败 | - | 落 last_error_at，触达 Pipeline 重试 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `messages` | 统一消息存储（含 channel / session_id / sender_id / content / direction） |
| `{channel}_accounts` | 各渠道账号配置（凭据加密） |
| `ai_agents` | 智能体配置（绑定关系） |
| `customer_sessions` | 会话主表（session_id 映射） |

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 7 大渠道账号 CRUD | 各渠道参数 | 创建成功 | 待执行 |
| TC-002 | Webhook 入站 + 签名校验 | 合法签名 | MessageEvent 入库 | 待执行 |
| TC-003 | Webhook 签名失败 | 错误签名 | 401 | 待执行 |
| TC-004 | 人工锁优先 | 已有人工锁 | 仅入库不路由 AI | 待执行 |
| TC-005 | AI 锁抢成功 | 无人工锁 | 触发 AgentRuntime | 待执行 |
| TC-006 | 上下文追加 | AI 锁被占 | 推入 pending 队列 | 待执行 |
| TC-007 | 智能体绑定 | ai_agent_id | 路由到指定智能体 | 待执行 |
| TC-008 | 主动触达 | 渠道 adapter 调用 | 发送 + 合规提示 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| AI 锁 TTL | INBOX_AI_LOCK_TTL | 15s | AI 处理锁超时 |
| pending 队列长度 | INBOX_PENDING_MAX | 100 | 单会话上下文追加上限 |
| Webhook 超时 | WEBHOOK_TIMEOUT | 30s | 入站响应超时 |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| Webhook 入站失败率 | > 1% | 钉钉 |
| AI 锁等待时长 | > 15s 持续 | 钉钉 |
| pending 队列堆积 | > 50 | 钉钉 |
| 渠道 adapter 失败率 | > 5% | 钉钉 + 邮件 |

---

## 九、参考资料

- `user-server/internal/controller/inbox_controller.go`
- `user-server/internal/controller/message_hub_controller.go`
- `user-server/internal/controller/webhook_controller.go`
- [企业级架构优化/渠道接入消息中台.md](../企业级架构优化/渠道接入消息中台.md)
- [unified-message.md](unified-message.md)
- [unified-inbox.md](unified-inbox.md)
- [message-hub.md](message-hub.md)
- [ai-agent.md](ai-agent.md)
- [channel-agent-binding.md](channel-agent-binding.md)
- [feishu-account.md](feishu-account.md)
- [telegram-account.md](telegram-account.md)
- [wecom-account.md](wecom-account.md)
- [dingtalk-app-account.md](dingtalk-app-account.md)
- [card-douyin.md](card-douyin.md)
- [card-kuaishou.md](card-kuaishou.md)
- [card-xiaohongshu.md](card-xiaohongshu.md)
- [card-xianyu.md](card-xianyu.md)
- [card-tiktok.md](card-tiktok.md)
- [community-whatsapp.md](community-whatsapp.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-24 | 独立功能文档生成（F-P1-110 补建） | |
