package model

import (
	"time"
)

// KuaishouCard 快手卡片模型
type KuaishouCard struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Title        string    `json:"title" gorm:"size:255;not null"`   // 卡片标题
	Description  string    `json:"description" gorm:"size:500"`      // 卡片描述
	ImageURL     string    `json:"image_url" gorm:"size:500"`        // 卡片图片URL
	RedirectURL  string    `json:"redirect_url" gorm:"size:500"`     // 跳转链接
	ShortLinkID  *uint     `json:"short_link_id"`                    // 关联的短链ID
	DomainPoolID uint      `json:"domain_pool_id"`                   // 关联的域名池ID
	Tags         string    `json:"tags" gorm:"size:255"`             // 标签，逗号分隔
	ViewCount    int       `json:"view_count" gorm:"default:0"`      // 浏览数
	LikeCount    int       `json:"like_count" gorm:"default:0"`      // 点赞数
	ShareCount   int       `json:"share_count" gorm:"default:0"`     // 分享数
	IsActive     bool      `json:"is_active" gorm:"default:true"`    // 是否激活
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"` // 创建时间
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"` // 更新时间
}
