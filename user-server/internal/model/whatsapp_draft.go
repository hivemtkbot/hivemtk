package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WhatsappDraft struct {
	ID        uuid.UUID `gorm:"type:char(36);primary_key" json:"id"`
	Title     string    `gorm:"type:varchar(150);not null" json:"title"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (m *WhatsappDraft) TableName() string {
	return "whatsapp_drafts"
}

func (m *WhatsappDraft) BeforeCreate(tx *gorm.DB) error {
	if m.ID == (uuid.UUID{}) {
		m.ID = uuid.New()
	}
	return nil
}
