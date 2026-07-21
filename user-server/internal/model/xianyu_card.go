package model

import (
	"time"
)

// XianyuCard 咸鱼卡片模型
type XianyuCard struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Title        string    `json:"title" gorm:"size:255;not null"`   // 卡片标题
	Description  string    `json:"description" gorm:"size:500"`      // 卡片描述
	ImageURL     string    `json:"image_url" gorm:"size:500"`        // 卡片图片URL
	RedirectURL  string    `json:"redirect_url" gorm:"size:500"`     // 跳转链接
	ShortLinkID  uint      `json:"short_link_id" gorm:"default:0"`   // 关联的短链ID，0表示无短链
	DomainPoolID uint      `json:"domain_pool_id" gorm:"default:0"`  // 关联的域名池ID，0表示无域名池
	Tags         string    `json:"tags" gorm:"size:255"`             // 标签，逗号分隔
	ViewCount    int       `json:"view_count" gorm:"default:0"`      // 浏览数
	ClickCount   int       `json:"click_count" gorm:"default:0"`     // 点击数
	ShareCount   int       `json:"share_count" gorm:"default:0"`     // 分享数
	LikeCount    int       `json:"like_count" gorm:"default:0"`      // 点赞数
	IsActive     bool      `json:"is_active" gorm:"default:true"`    // 是否激活
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"` // 创建时间
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"` // 更新时间
}

// TableName 返回表名
func (XianyuCard) TableName() string {
	return "xianyu_cards"
}

// IsActiveCard 检查卡片是否激活
func (x *XianyuCard) IsActiveCard() bool {
	return x.IsActive
}
