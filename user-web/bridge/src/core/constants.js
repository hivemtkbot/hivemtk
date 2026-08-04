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
// 用途：popup 「打开抖音/小红书/TikTok 私信」按钮的跳转目标
export const PLATFORM_ENTRY_URLS = {
  douyin_web: 'https://www.douyin.com/chat',
  xhs_web: 'https://www.xiaohongshu.com/chat',
  tiktok_web: 'https://www.tiktok.com/messages',
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
  // 会话间切换节流（避免被风控判定为机器人）
  throttleMs: 1500,
  // 单轮最多访问多少个会话（0 = 不限，按需设上限防止超长轮）
  maxPerRound: 0,
  // 列表底部滚动加载更多的等待时间
  scrollLoadMs: 500,
  // 多轮滚动扫描最多多少 pass（虚拟/懒加载列表逐步加载）
  maxPasses: 8,
});

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

// 渠道展示名 → 统一只显示「抖音 / 小红书 / TikTok」，不出现「抖音私信(网页)」这类冗长写法
// （需求④：只有一个渠道名称、来源平台编码只有 抖音/小红书，列表渲染/搜索同理）
export const CHANNEL_DISPLAY = Object.freeze({
  douyin_web: '抖音',
  xhs_web: '小红书',
  tiktok_web: 'TikTok',
});

// =============================================================
// 7) 协议常量（与服务端 user-server/internal/bridge/frames.go 一一对应）
// =============================================================
// 文档源：bridge.md §3 / frames.go Frame* 常量
// 警告：frame 名称是协议契约，禁改字面量
export const PROTOCOL = Object.freeze({
  CHANNELS: { DOUYIN: 'douyin_web', XHS: 'xhs_web', TIKTOK: 'tiktok_web' },
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
  SENDER: { CUSTOMER: 'customer', AGENT: 'agent', SELF: 'self' },
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
