package model

import (
	"time"
)

// LongTermMemoryType L2 长期记忆分类
// 对应 PRD §5.2 P1-1 G5：preference/habit/feedback/event/fact
type LongTermMemoryType string

const (
	LongTermMemoryPreference LongTermMemoryType = "preference" // 客户偏好（预算/品牌/渠道）
	LongTermMemoryHabit      LongTermMemoryType = "habit"      // 客户习惯（沟通时间/频率）
	LongTermMemoryFeedback   LongTermMemoryType = "feedback"   // 客户反馈（喜欢/不满意）
	LongTermMemoryEvent      LongTermMemoryType = "event"      // 关键事件（生日/购买/投诉）
	LongTermMemoryFact       LongTermMemoryType = "fact"       // 关键事实（身份/属性）
)

// LongTermMemorySource 长期记忆来源
type LongTermMemorySource string

const (
	LongTermMemorySourceConversation LongTermMemorySource = "conversation" // 对话抽取
	LongTermMemorySourceTool         LongTermMemorySource = "tool"         // 工具调用
	LongTermMemorySourceManual       LongTermMemorySource = "manual"       // 人工录入
)

// CustomerLongTermMemory P1-1 G5 L2 长期记忆（pgvector 增强版）
// 对应 PRD §5.2 P1-1 G5：customer_long_term_memory 表
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
	Importance int                  `gorm:"default:5;index:idx_cltm_importance,priority:1" json:"importance"` // 1-10
	Source     LongTermMemorySource `gorm:"type:varchar(50);default:'conversation'" json:"source"`
	// Embedding 使用 string + gorm:"type:vector(1024)"：
	//   - PostgreSQL：pgvector 自动识别 vector(1024) 类型
	//   - GORM []byte 会被映射为 bytea，导致 pgvector 解析失败（SQLSTATE 22P02）。
	//     改用 string 走 text 通道，pgvector 接受 '[v1,v2,...]' 格式。
	//   - 写入时使用 embeddingToString(vec) 序列化；读取时用 []byte(it.Embedding) 还原。
	//   - 维度 1024 与本地 TEI bge-m3 真实输出一致（2026-07-18 私域基线）。
	//     严禁改回 768（BAAI/bge-base-zh-v1.5），否则 pgvector 写入会报维度不匹配。
	Embedding string     `gorm:"type:vector(1024)" json:"embedding,omitempty"`
	Metadata  JSONMap    `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	CreatedAt time.Time  `gorm:"autoCreateTime;index:idx_cltm_importance,priority:2" json:"created_at"`
	ExpiresAt *time.Time `gorm:"index" json:"expires_at,omitempty"`
}

// TableName 表名
func (CustomerLongTermMemory) TableName() string { return "customer_long_term_memory" }
