package model

import "time"

// SessionMessage 会话消息
type SessionMessage struct {
	ID           uint        `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID    string      `gorm:"type:varchar(50);index;not null" json:"session_id"`
	Content      string      `gorm:"type:text" json:"content"`
	ContentType  MessageType `gorm:"type:varchar(20)" json:"content_type"`
	// 结构化富卡片：ContentType=card 时承载（RichCard 的 JSON 序列化）
	CardData string       `gorm:"type:text" json:"card_data"`
	CardType RichCardType `gorm:"type:varchar(20)" json:"card_type"`
	MediaURL     string      `gorm:"type:varchar(500)" json:"media_url"`
	SenderType   string      `gorm:"type:varchar(20);not null" json:"sender_type"` // user, ai, agent
	SenderID     string      `gorm:"type:varchar(50)" json:"sender_id"`
	SenderName   string      `gorm:"type:varchar(100)" json:"sender_name"`
	SenderAvatar string      `gorm:"type:varchar(500)" json:"sender_avatar"`
	AIConfidence float64     `gorm:"type:decimal(5,2)" json:"ai_confidence"`
	AISource     string      `gorm:"type:varchar(20)" json:"ai_source"` // rule, rag, llm
	IsRead       bool        `gorm:"default:false" json:"is_read"`
	ReadAt       *time.Time  `json:"read_at"`
	// DeliveredAt 投递给访客 WebSocket 的时间（nil 表示未投递，用于离线消息）
	DeliveredAt *time.Time `gorm:"index" json:"delivered_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 指定表名
func (SessionMessage) TableName() string {
	return "session_messages"
}
