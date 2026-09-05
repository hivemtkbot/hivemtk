package model

import "time"

// WechatMessage 微信公众号消息记录
type WechatMessage struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	AccountID  uint      `gorm:"index;not null" json:"account_id"`
	FromUser   string    `gorm:"type:varchar(128);index" json:"from_user"`
	ToUser     string    `gorm:"type:varchar(64)" json:"to_user"`
	MsgType    string    `gorm:"type:varchar(32)" json:"msg_type"`
	Content    string    `gorm:"type:text" json:"content"`
	MsgID      string    `gorm:"type:varchar(64);uniqueIndex" json:"msg_id"`
	RawXML     string    `gorm:"type:text" json:"-"`
	IsOutgoing bool      `gorm:"default:false" json:"is_outgoing"`
	CreatedAt  time.Time `json:"created_at"`
}

func (WechatMessage) TableName() string {
	return "wechat_messages"
}
