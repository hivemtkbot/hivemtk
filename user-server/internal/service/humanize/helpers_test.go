package humanize

// helpers_test.go 测试通用辅助函数
//
// 提供浮点近似比较、stub LLM 调度器、stub 仓储等测试基础设施

import (
	"context"
	"math"
	"sync"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

// approxEqual 浮点近似相等（绝对误差 ≤ 1e-6）
func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

// approxEqualTol 带容差的浮点近似相等
func approxEqualTol(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

// approxSlice 切片近似相等
func approxSlice(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !approxEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// ============================================================================
// stubLLMDispatcher 测试用 LLM 调度器
// ============================================================================

// stubLLMDispatcher 测试用 LLM 调度器
type stubLLMDispatcher struct {
	mu              sync.Mutex
	responses       []string // 预置响应队列
	model           string
	calls           int
	failOn          int   // 第 N 次调用失败（0=不失败）
	err             error // 失败时返回的错误
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

// ============================================================================
// stubScoreRepo 测试用评分仓储
// ============================================================================

// stubScoreRepo 测试用评分仓储
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

// Save 实现 humanize.HumanizeScoreRepository 接口
// 五层架构：repository 仅持久化 model 类型，dto→model 转换在 service 层完成
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

// ============================================================================
// stubBaselineRepo 测试用销冠基线仓储
// ============================================================================

// stubBaselineRepo 测试用销冠基线仓储
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

// ============================================================================
// stubSampleCollector 测试用低质样本收集器
// ============================================================================

// stubSampleCollector 测试用低质样本收集器
type stubSampleCollector struct {
	mu         sync.Mutex
	collected  int
	lastSample *model.LowQualitySample
	collectErr error
}

func newStubSampleCollector() *stubSampleCollector {
	return &stubSampleCollector{}
}

// Collect 实现 humanize.LowQualitySampleCollector 接口
// 五层架构：service 层预构建 model.LowQualitySample，repository 仅负责持久化
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

// 编译时接口断言
var (
	_ HumanizeScoreRepository    = (*stubScoreRepo)(nil)
	_ ChampionBaselineRepository = (*stubBaselineRepo)(nil)
	_ LowQualitySampleCollector  = (*stubSampleCollector)(nil)
	_ LLMDispatcher              = (*stubLLMDispatcher)(nil)
	_ HumanizeEvaluator          = (*stubEvaluator)(nil)
)

// ============================================================================
// stubEvaluator 测试用 HumanizeEvaluator
// ============================================================================

// stubEvaluator 测试用评估器：按调用顺序返回预设结果
//
// 用法：
//   - results: 按调用顺序依次返回（每次返回副本，避免被修改）
//   - err: 非 nil 时所有调用都报错
//   - calls: 累计调用次数
type stubEvaluator struct {
	mu      sync.Mutex
	results []*dto.HumanizeEvalResult
	err     error
	calls   int
	// capturedInputs 记录每次调用收到的 input（便于校验 regenerate 后 AIReply 变化）
	capturedInputs []*dto.HumanizeEvalInput
}

func newStubEvaluator(results ...*dto.HumanizeEvalResult) *stubEvaluator {
	return &stubEvaluator{results: results}
}

// Evaluate 实现 HumanizeEvaluator 接口
func (s *stubEvaluator) Evaluate(ctx context.Context, input *dto.HumanizeEvalInput) (*dto.HumanizeEvalResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if input != nil {
		// 记录副本
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
