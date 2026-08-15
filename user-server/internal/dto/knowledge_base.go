package dto


// KnowledgeBaseType 知识库类型 (与 model.KnowledgeBaseType 保持一致)
const (
	KnowledgeBaseTypeFAQ = "faq"
	KnowledgeBaseTypeRAG = "rag"
	KnowledgeBaseTypeSOP = "sop"
)

// KnowledgeBaseOwnerType 知识库所有者类型 (与 model.KnowledgeBaseOwnerType 保持一致)
const (
	KnowledgeBaseOwnerPrivate = "private"
	KnowledgeBaseOwnerShared  = "shared"
)

// IsValidKBTypeDTO 校验知识库类型字段 (与 service.IsValidKBType 行为一致)
func IsValidKBTypeDTO(t string) bool {
	switch t {
	case KnowledgeBaseTypeFAQ, KnowledgeBaseTypeRAG, KnowledgeBaseTypeSOP:
		return true
	}
	return false
}

// KnowledgeBase 知识库主表 DTO
//
// 与 model.KnowledgeBase 的区别:
//   - CreatedAt/UpdatedAt 用 string (RFC3339), 便于前端直接展示
//   - 不暴露内部字段 (如 MemberCount/DocCount 等冗余统计可后续按需补充)
type KnowledgeBase struct {
	ID           uint   `json:"id"`
	KBCode       string `json:"kb_code"`
	Type         string `json:"type"` 
	Name         string `json:"name"`
	Description  string `json:"description"`
	OwnerType    string `json:"owner_type"` 
	OwnerAgentID *uint  `json:"owner_agent_id,omitempty"`
	MemberCount  int    `json:"member_count"`
	DocCount     int    `json:"doc_count"`
	Enabled      *bool  `json:"enabled,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// AgentKBBinding 智能体 × 知识库 绑定 DTO
type AgentKBBinding struct {
	ID        uint   `json:"id"`
	AgentID   uint   `json:"agent_id"`
	KBID      uint   `json:"knowledge_base_id"`
	KBType    string `json:"kb_type"` 
	Role      string `json:"role"`    
	Priority  int    `json:"priority"`
	Enabled   *bool  `json:"enabled,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// ChannelBinding 渠道账号 × 智能体 绑定 DTO
//
// 强 1对1 (Task 21):
//   - 同一 (channel_type, account_id) 只能有 1 条 is_primary=true 记录
//   - 数据库层由 uq_channel_account_primary 部分唯一索引保证
//   - 业务层通过 ChannelAgentBindingService.ReplaceBinding 实现原子替换
type ChannelBinding struct {
	ID          uint   `json:"id"`
	ChannelType string `json:"channel_type"`
	AccountID   string `json:"account_id"`
	AgentID     uint   `json:"agent_id"`
	IsPrimary   bool   `json:"is_primary"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// AgentBindingRole 智能体 × 知识库 角色枚举
const (
	AgentKBBindingRolePrimary   = "primary"
	AgentKBBindingRoleReference = "reference"
)

