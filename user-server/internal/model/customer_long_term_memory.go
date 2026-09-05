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

	ValidFrom *time.Time `gorm:"index" json:"valid_from,omitempty"`
	InvalidAt *time.Time `gorm:"index" json:"invalid_at,omitempty"`
}

// TableName 表名
func (CustomerLongTermMemory) TableName() string { return "customer_long_term_memory" }
