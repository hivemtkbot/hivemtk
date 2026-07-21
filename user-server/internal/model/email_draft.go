package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

// EmailDraft 邮件草稿模型
type EmailDraft struct {
	ID          uuid.UUID      `gorm:"type:char(36);primary_key" json:"id"`
	Subject     string         `gorm:"size:255;not null" json:"subject"` // 邮件主题
	Content     string         `gorm:"type:text" json:"content"`         // 邮件内容
	Attachments string         `gorm:"type:text" json:"attachments"`     // 附件JSON
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// BeforeCreate 创建前生成UUID
func (e *EmailDraft) BeforeCreate(tx *gorm.DB) error {
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	e.UpdatedAt = time.Now()
	return nil
}
