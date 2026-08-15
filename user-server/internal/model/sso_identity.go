package model

import "time"

// SSOIdentity SSO 外部身份与本地系统用户关联（企业 SSO 接入 P1-E3）
//
// 设计要点：
//   - 一个本地用户可绑定多个 SSO 身份（如同一员工同时接入飞书与企微）
//   - (provider, subject) 唯一：同一 IdP 下的同一主体只能绑定一个本地用户
//   - 不修改 system_users 表结构，SSO 关联数据独立成表，便于后续扩展
//     （多身份 / 解绑 / 审计）
type SSOIdentity struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Provider  string    `json:"provider" gorm:"size:20;not null;uniqueIndex:idx_sso_provider_subject"`
	Subject   string    `json:"subject" gorm:"size:255;not null;uniqueIndex:idx_sso_provider_subject"`
	UserID    uint      `json:"user_id" gorm:"index;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 返回表名
func (SSOIdentity) TableName() string {
	return "sso_identities"
}

