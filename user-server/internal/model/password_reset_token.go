package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PasswordResetToken 密码重置令牌
type PasswordResetToken struct {
	ID        string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	UserID    string         `gorm:"type:varchar(36);index;not null" json:"user_id"`
	Token     string         `gorm:"type:varchar(64);uniqueIndex;not null" json:"token"`
	ExpiresAt time.Time      `gorm:"index;not null" json:"expires_at"`
	UsedAt    *time.Time     `json:"used_at"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 指定表名
func (PasswordResetToken) TableName() string {
	return "password_reset_tokens"
}

// BeforeCreate 自动填充 ID 和 Token
func (t *PasswordResetToken) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	if t.Token == "" {
		t.Token = uuid.New().String() + uuid.New().String()
	}
	return nil
}
