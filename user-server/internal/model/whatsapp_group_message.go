package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WhatsappGroupMessage WhatsApp 群发消息记录
type WhatsappGroupMessage struct {
	ID         string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	QueueID    string    `gorm:"type:varchar(64);index" json:"queue_id"`
	MessageID  string    `gorm:"type:varchar(64);index" json:"message_id"`
	Phone      string    `gorm:"type:varchar(50);index" json:"phone"`
	LeadID     string    `gorm:"type:varchar(64);index" json:"lead_id"`
	Content    string    `gorm:"type:text" json:"content"`
	TemplateID string    `gorm:"type:varchar(64)" json:"template_id"`
	Status     string    `gorm:"type:varchar(20);index" json:"status"` // sent, failed, pending
	ErrorMsg   string    `gorm:"type:text" json:"error_msg"`
	SentAt     time.Time `json:"sent_at"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (WhatsappGroupMessage) TableName() string {
	return "whatsapp_group_messages"
}

// BeforeCreate 钩子
func (m *WhatsappGroupMessage) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}
