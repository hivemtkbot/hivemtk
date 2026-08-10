package browser

import (
	"encoding/json"
	"fmt"
	"hivemtk-user/internal/pkg/utils/logger"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// XianyuWSMessage 闲鱼 WebSocket 消息结构
type XianyuWSMessage struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

// XianyuChatMessage 闲鱼聊天消息
type XianyuChatMessage struct {
	MessageID  string `json:"messageId"`
	ChatID     string `json:"chatId"`
	SenderID   string `json:"senderId"`
	SenderName string `json:"senderName"`
	Content    string `json:"content"`
	MsgType    string `json:"msgType"` // text / image / system
	Timestamp  int64  `json:"timestamp"`
	IsMine     bool   `json:"isMine"` // 是否是自己发的
}

// XianyuWebSocket 闲鱼 WebSocket 客户端
type XianyuWebSocket struct {
	conn           *websocket.Conn
	url            string
	headers        http.Header
	msgChan        chan XianyuChatMessage // 消息通道
	errChan        chan error
	done           chan struct{}
	mu             sync.Mutex
	isActive       bool
	reconnectDelay time.Duration
	maxReconnect   int
	onMessage      func(XianyuChatMessage) // 消息回调
}

// XianyuWSConfig WebSocket 配置
type XianyuWSConfig struct {
	URL            string
	Token          string
	Cookie         string
	UserAgent      string
	ReconnectDelay time.Duration
	MaxReconnect   int
	OnMessage      func(XianyuChatMessage)
}

// NewXianyuWebSocket 创建闲鱼 WebSocket 客户端
func NewXianyuWebSocket(cfg XianyuWSConfig) *XianyuWebSocket {
	if cfg.ReconnectDelay == 0 {
		cfg.ReconnectDelay = 3 * time.Second
	}
	if cfg.MaxReconnect == 0 {
		cfg.MaxReconnect = 5
	}

	headers := http.Header{}
	headers.Set("Cookie", cfg.Cookie)
	headers.Set("User-Agent", cfg.UserAgent)
	headers.Set("Origin", "https://www.goofish.com")

	return &XianyuWebSocket{
		url:            cfg.URL,
		headers:        headers,
		msgChan:        make(chan XianyuChatMessage, 100),
		errChan:        make(chan error, 10),
		done:           make(chan struct{}),
		reconnectDelay: cfg.ReconnectDelay,
		maxReconnect:   cfg.MaxReconnect,
		onMessage:      cfg.OnMessage,
	}
}

// Connect 连接到闲鱼 WebSocket
func (xw *XianyuWebSocket) Connect() error {
	xw.mu.Lock()
	defer xw.mu.Unlock()

	if xw.isActive {
		return fmt.Errorf("WebSocket 已连接")
	}

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(xw.url, xw.headers)
	if err != nil {
		return fmt.Errorf("WebSocket 连接失败: %w", err)
	}

	xw.conn = conn
	xw.isActive = true

	logger.Info("[闲鱼WS] 连接成功")
	go xw.readLoop()
	go xw.pingLoop()

	return nil
}

// readLoop 读取消息循环
func (xw *XianyuWebSocket) readLoop() {
	reconnectCount := 0

	for {
		select {
		case <-xw.done:
			return
		default:
		}

		xw.mu.Lock()
		conn := xw.conn
		xw.mu.Unlock()

		if conn == nil {
			// 尝试重连
			if reconnectCount >= xw.maxReconnect {
				logger.Warn("[闲鱼WS] 超过最大重连次数，停止重连")
				xw.errChan <- fmt.Errorf("max reconnect reached")
				return
			}
			reconnectCount++
			logger.Infof("[闲鱼WS] 连接断开，%d/%d 次重连...", reconnectCount, xw.maxReconnect)
			time.Sleep(xw.reconnectDelay)

			xw.mu.Lock()
			dialer := websocket.DefaultDialer
			newConn, _, err := dialer.Dial(xw.url, xw.headers)
			if err != nil {
				logger.Errorf("[闲鱼WS] 重连失败: %v", err)
				xw.mu.Unlock()
				continue
			}
			xw.conn = newConn
			conn = newConn
			reconnectCount = 0
			logger.Info("[闲鱼WS] 重连成功")
			xw.mu.Unlock()
		}

		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			logger.Errorf("[闲鱼WS] 读取消息失败: %v", err)
			xw.mu.Lock()
			xw.conn.Close()
			xw.conn = nil
			xw.mu.Unlock()
			continue
		}

		// 解析消息
		var wsMsg XianyuWSMessage
		if err := json.Unmarshal(rawMsg, &wsMsg); err != nil {
			logger.Errorf("[闲鱼WS] 解析消息失败: %v, raw=%s", err, string(rawMsg))
			continue
		}

		// 只处理聊天消息
		if wsMsg.Type == "chat_message" || wsMsg.Type == "message" {
			var chatMsg XianyuChatMessage
			if err := json.Unmarshal(wsMsg.Data, &chatMsg); err != nil {
				logger.Errorf("[闲鱼WS] 解析聊天消息失败: %v", err)
				continue
			}

			// 忽略自己发送的消息
			if chatMsg.IsMine {
				continue
			}

			// 推送到消息通道
			select {
			case xw.msgChan <- chatMsg:
				logger.Infof("[闲鱼WS] 收到消息: sender=%s content=%s", chatMsg.SenderName, truncate(chatMsg.Content, 50))
				if xw.onMessage != nil {
					xw.onMessage(chatMsg)
				}
			default:
				logger.Warn("[闲鱼WS] 消息通道已满，丢弃消息")
			}
		}
	}
}

// pingLoop 心跳保持
func (xw *XianyuWebSocket) pingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-xw.done:
			return
		case <-ticker.C:
			xw.mu.Lock()
			if xw.conn != nil {
				if err := xw.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					logger.Errorf("[闲鱼WS] 心跳发送失败: %v", err)
				}
			}
			xw.mu.Unlock()
		}
	}
}

// Receive 接收消息 (阻塞)
func (xw *XianyuWebSocket) Receive() <-chan XianyuChatMessage {
	return xw.msgChan
}

// Errors 错误通道
func (xw *XianyuWebSocket) Errors() <-chan error {
	return xw.errChan
}

// Close 关闭连接
func (xw *XianyuWebSocket) Close() {
	xw.mu.Lock()
	defer xw.mu.Unlock()

	close(xw.done)

	if xw.conn != nil {
		xw.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client closing"))
		xw.conn.Close()
		xw.conn = nil
	}

	xw.isActive = false
	logger.Info("[闲鱼WS] 连接已关闭")
}

// IsActive 连接状态
func (xw *XianyuWebSocket) IsActive() bool {
	xw.mu.Lock()
	defer xw.mu.Unlock()
	return xw.isActive
}

// SendMessage 发送消息 (供外部使用)
func (xw *XianyuWebSocket) SendMessage(chatID, content string, msgType string) error {
	xw.mu.Lock()
	defer xw.mu.Unlock()

	if xw.conn == nil {
		return fmt.Errorf("WebSocket 未连接")
	}

	msg := map[string]any{
		"type": "send_message",
		"data": map[string]any{
			"chatId":  chatID,
			"content": content,
			"msgType": msgType,
		},
	}

	raw, _ := json.Marshal(msg)
	return xw.conn.WriteMessage(websocket.TextMessage, raw)
}
