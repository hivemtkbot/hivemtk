package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service/translation"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true 
	},
}

// WSHandler WebSocket 处理器
type WSHandler struct {
	hub          *Hub
	langResolver *translation.LangConfigResolver
}

// NewWSHandler 创建 WebSocket 处理器
func NewWSHandler() *WSHandler {
	return &WSHandler{
		hub: GetHub(),
	}
}

// SetLangResolver 注入多语言解析器（v1.2 出海方案）。
// 未注入时仍可正常工作，ctx 中语言走默认 zh 兜底。
func (h *WSHandler) SetLangResolver(r *translation.LangConfigResolver) {
	h.langResolver = r
}

// HandleWebSocket 处理 WebSocket 连接
func (h *WSHandler) HandleWebSocket(c *gin.Context) {
	agentIDStr := c.Query("agent_id")
	agentName := c.Query("agent_name")

	if agentIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少必要参数：agent_id",
		})
		return
	}

	agentID, err := strconv.ParseUint(agentIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的 agent_id",
		})
		return
	}


	ctx := logger.WithTraceID(c.Request.Context(), c.GetHeader("X-Trace-Id"))
	ctx = logger.WithModule(ctx, "websocket")

	ctx = injectLangToCtx(ctx, h.langResolver, "", 0)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("agent_id", agentIDStr).Msg("ws upgrade failed")
		return
	}

	client := NewWSClient(h.hub, agentIDStr, agentName)
	client.hub = h.hub

	h.hub.Register(client)

	go h.writePump(client, conn, ctx)
	go h.readPump(client, conn, uint(agentID), ctx)

	logger.Ctx(ctx).Info().
		Str("agent_name", agentName).
		Str("agent_id", agentIDStr).
		Msg("agent connected")
}

// writePump 向客户端发送消息
//
// 修复：
//   - 每次写操作前 SetWriteDeadline，防止对端 TCP 窗口关闭时本协程永久阻塞。
//   - 启动 pingPeriod 周期 ticker，主动发 PingMessage；
//     对端回 Pong 后会触发 readPump 中已注册的 SetPongHandler，
//     从而刷新 ReadDeadline，避免 60s 后误判超时断开。
func (h *WSHandler) writePump(client *Client, conn *websocket.Conn, ctx context.Context) {
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
			w, err := conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			msg := map[string]any{
				"type":    "message",
				"payload": json.RawMessage(message),
				"time":    time.Now().Format("2006-01-02 15:04:05"),
			}

			msgBytes, _ := json.Marshal(msg)
			w.Write(msgBytes)

			if err := w.Close(); err != nil {
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

// readPump 从客户端接收消息
//
// 修复：
//   - SetReadLimit：限制单条消息 8KB，防止恶意大帧耗尽内存。
//   - SetReadDeadline + SetPongHandler：60s 内必须收到对端 Pong 或任何帧，
//     否则判定连接僵死主动关闭；PongHandler 在收到 Pong 时刷新 ReadDeadline。
//   - 配合 writePump 的 pingPeriod ticker，形成完整的心跳保活闭环。
func (h *WSHandler) readPump(client *Client, conn *websocket.Conn, agentID uint, ctx context.Context) {
	defer func() {
		h.hub.Unregister(client)
		conn.Close()
		logger.Ctx(ctx).Info().Uint("agent_id", agentID).Msg("agent disconnected")
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
				logger.Ctx(ctx).Error().Err(err).Uint("agent_id", agentID).Msg("ws read error")
			}
			break
		}

		// 处理客户端消息
		var msg map[string]any
		if err := json.Unmarshal(message, &msg); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Uint("agent_id", agentID).Msg("message parse failed")
			continue
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		switch msgType {
		case "ping":
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"pong"}`))
		case "mark_read":
			h.handleMarkRead(client, msg)
		case "session_action":
			h.handleSessionAction(client, msg)
		}
	}
}

// handleMarkRead 处理标记已读
func (h *WSHandler) handleMarkRead(client *Client, msg map[string]any) {
	sessionID, ok := msg["session_id"].(string)
	if !ok {
		return
	}

	logger.GetLogger().Info().
		Str("agent_name", client.agentName).
		Str("session_id", sessionID).
		Msg("mark session read")
}

// handleSessionAction 处理会话操作
func (h *WSHandler) handleSessionAction(client *Client, msg map[string]any) {
	action, ok := msg["action"].(string)
	if !ok {
		return
	}

	sessionID, _ := msg["session_id"].(string)

	logger.GetLogger().Info().
		Str("agent_name", client.agentName).
		Str("session_id", sessionID).
		Str("action", action).
		Msg("session action")

	switch action {
	case "take_over":
		h.handleTakeOver(client, sessionID)
	case "transfer":
		targetAgentID, _ := msg["target_agent_id"].(float64)
		h.handleTransfer(client, sessionID, uint(targetAgentID))
	case "close":
		h.handleClose(client, sessionID)
	}
}

// handleTakeOver 处理接管会话
func (h *WSHandler) handleTakeOver(client *Client, sessionID string) {
	logger.GetLogger().Info().
		Str("agent_name", client.agentName).
		Str("session_id", sessionID).
		Msg("take over session")
}

// handleTransfer 处理转接会话
func (h *WSHandler) handleTransfer(client *Client, sessionID string, targetAgentID uint) {
	logger.GetLogger().Info().
		Str("agent_name", client.agentName).
		Str("session_id", sessionID).
		Uint("target_agent_id", targetAgentID).
		Msg("transfer session")
}

// handleClose 处理关闭会话
func (h *WSHandler) handleClose(client *Client, sessionID string) {
	logger.GetLogger().Info().
		Str("agent_name", client.agentName).
		Str("session_id", sessionID).
		Msg("close session")
}

func (h *WSHandler) BroadcastMessage(messageType string, data any) error {
	return GetHub().BroadcastToMerchant("", messageType, data)
}

// SendToAgent 发送消息给指定客服
func (h *WSHandler) SendToAgent(agentID uint, messageType string, data any) error {
	return GetHub().SendToAgent(strconv.FormatUint(uint64(agentID), 10), messageType, data)
}

