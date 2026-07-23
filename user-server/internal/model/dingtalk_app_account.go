package model

import "time"

// DingTalkAppAccount 钉钉「企业内部应用」账号（支持回调收消息）
//
// 与钉钉群机器人（仅出站，无法收消息）不同，企业内部应用可在「事件订阅」中
// 配置回调地址接收用户发给应用的消息，从而成为可收消息的正式渠道。
// 回调需经过：URL 验证（GET 验签）+ 消息接收（POST，AES 解密后入消息中台→AI）。
type DingTalkAppAccount struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	AccountName    string     `gorm:"type:varchar(100);not null" json:"account_name"`
	AppKey         string     `gorm:"type:varchar(200);not null" json:"app_key"`
	AppSecret      string     `gorm:"type:varchar(500);not null" json:"app_secret"`
	AgentID        string     `gorm:"type:varchar(100)" json:"agent_id"`     // 应用 AgentId（可选）
	Token          string     `gorm:"type:varchar(200)" json:"token"`        // 回调 URL 验证 token（验签用）
	AESKey         string     `gorm:"type:varchar(200)" json:"aes_key"`      // 回调数据加密密钥（base64，可选；配置后消息体 AES 解密）
	InboundEnabled bool       `gorm:"default:false" json:"inbound_enabled"` // 是否开启入站收消息
	AIAgentID      string     `gorm:"type:varchar(100)" json:"ai_agent_id"` // 绑定 AI 智能体（为空时降级默认引擎）
	UserID         uint       `gorm:"default:0" json:"user_id"`
	LastErrorAt    *time.Time `json:"last_error_at"`
	LastErrorMsg   string     `gorm:"type:text" json:"last_error_msg"`
	Status         int        `gorm:"default:1" json:"status"`
	CreatedAt      time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (DingTalkAppAccount) TableName() string { return "dingtalk_app_accounts" }
