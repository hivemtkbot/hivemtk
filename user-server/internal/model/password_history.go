package model

import (
	"time"
)

// PasswordHistory 密码历史
// 私域独立部署：无 merchant_id 字段
//
// 设计要点：
//   - 每次成功改密写入一条记录（password_hash = bcrypt 哈希）
//   - 用于 forbid_reuse 策略：禁止最近 N 个密码重复
//   - changed_at 由 service 层显式写入（避免 GORM autoUpdateTime 在更新时被覆盖）
//   - source: change_password / reset_password / init_change / forgot_reset
type PasswordHistory struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       uint      `gorm:"index;not null" json:"user_id"`
	PasswordHash string    `gorm:"type:varchar(255);not null" json:"-"` // bcrypt 哈希（不在 JSON 中返回）
	ChangedAt    time.Time `gorm:"index;not null" json:"changed_at"`
	Source       string    `gorm:"type:varchar(50);default:'change_password'" json:"source"` // change_password / reset_password / init_change / forgot_reset
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (PasswordHistory) TableName() string {
	return "password_history"
}

// 密码变更来源常量
const (
	PasswordSourceChangePassword = "change_password"
	PasswordSourceResetPassword  = "reset_password"
	PasswordSourceInitChange     = "init_change"
	PasswordSourceForgotReset    = "forgot_reset"
	PasswordSourceCreate         = "create"
)

// IsValidPasswordSource 校验密码变更来源
func IsValidPasswordSource(s string) bool {
	return s == PasswordSourceChangePassword ||
		s == PasswordSourceResetPassword ||
		s == PasswordSourceInitChange ||
		s == PasswordSourceForgotReset ||
		s == PasswordSourceCreate
}
