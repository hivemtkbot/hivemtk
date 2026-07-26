package model

import "time"

// 渠道标识（与消息中台 platform 白名单对齐）
const (
	ChannelWeb       = "web"
	ChannelTelegram  = "telegram"
	ChannelWhatsApp  = "whatsapp"
	ChannelXHS       = "xhs"
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
	EventID        string         `json:"event_id"`   // 全局唯一事件 ID（用于幂等）
	SessionID      string         `json:"session_id"` // 系统内映射的唯一会话 ID
	Channel        string         `json:"channel"`    // 渠道来源：web / telegram / whatsapp / xhs ...
	SenderID       string         `json:"sender_id"`  // 最终客户的唯一物理标识
	SenderName     string         `json:"sender_name,omitempty"`
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
}
