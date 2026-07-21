package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailJobs 邮件任务模型
type EmailJobs struct {
	ID           uuid.UUID      `gorm:"type:char(36);primary_key" json:"id"`
	Subject      string         `gorm:"size:255;not null" json:"subject"` // 邮件主题
	SendTotal    int64          `gorm:"default:0" json:"send_total"`      // 发送总数
	EmailTotal   int64          `gorm:"default:0" json:"email_total"`     // 邮件总数
	SuccessTotal int64          `gorm:"default:0" json:"success_total"`   // 成功总数
	FailTotal    int64          `gorm:"default:0" json:"fail_total"`      // 失败总数
	ReadTotal    int64          `gorm:"default:0" json:"read_total"`      // 阅读总数
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (*EmailJobs) TableName() string {
	return "email_jobs"
}

// BeforeCreate 创建前生成UUID
func (e *EmailJobs) BeforeCreate(tx *gorm.DB) error {
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	e.UpdatedAt = time.Now()
	return nil
}
