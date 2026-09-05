package humanize

import (
	"context"
	"math"
	"sync"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func approxEqualTol(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

type stubLLMDispatcher struct {
	mu              sync.Mutex
	responses       []string
	model           string
	calls           int
	failOn          int
	err             error
	capturedPrompts []string
}

func newHumanizeStubLLMDispatcher(responses []string) *stubLLMDispatcher {
	return &stubLLMDispatcher{
		responses: responses,
		model:     "stub-llm-v1",
	}
}

func (s *stubLLMDispatcher) ChatSend(ctx context.Context, prompt string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.capturedPrompts = append(s.capturedPrompts, prompt)
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

type stubScoreRepo struct {
	mu             sync.Mutex
	saved          int
	lastScore      *model.HumanizeScore
	lastDimensions []model.HumanizeDimensionRecord
	saveErr        error
}

func newStubScoreRepo() *stubScoreRepo {
	return &stubScoreRepo{}
}

func (r *stubScoreRepo) Save(ctx context.Context, score *model.HumanizeScore, dimensions []model.HumanizeDimensionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved++
	r.lastScore = score
	r.lastDimensions = dimensions
	return nil
}

type stubBaselineRepo struct {
	mu         sync.Mutex
	baseline   *model.ChampionBaseline
	findErr    error
	saved      []model.ChampionBaseline
	refreshErr error
	refreshed  int
}

func newStubBaselineRepo(baseline *model.ChampionBaseline) *stubBaselineRepo {
	return &stubBaselineRepo{baseline: baseline}
}

func (r *stubBaselineRepo) FindByPersonaIndustryIntent(ctx context.Context, persona, industry, intent string) (*model.ChampionBaseline, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.findErr != nil {
		return nil, r.findErr
	}
	return r.baseline, nil
}

func (r *stubBaselineRepo) Save(ctx context.Context, b *model.ChampionBaseline) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b == nil {
		return 0, nil
	}
	r.saved = append(r.saved, *b)
	return uint64(len(r.saved)), nil
}

func (r *stubBaselineRepo) ListEnabled(ctx context.Context) ([]model.ChampionBaseline, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.baseline == nil {
		return nil, nil
	}
	return []model.ChampionBaseline{*r.baseline}, nil
}

func (r *stubBaselineRepo) RefreshPhrases(ctx context.Context, baselineID uint64, phrases []model.ChampionPhrase) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshed++
	if r.refreshErr != nil {
		return r.refreshErr
	}
	return nil
}

type stubSampleCollector struct {
	mu         sync.Mutex
	collected  int
	lastSample *model.LowQualitySample
	collectErr error
}

func newStubSampleCollector() *stubSampleCollector {
	return &stubSampleCollector{}
}

func (c *stubSampleCollector) Collect(ctx context.Context, sample *model.LowQualitySample) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.collectErr != nil {
		return c.collectErr
	}
	c.collected++
	c.lastSample = sample
	return nil
}

var (
	_ HumanizeScoreRepository    = (*stubScoreRepo)(nil)
	_ ChampionBaselineRepository = (*stubBaselineRepo)(nil)
	_ LowQualitySampleCollector  = (*stubSampleCollector)(nil)
	_ LLMDispatcher              = (*stubLLMDispatcher)(nil)
	_ HumanizeEvaluator          = (*stubEvaluator)(nil)
)

type stubEvaluator struct {
	mu             sync.Mutex
	results        []*dto.HumanizeEvalResult
	err            error
	calls          int
	capturedInputs []*dto.HumanizeEvalInput
}

func newStubEvaluator(results ...*dto.HumanizeEvalResult) *stubEvaluator {
	return &stubEvaluator{results: results}
}

func (s *stubEvaluator) Evaluate(ctx context.Context, input *dto.HumanizeEvalInput) (*dto.HumanizeEvalResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if input != nil {
		cp := *input
		s.capturedInputs = append(s.capturedInputs, &cp)
	} else {
		s.capturedInputs = append(s.capturedInputs, nil)
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.calls <= len(s.results) && s.results[s.calls-1] != nil {
		r := *s.results[s.calls-1]
		return &r, nil
	}
	return nil, nil
}
