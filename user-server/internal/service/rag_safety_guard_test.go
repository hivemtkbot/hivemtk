package service

// rag_safety_guard_test.go RAG 内容风控卫士单元测试
//
// 测试覆盖：
//  1) 构造与默认词库
//  2) 敏感词命中 block
//  3) 广告法绝对化用语 warn
//  4) 竞品词 block
//  5) 画像越权 warn
//  6) 同时命中多个规则
//  7) 词库动态新增 / 去重
//  8) FilterSourcesByTenant
//  9) safeReplacement
// 10) 边界：nil req / 空 content / 防御性

import (
	"context"
	"strings"
	"testing"
	"time"
)

func newSafetyGuard() *RagSafetyGuardService {
	return NewRagSafetyGuardService(nil)
}

// 1) 构造
func TestRagSafetyGuard_NewService(t *testing.T) {
	s := newSafetyGuard()
	if s == nil {
		t.Fatal("Expected non-nil service")
	}
	lex := s.GetLexicon()
	if len(lex.AdPhrases) == 0 {
		t.Error("Expected default ad phrases non-empty")
	}
	if s.LastUpdate().IsZero() {
		// 默认构造 updatedAt 为零，允许
		_ = s.LastUpdate()
	}
}

// 2) 敏感词命中 block
func TestRagSafetyGuard_SensitiveWord_Block(t *testing.T) {
	s := newSafetyGuard()
	_ = s.AddSensitiveWord("违禁词TEST")
	res, err := s.Check(context.Background(), &SafetyCheckRequest{
		UserID:  "u1",
		Content: "这是一段包含违禁词TEST的内容",
		Stage:   "output",
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if res.Passed {
		t.Error("Expected not passed")
	}
	if !res.Blocked {
		t.Error("Expected blocked")
	}
	if len(res.Issues) == 0 {
		t.Fatal("Expected at least one issue")
	}
	if res.Issues[0].Type != SafetyIssueSensitiveWord {
		t.Errorf("Expected type sensitive_word, got %s", res.Issues[0].Type)
	}
	if res.Issues[0].Severity != SafetySeverityBlock {
		t.Errorf("Expected severity block, got %s", res.Issues[0].Severity)
	}
	if res.ReplacedContent == "" {
		t.Error("Expected replaced content on block")
	}
}

// 3) 广告法绝对化用语 warn（不阻断）
func TestRagSafetyGuard_AdCompliance_Warn(t *testing.T) {
	s := newSafetyGuard()
	res, err := s.Check(context.Background(), &SafetyCheckRequest{
		Content: "我们的产品是行业最佳",
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if res.Blocked {
		t.Error("Ad compliance should warn, not block")
	}
	if !res.Passed {
		t.Error("Ad compliance warn should still pass")
	}
	hasAd := false
	for _, iss := range res.Issues {
		if iss.Type == SafetyIssueAdCompliance {
			hasAd = true
			if iss.Severity != SafetySeverityWarn {
				t.Errorf("Expected warn, got %s", iss.Severity)
			}
		}
	}
	if !hasAd {
		t.Error("Expected ad compliance issue")
	}
}

// 4) 竞品词 block
func TestRagSafetyGuard_Competitor_Block(t *testing.T) {
	s := newSafetyGuard()
	_ = s.AddCompetitorWord("竞品X")
	res, err := s.Check(context.Background(), &SafetyCheckRequest{
		Content: "对比竞品X 我们更便宜",
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !res.Blocked {
		t.Error("Competitor should block")
	}
	found := false
	for _, iss := range res.Issues {
		if iss.Type == SafetyIssueCompetitor {
			found = true
		}
	}
	if !found {
		t.Error("Expected competitor issue")
	}
}

// 5) 画像越权
func TestRagSafetyGuard_PersonaAuthz(t *testing.T) {
	s := newSafetyGuard()
	res, err := s.Check(context.Background(), &SafetyCheckRequest{
		TenantID: "tenant-A",
		Sources: []SafetySource{
			{DocID: "d1", OwnerID: "tenant-A", Content: "A 知识"},
			{DocID: "d2", OwnerID: "tenant-B", Content: "B 知识"},
		},
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if res.Blocked {
		t.Error("Persona authz should warn, not block")
	}
	count := 0
	for _, iss := range res.Issues {
		if iss.Type == SafetyIssuePersonaAuthz {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Expected 1 persona issue, got %d", count)
	}
}

// 6) 同时命中多个规则
func TestRagSafetyGuard_MultipleRules(t *testing.T) {
	s := newSafetyGuard()
	_ = s.AddSensitiveWord("BOMB")
	_ = s.AddCompetitorWord("COMPET")

	res, err := s.Check(context.Background(), &SafetyCheckRequest{
		Content: "BOMB vs COMPET 我们最佳",
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !res.Blocked {
		t.Error("Sensitive + Competitor should block")
	}
	types := map[SafetyIssueType]bool{}
	for _, iss := range res.Issues {
		types[iss.Type] = true
	}
	for _, want := range []SafetyIssueType{
		SafetyIssueSensitiveWord, SafetyIssueAdCompliance, SafetyIssueCompetitor,
	} {
		if !types[want] {
			t.Errorf("Expected issue type %s", want)
		}
	}
}

// 7) 词库动态新增 / 去重
func TestRagSafetyGuard_Lexicon_Dedup(t *testing.T) {
	s := newSafetyGuard()
	_ = s.AddSensitiveWord("A")
	_ = s.AddSensitiveWord("A")
	_ = s.AddSensitiveWord("B")
	lex := s.GetLexicon()
	count := 0
	for _, w := range lex.SensitiveWords {
		if w == "A" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Expected dedup, got %d", count)
	}
}

// 8) FilterSourcesByTenant
func TestRagSafetyGuard_FilterSourcesByTenant(t *testing.T) {
	s := newSafetyGuard()
	srcs := []SafetySource{
		{DocID: "1", OwnerID: "T1"},
		{DocID: "2", OwnerID: "T2"},
		{DocID: "3", OwnerID: "T1"},
		{DocID: "4", OwnerID: ""},
	}
	kept, dropped := s.FilterSourcesByTenant(srcs, "T1")
	if dropped != 1 {
		t.Errorf("Expected dropped=1, got %d", dropped)
	}
	if len(kept) != 3 {
		t.Errorf("Expected kept=3, got %d", len(kept))
	}
}

// 9) safeReplacement
func TestRagSafetyGuard_SafeReplacement(t *testing.T) {
	s := newSafetyGuard()
	r := s.safeReplacement()
	if r == "" {
		t.Error("Expected non-empty replacement")
	}
}

// 10) 边界
func TestRagSafetyGuard_NilRequest(t *testing.T) {
	s := newSafetyGuard()
	_, err := s.Check(context.Background(), nil)
	if err == nil {
		t.Error("Expected error for nil request")
	}
}

func TestRagSafetyGuard_EmptyContent(t *testing.T) {
	s := newSafetyGuard()
	res, err := s.Check(context.Background(), &SafetyCheckRequest{Content: ""})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !res.Passed {
		t.Error("Empty content should pass")
	}
}

func TestRagSafetyGuard_AddWordEmpty(t *testing.T) {
	s := newSafetyGuard()
	if err := s.AddSensitiveWord("   "); err == nil {
		t.Error("Expected error for empty word")
	}
	if err := s.AddCompetitorWord("   "); err == nil {
		t.Error("Expected error for empty competitor word")
	}
}

func TestRagSafetyGuard_Latency(t *testing.T) {
	s := newSafetyGuard()
	start := time.Now()
	res, _ := s.Check(context.Background(), &SafetyCheckRequest{
		Content: "一段长文本..." + strings.Repeat("字", 200),
	})
	if time.Since(start) > 100*time.Millisecond {
		t.Errorf("Check too slow: %v", time.Since(start))
	}
	if res.LatencyMs < 0 {
		t.Error("LatencyMs should be non-negative")
	}
}

func TestRagSafetyGuard_SetLexicon(t *testing.T) {
	s := newSafetyGuard()
	s.SetLexicon(SafetyLexicon{
		SensitiveWords:  []string{"x", "y", "x"}, // 重复
		AdPhrases:       []string{"a"},
		CompetitorWords: []string{"c"},
	})
	lex := s.GetLexicon()
	if len(lex.SensitiveWords) != 2 {
		t.Errorf("Expected dedup to 2, got %d", len(lex.SensitiveWords))
	}
	if s.LastUpdate().IsZero() {
		t.Error("Expected updated_at set")
	}
}

func TestRagSafetyGuard_StageIgnored(t *testing.T) {
	s := newSafetyGuard()
	_ = s.AddSensitiveWord("BANG")
	for _, stage := range []string{"input", "output", "retrieval"} {
		res, _ := s.Check(context.Background(), &SafetyCheckRequest{
			Stage:   stage,
			Content: "BANG",
		})
		if !res.Blocked {
			t.Errorf("Stage=%s should still block", stage)
		}
	}
}
