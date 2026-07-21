package websocket

import "sync"

// visitorClients 跟踪访客客户端（sessionID -> Client）
var (
	visitorClientsMu sync.RWMutex
	visitorClients   = make(map[string]*Client)
)

// RegisterVisitorClient 注册访客客户端（导出，供跨包集成测试模拟前端在线）
func RegisterVisitorClient(c *Client) {
	registerVisitorClient(c)
}

// registerVisitorClient 注册访客客户端
func registerVisitorClient(c *Client) {
	visitorClientsMu.Lock()
	defer visitorClientsMu.Unlock()
	visitorClients[c.sessionID] = c
}

// UnregisterVisitorClient 注销访客客户端（导出，供跨包集成测试）
func UnregisterVisitorClient(c *Client) {
	unregisterVisitorClient(c)
}

// unregisterVisitorClient 注销访客客户端
func unregisterVisitorClient(c *Client) {
	visitorClientsMu.Lock()
	defer visitorClientsMu.Unlock()
	if existing, ok := visitorClients[c.sessionID]; ok && existing == c {
		delete(visitorClients, c.sessionID)
	}
}

// getVisitorClient 获取访客客户端
func getVisitorClient(sessionID string) *Client {
	visitorClientsMu.RLock()
	defer visitorClientsMu.RUnlock()
	return visitorClients[sessionID]
}

// IsVisitorOnline 判断访客是否在线（存在活跃 WebSocket 连接）
// 用于发送坐席/AI 回复时区分实时推送与离线补发，避免重复投递
func IsVisitorOnline(sessionID string) bool {
	return getVisitorClient(sessionID) != nil
}
