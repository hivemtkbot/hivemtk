package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

// EmailSend 已发送邮件模型
type EmailSend struct {
	ID          string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	To          string         `gorm:"size:255;not null" json:"to"`      // 收件人
	Subject     string         `gorm:"size:255;not null" json:"subject"` // 主题
	Content     string         `gorm:"type:text" json:"content"`         // 内容
	Attachments string         `gorm:"type:text" json:"attachments"`     // 附件
	Status      int            `gorm:"default:0" json:"status"`          // 0-待发送 1-已发送 2-发送失败
	SendTime    *time.Time     `json:"send_time,omitempty"`              // 计划发送时间
	SmtpID      string         `json:"smtp_id"`                          // 使用的SMTP账号ID
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (e *EmailSend) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}
