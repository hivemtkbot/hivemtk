package model

// intent_log.go 精细意图识别日志模型
//
// 五层架构归属: L5 数据层
// 设计依据: PRD § 缺口修复（8 大类 + 7 子类精细意图识别）
// 私域独立部署: 无 merchant_id 字段
//
// 表: intent_logs（与 intent_records 共存，互不依赖）
//   - intent_records: 旧版规则+LLM 识别记录（已有）
//   - intent_logs:    新版 8 大类+7 子类精细识别记录（本文件）
//
// 字段说明:
//   - intent_major: 8 大意图类之一（consult/price_inquiry/objection/...）
//   - intent_minor: 大类下的 7 子类细分（如 price_inquiry.discount_request）
//   - method: rule（规则匹配，快速）/ llm（LLM 识别，慢速）
//   - latency_ms: 识别耗时，用于性能监控
//   - confidence < 0.6 时触发 LLM 二次识别

import "time"

// IntentLog 精细意图识别日志
type IntentLog struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CustomerID  string    `gorm:"type:varchar(64);index" json:"customer_id"`
	SessionID   string    `gorm:"type:varchar(50);index" json:"session_id"`
	Message     string    `gorm:"type:text;not null" json:"message"`
	IntentMajor string    `gorm:"type:varchar(32);not null;index" json:"intent_major"`
	IntentMinor string    `gorm:"type:varchar(32);not null;index" json:"intent_minor"`
	Confidence  float64   `gorm:"type:decimal(5,4);not null" json:"confidence"`
	Method      string    `gorm:"type:varchar(16);not null" json:"method"` // rule / llm / hybrid
	LatencyMs   int       `gorm:"default:0" json:"latency_ms"`
	Reasoning   string    `gorm:"type:text" json:"reasoning,omitempty"` // LLM 推理过程（仅 method=llm 时填充）
	TraceID     string    `gorm:"type:varchar(64);index" json:"trace_id,omitempty"`
	Timestamp   time.Time `gorm:"index" json:"timestamp"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName GORM 表名
func (IntentLog) TableName() string { return "intent_logs" }
