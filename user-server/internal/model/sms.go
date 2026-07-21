package model

import (
	"time"

	"gorm.io/gorm"
)

// SmsConfig 短信配置模型
type SmsConfig struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	DefaultProvider string    `gorm:"type:varchar(50);default:'aliyun'" json:"defaultProvider"` // 默认短信平台
	RateLimit       int       `gorm:"default:100" json:"rateLimit"`                             // 发送频率限制(条/分钟)
	DailyLimit      int       `gorm:"default:10000" json:"dailyLimit"`                          // 每日发送限制(条/天)
	RetryTimes      int       `gorm:"default:3" json:"retryTimes"`                              // 失败重试次数
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// SmsAliyunConfig 阿里云短信配置
type SmsAliyunConfig struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	AccessKeyID     string    `gorm:"type:varchar(100)" json:"accessKeyId"`
	AccessKeySecret string    `gorm:"type:varchar(100)" json:"accessKeySecret"`
	SignName        string    `gorm:"type:varchar(50)" json:"signName"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// SmsTencentConfig 腾讯云短信配置
type SmsTencentConfig struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	SecretID  string    `gorm:"type:varchar(100)" json:"secretId"`
	SecretKey string    `gorm:"type:varchar(100)" json:"secretKey"`
	AppID     string    `gorm:"type:varchar(50)" json:"appId"`
	SignName  string    `gorm:"type:varchar(50)" json:"signName"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SmsHuaweiConfig 华为云短信配置
type SmsHuaweiConfig struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	AppKey    string    `gorm:"type:varchar(100)" json:"appKey"`
	AppSecret string    `gorm:"type:varchar(100)" json:"appSecret"`
	Sender    string    `gorm:"type:varchar(50)" json:"sender"` // 通道号
	Signature string    `gorm:"type:varchar(50)" json:"signature"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SmsRecord 短信发送记录
type SmsRecord struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	Phone     string         `gorm:"type:varchar(20);index" json:"phone"`
	Content   string         `gorm:"type:text" json:"content"`
	Provider  string         `gorm:"type:varchar(50)" json:"provider"`
	Status    string         `gorm:"type:varchar(20);index;default:'pending'" json:"status"` // pending, sending, sent, failed
	ErrorCode string         `gorm:"type:varchar(50)" json:"errorCode"`
	ErrorMsg  string         `gorm:"type:text" json:"errorMsg"`
	SendTime  *time.Time     `json:"sendTime"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// SmsDraft 短信草稿
type SmsDraft struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	Title     string         `gorm:"type:varchar(100);index" json:"title"`
	Content   string         `gorm:"type:text" json:"content"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// SmsJob 短信发送任务
type SmsJob struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	Name         string         `gorm:"type:varchar(100);index" json:"name"`
	Total        int            `gorm:"default:0" json:"total"`                           // 总数量
	Sent         int            `gorm:"default:0" json:"sent"`                            // 已发送数量
	Failed       int            `gorm:"default:0" json:"failed"`                          // 失败数量
	Status       string         `gorm:"type:varchar(20);default:'pending'" json:"status"` // pending, running, paused, completed, failed
	ScheduleTime *time.Time     `json:"scheduleTime"`                                     // 计划发送时间
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// SmsJobDetail 短信任务详情
type SmsJobDetail struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	JobID     uint       `gorm:"index" json:"jobId"`
	Phone     string     `gorm:"type:varchar(20)" json:"phone"`
	Content   string     `gorm:"type:text" json:"content"`
	Status    string     `gorm:"type:varchar(20);default:'pending'" json:"status"`
	ErrorCode string     `gorm:"type:varchar(50)" json:"errorCode"`
	ErrorMsg  string     `gorm:"type:text" json:"errorMsg"`
	SendTime  *time.Time `json:"sendTime"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}
