package feedbackloop

// helpers_test.go 反馈学习闭环测试通用辅助
//
// 提供：
//   1. stubLLMDispatcher   - LLM 调度器 stub（实现 LLMDispatcher 接口）
//   2. stubEmbedder        - Embedder stub（实现 Embedder 接口）
//   3. stubBanditAllocator  - BanditAllocator stub（实现 BanditAllocatorInterface）
// 4. PG 测试 DB setup - 复用 testutil.NewTestDB，自动 AutoMigrate 全部模型
//   5. 浮点近似比较辅助

import (
	"context"
	"math"
	"sync"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// ----------------------------------------------------------------------------
// 浮点近似比较
// ----------------------------------------------------------------------------

func approxEqualF64(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func approxEqualF32(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-6
}

// ----------------------------------------------------------------------------
// stubLLMDispatcher LLM 调度器 stub
// ----------------------------------------------------------------------------

// stubLLMDispatcher 测试用 LLM 调度器
//
// 行为：
//   - responses 队列按调用顺序返回；耗尽后返回最后一条
//   - failOn > 0 时第 N 次调用返回 err
//   - 捕获所有调用的 prompt 供断言
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

// Calls 返回调用次数
func (s *stubLLMDispatcher) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// ----------------------------------------------------------------------------
// stubEmbedder Embedder stub
// ----------------------------------------------------------------------------

// stubEmbedder 测试用 Embedder
//
// 行为：
//   - dimension 默认 8（测试用小维度，避免 1024 维开销）
//   - 相同文本返回相同向量（基于简单哈希）
//   - 不同文本返回不同向量（保证可聚类）
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

// Embed 实现 Embedder 接口
//
// 哈希策略：每个字符 ASCII 值累加后取模，生成 dim 维向量
// 同文本 → 同向量；相似文本 → 高余弦相似度
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
	// L2 归一化
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

// ----------------------------------------------------------------------------
// stubBanditAllocator BanditAllocator stub
// ----------------------------------------------------------------------------

// stubBanditAllocator 测试用 BanditAllocator
//
// 用于 SOPAutoOptimizer 测试，可控返回收敛结果与 PromoteArm 行为
type stubBanditAllocator struct {
	mu             sync.Mutex
	convergenceMap map[string]string // experimentID → winnerKey（空表示未收敛）
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

// SetConverged 设置实验为已收敛
func (s *stubBanditAllocator) SetConverged(experimentID, winnerKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.convergenceMap[experimentID] = winnerKey
}

// PromoteCalls 返回 PromoteArm 调用历史
func (s *stubBanditAllocator) PromoteCalls() []promoteCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]promoteCall, len(s.promoteCalls))
	copy(out, s.promoteCalls)
	return out
}

// ----------------------------------------------------------------------------
// PG 测试 DB setup
// ----------------------------------------------------------------------------

// setupFeedbackLoopTestDB 创建 测试 DB
//
// 行为：
//   - 调用 testutil.NewTestDB 创建独立 PG 库（项目规则"不允许跳过"，连接失败时 t.Fatal）
//
// AutoMigrate 全部 6 张新表 + 关联表（sop_agents / optimization_suggestions）
//   - testutil 已自动启用 pgvector 扩展（champion_dialogues.embedding 字段必需）
//   - 注：champion_dialogues.embedding 的 GORM tag 为 type:vector(1024)，
//     AutoMigrate 会创建为 vector(1024) 类型（pgvector 扩展已启用）
func setupFeedbackLoopTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 项目规则"不允许跳过"：PG 集成测试必须运行，testutil.NewTestDB 在连接失败时 t.Fatal
	return testutil.NewTestDB(t,
		// 6 张新表
		&model.FeedbackEvent{},
		&model.FeedbackSignal{},
		&model.ChampionDialogue{},
		&model.PromptCandidate{},
		&model.BanditArm{},
		&model.PromptABTest{},
		// 关联表（用于 SOPAutoOptimizer 测试）
		&model.SOPAgent{},
		&model.OptimizationSuggestion{},
	)
}
