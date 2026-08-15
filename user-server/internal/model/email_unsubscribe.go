package model

import (
	"time"

	"gorm.io/gorm"
)

// EmailUnsubscribe 邮件退订记录
//
// 合规依据：
//   - 《互联网电子邮件服务管理办法》第十三条：邮件发送者应当提供有效的退订方式，
//     不得以任何方式阻止收件人退订；收到退订请求后应当在 10 个工作日内停止发送。
//   - 模型保留 email 唯一索引，保证同一邮箱仅有一条退订记录；重新订阅时通过
//     ResubscribeEmail 删除记录（合规要求允许用户重新订阅）。
type EmailUnsubscribe struct {
	ID             uint           `gorm:"primarykey" json:"id"`
	Email          string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Reason         string         `gorm:"type:varchar(255)" json:"reason"`
	UnsubscribedAt time.Time      `gorm:"not null" json:"unsubscribedAt"`
	SourceLink     string         `gorm:"type:varchar(512)" json:"sourceLink"`
	IP             string         `gorm:"type:varchar(64)" json:"ip"`
	UA             string         `gorm:"type:varchar(512)" json:"ua"`
	JobID          string         `gorm:"type:varchar(36);index" json:"jobId"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 显式指定表名
func (*EmailUnsubscribe) TableName() string {
	return "email_unsubscribes"
}

// BeforeCreate 创建前补全退订时间
func (e *EmailUnsubscribe) BeforeCreate(tx *gorm.DB) error {
	if e.UnsubscribedAt.IsZero() {
		e.UnsubscribedAt = time.Now()
	}
	return nil
}

