package model

import "time"

// WechatMessage 微信公众号消息记录
type WechatMessage struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	AccountID   uint      `gorm:"index;not null" json:"account_id"`
	FromUser    string    `gorm:"type:varchar(128);index" json:"from_user"`     // 用户 OpenID
	ToUser      string    `gorm:"type:varchar(64)" json:"to_user"`             // 开发者微信号
	MsgType     string    `gorm:"type:varchar(32)" json:"msg_type"`            // text/image/voice/video/location/link
	Content     string    `gorm:"type:text" json:"content"`                    // 消息内容
	MsgID       string    `gorm:"type:varchar(64);uniqueIndex" json:"msg_id"` // 微信消息 ID
	RawXML      string    `gorm:"type:text" json:"-"`                         // 原始 XML
	IsOutgoing  bool      `gorm:"default:false" json:"is_outgoing"`           // 是否主动发送
	CreatedAt   time.Time `json:"created_at"`
}

func (WechatMessage) TableName() string {
	return "wechat_messages"
}