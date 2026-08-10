// Package channelgw 渠道网关：统一收件箱的唯一入站协议层与传输抽象。
//
// 架构定位（2026-08-10 统一收件箱 × Bridge 整合）：
//
//	渠道客户端（HTTP 扩展 / WS 客户端 / webhook bot）
//	      │  唯一线路协议 channelgw.IngestMessage
//	      ▼
//	channelgw（本包）
//	 ├─ protocol.go  统一协议类型 + 规范化转换器（ToEvent / HistoryToEvent）
//	 ├─ registry.go  渠道注册表（每渠道声明支持的传输 http/websocket）
//	 ├─ pipeline.go  IngressPipeline 接口（封装 InboxIngressService）
//	 └─ ws.go        WebSocket 传输（与 HTTP 三通道共享同一管道与下发队列）
//	      ▼
//	InboxIngressService（去重 / 人工接管锁 / 落库 / AI 触发）← 所有渠道唯一入站管道
//	      ▼
//	message_hub + inbox_conversations → 统一收件箱
//
// 设计原则：
//  1. 协议单源：本包类型是渠道侧与中台之间的唯一线路协议；
//     bridge 包的 UnifiedMessage / HTTPIngestMessage 等均为本包类型的别名，
//     消除多套结构重复定义与散落转换函数。
//  2. 传输无关：入站统一收敛为 []*model.MessageEvent 交给 IngressPipeline；
//     HTTP（请求-响应）与 WebSocket（帧流）仅是同一管道的两种传输呈现。
//  3. 下发单源：message_hub(direction=outbound, status=pending) 是唯一事实源，
//     HTTP 传输拉取（GET /api/bridge/outbox），WS 传输推帧（outbound_reply），
//     两者共用 ClaimPendingOutbound 权威认领，断线未 ack 由惰性回收重回 pending。
package channelgw

import (
	"strings"
	"time"

	"hivemtk-user/internal/model"
)

// 协议版本：
//   - v1: 初始版本（无 v 字段，默认按 v1 处理）
//   - v2: 加 protocol_version 字段、frame 增加 v 字段；保留 v1 向后兼容
//   - 后续破坏性变更必须 bump v+1，并写清迁移路径
const (
	ProtocolVersionV1      = 1
	ProtocolVersionV2      = 2
	CurrentProtocolVersion = ProtocolVersionV2
)

// 帧类型常量（渠道客户端 <-> 服务器 双向，HTTP 与 WS 传输共用同一套语义）。
const (
	FrameRegister       = "register"          // 上行：注册 channel+account（WS 首帧）
	FrameRegisterReject = "register_rejected" // 下行：注册被拒（Reason 携带原因）
	FrameRegistered     = "registered"        // 下行：注册成功确认
	FrameInbound        = "inbound_message"   // 上行：实时新私信（触发 AI）
	FrameHistory        = "history"           // 上行：历史/回填消息（仅落库，不触发 AI）
	FramePong           = "pong"              // 上行/下行：保活应答
	FrameAck            = "ack"               // 上行：下行确认（msg_ids）；下行：上行处理结果回执
	FramePing           = "ping"              // 上行/下行：保活
	FrameOutboundReply  = "outbound_reply"    // 下行：AI 回复（WS 推送）
	FrameConfigPush     = "config_push"       // 下行：配置推送
)

// 传输类型标识（渠道注册表声明每渠道支持的传输）。
type Transport string

const (
	TransportHTTP      Transport = "http"      // 请求-响应（ingest/outbox/ack 三通道）
	TransportWebSocket Transport = "websocket" // 帧流（register/inbound/ack + 出站推帧）
)

// HistoryItem 会话级 history 中的单轮消息（一个会话 = 一条消息，内含多轮历史）。
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

// IngestMessage 渠道 -> 服务器 上行统一消息（唯一线路协议）。
//
// 合并了历史三套结构（bridge.UnifiedMessage / bridge.HTTPIngestMessage /
// channelbot core.InboundMessage 的服务端侧对应物），字段名与 JSON tag
// 与既有前端（user-web/bridge types.js）严格兼容。
//
// 语义说明：
//   - inbound_message 与 history 两种上行帧复用本结构（通过 Frame.Type 区分语义）
//   - Direction / SenderType 仅在 history 场景使用（实时入站由服务端固定为 inbound）
//   - SenderType 仅供前端参考；服务端在入库环节按「内容是否命中本会话平台下发
//     (outbound)」权威重判自/他（见 InboxIngressService.isPlatformOutboundEcho）
//   - History 非空时为「会话级」帧：一个会话 = 一条消息，History 内含全部多轮
//   - ContentHash 前端按与服务端 ContentHashMsgID 同源算法生成（mh: 前缀 FNV-1a），
//     用作回环去重钩子2 的兜底依据
type IngestMessage struct {
	EventID        string         `json:"event_id,omitempty"`
	Channel        string         `json:"channel,omitempty"`
	AccountID      string         `json:"account_id,omitempty"`
	AgentID        uint           `json:"agent_id,omitempty"`     // 仅 register 帧携带：绑定到该智能体
	AccountName    string         `json:"account_name,omitempty"` // 仅 register 帧携带：账号昵称
	ConversationID string         `json:"conversation_id"`
	SenderID       string         `json:"sender_id"`
	SenderName     string         `json:"sender_name,omitempty"`
	ReceiverID     string         `json:"receiver_id,omitempty"`
	SenderType     string         `json:"sender_type,omitempty"` // customer | agent | self | system（服务端不采信）
	MsgType        string         `json:"msg_type"`
	Content        string         `json:"content"`
	MediaURL       string         `json:"media_url,omitempty"`
	Timestamp      int64          `json:"timestamp"`
	Direction      string         `json:"direction,omitempty"` // inbound | outbound（仅 history 帧）
	IsGroup        bool           `json:"is_group,omitempty"`  // 群聊消息
	GroupID        string         `json:"group_id,omitempty"`  // 群 ID（群聊会话聚合键）
	GroupName      string         `json:"group_name,omitempty"`
	History        []*HistoryItem `json:"history,omitempty"` // 会话级多轮历史
	Extra          map[string]any `json:"extra,omitempty"`
	ContentHash    string         `json:"content_hash,omitempty"`
}

// IngestRequest HTTP 上报请求体（与扩展端 http-ingest.js 严格对齐）。
type IngestRequest struct {
	V              int              `json:"v,omitempty"` // 协议版本（缺省 = 1）
	Channel        string           `json:"channel"`
	AccountID      string           `json:"account_id"`
	ConversationID string           `json:"conversation_id,omitempty"`
	Messages       []*IngestMessage `json:"messages"`
	ExpectReply    bool             `json:"expect_reply,omitempty"` // 是否长轮询等待 AI 回复（v2 已废弃，保留兼容）
	TimeoutMs      int              `json:"timeout_ms,omitempty"`   // 期望长轮询时长（毫秒，同上）
	AccountName    string           `json:"account_name,omitempty"` // 账号昵称（首次注册用）
	AgentID        uint             `json:"agent_id,omitempty"`     // 绑定到该智能体（首次注册用）
	// InternalOnly 内部调试字段：跳过 AI 触发（仅落库），用于 e2e 测试
	InternalOnly bool `json:"internal_only,omitempty"`
}

// IngestResult 单条消息处理结果（HTTP 响应与 WS ack 帧共用）。
type IngestResult struct {
	EventID   string `json:"event_id"`
	Accepted  bool   `json:"accepted"`
	Duplicate bool   `json:"duplicate,omitempty"`  // 幂等跳过 / 中间件拦截（重复投递）
	AIHandled bool   `json:"ai_handled,omitempty"` // 已触发 AI 推理
	Reason    string `json:"reason,omitempty"`
}

// IngestResponse HTTP 上报响应。
type IngestResponse struct {
	OK              bool             `json:"ok"`
	Ingested        []*IngestResult  `json:"ingested"`                   // 每条消息处理结果
	OutboundReplies []*OutboundReply `json:"outbound_replies,omitempty"` // AI 回复（v2 即时返回已废弃，保留兼容）
	SessionID       string           `json:"session_id,omitempty"`
	Reason          string           `json:"reason,omitempty"`
	ServerTime      int64            `json:"server_time"` // 服务端处理完时间（毫秒）
}

// OutboundReply 服务器 -> 渠道 下行统一回复（WS outbound_reply 帧载荷）。
//
// Truncated：服务端截断 content 到 4KB 后置 true，客户端可据此提示「消息被截断」。
type OutboundReply struct {
	Channel        string `json:"channel"`
	AccountID      string `json:"account_id"`
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
	MsgType        string `json:"msg_type"`
	MediaURL       string `json:"media_url,omitempty"`
	ReplyToEventID string `json:"reply_to_event_id,omitempty"`
	Truncated      bool   `json:"truncated,omitempty"`
}

// OutboxMessage 下发队列中的一条待发消息（HTTP GET /api/bridge/outbox 与 WS 推帧共用序列化）。
type OutboxMessage struct {
	MsgID          string    `json:"msg_id"`
	ConversationID string    `json:"conversation_id"`
	MsgType        string    `json:"msg_type"`
	Content        string    `json:"content"`
	MediaURL       string    `json:"media_url"`
	SenderID       string    `json:"sender_id"`
	ReceiverID     string    `json:"receiver_id"`
	IsAIReply      bool      `json:"is_ai_reply"`
	CreatedAt      time.Time `json:"created_at"`
}

// Frame 通用帧（WS 传输线路格式；HTTP 传输使用 IngestRequest/Response）。
//
// 字段说明：
//   - V: 协议版本；缺省按 v1 处理，老客户端无需升级即可兼容
//   - Message: 单条上行消息（inbound_message / history / register 帧）
//   - Messages: 批量上行消息（inbound_message 帧可选，与 Message 合并处理）
//   - Reply: 下行回复（outbound_reply 帧载荷），MsgID 为下发队列关联键（ack 回写用）
//   - MsgIDs/Status: ack 帧（客户端确认下发 delivered）
//   - Results: ack 帧（服务端回执上行处理结果）
type Frame struct {
	V         int              `json:"v,omitempty"`
	Type      string           `json:"type"`
	Channel   string           `json:"channel,omitempty"`
	AccountID string           `json:"account_id,omitempty"`
	Seq       int64            `json:"seq,omitempty"`
	EventID   string           `json:"event_id,omitempty"` // 帧级事件 ID（ack 关联用）
	TraceID   string           `json:"trace_id,omitempty"` // 端到端 trace ID
	MsgID     string           `json:"msg_id,omitempty"`   // 下行关联键（outbound_reply 帧 = message_hub.msg_id）
	Message   *IngestMessage   `json:"message,omitempty"`
	Messages  []*IngestMessage `json:"messages,omitempty"`
	Reply     *OutboundReply   `json:"reply,omitempty"`
	MsgIDs    []string         `json:"msg_ids,omitempty"` // ack 帧：确认 delivered 的 msg_id 列表
	Status    string           `json:"status,omitempty"`  // ack 帧状态（delivered）
	Results   []*IngestResult  `json:"results,omitempty"` // ack 帧：上行处理结果（与上行 messages 索引对齐）
	Reason    string           `json:"reason,omitempty"`  // 注册拒绝 / 错误原因
}

// MessageEventID 透传帧内单条上行消息的 EventID（便于日志/去重）。
func (f *Frame) MessageEventID() string {
	if f == nil || f.Message == nil {
		return ""
	}
	return f.Message.EventID
}

// ProtocolVersion 透传 Frame.V（缺省按 v1 处理，向后兼容）。
func (f *Frame) ProtocolVersion() int {
	if f == nil || f.V == 0 {
		return ProtocolVersionV1 // 缺省 = v1（旧客户端无 v 字段）
	}
	return f.V
}

// ───────────────────────── 规范化转换器（所有传输共用） ─────────────────────────

// ToEvent 将上行消息转换为消息中台的 model.MessageEvent（不含 History 拷贝）。
//
// transport 标记来源传输（"http" / "websocket"），写入 Extra["transport"] 供可观测。
// History 字段不在此拷贝：HTTP/WS 传输均由传输层先逐条 PersistHistory 落库，
// 实时消息事件再入批处理管道，避免双重落库（与历史 handler 行为严格一致）。
func (m *IngestMessage) ToEvent(transport string) *model.MessageEvent {
	if m == nil {
		return nil
	}
	ts := time.UnixMilli(m.Timestamp)
	if m.Timestamp == 0 {
		ts = time.Now()
	}
	ev := &model.MessageEvent{
		EventID:        m.EventID,
		SessionID:      m.Channel + ":" + m.AccountID + ":" + m.ConversationID,
		Channel:        m.Channel,
		SenderID:       m.SenderID,
		SenderName:     m.SenderName,
		SenderType:     m.SenderType,
		ReceiverID:     m.ReceiverID,
		MsgType:        m.MsgType,
		Content:        m.Content,
		MediaURL:       m.MediaURL,
		ConversationID: m.ConversationID,
		IsGroup:        m.IsGroup,
		GroupID:        m.GroupID,
		Timestamp:      ts,
		Extra: map[string]any{
			"account_id":  m.AccountID,
			"bridge":      true,
			"sender_type": m.SenderType,
		},
	}
	if transport != "" {
		ev.Extra["transport"] = transport
	}
	if m.ContentHash != "" {
		ev.Extra["content_hash"] = m.ContentHash
	}
	if m.IsGroup {
		ev.Extra["is_group"] = true
	}
	if m.GroupID != "" {
		ev.Extra["group_id"] = m.GroupID
	}
	if m.GroupName != "" {
		ev.Extra["group_name"] = m.GroupName
	}
	return ev
}

// ToEventFull 与 ToEvent 相同，但额外把 History 拷贝到 MessageEvent.History
// 并冗余到 Extra["history"]（供统一收件箱展示/可观测；兼容旧统一收信中心读取路径）。
func (m *IngestMessage) ToEventFull(transport string) *model.MessageEvent {
	ev := m.ToEvent(transport)
	if ev == nil || len(m.History) == 0 {
		return ev
	}
	hist := make([]model.MessageEventHistoryItem, 0, len(m.History))
	for _, it := range m.History {
		if it == nil {
			continue
		}
		hist = append(hist, model.MessageEventHistoryItem{
			EventID:    it.EventID,
			SenderType: it.SenderType,
			SenderID:   it.SenderID,
			SenderName: it.SenderName,
			ReceiverID: it.ReceiverID,
			MsgType:    it.MsgType,
			Content:    it.Content,
			MediaURL:   it.MediaURL,
			Timestamp:  it.Timestamp,
			Direction:  it.Direction,
			IsGroup:    it.IsGroup,
			GroupID:    it.GroupID,
			GroupName:  it.GroupName,
		})
	}
	ev.History = hist
	ev.Extra["history"] = hist
	return ev
}

// HistoryToEvent 把会话级 history 中的单轮（HistoryItem）映射为 model.MessageEvent。
// 会话元数据（channel/account/conversation/群）取自帧顶层消息 parent，轮次字段取自 item。
func HistoryToEvent(parent *IngestMessage, it *HistoryItem) *model.MessageEvent {
	if parent == nil || it == nil {
		return nil
	}
	ts := time.UnixMilli(it.Timestamp)
	if it.Timestamp == 0 {
		ts = time.Now()
	}
	ev := &model.MessageEvent{
		EventID:        it.EventID,
		SessionID:      parent.Channel + ":" + parent.AccountID + ":" + parent.ConversationID,
		Channel:        parent.Channel,
		SenderID:       it.SenderID,
		SenderName:     it.SenderName,
		ReceiverID:     firstNonEmpty(it.ReceiverID, parent.ReceiverID),
		MsgType:        it.MsgType,
		Content:        it.Content,
		MediaURL:       it.MediaURL,
		ConversationID: parent.ConversationID,
		IsGroup:        it.IsGroup || parent.IsGroup,
		GroupID:        firstNonEmpty(it.GroupID, parent.GroupID),
		Timestamp:      ts,
		Extra: map[string]any{
			"account_id":  parent.AccountID,
			"bridge":      true,
			"sender_type": firstNonEmpty(it.SenderType, parent.SenderType),
		},
	}
	// 出站轮次 receiver_id 兜底：旧版客户端未填时，统一收信中心仍能按「对方」聚合。
	if ev.ReceiverID == "" && it.Direction == "outbound" {
		ev.ReceiverID = parent.ConversationID
	}
	if ev.IsGroup {
		ev.Extra["is_group"] = true
	}
	if ev.GroupID != "" {
		ev.Extra["group_id"] = ev.GroupID
	}
	if groupName := firstNonEmpty(it.GroupName, parent.GroupName); groupName != "" {
		ev.Extra["group_name"] = groupName
	}
	return ev
}

// IsDuplicateReason 判断入站处理结果原因是否属于「已被服务端幂等/拦截确认为重复」。
// 命中则传输层把该 event_id 标记 Duplicate，客户端据此停止重发（允许重复上报，
// 服务端用 ack 确认去重）。
func IsDuplicateReason(reason string) bool {
	if reason == "" {
		return false
	}
	for _, kw := range []string{
		"msg_id already exists", // 钩子2：msg_id 已落库（幂等跳过）
		"intercepted",           // 统一收件中间件拦截（回环回显 / 短时重复）
		"echo",                  // 自/他回显
		"duplicate",             // 重复投递
		"skip",                  // 跳过
		"already exists",        // 兜底：任意已存在
	} {
		if strings.Contains(reason, kw) {
			return true
		}
	}
	return false
}

// firstNonEmpty 返回第一个非空字符串（v 为空时回退 def）。
func firstNonEmpty(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
