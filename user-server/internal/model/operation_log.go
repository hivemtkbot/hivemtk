package model

// operation_log.go 操作日志模型
//
// 设计依据：
//   - 从原 model/team_user.go 拆分（2026-07 阶段 1：单表化 system_users）
//   - OperationLog 与 TeamUser 表已无直接耦合，作为独立审计模型存在
//   - 与 middleware/audit.go 配合：audit.go 写入，operation_log 仓储读取

import "time"

// OperationLog 操作日志模型
type OperationLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     uint      `gorm:"index;not null" json:"user_id"`
	Username   string    `gorm:"type:varchar(50)" json:"username"`
	Action     string    `gorm:"type:varchar(50);not null" json:"action"` // create, update, delete, login, logout, anomaly_login_detected, etc.
	Module     string    `gorm:"type:varchar(50);not null" json:"module"` // user, role, card, shortlink, etc.
	Resource   string    `gorm:"type:varchar(50)" json:"resource"`        // 资源类型
	ResourceID string    `gorm:"type:varchar(50)" json:"resource_id"`     // 资源ID
	Detail     string    `gorm:"type:text" json:"detail"`                 // 操作详情 JSON
	OldValue   string    `gorm:"type:text" json:"old_value"`              // 旧值 JSON
	NewValue   string    `gorm:"type:text" json:"new_value"`              // 新值 JSON
	IP         string    `gorm:"type:varchar(50)" json:"ip"`
	UserAgent  string    `gorm:"type:varchar(255)" json:"user_agent"`
	CreatedAt  time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 指定表名
func (OperationLog) TableName() string {
	return "operation_logs"
}
