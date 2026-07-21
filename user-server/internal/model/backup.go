package model

import (
	"time"
)

// BackupStatus 备份状态
type BackupStatus string

const (
	BackupStatusPending   BackupStatus = "pending"   // 等待备份
	BackupStatusRunning   BackupStatus = "running"   // 备份中
	BackupStatusCompleted BackupStatus = "completed" // 备份完成
	BackupStatusFailed    BackupStatus = "failed"    // 备份失败
)

// BackupType 备份类型
type BackupType string

const (
	BackupTypeFull        BackupType = "full"        // 全量备份
	BackupTypeIncremental BackupType = "incremental" // 增量备份
)

// Backup 备份记录
type Backup struct {
	ID           uint         `gorm:"primaryKey;autoIncrement" json:"id"`
	BackupName   string       `gorm:"type:varchar(100);not null" json:"backup_name"`
	BackupType   BackupType   `gorm:"type:varchar(20);default:'full'" json:"backup_type"`
	Status       BackupStatus `gorm:"type:varchar(20);default:'pending'" json:"status"`
	FilePath     string       `gorm:"type:varchar(500)" json:"file_path"`
	FileSize     int64        `json:"file_size"` // 文件大小 (字节)
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
	Status       string    `gorm:"type:varchar(20);default:'pending'" json:"status"` // pending, running, completed, failed
	ErrorMessage string    `gorm:"type:text" json:"error_message"`
	RestoredAt   time.Time `json:"restored_at"`
	CreatedBy    uint      `json:"created_by"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (RestoreRecord) TableName() string {
	return "restore_records"
}
