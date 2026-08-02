package model

import (
	"time"

	"github.com/lib/pq"

	knowledgemodel "marketing/internal/aiagent/knowledge/model"
)

// LLMProviderConfig 引用 knowledge 包类型(避免循环依赖)
type LLMProviderConfig = knowledgemodel.LLMProviderConfig

// 引用 knowledge 包的状态/来源类型
type (
	EmbedStatus = knowledgemodel.EmbedStatus
	SourceType  = knowledgemodel.SourceType
)

// 引用 knowledge 包的常量
const (
	SourceTypeOpenAPI = knowledgemodel.SourceTypeOpenAPI
	SourceTypeUpload  = knowledgemodel.SourceTypeUpload
	SourceTypeText    = knowledgemodel.SourceTypeText
	SourceTypeURL     = knowledgemodel.SourceTypeURL
	SourceTypeBatch   = knowledgemodel.SourceTypeBatch
)

// 引用 knowledge 包的 embed status 常量
const (
	EmbedStatusPending    = knowledgemodel.EmbedStatusPending
	EmbedStatusProcessing = knowledgemodel.EmbedStatusProcessing
	EmbedStatusIndexed    = knowledgemodel.EmbedStatusIndexed
	EmbedStatusFailed     = knowledgemodel.EmbedStatusFailed
)

// ============================================================================
// 多 AI 智能体架构 - 数据模型
// ----------------------------------------------------------------------------
// 设计文档：docs/sales-champion/MULTI_AI_AGENT_DESIGN.md
//
// 三个核心实体：
//  1. AIAgent               - AI 智能体主表（人设/知识库/LLM/SOP/话术）
//  2. ChannelAgentBinding   - 渠道账号 ↔ 智能体绑定
//  3. CustomerServiceAgent  - 客服座席 ↔ 智能体挂载
//
// 私域独立部署：无 merchant_id 字段
// 五层架构：仅定义数据结构，业务逻辑在 Service 层
// ============================================================================

// AgentType 智能体类型（仅作为智能体内部子类型，平台层统一称「智能体」）
type AgentType string

const (
	AgentTypeSales           AgentType = "sales"            // 销售型智能体
	AgentTypeCustomerService AgentType = "customer_service" // 客服型智能体
	AgentTypeHybrid          AgentType = "hybrid"           // 混合智能体（销售+客服）
)

// AgentMode 智能体工作模式（平台层两大运行范式）
type AgentMode string

const (
	// AgentModePassive 被动模式：消息/事件进入系统 → 系统调用智能体完成对话 → 返回用户（智能体/智能体应答主路径）
	AgentModePassive AgentMode = "passive"
	// AgentModeActive 主动模式：智能体调用工具链主动触达用户（私信/短信/邮件/卡片），更多用于营销唤起与跟进
	AgentModeActive AgentMode = "active"
)

// AIAgent AI 智能体主表
//
// 一个 AIAgent = 一套完整的 智能体配置（人设 + 知识库 + LLM + SOP + 话术）
// 可被多渠道账号绑定，也可被多客服座席挂载
type AIAgent struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	AgentCode   string `gorm:"type:varchar(64);uniqueIndex;not null" json:"agent_code"`
	Name        string `gorm:"type:varchar(128);not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	Avatar      string `gorm:"type:varchar(500)" json:"avatar"`
	AgentType   string `gorm:"type:varchar(32);not null;default:sales;index" json:"agent_type"`
	AgentMode   string `gorm:"type:varchar(32);not null;default:passive;index" json:"agent_mode"` // passive 被动 / active 主动

	// 预设背景（核心）
	Persona      string `gorm:"type:text;not null" json:"persona"`
	SystemPrompt string `gorm:"type:text" json:"system_prompt"`
	Greeting     string `gorm:"type:text" json:"greeting"`

	// 知识库挂载（多对一：一个智能体可挂载多个 RAG 产品）
	RagProductIDs pq.StringArray `gorm:"type:text[];column:rag_product_ids" json:"rag_product_ids"`

	// FAQ 知识库挂载（: 每个智能体可绑专属 FAQ）
	// 空数组 = 全局共享（向后兼容）；非空 = 仅匹配绑定的 FAQ ID
	FAQEntryIDs pq.StringArray `gorm:"type:text[];column:faq_entry_ids" json:"faq_entry_ids"`

	// SOP 模板挂载（: 每个智能体可绑专属 SOP 模板）
	// 空数组 = 全局共享；非空 = 仅匹配绑定的 SOP template ID
	SOPTemplateIDs pq.StringArray `gorm:"type:text[];column:sop_template_ids" json:"sop_template_ids"`

	// SOP 流程图挂载（已有：智能体可挂载多个 sop_agents 流程图）
	SOPIDs pq.StringArray `gorm:"type:text[];column:sop_ids" json:"sop_ids"`

	// 话术库挂载
	ScriptLibraryIDs pq.StringArray `gorm:"type:text[];column:script_library_ids" json:"script_library_ids"`

	// 决策策略挂载 — 新增(§2.3)
	DecisionStrategyIDs pq.StringArray `gorm:"type:text[];column:decision_strategy_ids" json:"decision_strategy_ids"`

	// A/B 实验挂载 — 新增(§2.3)
	ABExperimentIDs pq.StringArray `gorm:"type:text[];column:ab_experiment_ids" json:"ab_experiment_ids"`

	// 资产包绑定 — 智能体可绑定一个资产包（AssetBundle.AssetID）
	// 执行时由 AssetBundleService.ResolveSystemPrompt 织入人设/话术，覆盖原 Persona
	AssetBundleID string `gorm:"type:varchar(128);column:asset_bundle_id;default:''" json:"asset_bundle_id"`

	// LLM 配置
	LLMModel          string            `gorm:"type:varchar(100);default:gpt-4o-mini" json:"llm_model"`
	LLMProviderConfig LLMProviderConfig `gorm:"embedded;embeddedPrefix:llm_" json:"llm_provider_config"`
	Temperature       float64           `gorm:"default:0.7" json:"temperature"`
	MaxTokens         int               `gorm:"default:800" json:"max_tokens"`
	TopP              float64           `gorm:"default:0.9" json:"top_p"`
	FrequencyPenalty  float64           `gorm:"default:0.5" json:"frequency_penalty"`
	PresencePenalty   float64           `gorm:"default:0.5" json:"presence_penalty"`

	// 多语言配置（v1.2 出海多语言方案）
	// InternalLanguage 商户内部语言（知识库语言+内部工具prompt语言），默认 zh
	// TargetLanguage   对外目标语言（空则退化=InternalLanguage）
	InternalLanguage string `gorm:"type:varchar(8);default:'zh'" json:"internal_language"`
	TargetLanguage   string `gorm:"type:varchar(8);default:''" json:"target_language"`

	// 销售引擎开关
	EnableRAG            bool `gorm:"default:true" json:"enable_rag"`
	EnableScriptMatch    bool `gorm:"default:true" json:"enable_script_match"`
	EnableHumanizePolish bool `gorm:"default:true" json:"enable_humanize_polish"`
	EnableContentAudit   bool `gorm:"default:true" json:"enable_content_audit"`
	EnablePlaybook       bool `gorm:"default:true" json:"enable_playbook"`
	RAGTopK              int  `gorm:"default:3" json:"rag_top_k"`

	// 转人工策略
	ConfidenceThreshold float64 `gorm:"default:0.7" json:"confidence_threshold"`
	MaxAIConsecutive    int     `gorm:"default:0" json:"max_ai_consecutive"` // 0=不限制，完全由置信度控制

	// 状态
	Status  int `gorm:"default:1;index" json:"status"` // 1=启用 0=禁用
	Version int `gorm:"default:1" json:"version"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (AIAgent) TableName() string {
	return "ai_agents"
}

// ChannelType 渠道类型枚举
type ChannelType string

const (
	ChannelTypeTelegram    ChannelType = "telegram"
	ChannelTypeWeCom       ChannelType = "wecom"
	ChannelTypeFeishu      ChannelType = "feishu"
	ChannelTypeWhatsApp    ChannelType = "whatsapp"
	ChannelTypeDingTalk    ChannelType = "dingtalk"
	ChannelTypeDouyin      ChannelType = "douyin"
	ChannelTypeXiaohongshu ChannelType = "xiaohongshu"
	ChannelTypeKuaishou    ChannelType = "kuaishou"
	ChannelTypeXianyu      ChannelType = "xianyu"
	ChannelTypeTikTok      ChannelType = "tiktok"
	// ChannelTypeWeb 网页客服（web 站点 / H5 嵌入式聊天窗 / 移动端 webview）
	ChannelTypeWeb ChannelType = "web"
)

// ChannelAgentBinding 渠道账号 ↔ 智能体绑定
//
// 强 1对1 改造 (Task 21):
//   - 同一 (channel_type, account_id) 只能有 1 条 is_primary=true 记录
//   - 数据库层通过部分 UNIQUE INDEX 保证: uq_channel_account_primary
//     (channel_type, account_id) WHERE is_primary = true
//   - 历史 is_primary=false 的辅助记录不再被业务使用, 保留为审计/回滚用途
//   - 业务层在 Replace / Bind 时, 必须先 ClearPrimaryByChannelAccount 再 Create
//
// 表: channel_agent_bindings
// 索引:
//   - INDEX(channel_type, account_id)       按渠道账号查询
//   - INDEX(agent_id)                       反查智能体被哪些渠道使用
//   - uq_channel_account_primary (迁移 035) 强 1对1 部分唯一索引
type ChannelAgentBinding struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	ChannelType string `gorm:"type:varchar(32);not null;index" json:"channel_type"`
	AccountID   string `gorm:"type:varchar(64);not null;index" json:"account_id"`
	AgentID     uint   `gorm:"not null;index" json:"agent_id"`
	IsPrimary   bool   `gorm:"default:true" json:"is_primary"`
	Enabled     bool   `gorm:"default:true" json:"enabled"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ChannelAgentBinding) TableName() string {
	return "channel_agent_bindings"
}

// CustomerServiceAgent 客服座席 ↔ 智能体挂载
//
// 同一座席可挂载多个智能体，但只有一个 is_primary=true
// SmartCSOrchestrator 在处理消息时按 agent_status_id 查询主挂载智能体
type CustomerServiceAgent struct {
	ID            uint `gorm:"primaryKey;autoIncrement" json:"id"`
	AgentStatusID uint `gorm:"not null;index;uniqueIndex:idx_cs_agent_unique" json:"agent_status_id"`
	AIAgentID     uint `gorm:"not null;index;uniqueIndex:idx_cs_agent_unique" json:"ai_agent_id"`
	IsPrimary     bool `gorm:"default:true" json:"is_primary"`
	Enabled       bool `gorm:"default:true" json:"enabled"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (CustomerServiceAgent) TableName() string {
	return "customer_service_agents"
}
