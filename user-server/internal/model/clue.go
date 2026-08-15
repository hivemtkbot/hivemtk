package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Clue struct {
	ID       string `gorm:"type:varchar(36);primary_key" json:"id"`
	SourceID string `gorm:"source_id" json:"source_id"`
	Account  string `gorm:"account" json:"account"`
	Type     int64  `gorm:"column:type" json:"type"`
	IsVerify int64  `gorm:"is_verify" json:"is_verify"`
	Name     string `gorm:"name" json:"name"`
	City     string `gorm:"city" json:"city"`
	Address  string `gorm:"address" json:"address"`
	Desc     string `gorm:"desc" json:"desc"`
	IntentScore   int64 `gorm:"column:intent_score;default:0" json:"intent_score"`
	IsOpportunity int64 `gorm:"column:is_opportunity;default:0" json:"is_opportunity"`
	MessageID      string `gorm:"column:message_id;type:varchar(100)" json:"message_id"`
	ConversationID string `gorm:"column:conversation_id;type:varchar(100);index" json:"conversation_id"`
	OneID          string `gorm:"column:one_id;type:varchar(100);index" json:"one_id"`
	CreateTime     int64  `gorm:"autoCreateTime" json:"create_time"`
}

func (m *Clue) TableName() string {
	return "clues"
}

func (m *Clue) BeforeCreate(tx *gorm.DB) error {
	m.ID = uuid.New().String()
	return nil
}

