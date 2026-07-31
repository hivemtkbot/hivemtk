package service

// sop_template_test.go SOP Template Service 单元测试 (T26)
//
// 设计依据: 2026-07-31 AI 智能体性能优化 (T10)
//
// 测试目标:
//   - Render: 基本变量替换 ({{.var_name}} -> 实际值)
//   - Render: nil-safe + 错误处理
//   - ShouldSkipLLM: 模板 confidence 阈值判断
//   - 不依赖真实 DB (使用 nil repo 测试纯逻辑)

import (
	"strings"
	"testing"

	"marketing/internal/model"
)

// TestSOPTemplate_Render_BasicVars 测试基本变量替换
func TestSOPTemplate_Render_BasicVars(t *testing.T) {
	svc := &SOPTemplateService{}
	vars := map[string]any{
		"customer_name": "张三",
		"product":       "纸皮核桃",
		"express":       "顺丰",
	}
	tpl := "亲 {{.customer_name}}，{{.product}} 发 {{.express}} 哦"
	got, err := svc.Render(tpl, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "亲 张三，纸皮核桃 发 顺丰 哦"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestSOPTemplate_Render_EmptyTemplate 测试空模板
func TestSOPTemplate_Render_EmptyTemplate(t *testing.T) {
	svc := &SOPTemplateService{}
	got, err := svc.Render("", map[string]any{"a": 1})
	if err != nil {
		t.Errorf("expected nil error for empty template, got %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// TestSOPTemplate_Render_MissingKey 测试缺失变量 (missingkey=zero 模式)
func TestSOPTemplate_Render_MissingKey(t *testing.T) {
	svc := &SOPTemplateService{}
	tpl := "你好 {{.unknown_var}}"
	got, err := svc.Render(tpl, map[string]any{})
	if err != nil {
		t.Fatalf("expected nil error with missingkey=zero, got %v", err)
	}
	// missingkey=zero 会输出 "<no value>"
	if !strings.Contains(got, "你好") {
		t.Errorf("expected result to contain 你好, got %q", got)
	}
}

// TestSOPTemplate_Render_InvalidSyntax 测试非法模板语法
func TestSOPTemplate_Render_InvalidSyntax(t *testing.T) {
	svc := &SOPTemplateService{}
	tpl := "非法 {{ .unclosed"
	_, err := svc.Render(tpl, nil)
	if err == nil {
		t.Error("expected error for invalid template syntax")
	}
}

// TestSOPTemplate_ShouldSkipLLM_HighConfidence 测试高置信度跳过 LLM
func TestSOPTemplate_ShouldSkipLLM_HighConfidence(t *testing.T) {
	svc := &SOPTemplateService{}
	tpl := &model.SOPTemplate{Confidence: 0.85}
	if !svc.ShouldSkipLLM(tpl) {
		t.Error("expected skip=true for confidence=0.85")
	}
}

// TestSOPTemplate_ShouldSkipLLM_LowConfidence 测试低置信度不跳过
func TestSOPTemplate_ShouldSkipLLM_LowConfidence(t *testing.T) {
	svc := &SOPTemplateService{}
	tpl := &model.SOPTemplate{Confidence: 0.5}
	if svc.ShouldSkipLLM(tpl) {
		t.Error("expected skip=false for confidence=0.5")
	}
}

// TestSOPTemplate_ShouldSkipLLM_NilTemplate 测试 nil 模板
func TestSOPTemplate_ShouldSkipLLM_NilTemplate(t *testing.T) {
	svc := &SOPTemplateService{}
	if svc.ShouldSkipLLM(nil) {
		t.Error("expected skip=false for nil template")
	}
}

// TestSOPTemplate_BuildLayer1Reply 测试完整 Layer1 回复构造
func TestSOPTemplate_BuildLayer1Reply(t *testing.T) {
	svc := &SOPTemplateService{}
	tpl := &model.SOPTemplate{
		Template:   "{{.intent}} 阶段 {{.stage}}: 你好",
		Confidence: 0.9,
	}
	vars := map[string]any{
		"intent": "greeting",
		"stage":  "initial",
	}
	got, err := svc.BuildLayer1Reply(tpl, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "greeting 阶段 initial: 你好"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestSOPTemplate_BuildLayer1Reply_NilTpl 测试 nil 模板
func TestSOPTemplate_BuildLayer1Reply_NilTpl(t *testing.T) {
	svc := &SOPTemplateService{}
	got, err := svc.BuildLayer1Reply(nil, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// TestSOPTemplate_IncrementHitCount_NilRepo 测试 nil repo + id=0 安全
func TestSOPTemplate_IncrementHitCount_NilRepo(t *testing.T) {
	svc := &SOPTemplateService{}
	// 不应 panic
	svc.IncrementHitCount(nil, 0)
	svc.IncrementHitCount(nil, 1)
}

// TestSOPTemplate_InvalidateCache_NilSafe 测试 nil cache 安全
func TestSOPTemplate_InvalidateCache_NilSafe(t *testing.T) {
	svc := &SOPTemplateService{}
	// 不应 panic
	svc.InvalidateCache()
}

// TestSOPTemplate_MatchByIntent_NilRepo 测试空仓库
func TestSOPTemplate_MatchByIntent_NilRepo(t *testing.T) {
	svc := &SOPTemplateService{repo: nil}
	got, err := svc.MatchByIntent(nil, "logistics")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil matches for nil repo, got %v", got)
	}

	// intent 为空也直接返回
	got2, _ := svc.MatchByIntent(nil, "")
	if got2 != nil {
		t.Errorf("expected nil for empty intent, got %v", got2)
	}
}

// TestSOPTemplate_MatchByIntentStage_NilRepo 测试 (intent, stage) 空仓库
func TestSOPTemplate_MatchByIntentStage_NilRepo(t *testing.T) {
	svc := &SOPTemplateService{repo: nil}
	got, err := svc.MatchByIntentStage(nil, "logistics", "initial")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil matches for nil repo, got %v", got)
	}
}
