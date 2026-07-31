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
	"context"
	"fmt"
	"testing"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
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

// ----------------------------------------------------------------------------
// B-021 WeekDecay 测试
// ----------------------------------------------------------------------------

// mockFAQRepoForDecay 专用于 WeekDecay 测试的 mock (B-021)
type mockFAQRepoForDecay struct {
	candidates       []model.FAQEntry
	cutoffSeen       time.Time
	decayCalls       []struct {
		ID    uint
		Decay float64
	}
	listErr          error
	decayErr         error
}

func (m *mockFAQRepoForDecay) MatchByKeyword(ctx context.Context, msg string, topK int) ([]model.FAQEntry, error) {
	return nil, nil
}

func (m *mockFAQRepoForDecay) MatchByIDs(ctx context.Context, msg string, ids []string, topK int) ([]model.FAQEntry, error) {
	return nil, nil
}

func (m *mockFAQRepoForDecay) IncrementHitCount(ctx context.Context, id uint) error {
	return nil
}

func (m *mockFAQRepoForDecay) DecayQuality(ctx context.Context, id uint, decay float64) error {
	m.decayCalls = append(m.decayCalls, struct {
		ID    uint
		Decay float64
	}{id, decay})
	if m.decayErr != nil {
		return m.decayErr
	}
	return nil
}

func (m *mockFAQRepoForDecay) ListDecayCandidates(ctx context.Context, cutoff time.Time, limit int) ([]model.FAQEntry, error) {
	m.cutoffSeen = cutoff
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.candidates, nil
}

func (m *mockFAQRepoForDecay) IncrementNegativeHit(ctx context.Context, id uint) error {
	return nil
}

func (m *mockFAQRepoForDecay) ListWithFilter(ctx context.Context, filter repository.FAQFilter) ([]model.FAQEntry, int64, error) {
	return nil, 0, nil
}

func (m *mockFAQRepoForDecay) GetByID(ctx context.Context, id uint) (*model.FAQEntry, error) {
	return nil, nil
}

func (m *mockFAQRepoForDecay) ListEnabled(ctx context.Context, limit int) ([]model.FAQEntry, error) {
	return nil, nil
}

func (m *mockFAQRepoForDecay) Create(ctx context.Context, entry *model.FAQEntry) error {
	return nil
}

func (m *mockFAQRepoForDecay) Update(ctx context.Context, id uint, entry *model.FAQEntry) error {
	return nil
}

func (m *mockFAQRepoForDecay) Delete(ctx context.Context, id uint) error {
	return nil
}

// fixedClock 固定时钟 (用于 WeekDecay 单测)
type fixedClock struct{ T time.Time }

func (f fixedClock) Now() time.Time { return f.T }

// TestFAQService_WeekDecay 测试周度质量衰减 (B-021)
//
// 验证:
//  1. cutoff = now - 7d
//  2. 候选列表由 ListDecayCandidates 返回, 逐条调用 DecayQuality(0.1)
//  3. 返回值为实际衰减条数
//  4. 时钟通过 mock 注入
func TestFAQService_WeekDecay(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	clock := fixedClock{T: now}
	repo := &mockFAQRepoForDecay{
		candidates: []model.FAQEntry{
			{ID: 1, HitCount: 2, LastHitAt: ptrTimeUniq(now.Add(-8 * 24 * time.Hour))},
			{ID: 2, HitCount: 0, LastHitAt: ptrTimeUniq(now.Add(-30 * 24 * time.Hour))},
			{ID: 3, HitCount: 4, LastHitAt: ptrTimeUniq(now.Add(-10 * 24 * time.Hour))},
		},
	}
	svc := NewFAQServiceWithRepo(repo)
	svc.SetClock(clock)

	decayed, err := svc.WeekDecay(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decayed != 3 {
		t.Errorf("expected decayed=3, got %d", decayed)
	}
	if len(repo.decayCalls) != 3 {
		t.Fatalf("expected 3 DecayQuality calls, got %d", len(repo.decayCalls))
	}
	for i, c := range repo.decayCalls {
		if c.ID != uint(i+1) {
			t.Errorf("call[%d].ID = %d, want %d", i, c.ID, i+1)
		}
		if c.Decay != faqDecayPerWeek {
			t.Errorf("call[%d].Decay = %f, want %f", i, c.Decay, faqDecayPerWeek)
		}
	}
	wantCutoff := now.Add(-faqDecayDays)
	if !repo.cutoffSeen.Equal(wantCutoff) {
		t.Errorf("cutoff mismatch: got %v, want %v", repo.cutoffSeen, wantCutoff)
	}
}

// TestFAQService_WeekDecay_NilRepo nil repo 安全
func TestFAQService_WeekDecay_NilRepo(t *testing.T) {
	svc := &FAQService{repo: nil, db: nil, clock: fixedClock{T: time.Now()}}
	decayed, err := svc.WeekDecay(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if decayed != 0 {
		t.Errorf("expected decayed=0, got %d", decayed)
	}
}

// TestFAQService_WeekDecay_ListErr list 报错时返回错误
func TestFAQService_WeekDecay_ListErr(t *testing.T) {
	repo := &mockFAQRepoForDecay{listErr: fmt.Errorf("db down")}
	svc := NewFAQServiceWithRepo(repo)
	_, err := svc.WeekDecay(context.Background())
	if err == nil {
		t.Error("expected error when ListDecayCandidates fails")
	}
}

// ptrTimeUniq 构造 *time.Time (helper, 避开 service.ptrTime 重定义)
func ptrTimeUniq(t time.Time) *time.Time { return &t }
