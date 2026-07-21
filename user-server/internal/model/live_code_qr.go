package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LiveCodeQR 活码二维码模型
type LiveCodeQR struct {
	ID         string    `json:"id" gorm:"primaryKey;size:36"`         // 主键
	LiveCodeID string    `json:"live_code_id" gorm:"size:36;not null"` // 活码ID
	QRType     string    `json:"qr_type" gorm:"size:50;not null"`      // 二维码类型
	QRContent  string    `json:"qr_content" gorm:"not null"`           // 二维码内容
	QRTitle    string    `json:"qr_title" gorm:"size:255"`             // 二维码标题
	ImageURL   string    `json:"image_url" gorm:"size:255"`            // 二维码图片URL
	Priority   int       `json:"priority" gorm:"default:1"`            // 优先级
	DailyLimit int       `json:"daily_limit" gorm:"default:200"`       // 每日展示限制
	ExpireDays int       `json:"expire_days" gorm:"default:7"`         // 过期天数
	Status     int       `json:"status" gorm:"default:1"`              // 状态：1-启用，0-禁用
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime"`     // 创建时间
	UpdatedAt  time.Time `json:"updated_at" gorm:"autoUpdateTime"`     // 更新时间
}

// TableName 返回表名
func (LiveCodeQR) TableName() string {
	return "live_code_qrs"
}

// BeforeCreate GORM钩子，在创建前生成UUID
func (l *LiveCodeQR) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// RecordClick 记录一次点击：自增内存计数
//
// 注意：LiveCodeQR 自身没有 TotalClicks 字段，扫码总点击由 LiveCode.TotalClicks 承担
// （RecordClick 调用方在调用后会通过 liveCodeRepo.Update 累加父级计数）。
// 此处仅作为方法扩展点保留真实实现路径，必要时可用于 in-memory rate limit 或埋点。
func (l *LiveCodeQR) RecordClick() {
	// 当前为 noop 实现：保留方法签名满足 service.RecordClick 调用，父级计数在调用方更新
}
