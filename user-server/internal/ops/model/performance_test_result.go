package model

import (
	"time"

	sysmodel "hivemtk-user/internal/model"
)

// PerformanceTestResult 性能压测结果
type PerformanceTestResult struct {
	ID            uint             `gorm:"primaryKey;autoIncrement" json:"id"`
	TestName      string           `gorm:"type:varchar(200);not null" json:"test_name"`
	TargetURL     string           `gorm:"type:varchar(500);not null" json:"target_url"`
	TestType      string           `gorm:"type:varchar(50);not null" json:"test_type"`
	Concurrency   int              `gorm:"not null" json:"concurrency"`
	DurationSec   int              `gorm:"not null" json:"duration_seconds"`
	Status        string           `gorm:"type:varchar(20);default:'running';index" json:"status"`
	TotalRequests int64            `gorm:"default:0" json:"total_requests"`
	SuccessCount  int64            `gorm:"default:0" json:"success_count"`
	ErrorCount    int64            `gorm:"default:0" json:"error_count"`
	QPS           float64          `gorm:"type:decimal(10,2);default:0" json:"qps"`
	LatencyP50    float64          `gorm:"type:decimal(10,2);default:0" json:"latency_p50"`
	LatencyP95    float64          `gorm:"type:decimal(10,2);default:0" json:"latency_p95"`
	LatencyP99    float64          `gorm:"type:decimal(10,2);default:0" json:"latency_p99"`
	LatencyAvg    float64          `gorm:"type:decimal(10,2);default:0" json:"latency_avg"`
	ErrorRate     float64          `gorm:"type:decimal(5,2);default:0" json:"error_rate"`
	Details       sysmodel.JSONMap `gorm:"type:text" json:"details"`
	ErrorMessage  string           `gorm:"type:text" json:"error_message"`
	StartedAt     *time.Time       `json:"started_at"`
	CompletedAt   *time.Time       `json:"completed_at"`
	CreatedAt     time.Time        `gorm:"autoCreateTime" json:"created_at"`
}

func (PerformanceTestResult) TableName() string { return "performance_test_results" }
