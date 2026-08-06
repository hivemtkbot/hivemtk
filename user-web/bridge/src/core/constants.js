// bridge 默认值单一源（Single Source of Truth）
//
// 设计原则（项目硬约束）：
//   1) 不允许"软启动"——任何默认值都必须能从文档溯源
//   2) 不允许在多个位置重复硬编码同一个值
//   3) 调用方必须从本文件导入，禁止就地写死数字/URL/端口
//   4) 新增/修改默认值必须同步更新 DEFAULTS.md 与 config 契约
//
// 文档源：
//   - user-server/docs/dev/DEVELOPMENT.md 端口对照表 / 应用启动清单
//   - user-server/cmd/api/main.go           listenAddr / Gin 端口
//   - user-server/Dockerfile                ENV SERVER_PORT=8204
//   - user-server/config.yaml               inference.llm/embedding/rerank.base_url
//   - docs/bridge/DEFAULTS.md               桥接自身默认值清单（向用户公开）
//   - bridge.md §17.3                       限速/风控设计

// =============================================================
// 1) 服务端默认地址（user-server 连接入口）
// =============================================================
// 文档源：DEVELOPMENT.md §端口对照表
//   | 8204 | user-server (Gin HTTP)            |
//   | 8202 | PostgreSQL（docker 宿主机映射）   |
//   | 8207 | llama.cpp (LLM)                   |
//   | 8208 | embedding (bge-m3)                |
//   | 8209 | rerank (bge-reranker-v2-m3)       |
//   | 8232 | PostgreSQL（dev 本地直连）        |
// 交叉验证：user-server/Dockerfile:57  ENV SERVER_PORT=8204
// 交叉验证：user-server/cmd/api/main.go listenAddr 默认 :8204
export const DEFAULT_USER_SERVER = {
  host: 'localhost',
  port: 8204,
  baseUrl: 'http://localhost:8204',
  // 健康检查路径（user-server/router.go 实际注册顺序）
  // 优先 /health（含依赖检查），降级到存活探针
  healthPaths: ['/health', '/healthz', '/readyz', '/api/health'],
  // WS 端点（service_routes.go:90  auth.GET("/ws/bridge", ...)）
  wsPath: '/api/ws/bridge',
  // 启动模式：dev / staging / prod（仅用于 UI 提示，不参与运行时逻辑）
  profile: 'dev',
};

// =============================================================
// 2) 平台入口（仅用于 popup 一键打开的快捷入口）
// =============================================================
// 文档源：用户指定的真实私信/聊天页入口（需求①）
//   - 抖音聊天页：https://www.douyin.com/chat
//   - 小红书聊天页：https://www.xiaohongshu.com/chat
//   - TikTok 私信：https://www.tiktok.com/messages（官网消息入口）
//   - 闲鱼聊天页：https://www.goofish.com/im（闲鱼 IM 入口）
// 用途：popup 「打开抖音/小红书/TikTok/闲鱼 私信」按钮的跳转目标
// 2026-08-05 渠道编码统一：key 改为平台全名（与后端 model.Channel* 对齐）。
export const PLATFORM_ENTRY_URLS = {
  douyin: 'https://www.douyin.com/chat',
  xiaohongshu: 'https://www.xiaohongshu.com/chat',
  tiktok: 'https://www.tiktok.com/messages',
  xianyu: 'https://www.goofish.com/im',
  kuaishou: 'https://www.kuaishou.com/new-reco',
};

// =============================================================
// 3) 限速/风控默认值（详见 bridge.md §17.3 三层风控）
// =============================================================
// 约束：所有数字都基于业务经验值，禁止"软启动"空值
// 调整建议：见 docs/bridge/DEFAULTS.md
export const RATE_LIMIT_DEFAULTS = Object.freeze({
  // —— 单账号令牌桶（每分钟）——
  // 经验：抖音 IM 网页端 12 条/分钟是平台风控安全阈值上限
  accountCapacity: 12,
  accountRefillPerMin: 12,
  // —— 拟人节奏（毫秒）——
  // 任意两次下行之间最小间隔 1.5s，避免被识别为机器人
  minIntervalMs: 1500,
  // 发送前随机抖动区间：800ms ~ 2600ms（模拟真人打字停顿）
  jitterMinMs: 800,
  jitterMaxMs: 2600,
  // —— 单会话（每条对话）——
  // 同一会话两次回复之间冷却 3s，防回环/刷屏
  conversationCooldownMs: 3000,
  // 同一会话每小时最多 40 条回复
  conversationPerHour: 40,
  // —— 去重（防 AI 重复回复相同内容）——
  // 同一会话 60s 内相同文案不重复发送
  dedupWindowMs: 60000,
});

// =============================================================
// 4) WS 客户端默认值（bridge-client.js）
// =============================================================
// 心跳超时 25s（与 server handler.go pongWait=60s 错开 35s 以上）
// 文档源：user-server/internal/bridge/handler.go
//   const pongWait   = 60 * time.Second
//   const pingPeriod = 50 * time.Second
// 客户端 25s 超时 < 服务端 60s，避免出现客户端先断导致连接泄漏
export const WS_CLIENT_DEFAULTS = Object.freeze({
  // 25s 内无任何服务端帧（ping/inbound/ack）即视为失联，主动断开重连
  serverIdleTimeoutMs: 25 * 1000,
  // 指数退避：1s -> 2s -> 4s -> 8s -> 16s，封顶 30s
  reconnectBaseMs: 1000,
  reconnectMaxMs: 30 * 1000,
  // jitter 抖动 < 500ms，避免雪崩重连
  reconnectJitterMs: 500,
});

// =============================================================
// 5) 巡检制度（patrol）默认值（详见 bridge.md 上行巡检）
// =============================================================
// 巡检语义：一轮巡检完成 → 自动进入下一轮。遍历左侧聊天列表，对有新消息
// （未读红点）的会话点击进入右侧聊天页，捕获新消息上行（触发 AI 自动对话）。
// 已 seen 的消息靠去重跳过，故只有真正新增的消息会上行。
export const PATROL_DEFAULTS = Object.freeze({
  // 巡检轮间隔：完成一轮后等待多久再开始下一轮（默认 60s）
  intervalMs: 60 * 1000,
  // 单个会话点击打开后等待线程渲染时长
  waitActiveMs: 5000,
  // 会话间切换节流：1.5 秒一个会话（用户诉求：遍历会话列表不允许太快，两个会话之间间隔 1-2 秒）
  //   - 太快（<1000ms）易触发平台风控（判定为机器人）+ 用户诉求明确要求 1-2s
  //   - 太慢（>2000ms）巡检延迟高，新消息捕获不及时
  //   - 1500ms 平衡点：落在 1-2s 区间中值，平台可接受 + 新消息近实时捕获
  throttleMs: 1500,
  // 单轮最多访问多少个会话（0 = 不限，按需设上限防止超长轮）
  maxPerRound: 0,
  // 两轮列表扫描之间的间隔（滚动到底加载更多后等下一轮）：用户诉求 1-2 秒
  //   - 旧值 500ms 太快，虚拟列表还没加载完就开下一轮，漏抓 + 风控
  //   - 1500ms 落在 1-2s 区间中值，给 SPA 足够渲染时间
  scrollLoadMs: 1500,
  // 多轮滚动扫描最多多少 pass（虚拟/懒加载列表逐步加载）
  maxPasses: 8,
});

// =============================================================
// 5b) 历史回填宽限期（per-channel，2026-08-05 审计 P0/A6）
// =============================================================
// 语义：会话初次挂载/切换后的一段时间内，新出现的客户消息仅回填历史（落库），
//   不触发 AI 自动回复。避免打开含存量私信的会话时被当成新消息逐一自动回复。
//
// per-channel 依据（实测不同平台 SPA 渲染延迟差异极大）：
//   - douyin：5s（聊天页结构稳定，DOM 渲染快）
//   - xiaohongshu：2s（XHS IM 是 React 受控组件，渲染最快）
//   - tiktok：6s（海外链路 + 重 SPA，渲染最慢）
//   - xianyu：4s（闲鱼 IM 入口 goofish.com，DOM 中等复杂度）
//   - kuaishou：5s（未实现，预留）
//
// 默认值（BaseAdapter 未注入 hooks.historyGraceMs 时使用）：8s（保守上限）
export const HISTORY_GRACE_MS = Object.freeze({
  douyin: 5000,
  xiaohongshu: 2000,
  tiktok: 6000,
  xianyu: 4000,
  kuaishou: 5000,
  default: 8000,
});

// getHistoryGraceMs 按 channel 取 per-channel 宽限期；未知 channel 用 default。
export function getHistoryGraceMs(channel) {
  if (!channel) return HISTORY_GRACE_MS.default;
  return HISTORY_GRACE_MS[channel] || HISTORY_GRACE_MS.default;
}

// =============================================================
// 6) UI / 测试用超时
// =============================================================
export const UI_DEFAULTS = Object.freeze({
  // popup「测试连接」单次 fetch 超时 3.5s
  healthCheckTimeoutMs: 3500,
  // 报告当前 meta（accountId/conversationId）周期
  metaReportIntervalMs: 5000,
  // 状态轮询：popup 打开时拉一次，无轮询
});

// 渠道展示名 → 统一只显示「抖音 / 小红书 / TikTok / 闲鱼」，不出现「抖音私信(网页)」这类冗长写法
// （需求④：只有一个渠道名称、来源平台编码只有 抖音/小红书/闲鱼/TikTok，列表渲染/搜索同理）
//
// 2026-08-05 渠道编码统一：key 与 value 解耦——key 是后端约定的渠道编码（与 user-server
// model.ChannelXHS/Douyin/Kuaishou/Xianyu/TikTok 完全一致，全名无 _web 后缀），
// value 是 UI 展示文案（中文/品牌名）。
export const CHANNEL_DISPLAY = Object.freeze({
  douyin: '抖音',
  xiaohongshu: '小红书',
  tiktok: 'TikTok',
  xianyu: '闲鱼',
  kuaishou: '快手',
});

// =============================================================
// 7) 协议常量（与服务端 user-server/internal/bridge/frames.go 一一对应）
// =============================================================
// 文档源：bridge.md §3 / frames.go Frame* 常量
// 警告：frame 名称是协议契约，禁改字面量
//
// 2026-08-05 渠道编码统一：CHANNELS 值改为平台全名（去掉 _web 后缀），与后端
// model.ChannelXHS/Douyin/Kuaishou/Xianyu/TikTok 一一对应。
export const PROTOCOL = Object.freeze({
  CHANNELS: { DOUYIN: 'douyin', XHS: 'xiaohongshu', TIKTOK: 'tiktok', XIANYU: 'xianyu', KUAISHOU: 'kuaishou' },
  FRAME: {
    REGISTER: 'register',
    INBOUND: 'inbound_message',
    HISTORY: 'history',
    OUTBOUND: 'outbound_reply',
    PONG: 'pong',
    PING: 'ping',
    ACK: 'ack',
    ERROR: 'error',
  },
  SENDER: { CUSTOMER: 'customer', AGENT: 'agent', SELF: 'self', SYSTEM: 'system' },
  DIRECTION: { INBOUND: 'inbound', OUTBOUND: 'outbound' },
});

// =============================================================
// 8) 安全护栏（防 XSS / 防滥用 / 防误用）
// =============================================================
// 服务端 maxReplyContentBytes=4*1024（handler.go:43）
// 客户端 4KB 与之对齐，避免 AI 生成超大回复被服务端截断后用户看到残缺
export const SECURITY = Object.freeze({
  maxReplyContentBytes: 4 * 1024,
  // 扩展端日志脱敏：超过 24 字符的字符串在 console 截断
  logMaskMaxChars: 24,
});

// =============================================================
// 9) 桥接下发三通道配置（2026-08-06 架构重构，要求1：参数可配置）
// =============================================================
// 三通道相互独立：
//   通道A·上报:  uplink  → POST /api/bridge/ingest（父层统一上报，消息 hash 前端完成）
//   通道B·状态:  status → POST /api/bridge/outbox/ack（确认 delivered）
//   通道C·下发:  downlink → GET /api/bridge/outbox（独立轮询拉取待发消息）
//
// 全部可在 docs/bridge/DEFAULTS.md 覆盖（运行时读取使用者覆盖值）。
export const BRIDGE_THREE_CHANNEL = Object.freeze({
  // 通道A：上报合并窗口（同 accountId|conversationId 在窗口内的多条消息合并一次 POST）
  uplinkMergeWindowMs: 350,
  uplinkMaxBatch: 20,
  // 通道C：下发轮询间隔（每次拉取待发消息并转发）
  outboxPollIntervalMs: 1500,
  // 通道C：单次拉取上限（前端 getOutbox 以 limit query 传给后端 ListPendingOutboundLimit，封顶 200）
  outboxBatchSize: 50,
  // 通道B：状态确认刷新间隔（聚合批量 ack）
  ackFlushIntervalMs: 500,
  // 通道B：本地已发缓存上限（持久化到 chrome.storage.local，防刷新/重开后重复下发）
  sentCacheMax: 2000,
  // 下发单条发送超时（超过视为失败，下个轮询重试）
  sendOutboundTimeoutMs: 20000,
  // 巡检周期（要求⑤：定时任务 3 秒一次）
  patrolIntervalMs: 3000,
  // 每个会话列表切换随机等待 1-2 秒（要求⑤）
  patrolSwitchMinMs: 1000,
  patrolSwitchMaxMs: 2000,
});

