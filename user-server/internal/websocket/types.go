package websocket

import (
	"context"
	"encoding/json"
	"time"
)

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
	MsgTypeSOP           = "sop_message"    // SOP 节点执行事件（新增）
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

// Envelope 信封：所有 WebSocket 下行消息统一外层结构
//
// 设计动机（方向B 鲁棒性加固）：
//   - seq：全局唯一递增序号，用于客户端断点续传 / 排序 / 丢包检测
//   - ts：服务端发送时间戳（Unix 毫秒），便于客户端做时序展示
//   - payload：业务原始负载（type + data），保持向后兼容
//
// 客户端协议：
//   - 重连时发 `{"type":"resume","since_seq":N}`，服务端返回 seq>N 的所有消息
//   - 收到消息后发 `{"type":"ack","seq":N}`，服务端清理待 ACK 队列
type Envelope struct {
	Seq     uint64          `json:"seq,omitempty"`
	TS      int64           `json:"ts,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// AgentSessionExecutor 坐席 WebSocket 操作执行器接口
//
// 设计动机：WebSocket 上行消息（已读 / 接管 / 转接 / 关闭）会触达 CustomerSessionService
// 的领域方法。为避免 websocket 包反向 import service（导致 import cycle），
// 在 web 层（websocket 包）定义本接口，由 service 层提供实现（ws_agent_executor.go）。
//
// 调用方：WSHandler / VisitorWSHandler
// 实现方：service.WSAgentExecutor
//
// sessionID 语义：WebSocket 协议使用业务字符串 session_id（sess_xxx），实现方负责
// 兼容数字主键与业务字符串两种形态。
type AgentSessionExecutor interface {
	// MarkSessionRead 标记会话内消息已读
	MarkSessionRead(ctx context.Context, agentID uint, sessionID string) error
	// TakeoverSession 坐席接管会话
	TakeoverSession(ctx context.Context, agentID uint, sessionID string, reason string) error
	// TransferSession 转接会话给目标坐席
	TransferSession(ctx context.Context, fromAgentID uint, sessionID string, toAgentID uint) error
	// CloseSession 关闭会话
	CloseSession(ctx context.Context, agentID uint, sessionID string) error
}
