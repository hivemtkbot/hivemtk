package model

import (
	"time"
)

// BackupStatus 备份状态
type BackupStatus string

const (
	BackupStatusPending   BackupStatus = "pending"
	BackupStatusRunning   BackupStatus = "running"
	BackupStatusCompleted BackupStatus = "completed"
	BackupStatusFailed    BackupStatus = "failed"
)

// BackupType 备份类型
type BackupType string

const (
	BackupTypeFull        BackupType = "full"
	BackupTypeIncremental BackupType = "incremental"
)

// Backup 备份记录
type Backup struct {
	ID           uint         `gorm:"primaryKey;autoIncrement" json:"id"`
	BackupName   string       `gorm:"type:varchar(100);not null" json:"backup_name"`
	BackupType   BackupType   `gorm:"type:varchar(20);default:'full'" json:"backup_type"`
	Status       BackupStatus `gorm:"type:varchar(20);default:'pending'" json:"status"`
	FilePath     string       `gorm:"type:varchar(500)" json:"file_path"`
	FileSize     int64        `json:"file_size"`
	ErrorMessage string       `gorm:"type:text" json:"error_message"`
	StartedAt    time.Time    `json:"started_at"`
	CompletedAt  *time.Time   `json:"completed_at"`
	CreatedBy    uint         `json:"created_by"`
	CreatedAt    time.Time    `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (Backup) TableName() string {
	return "backups"
}

// RestoreRecord 恢复记录
type RestoreRecord struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	BackupID     uint      `json:"backup_id"`
	BackupName   string    `gorm:"type:varchar(100)" json:"backup_name"`
	Status       string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	ErrorMessage string    `gorm:"type:text" json:"error_message"`
	RestoredAt   time.Time `json:"restored_at"`
	CreatedBy    uint      `json:"created_by"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (RestoreRecord) TableName() string {
	return "restore_records"
}
