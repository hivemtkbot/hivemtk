package service

// faq_test.go FAQ Service 单元测试 (T26)
//
// 设计依据: 2026-07-31 AI 智能体性能优化 (T9)
//
// 测试目标:
//   - Match: 返回的匹配项数 + 排序 (高分在前)
//   - ShouldSkipLLM: 高分命中 -> true; 低分不命中 -> false
//   - 不依赖真实 DB (使用 mock FAQRepository 或 nil 短路)
//
// 注意: FAQService.repo == nil 时 Match/Stats 直接返回 nil (不 panic),
//       可用此特性测试空仓库场景.

import (
	"testing"

	"marketing/internal/dto"
)

// fakeFAQEntry 用于测试的 FAQ 匹配项
type fakeFAQEntry struct {
	entry  *dto.FAQEntry
	score  float64
	rank   int
	hits   int64
	mType  string
}

// makeFAQMatchResult 构造测试用 FAQMatchResult
func makeFAQMatchResult(q, a, intent string, score float64) dto.FAQMatchResult {
	return dto.FAQMatchResult{
		Entry: &dto.FAQEntry{
			ID:         0,
			Question:   q,
			Answer:     a,
			Intent:     intent,
			Confidence: score,
		},
		Score:     score,
		Rank:      0,
		MatchType: "keyword",
	}
}

// TestFAQService_Match_NilRepo 测试 repo==nil 时的安全行为
func TestFAQService_Match_NilRepo(t *testing.T) {
	svc := &FAQService{repo: nil, db: nil}
	matches, err := svc.Match(nil, "你好", 3)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if matches != nil {
		t.Errorf("expected nil matches for nil repo, got %v", matches)
	}
}

// TestFAQService_ShouldSkipLLM_HighScore 测试高分命中 -> SkipLLM=true
func TestFAQService_ShouldSkipLLM_HighScore(t *testing.T) {
	svc := &FAQService{repo: nil, db: nil}
	matches := []dto.FAQMatchResult{
		makeFAQMatchResult("韵达发货吗", "韵达不发的哦", "logistics", 0.85),
	}
	skip, top := svc.ShouldSkipLLM(matches)
	if !skip {
		t.Error("expected skip=true for high-score match")
	}
	if top == nil {
		t.Fatal("expected non-nil top match")
	}
	if top.Score < faqHitThresh {
		t.Errorf("expected score >= %f, got %f", faqHitThresh, top.Score)
	}
}

// TestFAQService_ShouldSkipLLM_LowScore 测试低分命中 -> SkipLLM=false
func TestFAQService_ShouldSkipLLM_LowScore(t *testing.T) {
	svc := &FAQService{repo: nil, db: nil}
	matches := []dto.FAQMatchResult{
		makeFAQMatchResult("模糊问句", "回答", "unknown", 0.3),
	}
	skip, top := svc.ShouldSkipLLM(matches)
	if skip {
		t.Error("expected skip=false for low-score match")
	}
	if top != nil {
		t.Error("expected nil top when not skipping")
	}
}

// TestFAQService_ShouldSkipLLM_Empty 测试空匹配列表 -> SkipLLM=false
func TestFAQService_ShouldSkipLLM_Empty(t *testing.T) {
	svc := &FAQService{repo: nil, db: nil}
	skip, top := svc.ShouldSkipLLM(nil)
	if skip {
		t.Error("expected skip=false for empty matches")
	}
	if top != nil {
		t.Error("expected nil top for empty matches")
	}
}

// TestFAQService_ShouldSkipLLM_Boundary 测试临界值
func TestFAQService_ShouldSkipLLM_Boundary(t *testing.T) {
	svc := &FAQService{repo: nil, db: nil}

	// 恰好等于阈值 -> 应 skip
	matches := []dto.FAQMatchResult{
		makeFAQMatchResult("临界", "临界回复", "logistics", faqHitThresh),
	}
	skip, _ := svc.ShouldSkipLLM(matches)
	if !skip {
		t.Errorf("expected skip=true at exact threshold %f", faqHitThresh)
	}

	// 略低于阈值 -> 不 skip
	matches2 := []dto.FAQMatchResult{
		makeFAQMatchResult("略低", "略低回复", "logistics", faqHitThresh-0.01),
	}
	skip2, _ := svc.ShouldSkipLLM(matches2)
	if skip2 {
		t.Errorf("expected skip=false just below threshold %f", faqHitThresh)
	}
}

// TestFAQService_IncrementHitCount_NilRepo 测试 id=0 / nil repo 安全
func TestFAQService_IncrementHitCount_NilRepo(t *testing.T) {
	svc := &FAQService{repo: nil, db: nil}
	// 不应 panic
	svc.IncrementHitCount(nil, 0)
	svc.IncrementHitCount(nil, 1)
}

// TestFAQService_Stats_NilRepo 测试空仓库 Stats
func TestFAQService_Stats_NilRepo(t *testing.T) {
	svc := &FAQService{repo: nil, db: nil}
	total, enabled, err := svc.Stats(nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if total != 0 || enabled != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", total, enabled)
	}
}

// TestFAQService_InvalidateCache_NilSafe 测试 nil cache 也能安全失效
func TestFAQService_InvalidateCache_NilSafe(t *testing.T) {
	svc := &FAQService{repo: nil, db: nil}
	// 不应 panic
	svc.InvalidateCache()
}
