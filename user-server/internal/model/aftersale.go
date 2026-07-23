package model

import "time"

// AfterSale 售后单（客服侧发起，动作回写电商；本系统只记录状态，不履约）。
//
// 这是客服系统对"订单"唯一允许写入的形态。订单本身的创建/支付/履约由外部电商负责，
// 客服只在此记录"退款/退货/换货"请求并跟踪其状态（状态由电商通过 Webhook 回写）。
type AfterSale struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform      string    `gorm:"type:varchar(50);index" json:"platform"`
	OrderID       string    `gorm:"type:varchar(100);index" json:"order_id"`
	CustomerPhone string    `gorm:"type:varchar(50);index" json:"customer_phone"`
	CustomerName  string    `gorm:"type:varchar(100)" json:"customer_name"`
	Type          string    `gorm:"type:varchar(50)" json:"type"`   // refund / return / exchange
	Reason        string    `gorm:"type:text" json:"reason"`       // 售后原因
	Amount        int64     `gorm:"type:bigint;default:0" json:"amount"`
	Status        string    `gorm:"type:varchar(50);default:'pending'" json:"status"` // pending/processing/done/rejected
	ExternalID    string    `gorm:"type:varchar(100)" json:"external_id"`             // 电商侧售后单号
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (AfterSale) TableName() string { return "after_sales" }
