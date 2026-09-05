package model

import (
	"time"
)

// GeoJobRun GEO 定时任务运行历史（每次执行一条，append-only）
// 支撑管理端任务状态/历史展示与失败排查。
type GeoJobRun struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	JobName    string     `gorm:"column:job_name;size:64;index" json:"job_name"`
	Trigger    string     `gorm:"column:trigger;size:16" json:"trigger"`
	Status     string     `gorm:"column:status;size:16;index" json:"status"`
	Summary    string     `gorm:"column:summary;type:text" json:"summary"`
	Error      string     `gorm:"column:error;type:text" json:"error"`
	StartedAt  time.Time  `gorm:"column:started_at" json:"started_at"`
	FinishedAt *time.Time `gorm:"column:finished_at" json:"finished_at"`
	DurationMs int64      `gorm:"column:duration_ms" json:"duration_ms"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (GeoJobRun) TableName() string { return "geo_job_runs" }
