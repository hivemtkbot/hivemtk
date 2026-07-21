package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"marketing/internal/pkg/utils/logger"

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
// 私域部署模式（2026-07-17 优化）：无需鉴权，自己网站直连。
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
	hub *Hub
	db  *gorm.DB
}

// NewVisitorWSHandler 创建访客 WebSocket 处理器
func NewVisitorWSHandler(db *gorm.DB) *VisitorWSHandler {
	return &VisitorWSHandler{hub: GetHub(), db: db}
}

// upgraderVisitor 访客连接升级器（允许跨域，私域部署 + 自有网站）
var upgraderVisitor = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源（私域部署，自有网站无需鉴权）
	},
}

// HandleVisitorWebSocket 处理访客 WebSocket 连接
func (h *VisitorWSHandler) HandleVisitorWebSocket(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Query("session_id"))
	visitorID := strings.TrimSpace(c.Query("visitor_id"))
	channelID := strings.TrimSpace(c.Query("channel_id"))
	if channelID == "" {
		channelID = "default"
	}

	if sessionID == "" || visitorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少必要参数：session_id / visitor_id",
		})
		return
	}

	// 分配/透传追踪 ID：每个访客连接一条链路，便于追踪单个访客的会话生命周期
	ctx := logger.WithTraceID(c.Request.Context(), c.GetHeader("X-Trace-Id"))
	ctx = logger.WithModule(ctx, "websocket")

	// 升级 WebSocket 连接
	conn, err := upgraderVisitor.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("session_id", sessionID).Str("visitor_id", visitorID).Msg("ws upgrade failed")
		return
	}

	// 创建访客客户端
	client := NewVisitorClient(h.hub, sessionID, visitorID, channelID)
	client.hub = h.hub
	registerVisitorClient(client)

	logger.Ctx(ctx).Info().
		Str("session_id", sessionID).
		Str("visitor_id", visitorID).
		Str("channel_id", channelID).
		Msg("visitor connected")

	// 启动读写协程（均透传 ctx 以共享 trace_id）
	go h.writePump(client, conn)
	go h.readPump(client, conn, ctx)

	// 连接建立后异步推送欢迎消息 + 离线消息
	go h.onConnect(client, ctx)
}

// onConnect 访客连接建立后的处理
// 1. 推送 welcome 消息
// 2. 拉取离线期间未投递的坐席/AI 回复，批量推送
// 3. 标记已投递
func (h *VisitorWSHandler) onConnect(client *Client, ctx context.Context) {
	if h.db == nil {
		return
	}

	// 1. welcome 帧
	welcomeBytes, _ := json.Marshal(map[string]any{
		"type": TypeWelcome,
		"payload": map[string]any{
			"session_id": client.sessionID,
			"visitor_id": client.visitorID,
			"channel_id": client.channelID,
			"time":       time.Now().Unix(),
		},
	})
	sendToClient(client, welcomeBytes)

	// 2. 拉取离线消息
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
		return
	}

	// 3. 推送离线消息
	ids := make([]uint, 0, len(rows))
	payload := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
		payload = append(payload, map[string]any{
			"id":          r.ID,
			"content":     r.Content,
			"sender_type": r.SenderType,
			"sender_name": r.SenderName,
			"ai_source":   r.AISource,
			"confidence":  r.AIConfidence,
			"created_at":  r.CreatedAt,
			"offline":     true, // 标记为离线消息
		})
	}
	offlineBytes, _ := json.Marshal(map[string]any{
		"type":    TypeOfflineMessages,
		"payload": map[string]any{"messages": payload, "count": len(payload)},
	})
	sendToClient(client, offlineBytes)

	// 4. 标记已投递
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
		// 客户端发送 channel 阻塞，丢弃
	}
}

// writePump 写入协程
//
// P2-11 修复：
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
				// hub 关闭了 channel，发送 Close 帧后退出
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			// 定期发 Ping 维持连接；Pong 由 readPump 的 SetPongHandler 处理
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump 读取协程
func (h *VisitorWSHandler) readPump(client *Client, conn *websocket.Conn, ctx context.Context) {
	defer func() {
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

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Ctx(ctx).Error().Err(err).Str("session_id", client.sessionID).Msg("ws read error")
			}
			break
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
			// 心跳响应
			pongBytes, _ := json.Marshal(map[string]any{"type": TypePong, "time": time.Now().Unix()})
			sendToClient(client, pongBytes)
		case "delivered":
			// 客户端确认已收到某批消息（暂不持久化，靠 server 端主动标记为主）
			logger.Ctx(ctx).Debug().Str("session_id", client.sessionID).Interface("ack", msg).Msg("received delivered ack")
		case "close":
			logger.Ctx(ctx).Info().Str("session_id", client.sessionID).Msg("visitor closed connection")
			return
		default:
			logger.Ctx(ctx).Warn().Str("session_id", client.sessionID).Str("msg_type", msgType).Msg("unhandled message type")
		}
	}
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
