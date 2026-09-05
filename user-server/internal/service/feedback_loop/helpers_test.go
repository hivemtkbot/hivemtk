package feedbackloop

import (
	"context"
	"math"
	"sync"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func approxEqualF64(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func approxEqualF32(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-6
}

type stubLLMDispatcher struct {
	mu                sync.Mutex
	responses         []string
	model             string
	calls             int
	failOn            int
	err               error
	capturedPrompts   []string
	capturedScenarios []string
}

func newFeedbackLoopStubLLMDispatcher(responses []string) *stubLLMDispatcher {
	return &stubLLMDispatcher{
		responses: responses,
		model:     "stub-llm-v1",
	}
}

func (s *stubLLMDispatcher) Dispatch(ctx context.Context, scenario string, prompt, systemPrompt string, jsonMode bool, maxTokens int) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.capturedPrompts = append(s.capturedPrompts, prompt)
	s.capturedScenarios = append(s.capturedScenarios, scenario)
	if s.failOn > 0 && s.failOn == s.calls {
		return "", "", s.err
	}
	idx := s.calls - 1
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	if idx < 0 {
		return "", s.model, nil
	}
	return s.responses[idx], s.model, nil
}

func (s *stubLLMDispatcher) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type stubEmbedder struct {
	dimension int
	cache     map[string][]float32
	mu        sync.Mutex
}

func newStubEmbedder(dim int) *stubEmbedder {
	if dim <= 0 {
		dim = 8
	}
	return &stubEmbedder{
		dimension: dim,
		cache:     make(map[string][]float32),
	}
}

func (s *stubEmbedder) Embed(text string) []float32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.cache[text]; ok {
		return v
	}
	vec := make([]float32, s.dimension)
	if text == "" {
		s.cache[text] = vec
		return vec
	}
	runes := []rune(text)
	for i, r := range runes {
		idx := (int(r) + i) % s.dimension
		vec[idx] += 1.0
	}

	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum > 0 {
		norm := float32(1.0 / math.Sqrt(sum))
		for i := range vec {
			vec[i] *= norm
		}
	}
	s.cache[text] = vec
	return vec
}

func (s *stubEmbedder) Dimension() int {
	return s.dimension
}

type stubBanditAllocator struct {
	mu             sync.Mutex
	convergenceMap map[string]string
	promoteCalls   []promoteCall
	promoteErr     error
}

type promoteCall struct {
	ExperimentID string
	WinnerKey    string
}

func newStubBanditAllocator() *stubBanditAllocator {
	return &stubBanditAllocator{
		convergenceMap: make(map[string]string),
	}
}

func (s *stubBanditAllocator) CheckConvergence(ctx context.Context, experimentID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	winner, ok := s.convergenceMap[experimentID]
	return winner, ok
}

func (s *stubBanditAllocator) PromoteArm(ctx context.Context, experimentID, winnerKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.promoteCalls = append(s.promoteCalls, promoteCall{
		ExperimentID: experimentID,
		WinnerKey:    winnerKey,
	})
	return s.promoteErr
}

func (s *stubBanditAllocator) SetConverged(experimentID, winnerKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.convergenceMap[experimentID] = winnerKey
}

func (s *stubBanditAllocator) PromoteCalls() []promoteCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]promoteCall, len(s.promoteCalls))
	copy(out, s.promoteCalls)
	return out
}

func setupFeedbackLoopTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.FeedbackSignal{},
		&model.ChampionDialogue{},
		&model.PromptCandidate{},
		&model.BanditArm{},
		&model.PromptABTest{},
		&model.SOPAgent{},
		&model.OptimizationSuggestion{},
	)
}
