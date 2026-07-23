package model

// rag_alert.go RAG 风控预警模型层（C 域 P1 缺口 #3）
//
// 五层架构归属: L5 数据层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6.3 风控预警
//
// 表结构:
//   1. rag_alerts - 预警记录（按类型/严重度/状态管理）
//
// 预警条件:
//   a) 召回率 5 分钟均值 < 0.3
//   b) 向量化失败率 > 10%
//   c) 检索 P99 延迟 > 2 秒
//   d) 知识库空命中（0 docs）占比 > 20%
//
// 严重度分级:
//   - message: 信息（首次出现）
//   - warning: 警告（持续 3 个窗口）
//   - critical: 严重（持续 6 个窗口或单次极端值）
//
// 私域独立部署: 无 merchant_id 字段

import "time"

// RagAlertSeverity 预警严重度
type RagAlertSeverity string

const (
	// RagAlertSeverityMessage 信息级（首次出现）
	RagAlertSeverityMessage RagAlertSeverity = "message"
	// RagAlertSeverityWarning 警告级（持续异常）
	RagAlertSeverityWarning RagAlertSeverity = "warning"
	// RagAlertSeverityCritical 严重级（极端异常）
	RagAlertSeverityCritical RagAlertSeverity = "critical"
)

// RagAlertType 预警类型
type RagAlertType string

const (
	// RagAlertTypeLowRecall 召回率过低（5 分钟均值 < 0.3）
	RagAlertTypeLowRecall RagAlertType = "low_recall"
	// RagAlertTypeEmbeddingFailure 向量化失败率过高（> 10%）
	RagAlertTypeEmbeddingFailure RagAlertType = "embedding_failure"
	// RagAlertTypeHighLatency 检索 P99 延迟过高（> 2 秒）
	RagAlertTypeHighLatency RagAlertType = "high_latency"
	// RagAlertTypeZeroHit 知识库空命中占比过高（> 20%）
	RagAlertTypeZeroHit RagAlertType = "zero_hit"
)

// RagAlert RAG 预警记录
type RagAlert struct {
	ID          int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	AlertType   string     `gorm:"column:alert_type;size:32;not null;index:idx_rag_alerts_type,priority:1" json:"alert_type"`
	Severity    string     `gorm:"column:severity;size:16;not null;default:'message';index:idx_rag_alerts_severity" json:"severity"`
	MetricValue float64    `gorm:"column:metric_value;type:decimal(10,4);not null" json:"metric_value"`
	Threshold   float64    `gorm:"column:threshold;type:decimal(10,4);not null" json:"threshold"`
	Message     string     `gorm:"column:message;type:text;not null" json:"message"`
	WindowStart time.Time  `gorm:"column:window_start;not null" json:"window_start"`
	WindowEnd   time.Time  `gorm:"column:window_end;not null" json:"window_end"`
	Resolved    bool       `gorm:"column:resolved;not null;default:false;index:idx_rag_alerts_resolved" json:"resolved"`
	ResolvedAt  *time.Time `gorm:"column:resolved_at" json:"resolved_at"`
	ResolvedBy  string     `gorm:"column:resolved_by;size:64" json:"resolved_by"`
	ResolveNote string     `gorm:"column:resolve_note;type:text" json:"resolve_note"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;default:now();index:idx_rag_alerts_type,priority:2;index:idx_rag_alerts_created" json:"created_at"`
}

// TableName 表名
func (RagAlert) TableName() string { return "rag_alerts" }
