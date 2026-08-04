package bridge

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

// UnifiedMessage 扩展 -> 服务器 上行统一消息（与 user-web/bridge 前端类型对齐）
//
// 注意：inbound_message 与 history 两种上行帧复用本结构（通过 Frame.Type 区分语义）。
// Direction / SenderType 仅在 history 帧中使用（inbound_message 由服务端固定为 inbound）。
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
	Extra          map[string]any `json:"extra,omitempty"`
}

// UnifiedReply 服务器 -> 扩展 下行统一回复
type UnifiedReply struct {
	Channel        string `json:"channel"`
	AccountID      string `json:"account_id"`
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
	MsgType        string `json:"msg_type"`
	MediaURL       string `json:"media_url,omitempty"`
	ReplyToEventID string `json:"reply_to_event_id,omitempty"`
}

// EventID 透传 UnifiedMessage.EventID（便于日志/去重）
func (f *Frame) EventID() string {
	if f == nil || f.Message == nil {
		return ""
	}
	return f.Message.EventID
}

// Frame 通用帧
type Frame struct {
	Type      string          `json:"type"`
	Channel   string          `json:"channel,omitempty"`
	AccountID string          `json:"account_id,omitempty"`
	Seq       int64           `json:"seq,omitempty"`
	Message   *UnifiedMessage `json:"message,omitempty"`
	Reply     *UnifiedReply   `json:"reply,omitempty"`
}
