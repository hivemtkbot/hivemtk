package model

import "time"

// SecurityAuditResult 安全审计结果
type SecurityAuditResult struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	AuditName    string     `gorm:"type:varchar(200);not null" json:"audit_name"`
	Status       string     `gorm:"type:varchar(20);default:'running';index" json:"status"` // running/completed/failed
	TotalChecks  int        `gorm:"default:0" json:"total_checks"`
	PassedCount  int        `gorm:"default:0" json:"passed_count"`
	FailedCount  int        `gorm:"default:0" json:"failed_count"`
	WarningCount int        `gorm:"default:0" json:"warning_count"`
	Score        float64    `gorm:"type:decimal(5,2);default:0" json:"score"` // 0-100
	Results      JSONArray  `gorm:"type:text" json:"results"`
	ErrorMessage string     `gorm:"type:text" json:"error_message"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	CreatedAt    time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

func (SecurityAuditResult) TableName() string { return "security_audit_results" }
