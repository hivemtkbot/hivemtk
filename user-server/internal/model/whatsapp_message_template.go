package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WhatsappMessageTemplate WhatsApp 消息模板
type WhatsappMessageTemplate struct {
	ID          string    `gorm:"type:varchar(64);primaryKey" json:"id"`
	Name        string    `gorm:"type:varchar(255);not null" json:"name"`
	Content     string    `gorm:"type:text;not null" json:"content"`
	Category    string    `gorm:"type:varchar(100);index" json:"category"`
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	Description string    `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (*WhatsappMessageTemplate) TableName() string {
	return "whatsapp_message_templates"
}

// BeforeCreate 钩子：若未提供 ID 则自动生成
func (m *WhatsappMessageTemplate) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = "tmpl_" + uuid.New().String()
	}
	return nil
}
