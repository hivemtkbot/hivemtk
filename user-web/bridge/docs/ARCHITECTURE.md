# bridge 架构说明（2026-08-05 渠道编码统一 + OOM 巡检整改）

## 1. WS 长连接 vs HTTP 轮询：使用 WS 长连接

bridge 扩展到 user-server 之间使用 **WebSocket 长连接**（不是 HTTP 轮询）。

### 1.1 连接建立

```
扩展 background/index.js
  └─ registry.ensure({...})
       └─ BridgeClient.connect()
            └─ new WebSocket('ws://server:8204/api/ws/bridge?channel=...&account_id=...')
                 └─ 服务端 handler.go HandleWebSocket（验 channel + 可选 JWT）
```

- 每个 (channel, account_id) 一条独立 WS 连接
- WS 端点：`/api/ws/bridge`（与 `user-server/internal/router/service_routes.go:90` 对齐）
- 不使用 HTTP 轮询：HTTP 短轮询会带来秒级延迟 + 服务端 / 客户端双重资源消耗

### 1.2 心跳保活

| 角色 | 机制 | 间隔 | 文档源 |
|------|------|------|--------|
| 服务端 | gorilla/websocket SetPongHandler + pingPeriod | 50s | `bridge/handler.go:59` |
| 客户端 | setInterval + JSON {type:"pong"} 帧 | 25s | `core/bridge-client.js:104-116` |
| 客户端超时重连 | serverIdleTimeoutMs 内无任何帧 | 25s | `core/constants.js:96` |

- 客户端 25s < 服务端 60s 错开 35s，避免客户端先断导致连接泄漏
- 客户端发送 JSON pong 帧后置 alive=false，等服务端下次下行（pong/ack/outbound/error）置 true
- 25s 内 alive 仍 false → 主动 `ws.close()` 触发指数退避重连（1→2→4→8→16→30s 封顶 + 500ms jitter）

### 1.3 帧协议

| 方向 | 帧类型 | 含义 |
|------|--------|------|
| 客户端 → 服务端 | `register` | WS 建立后首帧，声明 channel + account_id + conversation_id |
| 客户端 → 服务端 | `inbound_message` | 客户消息上行（触发 AI） |
| 客户端 → 服务端 | `history` | 历史消息上行（仅落库，不触发 AI） |
| 客户端 → 服务端 | `pong` | 心跳保活 |
| 服务端 → 客户端 | `outbound_reply` | AI 回复下发 |
| 服务端 → 客户端 | `pong` / `ack` / `error` | 保活/确认/错误 |

详见 `user-server/internal/bridge/frames.go` 和 `user-web/bridge/src/core/types.js`。

## 2. 内容巡检（DOM 轮询）：扩展侧必须做的事

虽然传输层是 WS 长连接，**内容捕获层** 仍然需要轮询/监听 IM 网页 DOM：

- 抖音/小红书/TikTok/闲鱼的 IM 网页本质是 React/Vue SPA，私信渲染在 `<div id="root">` 内
- 不能用 HTTP 轮询网页内容（跨域 / 反爬）
- 必须用 MutationObserver + 定时兜底扫描监听 DOM 变化

### 2.1 现有内容巡检（patrol）机制

| 阶段 | 做什么 | 频率 |
|------|--------|------|
| 实时监听 | MutationObserver 监听消息线程容器 | 即时（childList + subtree） |
| 兜底扫描 | 3s setInterval 全量扫一遍线程 | 3s |
| 巡检 | 遍历左侧会话列表 → 点开未读会话 → 抓新消息 | 60s 一轮 |

### 2.2 2026-08-05 巡检机制优化

**问题**：旧版"列表遍历 + 即时点击"在虚拟列表（virtualized list）下会触发大量滚动加载 + 全部点开 → 几百个会话全部 visit、几十次 openConversation 抢占主线程，Chrome 内存 / CPU 飙升（OOM 风险）。

**新策略**（两阶段扫描）：

```
阶段 1（轻量）：getConversationList() 一次 → 收集 unread id（不点开、不滚动加载）
   ↓
阶段 2（限并发）：仅对 unread 会话逐个点开 → 抓新消息
   ├─ 并发上限 MAX_VISIT_PARALLEL=2
   ├─ 用户已停留的会话自动跳过（依赖 MutationObserver）
   └─ 单次抓取上限 80 条（防 OOM）
```

**Scan-Only 模式**（popup 可启用）：仅跑阶段 1，返回 `{scannedTotal, unreadCount}`，
不进入任何会话。用于"用户正在浏览时"实时探测未读变化，让 MutationObserver 自然捕获。

## 3. OOM 巡检整改（2026-08-05）

| 风险点 | 修复 |
|--------|------|
| `MutationObserver.observe(root, { characterData: true, subtree: true })` 输入框抖屏产生成百上千次 textNode 字符 mutation | 移除 `characterData: true`（捕获气泡走 childList 已足够） |
| 100ms 内 mutation 全部进 `_onMutations` 同步处理 | 100ms 节流批处理 + 单次最多 50 个新增元素 |
| `seen` Set 5000 满时清理一半（Array.from + slice） | 改用 Set 迭代器软清理 1/4（保更多去重证据，主线程抖动小） |
| `_collectUnseenText` 单次抓无上限（一个超长会话一次抓几千条） | 单次封顶 80 条 |
| 巡检阶段2无并发控制（串行点几百个会话） | 信号量限并发 = 2 |

## 4. 渠道编码统一（2026-08-05）

详见 `migrations/037_bridge_channel_unify_v2.sql` 和 `internal/migration/migrations/bridge_channel_unify_v2_migration.go`。

前端常量、后端常量、SQL 数据、Go 数据 全部统一为**全名（无 `_web` 后缀）**：

| 旧值 | 新值 |
|------|------|
| `xhs` / `xhs_web` / `xiaohongshu` | `xiaohongshu` |
| `douyin` / `douyin_web` | `douyin` |
| `kuaishou` / `kuaishou_web` | `kuaishou` |
| `xianyu` / `xianyu_web` | `xianyu` |
| `tiktok` / `tiktok_web` | `tiktok` |
