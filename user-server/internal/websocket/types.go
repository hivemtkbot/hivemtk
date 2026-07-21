package websocket

import "time"

// ClientType 客户端类型
type ClientType string

const (
	ClientTypeAgent   ClientType = "agent"   // 客服坐席
	ClientTypeVisitor ClientType = "visitor" // 网站访客
)

// MsgType WebSocket 消息类型常量
//
// 统一管理各业务模块推送的消息类型字符串，
// 避免散落在调用方导致命名不一致。
const (
	MsgTypeNewSession    = "new_session"    // 新会话通知
	MsgTypeNewMessage    = "new_message"    // 新消息通知
	MsgTypeSessionUpdate = "session_update" // 会话状态更新
	MsgTypeAgentStatus   = "agent_status"   // 客服状态变更
	MsgTypeAISuggestion  = "ai_suggestion"  // AI 建议推送
	MsgTypeHeartbeat     = "heartbeat"      // 心跳
	MsgTypeSOP           = "sop_message"    // SOP 节点执行事件（P0-1 新增）
	MsgTypeError         = "error"          // 错误通知
)

// WebSocket 连接超时与心跳常量（gorilla/websocket 官方推荐模式）
//
// 设计依据：
//   - pongWait：等待对端 Pong 的最大间隔；超过即认为连接僵死，主动关闭。
//     客户端需在 pongWait 内回 Pong 或发消息以维持连接。
//   - pingPeriod：服务端发送 Ping 的周期；必须严格小于 pongWait，
//     否则会出现 Ping 尚未发出、ReadDeadline 已超时的死锁。
//   - writeWait：单次写操作（含 Ping/Pong/普通消息）的超时，
//     防止对端 TCP 窗口关闭时本协程永久阻塞。
//   - maxMessageSize：单条上行消息最大字节数，防止恶意大帧耗尽内存。
const (
	pongWait       = 60 * time.Second    // 等待 Pong 的最大时间
	pingPeriod     = (pongWait * 9) / 10 // Ping 发送周期（54s，小于 pongWait）
	writeWait      = 10 * time.Second    // 单次写操作超时
	maxMessageSize = 8192                // 单条消息最大字节数
)
