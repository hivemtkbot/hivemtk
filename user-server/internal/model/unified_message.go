package model

import (
	"time"
)

// Platform 平台类型
type Platform string

const (
	PlatformDouyin      Platform = "douyin"
	PlatformKuaishou    Platform = "kuaishou"
	PlatformXiaohongshu Platform = "xiaohongshu"
	PlatformXianyu      Platform = "xianyu"
	PlatformTiktok      Platform = "tiktok"
	PlatformWeChat      Platform = "wechat"
	PlatformWeb         Platform = "web"
	// PlatformWebEmbed Web Widget 嵌入（访客端）
	// 区别于 PlatformWeb（商户 B 端后台），PlatformWebEmbed 用于第三方网站访客
	PlatformWebEmbed Platform = "web_embed"
)

// MessageType 消息类型
type MessageType string

const (
	MessageTypeText  MessageType = "text"
	MessageTypeImage MessageType = "image"
	MessageTypeVideo MessageType = "video"
	MessageTypeAudio MessageType = "audio"
	MessageTypeFile  MessageType = "file"
	MessageTypeCard  MessageType = "card"
)

// MessageStatus 消息状态
type MessageStatus string

const (
	MessageStatusPending    MessageStatus = "pending"
	MessageStatusProcessing MessageStatus = "processing"
	MessageStatusReplied    MessageStatus = "replied"
	MessageStatusFailed     MessageStatus = "failed"
	MessageStatusIgnored    MessageStatus = "ignored"
)

// ChatType 会话类型
type ChatType string

const (
	ChatTypePrivate ChatType = "private"
	ChatTypeGroup   ChatType = "group"
)

// UnifiedMessage 统一消息模型
type UnifiedMessage struct {
	ID           uint          `gorm:"primaryKey;autoIncrement" json:"id"`
	MessageID    string        `gorm:"type:varchar(50);uniqueIndex;not null" json:"message_id"`
	Platform     Platform      `gorm:"type:varchar(20);index;not null" json:"platform"`
	AccountID    string        `gorm:"type:varchar(50);index" json:"account_id"`
	AccountName  string        `gorm:"type:varchar(100)" json:"account_name"`
	ChatID       string        `gorm:"type:varchar(50);index" json:"chat_id"`
	ChatType     ChatType      `gorm:"type:varchar(20)" json:"chat_type"`
	SenderID     string        `gorm:"type:varchar(50);index" json:"sender_id"`
	SenderName   string        `gorm:"type:varchar(100)" json:"sender_name"`
	SenderAvatar string        `gorm:"type:varchar(500)" json:"sender_avatar"`
	Content      string        `gorm:"type:text" json:"content"`
	ContentType  MessageType   `gorm:"type:varchar(20)" json:"content_type"`
	MediaURL     string        `gorm:"type:varchar(500)" json:"media_url"`
	ReplyToID    string        `gorm:"type:varchar(50)" json:"reply_to_id"`
	Status       MessageStatus `gorm:"type:varchar(20);default:'pending'" json:"status"`
	RawData      string        `gorm:"type:text" json:"-"` // 原始平台数据(JSON)
	ReceivedAt   time.Time     `gorm:"autoCreateTime" json:"received_at"`
	CreatedAt    time.Time     `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (UnifiedMessage) TableName() string {
	return "unified_messages"
}

// UnifiedReply 统一回复模型
type UnifiedReply struct {
	ID            uint        `gorm:"primaryKey;autoIncrement" json:"id"`
	ReplyID       string      `gorm:"type:varchar(50);uniqueIndex" json:"reply_id"`
	MessageID     string      `gorm:"type:varchar(50);index;not null" json:"message_id"`
	Platform      Platform    `gorm:"type:varchar(20)" json:"platform"`
	AccountID     string      `gorm:"type:varchar(50)" json:"account_id"`
	ChatID        string      `gorm:"type:varchar(50)" json:"chat_id"`
	Content       string      `gorm:"type:text" json:"content"`
	ContentType   MessageType `gorm:"type:varchar(20)" json:"content_type"`
	MediaURL      string      `gorm:"type:varchar(500)" json:"media_url"`
	ReplyType     string      `gorm:"type:varchar(20)" json:"reply_type"` // rule, rag, llm, human
	Confidence    float64     `gorm:"type:decimal(5,2)" json:"confidence"`
	RuleID        uint        `json:"rule_id"`
	KnowledgeID   uint        `json:"knowledge_id"`
	AgentID       uint        `json:"agent_id"`
	Status        ReplyStatus `gorm:"type:varchar(20)" json:"status"`
	ErrorMessage  string      `gorm:"type:text" json:"error_message"`
	PlatformMsgID string      `gorm:"type:varchar(50)" json:"platform_msg_id"` // 平台返回的消息ID
	SentAt        *time.Time  `json:"sent_at"`
	CreatedAt     time.Time   `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (UnifiedReply) TableName() string {
	return "unified_replies"
}

// ReplyStatus 回复状态
type ReplyStatus string

const (
	ReplyStatusPending   ReplyStatus = "pending"
	ReplyStatusSent      ReplyStatus = "sent"
	ReplyStatusFailed    ReplyStatus = "failed"
	ReplyStatusDiscarded ReplyStatus = "discarded"
)

// PlatformAccount 平台账号配置
type PlatformAccount struct {
	ID            uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform      Platform   `gorm:"type:varchar(20);index;not null" json:"platform"`
	AccountID     string     `gorm:"type:varchar(50)" json:"account_id"`
	AccountName   string     `gorm:"type:varchar(100)" json:"account_name"`
	AccountAvatar string     `gorm:"type:varchar(500)" json:"account_avatar"`
	Config        string     `gorm:"type:text" json:"config"` // JSON格式的平台特定配置
	Cookie        string     `gorm:"type:text" json:"-"`      // 加密存储的Cookie
	Token         string     `gorm:"type:text" json:"-"`      // 加密存储的Token
	Status        int        `gorm:"default:1" json:"status"` // 1-正常, 0-禁用
	LastSyncAt    *time.Time `json:"last_sync_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (PlatformAccount) TableName() string {
	return "platform_accounts"
}

// ReplyDecision 回复决策
type ReplyDecision struct {
	ShouldReply  bool              `json:"should_reply"`
	ReplyType    string            `json:"reply_type"` // rule, rag, llm, human
	Content      string            `json:"content"`
	Confidence   float64           `json:"confidence"`
	Reason       string            `json:"reason"`
	KnowledgeHit *KnowledgeHit     `json:"knowledge_hit,omitempty"`
	Variables    map[string]string `json:"variables,omitempty"`
}

// KnowledgeHit 知识库命中结果
type KnowledgeHit struct {
	ID         uint    `json:"id"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	Source     string  `json:"source"`
	CategoryID uint    `json:"category_id"`
}

// PlatformAdapter 平台适配器接口
type PlatformAdapter interface {
	// 基本信息
	GetPlatform() Platform

	// 消息操作
	GetMessages(accountID string, opts *MessageQueryOptions) ([]*UnifiedMessage, error)
	SendMessage(accountID, chatID, content string, opts *SendOptions) (*UnifiedReply, error)
	SendImage(accountID, chatID, imageURL string) (*UnifiedReply, error)

	// 账号操作
	Login(credentials map[string]string) (*PlatformAccount, error)
	CheckLoginStatus(accountID string) (bool, error)
	Logout(accountID string) error
	RefreshToken(accountID string) error

	// 用户信息
	GetUserInfo(accountID, userID string) (*PlatformUser, error)
	GetChatInfo(accountID, chatID string) (*ChatInfo, error)

	// Webhook
	ParseWebhook(data []byte) (*UnifiedMessage, error)
	GetWebhookURL(accountID string) string
}

// MessageQueryOptions 消息查询选项
type MessageQueryOptions struct {
	ChatID    string
	SenderID  string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

// SendOptions 发送选项
type SendOptions struct {
	ReplyToID   string
	MediaURL    string
	ContentType MessageType
}

// PlatformUser 平台用户信息
type PlatformUser struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	Bio       string `json:"bio"`
	IsFollow  bool   `json:"is_follow"`
	Followers int    `json:"followers"`
}

// ChatInfo 会话信息
type ChatInfo struct {
	ChatID      string    `json:"chat_id"`
	ChatType    ChatType  `json:"chat_type"`
	Name        string    `json:"name"`
	Avatar      string    `json:"avatar"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
}
