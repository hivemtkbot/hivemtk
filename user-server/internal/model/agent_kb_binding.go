package model

import "time"

// agent_kb_binding.go 智能体 ↔ 知识库 多对多绑定 (P0-B 智能体 1:N 知识库)
//
// 五层架构归属: L5 数据层 (横向)
// 设计依据: 2026-07-31 P0 知识库隔离架构
//   - 一个智能体可挂载多个知识库 (主 + 参考)
//   - 一个知识库可被多个智能体共享 (通过白名单)
//   - 角色 (Role) 区分主参考(primary=主, reference=辅)
//   - 同一 (agent_id, kb_id) 唯一, 避免重复绑定
//
// 字段说明:
//   - AgentID: 智能体主键 (外键 ai_agents.id, NOT NULL)
//   - KBID:    知识库主键 (外键 knowledge_bases.id, NOT NULL)
//   - KBType:  冗余存知识库类型 (faq / rag / sop), 加速按类型查询
//   - Role:    primary=主知识库, reference=参考知识库
//   - Priority: 排序用 (数字越大越优先)
//   - Enabled: 用 *bool 避免 GORM v2 零值问题
//
// 表: agent_kb_bindings
// 约束:
//   - UNIQUE(agent_id, kb_id)   防重复绑定
//   - INDEX(agent_id)            按智能体查其绑定
//   - INDEX(kb_id)               按知识库查其被哪些智能体引用
//   - INDEX(kb_type)             按类型过滤
//   - INDEX(enabled)             过滤禁用绑定
type AgentKBBinding struct {
	ID       uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	AgentID  uint   `gorm:"not null;uniqueIndex:idx_agent_kb_unique,priority:1;index" json:"agent_id"`
	KBID     uint   `gorm:"not null;uniqueIndex:idx_agent_kb_unique,priority:2;index" json:"kb_id"`
	KBType   string `gorm:"type:varchar(16);not null;index" json:"kb_type"` // faq / rag / sop
	Role     string `gorm:"type:varchar(16);not null;default:primary" json:"role"`
	Priority int    `gorm:"type:integer;default:0" json:"priority"`
	// Enabled 用 *bool 避免 GORM v2 零值 false 被 column default 覆盖
	Enabled   *bool     `gorm:"type:boolean;default:true;not null;index" json:"enabled"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName GORM 表名
func (AgentKBBinding) TableName() string { return "agent_kb_bindings" }

// AgentKBBindingRole 绑定角色枚举
const (
	AgentKBBindingRolePrimary   = "primary"   // 主知识库
	AgentKBBindingRoleReference = "reference" // 参考知识库
)
