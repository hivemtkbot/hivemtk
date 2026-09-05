package model

import (
	"time"

	"gorm.io/gorm"
)

// 邮件追踪事件类型
const (
	EmailEventTypeOpen        = "open"
	EmailEventTypeClick       = "click"
	EmailEventTypeUnsubscribe = "unsubscribe"
	EmailEventTypeBounce      = "bounce"
)

// EmailTrackingEvent 邮件追踪事件
//
// 每个 (email + job_id) 在一次营销任务中产生多条事件：open / click / unsubscribe / bounce
// event_id 由服务层生成（UUID），保证 webhook 重放幂等
type EmailTrackingEvent struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	EventID   string         `gorm:"type:varchar(36);uniqueIndex;not null" json:"eventId"`
	Email     string         `gorm:"type:varchar(255);index;not null" json:"email"`
	JobID     string         `gorm:"type:varchar(36);index" json:"jobId"`
	EventType string         `gorm:"type:varchar(20);index;not null" json:"eventType"`
	UserAgent string         `gorm:"type:varchar(512)" json:"userAgent"`
	IP        string         `gorm:"type:varchar(64)" json:"ip"`
	Timestamp time.Time      `gorm:"not null" json:"timestamp"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 显式指定表名
func (*EmailTrackingEvent) TableName() string {
	return "email_tracking_events"
}

// BeforeCreate 创建前补全时间戳
func (e *EmailTrackingEvent) BeforeCreate(tx *gorm.DB) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	return nil
}

// EmailJobMetric 邮件任务指标（每 job_id 一条）
//
// open_rate  = total_opened / total_sent  * 100（保留 2 位小数）
// click_rate = total_clicked / total_sent * 100
type EmailJobMetric struct {
	ID                uint           `gorm:"primarykey" json:"id"`
	JobID             string         `gorm:"type:varchar(36);uniqueIndex;not null" json:"jobId"`
	TotalSent         int64          `gorm:"default:0" json:"totalSent"`
	TotalOpened       int64          `gorm:"default:0" json:"totalOpened"`
	TotalClicked      int64          `gorm:"default:0" json:"totalClicked"`
	TotalBounced      int64          `gorm:"default:0" json:"totalBounced"`
	TotalUnsubscribed int64          `gorm:"default:0" json:"totalUnsubscribed"`
	OpenRate          float64        `gorm:"type:numeric(5,2);default:0" json:"openRate"`
	ClickRate         float64        `gorm:"type:numeric(5,2);default:0" json:"clickRate"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 显式指定表名
func (*EmailJobMetric) TableName() string {
	return "email_job_metrics"
}
