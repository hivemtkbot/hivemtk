package model

import (
	"time"
)

// ShortLink 短链模型
type ShortLink struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	ShortCode   string     `json:"short_code" gorm:"uniqueIndex;size:20;not null;comment:短码"` // 短码，用于生成短链接
	OriginalURL string     `json:"original_url" gorm:"size:2048;not null;comment:原始URL"`      // 原始URL
	Title       string     `json:"title" gorm:"size:255;comment:标题"`                          // 标题
	Description string     `json:"description" gorm:"size:512;comment:描述"`                    // 描述
	DomainID    uint       `json:"domain_id" gorm:"comment:域名ID"`                             // 域名ID，用于选择短链域名
	Password    string     `json:"password" gorm:"size:100;comment:访问密码"`                     // 访问密码，为空则无需密码
	ExpireTime  *time.Time `json:"expire_time" gorm:"comment:过期时间"`                           // 过期时间，为空则永不过期
	ClickCount  int        `json:"click_count" gorm:"default:0;comment:点击次数"`                 // 点击次数
	Status      int        `json:"status" gorm:"default:1;comment:状态"`                        // 状态: 1-正常, 2-禁用
	CreatedAt   time.Time  `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`             // 创建时间
	UpdatedAt   time.Time  `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`             // 更新时间
}

// TableName 返回表名
func (ShortLink) TableName() string {
	return "short_links"
}

// IsExpired 检查短链是否已过期
func (s *ShortLink) IsExpired() bool {
	if s.ExpireTime == nil {
		return false
	}
	return time.Now().After(*s.ExpireTime)
}

// IsActive 检查短链是否处于活跃状态
func (s *ShortLink) IsActive() bool {
	return s.Status == 1 && !s.IsExpired()
}
