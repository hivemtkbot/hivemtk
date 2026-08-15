package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailDraft 邮件草稿模型
type EmailList struct {
	ID          uuid.UUID      `gorm:"type:char(36);primary_key" json:"id"`
	Subject     string         `gorm:"size:255;not null" json:"subject"` 
	Content     string         `gorm:"type:text" json:"content"`         
	Attachments string         `gorm:"type:text" json:"attachments"`     
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	From        string         `gorm:"size:255;not null" json:"from"` 
	To          string         `gorm:"size:255;not null" json:"to"`   
	IsSend      int            `gorm:"default:0" json:"is_send"`      
	SendTime    time.Time      `gorm:"default:null" json:"send_time"` 
	IsRead      int            `gorm:"default:0" json:"is_read"`      
	ReadTime    time.Time      `gorm:"default:null" json:"read_time"` 
	JobsID      uuid.UUID      `gorm:"default:null" json:"jobs_id"`   
	IsSuccess   int            `gorm:"default:0" json:"is_success"`   
	TraceID     uuid.UUID      `gorm:"default:null" json:"trace_id"`  
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

