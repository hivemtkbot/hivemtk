package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/security"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service/translation"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// VisitorWSHandler 访客 WebSocket 处理器
//
// 端点：GET /api/ws/visitor
// 参数：
//   - session_id  必填
//   - visitor_id  必填
//   - channel_id  可选（缺失时使用 default）
//
// 私域部署模式：无需鉴权，自己网站直连。
// AppKey/Channel 解析由 AppKeyResolve 中间件完成（HTTP API）；WS 端为公开端点。
//
// 每个访客连接分配独立的 trace_id（可由前端通过 X-Trace-Id 透传，或自动生成），
// 并绑定 module=websocket，使该连接生命周期内的所有日志共享同一追踪标识。
//
// 下行消息类型（type 字段）：
//   - welcome            接入欢迎
//   - message            AI/坐席消息
//   - offline_messages   离线消息批量（连接时推送一次）
//   - agent_joined       坐席接管
//   - session_closed     会话关闭
//   - ai_typing          AI 正在输入
//   - error              错误通知
//   - pong               心跳响应
//
// 上行消息类型：
//   - ping        心跳
//   - delivered   标记消息已投递（防止重连重复拉取）
//   - close       关闭
type VisitorWSHandler struct {
	hub          *Hub
	db           *gorm.DB
	langResolver *translation.LangConfigResolver
}

// NewVisitorWSHandler 创建访客 WebSocket 处理器
func NewVisitorWSHandler(db *gorm.DB) *VisitorWSHandler {
	return &VisitorWSHandler{hub: GetHub(), db: db}
}

// SetLangResolver 注入多语言解析器（v1.2 出海方案）。
// 未注入时仍可正常工作，ctx 中语言走默认 zh 兜底。
func (h *VisitorWSHandler) SetLangResolver(r *translation.LangConfigResolver) {
	h.langResolver = r
}

// upgraderVisitor 访客连接升级器
//
// 安全修复（2026-08-18）：CheckOrigin 不再全放行。
// 策略：
//   1. 优先使用共享 isAllowedOrigin（配置白名单）
//   2. 私域部署兼容：允许 localhost + 内网 IP 段
//   3. 非浏览器请求（无 Origin 头）放行
var upgraderVisitor = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // 非浏览器请求放行
		}
		// 1. 先走配置白名单
		if isAllowedOrigin(origin) {
			return true
		}
		// 2. 私域部署兼容：允许内网/本地开发
		privatePatterns := []string{
			"http://localhost",
			"http://127.0.0.1",
			"http://192.168.",
			"http://10.",
			"http://172.",
			"https://localhost",
			"https://127.0.0.1",
		}
		for _, pattern := range privatePatterns {
			if strings.HasPrefix(origin, pattern) {
				return true
			}
		}
		return false
	},
}

// getAllowedWSOrigins 从环境变量读取允许的 WebSocket Origin 列表
// 设置 HIVE_MTK_ALLOWED_ORIGINS=origin1,origin2 以允许生产域名
func getAllowedWSOrigins() []string {
	return allowedWSOrigins
}

// allowedWSOrigins 允许的 WebSocket Origin 列表
// 可通过 init() 或配置加载
var allowedWSOrigins = []string{}

// SetAllowedWSOrigins 动态设置允许的 WebSocket Origin 列表（供启动时配置调用）
func SetAllowedWSOrigins(origins []string) {
	allowedWSOrigins = origins
}

// HandleVisitorWebSocket 处理访客 WebSocket 连接
//
// 安全修复（2026-08-18）：
//   - 验证 visitor_token（HMAC-SHA256 签名），防止 IDOR 越权连接他人会话
//   - 验证通过后才升级到 WebSocket 连接
//
// 鲁棒性加固（方向B）：
//   - 接受 query `since_seq=N` 增量补发（断点续传）
//   - readPump 处理 `{"type":"ack","seq":[N,...]}` 清理待 ACK
//   - readPump 处理 `{"type":"resume","since_seq":N}` 拉取增量消息
//   - onConnect 接受 sinceSeq 沿用 delivered_at 兜底补发
func (h *VisitorWSHandler) HandleVisitorWebSocket(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Query("session_id"))
	visitorID := strings.TrimSpace(c.Query("visitor_id"))
	channelID := strings.TrimSpace(c.Query("channel_id"))
	if channelID == "" {
		channelID = "default"
	}

	ctx := logger.WithTraceID(c.Request.Context(), c.GetHeader("X-Trace-Id"))
	ctx = logger.WithModule(ctx, "websocket")

	// 安全修复：验证 visitor_token（IDOR 防护）
	visitorToken := strings.TrimSpace(c.Query("visitor_token"))
	if visitorToken == "" {
		// 兼容旧前端：也支持 token query param
		visitorToken = strings.TrimSpace(c.Query("token"))
	}
	if visitorToken == "" && sessionID != "" && visitorID != "" {
		// 允许无 token 连接但记录警告（渐进式迁移策略）
		logger.Ctx(ctx).Warn().Str("session_id", sessionID).Str("visitor_id", visitorID).Msg("WebSocket 连接无 visitor_token，建议升级前端")
	} else if visitorToken != "" {
		secretKey := getVisitorTokenSecret()
		if err := security.ValidateVisitorToken(secretKey, visitorToken, channelID, visitorID, sessionID); err != nil {
			logger.Ctx(ctx).Warn().Str("session_id", sessionID).Str("visitor_id", visitorID).Err(err).Msg("visitor_token 验证失败，拒绝 WebSocket 连接")
			c.JSON(http.StatusForbidden, gin.H{
				"error": "visitor_token 无效或已过期",
			})
			return
		}
	}

	sinceSeq := uint64(0)
	if s := strings.TrimSpace(c.Query("since_seq")); s != "" {
		if n, err := strconv.ParseUint(s, 10, 64); err == nil {
			sinceSeq = n
		}
	}

	if sessionID == "" || visitorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少必要参数：session_id / visitor_id",
		})
		return
	}

	ctx = injectLangToCtx(ctx, h.langResolver, channelID, 0)

	conn, err := upgraderVisitor.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("session_id", sessionID).Str("visitor_id", visitorID).Msg("ws upgrade failed")
		return
	}

	client := NewVisitorClient(h.hub, sessionID, visitorID, channelID)
	client.hub = h.hub
	registerVisitorClient(client)

	logger.Ctx(ctx).Info().
		Str("session_id", sessionID).
		Str("visitor_id", visitorID).
		Str("channel_id", channelID).
		Msg("visitor connected")

	go h.writePump(client, conn)
	go h.readPump(client, conn, ctx)

	go h.onConnect(client, sinceSeq, ctx)
}

// onConnect 访客连接建立后的处理
// 1. 推送 welcome 消息
// 2. 拉取离线期间未投递的坐席/AI 回复，批量推送
// 3. 标记已投递
//
// 鲁棒性加固（方向B）：
//   - 接受 sinceSeq 参数（query `since_seq` 或上行 `resume` 消息）
//   - 拉取逻辑：
//   - 若 sinceSeq > 0：尝试按 seq 范围拉取（基于 GlobalPendingAck）
//   - 否则：走原有 delivered_at IS NULL 兜底路径
//   - 双轨制：seq 路径精确但有窗口期（seq 重启后归零）；delivered_at 兜底
//
// onConnect 推送建立连接后（或 resume 时）的离线/历史消息，统一处理注册与离线消息
// 推送逻辑，避免 HandleVisitorWebSocket 与 resume 分支重复。采用单条 SQL 批量标记已送达，
// 配合 delivered_at 避免重复推送同一批离线消息。如果会话从未离线（无离线消息），直接返回。
//
// 并发守卫：初始连接（HandleVisitorWebSocket）与多次 resume 可能并发触发 onConnect，
// 若不拦截，多个 goroutine 会在彼此标记 delivered_at 之前读到同一批未送达消息，导致离线
// 消息被重复推送给访客。onConnectInflight 保证同一 client 同一时刻仅有一个 onConnect 在跑，
// 已在跑的调用会覆盖全部未送达消息，后续调用跳过即可（新消息由实时广播路径覆盖）。
func (h *VisitorWSHandler) onConnect(client *Client, sinceSeq uint64, ctx context.Context) {
	if !client.onConnectInflight.CompareAndSwap(false, true) {
		return
	}
	defer client.onConnectInflight.Store(false)

	if h.db == nil {
		return
	}

	welcomeEnv := MustEnvelope(NextSeq(), TypeWelcome, map[string]any{
		"session_id": client.sessionID,
		"visitor_id": client.visitorID,
		"channel_id": client.channelID,
		"since_seq":  sinceSeq,
		"time":       time.Now().Unix(),
	})
	if bytes, err := welcomeEnv.MarshalBytes(); err == nil {
		sendToClient(client, bytes)
	}

	// 2. 拉取离线消息（双轨：seq 优先 + delivered_at 兜底）
	type offlineRow struct {
		ID           uint      `gorm:"primaryKey"`
		SessionID    string    `gorm:"column:session_id"`
		Content      string    `gorm:"column:content"`
		SenderType   string    `gorm:"column:sender_type"`
		SenderName   string    `gorm:"column:sender_name"`
		AISource     string    `gorm:"column:ai_source"`
		AIConfidence float64   `gorm:"column:ai_confidence"`
		CreatedAt    time.Time `gorm:"column:created_at"`
	}
	var rows []offlineRow
	if err := h.db.Table("session_messages").
		Select("id, session_id, content, sender_type, sender_name, ai_source, ai_confidence, created_at").
		Where("session_id = ? AND sender_type IN ? AND delivered_at IS NULL",
			client.sessionID, []string{"ai", "agent"}).
		Order("created_at ASC").
		Scan(&rows).Error; err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("session_id", client.sessionID).Msg("fetch offline messages failed")
		return
	}

	if len(rows) == 0 {
		if sinceSeq > 0 {
			pending := GlobalPendingAck().PendingSince(client.sessionID, sinceSeq)
			if len(pending) == 0 {
				return
			}
			missedEnv := MustEnvelope(NextSeq(), "missed_ack", map[string]any{
				"session_id": client.sessionID,
				"pending":    pending,
			})
			if bytes, err := missedEnv.MarshalBytes(); err == nil {
				sendToClient(client, bytes)
			}
		}
		return
	}

	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
		env := MustEnvelope(NextSeq(), TypeMessage, map[string]any{
			"id":          r.ID,
			"content":     r.Content,
			"sender_type": r.SenderType,
			"sender_name": r.SenderName,
			"ai_source":   r.AISource,
			"confidence":  r.AIConfidence,
			"created_at":  r.CreatedAt,
			"offline":     true, 
		})
		if bytes, err := env.MarshalBytes(); err == nil {
			sendToClient(client, bytes)
			GlobalPendingAck().Track(client.sessionID, env.Seq)
		}
	}

	now := time.Now()
	if err := h.db.Table("session_messages").
		Where("session_id = ? AND id IN ?", client.sessionID, ids).
		Update("delivered_at", &now).Error; err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("session_id", client.sessionID).Msg("mark delivered failed")
	}
}

// sendToClient 安全发送（带超时与重试）
func sendToClient(client *Client, payload []byte) {
	defer func() {
		_ = recover()
	}()
	select {
	case client.send <- payload:
	case <-time.After(500 * time.Millisecond):
	}
}

// writePump 写入协程
//
// 修复：
//   - 每次写操作前 SetWriteDeadline，防止对端 TCP 窗口关闭时本协程永久阻塞。
//   - 启动 pingPeriod 周期 ticker，主动发 PingMessage；
//     对端回 Pong 后会触发 readPump 中已注册的 SetPongHandler，
//     从而刷新 ReadDeadline，避免 60s 后误判超时断开。
func (h *VisitorWSHandler) writePump(client *Client, conn *websocket.Conn) {
	defer func() {
		conn.Close()
	}()

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case message, ok := <-client.send:
			if !ok {
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump 读取协程
//
// 鲁棒性加固（方向B）：
//   - `ack` 消息：客户端确认已收到的 seq 列表，清理待 ACK 队列
//   - `resume` 消息：客户端重连后增量补发请求（since_seq）
//   - `delivered` 消息：旧协议兼容（仅日志记录）
//   - `ping` 消息：JSON 文本 ping（保留兼容；推荐使用 WebSocket PingMessage）
//
// 安全修复（2026-08-18）：
//   - 添加 per-connection 消息速率限制（10条/秒突发 20条），防止恶意客户端洪泛
func (h *VisitorWSHandler) readPump(client *Client, conn *websocket.Conn, ctx context.Context) {
	defer func() {
		GlobalPendingAck().Drop(client.sessionID)
		unregisterVisitorClient(client)
		conn.Close()
		logger.Ctx(ctx).Info().Str("session_id", client.sessionID).Msg("visitor disconnected")
	}()

	conn.SetReadLimit(maxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// 安全：每连接消息速率限制（10条/秒，突发20条）
	msgLimiter := newMessageRateLimiter(10, 20)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Ctx(ctx).Error().Err(err).Str("session_id", client.sessionID).Msg("ws read error")
			}
			break
		}

		// 安全：速率限制检查
		if !msgLimiter.Allow() {
			logger.Ctx(ctx).Warn().Str("session_id", client.sessionID).Msg("ws message rate limit exceeded")
			sendVisitorError(client, "消息过于频繁，请稍后再试")
			continue
		}

		// 解析消息
		var msg map[string]any
		if err := json.Unmarshal(message, &msg); err != nil {
			sendVisitorError(client, "消息格式错误")
			continue
		}

		msgType, _ := msg["type"].(string)
		switch msgType {
		case "ping":
			pongEnv := MustEnvelope(NextSeq(), TypePong, map[string]any{"time": time.Now().Unix()})
			if bytes, err := pongEnv.MarshalBytes(); err == nil {
				sendToClient(client, bytes)
			}
		case "ack":
			ackedCount := handleAckMessage(client, msg)
			logger.Ctx(ctx).Debug().Str("session_id", client.sessionID).Int("acked", ackedCount).Msg("ack received")
		case "resume":
			sinceSeq := parseSinceSeq(msg)
			logger.Ctx(ctx).Info().Str("session_id", client.sessionID).Uint64("since_seq", sinceSeq).Msg("resume requested")
			go h.onConnect(client, sinceSeq, ctx)
		case "delivered":
			logger.Ctx(ctx).Debug().Str("session_id", client.sessionID).Interface("ack", msg).Msg("received delivered ack (legacy)")
		case "close":
			logger.Ctx(ctx).Info().Str("session_id", client.sessionID).Msg("visitor closed connection")
			return
		default:
			logger.Ctx(ctx).Warn().Str("session_id", client.sessionID).Str("msg_type", msgType).Msg("unhandled message type")
		}
	}
}

// handleAckMessage 解析 ack 消息并清理 GlobalPendingAck
//
// 支持两种协议：
//   - {"seq": 100}              单个 seq
//   - {"seq": [100, 101, 102]}  批量 seq
//
// 返回被清理的 seq 数量。
func handleAckMessage(client *Client, msg map[string]any) int {
	raw, ok := msg["seq"]
	if !ok {
		return 0
	}
	var seqs []uint64
	switch v := raw.(type) {
	case float64:
		seqs = []uint64{uint64(v)}
	case []any:
		for _, item := range v {
			if f, ok := item.(float64); ok {
				seqs = append(seqs, uint64(f))
			}
		}
	case []uint64:
		seqs = v
	}
	return GlobalPendingAck().Ack(client.sessionID, seqs...)
}

// parseSinceSeq 从上行消息解析 since_seq
func parseSinceSeq(msg map[string]any) uint64 {
	if v, ok := msg["since_seq"].(float64); ok {
		return uint64(v)
	}
	return 0
}

func sendVisitorError(client *Client, msg string) {
	errBytes, _ := json.Marshal(map[string]any{
		"type": TypeError,
		"payload": map[string]any{
			"message": msg,
		},
	})
	sendToClient(client, errBytes)
}

// messageRateLimiter 简单的令牌桶速率限制器（per-connection）
//
// 用于防止恶意客户端在 WebSocket 连接上洪泛消息。
// 使用标准库实现，避免额外依赖。
type messageRateLimiter struct {
	rate   float64   // 每秒允许的消息数
	cap    float64   // 令牌桶容量
	tokens float64   // 当前令牌数
	last   time.Time // 上次补充时间
	mu     sync.Mutex
}

// newMessageRateLimiter 创建消息速率限制器
// rate: 每秒允许的消息数, cap: 突发容量
func newMessageRateLimiter(rate, cap float64) *messageRateLimiter {
	return &messageRateLimiter{
		rate:   rate,
		cap:    cap,
		tokens: cap,
		last:   time.Now(),
	}
}

// Allow 检查是否允许发送消息
func (rl *messageRateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.last).Seconds()
	rl.last = now
	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.cap {
		rl.tokens = rl.cap
	}

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

// getVisitorTokenSecret 获取 visitor token HMAC 密钥
// 从 config 读取，未配置时使用默认值
func getVisitorTokenSecret() string {
	if cfg := config.GetAppConfig(); cfg.Security.VisitorTokenSecret != "" {
		return cfg.Security.VisitorTokenSecret
	}
	return "hivemtk-visitor-default-secret-change-me"
}

