package llm

import "testing"

// TestPickEnabledFallback 验证「路由候选全部不可用」时，能自动兜底到
// 任意已启用且通过质量门禁的 provider（按质量分降序）。
// 对应修复：仅启用某个云端模型（如 sensenova）、关闭其余时，
// 路由默认指向已禁用的 primary_provider，对话应自动落到启用的模型而非降级。
func TestPickEnabledFallback(t *testing.T) {
	d := NewDispatcher(NewLLMService())
	// 清空种子 provider/路由，仅保留受控 fixture，避免种子干扰断言
	d.providers = map[string]*ProviderConfig{}
	d.routes = map[DispatchScenario]*ScenarioRoute{}

	// 默认路由指向的 primary 已禁用，sensenova 启用且质量最高
	d.providers["deepseek"] = &ProviderConfig{Name: "deepseek", Enabled: false, QualityScore: 0.90}
	d.providers["sensenova"] = &ProviderConfig{Name: "sensenova", Enabled: true, QualityScore: 0.95}
	d.providers["default"] = &ProviderConfig{Name: "default", Enabled: false, QualityScore: 0.99}

	route := &ScenarioRoute{Scenario: ScenarioSOPReply, Provider: "deepseek", MinQuality: 0.85}
	if got := d.pickEnabledFallback(route); got != "sensenova" {
		t.Fatalf("期望兜底到 sensenova，实际=%q", got)
	}

	// 当唯一启用的 provider 质量低于场景门禁时，应无兜底
	d2 := NewDispatcher(NewLLMService())
	d2.providers = map[string]*ProviderConfig{}
	d2.providers["lowq"] = &ProviderConfig{Name: "lowq", Enabled: true, QualityScore: 0.50}
	if got := d2.pickEnabledFallback(&ScenarioRoute{Scenario: ScenarioHighQuality, Provider: "disabled", MinQuality: 0.95}); got != "" {
		t.Fatalf("质量不达门禁时不应兜底，实际=%q", got)
	}

	// 全部禁用时，应无兜底
	d3 := NewDispatcher(NewLLMService())
	d3.providers = map[string]*ProviderConfig{}
	d3.providers["a"] = &ProviderConfig{Name: "a", Enabled: false}
	if got := d3.pickEnabledFallback(&ScenarioRoute{Scenario: ScenarioSOPReply, Provider: "a"}); got != "" {
		t.Fatalf("全部禁用时不应兜底，实际=%q", got)
	}
}
