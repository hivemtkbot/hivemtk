package dto

import (
	"time"

	"marketing/internal/model"
)

// sales.go 销冠域 - 销售引擎 DTO
//
// 本文件包含：
// 1. SalesStepLog：销售链路步骤日志（已迁入）
//  2. RAGChunk：RAG 召回片段（深度 DTO 迁移-3 已迁入）
//  3. ScriptTemplate：话术模板（深度 DTO 迁移-3 已迁入）
//  4. Industry/ObjectionType/PlaybookEntry：销冠话术库核心类型（深度 DTO 迁移-3 已迁入）
//  5. JourneyStage：客户旅程阶段（深度 DTO 迁移-3 已迁入）
//  6. SalesEngineConfig：销售引擎配置（深度 DTO 迁移-4 已迁入）
//  7. AgentContext：智能体执行上下文（深度 DTO 迁移-4 已迁入）
//
// 设计说明：
//   - dto 引用 model 是允许的（model 不引用 dto，无循环依赖）
//   - service 层使用 type alias 保持向后兼容（如 service.Industry = dto.Industry）
//   - 这些类型多为纯数据结构 + JSON 标签，迁移到 dto 后可被 controller / repository / 上层直接复用

// SalesStepLog 销售链路步骤日志
type SalesStepLog struct {
	Step      string `json:"step"`   // 步骤名
	Status    string `json:"status"` // ok / fail / skip
	LatencyMs int    `json:"latency_ms"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
	Extra     any    `json:"extra,omitempty"`
}

// ============================================================================
// RAG 召回片段 / 话术模板（从 service 包迁入）
// ============================================================================

// RAGChunk RAG 召回片段
type RAGChunk struct {
	Content string  `json:"content"`
	Source  string  `json:"source"`
	Score   float64 `json:"score"`
	DocID   string  `json:"doc_id"`
	ChunkID string  `json:"chunk_id"`
}

// ScriptTemplate 话术模板
type ScriptTemplate struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Scenario  string   `json:"scenario"`
	Tags      []string `json:"tags"`
	MatchRate float64  `json:"match_rate"`
}

// ============================================================================
// 销冠话术库核心类型（从 service/sales_playbook.go 迁入）
// ============================================================================

// Industry 行业
type Industry string

const (
	IndustryMedicalBeauty Industry = "medical_beauty" // 医美
	IndustryEducation     Industry = "education"      // 教培
	IndustryEcommerce     Industry = "ecommerce"      // 电商
	IndustryRealEstate    Industry = "real_estate"    // 房产
	IndustryAuto          Industry = "auto"           // 汽车
	IndustryFinance       Industry = "finance"        // 金融
	IndustryB2B           Industry = "b2b"            // B2B
)

// ObjectionType 异议类型（销冠话术库专用，与 objection_handler_service.go 的 ObjectionCategory 区分）
type ObjectionType string

const (
	PlayObjectionPrice       ObjectionType = "price"       // 价格异议
	PlayObjectionTime        ObjectionType = "time"        // 时机异议（再考虑下）
	PlayObjectionTrust       ObjectionType = "trust"       // 信任异议（不靠谱）
	PlayObjectionCompetition ObjectionType = "competition" // 竞品异议
	PlayObjectionNeed        ObjectionType = "need"        // 需求异议（不需要）
	PlayObjectionAuthority   ObjectionType = "authority"   // 决策权异议（要问老公）
	PlayObjectionStall       ObjectionType = "stall"       // 拖延异议
)

// PlaybookEntry 话术条目
type PlaybookEntry struct {
	ID           string        `json:"id"`
	Industry     Industry      `json:"industry"`      // 行业
	ProductID    string        `json:"product_id"`    // 产品ID（空=通用）
	Stage        JourneyStage  `json:"stage"`         // 客户旅程阶段
	Objection    ObjectionType `json:"objection"`     // 异议类型（空=开场/破冰）
	Title        string        `json:"title"`         // 话术标题
	Content      string        `json:"content"`       // 话术内容
	Tips         string        `json:"tips"`          // 使用技巧
	Tags         []string      `json:"tags"`          // 标签：开场/破冰/逼单/挽留
	UseCount     int           `json:"use_count"`     // 使用次数
	SuccessCount int           `json:"success_count"` // 成功次数（客户转化）
	CreatedBy    string        `json:"created_by"`    // 创建者（销冠姓名）
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// ============================================================================
// 客户旅程阶段（从 service/customer_journey.go 迁入）
// ============================================================================

// JourneyStage 客户旅程阶段
type JourneyStage string

const (
	StageStranger   JourneyStage = "stranger"   // 陌生（首次接触）
	StageLead       JourneyStage = "lead"       // 留资（留下联系方式）
	StageContact    JourneyStage = "contact"    // 初步接触（已加微/已回复）
	StageInterested JourneyStage = "interested" // 意向（咨询过产品/价格）
	StageQuoted     JourneyStage = "quoted"     // 报价（已发送报价单/价格）
	StageWon        JourneyStage = "won"        // 成交（已下单/已付款）
	StageAfterSale  JourneyStage = "after_sale" // 售后（服务履约中/已交付）
	StageRepurchase JourneyStage = "repurchase" // 复购（再次购买）
	StageSleeping   JourneyStage = "sleeping"   // 沉睡（30-90 天无互动）
	StageLost       JourneyStage = "lost"       // 流失（明确拒绝/拉黑）
)

// AllStages 所有阶段
var AllStages = []JourneyStage{
	StageStranger, StageLead, StageContact, StageInterested,
	StageQuoted, StageWon, StageAfterSale, StageRepurchase, StageSleeping, StageLost,
}

// JourneyTransitionRequest 客户旅程阶段迁移请求
type JourneyTransitionRequest struct {
	CustomerID string       `json:"customer_id" binding:"required"`
	ToStage    JourneyStage `json:"to_stage" binding:"required"`
	Source     string       `json:"source"` // 触发源：manual / sop / ai / system
	Reason     string       `json:"reason"` // 原因
}

// ============================================================================
// 销售引擎配置 + 智能体上下文（深度 DTO 迁移-4）
// ============================================================================

// SalesEngineConfig 销售引擎配置
type SalesEngineConfig struct {
	EnableRAG            bool    `json:"enable_rag"`             // 是否启用 RAG
	EnableScriptMatch    bool    `json:"enable_script_match"`    // 是否启用话术匹配
	EnableHumanizePolish bool    `json:"enable_humanize_polish"` // 是否启用拟人润色
	EnableContentAudit   bool    `json:"enable_content_audit"`   // 是否启用发送前审核
	RAGTopK              int     `json:"rag_top_k"`              // RAG 召回 TopK
	Temperature          float64 `json:"temperature"`            // LLM 温度
	MaxTokens            int     `json:"max_tokens"`             // LLM 最大 token
	Persona              string  `json:"persona"`                // 销售人设

	// 置信度驱动：客户等级 vip/normal/low（影响动态阈值）
	CustomerLevel string `json:"customer_level,omitempty"`

	// Tools 智能体工具白名单（可选）。
	// 非空时：Agent Loop 仅注入这些工具给 LLM，且执行期权限检查器按此名单放行。
	// 为空时：使用 limitToolsForAgent 的默认优先级集（见 service/sales_engine.go）。
	Tools []string `json:"tools,omitempty"`
}

// DefaultSalesEngineConfig 默认配置
func DefaultSalesEngineConfig() *SalesEngineConfig {
	return &SalesEngineConfig{
		EnableRAG:            true,
		EnableScriptMatch:    true,
		EnableHumanizePolish: true,
		EnableContentAudit:   false,
		RAGTopK:              3,
		Temperature:          0.7,
		MaxTokens:            800,
		Persona:              "你是一位资深销售专家，擅长用温和、专业的语气帮助客户解决问题。回复简洁、亲切、不超过 80 字。",
	}
}

// AgentContext 智能体执行上下文
// 由 AIAgentService.LoadContext 加载，传给 SalesEngine.HandleWithAgent
type AgentContext struct {
	AgentID              uint                    `json:"agent_id"`
	AgentCode            string                  `json:"agent_code"`
	Name                 string                  `json:"name"`
	AgentType            string                  `json:"agent_type"`
	AgentMode            string                  `json:"agent_mode"` // passive 被动 / active 主动
	Persona              string                  `json:"persona"`
	SystemPrompt         string                  `json:"system_prompt"`
	Greeting             string                  `json:"greeting"`
	RagProductIDs        []string                `json:"rag_product_ids"`    // 知识库挂载
	// 知识库挂载 (FAQ / SOP 模板)
	// 空切片 = 全局共享（向后兼容）；非空 = 仅匹配绑定的 ID 集合
	FAQEntryIDs    []string `json:"faq_entry_ids"`
	SOPTemplateIDs []string `json:"sop_template_ids"`
	SOPIDs               []string                `json:"sop_ids"`            // SOP 挂载
	ScriptLibraryIDs     []string                `json:"script_library_ids"` // 话术库挂载
	LLMModel             string                  `json:"llm_model"`
	LLMProviderConfig    model.LLMProviderConfig `json:"llm_provider_config"`
	Temperature          float64                 `json:"temperature"`
	MaxTokens            int                     `json:"max_tokens"`
	TopP                 float64                 `json:"top_p"`
	FrequencyPenalty     float64                 `json:"frequency_penalty"`
	PresencePenalty      float64                 `json:"presence_penalty"`
	EnableRAG            bool                    `json:"enable_rag"`
	EnableScriptMatch    bool                    `json:"enable_script_match"`
	EnableHumanizePolish bool                    `json:"enable_humanize_polish"`
	EnableContentAudit   bool                    `json:"enable_content_audit"`
	EnablePlaybook       bool                    `json:"enable_playbook"`
	RAGTopK              int                     `json:"rag_top_k"`
	ConfidenceThreshold  float64                 `json:"confidence_threshold"`
	MaxAIConsecutive     int                     `json:"max_ai_consecutive"`

	// 决策策略挂载 — 新增(§2.3)
	DecisionStrategyIDs []string `json:"decision_strategy_ids"`

	// A/B 实验挂载 — 新增(§2.3)
	ABExperimentIDs []string `json:"ab_experiment_ids"`

	// 资产包绑定 — 智能体可绑定一个资产包（AssetBundle），
	// 在 SalesEngine 执行前由 AssetBundleResolver 解析为 system prompt，
	// 覆盖原 Persona（实现「渠道→智能体→资产包」三层接线）
	AssetBundleID string `json:"asset_bundle_id,omitempty"`

	// Tools 智能体工具白名单（可选，覆盖默认优先级集）。
	// 由运营在智能体配置中指定该智能体可使用哪些工具（如电商客服智能体配置
	// ["order.lookup","logistics.track","aftersale.create","aftersale.query","reach.card.send"]）。
	// 空 = 走默认优先级集；非空 = 仅注入并放行名单内工具。
	Tools []string `json:"tools,omitempty"`

	// 版本号(用于缓存失效)
	Version int `json:"version"`
}

// AgentContextToSalesEngineConfig 将 AgentContext 转换为 SalesEngineConfig
// 包级函数,架构文档 §三 L4 要求(type alias 不允许定义方法,故用包级函数)
func AgentContextToSalesEngineConfig(a *AgentContext) *SalesEngineConfig {
	if a == nil {
		return DefaultSalesEngineConfig()
	}
	return &SalesEngineConfig{
		EnableRAG:            a.EnableRAG,
		EnableScriptMatch:    a.EnableScriptMatch,
		EnableHumanizePolish: a.EnableHumanizePolish,
		EnableContentAudit:   a.EnableContentAudit,
		RAGTopK:              a.RAGTopK,
		Temperature:          a.Temperature,
		MaxTokens:            a.MaxTokens,
		Persona:              a.Persona,
		Tools:                a.Tools,
	}
}

// ============================================================================
// 销售请求 / 销售回复（深度 DTO 迁移-5）
// ============================================================================

// SalesRequest 销售请求
type SalesRequest struct {
	SessionID   string             `json:"session_id"`
	CustomerID  string             `json:"customer_id"`
	OneID       string             `json:"one_id"`
	UserMessage string             `json:"user_message"`
	Platform    string             `json:"platform"`     // 渠道：wechat / douyin / xiaohongshu 等
	AutoExecute bool               `json:"auto_execute"` // 是否自动执行（true=AI 接管，false=辅助建议）
	Config      *SalesEngineConfig `json:"config,omitempty"`

	// AgentContext 多智能体上下文（新增字段，向下兼容）
	// 由 HandleWithAgent 注入；recallRAG / matchSOP / generateCandidate 等内部方法可读取
	// nil 表示走默认流程
	AgentContext *AgentContext `json:"agent_context,omitempty"`
}

// SalesResponse 销售回复
type SalesResponse struct {
	Reply               string                `json:"reply"`            // 最终回复
	Intent              *RecognizeResult      `json:"intent"`           // 意图
	Memory              *model.DialogueMemory `json:"memory,omitempty"` // 当前记忆
	MatchedSOP          *model.SOPAgent       `json:"matched_sop,omitempty"`
	RAGChunks           []RAGChunk            `json:"rag_chunks,omitempty"`
	ScriptTemplate      *ScriptTemplate       `json:"script_template,omitempty"`
	PlaybookSuggestions []*PlaybookEntry      `json:"playbook_suggestions,omitempty"` // 销冠话术推荐
	LLMProvider         string                `json:"llm_provider,omitempty"`
	LLMModel            string                `json:"llm_model,omitempty"`
	CostTokens          int                   `json:"cost_tokens"`
	LatencyMs           int                   `json:"latency_ms"`
	Polished            bool                  `json:"polished"`
	Audited             bool                  `json:"audited"`
	AuditIssues         []string              `json:"audit_issues,omitempty"`
	TransferredToHuman  bool                  `json:"transferred_to_human"` // 是否转人工
	TransferReason      string                `json:"transfer_reason,omitempty"`
	// Cards 会话内结构化富卡片（商品卡/订单卡/优惠卡等），随回复一并下发到会话方
	Cards               []model.RichCard      `json:"cards,omitempty"`
	Steps               []SalesStepLog        `json:"steps"` // 9 步链路日志

	// 置信度决策（注入 ConfidenceAggregator 时填充）
	Confidence *ConfidenceDecision `json:"confidence,omitempty"`

	// 拟人度评估结果（注入 HumanizeEvalService 时填充）
	HumanizeScore   float64 `json:"humanize_score,omitempty"`
	HumanizePassed  bool    `json:"humanize_passed,omitempty"`
	HumanizeAttempt int     `json:"humanize_attempt,omitempty"`
}

// SalesRequestHasAgentContext 判断 SalesRequest 是否携带智能体上下文
// 供 recallRAG / matchSOP 等内部方法判断是否按智能体过滤(包级函数,架构文档 §三 L4 要求)
func SalesRequestHasAgentContext(r *SalesRequest) bool {
	return r != nil && r.AgentContext != nil
}

// SalesRequestRagProductIDs 获取当前请求应检索的知识库产品 ID 列表
// 优先使用 agentCtx.RagProductIDs，回退空（由 RAG 服务默认处理）
func SalesRequestRagProductIDs(r *SalesRequest) []string {
	if SalesRequestHasAgentContext(r) && len(r.AgentContext.RagProductIDs) > 0 {
		return r.AgentContext.RagProductIDs
	}
	return nil
}

// SalesRequestSOPIDs 获取当前请求应匹配的 SOP ID 列表
func SalesRequestSOPIDs(r *SalesRequest) []string {
	if SalesRequestHasAgentContext(r) && len(r.AgentContext.SOPIDs) > 0 {
		return r.AgentContext.SOPIDs
	}
	return nil
}

// SalesRequestAgentPersona 获取当前请求的智能体人设
// 供 LLM 生成步骤注入系统提示
func SalesRequestAgentPersona(r *SalesRequest) string {
	if SalesRequestHasAgentContext(r) {
		if r.AgentContext.SystemPrompt != "" {
			return r.AgentContext.SystemPrompt
		}
		if r.AgentContext.Persona != "" {
			return r.AgentContext.Persona
		}
	}
	if r.Config != nil && r.Config.Persona != "" {
		return r.Config.Persona
	}
	return ""
}

// SalesRequestAgentLLMModel 获取当前请求应使用的 LLM 模型
// 留待 LLM 调度层使用（当前 dispatcher 默认模型）
func SalesRequestAgentLLMModel(r *SalesRequest) string {
	if SalesRequestHasAgentContext(r) && r.AgentContext.LLMModel != "" {
		return r.AgentContext.LLMModel
	}
	return ""
}
