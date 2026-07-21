package websocket

import (
	"encoding/json"
	"strconv"
)

// 消息类型常量（统一管理）
const (
	TypeNewSession    = "new_session"
	TypeNewMessage    = "new_message"
	TypeSessionUpdate = "session_update"
	TypeAgentStatus   = "agent_status"
	TypeAISuggestion  = "ai_suggestion"
	TypeHeartbeat     = "heartbeat"

	// 访客端下行消息类型
	TypeWelcome         = "welcome"          // 接入欢迎
	TypeMessage         = "message"          // AI/坐席消息
	TypeAgentJoined     = "agent_joined"     // 坐席接管
	TypeSessionClosed   = "session_closed"   // 会话关闭
	TypeAITyping        = "ai_typing"        // AI 正在输入
	TypeError           = "error"            // 错误通知
	TypePong            = "pong"             // 心跳响应
	TypeOfflineMessages = "offline_messages" // 离线消息补发
)

// SendToAgent 发送消息给指定客服（坐席 ID 为 uint）
func SendToAgent(messageType string, payload any, agentID uint) error {
	agentIDStr := strconv.FormatUint(uint64(agentID), 10)
	return GetHub().SendToAgent(agentIDStr, messageType, payload)
}

// SendToVisitor 发送消息给指定会话的访客
// 通过 sessionID 路由到对应的访客 WebSocket 连接
func SendToVisitor(messageType string, payload any, sessionID string) error {
	return GetHub().SendToVisitor(sessionID, messageType, payload)
}

// BroadcastToAll 广播给所有客户端（坐席 + 访客）
func BroadcastToAll(messageType string, payload any) error {
	return GetHub().BroadcastToAll(messageType, payload)
}

// BroadcastToAgents 广播给所有坐席（排除访客）
func BroadcastToAgents(messageType string, payload any) error {
	return GetHub().BroadcastToAgents(messageType, payload)
}

// ============================================================================
// Hub 方法扩展（SendToVisitor / BroadcastToAll / BroadcastToAgents）
// ============================================================================

// SendToVisitor 发送消息给指定会话的访客
func (h *Hub) SendToVisitor(sessionID, messageType string, payload any) error {
	if sessionID == "" {
		return nil
	}
	payloadBytes, err := json.Marshal(map[string]any{
		"type":    messageType,
		"payload": payload,
	})
	if err != nil {
		return err
	}

	visitor := getVisitorClient(sessionID)
	if visitor == nil {
		// 访客离线，消息暂存待重连拉取
		return nil
	}

	select {
	case visitor.send <- payloadBytes:
	default:
		// channel 满，丢弃
	}
	return nil
}

// BroadcastToAll 广播给所有客户端
func (h *Hub) BroadcastToAll(messageType string, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	h.mu.RLock()
	for _, client := range h.clients {
		select {
		case client.send <- payloadBytes:
		default:
		}
	}
	h.mu.RUnlock()

	// 访客客户端
	visitorClientsMu.RLock()
	for _, client := range visitorClients {
		select {
		case client.send <- payloadBytes:
		default:
		}
	}
	visitorClientsMu.RUnlock()

	return nil
}

// BroadcastToAgents 广播给所有坐席（不含访客）
func (h *Hub) BroadcastToAgents(messageType string, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, client := range h.clients {
		if client.IsVisitor() {
			continue
		}
		select {
		case client.send <- payloadBytes:
		default:
		}
	}
	return nil
}
