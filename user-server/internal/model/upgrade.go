package model

import (
	"time"
)

// UpgradeTask 升级任务
type UpgradeTask struct {
	ID              uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	FromVersion     string     `gorm:"type:varchar(50);not null" json:"from_version"`
	ToVersion       string     `gorm:"type:varchar(50);not null" json:"to_version"`
	Status          string     `gorm:"type:varchar(20);default:'pending'" json:"status"` 
	Progress        int        `gorm:"default:0" json:"progress"`                        
	TotalSteps      int        `gorm:"default:0" json:"total_steps"`
	CurrentStep     int        `gorm:"default:0" json:"current_step"`
	CurrentStepDesc string     `gorm:"type:varchar(255)" json:"current_step_desc"`
	ErrorMessage    string     `gorm:"type:text" json:"error_message"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (UpgradeTask) TableName() string {
	return "upgrade_tasks"
}

// MigrationRecord 迁移记录
type MigrationRecord struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Version    string    `gorm:"type:varchar(50);unique;not null" json:"version"`
	Name       string    `gorm:"type:varchar(100)" json:"name"`
	Type       string    `gorm:"type:varchar(20)" json:"type"` 
	Status     string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	ExecutedAt time.Time `json:"executed_at"`
	ExecutedBy string    `gorm:"type:varchar(50)" json:"executed_by"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (MigrationRecord) TableName() string {
	return "migration_records"
}

// VersionInfo 版本信息
type VersionInfo struct {
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ReleaseDate time.Time `json:"release_date"`
	IsCurrent   bool      `json:"is_current"`
	IsLatest    bool      `json:"is_latest"`
	Changes     []string  `json:"changes"`
}

// MigrationCheckpoint 迁移检查点
type MigrationCheckpoint struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Checkpoint string    `gorm:"type:varchar(50);unique;not null" json:"checkpoint"`
	Data       string    `gorm:"type:text" json:"data"` 
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (MigrationCheckpoint) TableName() string {
	return "migration_checkpoints"
}

