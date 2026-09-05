package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Smlist struct {
	ID        string    `gorm:"type:varchar(36);primary_key" json:"id"`
	QQ        string    `gorm:"qq" json:"qq"`
	Tg        string    `gorm:"tg" json:"tg"`
	WX        string    `gorm:"wx" json:"wx"`
	X         string    `gorm:"x" json:"x"`
	Name      string    `gorm:"name" json:"name"`
	Phone     string    `gorm:"phone" json:"phone"`
	City      string    `gorm:"city" json:"city"`
	Address   string    `gorm:"address" json:"address"`
	Desc      string    `gorm:"desc" json:"desc"`
	Age       string    `gorm:"age" json:"age"`
	Score     string    `gorm:"score" json:"score"`
	Price     string    `gorm:"price" json:"price"`
	Service   string    `gorm:"service" json:"service"`
	Images    string    `gorm:"images" json:"images"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (m *Smlist) TableName() string {
	return "smlist"
}

func (m *Smlist) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}
