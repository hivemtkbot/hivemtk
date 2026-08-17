# 13 渠道全链路 + 智能选渠道 完全指南

> 2026-08-16 严肃化交付（2026-08-17 终极交付：13 渠道全部贯通 + AI 工具到 Service 全链 + Bridge 5 渠道 Chrome 扩展）
> 关键变革：从"逐个渠道猜测"改为"按客户 OneID 完整信息智能选渠道"

## 1. 客户线索 13 渠道 OneID 完整设计

### 1.1 Customer 模型 13 渠道身份字段

[model/customer.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/model/customer.go) 完整覆盖：

```go
// 强标识（OneID 主键候选）
Phone         string  // SMS / WhatsApp 通用
Email         string  // Email

// 13 渠道 OpenID / UserID
TelegramChatID  int64
TelegramUsername string
WhatsAppPhone   string
WechatOpenID    string
FeishuOpenID    string
WeComExternalID string
DouyinOpenID    string
TikTokOpenID    string
KuaishouOpenID  string
XiaohongshuID   string
XianyuID        string
```

每个客户都有完整的多渠道身份映射，触达时**不再猜测**。

### 1.2 UnifiedID 生成优先级

```go
// 优先级排序（model/customer.go:GenerateCustomerUnifiedID）
1. phone              → "phone:13800138000"
2. email              → "email:user@example.com"
3. whatsapp_phone     → "whatsapp:+8613800138000"
4. telegram_chat_id   → "telegram:123456789"
5. wechat_open_id     → "wechat:oXyzABC"
6. feishu_open_id     → "feishu:ou_xyz"
7. wecom_external_id  → "wecom:wm_xyz"
8. douyin_open_id     → "douyin:dy_xyz"
9. tiktok_open_id     → "tiktok:tt_xyz"
10. kuaishou_open_id  → "kuaishou:ks_xyz"
11. xiaohongshu_id    → "xiaohongshu:xhs_xyz"
12. xianyu_id         → "xianyu:xy_xyz"
13. fallback → uuid
```

### 1.3 CustomerChannels 副表

[model/customer_channel.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/model/customer_channel.go)：

```go
type CustomerChannel struct {
    OneID         string  // 客户 OneID
    Channel       string  // 渠道名
    ChannelUserID string  // 该渠道上的用户 ID
    ChannelName   string  // 用户名/昵称
    AccountID     string  // 触达账号
    IsPrimary     bool    // 是否首选渠道
    PreferredRank int     // 触达优先级
    LastSeenAt    time.Time
}
```

一个客户在不同渠道的多个身份都通过 OneID 聚合。

---

## 2. 智能选渠道（核心反模式修复）

### 2.1 旧的反模式（必须废弃）

```go
// 之前 proactive_reach.go（已删除）
for _, ch := range []string{"telegram", "whatsapp", "wechat", "douyin", ...} {
    err := trySend(ch, ...)
    if err == nil { return success }
}
// 错误：盲目猜测、没有客户信息、利用率极低
```

### 2.2 新的智能选渠道逻辑

[service/proactive_reach.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/service/proactive_reach.go)：

```go
func (s *ProactiveReachService) ReachByCustomer(ctx, req) {
    // 1. 显式指定 phone/email → 直发该渠道
    if req.Phone != "" { return sendSMS(...) }
    if req.Email != "" { return sendEmail(...) }

    // 2. 加载客户完整信息
    customer := s.customerRepo.GetByID(req.CustomerID) // 或 GetByUnifiedID(req.OneID)

    // 3. 列出客户有完整身份的所有渠道
    available := customer.AvailableChannels(preferred)

    // 4. 从副表加载客户偏好排序
    preferred := s.loadCustomerPreferredOrder(customer.OneID, available)

    // 5. 智能选渠道（按顺序找第一个有 active 账号的）
    channel, recipient, accountID = s.pickChannel(available, customer)

    // 6. 按渠道分发到真实 service
    switch channel {
    case "sms":       return sendSMS(...)
    case "email":     return sendEmail(...)
    case "telegram":  return sendTelegram(...)
    case "whatsapp":  return sendWhatsApp(...)
    case "wecom":     return sendWeCom(...)
    case "feishu":    return sendFeishu(...)
    case "douyin" | "tiktok" | "kuaishou" | "xiaohongshu" | "xianyu":
                      return sendBridge(channel, ...)
    }

    // 7. 全部不可用 → 明确错误（带详细原因）
    return error("no active account for customer ...")
}
```

### 2.3 选渠道策略（不猜测）

```
优先级 1：显式 phone/email  → SMS / Email 直发
优先级 2：客户偏好渠道     → CustomerChannels.preferred_rank ASC
优先级 3：默认可靠渠道     → SMS > Email > Telegram > WhatsApp > 企微 > 微信 > 飞书
优先级 4：Bridge 渠道       → 抖音 > TikTok > 快手 > 小红书 > 闲鱼
优先级 5：第一个有 active 账号的渠道胜出
```

---

## 3. 13 渠道完整业务链（每个渠道都有"配置入口 + 对话接入 + 线索挖掘 + 主动触达"）

| 渠道 | 配置入口 | 对话接入 | 线索挖掘 | 主动触达 | 完整业务链状态 |
|------|---------|---------|---------|---------|--------------|
| **Telegram** | [setupTelegramRoutes](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/router/platform_routes.go#L51-L54) | webhook → SalesEngine | LeadMiner | SendMessage | ✅ 完整 |
| **WhatsApp Cloud** | setupWhatsAppCloudRoutes | webhook → SalesEngine | LeadMiner | SendMessage | ✅ 完整 |
| **WhatsApp 个人版** | setupWhatsappRoutes | webhook | LeadMiner | SendMessage | ✅ 完整 |
| **飞书** | setupFeishuRoutes | webhook → SalesEngine | LeadMiner | SendMessage | ✅ 完整 |
| **企业微信** | setupWeComRoutes | webhook → SalesEngine | LeadMiner | SendMessage | ✅ 完整 |
| **钉钉** | setupDingTalkAppRoutes | webhook | LeadMiner | SendRobot | ✅ 完整（仅群） |
| **SMS 短信** | setupSmsRoutes | 无（出站） | LeadMiner | SendSms | ✅ 完整 |
| **Email 邮件** | setupEmailRoutes | 无（出站） | LeadMiner | Send | ✅ 完整 |
| **抖音** | [Bridge 扩展](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/bridge/handler_http.go#L613-L626) | HTTP ingest | LeadMiner | BridgeOutbound | ✅ 完整 |
| **TikTok** | setupTiktokRoutes | HTTP ingest | LeadMiner | BridgeOutbound | ✅ 完整 |
| **快手** | [Bridge 扩展](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/bridge/src/content/kuaishou.js) | HTTP ingest | LeadMiner | BridgeOutbound | ✅ 完整 |
| **小红书** | [Bridge 扩展] | HTTP ingest | LeadMiner | BridgeOutbound | ✅ 完整 |
| **闲鱼** | [Bridge 扩展] | HTTP ingest | LeadMiner | BridgeOutbound | ✅ 完整 |
| **微信公众号** | [setupWechatRoutes](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/router/platform_routes.go#L82-L90) | webhook → SalesEngine | LeadMiner | SendCustomMessage | ✅ 完整 |

### 3.1 统一配置入口（新）

[controller/channel_overview.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/controller/channel_overview.go) + [router/channel_overview_routes.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/router/channel_overview_routes.go)：

#### `GET /api/channels/overview`
列出所有 13 渠道的当前状态：
```json
{
  "channels": [
    {
      "channel": "telegram",
      "channel_name": "Telegram Bot",
      "category": "official_api",
      "account_count": 5,
      "active_count": 4,
      "integration_ready": true,
      "required_fields": ["bot_token", "webhook_secret", "agent_id"],
      "config_urls": ["/api/telegram/accounts", "..."],
      "health_url": "/api/webhook/telegram/{account_id}"
    },
    {
      "channel": "douyin",
      "channel_name": "抖音（Bridge）",
      "category": "bridge",
      "account_count": 3,
      "online_count": 2,
      "integration_ready": true,
      "required_fields": ["chrome_extension_installed", "user_login_douyin"],
      "config_urls": ["Chrome 扩展: ingestBridge.js", "..."],
      "health_url": "/api/bridge/outbox?channel=douyin&account_id=..."
    },
    ...
  ],
  "total_channels": 13,
  "real_channels": 13,
  "bridge_channels": 5,
  "official_channels": 8
}
```

#### `POST /api/channels/bind`
手动绑定客户到某渠道（用于补全 OneID 信息）：
```json
{
  "customer_id": "cust_001",
  "channel": "telegram",
  "channel_user_id": "123456789",
  "channel_name": "Alice",
  "account_id": "bot_1",
  "is_primary": true
}
```

#### `GET /api/channels/customer/:customer_id`
列出客户的所有渠道绑定：
```json
{
  "customer_id": "cust_001",
  "one_id": "phone:13800138000",
  "name": "张三",
  "phone": "13800138000",
  "email": "user@example.com",
  "bindings": [
    {"channel": "telegram", "channel_user_id": "123456789", "is_primary": true, ...},
    {"channel": "douyin", "channel_user_id": "dy_xxx", "is_primary": false, ...}
  ],
  "total": 2
}
```

---

## 4. 13 渠道完整业务链实现细节

### 4.1 官方 API 渠道（7 个）
全部走标准 `webhook → 入站解析 → message_hub 落库 → SalesEngine 决策 → 出站 service`：

```
Telegram Bot / 飞书 / 企微 / 钉钉 / WhatsApp Cloud:
  POST /api/webhook/{channel}/{account_id}  →  [webhook.go]  →  [service]
                                                              ↓
                                                       InboxIngress
                                                              ↓
                                                       AI Agent (SalesEngine)
                                                              ↓
                                                       service.Send*  →  真实渠道 API
```

### 4.2 Bridge 渠道（5 个：抖音/快手/小红书/TikTok/闲鱼）

```
Chrome 扩展监听网页私信  →  POST /api/bridge/ingest
                                  ↓
                            BridgeIngestHandler
                                  ↓
                            message_hub 落库
                            群聊线索挖掘 (LeadMiner)
                                  ↓
                            InboxIngress
                                  ↓
                            AI Agent (SalesEngine)
                                  ↓
                            message_hub (outbound)
                                  ↓
                            Bridge Outbound →  扩展 polling
                                  ↓
                            扩展执行网页操作（发送私信）
```

### 4.3 SMS / Email（出站专用）

```
AI Agent 触发 reach.sms.send
  → SmsService.SendSms
  → 阿里云 API
  → 落库 message_hub (outbound)

AI Agent 触发 reach.email.send
  → EmailService.Send
  → SMTP
  → 落库 message_hub (outbound)
```

---

## 5. AI Agent 触达工具与 Service 全链接通

[app/reach_sender_wiring.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/app/reach_sender_wiring.go) 启动时调用 `RegisterAllReachServices(db)`：

```go
registry := tooluse.GlobalServiceRegistry()
registry.RegisterSMS(&smsLikeAdapter{...})         // 阿里云
registry.RegisterEmail(&emailLikeAdapter{...})     // SMTP
registry.RegisterWeCom(&weComLikeAdapter{...})     // 企微
registry.RegisterFeishu(&feishuLikeAdapter{...})    // 飞书
registry.RegisterTelegram(&telegramLikeAdapter{...}) // Telegram
registry.RegisterWhatsApp(&whatsAppLikeAdapter{...}) // WhatsApp
registry.RegisterDingTalk(&dingTalkLikeAdapter{...}) // 钉钉
registry.RegisterWechat(&wechatLikeAdapter{...})   // 微信公众号
tooluse.RegisterBridgeOutboundDeliver(...)         // Bridge 5 渠道
```

现在 AI Agent 触发 `reach.*.send` 不再是 NoOp，全部走真实 service。

---

## 6. 防骚扰与幂等

- 60 分钟冷却（Redis SetNX，per OneID）
- AI 评分 ≥ 60 才触发主动 DM（避免骚扰非意向用户）
- 群转 DM 仅在新晋商机时触发一次
- 客户线索去重：相同 (channel, account) 只建一条线索
- 触达失败重试：渠道发不出去时，AI Agent 不会无限重试（由 Pipeline 控制）

---

## 7. 启动流程

[router/router.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/router/router.go) 启动时：

```go
// 1. 注册全渠道 service 到 tooluse GlobalServiceRegistry
app.RegisterAllReachServices(db)        // setupReachPipelineRoutes 中调用

// 2. 注册主动触达 service
proactiveSvc := service.NewProactiveReachService(db, nil)
service.BindProactiveReachSenders(proactiveSvc, db)
proactiveCtrl := controller.NewProactiveReachController(proactiveSvc)

// 3. 挂载路由
auth.POST("/reach/proactive/send", proactiveCtrl.ProactiveSend)
auth.POST("/reach/proactive/quick", proactiveCtrl.QuickSend)
auth.POST("/reach/proactive/customer/:customer_id", proactiveCtrl.ProactiveSendFromCustomer)
auth.GET("/reach/proactive/customer/:customer_id/channels", proactiveCtrl.ListChannels)
auth.POST("/reach/proactive/batch", proactiveCtrl.BatchProactiveSend)
auth.POST("/reach/proactive/validate", proactiveCtrl.ValidateReach)

// 4. 13 渠道统一概览
auth.GET("/channels/overview", ov.Overview)
auth.POST("/channels/bind", ov.BindChannel)
auth.GET("/channels/customer/:customer_id", ov.ListCustomerChannels)
```

---

## 8. 完整 API 列表

### 8.1 主动触达（智能选渠道）
- `POST /api/reach/proactive/send` — 指定 customer/one_id + content，智能选渠道
- `POST /api/reach/proactive/quick` — 指定 channel + user_id/phone/email
- `POST /api/reach/proactive/customer/:customer_id` — 按客户 ID 触达
- `GET /api/reach/proactive/customer/:customer_id/channels` — 列出客户可用渠道
- `POST /api/reach/proactive/batch` — 批量触达
- `POST /api/reach/proactive/validate` — 验证触达条件

### 8.2 渠道概览与绑定
- `GET /api/channels/overview` — 13 渠道状态总览
- `POST /api/channels/bind` — 客户渠道绑定
- `GET /api/channels/customer/:customer_id` — 客户所有渠道绑定

### 8.3 触达 Pipeline（ReachPipeline）
- `GET /api/reach/pipelines` — 列出 Pipeline
- `POST /api/reach/pipelines` — 创建 Pipeline
- `GET /api/reach/pipelines/:id` — Pipeline 详情
- `PUT /api/reach/pipelines/:id` — 更新
- `DELETE /api/reach/pipelines/:id` — 删除
- `POST /api/reach/pipelines/:id/pause|resume|archive` — 状态
- `POST /api/reach/jobs` — 入队任务
- `GET /api/reach/jobs` — 任务列表
- `POST /api/reach/jobs/:id/cancel|retry|execute` — 任务操作
- `GET /api/reach/stats` — 统计

---

## 9. 验证清单

- [x] Customer 模型 13 渠道身份字段齐全
- [x] UnifiedID 生成覆盖 13 渠道
- [x] CustomerChannels 副表支持多渠道绑定
- [x] ProactiveReachService 按 OneID 智能选渠道
- [x] 13 渠道都有 Controller + Router（入口齐全）
- [x] AI Agent reach 工具全部接通真实 service
- [x] Bridge 5 渠道（抖音/快手/小红书/TikTok/闲鱼）支持完整 ingest/outbound
- [x] 快手 Chrome 扩展已支持（content-kuaishou.js + channel-kuaishou.js）
- [x] 微信公众号已实现（WeChatService + WeChatController + register + webhook）
- [x] 微信公众号注册到 AI Agent 服务链（reach_sender_wiring.go: RegisterWechat）
- [x] 60 分钟冷却防骚扰
- [x] 编译通过（go build + npm build）
- [x] 前端测试通过（npm test）
- [x] **IntegrationReachAdapter.SendWeixin 已打通** — 不再返回"未实现"，通过 GlobalServiceRegistry 调用 WechatService
- [x] **ProactiveReachService.sendWeChat 已打通** — 添加 wechatRegistry + SetWechatRegistry，BindProactiveReachSenders 注册真实 WechatService
- [x] **pipelineReachSender.SendReach 支持 xianyu** — 补全 xianyu 渠道分支（之前遗漏）
- [x] **微信公众号 webhook 对接 Ingress 管道** — 两个 WechatController 实例（auth + webhook）均注入 IngressSvc

## 10. 多角度头脑风暴审查（2026-08-17 终极审视）

### 角度 1: 代码逻辑正确性
- ✅ WechatService.SendCustomMessage 增加 `accountID=0` 自动 fallback 到 GetFirstActiveAccount
- ✅ ProactiveReachService.sendWeChat 接受 req.AccountID 而非硬编码 0
- ✅ WechatController.handleIncomingMessage 增加 panic recovery + 30s 超时 + 标准 ConversationID 格式

### 角度 2: 错误处理与边界
- ✅ 微信公众号所有公开方法（ListAccounts/GetAccount/CreateAccount/UpdateAccount/DeleteAccount/GetFirstActiveAccount）均加 nil DB 保护
- ✅ ChannelOverviewController 16 个 count 函数统一改用 safeCount()，DB 错误有日志（不再静默吞错）
- ✅ BindChannel 中 customers 表更新错误改为 Warnf 日志

### 角度 3: 跨渠道一致性
- ✅ 添加 model.SenderTypeCustomer / SenderTypeAI / SenderTypeSystem / SenderTypeAgent 常量
- ✅ wechat controller 的 SenderType 由魔法字符串 "customer" 改为 model.SenderTypeCustomer
- ✅ ConversationID 格式统一为 `channel:accountID:openID`（与 Bridge 渠道对齐）

### 角度 4: 关联性
- ✅ WechatService.GetFirstActiveAccount 新方法（智能选渠道兜底）
- ✅ SendCustomMessage 与 ProactiveReachService.sendWeChat 都支持 accountID 透传
- ✅ BindProactiveReachSenders 注册的 wechatRegistry 使用真实 WechatService

### 角度 5: 安全性与鉴权
- ✅ CORS 已严格化（仅 chrome-extension:// + CORS_ALLOW_ORIGINS 白名单）
- ✅ 微信公众号 webhook 走 VerifySignature 校验
- ✅ 公共 API 必须经过 JWTAuthMiddleware

### 角度 6: 性能与并发
- ✅ SendPipeline 9 步 + 限流已就位
- ✅ 微信公众号 AccessToken 自动刷新 + 缓存
- ✅ goroutine 内 panic recovery 防止进程崩溃

### 角度 7: 可观测性
- ✅ 关键路径都有 logger.Infof / Warnf / Errorf 日志
- ✅ wechat_count_failed / wechat_active_count_failed 等具体 label
- ✅ handleIncomingMessage panic 时记录 account/from/panic 内容

### 角度 8: 代码规范与一致性
- ✅ 所有错误检查后不再吞错
- ✅ WechatService 4 个新单测（nil DB 场景）
- ✅ 文件命名无 v1/v2
- ✅ Go vet 编译通过；前端 build 通过
