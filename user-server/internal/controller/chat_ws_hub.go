// Package controller 包含 HTTP / WebSocket 控制器（5 层架构 L3）
//
// chat_ws_hub.go - 聊天 WebSocket Hub
// ============================================================================
// 5 层架构归属: L3 Controller 层（编排层）
//   - 仅依赖 service 接口（不直接访问 db / repository / model）
//   - 通道抽象 (dto.StreamChunk) 与 service 解耦
//
// 设计依据: AI 智能体性能优化 WebSocket 流式输出 Hub
//
// 职责:
//   - 维护所有 WebSocket Client 连接（按 sessionID 索引）
//   - 提供 register / unregister / Broadcast / SendToClient 能力
//   - 后台 goroutine 异步处理 register / unregister / broadcast 三类事件
//   - 通过 sync.RWMutex 保护 clients map，发送使用 chan 异步无锁
//
// 不做:
//   - 不解析业务消息（由 chat_ws.go 的 Controller 处理）
//   - 不调用 LLM（由 service.SalesEngine.HandleStream 处理）
//   - 不持久化消息（由 service 层处理）
package controller

import (
	"time"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/logger"
)


const (
	chatWSClientSendBuffer = 64

	chatWSBroadcastBuffer = 256
)


// Client 单个 WebSocket 连接客户端
//
// 字段说明:
//   - SessionID: 会话 ID（来自 query param "session_id"）
//   - CustomerID: 客户 ID（来自 query param "customer_id"）
//   - Conn: gorilla/websocket 连接
//   - send: 异步发送通道（由 Hub.SendToClient 写入，由 writePump 消费）
//   - traceID: 当前连接的 trace_id（用于链路日志）
//   - mu: 保护字段的并发读写（写入消息时为降低开销改用 send chan）
type Client struct {
	SessionID  string
	CustomerID string
	Conn       *websocket.Conn
	send       chan []byte
	traceID    string

	mu     sync.Mutex
	closed bool // v3 审计 P2-3：以标志位替代 close(chan)，根除并发 "send on closed channel" panic
}

// NewClient 创建 Client 实例
func NewClient(sessionID, customerID string, conn *websocket.Conn, traceID string) *Client {
	return &Client{
		SessionID:  sessionID,
		CustomerID: customerID,
		Conn:       conn,
		send:       make(chan []byte, chatWSClientSendBuffer),
		traceID:    traceID,
	}
}

// SendChan 返回发送通道（供 Hub.SendToClient 写入）
func (c *Client) SendChan() chan<- []byte { return c.send }

// RecvChan 返回接收通道（供 writePump 读取并发送给 WebSocket）
func (c *Client) RecvChan() <-chan []byte { return c.send }

// TraceID 返回 trace_id
func (c *Client) TraceID() string { return c.traceID }

// Close 标记客户端关闭并安全关闭 send 通道（由 Hub.Run 在 unregister 时调用）。
// v3 审计 P2-3：closed 标志保证并发发送方（sendSafe）先看到标志而放弃投递，
// 因此这里的 close(chan) 不会被并发写入命中，根除 "send on closed channel" panic。
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.send)
}

// sendSafe 线程安全发送：已关闭或缓冲满时返回 false（不阻塞、不 panic）。
func (c *Client) sendSafe(payload []byte) (sent bool) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()
	defer func() {
		// 双保险：任何路径下 close 竞态导致的 panic 均降级为投递失败
		if r := recover(); r != nil {
			sent = false
		}
	}()
	select {
	case c.send <- payload:
		return true
	default:
		return false
	}
}


// ChatWSHub WebSocket 连接管理 Hub
//
// 字段说明:
//   - clients: sessionID -> Client 索引（一会话一连接）
//   - register: 注册事件通道（外部调用 Register 后写入）
//   - unregister: 注销事件通道
//   - broadcast: 全局广播通道
//   - mu: 保护 clients map（避免与 Run 中的 select 竞争）
//   - done: 关闭信号（用于优雅停止 Run goroutine）
//
// wg: 跟踪 Run goroutine 生命周期 (: Stop 等待 Run 真正退出)
type ChatWSHub struct {
	clients     map[string]*Client
	register    chan *Client
	unregister  chan *Client
	broadcast   chan []byte
	mu          sync.RWMutex
	done        chan struct{}
	startedCh   chan struct{} 
	closeOnce   sync.Once
	startedOnce sync.Once
	wg sync.WaitGroup
}

// NewChatWSHub 创建 Hub 实例
//
// 返回的 Hub 需要调用 Run() 启动后台 goroutine，
// 并在程序退出时通过 Stop() 优雅关闭。
func NewChatWSHub() *ChatWSHub {
	h := &ChatWSHub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
		broadcast:  make(chan []byte, chatWSBroadcastBuffer),
		done:       make(chan struct{}),
		startedCh:  make(chan struct{}),
	}
	// Run goroutine 的生命周期配额在构造期预占：
	// 保证 Stop() 的 wg.Wait() 与 Run 的 defer Done 严格配对，无 Add/Wait 竞态窗口
	h.wg.Add(1)
	return h
}

// Run 启动 Hub 后台循环（必须在独立 goroutine 中调用）
//
// 职责:
//   - 处理 register 事件（加入 clients map）
//   - 处理 unregister 事件（从 clients map 移除 + 关闭 Client send chan）
//   - 处理 broadcast 事件（向所有 client 写入）
//
// 通过 wg.Add(1) / defer wg.Done 跟踪 goroutine 生命周期，
// 让 Stop() 阻塞等待 Run 真正退出（防止 Hub 关闭后 Run goroutine 残留）。
func (h *ChatWSHub) Run() {
	defer h.wg.Done() // 配额在 NewChatWSHub 预占
	h.startedOnce.Do(func() {
		close(h.startedCh)
	})
	for {
		select {
		case client := <-h.register:
			h.addClient(client)
		case client := <-h.unregister:
			h.removeClient(client)
		case msg := <-h.broadcast:
			h.fanout(msg)
		case <-h.done:
			h.closeAll()
			return
		}
	}
}

// Stop 停止 Hub（幂等）
//
// 关闭后所有 register/unregister/broadcast 写入将被忽略；
// 已注册的 Client 仍可通过直接关闭 conn 退出。
//
// 阻塞等待 Run goroutine 真正退出，防止 goroutine 残留
// (goleak 检测可识别)。Stop 必须在调用方期望的等待时间窗内返回,
// 否则调用方需自行设置超时。
func (h *ChatWSHub) Stop() {
	h.closeOnce.Do(func() {
		close(h.done)
	})
	// v3 flaky 修复：done 关闭后原 select 两分支同时就绪，随机选中 done 分支
	// 会跳过 wg.Wait()，导致 Run 尚未退出就返回（goleak 偶发误报/真泄漏窗口）。
	// Run 未被调用时 startedCh 永不关闭 → 3s 超时兜底防死等。
	select {
	case <-h.startedCh:
	case <-time.After(500 * time.Millisecond):
		return
	}
	h.wg.Wait()
}

// Register 注册一个 Client（异步）
//
// 非阻塞：仅向 register 通道写入；真正的 map 更新由 Run goroutine 处理。
func (h *ChatWSHub) Register(c *Client) {
	select {
	case h.register <- c:
	default:
		h.addClient(c)
	}
}

// Unregister 注销一个 Client（异步）
//
// 通过 sessionID 查找；未找到时直接忽略（幂等）。
func (h *ChatWSHub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	default:
		h.removeClient(c)
	}
}

// SendToClient 向指定 sessionID 的 Client 发送字节流
//
// 返回 false 表示该 sessionID 不存在或 send 通道已满（积压）。
func (h *ChatWSHub) SendToClient(sessionID string, payload []byte) bool {
	h.mu.RLock()
	client, ok := h.clients[sessionID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	return client.sendSafe(payload)
}

// SendChunk 向指定 sessionID 发送 StreamChunk（便捷方法）
//
// 自动 JSON 编码；返回 false 表示发送失败（Client 不存在或通道已满）。
func (h *ChatWSHub) SendChunk(sessionID string, chunk *dto.StreamChunk) bool {
	if chunk == nil {
		return false
	}
	payload, err := json.Marshal(chunk)
	if err != nil {
		logger.Errorf("[ws-hub] marshal StreamChunk failed: %v", err)
		return false
	}
	return h.SendToClient(sessionID, payload)
}

// Broadcast 广播字节流到所有 Client
func (h *ChatWSHub) Broadcast(payload []byte) {
	select {
	case h.broadcast <- payload:
	default:
		logger.Warn("[ws-hub] broadcast channel full, message dropped")
	}
}

// BroadcastChunk 广播 StreamChunk 到所有 Client（便捷方法）
func (h *ChatWSHub) BroadcastChunk(chunk *dto.StreamChunk) bool {
	if chunk == nil {
		return false
	}
	payload, err := json.Marshal(chunk)
	if err != nil {
		logger.Errorf("[ws-hub] marshal StreamChunk failed: %v", err)
		return false
	}
	h.Broadcast(payload)
	return true
}

// ClientCount 返回当前在线 Client 数（用于监控）
func (h *ChatWSHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// IsOnline 判断指定 sessionID 是否在线
func (h *ChatWSHub) IsOnline(sessionID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.clients[sessionID]
	return ok
}


// addClient 内部方法：加入 clients map（不幂等；已存在则覆盖）
func (h *ChatWSHub) addClient(c *Client) {
	if c == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.clients[c.SessionID]; ok {
		old.Close()
		_ = old.Conn.Close()
	}
	h.clients[c.SessionID] = c
	logger.Infof("[ws-hub] client registered session_id=%s customer_id=%s trace_id=%s total=%d",
		c.SessionID, c.CustomerID, c.traceID, len(h.clients))
}

// removeClient 内部方法：从 clients map 移除
func (h *ChatWSHub) removeClient(c *Client) {
	if c == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if cur, ok := h.clients[c.SessionID]; ok && cur == c {
		delete(h.clients, c.SessionID)
		c.Close()
		logger.Infof("[ws-hub] client unregistered session_id=%s customer_id=%s total=%d",
			c.SessionID, c.CustomerID, len(h.clients))
	}
}

// fanout 内部方法：向所有 Client 广播
func (h *ChatWSHub) fanout(payload []byte) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		if !c.sendSafe(payload) {
			logger.Warnf("[ws-hub] broadcast to client session_id=%s dropped (closed or send channel full)", c.SessionID)
		}
	}
}

// closeAll 内部方法：关闭所有 Client（Hub 停止时调用）
func (h *ChatWSHub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sessionID, c := range h.clients {
		c.Close()
		if c.Conn != nil {
			_ = c.Conn.Close()
		}
		logger.Infof("[ws-hub] closing client session_id=%s due to hub stop", sessionID)
	}
	h.clients = make(map[string]*Client)
}


// WrapErr 统一错误包装（避免 chat_ws.go / chat_ws_hub.go 重复 fmt.Errorf）
//
// 保留 error chain（使用 %w），便于 controller 层 fmt.Errorf("xxx: %w", err) 包装。
func WrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}



