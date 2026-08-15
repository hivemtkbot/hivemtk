package model

import "time"

// SmsCarrier 运营商（短信到达率追踪 E 域）
type SmsCarrier string

const (
	SmsCarrierMobile  SmsCarrier = "mobile"  
	SmsCarrierUnicom  SmsCarrier = "unicom"  
	SmsCarrierTelecom SmsCarrier = "telecom" 
	SmsCarrierUnknown SmsCarrier = "unknown" 
)

// SmsNumberPortabilityRecord 携号转网记录
//
// 表：sms_number_portability_logs（私域独立部署，无 merchant_id）
// 用户携号转网后，运营商回执的 carrier 字段会变化；本表追踪 carrier 变化以发现
// "号码已换运营商"，对运营触达策略很重要。
type SmsNumberPortabilityRecord struct {
	ID              int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Phone           string     `gorm:"column:phone;size:20;not null;index" json:"phone"`
	OriginalCarrier SmsCarrier `gorm:"column:original_carrier;size:32" json:"original_carrier"`
	CurrentCarrier  SmsCarrier `gorm:"column:current_carrier;size:32" json:"current_carrier"`
	DetectedAt      time.Time  `gorm:"column:detected_at;not null;index:idx_sms_np_detected_at,sort:desc" json:"detected_at"`
	Source          string     `gorm:"column:source;size:32;not null;default:'webhook'" json:"source"`
	RawPayload      string     `gorm:"column:raw_payload;type:text" json:"raw_payload"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null;default:now()" json:"created_at"`
}

// TableName 表名
//
// 注：原 service 实现中此方法签名为 TableName(ctx context.Context) string，
// GORM 不会调用该错误签名；此处修正为标准 GORM TableName 签名，
// 表名与原注释保持一致：sms_number_portability_logs。
func (SmsNumberPortabilityRecord) TableName() string {
	return "sms_number_portability_logs"
}

