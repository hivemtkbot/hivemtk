package model

import (
	"time"
)

// TikTokCard TikTok 卡片模型
type TikTokCard struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Title        string    `json:"title" gorm:"size:255;not null"`  
	Description  string    `json:"description" gorm:"size:500"`     
	ImageURL     string    `json:"image_url" gorm:"size:500"`       
	RedirectURL  string    `json:"redirect_url" gorm:"size:500"`    
	ShortLinkID  uint      `json:"short_link_id" gorm:"default:0"`  
	DomainPoolID uint      `json:"domain_pool_id" gorm:"default:0"` 
	Tags         string    `json:"tags" gorm:"size:255"`            
	ViewCount    int       `json:"view_count" gorm:"default:0"`     
	LikeCount    int       `json:"like_count" gorm:"default:0"`     
	ShareCount   int       `json:"share_count" gorm:"default:0"`    
	IsActive     bool      `json:"is_active" gorm:"default:true"`   
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 返回表名
func (TikTokCard) TableName() string {
	return "tiktok_cards"
}

// TikTokCardActivity TikTok 卡片活动记录
type TikTokCardActivity struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	CardID       uint      `json:"card_id" gorm:"index;not null"`
	ActivityType string    `json:"activity_type" gorm:"size:20;index"` 
	UserID       string    `json:"user_id" gorm:"size:64"`
	IPAddress    string    `json:"ip_address" gorm:"size:64"`
	UserAgent    string    `json:"user_agent" gorm:"size:500"`
	Platform     string    `json:"platform" gorm:"size:50"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 返回表名
func (TikTokCardActivity) TableName() string {
	return "tiktok_card_activities"
}

