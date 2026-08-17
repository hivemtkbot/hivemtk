# 主动触达客户完全指南

## 1. 为什么需要主动触达

当 AI 客服/销售回复用户消息后，用户可能已读不回、未留下联系方式，或需要二次跟进。此时必须能够主动联系客户，否则销售链路断裂。

本系统支持 **14 种触达渠道**：

| 渠道 | 入口 | 是否需要用户先发起对话 |
|------|------|---------------------|
| Telegram | Bot API | ✅ 用户需先私聊 Bot |
| Douyin | Bridge（Chrome 扩展） | ❌ 群聊发现线索后可主动私信 |
| TikTok | Bridge | ❌ |
| Kuaishou | Bridge | ❌ |
| 小红书 | Bridge | ❌ |
| 闲鱼 | Bridge | ❌ |
| 微博 | Bridge | ❌ |
| 淘宝 | Bridge | ❌ |
| 拼多多 | Bridge | ❌ |
| 京东 | Bridge | ❌ |
| 1688 | Bridge | ❌ |
| WhatsApp | Cloud API / 个人版 | 模板消息：否 / 文本：需 24h 客服窗口 |
| 飞书 | Open API | ✅ 需用户先添加 Bot |
| 企业微信 | API | ✅ 需用户为外部联系人 |
| Email | SMTP | 否 |
| SMS | 短信网关 | 否 |

## 2. 触达服务设计

### 2.1 ProactiveReachService

文件：[internal/service/proactive_reach.go](internal/service/proactive_reach.go)

统一入口，根据用户身份自动选择最优渠道：

```go
resp, err := svc.ProactiveReach(ctx, &service.ProactiveReachRequest{
    CustomerID: "cust_001", // 可选
    Channel:    "",         // 空则自动选择
    UserID:     "123456789",
    Phone:      "+8613800138000",
    Email:      "user@example.com",
    Content:    "您好，关于您的咨询...",
})
```

自动选择策略（按优先级）：
1. `UserID` 为纯数字 → Telegram
2. `UserID` 存在 → 尝试 Bridge 渠道（抖音/快手/小红书...）
3. `Phone` 存在 → WhatsApp → SMS（降级）
4. `Email` 存在 → Email
5. 以上均失败 → 返回错误

### 2.2 渠道特定实现

- Telegram：[telegram_lead_outreach.go](internal/service/telegram_lead_outreach.go)
- Douyin/Bridge：[douyin_integration.go](internal/service/douyin_integration.go)
- WhatsApp：[whatsapp.go](internal/service/whatsapp.go)（新增 Cloud API 支持）
- 飞书：[feishu.go](internal/service/feishu.go) 已有 `SendMessage`
- 企业微信：[wecom_integration.go](internal/service/wecom_integration.go) 已有 `SendMessage`

## 3. 群聊线索转私信

### 3.1 Telegram

流程：
1. Webhook 收到群消息
2. `mineTelegramGroupLead` 进行 5 维意向打分
3. 首次达 60 分触发 `triggerTGDMOutreach`
4. 检查 60 分钟 Redis 冷却
5. 调用 `TelegramDMOutreachService.TriggerDMOutreach` 发送私信
6. 记录事件到 `message_hub`

注意：Telegram Bot 必须用户先私聊过才能发送消息，否则发送会失败。

### 3.2 Douyin / 所有 Bridge 渠道

流程：
1. Bridge HTTP Ingest 收到扩展上报
2. `DouyinLeadMiner()` 识别群消息
3. `DetectDouyinIntent` 打分
4. 首次达 60 分触发 `triggerDouyinDMOutreach`
5. 检查 60 分钟 Redis 冷却
6. `DeliverBridgeOutbound` 写入下发队列
7. Chrome 扩展轮询 `/api/bridge/outbox` 拉取任务并发送

2026-08-16 更新：所有 Bridge 网页渠道（闲鱼、微博、淘宝、拼多多、京东、1688、B站）统一复用抖音打分模型。

## 4. HTTP API

### 4.1 主动发送

```bash
POST /api/reach/proactive/send
Content-Type: application/json

{
  "channel": "telegram",
  "account_id": "bot_account_1",
  "user_id": "123456789",
  "content": "您好，看到您的咨询..."
}
```

### 4.2 快速发送

```bash
POST /api/reach/proactive/quick
{
  "channel": "douyin",
  "account_id": "acc1",
  "user_id": "openid_xxx",
  "content": "您好..."
}
```

### 4.3 从客户 ID 自动触达

```bash
POST /api/reach/proactive/customer/cust_001
{
  "content": "您好，关于您的咨询...",
  "channel": "whatsapp"
}
```

### 4.4 批量触达

```bash
POST /api/reach/proactive/batch
{
  "targets": [
    {"channel": "telegram", "user_id": "123", "content": "hi"},
    {"channel": "douyin", "user_id": "456", "content": "hi"}
  ]
}
```

## 5. 防骚扰机制

- Redis 60 分钟冷却：同一用户同一渠道 60 分钟内只触达一次
- 群聊转私信：每账号每用户每群独立冷却键
- 批量任务：自动跳过冷却期内用户

## 6. 未完成任务

- [ ] 将 `sendTelegramMessage/sendWhatsAppMessage/sendEmailMessage/sendSMSMessage/sendWeComMessage/sendFeishuMessage` 占位函数替换为真实集成调用
- [ ] 在 `router.go` 中注入真实的 `ReachSender` 而非 `nil`
- [ ] WhatsApp Cloud API 模板列表管理 UI
- [ ] Bridge 扩展对闲鱼/微博/淘宝等新渠道的 DOM 注入支持
- [ ] 触达效果追踪（送达率、已读率、回复率）

## 7. 二次论证要点

1. **入口统一**：所有渠道消息都进入 `MessageHub`，触达也统一走 `ProactiveReachService`
2. **数据统一**：线索/客户/触达事件全部关联到统一 OneID
3. **扩展性**：新增渠道只需在 `ProactiveReachService` 增加一个 case + 实现一个 sendXxx 函数
4. **合规性**：WhatsApp 24h 外必须走模板，Telegram 需用户先私聊 Bot，均已通过代码分支控制
