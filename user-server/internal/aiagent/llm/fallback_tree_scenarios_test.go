package llm

import (
	"strings"
	"testing"
)

// ============================================================================
// M11：fallback_tree 场景差异化兜底模板回归测试
// 原先所有场景共用单一兜底文案；修复后按场景匹配差异化文案池，同场景轮换。
// ============================================================================

func TestTemplateReplyFor_ScenarioDifferentiation(t *testing.T) {
	sop := TemplateReplyFor(ScenarioSOPReply)
	objection := TemplateReplyFor(ScenarioObjection)
	chat := TemplateReplyFor(ScenarioFriendlyChat)
	intent := TemplateReplyFor(ScenarioIntentRecognize)

	if sop == objection || sop == chat || objection == chat {
		t.Errorf("不同场景应命中差异化文案池: sop=%q objection=%q chat=%q", sop, objection, chat)
	}
	// 意图识别兜底必须是合法 unknown 契约 JSON（fail-closed）
	if !strings.Contains(intent, `"major":"unknown"`) {
		t.Errorf("intent_recognize 兜底应为 unknown 意图 JSON, got %q", intent)
	}
}

func TestTemplateReplyFor_RotationWithinScenario(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		seen[TemplateReplyFor(ScenarioFriendlyChat)] = true
	}
	if len(seen) < 2 {
		t.Errorf("friendly_chat 场景应有多文案轮换, 仅出现 %d 种", len(seen))
	}
}

func TestTemplateReplyFor_UnknownScenarioFallsToDefault(t *testing.T) {
	got := TemplateReplyFor(DispatchScenario("no_such_scenario"))
	if got == "" {
		t.Fatal("未知场景应有通用兜底文案")
	}
	found := false
	for _, tpl := range defaultFallbackTemplates {
		if got == tpl {
			found = true
		}
	}
	if !found {
		t.Errorf("未知场景应回退通用文案池, got %q", got)
	}
}

func TestResolveDegradedTemplate_ConfiguredOverrides(t *testing.T) {
	custom := "运营定制话术 XYZ"
	if got := ResolveDegradedTemplate(ScenarioSOPReply, custom); got != custom {
		t.Errorf("显式定制文案应优先, got %q", got)
	}
	// 出厂默认不视为定制——保留场景轮换
	factoryDefault := "抱歉，当前服务暂时繁忙，请稍后再试或联系人工客服。"
	got := ResolveDegradedTemplate(ScenarioSOPReply, factoryDefault)
	foundInPool := false
	for _, tpl := range scenarioFallbackTemplates[ScenarioSOPReply] {
		if got == tpl {
			foundInPool = true
		}
	}
	if !foundInPool {
		t.Errorf("出厂默认配置不应覆盖场景轮换, got %q", got)
	}
}

func TestResolveDegradedTemplate_EmptyConfigUsesScenario(t *testing.T) {
	got := ResolveDegradedTemplate(ScenarioObjection, "")
	foundInPool := false
	for _, tpl := range scenarioFallbackTemplates[ScenarioObjection] {
		if got == tpl {
			foundInPool = true
		}
	}
	if !foundInPool {
		t.Errorf("空配置应走场景轮换, got %q", got)
	}
}
