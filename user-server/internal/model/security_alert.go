package model

import (
	"time"
)

// SecurityAlertStatus 告警状态
type SecurityAlertStatus string

const (
	SecurityAlertStatusOpen     SecurityAlertStatus = "open"     // 未处理
	SecurityAlertStatusResolved SecurityAlertStatus = "resolved" // 已解决
	SecurityAlertStatusIgnored  SecurityAlertStatus = "ignored"  // 已忽略
)

// IsValidSecurityAlertStatus 校验告警状态
func IsValidSecurityAlertStatus(s SecurityAlertStatus) bool {
	return s == SecurityAlertStatusOpen || s == SecurityAlertStatusResolved || s == SecurityAlertStatusIgnored
}

// SecurityAlert 安全告警
// 私域独立部署：无 merchant_id 字段
//
// 设计要点：
//   - 由 login_risk 服务在检测到 high/critical 风险时写入
//   - alert_type: abnormal_login / brute_force / device_change / location_change / frequent_failure
//   - risk_level 关联 LoginEvent.RiskLevel
//   - notified: 是否已推送通知（站内信 / 邮件 / 短信）
//   - resolved_at / resolved_by: 处理轨迹
type SecurityAlert struct {
	ID           uint                `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       uint                `gorm:"index" json:"user_id"`                              // 关联用户（0=未知用户/匿名攻击）
	Username     string              `gorm:"type:varchar(50);index" json:"username"`            // 关联用户名
	AlertType    string              `gorm:"type:varchar(50);index;not null" json:"alert_type"` // abnormal_login / brute_force / device_change / location_change / frequent_failure
	RiskLevel    RiskLevel           `gorm:"type:varchar(20);index;not null" json:"risk_level"`
	Title        string              `gorm:"type:varchar(255);not null" json:"title"`
	Description  string              `gorm:"type:text" json:"description"`
	IP           string              `gorm:"type:varchar(50);index" json:"ip"`
	Location     string              `gorm:"type:varchar(255)" json:"location"`
	LoginEventID uint                `gorm:"index" json:"login_event_id"` // 关联 LoginEvent.ID
	Notified     bool                `gorm:"default:false" json:"notified"`
	Status       SecurityAlertStatus `gorm:"type:varchar(20);default:'open';index" json:"status"`
	ResolvedAt   *time.Time          `json:"resolved_at"`
	ResolvedBy   uint                `json:"resolved_by"` // 处理人 user_id
	ResolveNote  string              `gorm:"type:text" json:"resolve_note"`
	CreatedAt    time.Time           `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt    time.Time           `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (SecurityAlert) TableName() string {
	return "security_alerts"
}

// 告警类型常量
const (
	AlertTypeAbnormalLogin   = "abnormal_login"   // 异地登录
	AlertTypeBruteForce      = "brute_force"      // 暴力破解
	AlertTypeDeviceChange    = "device_change"    // 设备指纹变更
	AlertTypeLocationChange  = "location_change"  // 登录地变更
	AlertTypeFrequentFailure = "frequent_failure" // 频繁失败
	AlertTypeAbnormalTime    = "abnormal_time"    // 异常时段
)
