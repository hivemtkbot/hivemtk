package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EmailSmtp struct {
	ID       string `gorm:"type:varchar(36);primary_key" json:"id"`
	Name     string `gorm:"type:varchar(255)" json:"name"`
	Server   string `gorm:"type:varchar(255)" json:"server"`
	Port     int    `gorm:"type:int" json:"port"`
	Username string `gorm:"type:varchar(255)" json:"username"`
	Password string `gorm:"type:varchar(255)" json:"password"`
	Limit    int64  `gorm:"type:int" json:"limit"`
}

func (u *EmailSmtp) TableName() string {
	return "email_smtp"
}

func (u *EmailSmtp) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}

