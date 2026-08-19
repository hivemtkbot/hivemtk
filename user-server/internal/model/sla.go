package model

import "time"

// SLAPolicy SLA 策略（USR-WB-04）
type SLAPolicy struct {
	ID                  uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name                string    `gorm:"type:varchar(100);not null" json:"name"`
	FirstResponseSeconds int      `gorm:"not null" json:"first_response_seconds"`
	ResolutionSeconds   int       `json:"resolution_seconds"`
	AppliesTo           string    `gorm:"type:jsonb" json:"applies_to"` // {channel, customer_tier, business_hours}
	WarnThreshold       int       `gorm:"default:80" json:"warn_threshold"`
	Enabled             bool      `gorm:"default:true" json:"enabled"`
	CreatedAt           time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (SLAPolicy) TableName() string { return "sla_policies" }

// SLAViolation SLA 违规记录
type SLAViolation struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	PolicyID      uint      `gorm:"not null;index" json:"policy_id"`
	SessionID     string    `gorm:"type:varchar(120);not null;index" json:"session_id"`
	ViolationType string    `gorm:"type:varchar(20);not null" json:"violation_type"` // 'first_response' | 'resolution'
	SLASeconds    int       `gorm:"not null" json:"sla_seconds"`
	ActualSeconds int       `gorm:"not null" json:"actual_seconds"`
	DetectedAt    time.Time `gorm:"autoCreateTime;index" json:"detected_at"`
}

func (SLAViolation) TableName() string { return "sla_violations" }
