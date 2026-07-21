package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LiveCode 活码模型
type LiveCode struct {
	ID              string    `json:"id" gorm:"type:varchar(36);primary_key"`
	Name            string    `json:"name" gorm:"size:100;not null"`         // 活码名称
	ShortLink       string    `json:"short_link" gorm:"size:50;uniqueIndex"` // 短链
	ShortDomainID   int       `json:"short_domain_id" gorm:"not null"`       // 短链域名ID
	EntryDomainID   int       `json:"entry_domain_id" gorm:"not null"`       // 入口域名ID
	LandingDomainID int       `json:"landing_domain_id" gorm:"not null"`     // 落地域名ID
	Status          int       `json:"status" gorm:"default:1"`               // 状态：1-启用，0-禁用
	TotalViews      int       `json:"total_views" gorm:"default:0"`          // 总访问量
	TodayViews      int       `json:"today_views" gorm:"default:0"`          // 今日访问量
	TotalClicks     int       `json:"total_clicks" gorm:"default:0"`         // 总点击量
	DailyClicks     int       `json:"daily_clicks" gorm:"default:0"`         // 今日点击量
	ImageURL        string    `json:"image_url" gorm:"size:500"`             // 图片URL
	EntryURL        string    `json:"entry_url" gorm:"size:500"`             // 入口URL
	LandingURL      string    `json:"landing_url" gorm:"size:500"`           // 落地URL
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime"`      // 创建时间
	UpdatedAt       time.Time `json:"updated_at" gorm:"autoUpdateTime"`      // 更新时间

	// 关联
	ShortDomain   *DomainPool  `json:"short_domain" gorm:"foreignKey:ShortDomainID;references:id"`
	EntryDomain   *DomainPool  `json:"entry_domain" gorm:"foreignKey:EntryDomainID;references:id"`
	LandingDomain *DomainPool  `json:"landing_domain" gorm:"foreignKey:LandingDomainID;references:id"`
	QRCodes       []LiveCodeQR `json:"qr_codes" gorm:"foreignKey:LiveCodeID"`
}

// TableName 返回表名
func (LiveCode) TableName() string {
	return "live_codes"
}

// BeforeCreate GORM钩子，在创建前生成ID
func (l *LiveCode) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// GetFullShortLink 获取完整的短链
func (l *LiveCode) GetFullShortLink() string {
	if l.ShortDomain != nil {
		protocol := "http"
		if l.ShortDomain.Port == 443 {
			protocol = "https"
		}
		return protocol + "://" + l.ShortDomain.Domain + "/" + l.ShortLink
	}
	return ""
}

// GetFullEntryLink 获取完整的入口链接
func (l *LiveCode) GetFullEntryLink() string {
	if l.EntryDomain != nil {
		protocol := "http"
		if l.EntryDomain.Port == 443 {
			protocol = "https"
		}
		return protocol + "://" + l.EntryDomain.Domain + "/entry/" + l.ShortLink
	}
	return ""
}

// GetFullLandingLink 获取完整的落地链接
func (l *LiveCode) GetFullLandingLink() string {
	if l.LandingDomain != nil {
		protocol := "http"
		if l.LandingDomain.Port == 443 {
			protocol = "https"
		}
		return protocol + "://" + l.LandingDomain.Domain + "/landing/" + l.ShortLink
	}
	return ""
}
