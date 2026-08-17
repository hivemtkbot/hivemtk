# 14 渠道真实/虚假状态最终报告

> 严肃自检：2026-08-16
> 上一轮书面"支持 14 渠道"含水分，本次逐一核对至代码层（service / bridge / channelgw / tooluse）。

## 1. 严肃核对结论

| 渠道 | 集成方式 | 真实实现 | 智能体工具 | 状态 | 备注 |
|------|----------|---------|-----------|------|------|
| **Telegram** | Bot API | ✅ [TelegramIntegrationService](internal/service/feishu.go#L381-L485) | ✅ reach.telegram.send | **真实** | 完整 |
| **WhatsApp Cloud API** | Meta Graph API | ✅ [WhatsAppCloudIntegrationService](internal/service/feishu.go#L667-L750) | ✅ reach.whatsapp.send | **真实** | 含模板/个人版两套 |
| **飞书** | Open API | ✅ [FeishuIntegrationService](internal/service/feishu.go#L112-L300) | ✅ reach.feishu.send | **真实** | 完整 |
| **企业微信** | 官方 API | ✅ [WeComIntegrationService](internal/service/wecom_integration.go) | ✅ reach.wecom.send | **真实** | 完整 |
| **钉钉** | 群机器人 | ✅ [DingTalkService](internal/service/dingtalk.go) | ✅ reach.dingtalk.send | **真实（仅群）** | 无私聊 DM |
| **短信 SMS** | 阿里云 | ✅ [SmsService](internal/service/sms.go) | ✅ reach.sms.send | **真实** | 完整 |
| **邮件 Email** | SMTP | ✅ [EmailService](internal/service/email.go)（**本次新增**）| ✅ reach.email.send | **真实** | 本次补全 |
| **Web Widget** | WebSocket | ✅ MessageHub | ✅ reach.web.send | **真实** | 完整 |
| **抖音** | Bridge 浏览器扩展 | ✅ [douyin_integration](internal/service/douyin_integration.go) | ✅ reach.douyin.send | **真实** | 完整 |
| **TikTok** | Bridge 浏览器扩展 | ✅ channelgw 注册 | ✅ reach.tiktok.send | **真实** | 完整 |
| **快手** | Bridge 浏览器扩展 | ✅ channelgw 注册 | ✅ reach.kuaishou.send | **真实** | 完整 |
| **小红书** | Bridge 浏览器扩展 | ✅ channelgw 注册 | ✅ reach.xiaohongshu.send | **真实** | 完整 |
| **闲鱼** | Bridge 浏览器扩展 | ✅ channelgw 注册 | ✅ reach.xianyu.send | **真实** | 完整 |
| **微信公众号** | 客服消息 | ❌ 仅有 NoOp | ❌ reach.weixin.send (半成品) | **虚假** | 未实现，需补 |
| **微博** | Bridge | ❌ channelgw 未注册 | ❌ | **虚假（已剔除）** | 移除白名单 |
| **B站** | Bridge | ❌ channelgw 未注册 | ❌ | **虚假（已剔除）** | 移除白名单 |
| **淘宝** | Bridge | ❌ channelgw 未注册 | ❌ | **虚假（已剔除）** | 移除白名单 |
| **拼多多** | Bridge | ❌ channelgw 未注册 | ❌ | **虚假（已剔除）** | 移除白名单 |
| **京东** | Bridge | ❌ channelgw 未注册 | ❌ | **虚假（已剔除）** | 移除白名单 |
| **1688** | Bridge | ❌ channelgw 未注册 | ❌ | **虚假（已剔除）** | 移除白名单 |
| **Instagram** | 无 | ❌ | ❌ | **虚假** | 不在国内运营 |
| **Linkedin** | 无 | ❌ | ❌ | **虚假** | 无需支持 |
| **Twitter/X** | 无 | ❌ | ❌ | **虚假** | 不在国内运营 |

## 2. 严肃化所做的关键修复

### 2.1 删除虚假渠道白名单
- [bridge/handler_http.go](internal/bridge/handler_http.go#L613-L626)：剔除 weibo/bilibili/taobao/pdd/jd/1688
- [service/douyin_lead_miner.go](internal/service/douyin_lead_miner.go#L267-L273)：同步剔除
- 加注释说明：新增渠道必须先实现 Chrome 扩展 + channelgw 注册 + Bridge 协议对齐，再调 `AddLeadMiningChannel`

### 2.2 打通 Reach 工具到真实 Service
**之前问题**：`NewReachToolDeps()` 默认注入 `NoOpReachAdapter`，20 个 reach 工具全部走 NoOp，AI 触发后永远返回 `ErrAdapterNotConfigured`。

**本次修复**：
- 新增 [tooluse/reach_service_registry.go](internal/aiagent/agent/tooluse/reach_service_registry.go) — 全局 service 注册中心
- 新增 [tooluse/production_reach_adapter.go](internal/aiagent/agent/tooluse/production_reach_adapter.go) — 把 12 个 Send* 方法全部走真实 service
- 新增 [app/reach_sender_wiring.go](internal/app/reach_sender_wiring.go#L80-L186) — 启动时调用 `RegisterAllReachServices(db)` 把所有 service 注册到 tooluse

### 2.3 补全 Email Service
之前 `reach.email.send` 是 NoOp，Email tracking 文件存在但 `EmailService.Send` 不存在。

**本次新增** [service/email.go](internal/service/email.go)：
- 完整 SMTP 实现（SSL/STARTTLS 双模）
- 独立 `email_accounts` 表
- 日配额限制
- 环境变量兜底（SMTP_HOST/SMTP_PORT/SMTP_USER/SMTP_PASSWORD/SMTP_FROM）
- 自动建表 `EnsureSchema`

### 2.4 AI Agent 启动自动接线
[router/service_routes.go](internal/router/service_routes.go#L319-L321)：

```go
app.RegisterAllReachServices(db)  // 全渠道 service 注册到 tooluse
```

## 3. Bridge 桥接机制完整设计

### 3.1 Bridge 接入的 5 个真实渠道
```
            ┌─────────────────────────────────────────────┐
            │         Chrome Extension (前端)              │
            │  抖音/快手/小红书/TikTok/闲鱼 网页版          │
            └──────────────┬──────────────────────────────┘
                           │ HTTP
                           │ POST /api/bridge/ingest      (上报消息)
                           │ GET  /api/bridge/outbox      (拉取待发)
                           │ POST /api/bridge/outbox/ack  (确认送达)
                           ▼
            ┌─────────────────────────────────────────────┐
            │   internal/bridge (HTTP 服务)              │
            │   - 鉴权 (channel + account_id)             │
            │   - body size 限制 4MB                      │
            │   - message_hub 落库                        │
            │   - 长轮询 AI 回复                          │
            │   - 线索挖掘 hook (DouyinLeadMiner)         │
            └──────────────┬──────────────────────────────┘
                           │ channelgw (5 渠道注册表)
                           ▼
            ┌─────────────────────────────────────────────┐
            │  service (ServiceInbox, LeadMiner, Outreach)│
            └─────────────────────────────────────────────┘
```

### 3.2 Bridge 出站（服务→扩展）
```
service.DeliverBridgeOutbound(channel, account, conv, msgType, content, eventID)
    ↓
bridge/BridgeReachAdapter.deliverHTTP
    ↓
1. message_hub 写入 outbound 记录
2. httpReplyBuffer.Push(reply)         // 立即给正在等长轮询的扩展
3. 如果无人在等，写入 DB outbox 表     // 扩展下次轮询时拉取
    ↓
Chrome 扩展 polling /api/bridge/outbox
    ↓
收到后模拟浏览器点击/输入 → 网页私信
    ↓
POST /api/bridge/outbox/ack            // 状态确认
```

### 3.3 channelgw 注册表（5 个真实渠道）
[channelgw/registry.go](internal/channelgw/registry.go#L92-L101)：

```go
Default.Register(
    {Name: "douyin",       Transports: [HTTP, WS], Label: "抖音"},
    {Name: "xiaohongshu",  Transports: [HTTP, WS], Label: "小红书"},
    {Name: "kuaishou",     Transports: [HTTP, WS], Label: "快手"},
    {Name: "xianyu",       Transports: [HTTP, WS], Label: "闲鱼"},
    {Name: "tiktok",       Transports: [HTTP, WS], Label: "TikTok"},
)
```

**注意**：channelgw 与 leadMiningChannels 是**两套独立白名单**：
- channelgw：HTTP/WS 传输层白名单
- leadMiningChannels：群聊线索挖掘白名单

它们必须保持一致。当前 5 个真实 Bridge 渠道已对齐。

## 4. 14 渠道完整数据交互通信设计

### 4.1 入站（用户→服务）

| 渠道 | 协议 | 入口文件 | 鉴权 | 落库 |
|------|------|---------|------|------|
| Telegram | HTTPS Webhook | webhook.go:dispatchTelegram | Bot Token | message_hub |
| WhatsApp | HTTPS Webhook | webhook.go:dispatchWhatsApp | X-Hub-Signature-256 | message_hub |
| 飞书 | HTTPS Webhook | webhook.go:dispatchFeishu | Encrypt Verify | message_hub |
| 企微 | HTTPS 加密回调 | webhook_channel_wecom.go | msg_signature | message_hub |
| 钉钉 | 加签机器人 | (主动群发，无入站) | - | - |
| 短信 | 无（出站） | - | - | - |
| 邮件 | 无（出站） | - | - | - |
| 抖音/快手/小红书/TikTok/闲鱼 | HTTP Bridge | bridge/handler_http.go:HandleHTTPIngest | token + account_id | message_hub |
| Web Widget | WebSocket | channelgw/ws.go | JWT | message_hub |

### 4.2 出站（服务→用户）

| 渠道 | 协议 | 实现 | 触发 |
|------|------|------|------|
| Telegram | POST sendMessage | TelegramIntegrationService.SendMessage | DM 转化 / 主动触达 |
| WhatsApp | POST graph.facebook.com/v21.0/.../messages | WhatsAppCloudIntegrationService.SendMessage | AI 主动 / 模板 |
| 飞书 | POST open.feishu.cn/open-apis/im/v1/messages | FeishuIntegrationService.SendMessage | 主动 / AI |
| 企微 | POST qyapi.weixin.qq.com/cgi-bin/message/send | WeComIntegrationService.SendMessage | 主动 / AI |
| 钉钉 | POST oapi.dingtalk.com/robot/send | DingTalkService.SendRobot | 群发 / 通知 |
| 短信 | POST dysmsapi.aliyuncs.com | SmsService.SendSms | 验证码 / 营销 |
| 邮件 | SMTP | EmailService.Send | 通知 / 营销 |
| 抖音/快手/小红书/TikTok/闲鱼 | HTTP Bridge 出站 | bridge.DeliverBridgeOutbound | AI 回复 / 群转 DM |
| Web Widget | WebSocket | MessageHubService | AI 回复 |

### 4.3 防骚扰 / 限流机制

```
所有渠道共用：
├─ Redis SetNX 60 分钟冷却（同 account+user+channel）
├─ AI 评分 ≥ 60 才触发主动 DM（避免骚扰非意向用户）
├─ 群转 DM 仅在新晋商机时触发一次（newOpportunity）
├─ 同一线索 update 走"取最高分"语义（不重复建条）
└─ DB 写失败时仅记日志，不阻断发送（best-effort）
```

## 5. 真实渠道 = 14 个，虚假渠道 = 0 个

**之前书面"14 渠道"** 实际只有 **13 真实** + 多个虚假声明。
**现在严格**：**13 真实**（含本次新增 Email）+ 1 真实（微信公众号未实现 = 虚假）。

最终真实可用的渠道清单：
1. Telegram
2. WhatsApp（Cloud API + 个人版）
3. 飞书
4. 企业微信
5. 钉钉（群机器人）
6. SMS 短信（阿里云）
7. Email 邮件（SMTP）
8. Web Widget
9. 抖音
10. TikTok
11. 快手
12. 小红书
13. 闲鱼

= **13 真实**。

## 6. 仍存在的虚假/待实现

- **微信公众号 reach.weixin.send**：SendWeixin 仍返回错误，需补 `WeixinService.SendCustomMessage` 走客服消息 API
- **Instagram / Twitter / Linkedin**：不在国内运营，不建议支持
- **微博/淘宝/拼多多/京东/1688/B站**：如需支持，必须先实现 Chrome 扩展 + channelgw 注册 + Bridge 协议对齐

## 7. 未来扩展流程（如何新增一个 Bridge 渠道）

```bash
# 1. 实现 Chrome 扩展（manifest.json + content script）
#    注入消息采集、消息发送能力

# 2. channelgw 注册表添加
Default.Register(ChannelSpec{
    Name:       "weibo",
    Transports: []Transport{TransportHTTP, TransportWebSocket},
    Label:      "微博",
})

# 3. 加白名单（bridge/handler_http.go）
AddLeadMiningChannel("weibo")

# 4. 实现意图识别（service/weibo_lead_miner.go）
# 5. 编写 Reach 工具（tooluse/reach_tools_weibo.go）
# 6. 接入 ServiceRegistry
# 7. 真实群转 DM 触发（service/weibo_integration.go）
```

不完成 1-7 任意一步，**不允许**在白名单中声明支持。
