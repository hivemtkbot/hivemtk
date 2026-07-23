package model

import (
	"time"
)

// auto_reply_account.go
//
// Cookie 加解密方法 (GetCookie/SetCookie) 与自定义 JSON 序列化
// (MarshalJSON/UnmarshalJSON) 已迁移到 service/auto_reply_account_service.go
// 作为包级函数 GetAutoReplyAccountCookie / SetAutoReplyAccountCookie。
// Cookie 字段已有 `json:"-"`，默认序列化不会暴露，无需自定义 MarshalJSON。
type AutoReplyAccount struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	UserID            uint       `gorm:"index" json:"user_id"`
	Platform          string     `gorm:"size:20;index" json:"platform"`
	Username          string     `gorm:"size:100" json:"username"`
	Cookie            string     `gorm:"type:text" json:"-"`
	IsActive          bool       `gorm:"default:false" json:"is_active"`
	Headless          bool       `gorm:"default:true" json:"headless"`
	WsMode            bool       `gorm:"default:false" json:"ws_mode"`
	LastWSConnectedAt *time.Time `json:"last_ws_connected_at"`
	LoginAt           *time.Time `json:"login_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}
