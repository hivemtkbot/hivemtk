package model

import (
	"time"
)

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// IsValidRiskLevel 校验风险等级
func IsValidRiskLevel(r RiskLevel) bool {
	return r == RiskLevelLow || r == RiskLevelMedium || r == RiskLevelHigh || r == RiskLevelCritical
}

// LoginEvent 登录事件
// 私域独立部署：无 merchant_id 字段
//
// 设计要点：
//   - 记录每次登录尝试（含成功/失败）
//   - 通过 IP + UserAgent 计算 device_fingerprint
//   - login_at 用毫秒精度时间戳，便于按时间窗口聚合
//   - risk_level 由 login_risk 服务计算后写入
//   - success: true=登录成功 / false=登录失败
//   - location: IP 解析后的地理位置（国家+省+市）
type LoginEvent struct {
	ID                uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID            uint      `gorm:"index;not null" json:"user_id"`
	Username          string    `gorm:"type:varchar(50);index" json:"username"`
	IP                string    `gorm:"type:varchar(50);index;not null" json:"ip"`
	UserAgent         string    `gorm:"type:varchar(512)" json:"user_agent"`
	DeviceFingerprint string    `gorm:"type:varchar(128);index" json:"device_fingerprint"`
	LoginAt           time.Time `gorm:"index;not null" json:"login_at"`
	Success           bool      `json:"success"`
	RiskLevel         RiskLevel `gorm:"type:varchar(20);default:'low';index" json:"risk_level"`
	Location          string    `gorm:"type:varchar(255)" json:"location"`
	Reason            string    `gorm:"type:varchar(255)" json:"reason"`
	CreatedAt         time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (LoginEvent) TableName() string {
	return "login_events"
}
