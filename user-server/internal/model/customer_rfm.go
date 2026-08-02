// Package model 数据层模型 - 客户 RFM 分层
//
// 五层架构归属: L3 数据模型层
// 设计依据: 缺口修复 - RFM 联动分层
// 私域独立部署: 无 merchant_id 字段
package model

import (
	"time"
)

// CustomerRFM 客户 RFM 维度记录
//
//	R (Recency):   最近一次消费距今天数（越小越好）
//	F (Frequency): 累计消费次数（越大越好）
//	M (Monetary):  累计消费金额（越大越好，单位：分）
//
// 每项独立计算 1-5 分（5 最佳），加权后聚合为综合 RFM 分（0-100）。
// 综合分层：champion(8+)/loyal(6-7)/potential(5)/at_risk(3-4)/churn(<=2)
type CustomerRFM struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	CustomerID string `gorm:"type:varchar(36);not null;uniqueIndex" json:"customer_id"`
	UnifiedID  string `gorm:"type:varchar(64);default:'';index" json:"unified_id"`

	// 原始指标
	RecencyDays   int   `gorm:"type:int;not null;default:9999" json:"recency_days"`    // 距今天数
	Frequency     int   `gorm:"type:int;not null;default:0" json:"frequency"`          // 累计订单数
	MonetaryTotal int64 `gorm:"type:bigint;not null;default:0" json:"monetary_total"`  // 累计金额（分）
	AvgOrderValue int64 `gorm:"type:bigint;not null;default:0" json:"avg_order_value"` // 客单价（分）

	// 评分（1-5）
	RScore int `gorm:"type:int;not null;default:1" json:"r_score"`
	FScore int `gorm:"type:int;not null;default:1" json:"f_score"`
	MScore int `gorm:"type:int;not null;default:1" json:"m_score"`

	// 综合 RFM 分（0-100，加权：R 30% + F 30% + M 40%）
	CompositeScore int    `gorm:"type:int;not null;default:0" json:"composite_score"`
	Segment        string `gorm:"type:varchar(16);not null;default:'churn'" json:"segment"`

	// 流失风险
	ChurnRiskLevel string     `gorm:"type:varchar(8);not null;default:'high'" json:"churn_risk_level"` // low/medium/high
	ChurnScore     int        `gorm:"type:int;not null;default:100" json:"churn_score"`                // 0-100，越高越可能流失
	LastActiveAt   *time.Time `gorm:"index" json:"last_active_at"`

	// 元数据
	ComputedAt time.Time `gorm:"not null;default:NOW()" json:"computed_at"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (CustomerRFM) TableName() string { return "customer_rfm" }

// RFM 分层常量
const (
	RFMSegmentChampion  = "champion"  // 高价值忠诚客户：R=5, F>=4
	RFMSegmentLoyal     = "loyal"     // 忠诚客户：R>=4, F>=3
	RFMSegmentPotential = "potential" // 潜力客户：R>=3, F>=2 或 M 较高
	RFMSegmentAtRisk    = "at_risk"   // 风险客户：R=2 或 (F>=2, M>=3)
	RFMSegmentChurn     = "churn"     // 流失客户：R=1 或 R=2 且 F<=1
)

// RFMSegmentDescriptions 分层描述
var RFMSegmentDescriptions = map[string]string{
	RFMSegmentChampion:  "高价值忠诚客户 - 优先维护和提升客单价",
	RFMSegmentLoyal:     "忠诚客户 - 保持活跃并推送新品",
	RFMSegmentPotential: "潜力客户 - 加强触达和复购引导",
	RFMSegmentAtRisk:    "风险客户 - 主动关怀并查明原因",
	RFMSegmentChurn:     "流失客户 - 进入挽回队列，启动召回",
}

// RecoveryQueue 流失挽回队列
// 触达任务，由 1 个或多个渠道触达 + 多个 step 组成
type RecoveryQueue struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	CustomerID string `gorm:"type:varchar(36);not null;index" json:"customer_id"`
	UnifiedID  string `gorm:"type:varchar(64);default:'';index" json:"unified_id"`
	Account    string `gorm:"type:varchar(255);default:''" json:"account"`

	// 流转
	Reason   string `gorm:"type:varchar(32);not null;default:'churn'" json:"reason"` // churn / downgrade / complaint
	Strategy string `gorm:"type:varchar(32);not null;default:'sms_coupon'" json:"strategy"`
	Priority int    `gorm:"type:int;not null;default:5" json:"priority"` // 1-10, 越小越优先
	Stage    string `gorm:"type:varchar(16);not null;default:'queued';index" json:"stage"`

	// 触达记录
	Attempts      int        `gorm:"type:int;not null;default:0" json:"attempts"`
	MaxAttempts   int        `gorm:"type:int;not null;default:3" json:"max_attempts"`
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	LastChannel   string     `gorm:"type:varchar(32);default:''" json:"last_channel"`
	LastResult    string     `gorm:"type:varchar(255);default:''" json:"last_result"`

	// 转化追踪
	RecoveredAt   *time.Time `json:"recovered_at"`
	RecoveryValue int64      `gorm:"type:bigint;not null;default:0" json:"recovery_value"`

	// 元数据
	MetaJSON  string    `gorm:"type:text;default:'{}'" json:"meta_json"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (RecoveryQueue) TableName() string { return "recovery_queue" }

// 阶段常量
const (
	RecoveryStageQueued    = "queued"
	RecoveryStageRunning   = "running"
	RecoveryStageSucceed   = "succeed"
	RecoveryStageFailed    = "failed"
	RecoveryStageCancelled = "cancelled"
)
