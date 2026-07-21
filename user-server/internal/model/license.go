package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type LicenseStatus string

const (
	LicenseStatusActive   LicenseStatus = "active"
	LicenseStatusExpired  LicenseStatus = "expired"
	LicenseStatusDisabled LicenseStatus = "disabled"
)

type License struct {
	ID           string        `gorm:"type:varchar(36);primaryKey" json:"id"`
	Key          string        `gorm:"type:varchar(32);uniqueIndex;not null" json:"key"`
	MerchantName string        `gorm:"type:varchar(100);not null" json:"merchant_name"`
	ContactEmail string        `gorm:"type:varchar(100)" json:"contact_email"`
	ContactPhone string        `gorm:"type:varchar(20)" json:"contact_phone"`
	MaxUsers     int           `gorm:"default:1" json:"max_users"`
	MaxStorage   int64         `gorm:"default:1073741824" json:"max_storage"` // 默认1GB
	Features     string        `gorm:"type:text" json:"features"`             // JSON格式存储功能列表
	ExpireAt     time.Time     `gorm:"not null" json:"expire_at"`
	Status       LicenseStatus `gorm:"type:varchar(20);default:'active'" json:"status"`
	Remark       string        `gorm:"type:text" json:"remark"`
	CreatedAt    time.Time     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time     `gorm:"autoUpdateTime" json:"updated_at"`
}

func (l *License) TableName() string {
	return "license"
}

func (l *License) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	if l.Key == "" {
		l.Key = generateLicenseKey()
	}
	return nil
}

// 生成32位许可证密钥
func generateLicenseKey() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const keyLength = 32

	key := make([]byte, keyLength)
	for i := range key {
		key[i] = charset[uuid.New().ID()%uint32(len(charset))]
	}
	return string(key)
}

// 检查许可证是否有效
func (l *License) IsValid() bool {
	return l.Status == LicenseStatusActive && time.Now().Before(l.ExpireAt)
}

// 获取剩余天数
func (l *License) GetRemainingDays() int {
	if l.ExpireAt.Before(time.Now()) {
		return 0
	}
	return int(l.ExpireAt.Sub(time.Now()).Hours() / 24)
}
