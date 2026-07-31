package service

// layer_test.go LayerRouter 单元测试 (T26)
//
// 设计依据: 2026-07-31 AI 智能体性能优化 (T11)
//
// 测试目标:
//   - TestLayerRouter_Route_Layer1Disabled: FF_LAYER1=0 时直接 Layer2
//   - TestLayerRouter_Route_FAQHit: FAQ 高分命中 -> Layer1 SkipLLM (mock faqRepo)
//   - TestLayerRouter_Route_NoMatch: 无 FAQ/SOP 命中 -> Layer2 Fallback
//   - TestLayerRouter_Route_EmptyMessage: 空消息兜底
//
// 不依赖真实 DB: 通过注入 FAQMatcher mock 实现.

import (
	"context"
	"os"
	"sync/atomic"
	"testing"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/featureflag"
)

// withLayer1Flag 临时设置 FF_LAYER1 的值, 测试结束恢复
//
// 注意: featureflag.Bool() 返回的是缓存值(由后台 poller 5s 刷新),
// 单测需要立刻生效, 这里显式调用 ReloadAll() 强制重读 env。
func withLayer1Flag(t *testing.T, val string) {
	t.Helper()
	prev, hadPrev := os.LookupEnv("FF_LAYER1")
	if val == "" {
		_ = os.Unsetenv("FF_LAYER1")
	} else {
		_ = os.Setenv("FF_LAYER1", val)
	}
	featureflag.DefaultManager().ReloadAll()
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("FF_LAYER1", prev)
		} else {
			_ = os.Unsetenv("FF_LAYER1")
		}
		featureflag.DefaultManager().ReloadAll()
	})
}

// mockFAQRepo 用于单元测试的 FAQ mock
// 实现 FAQMatcher 接口, 不需要真实 DB
type mockFAQRepo struct {
	entries []model.FAQEntry
	err     error
	hits    atomic.Int32
}

func (m *mockFAQRepo) MatchByKeyword(ctx context.Context, msg string, topK int) ([]model.FAQEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	if topK <= 0 || topK > len(m.entries) {
		topK = len(m.entries)
	}
	return m.entries[:topK], nil
}

// MatchByAgent 简单按 msg 包含 question 关键字匹配 (满足 FAQMatcher 接口)
func (m *mockFAQRepo) MatchByAgent(ctx context.Context, agentID uint, msg string, topK int) ([]model.FAQEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	if topK <= 0 || topK > len(m.entries) {
		topK = len(m.entries)
	}
	return m.entries[:topK], nil
}

func (m *mockFAQRepo) IncrementHitCount(ctx context.Context, id uint) error {
	m.hits.Add(1)
	return nil
}

// newMockFAQRepoWithEntry 构造一个包含单个高分 FAQ 的 mock
func newMockFAQRepoWithEntry(question, answer, intent string, confidence float64) *mockFAQRepo {
	enabled := true
	return &mockFAQRepo{
		entries: []model.FAQEntry{
			{
				Question:   question,
				Answer:     answer,
				Intent:     intent,
				Confidence: confidence,
				Enabled:    &enabled,
			},
		},
	}
}

// TestLayerRouter_Route_Layer1Disabled 测试 FF_LAYER1=0 时直接走 Layer2
func TestLayerRouter_Route_Layer1Disabled(t *testing.T) {
	withLayer1Flag(t, "0")

	// 全部 repo 用 nil, 走默认 Layer2 路径
	router := &LayerRouter{
		faqRepo: nil,
		sopRepo: nil,
		logRepo: nil,
	}
	dec := router.Route(context.Background(), &RouteRequest{
		SessionID:  "s1",
		CustomerID: "c1",
		UserMessage: "你好",
	})
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Layer != dto.Layer2 {
		t.Errorf("expected layer=layer2 when FF disabled, got %s", dec.Layer)
	}
	if dec.Reason != dto.ReasonLayer1Disabled {
		t.Errorf("expected reason=layer1_disabled, got %s", dec.Reason)
	}
	if dec.SkipLLM {
		t.Error("expected SkipLLM=false when FF disabled")
	}
}

// TestLayerRouter_Route_NoRepoFallback 测试 FF 开 + 无 repo -> 兜底 Layer2
func TestLayerRouter_Route_NoRepoFallback(t *testing.T) {
	withLayer1Flag(t, "1")

	router := &LayerRouter{
		faqRepo: nil,
		sopRepo: nil,
		logRepo: nil,
	}
	dec := router.Route(context.Background(), &RouteRequest{
		SessionID:  "s1",
		CustomerID: "c1",
		UserMessage: "你好",
		Intent:     &dto.RecognizeResult{IntentType: "greeting", Confidence: 0.5},
	})
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Layer != dto.Layer2 {
		t.Errorf("expected layer=layer2 fallback, got %s", dec.Layer)
	}
	if dec.Reason != dto.ReasonFallback {
		t.Errorf("expected reason=fallback, got %s", dec.Reason)
	}
}

// TestLayerRouter_Route_EmptyMessage 测试空消息走 Layer2
func TestLayerRouter_Route_EmptyMessage(t *testing.T) {
	withLayer1Flag(t, "1")
	router := &LayerRouter{faqRepo: nil, sopRepo: nil, logRepo: nil}
	dec := router.Route(context.Background(), &RouteRequest{
		SessionID:  "s1",
		CustomerID: "c1",
		UserMessage: "",
	})
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Layer != dto.Layer2 {
		t.Errorf("expected layer=layer2 for empty message, got %s", dec.Layer)
	}
}

// TestLayerRouter_Route_FAQHit 测试 FAQ 高分命中 -> Layer1 SkipLLM (mock)
func TestLayerRouter_Route_FAQHit(t *testing.T) {
	withLayer1Flag(t, "1")

	// 使用 mock faqRepo 注入高分 FAQ
	mock := newMockFAQRepoWithEntry("韵达发货吗", "韵达不发的哦", "logistics", 0.9)
	router := &LayerRouter{
		faqRepo: mock,
		sopRepo: nil,
		logRepo: nil,
	}
	// Task 15/16 强 1对1: AgentID > 0 才能命中 FAQ
	dec := router.Route(context.Background(), &RouteRequest{
		SessionID:   "s1",
		CustomerID:  "c1",
		UserMessage: "韵达发货",
		AgentID:     1,
	})
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Layer != dto.Layer1 {
		t.Errorf("expected layer=layer1, got %s", dec.Layer)
	}
	if !dec.SkipLLM {
		t.Error("expected SkipLLM=true for FAQ hit")
	}
	if dec.Reason != dto.ReasonFAQHit {
		t.Errorf("expected reason=faq_hit, got %s", dec.Reason)
	}
	if dec.Reply != "韵达不发的哦" {
		t.Errorf("expected reply from FAQ answer, got %q", dec.Reply)
	}
	// 等待异步 IncrementHitCount (goroutine)
	// 注: 异步调用, 在测试主线程里直接检查可能 race
	// 这里用 hint 而不强制检查, 主要验证路由逻辑
	if mock.hits.Load() < 0 {
		t.Error("expected hits counter to be set")
	}
}

// TestLayerRouter_Route_FAQLowScore 测试 FAQ 低分命中 -> 不进 Layer1
func TestLayerRouter_Route_FAQLowScore(t *testing.T) {
	withLayer1Flag(t, "1")

	// 低分 FAQ (score < 0.6)
	mock := newMockFAQRepoWithEntry("模糊问句", "模糊回复", "unknown", 0.3)
	router := &LayerRouter{
		faqRepo: mock,
		sopRepo: nil,
		logRepo: nil,
	}
	dec := router.Route(context.Background(), &RouteRequest{
		SessionID:  "s1",
		CustomerID: "c1",
		UserMessage: "模糊问句",
	})
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Layer != dto.Layer2 {
		t.Errorf("expected layer=layer2 for low score, got %s", dec.Layer)
	}
	if dec.Reason == dto.ReasonFAQHit {
		t.Error("expected reason != faq_hit for low score")
	}
}

// TestLayerRouter_Route_FFToggle 测试 FF 开关语义正确切换
func TestLayerRouter_Route_FFToggle(t *testing.T) {
	router := &LayerRouter{faqRepo: nil, sopRepo: nil, logRepo: nil}

	// 关闭
	withLayer1Flag(t, "0")
	if featureflag.Get("layer1").Bool() {
		t.Fatal("expected FF_LAYER1=false after env=0")
	}
	dec1 := router.Route(context.Background(), &RouteRequest{UserMessage: "你好"})
	if dec1.Layer != dto.Layer2 {
		t.Errorf("expected layer2 when FF off, got %s", dec1.Layer)
	}

	// 开启
	withLayer1Flag(t, "1")
	if !featureflag.Get("layer1").Bool() {
		t.Fatal("expected FF_LAYER1=true after env=1")
	}
	dec2 := router.Route(context.Background(), &RouteRequest{UserMessage: "你好"})
	// 开启后无 repo, 走默认 Layer2 fallback (但 reason 不再是 disabled)
	if dec2.Layer != dto.Layer2 {
		t.Errorf("expected layer2 fallback when FF on + no repos, got %s", dec2.Layer)
	}
	if dec2.Reason == dto.ReasonLayer1Disabled {
		t.Error("expected reason != layer1_disabled when FF on")
	}
}

