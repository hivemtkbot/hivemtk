package model

import "time"

// WechatAccount 微信公众号账号配置
type WechatAccount struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	AppID     string `gorm:"type:varchar(64);uniqueIndex;not null" json:"app_id"`
	AppSecret string `gorm:"type:varchar(128);not null" json:"-"`
	// OriginalID 公众号原始ID（gh_xxxxx）
	OriginalID string `gorm:"type:varchar(64)" json:"original_id"`
	// Token 服务器配置 token（用于验证微信服务器）
	Token string `gorm:"type:varchar(64)" json:"-"`
	// EncodingAESKey 消息加解密密钥
	EncodingAESKey string `gorm:"type:varchar(64)" json:"-"`
	// AgentID 绑定的智能体 ID
	AgentID string `gorm:"type:varchar(36);index" json:"agent_id"`
	// Status 状态：active / inactive
	Status string `gorm:"type:varchar(20);default:active" json:"status"`
	// AccessToken 当前 access_token
	AccessToken string `gorm:"type:varchar(512)" json:"-"`
	// TokenExpiresAt token 过期时间
	TokenExpiresAt time.Time `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (WechatAccount) TableName() string {
	return "wechat_accounts"
}