package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// OwnershipChecker 账号归属校验回调：
//   - 输入：uid (JWT 解析的 user_id), channel, accountID
//   - 输出：true 表示当前 user 拥有该 bridge 账号
//   - 由 service 层在初始化时注入（避免 bridge → service 的反向依赖）
type OwnershipChecker func(ctx context.Context, uid uint, channel, accountID string) (bool, error)

// GlobalOwnershipChecker 全局归属校验（service 层在 init 时注入）
var GlobalOwnershipChecker OwnershipChecker

// RegisterOwnershipChecker 注册账号归属校验回调
func RegisterOwnershipChecker(fn OwnershipChecker) {
	GlobalOwnershipChecker = fn
}

// =============================================================
// 桥接默认参数（所有值必须能从文档溯源，禁"软启动"）
//
// 文档源：
//   - user-web/bridge/src/core/constants.js  (前端对应常量)
//   - docs/bridge/DEFAULTS.md                (用户公开清单)
//   - user-server/docs/dev/DEVELOPMENT.md    (端口对照表)
//
// 调整任意值需同步更新：
//   1) 此处常量
//   2) 前端 constants.js
//   3) DEFAULTS.md
//   4) 相关测试
// =============================================================
const (
	// writeWait 单次 WS 写操作超时（避免慢客户端长时间占用 conn）
	// 文档源：基于 gorilla/websocket 官方推荐 + 历史线上事故复盘
	writeWait = 10 * time.Second
	// pongWait 读空闲超时（连续 pongWait 无任何帧 → 主动断开）
	// 客户端心跳 25s（constants.WS_CLIENT_DEFAULTS.serverIdleTimeoutMs） < 此值，留 35s 缓冲
	pongWait = 60 * time.Second
	// pingPeriod 服务端主动 ping 周期（小于 pongWait）
	// 留 10s 缓冲给网络延迟 + 客户端响应
	pingPeriod = (60 * time.Second) - (10 * time.Second)
	// maxMessageSize 单帧最大字节数（1MB），超过则断开连接
	maxMessageSize = 1 << 20
	// maxReplyContentBytes 单条 AI 回复最大字节数（防止 XSS payload 巨大 + 平台限制）
	// 与前端 constants.SECURITY.maxReplyContentBytes 严格对齐
	maxReplyContentBytes = 4 * 1024
	// readBufferSize / writeBufferSize WS 缓冲区大小
	// 文档源：gorilla/websocket 默认值，适配普通 JSON 帧
	readBufferSize  = 1024
	writeBufferSize = 1024
	// sendBufferSize 客户端 send 通道容量（满后 Deliver 返回 ErrBridgeBufferFull）
	// 文档源：基于经验值；过小容易误判离线，过大浪费内存
	sendBufferSize = 256
)

var upgraderBridge = websocket.Upgrader{
	ReadBufferSize:  readBufferSize,
	WriteBufferSize: writeBufferSize,
	CheckOrigin:     bridgeCheckOrigin,
}

// bridgeCheckOrigin WS 升级 Origin 校验：
//   - 桥接 WS 为公开端点（仅过 InitGuard，无 JWT），身份由 channel+account_id 自证，
//     不依赖浏览器同源策略。扩展从抖音/小红书/TikTok/快手等第三方站点注入，
//     Origin 为各平台域名，无法预置白名单；且 Origin 头可被任意伪造，校验无实际安全收益。
//   - 故默认放行所有 Origin；如需收紧，可设 BRIDGE_ALLOWED_ORIGINS=origin1,origin2 仅放行列表内。
func bridgeCheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// 同源或非浏览器请求（如原生扩展 background）放行
		return true
	}
	cfg := os.Getenv("BRIDGE_ALLOWED_ORIGINS")
	if cfg == "" || cfg == "*" {
		return true
	}
	for _, o := range splitAndTrim(cfg, ",") {
		if o != "" && o == origin {
			return true
		}
	}
	return false
}

func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			parts = append(parts, trimSpaces(s[start:i]))
			start = i + 1
		}
	}
	parts = append(parts, trimSpaces(s[start:]))
	return parts
}

func trimSpaces(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// bridgeTestAutoReply 启动时一次性读取（避免每请求读 env）
var bridgeTestAutoReply = os.Getenv("BRIDGE_TEST_AUTOREPLY") == "true"

// WarnOnTestMode 启动时调用：BRIDGE_TEST_AUTOREPLY=true 时输出强告警，提示生产风险
func WarnOnTestMode() {
	if bridgeTestAutoReply {
		logger.Ctx(context.Background()).Warn().
			Str("module", "bridge").
			Msg("BRIDGE_TEST_AUTOREPLY is enabled — bridge will send [BRIDGE-E2E] auto replies. DO NOT use in production!")
	}
}

// BridgeClient 单条扩展连接
//
// 关闭语义：
//   - 正常关闭流程：Unregister 标记 closed → readPump defer close(send) → writePump 收到 !ok 退出
//   - 异常关闭：Kick() 仅关闭底层 conn，writePump 写失败后退出
//   - closed 用 sync/atomic 标记，IsOnline/Deliver 据此跳过已关闭连接（防 close-after-send panic）
type BridgeClient struct {
	hub     *BridgeHub
	channel string
	account string
	userID  uint
	conn    *websocket.Conn
	send    chan []byte
	closed  atomic.Bool
}

// CloseSend 安全关闭 send channel（幂等）；writePump 收到 !ok 后退出
func (c *BridgeClient) CloseSend() {
	if c.closed.CompareAndSwap(false, true) {
		close(c.send)
	}
}

// IsClosed 判断连接是否已关闭
func (c *BridgeClient) IsClosed() bool {
	return c.closed.Load()
}

// Kick 踢除旧连接（关闭连接触发其 readPump 退出）。conn 为 nil 时安全 no-op（用于测试 client）。
func (c *BridgeClient) Kick() {
	if c.conn == nil {
		return
	}
	_ = c.conn.Close()
}

// sendTestAutoReply 测试模式专用：模拟 AI 回复经 hub 投递回扩展。
// 触发条件：BRIDGE_TEST_AUTOREPLY=true（生产禁用，启动时 WARN 提示）。
// 用于端到端验证：bridge WS 下行（outbound_reply 帧）→ 扩展 sendOutbound → 网页回写。
func (c *BridgeClient) sendTestAutoReply(ctx context.Context, event *model.MessageEvent) {
	time.Sleep(800 * time.Millisecond) // 模拟 AI 推理耗时
	reply := &UnifiedReply{
		Channel:        c.channel,
		AccountID:      c.account,
		ConversationID: event.ConversationID,
		Content:        fmt.Sprintf("[BRIDGE-E2E] 收到您的消息: %s — 当前为测试模式，AI 引擎已记录并已回执。", event.Content),
		MsgType:        "text",
		ReplyToEventID: event.EventID,
	}
	if err := c.hub.Deliver(c.channel, c.account, reply); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").Str("event_id", event.EventID).Msg("sendTestAutoReply hub.Deliver failed")
	} else {
		logger.Ctx(ctx).Info().Str("module", "bridge").Str("event_id", event.EventID).Str("channel", c.channel).Str("account_id", c.account).Msg("sendTestAutoReply delivered")
	}
}

func (c *BridgeClient) readPump(ctx context.Context, ingress *service.InboxIngressService) {
	defer func() {
		// 私域部署: 已移除 Prometheus 指标, 关闭事件仅记录日志
		c.hub.Unregister(c) // Unregister 内部已 CloseSend（c.closed = true）
		// 连接断开：账号置离线 + 记录最后同步时间（G12）
		if GlobalBridgeAccountRepo != nil {
			if err := GlobalBridgeAccountRepo.SetOffline(c.channel, c.account); err != nil {
				logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").
					Str("channel", c.channel).Str("account_id", c.account).Msg("bridge SetOffline failed")
			}
		}
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if !errors.Is(err, websocket.ErrCloseSent) {
				logger.Ctx(ctx).Debug().Err(err).Str("module", "bridge").Str("channel", c.channel).Str("account_id", c.account).Msg("bridge readPump closed")
			}
			return
		}
		c.handleFrame(ctx, data, ingress)
	}
}

func (c *BridgeClient) handleFrame(ctx context.Context, data []byte, ingress *service.InboxIngressService) {
	var f Frame
	if err := json.Unmarshal(data, &f); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").Str("raw", truncateForLog(string(data))).Msg("bridge handleFrame json unmarshal failed")
		return
	}
	logger.Ctx(ctx).Info().Str("module", "bridge").Str("type", f.Type).Str("channel", f.Channel).Str("account_id", f.AccountID).Str("event_id", f.EventID()).Msg("bridge frame received")
	// 私域部署: 已移除 Prometheus 指标, 帧接收事件仅记录日志
	switch f.Type {
	case FrameRegister:
		if f.Channel != "" {
			c.channel = f.Channel
		}
		if f.AccountID != "" {
			c.account = f.AccountID
		}
		if old := c.hub.Register(c); old != nil && old != c {
			old.Kick()
		}
		// G6 + G12：注册即落库（绑定智能体 + 写入 channel_agent_bindings + 置在线）
		if GlobalBridgeAccountRepo != nil {
			up := BridgeAccountUpsert{
				UserID:      c.userID,
				Channel:     c.channel,
				AccountID:   c.account,
				AgentID:     0,
				AccountName: "",
				Status:      "online",
			}
			if f.Message != nil {
				up.AgentID = f.Message.AgentID
				up.AccountName = f.Message.AccountName
			}
			upErr := GlobalBridgeAccountRepo.Upsert(context.Background(), up)
			if upErr != nil {
				if errors.Is(upErr, ErrAccountOwnedByOther) {
					// 归属冲突：连接已建立（hub 注册在上方完成），仅记审计日志，不阻断收发。
					logger.Ctx(ctx).Warn().Str("module", "bridge").
						Uint("user_id", c.userID).Str("channel", c.channel).
						Str("account_id", c.account).
						Msg("bridge register: account owned by another user, ownership unchanged")
				} else {
					logger.Ctx(ctx).Error().Err(upErr).Str("module", "bridge").
						Str("channel", c.channel).Str("account_id", c.account).
						Msg("bridge register upsert failed")
				}
			}
		}
	case FrameInbound:
		if f.Message == nil {
			return
		}
		event := toMessageEvent(f.Message)
		logger.Ctx(ctx).Info().Str("module", "bridge").Str("event_id", event.EventID).Str("session", event.SessionID).Msg("bridge FrameInbound handle")
		// 中台已做幂等/锁处理；此处仅记录，不影响连接
		res, err := ingress.HandleIngressMessage(ctx, event)
		// 降级为 Debug：res 可能含完整编排/AI 上下文大报文，生产用 Info 会刷屏
		logger.Ctx(ctx).Debug().Str("module", "bridge").Interface("res", res).Err(err).Msg("bridge FrameInbound result")

		// 测试模式：当未配置 LLM 时，编排器会转人工不出站。
		// 此处用 BRIDGE_TEST_AUTOREPLY=true 注入一条占位 AI 回复到 hub，
		// 用于端到端验证 WebSocket 下行链路（仅测试，生产应保持默认 false）。
		if bridgeTestAutoReply {
			go c.sendTestAutoReply(ctx, event)
		}
	case FrameHistory:
		if f.Message == nil {
			return
		}
		// 历史/回填消息：仅落库，绝不触发 AI（避免回填空历史误触发推理与自回环）
		// 点3：会话级 history 帧（message.History 非空）→ 一个会话一条消息、内含多轮历史，逐条落库。
		if len(f.Message.History) > 0 {
			for _, it := range f.Message.History {
				if it == nil {
					continue
				}
				ev := historyItemToEvent(f.Message, it)
				dir := it.Direction
				if dir == "" {
					dir = "inbound"
				}
				if err := ingress.PersistBridgeHistory(ctx, ev, dir); err != nil {
					logger.Ctx(ctx).Error().
						Err(err).
						Str("module", "bridge").
						Str("event_id", it.EventID).
						Str("channel", ev.Channel).
						Str("direction", dir).
						Msg("bridge history persist failed (history item)")
				}
			}
			return
		}
		// 兼容单条 history 帧（旧协议）
		event := toMessageEvent(f.Message)
		direction := f.Message.Direction
		if direction == "" {
			direction = "inbound"
		}
		if err := ingress.PersistBridgeHistory(ctx, event, direction); err != nil {
			logger.Ctx(ctx).Error().
				Err(err).
				Str("module", "bridge").
				Str("event_id", event.EventID).
				Str("channel", event.Channel).
				Str("direction", direction).
				Msg("bridge history persist failed")
		}
	case FramePong, FrameAck:
		// 保活 / 下行确认。
		// 关键修复：客户端 bridge-client.js 的 alive 标志只在收到「下行 JSON 帧」
		// 时重置（协议级 ping 由浏览器自动应答，不触发 JS onmessage）。若服务端不回
		// JSON pong，客户端发送后 alive 恒为 false → 每 ~serverIdleTimeoutMs(25s)
		// 误判离线强制重连（50s 一个循环）。这里回一帧 JSON pong 打破该抖动。
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait)) // 任意上行帧均续期（防御纵深）
		c.replyPong()
	}
}

// replyPong 向客户端回一帧 JSON pong（保活闭环）。
// 与 Deliver 一致：持有 hub.mu 再写 send，避免并发 Unregister close(send) 造成
// send-on-closed-channel panic；非阻塞发送，缓冲满时丢弃（保活帧可容忍）。
func (c *BridgeClient) replyPong() {
	if c == nil || c.hub == nil {
		return
	}
	data, err := json.Marshal(&Frame{Type: FramePong})
	if err != nil {
		return
	}
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	if c.IsClosed() {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

// truncateForLog 日志截断：避免大 content 撑爆日志
func truncateForLog(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func (c *BridgeClient) writePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	// 6 writePump defer Unregister：写失败时确保从 hub 摘除，避免僵尸连接
	defer func() {
		ticker.Stop()
		c.hub.Unregister(c)
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				logger.Ctx(ctx).Debug().Err(err).Str("module", "bridge").Msg("bridge writePump write failed")
				return
			}
			// 私域部署: 已移除 Prometheus 指标, 出站帧事件仅记录日志
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logger.Ctx(ctx).Debug().Err(err).Str("module", "bridge").Msg("bridge writePump ping failed")
				return
			}
		}
	}
}

// Start 启动读写泵（ctx 用于日志透传 trace_id）
func (c *BridgeClient) Start(ctx context.Context, ingress *service.InboxIngressService) {
	go c.writePump(ctx)
	c.readPump(ctx, ingress)
}

// BridgeWSHandler 桥接 WebSocket 处理器
type BridgeWSHandler struct {
	hub     *BridgeHub
	ingress *service.InboxIngressService
}

// NewBridgeWSHandler 构造处理器（账号归属校验通过 GlobalOwnershipChecker 注入）
func NewBridgeWSHandler(hub *BridgeHub, ingress *service.InboxIngressService) *BridgeWSHandler {
	return &BridgeWSHandler{hub: hub, ingress: ingress}
}

// HandleWebSocket GET /api/ws/bridge?channel=douyin_web&account_id=xxx
//
// 鉴权：
//   - 桥接 WS 不要求前端 JWT：路由层仅过 InitGuard（系统须已初始化），不过 JWTAuthMiddleware。
//     账号以 channel+account_id 自证身份（私有化部署单用户场景，扩展运行在用户浏览器中）。
//   - 若请求携带有效 JWT，则再校验 (channel, account_id) 是否属于该 user（多租户安全路径）；
//     无 JWT 时归属写入 user_id=0，可正常落库。
// 归属不通过（且携带了 JWT）返回 403，无 JWT 时跳过校验。
//
// trace_id：
//   - 优先从请求头 X-Trace-Id 读取（前端/扩展可显式携带）
//   - 否则从 gin context 读取（由 GinTraceMiddleware 写入）
//   - 否则由 WithTraceID 自动生成（保证链路必有追踪标识）
func (h *BridgeWSHandler) HandleWebSocket(c *gin.Context) {
	channel := c.Query("channel")
	accountID := c.Query("account_id")
	if channel == "" || accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "channel and account_id required"})
		return
	}
	// 渠道白名单
	if !IsBridgeChannel(channel) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported bridge channel"})
		return
	}
	// 解析 JWT 中的 user_id（可选）：桥接 WS 允许无 JWT 连接——账号以 channel+account_id
	// 自证身份（私有化部署单用户场景）。仅当携带有效 JWT 时才做账号归属校验；
	// 无 JWT 时归属写入 user_id=0（"无归属/全体"语义，schema 默认值，可正常落库）。
	uidAny, _ := c.Get("user_id")
	uid, _ := uidAny.(uint)
	// 1 账号归属校验：仅当携带有效 JWT 时执行（回调注入，避免 bridge → service 反向依赖）
	if GlobalOwnershipChecker != nil && uid != 0 {
		owns, err := GlobalOwnershipChecker(c.Request.Context(), uid, channel, accountID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ownership check failed: " + err.Error()})
			return
		}
		if !owns {
			c.JSON(http.StatusForbidden, gin.H{"error": "account not owned by current user"})
			return
		}
	}

	// 2 trace_id 透传：优先 X-Trace-Id 头 > gin context > 自动生成
	traceID := c.GetHeader("X-Trace-Id")
	if traceID == "" {
		if v, ok := c.Get("trace_id"); ok {
			traceID, _ = v.(string)
		}
	}
	ctx := c.Request.Context()
	if traceID != "" {
		ctx = logger.WithTraceID(ctx, traceID)
	} else {
		ctx = logger.WithTraceID(ctx, "")
	}
	ctx = logger.WithModule(ctx, "bridge")

	conn, err := upgraderBridge.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").Msg("bridge ws upgrade failed")
		return
	}
	client := &BridgeClient{
		hub:     h.hub,
		channel: channel,
		account: accountID,
		userID:  uid,
		conn:    conn,
		send:    make(chan []byte, sendBufferSize),
	}
	if old := h.hub.Register(client); old != nil && old != client {
		old.Kick()
	}
	_ = channel // 私域: 无 Prometheus, 审计日志已落库
	logger.Ctx(ctx).Info().Str("module", "bridge").Str("channel", channel).Str("account_id", accountID).Msg("bridge ws connected")
	go client.Start(ctx, h.ingress)
}

// historyItemToEvent 把会话级 history 帧中的单轮（HistoryItem）映射为 model.MessageEvent。
// 会话元数据（channel/account/conversation/群）取自帧顶层的 message，轮次字段取自 item。
func historyItemToEvent(m *UnifiedMessage, it *HistoryItem) *model.MessageEvent {
	ch := ToBridgeChannel(m.Channel)
	ts := time.UnixMilli(it.Timestamp)
	if it.Timestamp == 0 {
		ts = time.Now()
	}
	ev := &model.MessageEvent{
		EventID:        it.EventID,
		SessionID:      ch + ":" + m.AccountID + ":" + m.ConversationID,
		Channel:        ch,
		SenderID:       it.SenderID,
		SenderName:     it.SenderName,
		ReceiverID:     orDefault(it.ReceiverID, m.ReceiverID),
		MsgType:        it.MsgType,
		Content:        it.Content,
		MediaURL:       it.MediaURL,
		ConversationID: m.ConversationID,
		IsGroup:        it.IsGroup || m.IsGroup,
		GroupID:        orDefault(it.GroupID, m.GroupID),
		Timestamp:      ts,
		Extra: map[string]any{
			"account_id": m.AccountID, "bridge": true,
			"sender_type": orDefault(it.SenderType, m.SenderType),
		},
	}
	// 出站轮次 receiver_id 兜底：扩展侧 _historyItem 已填「会话对方」，此处再兜一层
	// （旧版扩展未填时，统一收信中心仍能按「对方」聚合而非「自己」）。
	if ev.ReceiverID == "" && it.Direction == "outbound" {
		ev.ReceiverID = m.ConversationID
	}
	if ev.IsGroup {
		ev.Extra["is_group"] = true
	}
	if ev.GroupID != "" {
		ev.Extra["group_id"] = ev.GroupID
	}
	if groupName := orDefault(it.GroupName, m.GroupName); groupName != "" {
		ev.Extra["group_name"] = groupName
	}
	return ev
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// toMessageEvent 将 UnifiedMessage 映射为 model.MessageEvent
func toMessageEvent(m *UnifiedMessage) *model.MessageEvent {
	ch := ToBridgeChannel(m.Channel)
	ts := time.UnixMilli(m.Timestamp)
	if m.Timestamp == 0 {
		ts = time.Now()
	}
	ev := &model.MessageEvent{
		EventID:        m.EventID,
		SessionID:      ch + ":" + m.AccountID + ":" + m.ConversationID,
		Channel:        ch,
		SenderID:       m.SenderID,
		SenderName:     m.SenderName,
		ReceiverID:     m.ReceiverID,
		MsgType:        m.MsgType,
		Content:        m.Content,
		MediaURL:       m.MediaURL,
		ConversationID: m.ConversationID,
		IsGroup:        m.IsGroup,
		GroupID:        m.GroupID,
		Timestamp:      ts,
		Extra:          map[string]any{"account_id": m.AccountID, "bridge": true, "sender_type": m.SenderType},
	}
	// 会话级多轮历史透传（点3）：扩展 inbound 帧携带的 history 窗口 → MessageEvent，
	// 落 message_hub.Extra 供统一收件箱展示/可观测（AI 上下文由 session_messages 自行重建）。
	if len(m.History) > 0 {
		hist := make([]model.MessageEventHistoryItem, 0, len(m.History))
		for _, it := range m.History {
			if it == nil {
				continue
			}
			hist = append(hist, model.MessageEventHistoryItem{
				EventID:    it.EventID,
				SenderType: it.SenderType,
				SenderID:   it.SenderID,
				SenderName: it.SenderName,
				ReceiverID: it.ReceiverID,
				MsgType:    it.MsgType,
				Content:    it.Content,
				MediaURL:   it.MediaURL,
				Timestamp:  it.Timestamp,
				Direction:  it.Direction,
				IsGroup:    it.IsGroup,
				GroupID:    it.GroupID,
				GroupName:  it.GroupName,
			})
		}
		ev.History = hist
		ev.Extra["history"] = hist
	}
	// 群聊 / 非文字元信息冗余进 Extra，保证下游（inbox_ingress / 统一收件箱）可读
	if m.IsGroup {
		ev.Extra["is_group"] = true
	}
	if m.GroupID != "" {
		ev.Extra["group_id"] = m.GroupID
	}
	if m.GroupName != "" {
		ev.Extra["group_name"] = m.GroupName
	}
	return ev
}

// ===== unused imports guard =====
var _ sync.Mutex
