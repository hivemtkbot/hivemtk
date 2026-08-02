package service

// sse_hub.go SSE 实时驾驶舱事件总线
//
// 五层架构归属: L2 服务层
// 设计依据: PRD § 缺口修复
// 私域独立部署: 无 merchant_id 字段
//
// 功能：
//   - SSE Hub 管理 client 连接，按 topic 订阅
//   - Topics：llm_calls / intent_recognition / rag_queries / agent_actions / humanize_scores / system_alerts
//   - 心跳：每 15 秒发送 :ping
//   - 单 IP 最多 5 个连接
//   - 缓冲区 100 条消息
//   - 支持 Publish 广播事件到所有订阅该 topic 的 client
//
// 与 trace_context.go 的 InMemoryTraceBus 的关系：
//   - TraceBus 用于内部 trace 事件传递（service → service）
//   - SSEHub 用于将事件推送到前端 dashboard（service → 浏览器）
//   - 两者解耦：TraceBus 的订阅者可以调用 SSEHub.Publish 转发到前端

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// SSE Topic 常量
const (
	SSETopicLLMCalls       = "llm_calls"          // LLM 调用事件
	SSETopicIntentRecogn   = "intent_recognition" // 意图识别事件
	SSETopicRAGQueries     = "rag_queries"        // RAG 检索事件
	SSETopicAgentActions   = "agent_actions"      // 智能体动作事件
	SSETopicHumanizeScores = "humanize_scores"    // 拟人度评分事件
	SSETopicSystemAlerts   = "system_alerts"      // 系统告警事件
)

// SSE 默认参数
const (
	SSEHeartbeatInterval = 15 * time.Second // 心跳间隔
	SSEMaxConnPerIP      = 5                // 单 IP 最大连接数
	SSEClientBufferSize  = 100              // 客户端缓冲区大小
	SSEWriteTimeout      = 30 * time.Second // 写超时
)

// SSEEvent SSE 事件
type SSEEvent struct {
	Topic     string    `json:"topic"`              // 事件 topic
	EventType string    `json:"event_type"`         // 事件类型（如 llm_call_completed）
	Data      any       `json:"data"`               // 事件数据
	TraceID   string    `json:"trace_id,omitempty"` // 关联 trace_id
	Timestamp time.Time `json:"timestamp"`          // 事件时间戳
}

// SSEClient 单个 SSE 客户端连接
type SSEClient struct {
	id        string
	ip        string
	topics    map[string]bool
	eventCh   chan SSEEvent
	closeCh   chan struct{}
	closed    atomic.Bool
	createdAt time.Time
}

// NewSSEClient 创建新的 SSE 客户端
func NewSSEClient(id, ip string, topics []string) *SSEClient {
	topicSet := make(map[string]bool, len(topics))
	for _, t := range topics {
		topicSet[t] = true
	}
	return &SSEClient{
		id:        id,
		ip:        ip,
		topics:    topicSet,
		eventCh:   make(chan SSEEvent, SSEClientBufferSize),
		closeCh:   make(chan struct{}),
		createdAt: time.Now(),
	}
}

// ID 返回客户端 ID
func (c *SSEClient) ID(ctx context.Context) string { return c.id }

// IP 返回客户端 IP
func (c *SSEClient) IP(ctx context.Context) string { return c.ip }

// Topics 返回订阅的 topics
func (c *SSEClient) Topics(ctx context.Context) []string {
	out := make([]string, 0, len(c.topics))
	for t := range c.topics {
		out = append(out, t)
	}
	return out
}

// IsSubscribed 判断是否订阅了指定 topic
func (c *SSEClient) IsSubscribed(ctx context.Context, topic string) bool {
	return c.topics[topic]
}

// Send 发送事件到客户端缓冲区（非阻塞，缓冲区满时关闭连接）
func (c *SSEClient) Send(ctx context.Context, event SSEEvent) bool {
	if c.closed.Load() {
		return false
	}
	select {
	case c.eventCh <- event:
		return true
	default:
		// 缓冲区满，标记关闭（下次清理时移除）
		return false
	}
}

// Events 返回事件 channel（供 controller 读取）
func (c *SSEClient) Events(ctx context.Context) <-chan SSEEvent {
	return c.eventCh
}

// CloseCh 返回关闭信号 channel
func (c *SSEClient) CloseCh(ctx context.Context) <-chan struct{} {
	return c.closeCh
}

// Close 关闭客户端
func (c *SSEClient) Close(ctx context.Context) {
	if c.closed.CompareAndSwap(false, true) {
		close(c.closeCh)
	}
}

// Closed 判断是否已关闭
func (c *SSEClient) Closed(ctx context.Context) bool {
	return c.closed.Load()
}

// SSEHub SSE 事件总线（管理所有 client 连接）
type SSEHub struct {
	mu      sync.RWMutex
	clients map[string]*SSEClient // client_id -> client
	ipCount map[string]int        // ip -> connection count
	stopCh  chan struct{}
	stopped atomic.Bool
}

// NewSSEHub 创建 SSE Hub
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[string]*SSEClient),
		ipCount: make(map[string]int),
		stopCh:  make(chan struct{}),
	}
}

// Register 注册新客户端
// 返回 nil 表示超过单 IP 连接上限
func (h *SSEHub) Register(ctx context.Context, client *SSEClient) error {
	if client == nil {
		return fmt.Errorf("client is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped.Load() {
		return fmt.Errorf("hub stopped")
	}
	// 检查单 IP 连接数
	if h.ipCount[client.ip] >= SSEMaxConnPerIP {
		return fmt.Errorf("exceeded max connections per IP: %d", SSEMaxConnPerIP)
	}
	if _, exists := h.clients[client.id]; exists {
		return fmt.Errorf("client id already exists: %s", client.id)
	}
	h.clients[client.id] = client
	h.ipCount[client.ip]++
	return nil
}

// Unregister 注销客户端
func (h *SSEHub) Unregister(ctx context.Context, clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	client, ok := h.clients[clientID]
	if !ok {
		return
	}
	delete(h.clients, clientID)
	if _, exists := h.ipCount[client.ip]; exists {
		h.ipCount[client.ip]--
		if h.ipCount[client.ip] <= 0 {
			delete(h.ipCount, client.ip)
		}
	}
	client.Close(context.Background())
}

// Publish 向指定 topic 的所有订阅者广播事件
func (h *SSEHub) Publish(ctx context.Context, event SSEEvent) {
	if h.stopped.Load() {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	h.mu.RLock()
	clients := make([]*SSEClient, 0, len(h.clients))
	for _, c := range h.clients {
		if c.IsSubscribed(context.Background(), event.Topic) {
			clients = append(clients, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range clients {
		if !c.Send(context.Background(), event) {
			// 发送失败（缓冲区满），异步注销
			go h.Unregister(context.Background(), c.id)
		}
	}
}

// GetClientCount 返回当前客户端总数
func (h *SSEHub) GetClientCount(ctx context.Context) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// GetIPCount 返回指定 IP 的连接数
func (h *SSEHub) GetIPCount(ctx context.Context, ip string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ipCount[ip]
}

// GetClient 返回指定 ID 的客户端
func (h *SSEHub) GetClient(ctx context.Context, clientID string) *SSEClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.clients[clientID]
}

// ListClients 列出所有客户端信息（用于管理 API）
func (h *SSEHub) ListClients(ctx context.Context) []map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]map[string]any, 0, len(h.clients))
	for _, c := range h.clients {
		out = append(out, map[string]any{
			"id":         c.id,
			"ip":         c.ip,
			"topics":     c.Topics(context.Background()),
			"created_at": c.createdAt,
			"uptime_sec": int(time.Since(c.createdAt).Seconds()),
		})
	}
	return out
}

// Stop 停止 Hub（关闭所有客户端连接）
func (h *SSEHub) Stop(ctx context.Context) {
	if h.stopped.CompareAndSwap(false, true) {
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, c := range h.clients {
			c.Close(context.Background())
		}
		h.clients = make(map[string]*SSEClient)
		h.ipCount = make(map[string]int)
		close(h.stopCh)
	}
}

// Stopped 返回是否已停止
func (h *SSEHub) Stopped(ctx context.Context) bool {
	return h.stopped.Load()
}

// ===== 全局 SSE Hub 单例 =====

var (
	globalSSEHub     *SSEHub
	globalSSEHubOnce sync.Once
)

// InitGlobalSSEHub 初始化全局 SSE Hub
func InitGlobalSSEHub() *SSEHub {
	globalSSEHubOnce.Do(func() {
		globalSSEHub = NewSSEHub()
	})
	return globalSSEHub
}

// GetGlobalSSEHub 获取全局 SSE Hub
func GetGlobalSSEHub() *SSEHub {
	if globalSSEHub == nil {
		return InitGlobalSSEHub()
	}
	return globalSSEHub
}

// PublishSSEEvent 便捷方法：发布事件到全局 SSE Hub
func PublishSSEEvent(topic, eventType string, data any, traceID string) {
	hub := GetGlobalSSEHub()
	if hub == nil || hub.Stopped(context.Background()) {
		return
	}
	hub.Publish(context.Background(), SSEEvent{
		Topic:     topic,
		EventType: eventType,
		Data:      data,
		TraceID:   traceID,
		Timestamp: time.Now(),
	})
}

// ParseTopics 从 query 参数解析 topics 列表
// topics 格式：topics=llm_calls,intent_recognition
func ParseTopics(topicsStr string) []string {
	if topicsStr == "" {
		return nil
	}
	var topics []string
	for _, t := range splitComma(topicsStr) {
		if IsValidSSETopic(t) {
			topics = append(topics, t)
		}
	}
	return topics
}

// IsValidSSETopic 判断是否合法的 SSE topic
func IsValidSSETopic(topic string) bool {
	switch topic {
	case SSETopicLLMCalls, SSETopicIntentRecogn, SSETopicRAGQueries,
		SSETopicAgentActions, SSETopicHumanizeScores, SSETopicSystemAlerts:
		return true
	}
	return false
}

// splitComma 按逗号分割字符串（去除空白）
func splitComma(s string) []string {
	var out []string
	current := make([]rune, 0, len(s))
	for _, r := range s {
		if r == ',' {
			if len(current) > 0 {
				out = append(out, string(current))
				current = current[:0]
			}
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			continue
		}
		current = append(current, r)
	}
	if len(current) > 0 {
		out = append(out, string(current))
	}
	return out
}

// SSEWriteHeartbeat 写心跳消息（:ping 注释行，不计入 event-stream）
func SSEWriteHeartbeat(c *gin.Context) {
	_, _ = c.Writer.WriteString(":ping\n\n")
	c.Writer.Flush()
}

// SSEWriteEvent 写 SSE 事件
func SSEWriteEvent(c *gin.Context, event SSEEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	// SSE 协议：event: <type>\ndata: <json>\n\n
	_, _ = fmt.Fprintf(c.Writer, "event: %s\n", event.EventType)
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
	c.Writer.Flush()
	return nil
}

// SSEStreamHandler SSE 流处理（在 controller 中调用）
//
// 参数：
//   - c: gin context
//   - hub: SSE Hub 实例
//   - client: 已注册的 SSE 客户端
//
// 行为：
//   - 设置 SSE 响应头
//   - 每 15 秒发送心跳
//   - 监听 client.Events() 推送事件
//   - 客户端断开或 Hub 停止时退出
func SSEStreamHandler(c *gin.Context, hub *SSEHub, client *SSEClient) {
	// 设置 SSE 响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // nginx 不缓冲
	c.Writer.WriteHeader(http.StatusOK)

	// 发送初始连接成功事件
	_ = SSEWriteEvent(c, SSEEvent{
		Topic:     "system_alerts",
		EventType: "connected",
		Data: map[string]any{
			"client_id": client.id,
			"topics":    client.Topics(context.Background()),
			"message":   "SSE connected",
		},
		Timestamp: time.Now(),
	})

	heartbeatTicker := time.NewTicker(SSEHeartbeatInterval)
	defer heartbeatTicker.Stop()

	clientClosed := c.Request.Context().Done()

	for {
		select {
		case <-clientClosed:
			// 客户端断开
			return
		case <-hub.stopCh:
			// Hub 停止
			return
		case <-client.CloseCh(context.Background()):
			// 客户端被关闭
			return
		case event := <-client.Events(context.Background()):
			// 推送事件
			if err := SSEWriteEvent(c, event); err != nil {
				return
			}
		case <-heartbeatTicker.C:
			// 心跳
			SSEWriteHeartbeat(c)
		}
	}
}
