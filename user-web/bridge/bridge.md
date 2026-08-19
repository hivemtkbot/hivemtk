# Bridge · HiveBridge 蜂桥

> 通过部署在用户浏览器里的 Chrome 扩展（`user-web/bridge`），
> 把抖音 / 小红书 / TikTok / 闲鱼 / 快手 网页私信实时桥接到 `user-server` 的 AI 智能体，
> 实现"客户发私信 → 智能体自动回复 → 回复回写网页发送"的闭环。

**核心特性：**

- 实时上行：客户私信 → 触发 AI 自动回复
- 历史回填：会话切换 / 页面刷新时把存量消息落库（不触发 AI，防回环）
- 下行回写：AI 回复 → 回写网页私信输入框 → 自动发送
- 拟人限速风控：拟人节奏 + 令牌桶 + 会话冷却 + 相同文案去重
- 多用户多会话：按 `(channel, account_id, conversation_id)` 三维隔离
- MV3 Manifest V3 + 纯 JavaScript + esbuild 逐入口 IIFE 打包

---

## 0. 阅读指引

| 节 | 内容 |
| --- | --- |
| 1 | 背景、现状 |
| 2 | 整体架构与数据流 |
| 3 | 领域模型（统一消息） |
| 4 | 协议（HTTP 三通道） |
| 5 | 服务端设计（`user-server/internal/bridge`） |
| 6 | 扩展设计（`user-web/bridge`） |
| 7 | 限速 / 风控 / 安全 |
| 8 | 配置与默认值（详见 `docs/DEFAULTS.md`） |
| 9 | 测试策略 |
| 10 | 真机校准清单 |

辅助文档：

- [./docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) — 架构总览
- [./docs/DEFAULTS.md](./docs/DEFAULTS.md) — 默认值单一源
- [./docs/dev/DEVELOPMENT.md](./docs/dev/DEVELOPMENT.md) — 二次开发手册
- [./RELEASE.md](./RELEASE.md) — 构建与发布

---

## 1. 背景与现状

### 1.1 为什么需要浏览器桥接

抖音 / 小红书 / TikTok 网页版**没有面向第三方的开放私信 API**（尤其个人/企业号网页端）。现有的 `user-server` 触达渠道（微信、企微、飞书、Telegram、WhatsApp 等）都依赖官方 API 或 webhook 主动回调。对于这三类平台，**唯一可行的"收发"路径就是模拟用户在网页上操作 DOM**（监听新消息、往输入框填字、点发送）。

因此需要 Chrome 扩展充当浏览器侧代理：

1. 在抖音 / 小红书 / TikTok / 闲鱼 / 快手 网页注入 content script，监听并解析私信；
2. 上报新私信到 `user-server`；
3. 接收 `user-server` 下发的 AI 回复，回写到网页发送框并点击发送。

### 1.2 现有代码已经具备的能力（最大化复用）

`user-server` 端以下能力**已现成**，bridge 只做"接线"而非"重造"：

| 能力 | 位置 | 复用方式 |
| --- | --- | --- |
| 统一消息中台入口 | `internal/service/inbox_ingress.go` → `InboxIngressService.HandleIngressMessage` | bridge 入站直接调用，自动触发 AI |
| 统一消息标准 | `internal/model/message_event.go` → `MessageEvent` | bridge 与中台之间的协议契约 |
| AI 触发与编排 | `internal/aiagent/agent/runtime` + `SmartCSOrchestrator` | 入站后自动跑，无需 bridge 关心 |
| 智能体↔渠道绑定 | `internal/service/ai_agent.go` → `ChannelAgentBinding(channel_type, account_id)→agent_id` | bridge 账号注册即可多智能体路由 |
| 统一触达（出站）接口 | `internal/aiagent/agent/tooluse/reach_tools.go` → `ReachAdapter` 接口 | 包装为 `BridgeReachAdapter`，把网页渠道回复写入 HTTP outbox 缓冲 |
| 渠道标识常量 | `internal/model/message_event.go`：`ChannelDouyin` / `ChannelXHS` / `ChannelTikTok` / `ChannelXianyu` / `ChannelKuaishou` | bridge 直接复用 |

> 关键结论：入站天然复用、出站天然有扩展点（ReachAdapter）、智能体绑定天然支持。bridge 的工程量是"把扩展点接上" + "写 Chrome 扩展"。

---

## 2. 整体架构与数据流

```
┌─────────────────────────────────────────────────────────────────────────┐
│  浏览器（用户登录了抖音/小红书/TikTok/闲鱼/快手 网页）                       │
│                                                                          │
│  ┌─────────────── Chrome 扩展 user-web/bridge ───────────────────────┐   │
│  │  content script（每渠道一个）                                       │   │
│  │   ├─ 监听 DOM 新私信 → 解析为 UnifiedMessage                        │   │
│  │   ├─ 监听"待发送回复" → 填输入框 + 点发送                          │   │
│  │   └─ background 路由消息（chrome.runtime.connect 长连接 port）       │   │
│  │  background service worker                                         │   │
│  │   ├─ Uplink：合并窗口批量 POST /api/bridge/ingest                  │   │
│  │   ├─ Downlink：定时 GET /api/bridge/outbox 拉取待发回复            │   │
│  │   └─ Ack：发送成功后 POST /api/bridge/outbox/ack 标记 delivered    │   │
│  └──────────────────────────────────────────────────────────────────┘   │
└───────────────────────────────┬──────────────────────────────────────────┘
                                │  HTTP (Authorization: Bearer <token>)
                                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  user-server                                                             │
│                                                                          │
│  BridgeIngestHandler                                                     │
│    POST /api/bridge/ingest       (InitGuard 校验 + 入站 + 拉下行回复)     │
│    GET  /api/bridge/outbox       (InitGuard 校验 + 拉待发 reply)         │
│    POST /api/bridge/outbox/ack   (InitGuard 校验 + 标记 delivered)       │
│                                                                          │
│  InboxIngressService（现有）──► AgentRuntime ──► SmartCSOrchestrator     │
│                                              │                           │
│                                              ▼                           │
│                                   ReachAdapter.Send(req)  ◄── AI 回复   │
│                                        │                                │
│                          ┌─────────────┴──────────────┐                 │
│                          ▼                            ▼                 │
│               IntegrationReachAdapter        BridgeReachAdapter         │
│               （官方 API 渠道）   （网页渠道 → 入 HTTP outbox，扩展     │
│                                       下次轮询 GET 时拉回）             │
│                                                                          │
│  channelgw：HTTP / WS 协议类型单源（IngestMessage / OutboxMessage /     │
│             HistoryItem），与前端 `src/core/types.js` 严格对齐            │
└─────────────────────────────────────────────────────────────────────────┘
```

一句话数据流：

> **网页私信 → content script 监听 → background 合并上行 → `POST /api/bridge/ingest` → `InboxIngressService` → AI 智能体 → `ReachAdapter` → `BridgeReachAdapter` 写入 outbox → 扩展下次 `GET /api/bridge/outbox` 拉回 → content script 回写网页 → 发送成功后 `POST /api/bridge/outbox/ack`。**

---

## 3. 领域模型：统一消息

各平台私信结构不同，bridge 在 content script 层就把它们**规范化为统一消息**，后续链路只认统一消息。

```ts
// user-web/bridge/src/core/types.js
export type BridgeChannel =
  | 'douyin'        // 抖音
  | 'xiaohongshu'   // 小红书
  | 'tiktok'        // TikTok
  | 'xianyu'        // 闲鱼
  | 'kuaishou';     // 快手

export interface UnifiedMessage {
  event_id: string;        // 平台内唯一消息 ID（用于幂等去重）
  channel: BridgeChannel;  // 渠道
  account_id: string;      // 本机登录的账号标识，路由到正确智能体
  conversation_id: string; // 会话 ID
  sender_id: string;       // 客户唯一 ID（客户消息=conversation_id；自己/AI=account_id）
  sender_name?: string;
  receiver_id?: string;
  msg_type: 'text' | 'image' | 'audio' | 'video' | 'card';
  content: string;         // 文本；非文本时放描述/URL
  media_url?: string;
  timestamp: number;       // 毫秒
  direction?: 'inbound' | 'outbound';  // history 帧专用
  is_group?: boolean;
  group_id?: string;
  group_name?: string;
  history?: UnifiedMessage[];          // 该会话多轮历史
  extra?: Record<string, unknown>;     // 透传平台原始字段
}
```

> 字段命名严格对齐 `user-server/internal/channelgw`（HTTP / WS 共用的协议类型单源）。
> 服务端 BridgeIngestHandler 把 `UnifiedMessage` 映射为 `model.MessageEvent`，无需理解平台语义。

---

## 4. 协议（HTTP 三通道）

bridge ↔ user-server 走**纯 HTTP 三通道**（非 WebSocket，非 SSE）。每个通道相互独立、参数可配置。

### 4.1 通道 A · 上行（Uplink）

```
POST /api/bridge/ingest?channel=<ch>&account_id=<acc>[&conversation_id=<c>]
Headers:
  Authorization: Bearer <token>          # token 走 Header，不进 URL
  Content-Type: application/json
  X-Request-Id: <trace-id>               # 端到端 trace 透传（P3-E）

Body:
  {
    "channel": "douyin",
    "account_id": "acc-001",
    "conversation_id": "conv-abc",
    "messages": [UnifiedMessage, ...]
  }

Response 200:
  {
    "ok": true,
    "ingested": [{"event_id": "...", "accepted": true}, ...]
  }
```

> **2026-08-18 二次审核**：早期文档承诺 `outbound_replies` 随 ingest 响应返回（省一轮下行轮询）。
> 实际代码中该字段恒空，下行已统一走通道 B 独立轮询（GET /api/bridge/outbox）。
> 为避免扩展端按文档"省一次请求"造成 bug，已从 `IngestResponse` 中删除该字段。

### 4.2 通道 B · 下行（Downlink · 轮询）

```
GET /api/bridge/outbox?channel=<ch>&account_id=<acc>&limit=<n>
Headers:
  Authorization: Bearer <token>

Response 200:
  {
    "status": "ok",
    "messages": [{
      "msg_id": "mh:abc12345",
      "channel": "douyin",
      "account_id": "acc-001",
      "conversation_id": "conv-abc",
      "msg_type": "text",
      "content": "...",
      "reply_to_event_id": "...",
      "truncated": false,
      "extra": { ... }
    }, ...]
  }
```

扩展按 `BRIDGE_THREE_CHANNEL.outboxPollIntervalMs`（默认 1500ms）定时拉取；批大小 `BRIDGE_THREE_CHANNEL.outboxBatchSize`（默认 50）。

### 4.3 通道 C · 状态（Ack）

```
POST /api/bridge/outbox/ack?channel=<ch>&account_id=<acc>
Headers:
  Authorization: Bearer <token>
  Content-Type: application/json

Body（v1 协议，与前端 v2 单源化常量 BRIDGE_PROTOCOL_V2 兼容）:
  {
    "msg_ids": ["mh:abc", "mh:def"],
    "status": "delivered" | "failed"
  }

Response 200:
  {
    "status": "ok",
    "acked": 1, "duplicate_count": 0, "not_found_count": 1,
    "items": [
      { "msg_id": "mh:abc", "status": "acked" },
      { "msg_id": "mh:def", "status": "not_found" }
    ]
  }
```

详细状态机与 v2 协议见 `src/core/downlink.js` 与 `internal/bridge/handler_http.go`。

### 4.4 鉴权

- 仅过 `InitGuard` 中间件（系统须已初始化，私有化部署单用户）；
- 不过 JWT，账号以 `channel` + `account_id` 在 URL 自证身份；
- `Authorization: Bearer <token>` Header 可选（增强部署可加 token 校验）；
- `account_id` 缺失 → 400 拒绝（不写 `default` 兜底，避免脏数据）；
- `channel` 不在白名单（`douyin/xiaohongshu/tiktok/xianyu/kuaishou`） → 400 拒绝。

### 4.5 为什么是 HTTP 三通道（不是 WebSocket / SSE）

- **MV3 Service Worker 友好**：WS 长连接在 SW 冻结/恢复时易断，HTTP 三通道天然无状态；
- **可测性强**：`curl` 即可端到端联调，无需专用 WS 客户端；
- **OOM 安全**：无长连接、无心跳、无 map 索引；
- **离线不丢回复**：AI 回复入 outbox 持久层，扩展下次轮询自然拉回（连接恢复即补发）；
- **运营友好**：故障定位只需看 HTTP access log。

> 30s 实时性损失换来架构简单 / 部署简单 / 故障定位简单，私域客服场景接受。

---

## 5. 服务端设计（`user-server/internal/bridge`）

### 5.1 包结构

```
internal/bridge/
  handler_http.go          POST /api/bridge/ingest  入口（InitGuard + 入站 + 拉下行回复）
  handler_http_*.go        7 个测试文件
  http_reply_buffer.go     内存 reply 缓冲（outbox 拉取的源）
  reach_adapter.go         BridgeReachAdapter：网页渠道 AI 回复入 outbox
  frames.go                UnifiedMessage / UnifiedReply / Frame 数据模型
  channel.go               渠道常量（douyin/xiaohongshu/tiktok/xianyu/kuaishou）
  account.go / account_repo.go  桥接账号 CRUD + IsOnline
  bridge_helpers.go        共享工具：token 脱敏 / body 解析 / HistoryToEvent
```

> 协议类型单源：`internal/channelgw`（HTTP / WS 共用 IngestMessage / OutboxMessage / HistoryItem）。

### 5.2 HTTP 端点注册

`internal/router/router.go` 注册（`bridgeWS` 路由组，仅过 `InitGuard`）：

```go
bridgeWS.POST("/bridge/ingest",       bridgeHandler.HandleHTTPIngest)
bridgeWS.GET ("/bridge/outbox",       bridgeHandler.GetBridgeOutbox)
bridgeWS.POST("/bridge/outbox/ack",   bridgeHandler.AckBridgeOutbox)
```

### 5.3 入站接线（零改动中台）

`BridgeIngestHandler.HandleHTTPIngest`：

1. 解析 URL query（channel / account_id / conversation_id）；
2. 校验 `account_id` 必填（缺失 → 400 `account_id required`）；
3. 校验 `channel` 在白名单；
4. 读取 body（限 `HTTPIngestMaxBodySize = 4MB`、限 `HTTPIngestMaxMessages = 200`）；
5. 逐条 `UnifiedMessage` → `model.MessageEvent`，调 `InboxIngressService.HandleIngressMessage`；
6. 拉取该 `(channel, account, conversation)` 的待发 reply 随响应返回。

### 5.4 出站接线：`BridgeReachAdapter`

```go
type BridgeReachAdapter struct {
    inner    *tooluse.IntegrationReachAdapter  // 非网页渠道委托
    outbox   OutboxWriter                       // 网页渠道 AI 回复入 outbox
}

func (a *BridgeReachAdapter) Send(ctx, req) (string, error) {
    if isBridgeChannel(req.Channel) {
        payload := buildOutbound(req)   // *UnifiedReply
        a.outbox.Enqueue(payload)       // 入 outbox
        return "bridge:" + req.Channel + ":" + req.AccountID, nil
    }
    return a.inner.Send(ctx, req)        // 其它渠道原样委托
}
```

接线点（最小侵入）：

- `internal/router/sales_engine_factory.go` `registerAgentReachTools` 用 `bridge.NewBridgeReachAdapter(...)` 包装原 adapter；
- `internal/router/tool_provider_wiring.go` `ReachToolProvider.Provide` 同理替换；
- `WebhookService.sendOutbound` / `bridge_account_controller.SendManual` 经 `RegisterBridgeOutbound` 回调把 reply 入 outbox（回调注入避免 service → bridge 导入环）。

> AI 智能体的 `reach.web.send` 等工具在"网页渠道"下自动改走 HTTP outbox，对编排层完全透明。

### 5.5 渠道标识与智能体绑定

- 渠道常量（`internal/model/message_event.go`）：`ChannelDouyin` / `ChannelXHS` / `ChannelTikTok` / `ChannelXianyu` / `ChannelKuaishou`（无 `_web` 后缀）。
- 智能体绑定表 `channel_agent_bindings` 写入 `(channel_type, account_id, agent_id)`。
- `account_id` 由扩展在请求 URL 上报（content script 从页面本账号主页派生），保证"同一人多个浏览器/账号"互不串号。

---

## 6. 扩展设计（`user-web/bridge`）

### 6.1 设计原则

1. **按渠道封装**：`channels/{douyin,xhs,tiktok,xianyu,kuaishou}.js` 各自独立；
2. **渠道内保持抽象，便于扩展评论/回关等**：每个渠道实现统一接口 `ChannelAdapter`（`src/core/channel-adapter.js`）；
3. **桥接职责单一**：background service worker 只负责 HTTP 三通道调度与路由；content script 只负责 DOM 操作；
4. **左列表驱动**：上行先点开会话再读消息，下行先点开目标用户再发送；不依赖"当前恰好打开的会话"。

### 6.2 目录结构

```
user-web/bridge/
  bridge.md                  # 本文档
  manifest.json              # MV3 扩展声明
  package.json
  vite.config.js             # 仅 vitest 使用
  eslint.config.mjs
  scripts/
    build.mjs                # esbuild 逐入口独立 IIFE 打包
    gen-icons.mjs            # 零依赖生成 PNG 图标
    release.mjs              # 本机构建发布（无 CI 依赖）
    mock-upstream-url.mjs    # 上行 URL / 参数 mock 验证
  src/
    core/                    # 核心模块（与平台无关）
      types.js               # UnifiedMessage / UnifiedReply
      channel-adapter.js     # BaseAdapter + ChannelAdapter 接口
      base-adapter.js        # MutationObserver / 去重 / 上下行封装
      http-ingest.js         # postIngest / getOutbox / ackOutbox
      uplink.js              # Uplink 队列（合并窗口 + 持久化 _confirmed）
      downlink.js            # pollDownlink + _pendingAck 重试 + SentCache
      polling-loop.js        # 下发轮询调度器
      circuit-breaker.js     # 熔断器 + 幂等键
      rate-limiter.js        # 拟人节奏 + 令牌桶 + 会话冷却 + 去重
      humanize.js            # 拟人化（贝塞尔鼠标 + 键入节奏）
      sanitize.js            # XSS 防护
      config-store.js        # 配置热更新
      selector-engine.js     # 选择器引擎
      selector-ai.js         # 选择器 UI 配置面板（chrome.storage 持久化）
      fallback.js            # account_id fallback 派生
      offline-cache.js       # 离线缓存
      logger.js              # 频道着色 + 敏感字段脱敏
      constants.js           # ⭐ 默认值单一源
    channels/                # 各平台适配层
      douyin.js xhs.js tiktok.js xianyu.js kuaishou.js
    content/                 # content script 入口
      common.js              # PollingLoop / dispatchOutbound / 解析
      douyin.js xhs.js tiktok.js xianyu.js kuaishou.js
    background/              # 后台 service worker
      index.js               # HTTP 三通道调度 + port 路由 + 配置/状态
      injector.js            # content script 注入
    popup/                   # popup UI
      index.html
      index.js               # 后端地址配置 / 状态 / 自检
      accounts.js            # 多账号管理
      config-io.js           # 配置读写
      alert-banner.js        # 告警横幅
      error-messages.js      # 错误文案友好化
      health.js              # 健康度面板
      emergency-stop.js      # 紧急停止
  docs/
    ARCHITECTURE.md          # 架构总览
    DEFAULTS.md              # 默认值单一文档源
    dev/DEVELOPMENT.md       # 二次开发手册
  test/                      # Vitest 单测（44 文件，674 用例）
  assets/
    icon.svg logo.svg icons/  # 蜂巢 logo + 栅格图标
  dist/                      # esbuild 产物（不入仓）
  release/                   # 发布产物
```

### 6.3 协议常量（单源）

`src/core/constants.js` 的 `PROTOCOL` 与 `user-server/internal/bridge/frames.go` 严格对齐（且经 `internal/channelgw` 收敛为单一类型）：

```javascript
export const PROTOCOL = Object.freeze({
  CHANNELS: { DOUYIN: 'douyin', XHS: 'xiaohongshu', TIKTOK: 'tiktok', XIANYU: 'xianyu', KUAISHOU: 'kuaishou' },
  FRAME: {
    REGISTER: 'register',          // 协议帧（已不直接使用，但保留常量）
    INBOUND: 'inbound_message',
    HISTORY: 'history',
    OUTBOUND: 'outbound_reply',
    PONG: 'pong', PING: 'ping',
    ACK: 'ack', ERROR: 'error',
  },
  SENDER: { CUSTOMER: 'customer', AGENT: 'agent', SELF: 'self', SYSTEM: 'system' },
  DIRECTION: { INBOUND: 'inbound', OUTBOUND: 'outbound' },
});
```

### 6.4 左侧列表驱动交互模型

抖音 / 小红书 IM 网页的私信页是「左侧会话列表 + 右侧聊天线程」双栏结构。一切收发都通过左侧列表驱动：

**上行**：

```
左侧会话列表枚举（getConversationList）
  └─ 逐个会话：openConversation(cid)
       ├─ 左侧找目标项 → 滚入视口 → 模拟真实点击
       ├─ 等待 getConversationId() 变为目标（SPA 异步渲染容忍）
       └─ _backfill() 读取右侧线程全部消息 → 会话级 history 帧（仅落库）

实时新消息：MutationObserver + 3s 兜底扫描 → inbound 帧（触发 AI）
```

**下行**：

```
sendOutbound(text, targetConvId)
  ├─ targetConvId == 当前会话 → 直接模拟输入发送
  ├─ targetConvId ≠ 当前会话 → openConversation(targetConvId)
  │    ├─ 左侧列表找目标用户 → 点击进入右侧聊天页
  │    └─ 找不到目标 → 放弃发送（防串台）
  └─ 模拟输入（fillContentEditable / setValue） + 点发送按钮
```

> 关键行为：下行回复**不再丢弃「非当前会话」的消息**——那是发给正确用户的必经路径。

### 6.5 会话 ID 提取（兜底链）

| 渠道 | 兜底链（自上而下） |
| --- | --- |
| 小红书 | `/chat/{id}` 路径 → `?conversation_id` → 活动项 data-* → header 对方链接（跳过自己账号）→ 容器 data-id → 昵称派生 `conv:<name>` |
| 抖音 | `/chat/{id}` 路径 → `?group/conversation_id` → 活动项 data-* → `/user/` 链接 → 昵称派生 `conv:<name>` |
| TikTok | 活动 chat list 项的 data-id / data-e2e / 对方主页链接 |
| 闲鱼 | 会话列表项 data-* → URL 路径 / query |
| 快手 | 会话列表项 data-* → URL 路径 / query |

> 昵称派生是兜底：/chat 专用路由的活动会话项常无 `/user/` 链接、无 data-id 属性，昵称在同会话内恒定，足够作会话聚合键。

### 6.6 自/他判定（已移交后端）

**前端的"自/他消息"判定已完全移交后端**——content script 不再计算几何/类名判定。

- 前端 `UnifiedMessage.sender_type = "customer"` 一律上报；
- 服务端 `InboxIngressService.isPlatformOutboundEcho` 按"内容回显"权威识别平台回显 → 视为 SELF 跳过；否则按 CUSTOMER 入库；
- 前端仅识别系统/撤回消息并标记 `sender_type="system"`。

---

## 7. 限速 / 风控 / 安全

### 7.1 扩展端三层风控（`src/core/rate-limiter.js`）

1. **拟人节奏**（`humanize.js`）：发送前随机等待 `jitter(800~2600ms)` + 强制任意两次下行最小间隔 `minIntervalMs=1500`；贝塞尔鼠标轨迹 + 键入节奏建模；
2. **令牌桶**：单账号 `accountCapacity=12/min`；单会话 `conversationPerHour=40`；
3. **防回环 / 去重**：同会话冷却 `conversationCooldownMs=3000`（不重复回复同一会话）；相同文案 `dedupWindowMs=60000` 内不重复发送。

> 全部"软失败"：超限丢弃本次下行并记录 `reason`，绝不堆积重试（避免报复性补发）。

### 7.2 服务端兜底（`reach_adapter.go` / outbox）

- 入 outbox 前每账号令牌桶（60/min），应对 AI 失控洪泛；
- 超限直接丢弃并告警；
- ack 重试：`_pendingAck` Map 最多重试 10 次，指数退避 1s→60s 封顶，24h TTL。

### 7.3 幂等 / 重复处理

- `event_id` 客户端用 `contentHash(channel, content)`（FNV-1a，前缀 `mh:`）派生；
- `InboxIngressService` 已有 `MessageEvent.EventID` 唯一约束 + AI 锁；
- 出站 `BridgeReachAdapter` 复用 `agentruntime.ClaimReply(eventID)` 幂等守卫，同一回复只入 outbox 一次。

### 7.4 内容安全

- `maxReplyContentBytes=4KB`（前端 + 后端对齐），避免 XSS payload 巨大；
- `sanitize.js` 在 popup / 日志中 `textContent` 注入而非 innerHTML；
- `logger.js` 自动 mask 超过 24 字符的字符串（防 PII 泄露）。

### 7.5 鉴权

- 服务端仅 `InitGuard`（私有化部署单用户，无 token）；
- 可选 `Authorization: Bearer <token>` Header（增强部署）；
- `account_id` 缺失 / `channel` 非白名单 → 400 拒绝（不写 `default` 兜底）。

---

## 8. 配置与默认值

所有默认值的单一源：

| 维度 | 单一源 | 文档 |
| --- | --- | --- |
| 扩展端 | `user-web/bridge/src/core/constants.js` | [./docs/DEFAULTS.md](./docs/DEFAULTS.md) |
| 服务端 | `user-server/internal/config/ports.go` + `user-server/internal/bridge/handler_http.go` | [./docs/DEFAULTS.md](./docs/DEFAULTS.md) + `user-server/docs/dev/DEVELOPMENT.md` §2.4 |

**硬约束**：

- 禁止"软启动"（默认值兜底为空）；
- 禁止"多处硬编码"（同一数字字面量在多处出现）；
- 禁止"兜底成空值"（config 缺失应明确报错）。

调整流程见 [./docs/DEFAULTS.md §3](./docs/DEFAULTS.md)。

---

## 9. 测试策略

### 9.1 服务端

```bash
cd user-server
go test -race -count=1 ./internal/bridge/...
```

覆盖范围：

- `handler_http_p0_test.go` / `handler_http_p3d_test.go` / `handler_http_ack_test.go` / `handler_http_metrics_test.go` / `handler_http_mock_e2e_test.go` / `handler_http_test.go` — 三个端点的入参/出参/错误分支；
- `http_reply_buffer_test.go` — 内存 outbox 容量 / 超时 / 拉取匹配；
- `reach_adapter_test.go` — bridge 渠道入缓冲、非 bridge 委托 inner；
- `bridge_helpers_test.go` / `bridge_outbound_test.go` / `channel_test.go` / `defaults_test.go` / `fix_2026_08_14_test.go` — 工具与历史修复点；
- CI `-race` 强制：`.github/workflows/`。

### 9.2 扩展端

```bash
cd user-web/bridge
npx vitest run
```

当前覆盖（44 文件 / 674 用例）：

- `constants.test.js` — 与 DEFAULTS.md 字面一致；
- `protocol.test.js` — 帧类型与字段名；
- `http-ingest.test.js` — URL 构造 / Header / 重试；
- `uplink.test.js` / `downlink.test.js` / `polling-loop.test.js` — 三通道调度；
- `p3d-contract.test.js` / `p3h-e2e.test.js` — 前后端契约 + e2e；
- `adapter*.test.js` / `douyin-*.test.js` / `xhs-*.test.js` / `xianyu.test.js` — 各渠道 DOM 解析与发送；
- `rate-limiter*.test.js` / `circuit-breaker*.test.js` / `humanize.test.js` — 风控三件套；
- `sanitize.test.js` / `fallback.test.js` / `popup*.test.js` / `background.test.js` — UI 与基础。

### 9.3 端到端（手动）

```bash
# 1. user-server 启动（必先）
cd user-server && go run ./cmd/api

# 2. 扩展构建
cd user-web/bridge && npm run build

# 3. Chrome 加载 dist/

# 4. 验证
#   - 客户发消息 → user-server 收到 inbound_message（AI 触发）
#   - AI 回复回写到网页并出现在对话框
#   - 刷新/切换会话后历史进入 user-server（history 帧，direction=inbound/outbound）
#   - 连续高频消息被限速拦截（popup / 后台日志可见 reason）
```

---

## 10. 真机校准清单（上线前必做）

平台网页 DOM 经常改版，每次升级前必须做真机校准：

- [ ] 抖音：私信列表容器 / 消息气泡 / 输入框 / 发送按钮真实生效
- [ ] 小红书：`/chat` 路由 + 活动项 `.xhs-im-conv-item` + 气泡 `.chat-item` + 输入框 `[contenteditable]` 真实生效
- [ ] TikTok：DraftEditor 输入框 / 发送动作（Enter vs 飞机按钮）/ 消息气泡
- [ ] 闲鱼 / 快手：会话列表 / 消息气泡 / 输入框真实生效
- [ ] `account_id` 派生规则（平台账号标识稳定可取）
- [ ] 受控输入框填值不被框架拦截（粘贴 + input 事件双写）
- [ ] 会话切换时历史回填进入 `user-server`（`history` 帧，不触发 AI）
- [ ] 拟人节奏不触发平台风控（连续点击 / 高频发送不出现滑块 / 封号预警）
- [ ] popup「自检」按钮能正确识别当前页面

校准流程见 [./docs/dev/DEVELOPMENT.md §6.2](./docs/dev/DEVELOPMENT.md)。
