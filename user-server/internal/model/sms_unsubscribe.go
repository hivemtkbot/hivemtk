package model

import (
	"time"

	"gorm.io/gorm"
)

// SmsUnsubscribe 短信退订记录
//
// 合规依据：
//   - 《通信短消息服务管理规定》第十八条：短消息服务提供者、短消息内容提供者
//     未经用户同意或者请求，不得向其发送商业性短消息；用户明确表示拒绝接收的，
//     应当停止向其发送。本表记录用户退订请求，发送前必须检查 IsUnsubscribed。
//   - 退订关键词识别：回复 TD / 退订 / T退 / 取消 / N / Q 等关键词自动加入退订名单。
//   - 重新订阅：用户可通过客服或管理界面重新订阅，删除退订记录即可。
type SmsUnsubscribe struct {
	ID              uint           `gorm:"primarykey" json:"id"`
	Phone           string         `gorm:"type:varchar(20);uniqueIndex;not null" json:"phone"`
	Reason          string         `gorm:"type:varchar(255)" json:"reason"`
	UnsubscribedAt  time.Time      `gorm:"not null" json:"unsubscribedAt"`
	SourceMessageID string         `gorm:"type:varchar(64)" json:"sourceMessageId"`
	KeywordMatched  string         `gorm:"type:varchar(20)" json:"keywordMatched"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 显式指定表名
func (*SmsUnsubscribe) TableName() string {
	return "sms_unsubscribes"
}

// BeforeCreate 创建前补全退订时间
func (s *SmsUnsubscribe) BeforeCreate(tx *gorm.DB) error {
	if s.UnsubscribedAt.IsZero() {
		s.UnsubscribedAt = time.Now()
	}
	return nil
}

