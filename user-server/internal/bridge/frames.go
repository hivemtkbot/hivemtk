package bridge

// 协议版本（2026-08-05 P2 大扫除）：
//   - v1: 初始版本（无 v 字段，默认按 v1 处理）
//   - v2: 加 protocol_version 字段、frame 增加 v 字段；保留 v1 向后兼容
//   - 后续破坏性变更必须 bump v+1，并在下方 v1/v2 compatibility shims 段落写清迁移路径
const (
	ProtocolVersionV1 = 1
	ProtocolVersionV2 = 2
	CurrentProtocolVersion = ProtocolVersionV2
)

// 帧类型常量（扩展 <-> 服务器 双向）
const (
	FrameRegister      = "register"       // 扩展上行：注册 channel+account
	FrameInbound       = "inbound_message" // 扩展上行：实时新私信（触发 AI）
	FrameHistory       = "history"        // 扩展上行：历史/回填消息（仅落库，不触发 AI）
	FramePong          = "pong"            // 扩展上行：保活
	FrameAck           = "ack"             // 扩展上行：下行确认
	FramePing          = "ping"            // 服务器下行：保活
	FrameOutboundReply = "outbound_reply"  // 服务器下行：AI 回复
	FrameConfigPush    = "config_push"     // 服务器下行：配置推送
)

// HistoryItem 会话级 history 帧中的单轮消息（点3：一个会话 = 一条消息，内含多轮历史）
type HistoryItem struct {
	EventID    string `json:"event_id,omitempty"`
	SenderType string `json:"sender_type,omitempty"` // customer | agent | self
	SenderID   string `json:"sender_id,omitempty"`
	SenderName string `json:"sender_name,omitempty"`
	ReceiverID string `json:"receiver_id,omitempty"` // 出站时 = 会话对方（统一收信中心聚合键）
	MsgType    string `json:"msg_type"`
	Content    string `json:"content"`
	MediaURL   string `json:"media_url,omitempty"`
	Timestamp  int64  `json:"timestamp"`
	Direction  string `json:"direction,omitempty"` // inbound | outbound（每轮各自方向）
	IsGroup    bool   `json:"is_group,omitempty"`
	GroupID    string `json:"group_id,omitempty"`
	GroupName  string `json:"group_name,omitempty"`
}

// UnifiedMessage 扩展 -> 服务器 上行统一消息（与 user-web/bridge 前端类型对齐）
//
// 注意：inbound_message 与 history 两种上行帧复用本结构（通过 Frame.Type 区分语义）。
// Direction / SenderType 仅在 history 帧中使用（inbound_message 由服务端固定为 inbound）。
// History 非空时为「会话级」帧：一个会话 = 一条消息，History 内含全部多轮（服务端逐条落库）。
type UnifiedMessage struct {
	EventID        string         `json:"event_id"`
	Channel        string         `json:"channel"`
	AccountID      string         `json:"account_id"`
	AgentID        uint           `json:"agent_id,omitempty"`   // 仅 register 帧携带：绑定到该智能体
	AccountName    string         `json:"account_name,omitempty"` // 仅 register 帧携带：账号昵称
	ConversationID string         `json:"conversation_id"`
	SenderID       string         `json:"sender_id"`
	SenderName     string         `json:"sender_name,omitempty"`
	ReceiverID     string         `json:"receiver_id,omitempty"`
	SenderType     string         `json:"sender_type,omitempty"` // customer | agent | self
	MsgType        string         `json:"msg_type"`
	Content        string         `json:"content"`
	MediaURL       string         `json:"media_url,omitempty"`
	Timestamp      int64          `json:"timestamp"`
	Direction      string         `json:"direction,omitempty"` // inbound | outbound（仅 history 帧）
	IsGroup        bool           `json:"is_group,omitempty"`  // 群聊消息
	GroupID        string         `json:"group_id,omitempty"`  // 群 ID（群聊会话聚合键）
	GroupName      string         `json:"group_name,omitempty"` // 群名
	History        []*HistoryItem `json:"history,omitempty"`   // 会话级多轮历史（点3）
	Extra          map[string]any `json:"extra,omitempty"`
}

// UnifiedReply 服务器 -> 扩展 下行统一回复
//
// Truncated 字段（2026-08-05 审计 P0 修复）：
//   - 服务端 deliverWS 截断 content 到 4KB 后置 Truncated=true
//   - 扩展端可据此在 UI 显示"消息被截断"提示，避免用户看到半截消息不知情
//   - omitempty：未截断时不发送该字段，老扩展无需升级即可兼容
type UnifiedReply struct {
	Channel        string `json:"channel"`
	AccountID      string `json:"account_id"`
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
	MsgType        string `json:"msg_type"`
	MediaURL       string `json:"media_url,omitempty"`
	ReplyToEventID string `json:"reply_to_event_id,omitempty"`
	Truncated      bool   `json:"truncated,omitempty"` // 内容被截断标记（4KB 上限触发）
}

// MessageEventID 透传 UnifiedMessage.EventID（便于日志/去重）。
//
// 命名变更（2026-08-05 P0 修复）：原方法名 EventID() 与 Frame.EventID 字段同名冲突，
// 重命名为 MessageEventID()。旧代码 f.EventID() 全部替换为 f.MessageEventID()，行为不变。
func (f *Frame) MessageEventID() string {
	if f == nil || f.Message == nil {
		return ""
	}
	return f.Message.EventID
}

// ProtocolVersion 透传 Frame.V（缺省按 v1 处理，向后兼容）
func (f *Frame) ProtocolVersion() int {
	if f == nil || f.V == 0 {
		return ProtocolVersionV1 // 缺省 = v1（旧扩展无 v 字段）
	}
	return f.V
}

// Frame 通用帧
//
// 字段说明：
//   - V: 协议版本（2026-08-05 新增）；缺省按 v1 处理，老扩展无需升级即可兼容
//   - Type/Channel/AccountID/Seq: 见帧类型常量
//   - EventID: 帧级事件 ID（用于下行 ack 关联上行 event_id，omitempty 老扩展忽略）
//   - TraceID: 全链路追踪 ID（用于 H1+C7 端到端透传，omitempty 老扩展忽略）
//   - Message: 上行消息（inbound_message / history / register 帧携带）
//   - Reply: 下行回复（outbound_reply 帧携带）
type Frame struct {
	V         int             `json:"v,omitempty"` // 协议版本（缺省 = 1）
	Type      string          `json:"type"`
	Channel   string          `json:"channel,omitempty"`
	AccountID string          `json:"account_id,omitempty"`
	Seq       int64           `json:"seq,omitempty"`
	EventID   string          `json:"event_id,omitempty"` // 帧级事件 ID（ack 关联用）
	TraceID   string          `json:"trace_id,omitempty"` // 端到端 trace ID（h1+c7 透传）
	Message   *UnifiedMessage `json:"message,omitempty"`
	Reply     *UnifiedReply   `json:"reply,omitempty"`
}
