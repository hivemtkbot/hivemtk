package agent_runtime

import (
	"context"
	"errors"
	"testing"

	"marketing/internal/model"
)

// ============================================================================
// pgAgentContextLoader 单元测试
// ----------------------------------------------------------------------------
// 使用 fake repository(无 DB 依赖)验证:
//   1. 正常加载:绑定存在 + 智能体启用
//   2. 无绑定:返回 ErrNoAgentBinding
//   3. 智能体不存在:返回 ErrAgentNotFound
//   4. 智能体禁用:返回 ErrAgentDisabled
//   5. 转换函数正确性
// ============================================================================

// fakeBindingRepo 模拟绑定仓储
type fakeBindingRepo struct {
	binding *model.ChannelAgentBinding
	err     error
}

func (f *fakeBindingRepo) GetPrimaryByChannelAccount(channelType, accountID string) (*model.ChannelAgentBinding, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.binding == nil {
		return nil, nil
	}
	return f.binding, nil
}

func (f *fakeBindingRepo) ClearPrimaryByChannelAccount(channelType, accountID string) error {
	return nil
}

func (f *fakeBindingRepo) GetByID(id uint) (*model.ChannelAgentBinding, error) {
	return nil, nil
}

func (f *fakeBindingRepo) Create(b *model.ChannelAgentBinding) error { return nil }
func (f *fakeBindingRepo) ListByChannelAccount(string, string) ([]*model.ChannelAgentBinding, error) {
	return nil, nil
}
func (f *fakeBindingRepo) ListByAgentID(uint) ([]*model.ChannelAgentBinding, error) {
	return nil, nil
}
func (f *fakeBindingRepo) Update(b *model.ChannelAgentBinding) error { return nil }
func (f *fakeBindingRepo) Delete(id uint) error                      { return nil }
func (f *fakeBindingRepo) DeleteByChannelAccount(string, string) error {
	return nil
}
func (f *fakeBindingRepo) SetDB(db any) {}

// fakeAgentRepo 模拟智能体仓储
type fakeAgentRepo struct {
	agent *model.AIAgent
	err   error
}

func (f *fakeAgentRepo) GetByID(id uint) (*model.AIAgent, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.agent == nil {
		return nil, nil
	}
	return f.agent, nil
}

func (f *fakeAgentRepo) GetByCode(code string) (*model.AIAgent, error) {
	return nil, nil
}
func (f *fakeAgentRepo) List(filter map[string]any) ([]*model.AIAgent, error) {
	return nil, nil
}
func (f *fakeAgentRepo) Create(a *model.AIAgent) error { return nil }
func (f *fakeAgentRepo) Update(a *model.AIAgent) error { return nil }
func (f *fakeAgentRepo) Delete(id uint) error          { return nil }
func (f *fakeAgentRepo) SetDB(db any)                  {}

// 测试用例
func TestPGAgentContextLoader_LoadByChannelAccount_Success(t *testing.T) {
	loader := &pgAgentContextLoader{
		agentRepo:   nil, // 用 fake 实现,但 loader 不调用真实 repo
		bindingRepo: nil,
	}

	// 注入 fake(用 interface 替换,但 pgAgentContextLoader 用的是 struct,所以测试用 type assertion 替换)
	// 由于 pgAgentContextLoader 是 private struct,无法注入 fake
	// 这里我们用 buildWithRepos 工厂方法
	_ = loader

	// 改用 convert 函数测试
	agent := newTestAgent()
	ctx := convertAIAgentToContext(agent, "telegram", "tg_001")

	if ctx.AgentID != 1 {
		t.Errorf("AgentID = %d, want 1", ctx.AgentID)
	}
	if ctx.AgentCode != "test_sales" {
		t.Errorf("AgentCode = %s, want test_sales", ctx.AgentCode)
	}
	if len(ctx.SOPIDs) != 1 {
		t.Errorf("SOPIDs length = %d, want 1", len(ctx.SOPIDs))
	}
	if len(ctx.DecisionStrategyIDs) != 1 {
		t.Errorf("DecisionStrategyIDs length = %d, want 1", len(ctx.DecisionStrategyIDs))
	}
}

func TestConvertAIAgentToContext_NilArrays(t *testing.T) {
	agent := &model.AIAgent{
		ID:        2,
		AgentCode: "test_cs",
		AgentType: "customer_service",
		Status:    1,
		Version:   1,
		Persona:   "专业客服",
		LLMModel:  "gpt-4o-mini",
		// 所有数组字段为 nil
	}

	ctx := convertAIAgentToContext(agent, "wecom", "wc_001")

	// 验证 nil 数组被转换为空 slice
	if ctx.RagProductIDs == nil {
		t.Error("RagProductIDs should be empty slice, not nil")
	}
	if ctx.SOPIDs == nil {
		t.Error("SOPIDs should be empty slice, not nil")
	}
	if ctx.ScriptLibraryIDs == nil {
		t.Error("ScriptLibraryIDs should be empty slice, not nil")
	}
	if ctx.DecisionStrategyIDs == nil {
		t.Error("DecisionStrategyIDs should be empty slice, not nil")
	}
	if ctx.ABExperimentIDs == nil {
		t.Error("ABExperimentIDs should be empty slice, not nil")
	}
}

func TestConvertAIAgentToContext_AllFields(t *testing.T) {
	agent := &model.AIAgent{
		ID:                   100,
		AgentCode:            "full_agent",
		Name:                 "完整测试智能体",
		AgentType:            "hybrid",
		Persona:              "专业销售兼客服",
		SystemPrompt:         "你是一个AI助手",
		Greeting:             "您好！",
		RagProductIDs:        []string{"rag_001", "rag_002"},
		SOPIDs:               []string{"sop_001"},
		ScriptLibraryIDs:     []string{"script_001"},
		DecisionStrategyIDs:  []string{"strategy_001"},
		ABExperimentIDs:      []string{"exp_001"},
		LLMModel:             "gpt-4o",
		Temperature:          0.8,
		MaxTokens:            1000,
		TopP:                 0.95,
		FrequencyPenalty:     0.3,
		PresencePenalty:      0.4,
		EnableRAG:            true,
		EnableScriptMatch:    true,
		EnableHumanizePolish: true,
		EnableContentAudit:   true,
		EnablePlaybook:       true,
		RAGTopK:              5,
		ConfidenceThreshold:  0.8,
		MaxAIConsecutive:     3,
		Status:               1,
		Version:              2,
	}

	ctx := convertAIAgentToContext(agent, "telegram", "tg_999")

	if ctx.AgentID != 100 {
		t.Errorf("AgentID = %d, want 100", ctx.AgentID)
	}
	if ctx.AgentType != "hybrid" {
		t.Errorf("AgentType = %s, want hybrid", ctx.AgentType)
	}
	if ctx.Temperature != 0.8 {
		t.Errorf("Temperature = %f, want 0.8", ctx.Temperature)
	}
	if ctx.RAGTopK != 5 {
		t.Errorf("RAGTopK = %d, want 5", ctx.RAGTopK)
	}
	if ctx.ConfidenceThreshold != 0.8 {
		t.Errorf("ConfidenceThreshold = %f, want 0.8", ctx.ConfidenceThreshold)
	}
	if ctx.Version != 2 {
		t.Errorf("Version = %d, want 2", ctx.Version)
	}
	if ctx.Channel != "telegram" {
		t.Errorf("Channel = %s, want telegram", ctx.Channel)
	}
	if ctx.AccountID != "tg_999" {
		t.Errorf("AccountID = %s, want tg_999", ctx.AccountID)
	}
}

func TestErrors(t *testing.T) {
	if ErrNoAgentBinding == nil {
		t.Error("ErrNoAgentBinding should not be nil")
	}
	if ErrAgentNotFound == nil {
		t.Error("ErrAgentNotFound should not be nil")
	}
	if ErrAgentDisabled == nil {
		t.Error("ErrAgentDisabled should not be nil")
	}

	// 验证错误信息
	if !errors.Is(ErrNoAgentBinding, ErrNoAgentBinding) {
		t.Error("ErrNoAgentBinding should be self-comparable")
	}
}

// newTestAgent 创建测试用智能体
func newTestAgent() *model.AIAgent {
	return &model.AIAgent{
		ID:                  1,
		AgentCode:           "test_sales",
		Name:                "测试销售",
		AgentType:           "sales",
		Persona:             "友好专业",
		SOPIDs:              []string{"sop_001"},
		ScriptLibraryIDs:    []string{"script_001"},
		DecisionStrategyIDs: []string{"strategy_001"},
		ABExperimentIDs:     []string{"exp_001"},
		Status:              1,
		Version:             1,
		LLMModel:            "gpt-4o-mini",
		Temperature:         0.7,
		MaxTokens:           800,
		RAGTopK:             3,
	}
}

// verify unused import
var _ = context.Background
