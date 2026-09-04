package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/aiagent/llm"
)

// D10 mock：窄接口注入
type mockObjectionDispatcher struct {
	content string
	err     error
	calls   int
}

func (m *mockObjectionDispatcher) Dispatch(_ context.Context, _ llm.DispatchRequest) (*llm.DispatchResult, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return &llm.DispatchResult{Content: m.content}, nil
}

// D10: 低置信单命中 → LLM 兜底覆盖类别；is_genuine=false 归 Other
func TestD08_LLMFallbackOverridesLowConfidence(t *testing.T) {
	s := NewObjectionHandlerService()
	md := &mockObjectionDispatcher{content: `{"category":"timing","confidence":0.8,"is_genuine":false}`}
	s.dispatcher = md

	resp, err := s.Handle(context.Background(), HandleRequest{Text: "再看看"})
	if err != nil {
		t.Fatal(err)
	}
	if md.calls != 1 {
		t.Fatalf("单命中应调 LLM 1 次, got %d", md.calls)
	}
	if resp.Category != ObjectionOther {
		t.Errorf("is_genuine=false 应归 Other, got %s", resp.Category)
	}
	if resp.IsGenuine == nil || *resp.IsGenuine {
		t.Errorf("IsGenuine 应为 false, got %v", resp.IsGenuine)
	}
	if resp.LLMCategory != ObjectionTiming {
		t.Errorf("LLMCategory 应保留原分类 timing, got %s", resp.LLMCategory)
	}
	if resp.Source != "llm" {
		t.Errorf("source 应为 llm, got %s", resp.Source)
	}
}

// D10: 多词命中 0.90 首中即返语义不变——不调 LLM
func TestD08_MultiHitSkipsLLM(t *testing.T) {
	s := NewObjectionHandlerService()
	md := &mockObjectionDispatcher{}
	s.dispatcher = md

	resp, err := s.Handle(context.Background(), HandleRequest{Text: "太贵了 便宜点"})
	if err != nil {
		t.Fatal(err)
	}
	if md.calls != 0 {
		t.Errorf("多词命中不应调 LLM, got %d 次", md.calls)
	}
	if resp.Category != ObjectionPrice || resp.Source != "rule" {
		t.Errorf("应保持规则首中结果 price, got %s/%s", resp.Category, resp.Source)
	}
}

// D10: dispatcher=nil 完全回退纯规则（零行为变化）
func TestD08_NilDispatcherRuleOnly(t *testing.T) {
	s := NewObjectionHandlerService()
	s.dispatcher = nil
	resp, err := s.Handle(context.Background(), HandleRequest{Text: "太贵了 便宜点"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Source != "rule" || resp.IsGenuine != nil {
		t.Errorf("nil dispatcher 应纯规则无 LLM 字段, got %+v", resp)
	}
}

// D10: LLM 失败 → 规则结果兜底
func TestD08_LLMErrorFallsBack(t *testing.T) {
	s := NewObjectionHandlerService()
	md := &mockObjectionDispatcher{err: context.DeadlineExceeded}
	s.dispatcher = md

	resp, err := s.Handle(context.Background(), HandleRequest{Text: "考虑一下"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Category != ObjectionTiming || resp.Source != "rule" {
		t.Errorf("LLM 失败应回退规则 timing, got %s/%s", resp.Category, resp.Source)
	}
	if resp.IsGenuine != nil {
		t.Errorf("失败路径不应有 IsGenuine, got %v", resp.IsGenuine)
	}
}

// D10: LLM 正常真异议 → 覆盖
func TestD08_LLMGenuineOverride(t *testing.T) {
	s := NewObjectionHandlerService()
	md := &mockObjectionDispatcher{content: `{"category":"trust","confidence":0.85,"is_genuine":true}`}
	s.dispatcher = md

	resp, err := s.Handle(context.Background(), HandleRequest{Text: "有点不靠谱"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Category != ObjectionTrust || resp.Source != "llm" {
		t.Errorf("真异议应覆盖为 trust/llm, got %s/%s", resp.Category, resp.Source)
	}
	if resp.IsGenuine == nil || !*resp.IsGenuine {
		t.Errorf("IsGenuine 应为 true, got %v", resp.IsGenuine)
	}
}
