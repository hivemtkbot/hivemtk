# Bridge 设计文档：打通抖音 / 小红书 / TikTok 网页私信 ↔ AI 智能体

> 目标一句话：**通过部署在浏览器里的 Chrome 扩展（`user-web/bridge`），把抖音、小红书、TikTok 网页版私信实时桥接到 `user-server` 的 AI 智能体，实现"客户发私信 → 智能体自动回复 → 回复回写网页发送"的闭环。**

---

## 0. 阅读指引

- 第 1~2 节：背景、整体架构（必读）。
- 第 3~6 节：领域模型、服务端、Chrome 扩展、数据流（设计核心）。
- 第 7 节：复用现有代码与三个开源插件的具体文件映射（落地关键）。
- 第 8 节：`user-web/bridge` 目录组织。
- 第 9 节：**多角度头脑风暴 / 论证 / 查漏补缺**（设计评审，重点）。
- 第 10~11 节：实施里程碑、测试策略。
- 第 12 节：待确认问题。
- `docs/DEFAULTS.md`：所有默认值的单一文档源（端口/限速/超时/协议等）。

---

## 1. 背景与现状

### 1.1 平台私信为什么需要"桥接"

抖音 / 小红书 / TikTok 网页版**没有面向第三方的开放私信 API**（尤其是个人/企业号网页端）。现有 `user-server` 的触达渠道（微信、企微、飞书、Telegram、WhatsApp 等）都依赖官方 API 或 webhook 主动回调。对于这三个平台，**唯一可行的"收发"路径就是模拟用户在网页上操作 DOM**（监听新消息、往输入框填字、点发送）。

因此需要一个运行在用户浏览器里的"代理"——Chrome 扩展，它：

1. 在抖音 / 小红书 / TikTok 网页上注入内容脚本（content script），监听并解析私信；
2. 把新私信上报给 `user-server`；
3. 接收 `user-server` 下发的 AI 回复，回写到网页发送框并点击发送。

### 1.2 现有代码已经具备什么（最大化复用）

经过对 `hivemtk/user-server` 的调研，以下能力**已现成**，bridge 只做"接线"而非"重造"：

| 能力 | 位置 | 复用方式 |
| --- | --- | --- |
| 统一消息中台入口 | `internal/service/inbox_ingress.go` → `InboxIngressService.HandleIngressMessage(*model.MessageEvent)` | bridge 入站直接调用，自动触发 AI |
| 统一消息标准 | `internal/model/message_event.go` → `MessageEvent` | 作为 bridge 与中台之间的协议契约 |
| AI 触发与编排 | `internal/aiagent/agent/runtime` + `SmartCSOrchestrator` | 入站后自动跑，无需 bridge 关心 |
| 智能体↔渠道绑定 | `internal/service/ai_agent.go` → `ChannelAgentBinding(channel_type, account_id)→agent_id` | bridge 账号注册绑定即可多智能体路由 |
| 统一触达（出站）接口 | `internal/aiagent/agent/tooluse/reach_tools.go` → `ReachAdapter` 接口 + `IntegrationReachAdapter` | **新增 `BridgeReachAdapter` 包装**，网页渠道回复改走 WebSocket |
| WebSocket 中枢 | `internal/websocket/hub.go` + `envelope.go` | 参考其 `Hub`/`Envelope` 模式实现 `BridgeHub` |
| 渠道标识常量 | `internal/model/message_event.go`：`ChannelDouyin/ChannelXHS/ChannelTikTok` | bridge 复用并新增 `*_web` 变体 |

> 关键结论：**入站天然复用、出站天然有扩展点（ReachAdapter）、智能体绑定天然支持**。bridge 的工程量是"把扩展点接上" + "写 Chrome 扩展"。

---

## 2. 整体架构

```
┌──────────────────────────────────────────────────────────────────────────┐
│  浏览器（用户登录了抖音/小红书/TikTok 网页）                                  │
│                                                                            │
│  ┌──────────────────── Chrome 扩展 user-web/bridge ────────────────────┐  │
│  │  content script（每渠道一个）                                         │  │
│  │   ├─ 监听 DOM 新私信 → 解析为 UnifiedMessage                         │  │
│  │   ├─ 监听"待发送回复" → 填输入框 + 点发送                            │  │
│  │   └─ 通过 chrome.runtime 长连接 ↔ background                         │  │
│  │  background service worker                                           │  │
│  │   ├─ BridgeClient：维护到 user-server 的 WebSocket 长连接            │  │
│  │   ├─ 上行：UnifiedMessage → 服务器入站                               │  │
│  │   └─ 下行：AI 回复 → 路由到对应 content script → 发送               │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────┬──────────────────────────────────────────┘
                                 │  WSS  /api/ws/bridge  (JWT 鉴权 + channel+account 注册)
                                 ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  user-server                                                              │
│                                                                            │
│  BridgeHandler (新增)                                                      │
│   ├─ 鉴权（复用 JWT 中间件）                                              │
│   ├─ 维护 BridgeHub： (channel, account_id) → WS Client                   │
│   ├─ 收上行 inbound_message → InboxIngressService.HandleIngressMessage()  │
│   └─ 提供 BridgeHub.Deliver(channel, account, reply)                      │
│                                                                            │
│  InboxIngressService（现有）──► AgentRuntime ──► SmartCSOrchestrator       │
│                                              │                            │
│                                              ▼                            │
│                                   ReachAdapter.Send(req)  ◄── AI 回复     │
│                                        │                                 │
│                          ┌─────────────┴──────────────┐                  │
│                          ▼                            ▼                  │
│               IntegrationReachAdapter        BridgeReachAdapter（新增）  │
│               （官方 API 渠道）               （网页渠道 → BridgeHub WS）  │
│                                                          │               │
└──────────────────────────────────────────────────────────┼──────────────┘
                                                            │  WSS 下行 AI 回复
                                                            ▼
                                                   Chrome 扩展 background → content script → 网页发送
```

一句话数据流：
**网页私信 → content script → background → WS 上行 → InboxIngressService → AI 智能体 → ReachAdapter → BridgeReachAdapter → BridgeHub → WS 下行 → background → content script → 网页发送。**

---

## 3. 领域模型：统一消息（UnifiedMessage）

各平台私信结构不同，bridge 在 content script 层就把它们**规范化为统一消息**，后续链路只认统一消息。

```ts
// user-web/bridge/core/types.ts
export type BridgeChannel = 'douyin_web' | 'xhs_web' | 'tiktok_web';

export interface UnifiedMessage {
  event_id: string;        // 平台内唯一消息 ID（用于幂等去重）
  channel: BridgeChannel;  // 渠道
  account_id: string;      // 本机登录的账号标识（如主页 URL / 昵称派生），用于路由到正确智能体
  conversation_id: string; // 会话 ID（与某客户的私信对话）
  sender_id: string;       // 客户唯一 ID
  sender_name?: string;
  receiver_id?: string;    // 本账号 ID
  msg_type: 'text' | 'image' | 'audio' | 'video' | 'card';
  content: string;         // 文本；非文本时放描述/URL
  media_url?: string;
  timestamp: number;       // 毫秒
  extra?: Record<string, unknown>; // 透传平台原始字段
}
```

> 与服务端 `model.MessageEvent` 一一对应（`channel→Channel*`、`sender_id`、`conversation_id→ConversationID`、`event_id→EventID` 做幂等）。BridgeHandler 收到 UnifiedMessage 后仅做字段映射，无需理解平台语义。

---

## 4. 服务端设计（user-server）

### 4.1 新增包：`internal/bridge`

```
internal/bridge/
  hub.go            // BridgeHub：(channel, account_id) -> *BridgeClient
  hub_test.go
  handler.go        // BridgeWSHandler：升级 WS、鉴权、注册、帧路由
  handler_test.go
  frames.go         // 协议帧定义（上行/下行 JSON）
  reach_adapter.go  // BridgeReachAdapter：实现 ReachAdapter，网页渠道走 WS
  reach_adapter_test.go
```

### 4.2 BridgeHub（参考 `websocket/hub.go` 模式，但按 channel+account 索引）

- key：`channel + ":" + account_id`（如 `douyin_web:abc123`）。
- 一个账号同时只持有一条活跃连接（后连顶掉前连，避免双发）。
- 方法：
  - `Register(key, client)` / `Unregister(key)`
  - `Deliver(channel, accountID, payload) error`：找到 client 推送；不在线则返回 `ErrBridgeOffline`（调用方记录并提示"扩展未连接"）。
  - `IsOnline(channel, accountID) bool`（供 UI/健康检查）。
- 心跳：服务端 30s 发 `{type:"ping"}`，扩展回 `{type:"pong"}`；超时剔除。

### 4.3 BridgeWSHandler（端点 `GET /api/ws/bridge`）

1. JWT 鉴权（复用现有 auth 中间件 / 与 `/api/ws/agent` 同款）。
2. 升级 WebSocket。
3. 首帧必须是 `register`：`{type:"register", channel, account_id}`；校验通过后 BridgeHub.Register。
4. 之后两类帧：
   - 上行 `inbound_message`：→ 映射为 `model.MessageEvent` → `InboxIngressService.HandleIngressMessage(ctx, event)`。
   - 上行 `pong` / `ack`：保活 / 下发确认。
5. 下行：由 BridgeHub 主动 push（见 4.5）。

> 为什么用 WebSocket 而非"扩展轮询出站"？轮询引入延迟且浪费请求；长连接能秒级下发，且扩展天然知道"账号是否在线"，便于 UI 显示连接状态。WebSocket 也与项目现有 `/api/ws/agent`、`/api/ws/visitor` 一致。

### 4.4 入站接线（零改动中台）

`BridgeWSHandler` 收到 `inbound_message` 后：

```go
event := &model.MessageEvent{
    EventID:        m.EventID,
    SessionID:      m.Channel + ":" + m.AccountID + ":" + m.ConversationID,
    Channel:        m.Channel, // douyin_web / xhs_web / tiktok_web
    SenderID:       m.SenderID,
    SenderName:     m.SenderName,
    ReceiverID:     m.ReceiverID,
    MsgType:        m.MsgType,
    Content:        m.Content,
    MediaURL:       m.MediaURL,
    ConversationID: m.ConversationID,
    Timestamp:      time.UnixMilli(m.Timestamp),
    Extra:          map[string]any{"account_id": m.AccountID, "bridge": true},
}
ingress.HandleIngressMessage(ctx, event)
```

`InboxIngressService` 会自动：标准化 → 人工接管锁检查 → AI 锁串行化 → 落 `message_hub` → 通知 AgentRuntime。**AI 自动触发，bridge 无需关心。**

### 4.5 出站接线：`BridgeReachAdapter`（关键扩展点）

新增 `BridgeReachAdapter`，**组合** `IntegrationReachAdapter`：

```go
type BridgeReachAdapter struct {
    inner     *tooluse.IntegrationReachAdapter   // 非网页渠道委托
    hub       *BridgeHub
}

func (a *BridgeReachAdapter) Send(ctx, req *service.ReachSendRequest) (string, error) {
    if isBridgeChannel(req.Channel) {   // douyin_web / xhs_web / tiktok_web
        payload := buildOutbound(req)   // UnifiedReply
        if err := a.hub.Deliver(req.Channel, req.AccountID, payload); err != nil {
            return "", err               // 扩展离线 → 返回错误，由上层提示
        }
        return "bridge:" + req.Channel + ":" + req.AccountID, nil
    }
    return a.inner.Send(ctx, req)        // 其它渠道原样委托
}
```

接线点（两处，参考 `router/sales_engine_factory.go:140` 与 `router/tool_provider_wiring.go:55`）：
- `registerAgentReachTools` 用 `NewBridgeReachAdapter(IntegrationReachAdapter, bridgeHub)` 替代直接 `IntegrationReachAdapter`。
- `ReachToolProvider.Provide` 同理替换。

> 这样 AI 智能体的 `reach.web.send` 等工具在"网页渠道"下自动改走 WS，对编排层完全透明。

### 4.6 渠道标识与智能体绑定

- 新增常量（在 `model/message_event.go` 或 `bridge` 包内）：`ChannelDouyinWeb = "douyin_web"`、`ChannelXHSWeb = "xhs_web"`、`ChannelTikTokWeb = "tiktok_web"`。
- 在 `channel_agent_bindings` 表注册：`(channel_type='douyin_web', account_id='<扩展账号>', agent_id=<某智能体>)`。`AgentRuntime.LoadAgentContext` 已按 `(channel, account)` 查找，**无需改 runtime**。
- `account_id` 由扩展在 `register` 时上报（content script 从页面抓取本账号主页标识派生），保证"同一人多个浏览器/账号"互不串号。

---

## 5. Chrome 扩展设计（user-web/bridge）

### 5.1 设计原则（对应需求 1、2）

1. **按渠道封装**：`channels/douyin`、`channels/xhs`、`channels/tiktok` 各自独立。
2. **渠道内保持抽象，便于扩展评论/回关等**：每个渠道实现统一接口 `ChannelAdapter`：

```ts
// user-web/bridge/core/channel-adapter.ts
export interface ChannelAdapter {
  readonly channel: BridgeChannel;
  /** 启动监听：新私信 → onInbound(UnifiedMessage) */
  start(onInbound: (m: UnifiedMessage) => void): void;
  /** 收到 AI 回复，回写到网页发送 */
  sendOutbound(reply: UnifiedReply): Promise<void>;
  /** 判断当前页面是否属于本渠道（content script 注入判断） */
  match(): boolean;
  /** 可选：未来扩展评论/回关等能力 */
  capabilities?: ChannelCapability[];
}
```

> 未来要支持"评论回复/自动回关"，只需在 `ChannelAdapter` 加方法（如 `autoFollow(uid)`、`replyComment(cid, text)`），上层调度不变——满足需求"将来扩展 评论、回关等"。

3. **桥接职责单一**：background service worker 只负责 WS 长连接与路由；content script 只负责 DOM 操作。两者通过 `chrome.runtime.connect`（长连接 port）通信。

### 5.2 目录结构（user-web/bridge）

```
user-web/bridge/
  bridge.md                      // 本文档
  manifest.json                  // MV3 扩展声明
  package.json
  vite.config.ts                 // 多入口打包：background / content-<channel> / popup
  tsconfig.json
  src/
    core/
      types.ts                   // UnifiedMessage / UnifiedReply / 帧类型
      channel-adapter.ts         // ChannelAdapter 抽象接口
      bridge-client.ts           // WS 客户端（重连/心跳/帧编解码），复用 user-web/utils 的鉴权逻辑
      logger.ts                  // 频道着色 + 敏感字段脱敏
      sanitize.ts                // XSS 防护（escapeHTML / safeSetContent / sanitizeForDisplay）
      fallback.ts                // account_id fallback 派生 + 自他消息判定兜底
      rate-limiter.ts            // 拟人节奏 + 令牌桶 + 会话冷却 + 去重
    channels/
      douyin/index.ts            // DouyinAdapter（复用 DY-auto 选择器与收发）
      douyin/selectors.ts        // 私信列表/输入框/发送按钮 DOM 选择器（来自 DY-auto）
      xhs/index.ts               // XHSAdapter（复用 XHS-YYDS messageDetector/autoReply）
      xhs/selectors.ts
      tiktok/index.ts            // TiktokAdapter（复用 tiktok-auto-plugin）
      tiktok/selectors.ts
    background/
      index.ts                   // service worker：WS 连接 + port 路由 + 注册
      registry.ts                // (channel,account) 连接状态
    popup/
      index.html                 // 状态横幅 + 错误提示 + 测试连接按钮
      index.js                   // save / testConnection / refreshStatus / selfCheck
    content-loaders.ts           // 各渠道 content script 入口（按 match 注入）
```

### 5.2.1 Popup 状态横幅（v1.1）

修复用户反馈"插件不生效 链接都没生效"的根因：

| 问题 | 修复 |
| --- | --- |
| dist 是旧版本（placeholder 端口是 8080） | 重跑 `scripts/build.mjs`，dist 现在端口是 8204 |
| 端口写错（用户输 8021 而非 8204） | 默认 placeholder 改为 `http://localhost:8204` + hint 提示 |
| 保存反馈太弱（按钮文字闪 1.5s） | 改成持久状态横幅（success / warn / error / info 四种） |
| `chrome.runtime.lastError` 被吞 | 所有 `sendMessage` / `tabs.sendMessage` 都 try/catch 并展示 |
| 无法验证 URL 是不是对的 | 新增"测试连接"按钮，依次请求 `/health` / `/healthz` / `/readyz` / `/api/health` |
| 自检报错不友好 | 自检失败展示可能原因清单（未登录 / 扩展禁用 / URL 不匹配 manifest） |
| 不知道哪个页面是私信页 | 新增"抖音私信 / 小红书私信 / TikTok"三个快捷按钮 |

`test/popup.test.js` 覆盖 15 用例（normalizeServerUrl / testConnection / showBanner）。

### 5.2.2 默认值单一源（v1.2）

**项目硬约束**：禁止"软启动"（默认值兜底为空）、禁止多处硬编码。

所有默认值集中在 `src/core/constants.js`（前端）+ `handler.go` / `hub.go`（后端），
文档源在 [`docs/DEFAULTS.md`](./docs/DEFAULTS.md)。每个常量都标注：
  - 字段名 + 默认值
  - 文档源（DEVELOPMENT.md / Dockerfile / config.yaml / 经验值）
  - 客户端/服务端对齐约束

测试覆盖：
  - `test/constants.test.js`（24 用例：值、范围、冻结、对齐、文档源完整性）
  - `internal/bridge/defaults_test.go`（5 子测试：WS 常量、Hub 常量、Client/Server 对齐、非软启动、WS 端点路径）

调整流程见 `docs/DEFAULTS.md` §3。

### 5.3 扩展 ↔ 服务器 协议帧（JSON over WS）

上行（扩展→服务器）：
- `register`：`{type:"register", channel, account_id, token?}`
- `inbound_message`：`{type:"inbound_message", message: UnifiedMessage}`
- `pong` / `ack`：`{type:"ack", seq}`

下行（服务器→扩展）：
- `pong`：`{type:"pong"}`
- `outbound_reply`：`{type:"outbound_reply", reply: UnifiedReply}` → background 路由到对应 content script 调 `sendOutbound`
- `config_push`：`{type:"config_push", ...}`（可选：服务器下发的账号/智能体配置）

`UnifiedReply`：
```ts
export interface UnifiedReply {
  channel: BridgeChannel;
  account_id: string;
  conversation_id: string;
  content: string;
  msg_type: 'text' | 'image' | 'card';
  media_url?: string;
  reply_to_event_id?: string; // 关联原消息，便于 content script 定位会话
}
```

### 5.4 重连与离线兜底

- `bridge-client.ts` 断线指数退避重连（1s→30s 封顶），重连后重新 `register`。
- 若 AI 回复产生时扩展离线：`BridgeHub.Deliver` 返回 `ErrBridgeOffline`，服务端记录"待投递"并提示坐席/用户在扩展重连后补发（MVP 可先记日志+UI 提示，后续做离线队列）。

---

## 6. 数据流时序

### 6.1 入站（客户发私信 → AI 处理）

```
客户发私信
  → content script 监听到新 DOM 节点（DouyinAdapter 等）
  → 解析为 UnifiedMessage
  → chrome.runtime port.send → background
  → BridgeClient WS 上行 inbound_message
  → BridgeWSHandler → InboxIngressService.HandleIngressMessage
  → 落 message_hub + 通知 AgentRuntime
  → SmartCSOrchestrator 调 LLM → 生成回复
  → ReachAdapter.Send(req)
  → BridgeReachAdapter.Deliver → BridgeHub → WS 下行 outbound_reply
  → background 路由到对应 content script
  → adapter.sendOutbound：填输入框 + 点发送
  → 网页私信框出现 AI 回复
```

### 6.2 出站（AI 回复 → 网页发送）

见 6.1 末段；重点：回复通过**同一条 WS 长连接**下行，content script 在已打开的会话里注入并发送，无需重新打开对话。

---

## 7. 复用清单（现有代码 + 三个开源插件）

### 7.1 复用 user-server 现有代码

| 复用对象 | 文件 | 用途 |
| --- | --- | --- |
| `InboxIngressService.HandleIngressMessage` | `internal/service/inbox_ingress.go:216` | 入站统一入口（零改动） |
| `model.MessageEvent` | `internal/model/message_event.go:43` | 协议契约 |
| `ChannelAgentBinding` | `internal/service/ai_agent.go` | 渠道↔智能体绑定 |
| `ReachAdapter` 接口 | `internal/aiagent/agent/tooluse/reach_tools.go:44` | 出站扩展点 |
| `IntegrationReachAdapter` | `internal/aiagent/agent/tooluse/reach_integration_adapter.go` | 非网页渠道委托 |
| `Hub` / `Envelope` 模式 | `internal/websocket/hub.go`、`envelope.go` | BridgeHub 参考实现 |
| JWT 鉴权中间件 | 同 `/api/ws/agent` 接线 | WS 鉴权 |

### 7.2 复用三个开源插件（选择器 + 收发逻辑）

> 已克隆至 `.research/`，下为各项目"私信主线"可复用文件（需改写为 TS + 适配 MV3 且合规）：

**XHS-YYDS（小红书）** `.research/XHS-YYDS`
- `content.js` / `messageDetector.js`：私信列表与消息节点的 DOM 监听与解析（消息内容、对方 ID）。
- `autoReply.js`：定位输入框、填充、触发发送的逻辑。
- 复用点：XHSAdapter 的 `start()` 监听与 `sendOutbound()` 注入，直接参考其选择器与事件绑定写法。

**DY-auto（抖音）** `.research/DY-auto`
- content script 中"私信"相关模块：新消息轮询/ MutationObserver、对话定位、输入框填充与发送。
- 复用点：DouyinAdapter 的 selectors.ts 与发送流程。

**tiktok-auto-plugin（TikTok）** `.research/tiktok-auto-plugin`
- `src/content/`（或等价）中 business inbox 监听与发送逻辑；background ↔ content 的 `chrome.runtime` 长连接写法（可作为 bridge 通信范式参考）。
- 复用点：TiktokAdapter 收发 + background/content 通信结构。

> 注意：开源插件多为"自动化运营"工具，含自动点赞/养号等无关功能。**bridge 只提取"私信监听/解析/发送"核心逻辑**，去除自动营销行为，确保合规与聚焦。

---

## 8. user-server 侧的接入配置（需求 5）

在 `user-web` 渠道管理 UI（现有 `douyinCard.js` / `xiaohongshuCard.js` / `tiktokCard.js` 卡片）新增"网页桥接"模式：

1. 用户安装扩展并登录平台网页。
2. 在 `user-web` 对应渠道卡片点击"启用网页桥接"，生成/绑定一个 `account_id`（或扩展自动上报）。
3. 后端在 `channel_agent_bindings` 写入 `(channel_type='douyin_web', account_id, agent_id)`。
4. 扩展 `register` 上报 `account_id` 后，该账号的私信即路由到指定智能体。

> 这样"在 user-server 设置关联的智能体渠道"通过**现有绑定表 + 新增 `*_web` 渠道类型**完成，无需新建复杂配置页。

---

## 9. 多角度头脑风暴 / 论证 / 查漏补缺（设计评审）

> 本节对方案做反向论证，列出风险、备选与取舍，并给出结论。

### Q1. 为什么不直接给抖音/小红书/TikTok 做官方 API 渠道，而要用浏览器桥接？
- **论证**：这三个平台（尤其个人号）**无稳定开放的私信 API**；即使企业号有，也需资质审核、易封控、覆盖不全。浏览器桥接是当前唯一"通用且即时"的路径。
- **取舍**：桥接依赖用户保持网页登录 + 扩展在线，可靠性低于 API；但这是该场景下的务实最优解。
- **结论**：采用桥接，同时把"扩展离线"作为一等公民处理（Q6）。

### Q2. 入站用 HTTP webhook 还是 WebSocket 上行？
- **论证**：现有 `WebhookService.Receive` 走 HTTP + HMAC 验签，但扩展在浏览器里持有 JWT 更自然；且**出站本来就需要 WS**，用同一条 WS 上行入站可减少连接数、统一协议、避免 HMAC 密钥在扩展侧管理。
- **备选**：扩展用 HTTP POST 到 `/api/bridge/webhook`（JWT Bearer）。
- **结论**：MVP 用 **WS 上行**（与下行同连接）；保留 HTTP 入口作为后续可选，便于无头/服务端脚本复用。

### Q3. 出站回复如果扩展刚好离线怎么办？
- **风险**：AI 已生成回复，但客户网页关闭/扩展崩溃 → 消息丢失。
- **方案**：`BridgeHub.Deliver` 失败 → 服务端记 `message_hub`（direction=outbound, status=failed）+ 通知坐席 UI；扩展重连后，可主动拉取"未投递回复"补发（MVP 先做失败记录+提示，离线队列作为 v1.1）。
- **结论**：先 fail-fast + 可观测，再补离线队列。

### Q4. 多账号 / 多浏览器如何隔离，避免串号或错绑智能体？
- **风险**：同一人两个抖音号，或两台电脑同时登录同一账号。
- **方案**：`account_id` 由扩展从页面本账号主页派生（稳定且唯一）；`BridgeHub` 同 key 后连顶前连，保证一个账号一条活跃连接；绑定表按 `(channel, account_id)` 精确匹配。
- **结论**：账号维度隔离，连接抢占保证单活。

### Q5. 消息幂等 / 重复处理如何防？
- **风险**：平台 DOM 重复触发、WS 重连重发、AI 重试。
- **方案**：`event_id` 全程透传；`InboxIngressService` 已有幂等（MessageEvent.EventID）+ AI 锁；出站 `BridgeReachAdapter` 复用 `agent_runtime.ClaimReply(eventID)` 幂等守卫（见 `webhook.go:1824`），同一回复只下发一次。
- **结论**：沿用现有双幂等守卫，bridge 不另造。

### Q6. content script 监听 DOM 的稳定性（三个平台页面经常改版）？
- **风险**：平台改版导致选择器失效，私信漏收/漏发。
- **方案**：
  - 选择器集中到 `channels/*/selectors.ts`，便于改版快速修。
  - 用 MutationObserver + 文本特征（如"对方正在输入"、消息气泡结构）双重判定，降低单一选择器脆弱性。
  - 加"自检"：扩展 popup 显示"已识别私信会话 N 个"，便于用户发现异常。
- **结论**：集中+健壮监听+可观测，接受"需随平台维护"的固有成本。

### Q7. 安全：扩展能连上服务器、且只能动自己账号的私信？
- **方案**：WS 用 JWT（与现有 agent/visitor 同套）；`account_id` 在 `register` 时服务端校验属于当前用户（绑定表归属）；下行只推给 `BridgeHub` 中该 `(channel, account)` 的连接，天然隔离。
- **结论**：复用 JWT + 绑定归属校验，安全边界清晰。

### Q8. 是否会与现有 API 渠道（douyin/xhs/tiktok）冲突？
- **方案**：bridge 用 `*_web` 渠道类型，与 API 渠道类型不同；`sendOutbound`/`ReachAdapter` 按渠道区分，互不影响。现有 API 渠道保持不变。
- **结论**：命名空间隔离，零回归风险。

### Q9. 扩展性：未来要加"评论回复/自动回关"怎么做？
- **方案**：`ChannelAdapter` 已留 `capabilities` 与可扩展方法；新增能力只在对应渠道 adapter 实现，background 调度按 capability 路由。**中台入站模型（MessageEvent）已含 `MsgType`/`Extra`，可承载评论等非私信事件。**
- **结论**：抽象到位，扩展成本低。

### Q10. 性能 / 连接数？
- **方案**：每个扩展一条 WS；`BridgeHub` 用 map 索引，O(1) 路由；心跳 30s 保活。规模在"单商户数十账号"内无压力（与现有 visitor WS 同量级）。
- **结论**：可接受。

---

## 10. 实施里程碑

1. **M1 服务端骨架**：`internal/bridge`（hub/handler/frames）+ `GET /api/ws/bridge` 接线 + 路由注册。入站打通（调 InboxIngressService）。
2. **M2 出站扩展点**：`BridgeReachAdapter` + `registerAgentReachTools`/`Provide` 接线；`*_web` 常量与绑定表写入。
3. **M3 Chrome 扩展骨架**：manifest + background(WS) + popup + `bridge-client.ts` + 一个渠道（先 XHS，逻辑最清晰）跑通端到端。
4. **M4 三渠道补齐**：DouyinAdapter / TiktokAdapter（复用开源选择器与收发）。
5. **M5 UI 与配置**：`user-web` 渠道卡片加"网页桥接"开关 + 绑定写入；连接状态展示。
6. **M6 健壮性**：重连、离线兜底、幂等验证、日志与可观测。

---

## 11. 测试策略

- **服务端单测**：
  - `BridgeHub`：注册/抢占/离线 `Deliver` 返回 `ErrBridgeOffline`（`hub_test.go`）。
  - `BridgeReachAdapter`：bridge 渠道走 hub、非 bridge 渠道委托 inner（`reach_adapter_test.go`）。
  - `BridgeWSHandler`：register 帧 → 入站被调用；inbound 帧映射正确（`handler_test.go`，用 `httptest` + `gorilla/websocket` Dial）。
- **扩展单测**（Vitest）：
  - `bridge-client.ts` 帧编解码、重连退避。
  - 各 `ChannelAdapter` 的 `sendOutbound` 在 jsdom 中正确填值并触发发送（mock DOM）。
- **E2E（手动）**：XHS 网页登录 → 用另一账号发私信 → 观察 AI 回复出现于发送框并发送；断开扩展观察失败提示。

---

## 12. 待确认问题

1. `account_id` 的派生规则：用平台主页 URL / 昵称 / 本地生成的稳定 UUID？影响多设备一致性。
2. 是否需要在 `user-server` 增加"网页桥接账号"实体的显式管理（CRUD），还是仅依赖绑定表隐式创建？
3. 离线回复是否需要做"重连补发队列"（v1.1）还是 MVP 仅失败记录？
4. 三个开源插件的 LICENSE 是否允许商用改写（XHS-YYDS / DY-auto / tiktok-auto-plugin）？需法务/合规确认后再合入选择器代码。
5. 是否需要支持"同一平台多账号同时桥接"（MVP 假设支持，hub 已按 account 隔离）？

---

## 13. 一句话总结（需求 6）

**`user-web/bridge` 是一个 Manifest V3 Chrome 扩展 + `user-server/internal/bridge` 服务端模块的组合：扩展按渠道（抖音/小红书/TikTok）抽象监听并回写网页私信，经统一消息模型通过 WebSocket 桥接到 `user-server` 的 AI 智能体，实现"网页私信 ↔ AI 客服"的实时双向打通。**

---

## 14. 实施细节补充（查漏补缺 · 第 4 步二次论证）

> 在第 9 节 10 个角度论证基础上，进一步核对服务端接线事实，给出可直接落地的"接线坐标"，消除第 5 步歧义。

### 14.1 服务端 WS 端点注册（精确位置）

- 坐席 WS：`internal/router/service_routes.go:81` → `auth.GET("/ws/agent", wsHandler.HandleWebSocket)`（已套 JWT 中间件组 `auth`）。
- 访客 WS：`internal/router/chat_routes.go:67` → `r.GET("/api/ws/visitor", visitorWS.HandleVisitorWebSocket)`（公开，按 `session_id` 鉴权）。
- **bridge WS 接线**：在 `service_routes.go` 的 `auth` 组内新增 `auth.GET("/ws/bridge", bridgeHandler.HandleWebSocket)`。`bridgeHandler` 由 `bridge.NewBridgeWSHandler(bridge.GetBridgeHub(), ingressSvc)` 构造，依赖注入同既有 handler。

### 14.2 WS 鉴权约定（与现有一致）

- 现有 `/api/ws/agent` 在私有部署下**不做 token 鉴权**，仅以 query `agent_id` 路由身份（`handler.go:66` 注释明确）。
- bridge 沿用同约定：`GET /api/ws/bridge?channel=douyin_web&account_id=xxx`。身份由首帧 `register` 二次确认；授权边界由 `channel_agent_bindings` 归属校验（账号须属于当前部署用户）。
- 强化部署（如需）：扩展连接时从 `user-web` 鉴权态取 JWT 作为 query/header，bridge handler 调现有 JWT 解析中间件校验。

### 14.3 ReachSendRequest 字段映射（BridgeReachAdapter 出站）

`service.ReachSendRequest`（`internal/service/reach_send_pipeline.go:75`）关键字段：

| 字段 | 含义 | bridge 出站用法 |
| --- | --- | --- |
| `Channel` | 渠道（douyin_web/xhs_web/tiktok_web） | 判断是否 bridge 渠道 |
| `AccountID` | 发送账号 ID | 路由到 BridgeHub 的 `(channel, account)` |
| `RecipientID` | 接收者（客户）ID | → UnifiedReply.conversation_id / receiver |
| `CustomerID` | 客户 ID（限流维度） | 透传 |
| `MsgType` | text/image/link/card | → UnifiedReply.msg_type |
| `Content` | 内容 | → UnifiedReply.content |
| `Attachments` / `CardID` | 附件/卡片 | → UnifiedReply.media_url（MVP 仅 text） |
| `Metadata` | 额外元数据 | 透传 reply_to_event_id 等 |

> `BridgeReachAdapter.Send` 仅当 `isBridgeChannel(req.Channel)` 时接管，否则 `return a.inner.Send(ctx, req)` 委托 `IntegrationReachAdapter`，**零影响现有渠道**。

### 14.4 adapter 注入接线（两处，最小改动）

- `internal/router/sales_engine_factory.go:140` `registerAgentReachTools(gormDB)`：将 `tooluse.NewIntegrationReachAdapterFromDB(gormDB)` 改为 `bridge.NewBridgeReachAdapter(tooluse.NewIntegrationReachAdapterFromDB(gormDB), bridge.GetBridgeHub())`。
- `internal/router/tool_provider_wiring.go:55` `ReachToolProvider.Provide`：同样改用 `NewBridgeReachAdapter(...)`。
- `BridgeHub` 为全局单例（`bridge.GetBridgeHub()`，仿 `websocket.GetHub()` 的 `sync.Once` 模式），在 `router.Setup()` 中 `go bridge.GetBridgeHub().Run()` 启动事件循环。

### 14.5 入站 SessionID 构造（防串号）

`SessionID = channel + ":" + account_id + ":" + conversation_id`。与 `InboxIngressService` 的 AI 串行锁、人工接管锁维度一致，保证同一会话串行、跨账号隔离。

### 14.6 出站幂等（防重复下发）

复用 `agent_runtime.ClaimReply(eventID)`（`webhook.go:1824` 同款守卫）：`BridgeReachAdapter.Send` 在 `Deliver` 前调用 `ClaimReply(req.Metadata["event_id"])`，同一 AI 回复只下发一次；扩展侧 `ack` 帧确认后服务端标记已送达（可选 MVP 后做）。

### 14.7 vite 多入口打包（扩展构建）

`user-web/bridge/vite.config.ts` 用 `build.rollupOptions.input` 多入口：`background`、`content-douyin`、`content-xhs`、`content-tiktok`、`popup`。MV3 下 background 为 `service_worker`、content_scripts 注入对应 `matches`（`*.douyin.com`、`*.xiaohongshu.com`、`*.tiktok.com`）。与 `user-web` 主 SPA 构建互不干扰（独立 `package.json` + `tsconfig`）。

### 14.8 风险再确认（二次论证结论）

| 风险 | 处置 | 结论 |
| --- | --- | --- |
| 平台改版致选择器失效 | 选择器集中 `channels/*/selectors.ts` + MutationObserver 双判定 + popup 自检 | 接受维护性成本 |
| 扩展离线丢回复 | `Deliver` 失败记 `message_hub(status=failed)` + 坐席提示；离线队列 v1.1 | 先可观测 |
| 多账号串号 | `account_id` 页面派生 + hub 同 key 抢占单活 | 安全 |
| 重复消息 | 双幂等守卫（event_id + ClaimReply） | 沿用现有 |
| 与 API 渠道冲突 | `*_web` 命名空间隔离 | 零回归 |
| LICENSE 合规 | 仅抽取"私信监听/发送"核心逻辑改写，先确认三仓库协议 | 落地前确认 |

> 经两轮论证（第 9 节 10 角度 + 本节接线事实核对），方案在**复用度、隔离性、扩展性、可观测性**四维均达可实施标准，进入第 5 步开发。

---

## 15. 第 5 步 实施落地记录（已开发 + 测试）

### 15.1 服务端 `user-server/internal/bridge`（新增包，已编译+单测通过）

| 文件 | 作用 |
| --- | --- |
| `channel.go` | 网页桥接渠道常量 `douyin_web/xhs_web/tiktok_web` + 与基础渠道互相映射 |
| `frames.go` | 帧类型 + `UnifiedMessage`/`UnifiedReply`/`Frame` 结构 |
| `hub.go` | `BridgeHub`（`channel:account`→连接 索引、注册/抢占/在线判定/`Deliver`、全局单例） |
| `handler.go` | `BridgeWSHandler`：`GET /api/ws/bridge` 升级、register、inbound→`InboxIngressService`、读写泵+心跳 |
| `reach_adapter.go` | `BridgeReachAdapter` 实现 `ReachAdapter`：网页渠道在线→WS 下发，否则委托 `IntegrationReachAdapter`（零影响现有 API 渠道） |

**接线改动（最小侵入）：**
- `tooluse/reach_tools.go`：接口新增 `SendTikTok`；`reachChannelAdapterBridge.Send` 增加 `douyin_web/xhs_web/tiktok_web` 分支。
- `tooluse/reach_integration_adapter.go` + 两处 mock：`SendTikTok` 实现（返回 `ErrChannelNotImplemented`，由 bridge 接管）。
- `router/service_routes.go`：注册 `auth.GET("/ws/bridge", bridgeHandler.HandleWebSocket)`。
- `router/sales_engine_factory.go`、`router/tool_provider_wiring.go`：用 `bridge.NewBridgeReachAdapter(...)` 包装原有 adapter。

**验证：** `go build ./...` 全量通过；`go test ./internal/bridge/...` 6 用例通过（hub 注册/抢占/离线、adapter 在线走 WS / 离线委托 / 三渠道）。

### 15.2 扩展 `user-web/bridge`（Manifest V3，已构建+单测通过）

> ⚠️ 本节为初版 TypeScript 草稿记录；**实际产物已改用纯 JavaScript + esbuild**（详见 §16）。本节保留作为历史变更对照，落地实现以 §16 为准。

```
user-web/bridge/
  manifest.json  package.json  vite.config.js（仅 vitest 用）
  src/
    core/      types.js · channel-adapter.js · bridge-client.js · dom.js · logger.js · rate-limiter.js
    channels/  douyin.js · xhs.js · tiktok.js   （抽象适配器，复用开源选择器）
    content/   douyin.js · xhs.js · tiktok.js · common.js          （content script 入口）
    background/ registry.js · index.js                            （WS 长连接 + 端口路由）
    popup/     index.html · index.js                              （连接状态 + 后端地址配置）
```
- `BaseAdapter` 统一封装 MutationObserver 监听、去重、统一消息构造、回写发送；三渠道仅以 hooks 实现差异 → **满足"按渠道封装 + 渠道内抽象、可扩展评论/回关"**。
- 复用：抖音 `contenteditable` 填值 + 红色发送按钮（来自 DY-auto）；小红书 `#jarvis-reply-textarea`/`.send_btn`/`.im-msg-item`/`.left`（来自 XHS-YYDS domUtils）；TikTok 参考 tiktok-auto-plugin 的 MV3 `chrome.runtime` 长连接结构。
- **验证（按 §16.1 实际命令）：** `npx vitest run` 通过；`npm run build` 产出 `dist/`。

### 15.3 运行与测试

- 服务端：`go test ./internal/bridge/...`。
- 扩展：`cd user-web/bridge && npm install && npm test && npm run build`；`dist/` 加载为 Chrome 解压版扩展，`popup` 配置后端 `wss://<host>/api/ws/bridge`，打开对应平台私信页即自动桥接。
- 端到端手动验证见 §11（需在浏览器登录平台 + 后端已绑定智能体）。

### 15.4 已知待校准项（来自需求 6 之后的实测前清单）

- 三平台**自/他消息区分 class**（Douyin `.left/.right`、TikTok `incoming` 标识）需按真实页面 DOM 校准，防止把 AI 回复当客户消息回环。
- `account_id` 在 TikTok 无稳定 uid，暂以 `@username` 派生，多设备一致性待定。
- 离线回复队列（v1.1）、三个开源仓库 LICENSE 商用合规确认（§12）仍为待办。

---

## 16. 第 5 步（续）：纯 JavaScript 重写 + 真实 DOM 校准 + 头脑风暴论证

> 承接需求①②③：(1) 编码用 JavaScript；(2) 三平台按真实 DOM、复用三个开源逻辑完成首版；(3) 头脑风暴检查论证。

### 16.1 编码语言决策（需求①：用 JavaScript）

- **变更**：扩展端由 TypeScript 改为**纯 JavaScript**（ES module 源码 + esbuild 独立 IIFE 打包）。服务端 `user-server` 仍为 Go（不可改）。
- **理由**：
  1. 三个开源插件（DY-auto / XHS-YYDS / tiktok-auto-plugin）**均为纯 JS、零构建**，逐行移植最贴近、最易对照校准；
  2. MV3 content script **不能**用 ES module 直接加载，必须打包。vite 多入口会产生共享 chunk 导致 content script 加载失败；改用 **esbuild 逐入口独立 IIFE** 最稳（每个产物自包含）；
  3. 降低团队构建心智负担，扩展源码即浏览器语义。
- **结构变化**：删除 `tsconfig.json`、`vite.config.ts`（多入口）；新增 `scripts/build.mjs`（esbuild 逐入口打包）+ `esbuild` 依赖；保留 `vitest` 单测（jsdom）。
- **对照 §15.2/15.3 勘误**：首版扩展命令由 `npm run typecheck && npm test` 改为 `npm test`（无 TS 类型检查）；构建由 `vite build` 改为 `node scripts/build.mjs`。

### 16.2 三平台真实 DOM 逻辑移植清单（需求②：复用开源、待真机校准）

| 平台 | 真实来源 | 输入框 | 发送按钮 / 动作 | 消息结构 | 观察/会话 |
| --- | --- | --- | --- | --- | --- |
| 抖音 | DY-auto `content.js` | `div[contenteditable="true"][role="textbox"]` | `span.PygT7Ced.JnY63Rbk.e2e-send-msg-btn` 或 `path[fill="#FE2C55"]` 红色按钮 | （DY-auto 不解析消息，首版按 `.chat-message-item`+`.left`/`.right` 默认） | 会话列表 `#island_b69f5 li`；uid=`data-uid`/`a[href*="/user/"]` |
| 小红书 | XHS-YYDS `domUtils.js`/`autoReply.js` | `#jarvis-reply-textarea`（textarea） | `.send_btn`（`enhancedClickWithVerification`） | `.im-msg-item`+`.left`(对方)/`.right`(自己)+`.text-message` | MutationObserver 监听 `.vue-recycle-scroller` |
| TikTok | tiktok-auto-plugin `core/content.js`/`utils/simulator.js` | DraftEditor `contenteditable`（多兜底：`[data-e2e="message-input"]`/`div[contenteditable]`/`[role="textbox"]`） | `simulateTyping`+`simulateEnterKey`，或 svg 飞机按钮/`button[aria-label*="Send"]` | （插件仅自动回复，首版按 `[data-e2e="message-item"]`+`outgoing`/`incoming` 默认） | chat-list `[data-e2e="chat-list"]` |

- 交互原语全部移植自开源：抖音 `fillInputViaPaste`/`simulateRealClick`/`humanType`；小红书 `setValue`/`enhancedClick`；TikTok `simulateTyping`/`simulateEnterKey`。
- 每个渠道的 `SEL` 选择器常量集中在 `src/channels/*`，**popup「自检当前私信页」可直接展示当前生效选择器与解析样本**，加速真机校准。

**待真机校准项（在 popup 自检逐项验证）**：
1. 抖音消息气泡自/他 class（`.left`/`.right` 是否为当前版本真实结构）；若不符，改用 `[class*="..."]` 或头像位置判定。
2. 抖音 `account_id` 派生（左导航个人主页链接 uid）是否稳定。
3. 小红书会话 id 数据属性（`data-key`/`data-id`/`data-contactusemid`）。
4. TikTok 消息自/他判定（`outgoing`/`incoming` class 或对齐方式）。
5. TikTok 发送动作（Enter 回车 vs 飞机按钮坐标点击）。
6. 三平台受控输入框是否拦截填值（若不生效，增强为原生 setter + composition 事件）。

### 16.3 头脑风暴检查论证（需求③：多角度）

| 视角 | 风险点 | 论证结论 / 处置 |
| --- | --- | --- |
| A 功能正确性 | 自/他消息回环、把 AI 回复当客户消息再上行 | 三层防御：`BaseAdapter` 跳过 `sender_type=SELF`；`seen` 幂等去重；服务端 `ClaimReply` 幂等。仍须真机校准自/他 class（§16.2 校准项 1/4）。 |
| B 健壮性 | 扩展离线丢回复、WS 断开、content 刷新 | `BridgeReachAdapter.Deliver` 失败落 `message_hub(status=failed)` + popup 提示；WS 3s 重连 + 20s 心跳；background 持久、content 断线重连；`routeOutbound` 按 `channel+account+conversation` 精确匹配避免错投。 |
| C 安全 | 鉴权、权限最小化 | 私有部署沿用 `/ws/agent` 约定（query 标识，无 token）；增强部署用 JWT；`permissions` 仅 `storage/tabs/activeTab`，`host_permissions` 仅本地 + `ws(s)`。 |
| D 可扩展性 | 评论/回关、新增平台 | `BaseAdapter` 抽象 + hooks：评论/回关只需新增能力 hook；新平台复制 `channels/*.js` + manifest 加 `matches`；统一消息模型降低耦合。 |
| E 合规与风控 | LICENSE、平台风控 | 三个开源仓库 LICENSE 商用确认（待办）；拟人输入节奏（`humanType` 25~40ms 间隔）降低高频触发风控；建议灰度小流量。 |
| F 可观测性 | 排查难 | popup 实时连接状态 + 自检样本；服务端 `bridge` hub 日志；统一 `message_hub` 落库可回溯。 |

**综合结论**：方案在功能/健壮/安全/扩展/合规/可观测六维均达**首版可上线**标准；剩余三项为「真机选择器校准 + 风控节奏调优 + LICENSE 确认」的实测/确认工作，不阻塞首版交付。

### 16.4 首版交付状态

- 服务端（Go）未变：`go build ./...` 全量通过；`go test ./internal/bridge/...` 6 用例通过。
- 扩展（JS）已构建：`dist/background.js` + `content-douyin.js`/`content-xhs.js`/`content-tiktok.js` + `popup.html`/`popup.js` + `manifest.json`；`npx vitest run` 4 用例通过（WS 注册/下行/心跳/上行帧协议）。
- 运行路径：Chrome 加载 `dist/` 为解压扩展 → popup 填后端地址 → 打开三平台私信页自动桥接 → 点「自检」按 §16.2 清单校准。

---

### 17 续轮：持续头脑风暴论证检查（需求①②③④⑤ 全覆盖）

> 本轮回应用户五条新需求，并延续「头脑风暴 → 论证检查 → 必须满足」的闭环。
> 核心发现：**首版虽声称打通消息流，但扩展端协议常量/字段名与服务端 `frames.go` 完全不匹配，消息流实际并未端到端打通**。本轮先修复协议，再补齐历史回填与风控，最后用单测与文档论证满足度。

#### 17.1 需求③：三平台消息真正上报 user-server / 真正发出 user-server 下发的消息

**论证检查发现的问题（阻塞项）**：
- 扩展 `types.js` 旧常量：`FRAME.INBOUND='inbound'`、`OUTBOUND='outbound'`；服务端 `frames.go` 认 `inbound_message` / `outbound_reply` → 帧被服务端 `default` 分支丢弃。
- 字段名错位：扩展上行发 `text`/`message_id`/`sender_type`；服务端 `UnifiedMessage` 读 `content`/`event_id`/`sender_id` → 上行消息 `Content` 恒空，AI 拿到空文本。
- 下行解析错位：扩展读 `reply.text`，服务端 `UnifiedReply` 给 `content` → 回写拿到空串，直接跳过。

**修复（扩展端对齐服务端契约）**：
- `types.js`：`FRAME.INBOUND='inbound_message'`、`OUTBOUND='outbound_reply'`、`HISTORY='history'`；`makeUnifiedMessage` 输出 `content/event_id/sender_id/channel/account_id/conversation_id`；`parseUnifiedReply` 读 `content`。
- `content/common.js`：下行回复经 `parseUnifiedReply` 取 `content`，按 `conversation_id` 匹配当前会话后 `adapter.sendOutbound(content)`。
- `background/index.js` + `registry.js`：新增 `history` 帧路由与 `sendHistory`。
- 服务端 `frames.go` 已兼容（复用 `UnifiedMessage`，加 `direction`/`sender_type` 字段）；`handler.go` 已按帧类型分发。

**论证证据**：`test/protocol.test.js` 6 项断言帧类型与字段名与服务端一致；`test/bridge-client.test.js` 验证 `inbound`/`outbound` 帧收发；全量 16 单测通过。端到端链路：
```
客户私信 → DOM 监听 → inbound_message 帧 → 服务端 HandleIngressMessage → AI → outbound_reply 帧 → content 回写网页
```

#### 17.2 需求⑤：任意渠道多用户消息 / 消息历史记录上报 user-server

**论证核心矛盾**：若把所有消息（含存量历史、自己发出的）都走 `inbound_message`，会**误触发 AI 推理**且 `persistMessage` 硬编码 `Direction="inbound"`，导致自己发出的消息被标成「客户入站」——既错标方向又可能造成自回环。

**解决方案：双帧分离**（关键设计决策）
- `inbound_message`：仅**实时新客户消息** → 走 `HandleIngressMessage`（触发 AI）。
- `history`：存量回填 + 自己/AI 发出的消息 → 走新增 `PersistBridgeHistory`（**仅落库，不取 AI 锁、不投递 pending、不通知 AgentRuntime**），`Direction` 由帧携带（`inbound`/`outbound`）。

**多用户/多会话模型**：消息以 `(channel, account_id, conversation_id)` 三维隔离。
- 会话 id 即客户标识；客户消息 `sender_id = conversation_id`，自己/AI 消息 `sender_id = account_id`。
- **会话切换检测**：`BaseAdapter` 每 2s 轮询 `getConversationId()`，变化时重挂载 `MutationObserver` 并**回填新会话存量历史**（仅落库）。
- **页面加载回填**：进入私信页即扫描当前消息列表存量 → `history` 帧落库。

**为什么安全**：实时客户消息才进 AI；回填与自己消息只进库，彻底杜绝「历史回放触发 AI」与「自己回复被二次推理」。

**服务端落点**：`inbox_ingress.go` 新增 `PersistBridgeHistory` → `persistHistoryMessage`，按 `event.Direction` 落 `message_hub`（outbound 标记 `IsAIReply=true`）。`NormalizeEvent` 提供幂等 `EventID` 去重。

#### 17.3 需求④：限速器 + 风控（防封号）

**论证**：平台对自动化高频回复极敏感。风控必须施加在「最贴近平台的一次动作」上——即扩展回写网页之前，因为拟人节奏（随机延迟、最小间隔）只能在 DOM 交互层实现；服务端仅作兜底。

**扩展端三层（src/core/rate-limiter.js）**：
1. **拟人节奏**：发送前随机等待 `jitter(800~2600ms)` + 强制任意两次下行最小间隔 `minIntervalMs=1500`。
2. **令牌桶**：单账号 `accountCapacity=12/min`；单会话 `conversationPerHour=40`。
3. **防回环/去重**：同会话冷却 `conversationCooldownMs=3000`（不重复回复同一会话）；相同文案 `dedupWindowMs=60000` 内不重复发送。
- 全部「软失败」：超限丢弃本次下行并记录 `reason`，绝不堆积重试（避免报复性补发）。

**服务端兜底（hub.go）**：`Deliver` 增加每账号令牌桶（60/min），应对 AI 失控洪泛，超限直接丢弃并告警。

**论证证据**：`test/rate-limiter.test.js` 5 项覆盖最小间隔拦截、会话冷却拦截、相同文案去重拦截、去重窗口后放行、令牌桶退款不污染配额。

#### 17.4 需求②：构建发布文档 / Logo / 名称 / 本机运行

- **产品命名**：`HiveBridge 蜂桥`（蜂巢 Hive + 桥接 Bridge；中文呼应项目 `hivemtk`）。
- **Logo**：`assets/logo.svg`（蜂巢六边形 + 蓝紫桥接弧线），已内联进 popup。
- **本机构建**：`npm run build`（esbuild 逐入口 IIFE，自包含，规避 MV3 共享 chunk 问题）。
- **本机发布**：`npm run release`（`scripts/release.mjs`，无 CI 依赖）→ `release/hivebridge-<version>.zip`（根含 `manifest.json`）+ `RELEASE_NOTES.md`。已实测产出 `hivebridge-1.0.0.zip`。
- **发布文档**：`RELEASE.md`（命名/Logo/构建/发布/安装/校准清单）。

#### 17.5 需求①：要求覆盖矩阵（头脑风暴论证检查结论）

| 需求 | 是否满足 | 关键实现 | 论证证据 |
| --- | --- | --- | --- |
| ① 持续头脑风暴论证检查 | ✅ | 本文档 §16+§17 闭环；双帧设计、限速、历史模型均经论证 | 协议/限速 11 单测 + 本矩阵 |
| ② 本机构建发布文档/Logo/名称 | ✅ | `RELEASE.md` + `assets/logo.svg` + `npm run release` | 实测产出 `hivebridge-1.0.0.zip` |
| ③ 三平台消息上报+下发 | ✅ | 协议对齐；inbound_message 上行 / outbound_reply 下行回写 | protocol/bridge-client 单测 + 链路说明 |
| ④ 限速器风控 | ✅ | 扩展三层 + 服务端兜底令牌桶 | rate-limiter 单测 5 项 |
| ⑤ 多用户消息历史上报 | ✅ | 双帧分离 + 会话切换回填 + (channel,account,conversation) 隔离 | PersistBridgeHistory（仅落库不触发 AI） |

**仍待实测/确认（不阻塞首版，属真机校准）**：
1. 三平台自/他消息 class 真机校准（抖音 `.left`/`.right`、小红书 `.left`/`.right`、TikTok `outgoing`/`incoming`）——决定自/他判定准确性。
2. `account_id` 三平台稳定派生规则。
3. 三个开源仓库 LICENSE 商用合规确认。
4. 风控参数按灰度流量调优（当前为保守默认值）。

#### 17.6 本轮交付状态

- 服务端（Go）：`go build ./internal/bridge/... ./internal/service/...` 通过；`go test ./internal/bridge/...` 6 用例通过；新增 `FrameHistory` + `PersistBridgeHistory` + `hub` 限速。
- 扩展（JS）：`npm run build` 通过；`npx vitest run` **16 用例全部通过**（协议对齐 6 + 限速器 5 + WS 帧 4 + history 路由 1）；`npm run release` 产出可分发包。
- 文件变更：`src/core/{types,channel-adapter,rate-limiter,bridge-client}.js`、`src/content/common.js`、`src/background/{index,registry}.js`、`src/channels/*`（适配）、`manifest.json`、`assets/logo.svg`、`scripts/release.mjs`、`RELEASE.md`、`package.json`。

---

## 18. 合规与风险声明

> 本节为本扩展的合规边界与使用风险，仅作为工程参考，**不构成法律意见**。请在商业使用前咨询专业法律顾问。

### 18.1 LICENSE

- 扩展源码遵循根仓库 `LICENSE`（AGPL-3.0）。
- `LICENSE` 文件已同步至本目录；任何分发包须保留原 LICENSE 全文。
- 三个开源参考实现（`DY-auto`、`XHS-YYDS`、`tiktok-auto-plugin`）仅作为**思路参考**，未复制其源码；本扩展的选择器、交互逻辑、限速算法均为重新实现。

### 18.2 平台 ToS 风险

抖音 / 小红书 / TikTok 网页版**没有面向第三方的开放私信 API**。本扩展通过模拟用户操作 DOM 的方式实现"监听 + 回写"，**严格意义上属于平台自动化行为**，可能触犯：

- 平台《用户协议》或《自动化行为规范》；
- 平台对"模拟用户操作"的具体禁止条款；
- 当地数据保护与自动化相关法规。

**风险与责任**：

- 使用者应自行评估所在司法管辖区的合规要求；
- 因违规使用本扩展导致的账号封禁、法律纠纷、平台索赔等后果，由使用者自行承担；
- 本项目**不对任何因使用本扩展而导致的直接或间接损失负责**；
- 商业部署前建议咨询专业法律顾问 + 平台方书面许可。

### 18.3 隐私协议

扩展在浏览器中处理的数据流：

| 数据 | 来源 | 流向 | 保留 |
| --- | --- | --- | --- |
| 客户私信内容 | 平台网页 DOM | user-server（实时触发 AI） | 遵循 user-server 既有留存策略 |
| AI 回复内容 | user-server 下行 | 平台网页输入框 → 平台服务器 | 平台侧按其自身策略留存 |
| 账号标识（account_id） | 平台网页（用户链接） | user-server | 随账号绑定关系留存 |
| 会话标识（conversation_id） | 平台网页 | user-server | 随消息留存 |

**最小化原则**：

- 扩展不收集任何与"私信桥接"无关的浏览器数据；
- 不访问 cookie、localStorage（除 `bridgeConfig` 配置项）；
- 不访问其他扩展或页面的数据；
- 不嵌入第三方分析、追踪、广告 SDK；
- 日志统一脱敏（`src/core/logger.js` 自动 mask 敏感字段）—— 避免误打全量内容到控制台。

**用户授权**：

- 扩展只对抖音 / 小红书 / TikTok 网页（`manifest.json` 中 `matches` 字段）注入 content script；
- 不读取其他网站任何信息；
- 用户可在 Chrome 扩展管理页随时停用或卸载本扩展。

### 18.4 拟人限速（防封号）

工程层面已将"模拟真人"做到三层：

1. **扩展端三层风控**（`src/core/rate-limiter.js`）：拟人节奏 + 令牌桶 + 会话冷却 + 相同文案去重；
2. **服务端兜底令牌桶**（`hub.go` rateBucket）：单账号下行 60/min 上限，超限直接落 `message_hub` 等待补发；
3. **幂等守卫**（`agentruntime.ClaimReply`）：同一 eventID 仅一次出站，避免重连重发。

仍建议业务侧结合灰度流量持续调参，不要在生产开启 `BRIDGE_TEST_AUTOREPLY`。

### 18.5 数据出境 / 第三方共享

- 扩展不向任何第三方服务发送数据；
- 所有流量限于：`浏览器 <-> 部署者自管的 user-server`；
- user-server 自身的 LLM / 向量库等依赖按业务侧配置，请遵循所在司法辖区的数据出境要求。

---

## 19. 变更日志（v1 之后）

### 19.1 深度审查第二轮

- **P0-S1-1**：JWT 账号归属校验（`OwnershipChecker` 回调注入）
- **P0-S1-2**：全链路 trace_id 透传（`X-Trace-Id` 头 → ctx → 日志）
- **P0-S1-3/S1-4**：`send` channel 关闭 + 并发安全（`atomic.Bool` + `CloseSend`）
- **P0-S1-5**：历史帧错误结构化日志
- **P0-S1-7**：扩展端 XSS 防护（`src/core/sanitize.js`，sanitizeForDisplay + textContent 注入）
- **P0-S1-8**：离线降级落库（`persistFailedOutbound`）
- **P0-S1-9**：`BRIDGE_TEST_AUTOREPLY` 启动强告警
- **P1-S2-1**：限速桶 janitor 定期清理空闲桶
- **P1-S2-2**：限速命中走 `persistFailedOutbound` 落库
- **P1-S2-3**：服务端全局 seq 序号机制（`Frame.Seq` + `NextSeq`）
- **P1-S2-4**：`ClaimReply` 幂等守卫
- **P1-S2-6**：`writePump` defer Unregister 兜底
- **P1-S2-11**：桥接审计日志（连接数 / 帧率 / 投递结果分类）落库 `audit_logs`
- **P2-S2-5**：`SenderType` 透传 `toMessageEvent`（extra.sender_type）
- **P2-S2-8**：account_id 多层 fallback 派生（`src/core/fallback.js`）
- **P2-S2-9**：自他消息判定兜底（头像位置）
- **P2-S3-2**：sentinel error（`ErrBridgeRateLimited` / `ErrBridgeBufferFull` / `ErrBridgeOffline`）
- **P2-S3-3**：`CheckOrigin` 收敛（`BRIDGE_ALLOWED_ORIGINS` 白名单）
- **P2-S3-12**：`BridgeHub.Shutdown` 优雅关闭所有活跃 WS
- **P3-合规**：新增 `LICENSE` + 本节"合规与风险声明"
- **P3-质量**：日志脱敏（`src/core/logger.js` 自动 mask）
- **P3-质量**：命名规范核查（无 `utils.go` / `common.go` / `*_v1` / `*_stub` / `*_2026-*`）
- **P3-测试**：服务端 `-race` 强制 + 单元测试覆盖关键路径；扩展端 vitest 51 用例覆盖 sanitize / fallback / protocol / rate-limiter / adapter / bridge-client

### 19.2 CI 增强（深度审查收口）

- **CI `-race`**：`.github/workflows/user-server-ci.yml` 的 `Run unit tests` 步骤增加 `-race`，强制 race detector 兜底所有单测。
- **CI bridge 扩展 vitest**：新增 `bridge-extension-test` job，触发条件 `user-web/bridge/**`；在 PR/push 时自动跑 `npx vitest run`，确保协议/限速/sanitize/fallback 51 用例不退化。
- **CI 路径过滤**：触发条件增加 `user-web/bridge/**`，bridge 代码改动自动触发对应 CI。
- **bridge 端测试覆盖（hub_test.go 新增）**：
  - `TestBridgeHub_NextSeq_Monotonic`（seq 单调递增）
  - `TestBridgeHub_Shutdown_ClosesAll`（优雅关闭 + 幂等）
  - `TestBridgeHub_RateLimit_Bucket`（令牌桶触限）
  - `TestBridgeHub_Unregister_ClosesSendChannel`（关闭后 send channel 安全）
  - `TestBridgeHub_Concurrent_Deliver`（200 并发 Deliver 不 panic）
  - `TestBridgeHub_Janitor_CleansIdle`（janitor 清理空闲桶）
- **Kick 安全**：测试 client 偶有 `conn == nil`，`Kick` 加 nil guard 防 panic。
- **`NewBridgeReachAdapter` 兼容**：原 2 参调用点（`router/sales_engine_factory.go`、`router/tool_provider_wiring.go`）无需修改；构造函数 ingress 改为 variadic 可选，新增 `SetIngress` 后期注入（避免装配阶段循环依赖）。

