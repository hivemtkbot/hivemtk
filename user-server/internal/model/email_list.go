package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailDraft 邮件草稿模型
type EmailList struct {
	ID          uuid.UUID      `gorm:"type:char(36);primary_key" json:"id"`
	Subject     string         `gorm:"size:255;not null" json:"subject"` // 邮件主题
	Content     string         `gorm:"type:text" json:"content"`         // 邮件内容
	Attachments string         `gorm:"type:text" json:"attachments"`     // 附件JSON
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	From        string         `gorm:"size:255;not null" json:"from"` // 发件人
	To          string         `gorm:"size:255;not null" json:"to"`   // 收件人
	IsSend      int            `gorm:"default:0" json:"is_send"`      // 是否发送
	SendTime    time.Time      `gorm:"default:null" json:"send_time"` // 发送时间
	IsRead      int            `gorm:"default:0" json:"is_read"`      // 是否已读
	ReadTime    time.Time      `gorm:"default:null" json:"read_time"` // 阅读时间
	JobsID      uuid.UUID      `gorm:"default:null" json:"jobs_id"`   // 任务ID
	IsSuccess   int            `gorm:"default:0" json:"is_success"`   // 是否成功
	TraceID     uuid.UUID      `gorm:"default:null" json:"trace_id"`  // 跟踪ID
}

func (a *EmailList) TableName() string {
	return "email_list"
}

// BeforeCreate 创建前生成UUID
func (e *EmailList) BeforeCreate(tx *gorm.DB) error {
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	return nil
}
