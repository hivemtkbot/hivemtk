package model

import (
	_type "marketing/internal/pkg/utils/type"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Account struct {
	ID               string `gorm:"type:varchar(36);primary_key" json:"id"`
	TgName           string `gorm:"tg_name" json:"tg_name"`
	TgBotToken       string `gorm:"tg_bot_token" json:"tg_bot_token"`
	GroupID          int64  `gorm:"group_id" json:"group_id"`
	Price            string `gorm:"price" json:"price"`
	ProxyEnableProxy bool   `gorm:"default:false" json:"proxy_enable_proxy"`
	ProxyProtoclo    string `gorm:"default:'http'" json:"proxy_protoclo"`
	ProxyHost        string `gorm:"default:'127.0.0.1'" json:"proxy_host"`
	ProxyPort        int    `gorm:"default:1080" json:"proxy_port"`

	// 自动回复平台无头模式设置
	DouyinHeadless      *bool `gorm:"default:true" json:"douyin_headless"`
	KuaishouHeadless    *bool `gorm:"default:true" json:"kuaishou_headless"`
	XiaohongshuHeadless *bool `gorm:"default:true" json:"xiaohongshu_headless"`
	XianyuHeadless      *bool `gorm:"default:true" json:"xianyu_headless"`
	TiktokHeadless      *bool `gorm:"default:true" json:"tiktok_headless"`

	Status     _type.AccountStatusType `gorm:"index;default:1" json:"status"`
	CreateTime int64                   `gorm:"autoCreateTime" json:"create_time"`
	Msg        string                  `gorm:"msg" json:"msg"`
	URL        string                  `gorm:"url" json:"url"`
}

func (a *Account) TableName() string {
	return "account"
}

func (a *Account) BeforeCreate(tx *gorm.DB) error {
	a.ID = uuid.New().String()
	return nil
}
