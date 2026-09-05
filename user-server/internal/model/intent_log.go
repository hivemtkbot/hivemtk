package model

import "time"

// IntentLog 精细意图识别日志
type IntentLog struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CustomerID  string    `gorm:"type:varchar(64);index" json:"customer_id"`
	SessionID   string    `gorm:"type:varchar(120);index" json:"session_id"`
	Message     string    `gorm:"type:text;not null" json:"message"`
	IntentMajor string    `gorm:"type:varchar(32);not null;index" json:"intent_major"`
	IntentMinor string    `gorm:"type:varchar(32);not null;index" json:"intent_minor"`
	Confidence  float64   `gorm:"type:decimal(5,4);not null" json:"confidence"`
	Method      string    `gorm:"type:varchar(16);not null" json:"method"`
	LatencyMs   int       `gorm:"default:0" json:"latency_ms"`
	Reasoning   string    `gorm:"type:text" json:"reasoning,omitempty"`
	TraceID     string    `gorm:"type:varchar(64);index" json:"trace_id,omitempty"`
	Timestamp   time.Time `gorm:"index" json:"timestamp"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName GORM 表名
func (IntentLog) TableName() string { return "intent_logs" }
