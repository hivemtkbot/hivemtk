package model

import "time"

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
