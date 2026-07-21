package model

import "time"

// PaymentConfig 支付配置模型
type PaymentConfig struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	DefaultMethod   string    `gorm:"size:32;default:'alipay'" json:"default_method"` // alipay/wechat/unionpay
	Timeout         int       `gorm:"default:30" json:"timeout"`                      // 支付超时时间(分钟)
	AutoConfirm     bool      `gorm:"default:true" json:"auto_confirm"`               // 是否自动确认
	AutoConfirmDays int       `gorm:"default:7" json:"auto_confirm_days"`             // 自动确认天数
	RefundAudit     bool      `gorm:"default:false" json:"refund_audit"`              // 退款是否需要审核
	AlipayConfig    string    `gorm:"type:text" json:"alipay_config"`                 // JSON: {app_id, private_key, public_key}
	WechatConfig    string    `gorm:"type:text" json:"wechat_config"`                 // JSON: {app_id, mch_id, api_key}
	UnionpayConfig  string    `gorm:"type:text" json:"unionpay_config"`               // JSON: {mer_id, cert_path}
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TableName 指定表名
func (*PaymentConfig) TableName() string {
	return "payment_configs"
}
