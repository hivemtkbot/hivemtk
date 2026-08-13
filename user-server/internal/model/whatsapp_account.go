package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WhatsappAccountStatus string

const (
	WhatsappStatusPending WhatsappAccountStatus = "pending"
	WhatsappStatusOnline  WhatsappAccountStatus = "online"
	WhatsappStatusOffline WhatsappAccountStatus = "offline"
)

type WhatsappAccount struct {
	ID        uuid.UUID             `gorm:"type:char(36);primary_key" json:"id"`
	Name      string                `gorm:"type:varchar(100);not null" json:"name"`
	Remark    string                `gorm:"type:varchar(255)" json:"remark"`
	Status    WhatsappAccountStatus `gorm:"type:varchar(20);default:'pending'" json:"status"`
	CreatedAt time.Time             `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time             `gorm:"autoUpdateTime" json:"updated_at"`
}

func (m *WhatsappAccount) TableName() string {
	return "whatsapp_accounts"
}

func (m *WhatsappAccount) BeforeCreate(tx *gorm.DB) error {
	if m.ID == (uuid.UUID{}) {
		m.ID = uuid.New()
	}
	return nil
}
