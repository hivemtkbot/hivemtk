package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailSend 已发送邮件模型
type EmailSend struct {
	ID          string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	To          string         `gorm:"size:255;not null" json:"to"`      
	Subject     string         `gorm:"size:255;not null" json:"subject"` 
	Content     string         `gorm:"type:text" json:"content"`         
	Attachments string         `gorm:"type:text" json:"attachments"`     
	Status      int            `gorm:"default:0" json:"status"`          
	SendTime    *time.Time     `json:"send_time,omitempty"`              
	SmtpID      string         `json:"smtp_id"`                          
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

