package model

import "time"

// UserBlacklist 用户黑名单
//
// 方向10：坐席实时聊天看板 - 拉黑/取消拉黑功能
//
// 设计：
//   - 以 UserID 维度拉黑（非单次会话），避免该访客后续再次接入。
//   - 软删除：Active=false 保留历史，便于审计与解除。
//   - Reason / Source 记录拉黑原因与来源（坐席手动 / 风控自动）。
//   - 过期字段 ExpiresAt 支持临时拉黑（time=永久，nil=永久）。
type UserBlacklist struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       string     `gorm:"type:varchar(50);index;not null" json:"user_id"`
	Platform     Platform   `gorm:"type:varchar(20);index" json:"platform"`
	Reason       string     `gorm:"type:varchar(500)" json:"reason"`
	Source       string     `gorm:"type:varchar(50);default:'manual'" json:"source"`
	OperatorID   uint       `gorm:"index" json:"operator_id"`
	OperatorName string     `gorm:"type:varchar(100)" json:"operator_name"`
	SessionID    string     `gorm:"type:varchar(120);index" json:"session_id"`
	Active       bool       `gorm:"default:true;index" json:"active"`
	ExpiresAt    *time.Time `json:"expires_at"`
	CreatedAt    time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (UserBlacklist) TableName() string {
	return "user_blacklist"
}
