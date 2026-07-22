package model

import (
	"time"
)

// XiaohongshuCard 小红书卡片模型
type XiaohongshuCard struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Title        string    `gorm:"size:255;not null" json:"title"`
	Description  string    `gorm:"type:text" json:"description"`
	ImageURL     string    `gorm:"size:500" json:"image_url"`
	RedirectURL  string    `gorm:"size:500" json:"redirect_url"`
	ShareURL     string    `gorm:"size:500" json:"share_url"`
	ShortLinkID  *uint     `json:"short_link_id" gorm:""`  // 关联的短链ID
	DomainPoolID *uint     `json:"domain_pool_id" gorm:""` // 关联的域名池ID
	Tags         string    `gorm:"size:255" json:"tags"`
	ViewCount    int       `gorm:"default:0" json:"view_count"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定表名
func (XiaohongshuCard) TableName() string {
	return "xiaohongshu_cards"
}
