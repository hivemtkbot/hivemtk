package model

import (
	"github.com/google/uuid"
	"marketing/internal/pkg/utils/bcrypt"
	_type "marketing/internal/pkg/utils/type"
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID         string               `gorm:"type:varchar(36);primaryKey" json:"id"`
	Username   string               `gorm:"type:varchar(50);uniqueIndex;not null" json:"username"`
	Password   string               `gorm:"type:varchar(255);not null" json:"-"`
	Email      string               `gorm:"type:varchar(100);uniqueIndex" json:"email"`
	RealName   string               `gorm:"type:varchar(50)" json:"real_name"`
	Phone      string               `gorm:"type:varchar(20)" json:"phone"`
	Avatar     string               `gorm:"type:varchar(255)" json:"avatar"`
	Role       string               `gorm:"type:varchar(20);default:'user'" json:"role"` // admin, user
	Status     _type.UserStatusType `gorm:"status;default:1" json:"status"`
	TgID       int64                `gorm:"tg_id" json:"tg_id"`
	CreateTime int64                `gorm:"autoCreateTime" json:"create_time"`
	UpdateTime int64                `gorm:"autoUpdateTime" json:"update_time"`
	AccountID  string               `gorm:"type:varchar(36)" json:"account_id"`
	FirstName  string               `gorm:"type:varchar(255)" json:"first_name"`
	LastName   string               `gorm:"type:varchar(255)" json:"last_name"`
	UserName   string               `gorm:"type:varchar(255)" json:"user_name"`
}

func (u *User) TableName() string {
	return "user"
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	// 生成 UUID
	if u.ID == "" {
		u.ID = uuid.New().String()
	}

	// 加密密码
	if u.Password != "" {
		hashedPassword, err := bcrypt.HashPassword(u.Password)
		if err != nil {
			return err
		}
		u.Password = hashedPassword
	}
	return nil
}

// UserTag 用户标签
type UserTag struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    string    `gorm:"type:varchar(36);index;not null" json:"user_id"`
	TagName   string    `gorm:"type:varchar(50);index;not null" json:"tag_name"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (UserTag) TableName() string {
	return "user_tags"
}
