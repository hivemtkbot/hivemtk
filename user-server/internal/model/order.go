package model

import (
	_type "marketing/internal/pkg/utils/type"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Order struct {
	ID         string                `gorm:"type:varchar(36);primary_key;not null" json:"id"`
	Status     _type.OrderStatusType `gorm:"default:0" json:"status"` // 0待支付 100已支付 -1超时 -2强行关闭
	CreateTime int64                 `gorm:"create_time;autoCreateTime;not null" json:"create_time"`
	Price      string                `gorm:"not null" json:"price"`
	TgID       int64                 `gorm:"tg_id" json:"tg_id"` // 不能给unique，一个tg_id会创建多个订单
	AccountID  string                `gorm:"type:varchar(36);not null" json:"account_id"`
}

func (*Order) TableName() string {
	return "order"
}

func (o *Order) BeforeCreate(tx *gorm.DB) error {
	o.ID = uuid.New().String()
	return nil
}
