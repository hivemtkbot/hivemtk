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
	// B-022: 仅使用白名单字段 (customer_id/intent/stage/agent_name/product_name)
	vars := map[string]any{
		"customer_id":  "张三",
		"product_name": "纸皮核桃",
		"agent_name":   "顺丰",
	}
	tpl := "亲 {{.customer_id}}，{{.product_name}} 发 {{.agent_name}} 哦"
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

// ----------------------------------------------------------------------------
// B-022 SOP 模板 SSTI 白名单测试
// ----------------------------------------------------------------------------

// TestSOPTemplate_Render_UserMessageNotAllowed 验证 user_message 不入模板 (B-022)
//
// 攻击场景: 模板里写 {{.user_message}}, 攻击者输入 {{ .Intent }} 之类的模板注入。
// 修复: Render 走白名单 filterWhitelistVars, user_message 被丢弃。
func TestSOPTemplate_Render_UserMessageNotAllowed(t *testing.T) {
	svc := &SOPTemplateService{}
	tpl := "USER=[{{.user_message}}]"
	vars := map[string]any{
		"customer_id":  "c123",
		"intent":       "logistics",
		"user_message": "{{.SneakyTemplate}}", // 攻击 payload
	}
	got, err := svc.Render(tpl, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// user_message 应该是 "<no value>" (missingkey=zero)
	if strings.Contains(got, "SneakyTemplate") {
		t.Errorf("user_message should be filtered, but got leaked payload: %q", got)
	}
	if !strings.Contains(got, "USER=[<no value>]") {
		t.Errorf("expected user_message to be <no value>, got %q", got)
	}
}

// TestSOPTemplate_Render_OnlyWhitelistVars 验证只有白名单字段透传 (B-022)
func TestSOPTemplate_Render_OnlyWhitelistVars(t *testing.T) {
	svc := &SOPTemplateService{}
	tpl := "客户={{.customer_id}} 意图={{.intent}} 阶段={{.stage}} 坐席={{.agent_name}} 商品={{.product_name}} 意图名={{.intent_name}}"
	vars := map[string]any{
		"customer_id":   "c001",
		"intent":        "logistics",
		"stage":         "initial",
		"agent_name":    "小薇",
		"product_name":  "纸皮核桃",
		"intent_name":   "物流查询",
		"user_message":  "SHOULD_NOT_APPEAR",
		"admin_secret":  "topsecret",
		"jwt_token":     "ey...",
		"db_password":   "p@ssw0rd",
	}
	got, err := svc.Render(tpl, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "客户=c001 意图=logistics 阶段=initial 坐席=小薇 商品=纸皮核桃 意图名=物流查询"
	if got != want {
		t.Errorf("whitelist mismatch:\n got:  %q\n want: %q", got, want)
	}
	for _, leaked := range []string{"SHOULD_NOT_APPEAR", "topsecret", "ey...", "p@ssw0rd"} {
		if strings.Contains(got, leaked) {
			t.Errorf("non-whitelisted field leaked: %q", leaked)
		}
	}
}

// TestSOPTemplate_filterWhitelistVars_NilSafe 验证 nil 入参安全 (B-022)
func TestSOPTemplate_filterWhitelistVars_NilSafe(t *testing.T) {
	out := filterWhitelistVars(nil)
	if out == nil {
		t.Error("expected non-nil empty map for nil input")
	}
	if len(out) != 0 {
		t.Errorf("expected empty map, got %v", out)
	}
}

// TestSOPTemplate_filterWhitelistVars_DoesNotMutate 验证不修改入参 (B-022)
func TestSOPTemplate_filterWhitelistVars_DoesNotMutate(t *testing.T) {
	vars := map[string]any{
		"customer_id":  "c1",
		"user_message": "leak",
	}
	_ = filterWhitelistVars(vars)
	if v, ok := vars["user_message"]; !ok || v != "leak" {
		t.Errorf("filterWhitelistVars mutated input map, user_message=%v", v)
	}
}
