package model

import "time"

// FeatureFlag 功能开关（K2，对标 Unleash/GrowthBook 管理端最小完备集）
//
// 评估语义（service 层实现）：
//  1. !Enabled            → off（kill switch 紧急全关）
//  2. rollout 0-100       → FNV-1a(key:contextKey)%100 < RolloutPercentage 灰度放量（确定性粘性分桶）
//  3. not found           → off, reason=not_found（前端降级默认值）
type FeatureFlag struct {
	ID                uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Key               string     `gorm:"type:varchar(100);not null;uniqueIndex" json:"key"`
	Name              string     `gorm:"type:varchar(200)" json:"name"`
	Description       string     `gorm:"type:varchar(500)" json:"description"`
	Enabled           bool       `gorm:"default:false;index" json:"enabled"`
	RolloutPercentage int        `gorm:"default:100" json:"rollout_percentage"` // 0-100
	Payload           string     `gorm:"type:text" json:"payload"`              // 开启时返回值（JSON 字符串，可空）
	Tags              string     `gorm:"type:varchar(200)" json:"tags"`
	LastEvaluatedAt   *time.Time `json:"last_evaluated_at,omitempty"`
	CreatedBy         uint       `json:"created_by"`
	CreatedAt         time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (FeatureFlag) TableName() string { return "feature_flags" }

// FeatureFlagAuditLog Flag 变更审计（谁/何时/改了什么）
type FeatureFlagAuditLog struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	FlagID    uint      `gorm:"index" json:"flag_id"`
	FlagKey   string    `gorm:"type:varchar(100);index" json:"flag_key"`
	Action    string    `gorm:"type:varchar(50);not null" json:"action"` // create/update/enable/disable/rollout/delete
	ActorID   uint      `json:"actor_id"`
	Detail    string    `gorm:"type:varchar(500)" json:"detail"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (FeatureFlagAuditLog) TableName() string { return "feature_flag_audit_logs" }

// FeatureFlagEvalLog 评估/曝光日志（Unleash impression data：供 AB 分析与审计）
type FeatureFlagEvalLog struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	FlagKey   string    `gorm:"type:varchar(100);index" json:"flag_key"`
	ContextID string    `gorm:"type:varchar(100);index" json:"context_id"` // user_id/one_id/anonymous
	Enabled   bool      `json:"enabled"`
	Value     string    `gorm:"type:text" json:"value"`
	Reason    string    `gorm:"type:varchar(100)" json:"reason"` // disabled/not_found/rollout/rollout_excluded/payload
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (FeatureFlagEvalLog) TableName() string { return "feature_flag_eval_logs" }
