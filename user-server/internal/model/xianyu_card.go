package model

import (
	"time"
)

// XianyuCard 闲鱼卡片模型
type XianyuCard struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Title        string    `json:"title" gorm:"size:255;not null"`
	Description  string    `json:"description" gorm:"size:500"`
	ImageURL     string    `json:"image_url" gorm:"size:500"`
	RedirectURL  string    `json:"redirect_url" gorm:"size:500"`
	ShortLinkID  uint      `json:"short_link_id" gorm:"default:0"`
	DomainPoolID uint      `json:"domain_pool_id" gorm:"default:0"`
	Tags         string    `json:"tags" gorm:"size:255"`
	ViewCount    int       `json:"view_count" gorm:"default:0"`
	ClickCount   int       `json:"click_count" gorm:"default:0"`
	ShareCount   int       `json:"share_count" gorm:"default:0"`
	LikeCount    int       `json:"like_count" gorm:"default:0"`
	IsActive     bool      `json:"is_active" gorm:"default:true"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 返回表名
func (XianyuCard) TableName() string {
	return "xianyu_cards"
}
