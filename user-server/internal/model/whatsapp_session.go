package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WhatsappSession struct {
	ID          uuid.UUID `gorm:"type:char(36);primary_key" json:"id"`
	AccountID   string    `gorm:"type:varchar(36);index" json:"account_id"`
	SessionJSON string    `gorm:"type:text" json:"session_json"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (m *WhatsappSession) TableName() string {
	return "whatsapp_sessions"
}

func (m *WhatsappSession) BeforeCreate(tx *gorm.DB) error {
	if m.ID == (uuid.UUID{}) {
		m.ID = uuid.New()
	}
	return nil
}
