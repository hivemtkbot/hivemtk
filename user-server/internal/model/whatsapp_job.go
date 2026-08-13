package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WhatsappJobStatus string

const (
	WhatsappJobPending  WhatsappJobStatus = "pending"
	WhatsappJobRunning  WhatsappJobStatus = "running"
	WhatsappJobFinished WhatsappJobStatus = "finished"
	WhatsappJobFailed   WhatsappJobStatus = "failed"
)

type WhatsappJob struct {
	ID        uuid.UUID         `gorm:"type:char(36);primary_key" json:"id"`
	DraftID   uuid.UUID         `gorm:"type:char(36);index" json:"draft_id"`
	Status    WhatsappJobStatus `gorm:"type:varchar(20);default:'pending'" json:"status"`
	Total     int64             `gorm:"default:0" json:"total"`
	Success   int64             `gorm:"default:0" json:"success"`
	Failed    int64             `gorm:"default:0" json:"failed"`
	CreatedAt time.Time         `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time         `gorm:"autoUpdateTime" json:"updated_at"`
}

func (m *WhatsappJob) TableName() string {
	return "whatsapp_jobs"
}

func (m *WhatsappJob) BeforeCreate(tx *gorm.DB) error {
	if m.ID == (uuid.UUID{}) {
		m.ID = uuid.New()
	}
	return nil
}

type WhatsappJobDetailStatus string

const (
	WhatsappJobDetailPending WhatsappJobDetailStatus = "pending"
	WhatsappJobDetailSuccess WhatsappJobDetailStatus = "success"
	WhatsappJobDetailFailed  WhatsappJobDetailStatus = "failed"
)

type WhatsappJobDetail struct {
	ID        uuid.UUID               `gorm:"type:char(36);primary_key" json:"id"`
	JobID     uuid.UUID               `gorm:"type:char(36);index" json:"job_id"`
	AccountID uuid.UUID               `gorm:"type:char(36);index" json:"account_id"`
	ToJid     string                  `gorm:"type:varchar(100);index" json:"to_jid"`
	Status    WhatsappJobDetailStatus `gorm:"type:varchar(20);default:'pending'" json:"status"`
	ErrorMsg  string                  `gorm:"type:varchar(255)" json:"error_msg"`
	CreatedAt time.Time               `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time               `gorm:"autoUpdateTime" json:"updated_at"`
}

func (m *WhatsappJobDetail) TableName() string {
	return "whatsapp_job_details"
}

func (m *WhatsappJobDetail) BeforeCreate(tx *gorm.DB) error {
	if m.ID == (uuid.UUID{}) {
		m.ID = uuid.New()
	}
	return nil
}
