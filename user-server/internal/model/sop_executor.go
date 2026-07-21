package model

import "time"

// ============================================================================
// SOP 执行器相关模型（P0-1 SOP 节点执行器完善设计）
// ----------------------------------------------------------------------------
// 设计依据：docs/核心链路优化.md 第十三章
// 私域独立部署：无 merchant_id 字段
// 五层架构：本文件位于 L5 数据层（Model），被 L3 业务层（Service）和 L4 能力层引用
// ============================================================================

// SOPExecEvent SOP 执行事件流（审计 + 幂等性 + 调试回放）
type SOPExecEvent struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ExecutionID  uint      `gorm:"index;not null" json:"execution_id"`
	SOPID        uint      `gorm:"not null" json:"sop_id"`
	NodeID       string    `gorm:"type:varchar(50);not null" json:"node_id"`
	NodeType     string    `gorm:"type:varchar(30);not null" json:"node_type"`
	EventType    string    `gorm:"type:varchar(30);not null" json:"event_type"`
	Attempt      int       `gorm:"default:0" json:"attempt"`
	Status       string    `gorm:"type:varchar(20)" json:"status"`
	Input        JSONMap   `gorm:"type:jsonb" json:"input"`
	Output       JSONMap   `gorm:"type:jsonb" json:"output"`
	SideEffects  JSONArray `gorm:"type:jsonb" json:"side_effects"`
	ErrorMessage string    `gorm:"type:text" json:"error_message"`
	LatencyMs    int       `json:"latency_ms"`
	TokensUsed   int       `json:"tokens_used"`
	TraceID      string    `gorm:"type:varchar(64);index" json:"trace_id"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 指定表名
func (SOPExecEvent) TableName() string { return "sop_exec_events" }

// SOPTimer SOP 定时器（wait/timer 节点的长任务实现）
type SOPTimer struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	ExecutionID uint       `gorm:"index;not null" json:"execution_id"`
	NodeID      string     `gorm:"type:varchar(50);not null" json:"node_id"`
	WaitEvent   string     `gorm:"type:varchar(30);not null" json:"wait_event"`
	WaitUntil   time.Time  `gorm:"not null" json:"wait_until"`
	Status      string     `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	Payload     JSONMap    `gorm:"type:jsonb" json:"payload"`
	FiredAt     *time.Time `json:"fired_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (SOPTimer) TableName() string { return "sop_timers" }

// SOPOutbox SOP Outbox 事件（解耦执行与外部副作用）
type SOPOutbox struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	ExecutionID uint       `gorm:"index;not null" json:"execution_id"`
	EventType   string     `gorm:"type:varchar(50);not null" json:"event_type"`
	Payload     JSONMap    `gorm:"type:jsonb;not null" json:"payload"`
	Processed   bool       `gorm:"default:false;index" json:"processed"`
	ProcessedAt *time.Time `json:"processed_at"`
	RetryCount  int        `gorm:"default:0" json:"retry_count"`
	CreatedAt   time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 指定表名
func (SOPOutbox) TableName() string { return "sop_outbox" }
