package model

import (
	_type "hivemtk-user/internal/pkg/utils/type"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Order struct {
	ID         string                `gorm:"type:varchar(36);primary_key;not null" json:"id"`
	Status     _type.OrderStatusType `gorm:"default:0" json:"status"`
	CreateTime int64                 `gorm:"create_time;autoCreateTime;not null" json:"create_time"`
	Price      string                `gorm:"not null" json:"price"`
	TgID       int64                 `gorm:"tg_id" json:"tg_id"`
	AccountID  string                `gorm:"type:varchar(36);not null" json:"account_id"`
	DeletedAt  gorm.DeletedAt        `gorm:"index" json:"deleted_at,omitempty"`
}

func (*Order) TableName() string {
	return "order"
}

func (o *Order) BeforeCreate(tx *gorm.DB) error {
	o.ID = uuid.New().String()
	return nil
}

