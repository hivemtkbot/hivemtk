package model

import (
	_type "hivemtk-user/internal/pkg/utils/type"

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
