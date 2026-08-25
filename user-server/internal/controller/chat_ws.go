// Package controller - chat_ws.go 聊天 WebSocket 控制器（-）
// ============================================================================
// 5 层架构归属: L3 Controller 层（编排层）
//   - 仅依赖 service 接口（不直接访问 db / repository / model）
//   - 通过 StreamEngineInterface 抽象 service.SalesEngine（依赖倒置 / 解耦）
//
// 设计依据: AI 智能体性能优化 - WebSocket 流式输出
//
// 职责:
//   - WebSocket 协议升级（gorilla/websocket）
//   - 鉴权：query param session_id + customer_id
//   - 绑定 trace_id（时间戳 + 随机数）
//   - 监听客户端消息：解析为 dto.SalesRequest
//   - 调用 StreamEngine.HandleStream
//   - 将 LLM 增量输出通过 dto.StreamChunk 推送给客户端
//   - 心跳：30 秒 ping/pong
//   - 关闭时清理 client
//
// 协议说明（client -> server）:
//   - {"type":"message","user_message":"...","platform":"..."} -> 业务消息
//   - {"type":"ping"} -> 心跳（兼容旧客户端）
//
// 协议说明（server -> client, 复用 dto.StreamChunk）:
//   - {"type":"start","trace_id":"xxx","step":"start"} -> 流开始
//   - {"type":"delta","text":"...","step":"delta"} -> 增量
//   - {"type":"final","text":"完整回复","steps":[...],"wall_ms":1234} -> 流结束
//   - {"type":"error","error":"..."} -> 错误
//   - {"type":"cancel"} -> 取消
package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/contract"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/pkg/utils/response"
)


const (
	chatWSPingPeriod = 30 * time.Second

	chatWSPongWait = 60 * time.Second

	chatWSWriteWait = 10 * time.Second

	chatWSReadLimit = 8 * 1024
)


// StreamEngineInterface 流式销售引擎接口（向后兼容别名）
//
// Deprecated: 请直接引用 contract.StreamEngineInterface。
// 保留别名仅为兼容历史调用方, 后续 controller/ 内部将统一改用 contract。
type StreamEngineInterface = contract.StreamEngineInterface


// ChatWSController WebSocket 聊天控制器
//
// 字段说明:
//   - hub: WebSocket 连接管理 Hub
//   - engine: 流式销售引擎（通过 interface 注入，依赖倒置）
//   - upgrader: gorilla/websocket 升级器（生产环境应配置精确的 CheckOrigin）
type ChatWSController struct {
	hub      *ChatWSHub
	engine   contract.StreamEngineInterface
	upgrader websocket.Upgrader
}

// NewChatWSController 创建控制器
//
// 参数 engine 可为 nil（nil 时 HandleChatWS 返回 503）；hub 必须非 nil。
//
// CheckOrigin 改为从 config 读取白名单
//   - 默认 ["http://localhost:3000", "http://localhost:8080"] (本地开发)
//   - 生产应通过 env ALLOWED_WS_ORIGINS 或 config.yaml platform.allowed_ws_origins 覆盖
//   - 白名单包含 "*" 时放行所有 origin (仅调试)
func NewChatWSController(hub *ChatWSHub, engine contract.StreamEngineInterface) *ChatWSController {
	return &ChatWSController{
		hub:    hub,
		engine: engine,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     buildCheckOrigin(config.GetAllowedWSOrigins()),
		},
	}
}

// buildCheckOrigin 构造 gorilla/websocket.Upgrader.CheckOrigin 回调
//
// 规则:
//   - 白名单包含 "*" -> 放行所有 origin (调试用, 生产禁用)
//   - 否则严格匹配 (==); 大小写敏感, 不支持通配符
//
// 返回的 func 闭包持有 allowedOrigins 切片, 单次 controller 启动时锁定。
// 如需运行时重载, 调用 config.ReloadAllowedWSOrigins() 后重建 controller。
func buildCheckOrigin(allowedOrigins []string) func(r *http.Request) bool {
	allowed := make([]string, len(allowedOrigins))
	copy(allowed, allowedOrigins)
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		for _, o := range allowed {
			if o == "*" {
				return true
			}
			if o == origin {
				return true
			}
		}
		return false
	}
}

// HandleChatWS WebSocket 端点处理函数
//
// 路由: GET /ws/chat?session_id=xxx&customer_id=xxx
//
// 流程:
//  1. 鉴权（query param 必填）
//  2. 绑定 trace_id
//  3. 升级 WebSocket 协议
//  4. 注册到 Hub
//  5. 启动 readPump / writePump
//  6. 连接关闭时清理
func (ctrl *ChatWSController) HandleChatWS(c *gin.Context) {
	if ctrl.engine == nil {
		response.Error(c, http.StatusServiceUnavailable, "stream engine not configured")
		return
	}
	if ctrl.hub == nil {
		response.Error(c, http.StatusInternalServerError, "ws hub not configured")
		return
	}

	sessionID := c.Query("session_id")
	customerID := c.Query("customer_id")
	if sessionID == "" {
		response.Error(c, http.StatusBadRequest, "session_id is required")
		return
	}
	if customerID == "" {
		response.Error(c, http.StatusBadRequest, "customer_id is required")
		return
	}

	traceID := newTraceID()
	ctx := logger.WithTraceID(c.Request.Context(), traceID)
	ctx = logger.WithModule(ctx, "ws-chat")
	logger.Ctx(ctx).Info().Msgf("[ws-chat] new connection session_id=%s customer_id=%s", sessionID, customerID)

	conn, err := ctrl.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[ws-chat] upgrade failed")
		return
	}

	client := NewClient(sessionID, customerID, conn, traceID)
	ctrl.hub.Register(client)

	go ctrl.writePump(client, conn, ctx)
	go ctrl.readPump(client, conn, ctrl.engine, ctx)
}


// readPump 读取客户端消息并处理业务逻辑
//
// 职责:
//   - 设置读超时 / pong handler
//   - 解析消息为 dto.SalesRequest
//   - 调用 engine.HandleStream（StreamChunk 回调 -> 写入 client.send）
//   - 连接断开时注销 client
func (ctrl *ChatWSController) readPump(client *Client, conn *websocket.Conn, engine StreamEngineInterface, ctx context.Context) {
	defer func() {
		ctrl.hub.Unregister(client)
		_ = conn.Close()
		client.Close()
		logger.Ctx(ctx).Info().Str("session_id", client.SessionID).Msg("[ws-chat] readPump exit")
	}()

	conn.SetReadLimit(chatWSReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(chatWSPongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(chatWSPongWait))
		return nil
	})

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
				websocket.CloseNormalClosure) {
				logger.Ctx(ctx).Error().Err(err).Msg("[ws-chat] read error")
			}
			return
		}

		// 解析消息
		var msg wsIncomingMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Msg("[ws-chat] invalid message format")
			ctrl.sendErrorChunk(ctx, client, "invalid_message", "消息格式错误")
			continue
		}

		switch msg.Type {
		case "ping":
			pong := []byte(`{"type":"pong","trace_id":"` + client.TraceID() + `"}`)
			select {
			case client.SendChan() <- pong:
			default:
				logger.Ctx(ctx).Warn().Msg("[ws-chat] send pong: channel full, dropped")
			}
		case "message":
			ctrl.handleBusinessMessage(ctx, client, engine, msg)
		case "cancel":
			logger.Ctx(ctx).Info().Msg("[ws-chat] client sent cancel")
		default:
			logger.Ctx(ctx).Warn().Str("type", msg.Type).Msg("[ws-chat] unknown message type")
		}
	}
}

// writePump 将 client.send 中的字节写入 WebSocket
//
// 职责:
//   - 监听 client.send 通道（异步发送）
//   - 周期性 ping（保活）
func (ctrl *ChatWSController) writePump(client *Client, conn *websocket.Conn, ctx context.Context) {
	ticker := time.NewTicker(chatWSPingPeriod)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
		logger.Ctx(ctx).Info().Str("session_id", client.SessionID).Msg("[ws-chat] writePump exit")
	}()

	for {
		select {
		case payload, ok := <-client.RecvChan():
			if !ok {
				_ = conn.SetWriteDeadline(time.Now().Add(chatWSWriteWait))
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(chatWSWriteWait))
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				logger.Ctx(ctx).Error().Err(err).Msg("[ws-chat] write failed")
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(chatWSWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logger.Ctx(ctx).Debug().Err(err).Msg("[ws-chat] ping failed, client disconnected")
				return
			}
		}
	}
}


// handleBusinessMessage 处理业务消息（user_message -> 流式回复）
//
// 流程:
//  1. 构造 dto.SalesRequest
//  2. 定义 StreamChunk 回调（写入 client.send）
//  3. 调用 engine.HandleStream
//  4. 错误处理（发送 error chunk）
func (ctrl *ChatWSController) handleBusinessMessage(
	ctx context.Context,
	client *Client,
	engine StreamEngineInterface,
	msg wsIncomingMessage,
) {
	if msg.UserMessage == "" {
		ctrl.sendErrorChunk(ctx, client, "empty_message", "user_message 不能为空")
		return
	}

	req := &dto.SalesRequest{
		SessionID:   client.SessionID,
		CustomerID:  client.CustomerID,
		UserMessage: msg.UserMessage,
		Platform:    defaultStr(msg.Platform, "ws"),
		Config:      dto.DefaultSalesEngineConfig(),
	}

	sendChunk := func(chunk *dto.StreamChunk) bool {
		if chunk == nil {
			return true
		}
		payload, err := json.Marshal(chunk)
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[ws-chat] marshal chunk failed")
			return true 
		}
		select {
		case client.SendChan() <- payload:
			return true
		default:
			logger.Ctx(ctx).Warn().Msg("[ws-chat] client send channel full, abort stream")
			return false
		}
	}

	logger.Ctx(ctx).Info().Msgf("[ws-chat] handle message session=%s msg_len=%d", client.SessionID, len(msg.UserMessage))
	if err := engine.HandleStream(ctx, req, sendChunk); err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[ws-chat] HandleStream failed")
	}
}

// sendErrorChunk 发送错误 chunk 给客户端（异步，写入 client.send）
func (ctrl *ChatWSController) sendErrorChunk(ctx context.Context, client *Client, code, msg string) {
	chunk := &dto.StreamChunk{
		Type:    dto.ChunkTypeError,
		TraceID: client.TraceID(),
		Error:   code + ": " + msg,
	}
	payload, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	select {
	case client.SendChan() <- payload:
	default:
		logger.Ctx(ctx).Warn().Msg("[ws-chat] send error chunk: channel full")
	}
}


// wsIncomingMessage 客户端 -> 服务端 消息
//
// 字段大小写不敏感（JSON 标准）；使用 client API 时建议保持小写。
type wsIncomingMessage struct {
	Type        string `json:"type"` 
	UserMessage string `json:"user_message"`
	Platform    string `json:"platform,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	CustomerID  string `json:"customer_id,omitempty"`
}


// newTraceID 生成 trace_id（时间戳 + 8 字节随机）
//
// 示例: "1753948800123-a1b2c3d4e5f6g7h8"
// 用于跨协程 / 跨服务调用链路追踪。
func newTraceID() string {
	ts := time.Now().UnixMilli()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d-fallback", ts)
	}
	return fmt.Sprintf("%d-%s", ts, hex.EncodeToString(b))
}

// defaultStr 返回 s 当 s 非空，否则返回 fallback
func defaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

