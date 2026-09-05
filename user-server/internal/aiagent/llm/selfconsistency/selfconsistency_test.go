package selfconsistency

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type stringSampler struct {
	mu      sync.Mutex
	answers []string
	idx     int
	failAt  int
}

func (s *stringSampler) Sample(_ context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.idx
	if idx >= len(s.answers) {
		idx = len(s.answers) - 1
	}
	s.idx++
	if s.failAt >= 0 && idx == s.failAt {
		return "", errors.New("simulated failure")
	}
	return s.answers[idx], nil
}

type stringVoter struct{}

func (stringVoter) Key(answer string) string {
	return strings.ToLower(strings.TrimSpace(answer))
}

// TestSelfConsistency_BasicMajorityVote 验证基本多数票
func TestSelfConsistency_BasicMajorityVote(t *testing.T) {

	sampler := &stringSampler{
		answers: []string{"Paris", "London", "Paris", "Paris", "London"},
		failAt:  -1,
	}
	sc := NewSelfConsistency[string](5)

	result, err := sc.Run(context.Background(), sampler, stringVoter{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Winner != "Paris" {
		t.Errorf("expected Paris, got %s", result.Winner)
	}
	if result.Count != 3 {
		t.Errorf("expected 3 votes, got %d", result.Count)
	}
	if result.Confidence < 0.5 {
		t.Errorf("expected confidence >= 0.5, got %v", result.Confidence)
	}
}

// TestSelfConsistency_AllAgree 验证全部一致
func TestSelfConsistency_AllAgree(t *testing.T) {
	sampler := &stringSampler{
		answers: []string{"Yes", "Yes", "Yes", "Yes", "Yes"},
		failAt:  -1,
	}
	sc := NewSelfConsistency[string](5)
	result, _ := sc.Run(context.Background(), sampler, stringVoter{})
	if result.Winner != "Yes" {
		t.Errorf("expected Yes, got %s", result.Winner)
	}
	if result.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %v", result.Confidence)
	}
}

// TestSelfConsistency_AllDiffer 验证全部不同
func TestSelfConsistency_AllDiffer(t *testing.T) {
	sampler := &stringSampler{
		answers: []string{"A", "B", "C", "D", "E"},
		failAt:  -1,
	}
	sc := NewSelfConsistency[string](5)
	result, _ := sc.Run(context.Background(), sampler, stringVoter{})

	if result.Count != 1 {
		t.Errorf("expected 1 vote, got %d", result.Count)
	}
	if result.Confidence != 0.2 {
		t.Errorf("expected confidence 0.2, got %v", result.Confidence)
	}
}

// TestSelfConsistency_Concurrent 验证并发安全
func TestSelfConsistency_Concurrent(t *testing.T) {

	sampler := &stringSampler{
		answers: []string{"Same"},
		failAt:  -1,
	}
	sc := NewSelfConsistency[string](5)
	result, _ := sc.Run(context.Background(), sampler, stringVoter{})
	if result.Winner != "Same" {
		t.Errorf("expected Same, got %s", result.Winner)
	}
	if result.Total != 5 {
		t.Errorf("expected 5 total, got %d", result.Total)
	}
}

// TestSelfConsistency_PartialFailure 验证部分采样失败
func TestSelfConsistency_PartialFailure(t *testing.T) {

	sampler := &stringSampler{
		answers: []string{"", "A", "A", "B", "A"},
		failAt:  0,
	}
	sc := NewSelfConsistency[string](5)
	result, _ := sc.Run(context.Background(), sampler, stringVoter{})
	if result.Winner != "A" {
		t.Errorf("expected A, got %s", result.Winner)
	}
	if result.Total != 4 {
		t.Errorf("expected 4 valid, got %d", result.Total)
	}
	if result.Count != 3 {
		t.Errorf("expected A to have 3 votes, got %d", result.Count)
	}
}

// TestSelfConsistency_AllFail 验证全部失败
func TestSelfConsistency_AllFail(t *testing.T) {
	sampler := &stringSampler{answers: []string{"X"}, failAt: 0}
	sc := NewSelfConsistency[string](3)
	_, err := sc.Run(context.Background(), sampler, stringVoter{})
	if err != ErrNoValidSamples {
		t.Errorf("expected ErrNoValidSamples, got %v", err)
	}
}

// TestSelfConsistency_NilSampler 验证 nil 拒绝
func TestSelfConsistency_NilSampler(t *testing.T) {
	sc := NewSelfConsistency[string](3)
	_, err := sc.Run(context.Background(), nil, stringVoter{})
	if err != ErrNilSampler {
		t.Errorf("expected ErrNilSampler, got %v", err)
	}
}

// TestSelfConsistency_NilVoter 验证 nil voter 拒绝
func TestSelfConsistency_NilVoter(t *testing.T) {
	sc := NewSelfConsistency[string](3)
	sampler := &stringSampler{answers: []string{"X"}}
	_, err := sc.Run(context.Background(), sampler, nil)
	if err != ErrNilVoter {
		t.Errorf("expected ErrNilVoter, got %v", err)
	}
}

// TestSelfConsistency_DefaultSamples 验证默认 samples
func TestSelfConsistency_DefaultSamples(t *testing.T) {
	sc := NewSelfConsistency[string](0)
	if sc.samples != 5 {
		t.Errorf("default samples should be 5, got %d", sc.samples)
	}
	sc = NewSelfConsistency[string](-1)
	if sc.samples != 5 {
		t.Errorf("negative samples should default to 5, got %d", sc.samples)
	}
}

// TestSelfConsistency_ContextCancel 验证 ctx 取消
func TestSelfConsistency_ContextCancel(t *testing.T) {
	slowSampler := &slowStringSampler{delay: 100 * time.Millisecond}
	sc := NewSelfConsistency[string](5)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, _ := sc.Run(ctx, slowSampler, stringVoter{})

	_ = result
}

// TestSelfConsistency_AllKeysSorted 验证 AllKeys 排序
func TestSelfConsistency_AllKeysSorted(t *testing.T) {
	sampler := &stringSampler{
		answers: []string{"A", "B", "A", "C", "A"},
		failAt:  -1,
	}
	sc := NewSelfConsistency[string](5)
	result, _ := sc.Run(context.Background(), sampler, stringVoter{})
	if len(result.AllKeys) != 3 {
		t.Errorf("expected 3 distinct keys, got %d", len(result.AllKeys))
	}

	if result.AllKeys[0].Count < result.AllKeys[1].Count {
		t.Error("AllKeys should be sorted by count desc")
	}
}

// TestSelfConsistency_AllKeysContent 验证内容
func TestSelfConsistency_AllKeysContent(t *testing.T) {
	sampler := &stringSampler{
		answers: []string{"X", "Y", "X", "Y", "X"},
		failAt:  -1,
	}
	sc := NewSelfConsistency[string](5)
	result, _ := sc.Run(context.Background(), sampler, stringVoter{})

	counts := make(map[string]int)
	for _, kc := range result.AllKeys {
		counts[kc.Key] = kc.Count
	}
	if counts["x"] != 3 || counts["y"] != 2 {
		t.Errorf("expected x=3, y=2, got %v", counts)
	}
}

// TestSelfConsistency_Normalization 验证大小写无关
func TestSelfConsistency_Normalization(t *testing.T) {
	sampler := &stringSampler{
		answers: []string{"PARIS", "paris", "Paris", "PARIS", "paris"},
		failAt:  -1,
	}
	sc := NewSelfConsistency[string](5)
	result, _ := sc.Run(context.Background(), sampler, stringVoter{})
	if result.Winner != "paris" {
		t.Errorf("expected normalized winner 'paris', got %q", result.Winner)
	}
	if result.Count != 5 {
		t.Errorf("expected 5 votes (all same after norm), got %d", result.Count)
	}
}

type intVoter struct{}

func (intVoter) Key(answer int) string {
	if answer%2 == 0 {
		return "even"
	}
	return "odd"
}

type intSampler struct {
	mu      sync.Mutex
	answers []int
	idx     int
	failAt  int
}

func (s *intSampler) Sample(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.idx
	if idx >= len(s.answers) {
		idx = len(s.answers) - 1
	}
	s.idx++
	if s.failAt >= 0 && idx == s.failAt {
		return 0, errors.New("simulated failure")
	}
	return s.answers[idx], nil
}

// TestSelfConsistency_GenericType 验证泛型
func TestSelfConsistency_GenericType(t *testing.T) {
	sampler := &intSampler{
		answers: []int{2, 4, 6, 1, 3},
		idx:     0,
		failAt:  -1,
	}
	sc := NewSelfConsistency[int](5)
	result, _ := sc.Run(context.Background(), sampler, intVoter{})
	if result.WinnerKey != "even" {
		t.Errorf("expected 'even' (3 votes for 2,4,6), got %q", result.WinnerKey)
	}
	if result.Count != 3 {
		t.Errorf("expected 3, got %d", result.Count)
	}
}

type slowStringSampler struct {
	delay time.Duration
	mu    sync.Mutex
	calls int32
}

func (s *slowStringSampler) Sample(ctx context.Context) (string, error) {
	s.mu.Lock()
	_ = s.calls
	s.calls++
	s.mu.Unlock()
	select {
	case <-time.After(s.delay):
		return "slow", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// TestSelfConsistency_AllSampleCancel 验证全部取消
func TestSelfConsistency_AllSampleCancel(t *testing.T) {
	slow := &slowStringSampler{delay: 1 * time.Second}
	sc := NewSelfConsistency[string](3)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := sc.Run(ctx, slow, stringVoter{})
	if err != ErrNoValidSamples {
		t.Errorf("expected ErrNoValidSamples, got %v", err)
	}
}
