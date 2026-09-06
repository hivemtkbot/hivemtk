package model

import (
	"time"
)

// LongTermMemoryType L2 长期记忆分类
// 对应 PRD §5.2 G5：preference/habit/feedback/event/fact
type LongTermMemoryType string

const (
	LongTermMemoryPreference LongTermMemoryType = "preference"
	LongTermMemoryHabit      LongTermMemoryType = "habit"
	LongTermMemoryFeedback   LongTermMemoryType = "feedback"
	LongTermMemoryEvent      LongTermMemoryType = "event"
	LongTermMemoryFact       LongTermMemoryType = "fact"
)

// LongTermMemorySource 长期记忆来源
type LongTermMemorySource string

const (
	LongTermMemorySourceConversation LongTermMemorySource = "conversation"
	LongTermMemorySourceTool         LongTermMemorySource = "tool"
	LongTermMemorySourceManual       LongTermMemorySource = "manual"
)

// CustomerLongTermMemory G5 L2 长期记忆（pgvector 增强版）
// 对应 PRD §5.2 G5：customer_long_term_memory 表
// 与 4 层记忆系统中 MemoryItem(L2) 平行存在：
//   - MemoryItem(L2)：基础事实/摘要，无向量，简单排序
//   - CustomerLongTermMemory：高级长期记忆，带 embedding 向量，支持语义召回 + 重排序
//
// 验收标准：第一次对话客户说预算 5000，第二次对话 Recall 能主动返回该记忆
type CustomerLongTermMemory struct {
	ID         uint64               `gorm:"primaryKey;autoIncrement" json:"id"`
	CustomerID string               `gorm:"type:varchar(64);not null;index:idx_cltm_customer,priority:1" json:"customer_id"`
	MemoryType LongTermMemoryType   `gorm:"type:varchar(50);not null;index:idx_cltm_customer,priority:2" json:"memory_type"`
	Content    string               `gorm:"type:text;not null" json:"content"`
	Importance int                  `gorm:"default:5;index:idx_cltm_importance,priority:1" json:"importance"`
	Source     LongTermMemorySource `gorm:"type:varchar(50);default:'conversation'" json:"source"`
	Embedding  string               `gorm:"type:vector(1024)" json:"embedding,omitempty"`
	Metadata   JSONMap              `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	CreatedAt  time.Time            `gorm:"autoCreateTime;index:idx_cltm_importance,priority:2" json:"created_at"`
	ExpiresAt  *time.Time           `gorm:"index" json:"expires_at,omitempty"`
	// M-6 双时间轴（Zep 模式）：ValidFrom=事件生效时间 t_valid，InvalidAt=软失效时间 t_invalid
	// 可空，读取层 NULL 兜底为 created_at；软失效不物理删
	ValidFrom *time.Time `gorm:"index" json:"valid_from,omitempty"`
	InvalidAt *time.Time `gorm:"index" json:"invalid_at,omitempty"`
}

// TableName 表名
func (CustomerLongTermMemory) TableName() string { return "customer_long_term_memory" }
