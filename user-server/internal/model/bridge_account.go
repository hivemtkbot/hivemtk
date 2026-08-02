package model

import "time"

// BridgeAccount 桥接账号（抖音/小红书/TikTok 网页私信扩展账号）
//
// 由扩展连接时（register 帧）自动注册/更新；user_id 记录归属，
// agent_id 用于 AI 路由（与 channel_agent_bindings 联动）。
type BridgeAccount struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint       `gorm:"index;not null;default:0" json:"user_id"`
	Channel     string     `gorm:"type:varchar(32);not null;uniqueIndex:uk_bridge_ch_acc" json:"channel"`
	AccountID   string     `gorm:"type:varchar(64);not null;uniqueIndex:uk_bridge_ch_acc" json:"account_id"`
	AccountName string     `gorm:"type:varchar(128);not null;default:''" json:"account_name"`
	AgentID     uint       `gorm:"index;not null;default:0" json:"agent_id"`
	Status      string     `gorm:"type:varchar(16);not null;default:'offline'" json:"status"` // online | offline
	LastSyncAt  *time.Time `json:"last_sync_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (BridgeAccount) TableName() string { return "bridge_accounts" }
