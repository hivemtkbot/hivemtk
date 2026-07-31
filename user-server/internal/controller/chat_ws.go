// Package controller - chat_ws.go 聊天 WebSocket 控制器（T15-T17）
// ============================================================================
// 5 层架构归属: L3 Controller 层（编排层）
//   - 仅依赖 service 接口（不直接访问 db / repository / model）
//   - 通过 StreamEngineInterface 抽象 service.SalesEngine（依赖倒置 / 解耦）
//
// 设计依据: 2026-07-31 AI 智能体性能优化 (T15-T17 - WebSocket 流式输出)
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

	"marketing/internal/dto"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// 常量
// ============================================================================

const (
	// chatWSPingPeriod 心跳周期（30s）- 主动向客户端发 PingMessage
	chatWSPingPeriod = 30 * time.Second

	// chatWSPongWait 读超时（60s）- 客户端需在 60s 内回 Pong 或任意帧
	chatWSPongWait = 60 * time.Second

	// chatWSWriteWait 写超时（10s）- 单次写操作最长 10s
	chatWSWriteWait = 10 * time.Second

	// chatWSReadLimit 读消息最大体积（8KB）- 防止恶意大帧
	chatWSReadLimit = 8 * 1024
)

// ============================================================================
// StreamEngineInterface
// ============================================================================

// StreamEngineInterface 流式销售引擎接口（依赖倒置：避免 controller ↔ service 循环依赖）
//
// 实现方: *service.SalesEngine（通过 Go 鸭子类型自动满足）
//
// 设计动机：
//   - Controller 不直接持有 *service.SalesEngine 具体类型（编译期解耦）
//   - 单测可注入 mock 实现（无需启动完整 service 栈）
//   - router 层注入 *service.SalesEngine 时只需传 interface，类型签名更稳定
type StreamEngineInterface interface {
	// HandleStream 流式处理销售请求，逐 chunk 回调
	//
	// 返回 false 表示调用方（controller）希望中断流；返回 nil 错误表示正常完成。
	// 任何内部错误会被包装为 dto.StreamChunk{Type: error} 推给客户端。
	HandleStream(ctx context.Context, req *dto.SalesRequest, onChunk func(chunk *dto.StreamChunk) bool) error
}

// ============================================================================
// ChatWSController
// ============================================================================

// ChatWSController WebSocket 聊天控制器
//
// 字段说明:
//   - hub: WebSocket 连接管理 Hub
//   - engine: 流式销售引擎（通过 interface 注入，依赖倒置）
//   - upgrader: gorilla/websocket 升级器（生产环境应配置精确的 CheckOrigin）
type ChatWSController struct {
	hub      *ChatWSHub
	engine   StreamEngineInterface
	upgrader websocket.Upgrader
}

// NewChatWSController 创建控制器
//
// 参数 engine 可为 nil（nil 时 HandleChatWS 返回 503）；hub 必须非 nil。
func NewChatWSController(hub *ChatWSHub, engine StreamEngineInterface) *ChatWSController {
	return &ChatWSController{
		hub:    hub,
		engine: engine,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// TODO: 生产环境应按渠道白名单校验 Origin
				return true
			},
		},
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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "stream engine not configured"})
		return
	}
	if ctrl.hub == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ws hub not configured"})
		return
	}

	// 1) 鉴权：query param
	sessionID := c.Query("session_id")
	customerID := c.Query("customer_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}
	if customerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "customer_id is required"})
		return
	}

	// 2) 绑定 trace_id（时间戳 + 8 字节随机十六进制）
	traceID := newTraceID()
	ctx := logger.WithTraceID(c.Request.Context(), traceID)
	ctx = logger.WithModule(ctx, "ws-chat")
	logger.Ctx(ctx).Info().Msgf("[ws-chat] new connection session_id=%s customer_id=%s", sessionID, customerID)

	// 3) 升级 WebSocket 协议
	conn, err := ctrl.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[ws-chat] upgrade failed")
		return
	}

	// 4) 创建 Client 并注册到 Hub
	client := NewClient(sessionID, customerID, conn, traceID)
	ctrl.hub.Register(client)

	// 5) 启动读写协程（透传 ctx）
	go ctrl.writePump(client, conn, ctx)
	go ctrl.readPump(client, conn, ctrl.engine, ctx)
}

// ============================================================================
// readPump / writePump
// ============================================================================

// readPump 读取客户端消息并处理业务逻辑
//
// 职责:
//   - 设置读超时 / pong handler
//   - 解析消息为 dto.SalesRequest
//   - 调用 engine.HandleStream（StreamChunk 回调 -> 写入 client.send）
//   - 连接断开时注销 client
func (ctrl *ChatWSController) readPump(client *Client, conn *websocket.Conn, engine StreamEngineInterface, ctx context.Context) {
	defer func() {
		// 清理
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
			// 心跳（兼容旧客户端的文本 ping）
			_ = conn.SetWriteDeadline(time.Now().Add(chatWSWriteWait))
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"pong","trace_id":"`+client.TraceID()+`"}`))
		case "message":
			// 业务消息
			ctrl.handleBusinessMessage(ctx, client, engine, msg)
		case "cancel":
			// 客户端主动取消（仅记录，不打断流；流式本身 ctx 不可由外部中断）
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
				// hub 关闭了 client.send 通道，发送 Close 帧后退出
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

// ============================================================================
// 业务消息处理
// ============================================================================

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

	// 1) 构造请求
	req := &dto.SalesRequest{
		SessionID:   client.SessionID,
		CustomerID:  client.CustomerID,
		UserMessage: msg.UserMessage,
		Platform:    defaultStr(msg.Platform, "ws"),
		Config:      dto.DefaultSalesEngineConfig(),
	}

	// 2) 定义 chunk 回调（直接写入 client.send）
	sendChunk := func(chunk *dto.StreamChunk) bool {
		if chunk == nil {
			return true
		}
		payload, err := json.Marshal(chunk)
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[ws-chat] marshal chunk failed")
			return true // 继续流
		}
		select {
		case client.SendChan() <- payload:
			return true
		default:
			// 通道已满（client 异常），中断流
			logger.Ctx(ctx).Warn().Msg("[ws-chat] client send channel full, abort stream")
			return false
		}
	}

	// 3) 调用 engine
	logger.Ctx(ctx).Info().Msgf("[ws-chat] handle message session=%s msg_len=%d", client.SessionID, len(msg.UserMessage))
	if err := engine.HandleStream(ctx, req, sendChunk); err != nil {
		// error chunk（HandleStream 内部已发 error chunk，此处仅记录日志）
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

// ============================================================================
// 消息结构
// ============================================================================

// wsIncomingMessage 客户端 -> 服务端 消息
//
// 字段大小写不敏感（JSON 标准）；使用 client API 时建议保持小写。
type wsIncomingMessage struct {
	Type        string `json:"type"` // "message" / "ping" / "cancel"
	UserMessage string `json:"user_message"`
	Platform    string `json:"platform,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	CustomerID  string `json:"customer_id,omitempty"`
}

// ============================================================================
// 工具函数
// ============================================================================

// newTraceID 生成 trace_id（时间戳 + 8 字节随机）
//
// 示例: "1753948800123-a1b2c3d4e5f6g7h8"
// 用于跨协程 / 跨服务调用链路追踪。
func newTraceID() string {
	ts := time.Now().UnixMilli()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// 退化：使用固定字符串
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
