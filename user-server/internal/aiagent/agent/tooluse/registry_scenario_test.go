package tooluse

import (
	"context"
	"testing"
)

type scenarioMockTool struct {
	name string
	cat  ToolCategory
}

func (m *scenarioMockTool) Name() string               { return m.name }
func (m *scenarioMockTool) Category() ToolCategory     { return m.cat }
func (m *scenarioMockTool) Description() string        { return "mock" }
func (m *scenarioMockTool) Parameters() ToolParameters { return ToolParameters{Type: "object"} }
func (m *scenarioMockTool) Execute(_ context.Context, _ map[string]any) (ToolResult, error) {
	return ToolResult{Success: true}, nil
}

func newScenarioTestRegistry() *ToolRegistry {
	r := NewToolRegistry()
	r.MustRegister(&scenarioMockTool{name: "rag.search", cat: CategoryKnowledge})
	r.MustRegister(&scenarioMockTool{name: "knowledge.list_kb", cat: CategoryKnowledge})
	r.MustRegister(&scenarioMockTool{name: "customer.search", cat: CategoryCustomer})
	r.MustRegister(&scenarioMockTool{name: "reach.sms.send", cat: CategoryReach})
	r.MustRegister(&scenarioMockTool{name: "order.lookup", cat: CategoryBusiness})
	r.MustRegister(&scenarioMockTool{name: "pm.message.send", cat: CategoryPrivateMessage})
	return r
}

func names(fns []LLMFunction) map[string]bool {
	out := make(map[string]bool, len(fns))
	for _, fn := range fns {
		out[fn.Name] = true
	}
	return out
}

// 正例：intent_recognize 只暴露 knowledge/customer 类
func TestToLLMFunctionsForScenario_IntentRecognize(t *testing.T) {
	r := newScenarioTestRegistry()
	got := names(r.ToLLMFunctionsForScenario(ScenarioIntentRecognize))
	if len(got) != 3 {
		t.Fatalf("expected 3 functions, got %v", got)
	}
	for _, want := range []string{"rag.search", "knowledge.list_kb", "customer.search"} {
		if !got[want] {
			t.Errorf("expected %q in whitelist export", want)
		}
	}
	for _, banned := range []string{"reach.sms.send", "order.lookup", "pm.message.send"} {
		if got[banned] {
			t.Errorf("%q must be trimmed for intent_recognize scenario", banned)
		}
	}
}

// 反例：sales/cs 等未配置场景全量（向后兼容）
func TestToLLMFunctionsForScenario_FullScenarios(t *testing.T) {
	r := newScenarioTestRegistry()
	full := r.ToLLMFunctions()
	for _, scenario := range []string{"", "sales", "cs", "unknown_future"} {
		got := r.ToLLMFunctionsForScenario(scenario)
		if len(got) != len(full) {
			t.Errorf("scenario %q: expected full set (%d), got %d", scenario, len(full), len(got))
		}
	}
}

func TestScenarioAllowedCategories(t *testing.T) {
	cats, restricted := ScenarioAllowedCategories(ScenarioIntentRecognize)
	if !restricted || len(cats) != 2 {
		t.Errorf("intent_recognize should be restricted to 2 categories, got %v restricted=%v", cats, restricted)
	}
	if _, restricted := ScenarioAllowedCategories("sales"); restricted {
		t.Error("sales should not be restricted (full set)")
	}
}

// ScenarioAllowsTool：全局注册中心路径的正反例
func TestScenarioAllowsTool(t *testing.T) {
	prev := globalRegistry
	globalRegistryOnce.Do(func() {})
	globalRegistry = newScenarioTestRegistry()
	defer func() { globalRegistry = prev }()

	if !ScenarioAllowsTool(ScenarioIntentRecognize, "customer.search") {
		t.Error("customer.search should be allowed in intent_recognize")
	}
	if ScenarioAllowsTool(ScenarioIntentRecognize, "reach.sms.send") {
		t.Error("reach.sms.send should be blocked in intent_recognize")
	}
	if !ScenarioAllowsTool("sales", "reach.sms.send") {
		t.Error("unrestricted scenario should allow any registered tool")
	}
	if ScenarioAllowsTool(ScenarioIntentRecognize, "not.registered") {
		t.Error("unregistered tool should be blocked in restricted scenario")
	}
}
