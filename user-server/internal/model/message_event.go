package model

import "time"

// 渠道标识（与消息中台 platform 白名单对齐）
//
// 2026-08-05 渠道编码统一：所有渠道统一为全名，去掉 xhs / *_web 等简写/后缀
// （历史曾并存 xhs / xhs_web / xiaohongshu 三种命名，导致前端、后端、DB
// 数据三方不一致；现已统一为以下 5 套：xiaohongshu / douyin / kuaishou / xianyu / tiktok）。
const (
	ChannelWeb       = "web"
	ChannelTelegram  = "telegram"
	ChannelWhatsApp  = "whatsapp"
	ChannelXHS       = "xiaohongshu" // 历史值 "xhs" / "xhs_web" 已统一为全名
	ChannelWeCom     = "wecom"
	ChannelXianyu    = "xianyu"
	ChannelDouyin    = "douyin"
	ChannelKuaishou  = "kuaishou"
	ChannelTikTok    = "tiktok"
	ChannelSMS       = "sms"
	ChannelEmail     = "email"
	ChannelFeishu    = "feishu"
	ChannelDingTalk  = "dingtalk"
	ChannelPersonal  = "personal_wx"
	ChannelInstagram = "instagram"
)

// 消息类型
const (
	MsgTypeText     = "text"
	MsgTypeImage    = "image"
	MsgTypeFile     = "file"
	MsgTypeAudio    = "audio"
	MsgTypeVideo    = "video"
	MsgTypeLink     = "link"
	MsgTypeCard     = "card"
	MsgTypeLocation = "location"
)

// MessageEvent 渠道接入消息中台统一消息标准
//
// 设计要点：
//  1. 作为 InboxService.HandleIngressMessage 的入参，是渠道适配器与中台之间的协议
//  2. 包含唯一的 EventID（用于幂等去重）和 SessionID（用于会话锁定）
//  3. 不映射到数据库表，仅作为运行时事件结构在内存与 Redis 中流转
//  4. 与 model.MessageHub（持久化消息）通过 Normalize 转换：MessageEvent -> MessageHub
type MessageEvent struct {
	EventID    string `json:"event_id"`   // 全局唯一事件 ID（用于幂等）
	SessionID  string `json:"session_id"` // 系统内映射的唯一会话 ID
	Channel    string `json:"channel"`    // 渠道来源：web / telegram / whatsapp / xhs ...
	SenderID   string `json:"sender_id"`  // 最终客户的唯一物理标识
	SenderName string `json:"sender_name,omitempty"`
	// SenderType 发送者类型（2026-08-05 钩子机制需求）：
	//   - "customer"：客户消息 → 入库 + 触发 AI 判断
	//   - "self" / "agent"：自己/坐席发出的消息 → 直接丢弃，不入库不触发 AI
	//     （桥接场景扩展上报自己发的消息仅用于状态同步，服务端无需持久化）
	//   - "system"：系统消息 → 仅落库，不触发 AI
	//   - ""：未携带（历史调用方） → 默认按 customer 处理
	SenderType     string         `json:"sender_type,omitempty"`
	ReceiverID     string         `json:"receiver_id,omitempty"`
	MsgType        string         `json:"msg_type"` // text / image / file ...
	Content        string         `json:"content"`
	MediaURL       string         `json:"media_url,omitempty"`
	ConversationID string         `json:"conversation_id,omitempty"`
	IsGroup        bool           `json:"is_group,omitempty"`
	GroupID        string         `json:"group_id,omitempty"`
	IsAIReply      bool           `json:"is_ai_reply,omitempty"`
	AIAgent        string         `json:"ai_agent,omitempty"`
	Extra          map[string]any `json:"extra,omitempty"`
	Timestamp      time.Time      `json:"timestamp"`
	// History 会话级多轮历史（扩展上行携带的即时上下文窗口，服务端落 message_hub.Extra
	// 供统一收件箱展示/可观测；AI 编排的对话上下文由 session_messages 自行重建，不依赖此字段）。
	History []MessageEventHistoryItem `json:"history,omitempty"`
}

// MessageEventHistoryItem 多轮历史中的单轮（与扩展 HistoryItem 对齐的轻量镜像）
type MessageEventHistoryItem struct {
	EventID    string `json:"event_id,omitempty"`
	SenderType string `json:"sender_type,omitempty"`
	SenderID   string `json:"sender_id,omitempty"`
	SenderName string `json:"sender_name,omitempty"`
	ReceiverID string `json:"receiver_id,omitempty"`
	MsgType    string `json:"msg_type"`
	Content    string `json:"content"`
	MediaURL   string `json:"media_url,omitempty"`
	Timestamp  int64  `json:"timestamp"`
	Direction  string `json:"direction,omitempty"`
	IsGroup    bool   `json:"is_group,omitempty"`
	GroupID    string `json:"group_id,omitempty"`
	GroupName  string `json:"group_name,omitempty"`
}
