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
//   - 默认白名单：当前 host + 私有部署（空 Host）
//   - 通过 BRIDGE_ALLOWED_ORIGINS 环境变量补充允许的 Origin（逗号分隔）
//   - 全部不匹配时拒绝升级（403）
func bridgeCheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// 同源或非浏览器请求（如原生扩展 background）放行
		return true
	}
	host := r.Host
	allowed := map[string]bool{}
	if host != "" {
		allowed["http://"+host] = true
		allowed["https://"+host] = true
	}
	if env := os.Getenv("BRIDGE_ALLOWED_ORIGINS"); env != "" {
		for _, o := range splitAndTrim(env, ",") {
			if o != "" {
				allowed[o] = true
			}
		}
	}
	return allowed[origin]
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
		// 保活 / 下行确认，无需处理
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
//   - 路由层 JWTAuthMiddleware 写入 user_id 到 gin context
//   - 本方法在升级前再校验 (channel, account_id) 是否属于当前 user
// 不通过返回 403（修复 -1：水平越权）
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
	// 1 账号归属校验：通过回调注入（避免 bridge → service 反向依赖）
	if GlobalOwnershipChecker != nil {
		uidAny, ok := c.Get("user_id")
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user_id missing from context"})
			return
		}
		uid, ok := uidAny.(uint)
		if !ok || uid == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user_id"})
			return
		}
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
		Timestamp:      ts,
		Extra:          map[string]any{"account_id": m.AccountID, "bridge": true, "sender_type": m.SenderType},
	}
	return ev
}

// ===== unused imports guard =====
var _ sync.Mutex
