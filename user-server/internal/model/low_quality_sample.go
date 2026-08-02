package model

import (
	"time"
)

// LowQualitySampleType 低质样本类型
type LowQualitySampleType string

const (
	LowQualitySamplePersona        LowQualitySampleType = "persona"         // 拟人度不达标
	LowQualitySampleCompliance     LowQualitySampleType = "compliance"      // 合规不达标
	LowQualitySampleNaturalness    LowQualitySampleType = "naturalness"     // 自然度不达标
	LowQualitySampleRelevance      LowQualitySampleType = "relevance"       // 相关性不达标
	LowQualitySampleManualReview   LowQualitySampleType = "manual_review"   // 转人工
	LowQualitySampleRetryExhausted LowQualitySampleType = "retry_exhausted" // 重试耗尽
)

// LowQualitySample G6 低质样本（用于后续训练 / 人工分析）
// 对应 PRD §5.2 G5：3 次仍不达标 → 转人工 + 记录低质样本
// 验收：低质样本自动收集用于后续训练
type LowQualitySample struct {
	ID              uint64               `gorm:"primaryKey;autoIncrement" json:"id"`
	CustomerID      string               `gorm:"type:varchar(64);index" json:"customer_id"`
	SessionID       string               `gorm:"type:varchar(64);index" json:"session_id"`
	SampleType      LowQualitySampleType `gorm:"type:varchar(32);not null;index" json:"sample_type"`
	CustomerMessage string               `gorm:"type:text" json:"customer_message"`
	AIReply         string               `gorm:"type:text;not null" json:"ai_reply"`
	Persona         string               `gorm:"type:varchar(128)" json:"persona"`
	Industry        string               `gorm:"type:varchar(64)" json:"industry"`
	Platform        string               `gorm:"type:varchar(32)" json:"platform"`
	Intent          string               `gorm:"type:varchar(32)" json:"intent"`
	// 维度得分（JSON 字符串）：{"naturalness":0.6,"relevance":0.9,...}
	DimensionScores string  `gorm:"type:jsonb;default:'{}'" json:"dimension_scores"`
	TotalScore      float64 `gorm:"default:0" json:"total_score"`
	Threshold       float64 `gorm:"default:0.85" json:"threshold"`
	AttemptCount    int     `gorm:"default:1" json:"attempt_count"`
	// 所有候选回复（JSON 数组字符串）：["reply1","reply2","reply3"]
	CandidateReplies string     `gorm:"type:jsonb;default:'[]'" json:"candidate_replies"`
	Handled          bool       `gorm:"default:false;index" json:"handled"` // 是否已人工处理
	HandledBy        string     `gorm:"type:varchar(64)" json:"handled_by"`
	HandledAt        *time.Time `gorm:"index" json:"handled_at"`
	HandledNote      string     `gorm:"type:text" json:"handled_note"`
	CreatedAt        time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 表名
func (LowQualitySample) TableName() string { return "low_quality_samples" }
