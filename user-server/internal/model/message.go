package model

import (
	_type "marketing/internal/pkg/utils/type"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Message struct {
	ID         string               `gorm:"type:varchar(36);primary_key" json:"id"`
	Status     _type.UserStatusType `gorm:"status;default:1" json:"status"`
	TgID       int64                `gorm:"tg_id" json:"tg_id"`
	CreateTime int64                `gorm:"autoCreateTime" json:"create_time"`
	AccountID  string               `gorm:"type:varchar(36)" json:"account_id"`
	UserID     string               `gorm:"type:varchar(36)" json:"user_id"` // 改为string类型，与User模型一致
	Text       string               `gorm:"type:varchar(255)" json:"text"`
}

func (m *Message) TableName() string {
	return "message"
}

func (m *Message) BeforeCreate(tx *gorm.DB) error {
	m.ID = uuid.New().String()
	return nil
}
