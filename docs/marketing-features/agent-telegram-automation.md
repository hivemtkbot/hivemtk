# Telegram 智能体自动化

> 私域独立部署 · 单租户架构 · 入群即销售触点 · 消息即销售线索

## 一、功能概述

本模块彻底摒弃传统的"付费入群"模式，转而聚焦**销售转化核心场景**：

1. **TG 用户入群自动触发 智能体流程**：新用户加入群组的瞬间，智能体引擎主动发起欢迎+销售开场白，无需用户主动 /start 或发消息。
2. **TG 机器人收到消息自动列入 智能体流程**：群组或私聊中的每一条用户消息都会自动进入 SalesEngine 8 步销售管线（识别客户 → 召回记忆 → 意图识别 → SOP 匹配 → RAG 召回 → 候选生成 → 话术润色 → 合规审核）。
3. **统一消息中台**：所有入群事件、退群事件、普通消息统一写入 MessageHub + InboxConversation，便于审计、复盘、销售跟进。
4. **Webhook 安全验签**：使用 `X-Telegram-Bot-Api-Secret-Token` 头 + `subtle.ConstantTimeCompare` 防止伪造。

### 与旧"付费入群"模式的对比

| 维度 | 旧模式（已废弃） | 新模式（智能体自动化） |
|---|---|---|
| 触发点 | 用户付费 → 入群 | 用户入群 → AI 主动开场 |
| 价值定位 | 收门票 | 销售转化 |
| AI 介入 | 无 | 入群即触发 8 步销售管线 |
| 消息处理 | 仅记录订单 | 每条消息进入 智能体流程 |
| 商业模型 | 一次性收入 | 持续销售转化 + 售后跟进 |

## 二、核心架构

### 2.1 调用链路

```
Telegram Bot API (setWebhook)
    │
    ▼
[POST /api/webhook/telegram/:account_id]
    │
    ▼
WebhookService.Receive
    │
    ├─► Verify (X-Telegram-Bot-Api-Secret-Token + ConstantTimeCompare)
    ├─► ParsePayload
    ├─► 幂等去重 (event_id + 5min TTL)
    ├─► 限流 (token bucket 30/60)
    ├─► 持久化 WebhookEvent
    └─► enqueue → worker.handleJob
                    │
                    ▼
              dispatchTelegram
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
  new_chat_members  left   普通消息 text
        │           │           │
        ▼           │           ▼
  MessageHub        │      MessageHub (MsgType=text)
  (MsgType=event)   │      upsertInboxFromHub
        │           │           │
        ▼           │           ▼
  triggerTelegram   │      triggerSalesEngine
  JoinSales         │      (SmartCSOrchestrator 9 步
        │           │       或 SalesEngine 8 步)
        ▼           │           │
  SalesEngine       │           ▼
  .Handle           │      AI 回复 / 转人工
        │           │           │
        ▼           │           ▼
  tgIntegration     │      tgIntegration
  .SendMessage      │      .SendMessage
  (出站欢迎语)       │      (出站销售话术)
                    ▼
              MessageHub (event, 仅记录)
```

### 2.2 五层架构落位

| 层级 | 文件 | 职责 |
|---|---|---|
| Controller | `user/user-server/internal/controller/telegram_account_controller.go` | Bot 账号 CRUD、RegisterWebhook、TestSend |
| Controller | `user/user-server/internal/controller/webhook_controller.go` | `/api/webhook/telegram/:account_id` 入口 |
| Service | `user/user-server/internal/service/webhook_service.go` | Verify / Receive / dispatchTelegram / triggerTelegramJoinSales / triggerSalesEngine |
| Service | `user/user-server/internal/service/telegram_integration.go` | TelegramIntegrationService.SendMessage 出站发送 |
| Service | `user/user-server/internal/service/sales_engine.go` | SalesEngine 8 步销售管线 |
| Service | `user/user-server/internal/service/smart_cs_orchestrator.go` | SmartCSOrchestrator 9 步统一编排（可选注入） |
| Repository | `user/user-server/internal/repository/feishu.go` | TelegramAccountRepository CRUD |
| Model | `user/user-server/internal/model/feishu.go` | TelegramAccount 模型 |
| Utils | `user/user-server/internal/utils/tgbot/tgbot.go` | SetWebhook / SendMessage 无状态函数 |
| 前端 View | `user/user-web/src/views/telegram/account.vue` | Bot 账号管理 UI |
| 前端 API | `user/user-web/src/api/telegram.js` | 账号 CRUD + Webhook 注册 + 测试发送 |

## 三、后端实现详解

### 3.1 Webhook 验签

Telegram Bot API 通过 `X-Telegram-Bot-Api-Secret-Token` 头传递 webhook secret。该 secret 在调用 `setWebhook` 时由我方指定，存储于 `TelegramAccount.WebhookSecret` 字段。

```go
case ChannelTelegram:
    secret := s.getTelegramWebhookSecret(accountID)
    if secret == "" {
        return true, nil // 兼容商户未设置 secret 的场景
    }
    headerSecret := headers["X-Telegram-Bot-Api-Secret-Token"]
    if headerSecret == "" {
        headerSecret = headers["x-telegram-bot-api-secret-token"]
    }
    if headerSecret == "" {
        return false, errors.New("missing X-Telegram-Bot-Api-Secret-Token header")
    }
    return subtle.ConstantTimeCompare([]byte(headerSecret), []byte(secret)) == 1, nil
```

**安全要点**：
- 使用 `subtle.ConstantTimeCompare` 防止时序攻击
- Secret 未配置时跳过验签（兼容老商户），但生产环境强烈建议配置
- Secret 与 Bot Token 一起存储于 `telegram_accounts` 表

### 3.2 入群事件处理（dispatchTelegram）

当 `new_chat_members` 数组非空时：

1. **过滤 Bot 成员**：仅处理非 bot 的新成员（避免其他 bot 入群触发误触)
2. **构造入群事件消息**：写入 MessageHub（`MsgType=event`、`MsgID=tg_join_{chatID}_{userID}`）
3. **upsertInboxFromHub**：同步到 InboxConversation 收件箱
4. **触发 智能体**：调用 `triggerTelegramJoinSales`

```go
// 入群场景的人设：销售助手主动开场
req.Config.Persona = "你是 Telegram 群组里的销售助手。新用户加入群组时，主动发起一段简洁、亲切的欢迎+销售开场白，引导用户了解产品。回复不超过 80 字。"
```

**关键设计**：入群即销售起点，不依赖用户主动发消息。SalesRequest.UserMessage 用自然语言描述入群事件（"新用户 XXX 刚加入群组「YYY」"），便于 LLM 理解上下文。

### 3.3 普通消息触发 智能体（triggerSalesEngine）

群组或私聊中的每一条 text 消息都会：
1. 写入 MessageHub（`MsgType=text`）
2. upsertInboxFromHub 同步到收件箱
3. 调用 `triggerSalesEngine`：

   - **分支 A（推荐）**：注入了 `SmartCSOrchestrator` → 9 步编排（会话查找/创建 → 入站消息保存 → 座席接管检查 → AI 连续上限检查 → 紧急投诉检查 → SalesEngine.Handle 8 步链路 → AI 建议保存 → 置信度决策）
   - **分支 B（回退）**：直接调用 `SalesEngine.Handle` 8 步链路（resolve customer → recall memory → recognize intent → match SOP → recall RAG → generate candidate → polish → audit）

### 3.4 AI 触发前置条件（shouldTriggerAI）

```go
case ChannelTelegram:
    acc, err := s.telegramRepo.GetByID(uint(accID))
    if err != nil { return false }
    return acc.AIAgentEnabled && acc.Status == 1
```

只有当 `TelegramAccount.AIAgentEnabled == true` 且 `Status == 1`（启用状态）时才会触发 AI。商户可在前端 Bot 账号管理页一键开关。

### 3.5 出站消息发送

```go
if err := s.tgIntegration.SendMessage(ctx, uint(accID), chatIDInt, resp.Reply); err != nil {
    log.Printf("[Webhook] TG 入群欢迎消息发送失败: %v", err)
}
```

`TelegramIntegrationService.SendMessage` 内部调用 `tgbot.SendMessage` 无状态函数，通过 Bot Token 调用 Telegram `sendMessage` API。

## 四、前端 Bot 账号管理

### 4.1 页面路由

| 路由 | 文件 | 功能 |
|---|---|---|
| `/telegram` | 重定向到 `/telegram/account` | 入口 |
| `/telegram/account` | `views/telegram/account.vue` | Bot 账号 CRUD + Webhook 注册 + 测试发送 |

### 4.2 账号管理 UI 功能

- **账号列表**：展示 Bot Token（掩码）、Webhook URL、Webhook Secret（掩码）、智能体开关、状态
- **新增/编辑对话框**：
  - 账号名称、Bot Token（密码框）
  - Webhook URL（自动生成：`{BASE_URL}/api/webhook/telegram/{id}`）
  - Webhook Secret（自动生成 32 位随机字符串）
  - 智能体开关（`AIAgentEnabled`）
  - 状态（启用/禁用）
- **注册 Webhook**：调用后端 `POST /api/telegram/accounts/:id/register-webhook`，后端调用 `tgbot.SetWebhook` 通知 Telegram
- **测试发送**：输入 Chat ID + 消息内容，调用 `POST /api/telegram/accounts/:id/test-send`

### 4.3 前端 API（`src/api/telegram.js`）

| 函数 | 后端接口 | 用途 |
|---|---|---|
| `listAccounts` | `GET /api/telegram/accounts` | 账号列表 |
| `getAccount` | `GET /api/telegram/accounts/:id` | 账号详情 |
| `createAccount` | `POST /api/telegram/accounts` | 创建账号 |
| `updateAccount` | `PUT /api/telegram/accounts/:id` | 更新账号 |
| `deleteAccount` | `DELETE /api/telegram/accounts/:id` | 删除账号 |
| `registerWebhook` | `POST /api/telegram/accounts/:id/register-webhook` | 注册 Webhook |
| `testSend` | `POST /api/telegram/accounts/:id/test-send` | 测试发送 |

## 五、部署与配置

### 5.1 环境变量

| 变量 | 说明 | 示例 |
|---|---|---|
| `WEBHOOK_BASE_URL` | 商户实例公网入口（用于拼接 Webhook URL） | `https://shop.example.com` |
| `TELEGRAM_API_BASE` | Telegram API 基础地址（兼容反代） | `https://api.telegram.org` |

### 5.2 部署步骤

1. **创建 Bot**：在 Telegram 中 @BotFather 创建 Bot，获取 Bot Token
2. **配置公网入口**：确保 `WEBHOOK_BASE_URL` 可被 Telegram 服务器访问（需 HTTPS）
3. **新增账号**：在前端 `/telegram/account` 页面新增 Bot 账号，填入 Bot Token，开启 智能体开关
4. **注册 Webhook**：点击"注册 Webhook"按钮，后端调用 `setWebhook` 通知 Telegram
5. **验证**：在 TG 群组中邀请新成员，观察后端日志是否出现 `[Webhook] TG 入群触发 智能体` 日志，以及群组是否收到 AI 欢迎语

### 5.3 Webhook URL 格式

```
{WEBHOOK_BASE_URL}/api/webhook/telegram/{account_id}
```

`account_id` 是 `telegram_accounts` 表主键，Telegram 在回调时携带该 ID，后端据此查找 Bot Token + WebhookSecret。

## 六、测试覆盖

### 6.1 后端单元测试

测试文件：`user/user-server/internal/service/telegram_ai_sales_test.go`

共 12 个测试用例，覆盖：

| 测试名 | 验证点 |
|---|---|
| `TestDispatchTelegram_JoinEvent_NewChatMembers` | 入群事件写入 MessageHub（MsgType=event, MsgID=tg_join_{chatID}_{userID}） |
| `TestDispatchTelegram_JoinEvent_OnlyBotsSkipped` | 仅 bot 成员入群不产生 event 类型记录 |
| `TestDispatchTelegram_LeftEvent_RecordsToHub` | 退群事件写入 MessageHub |
| `TestDispatchTelegram_RegularMessage_TextToHub` | 普通消息写入 MessageHub（MsgType=text） |
| `TestDispatchTelegram_GroupMessage_IsGroupTrue` | 群组消息 IsGroup=true |
| `TestDispatchTelegram_SystemNotification_Skipped` | 系统通知（new_chat_title 等）跳过 |
| `TestShouldTriggerAI_TelegramAccountStates` | 4 种账号状态判定（AIAgentEnabled × Status） |
| `TestShouldTriggerAI_NilSalesEngineReturnsFalse` | salesEngine 为 nil 返回 false |
| `TestShouldTriggerAI_InvalidAccountIDReturnsFalse` | accountID 非法返回 false |
| `TestTriggerTelegramJoinSales_NilSalesEngineNoCrash` | salesEngine 为 nil 不崩溃 |
| `TestTriggerTelegramJoinSales_ShouldNotTriggerWhenAIDisabled` | AI 关闭时不触发 |
| `TestWebhookService_Receive_TelegramJoinEvent` | 完整 Receive 链路：验签 → 解析 → 入队 → dispatch → MessageHub |

### 6.2 前后端联合测试

| 场景 | 操作 | 预期 |
|---|---|---|
| Bot 账号 CRUD | 新增→编辑→删除 | 列表正确反映变更 |
| Webhook 注册 | 点击注册按钮 | 后端返回成功，Telegram 端可查询到 webhook |
| 入群触发 AI | 在 TG 群邀请新成员 | 群内收到 AI 欢迎语，后端日志记录触发 |
| 消息触发 AI | 在 TG 群发消息 | 群内收到 智能体回复 |
| AI 开关关闭 | 关闭 AIAgentEnabled 后再发消息 | 不触发 AI，仅记录到 MessageHub |

## 七、数据模型

### 7.1 TelegramAccount（`telegram_accounts` 表）

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | uint PK | 账号 ID |
| `bot_token` | string | Bot Token（加密存储） |
| `webhook_url` | string | Webhook 回调地址 |
| `webhook_secret` | string | X-Telegram-Bot-Api-Secret-Token 验签密钥 |
| `ai_agent_enabled` | bool | 智能体开关 |
| `status` | int | 1=启用 / 0=禁用 |
| `created_at` / `updated_at` | timestamp | 时间戳 |

### 7.2 MessageHub（TG 入群事件记录）

| 字段 | 入群事件值 | 普通消息值 |
|---|---|---|
| `platform` | `telegram` | `telegram` |
| `msg_id` | `tg_join_{chatID}_{userID}` | `{message_id}` |
| `msg_type` | `event` | `text` |
| `content` | `[入群事件] 用户 XXX (@YYY) 加入群组 ZZZ` | 原始消息文本 |
| `is_group` | true（群组场景） | 视 chat.type 而定 |
| `group_id` | chatID | chatID（群组场景） |

## 八、商业价值

1. **入群即触达**：新用户入群瞬间即被 智能体触达，转化窗口前置 30 秒
2. **7×24 自动响应**：非工作时间不漏单，智能体引擎持续在线
3. **8 步销售管线**：每条消息都经过完整的销售管线处理，避免人工遗漏
4. **统一消息中台**：所有 TG 事件/消息统一入库，支持销售复盘和客户画像
5. **可扩展**：通过 `SmartCSOrchestrator` 注入可实现座席接管、紧急投诉升级、AI 连续上限等高级编排

## 九、参考资料

- [community-whatsapp.md](community-whatsapp.md)
- [community-wecom.md](community-wecom.md)
- [community-management.md](community-management.md)
- [Telegram Bot API Webhook 文档](https://core.telegram.org/bots/api#setwebhook)
