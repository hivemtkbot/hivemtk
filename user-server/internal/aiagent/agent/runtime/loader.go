package agent_runtime

import (
	"context"
	"errors"
	"fmt"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// ============================================================================
// pgAgentContextLoader — 基于 PostgreSQL 的智能体上下文加载器
// ----------------------------------------------------------------------------
// 设计依据： §2.1 (运行时隔离)
// 用途：替换 nil 降级实现,从 ai_agents + channel_agent_bindings 加载真实配置
//
// 流程：
//  1. ChannelAgentBindingRepository.GetPrimaryByChannelAccount(channel, account)
//     → 查到主绑定 → AgentID
//  2. AIAgentRepository.GetByID(AgentID)
//     → 加载完整智能体配置
//  3. model.AIAgent → AgentContext 转换
//  4. 缓存(由 defaultAgentRuntime 实现)
// ============================================================================

// pgAgentContextLoader 智能体上下文加载器
type pgAgentContextLoader struct {
	agentRepo   *repository.AIAgentRepository
	bindingRepo *repository.ChannelAgentBindingRepository
}

// NewPGAgentContextLoader 创建 PG loader
func NewPGAgentContextLoader(agentRepo *repository.AIAgentRepository, bindingRepo *repository.ChannelAgentBindingRepository) AgentContextLoader {
	return &pgAgentContextLoader{
		agentRepo:   agentRepo,
		bindingRepo: bindingRepo,
	}
}

// LoadByChannelAccount 按渠道+账号加载智能体上下文
//
// 步骤：
//  1. 查询主绑定 → AgentID
//  2. 查询智能体详情
//  3. 转换为 AgentContext
//
// 错误处理：
//   - 绑定不存在 → ErrNoAgentBinding(由上层降级到默认智能体)
//   - 智能体已禁用 → ErrAgentDisabled
//   - 智能体不存在 → ErrAgentNotFound
func (l *pgAgentContextLoader) LoadByChannelAccount(ctx context.Context, channelType, accountID string) (*AgentContext, error) {
	if l.agentRepo == nil || l.bindingRepo == nil {
		return nil, errors.New("agent_runtime: loader not initialized")
	}

	// 1. 查主绑定
	binding, err := l.bindingRepo.GetPrimaryByChannelAccount(ctx, channelType, accountID)
	if err != nil {
		return nil, fmt.Errorf("%w: channel=%s account=%s: %v", ErrNoAgentBinding, channelType, accountID, err)
	}
	if binding == nil {
		return nil, fmt.Errorf("%w: channel=%s account=%s", ErrNoAgentBinding, channelType, accountID)
	}

	// 2. 查智能体
	agent, err := l.agentRepo.GetByID(ctx, binding.AgentID)
	if err != nil {
		return nil, fmt.Errorf("%w: agent_id=%d: %v", ErrAgentNotFound, binding.AgentID, err)
	}
	if agent == nil {
		return nil, fmt.Errorf("%w: agent_id=%d", ErrAgentNotFound, binding.AgentID)
	}

	// 3. 状态校验
	if agent.Status == 0 {
		return nil, fmt.Errorf("%w: agent_id=%d code=%s", ErrAgentDisabled, agent.ID, agent.AgentCode)
	}

	// 4. 转换
	return convertAIAgentToContext(agent, channelType, accountID), nil
}

// Invalidate 失效指定智能体缓存
//
// AgentContextLoader 的 Invalidate 接口实现
// 实际缓存管理在 defaultAgentRuntime 中,这里只做 repository 层的清理
func (l *pgAgentContextLoader) Invalidate(ctx context.Context, agentID uint) error {
	// PG loader 本身不持有缓存,缓存由 runtime 层管理
	// 此方法预留供 future 重构(若把缓存下沉到 loader)
	return nil
}

// ============================================================================
// 转换函数
// ============================================================================

// convertAIAgentToContext model.AIAgent → AgentContext
//
// 加 FAQEntryIDs / SOPTemplateIDs 字段映射
// 这些字段在 layer.go 中被废弃 (强 1:1), 但 loader 仍需填充以保证向后兼容
//   - Service 层若要走"agent 绑定 ID 集合"路径, 仍可使用这两个字段
func convertAIAgentToContext(agent *model.AIAgent, channel, accountID string) *AgentContext {
	return &AgentContext{
		// 基础信息
		AgentID:   agent.ID,
		AgentCode: agent.AgentCode,
		Name:      agent.Name,
		AgentType: agent.AgentType,

		// 人设
		Persona:      agent.Persona,
		SystemPrompt: agent.SystemPrompt,
		Greeting:     agent.Greeting,

		// 知识库挂载
		RagProductIDs: stringSlice(agent.RagProductIDs),

		// FAQ / SOP 模板 ID 集合映射 (字段, 仍保留用于向后兼容)
		FAQEntryIDs:    stringSlice(agent.FAQEntryIDs),
		SOPTemplateIDs: stringSlice(agent.SOPTemplateIDs),

		// SOP / 话术库 / 决策策略 / A/B 实验挂载
		SOPIDs:              stringSlice(agent.SOPIDs),
		ScriptLibraryIDs:    stringSlice(agent.ScriptLibraryIDs),
		DecisionStrategyIDs: stringSlice(agent.DecisionStrategyIDs),
		ABExperimentIDs:     stringSlice(agent.ABExperimentIDs),

		// LLM 配置
		LLMModel:         agent.LLMModel,
		Temperature:      agent.Temperature,
		MaxTokens:        agent.MaxTokens,
		TopP:             agent.TopP,
		FrequencyPenalty: agent.FrequencyPenalty,
		PresencePenalty:  agent.PresencePenalty,

		// 引擎开关
		EnableRAG:            agent.EnableRAG,
		EnableScriptMatch:    agent.EnableScriptMatch,
		EnableHumanizePolish: agent.EnableHumanizePolish,
		EnableContentAudit:   agent.EnableContentAudit,
		EnablePlaybook:       agent.EnablePlaybook,
		RAGTopK:              agent.RAGTopK,

		// 转人工策略
		ConfidenceThreshold: agent.ConfidenceThreshold,
		MaxAIConsecutive:    agent.MaxAIConsecutive,

		// 元信息
		Version:   agent.Version,
		Channel:   channel,
		AccountID: accountID,
	}
}

// stringSlice pq.StringArray → []string
//
// pq.StringArray 在 nil 时会 panic(?),这里做 nil 检查
func stringSlice(arr []string) []string {
	if arr == nil {
		return []string{}
	}
	return arr
}

// ============================================================================
// 错误定义
// ============================================================================

var (
	// ErrNoAgentBinding 未找到智能体绑定
	ErrNoAgentBinding = errors.New("agent_runtime: no agent binding for channel account")

	// ErrAgentNotFound 智能体不存在
	ErrAgentNotFound = errors.New("agent_runtime: agent not found")

	// ErrAgentDisabled 智能体已禁用
	ErrAgentDisabled = errors.New("agent_runtime: agent is disabled")
)
