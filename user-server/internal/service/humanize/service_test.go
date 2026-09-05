package humanize

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

func newServiceWithStubs(
	rule, llm HumanizeEvaluator,
	baselineRepo ChampionBaselineRepository,
	scoreRepo HumanizeScoreRepository,
	sampleCollector LowQualitySampleCollector,
) *HumanizeEvalService {
	svc := NewHumanizeEvalService(rule, llm, baselineRepo, scoreRepo, sampleCollector)
	svc.rng = rand.New(rand.NewSource(1))
	return svc
}

func makeScores(nat, con, emp, pro, per float64) []dto.HumanizeDimensionScore {
	return []dto.HumanizeDimensionScore{
		{Dimension: dto.HumanizeDimNaturalness, Score: nat},
		{Dimension: dto.HumanizeDimConciseness, Score: con},
		{Dimension: dto.HumanizeDimEmpathy, Score: emp},
		{Dimension: dto.HumanizeDimProfessionalism, Score: pro},
		{Dimension: dto.HumanizeDimPersuasiveness, Score: per},
	}
}

func makeResult(total float64) *dto.HumanizeEvalResult {
	return &dto.HumanizeEvalResult{
		Scores:     makeScores(0.85, 0.85, 0.85, 0.85, 0.85),
		TotalScore: total,
	}
}

// TestHumanizeEvalService_Evaluate_RuleOnlyPass 规则评估直接通过
func TestHumanizeEvalService_Evaluate_RuleOnlyPass(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.90))
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, nil, nil, repo, nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "这款产品成分是烟酰胺，保湿效果很好，下单试试吧。",
		CustomerMessage: "产品怎么样？",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if !result.Passed {
		t.Errorf("Passed=false want true (TotalScore=%v)", result.TotalScore)
	}
	if result.TotalScore < 0.85 {
		t.Errorf("TotalScore=%v 应 ≥ 0.85", result.TotalScore)
	}
	if result.SampleStrategy != "full" {
		t.Errorf("SampleStrategy=%q want full", result.SampleStrategy)
	}
	if rule.calls != 1 {
		t.Errorf("rule calls=%d want 1（不应触发 LLM）", rule.calls)
	}
	if repo.saved != 1 {
		t.Errorf("persist saved=%d want 1", repo.saved)
	}
}

// TestHumanizeEvalService_Evaluate_LLM_BoundaryTrigger 边界样本触发 LLM
func TestHumanizeEvalService_Evaluate_LLM_BoundaryTrigger(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.78))
	llm := newStubEvaluator(makeResult(0.88))
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, llm, nil, repo, nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试回复",
		CustomerMessage: "测试问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if !result.Passed {
		t.Errorf("Passed=false want true")
	}
	if result.SampleStrategy != "boundary" {
		t.Errorf("SampleStrategy=%q want boundary", result.SampleStrategy)
	}
	if result.EvaluatorType != "hybrid" {
		t.Errorf("EvaluatorType=%q want hybrid", result.EvaluatorType)
	}
	if llm.calls != 1 {
		t.Errorf("llm calls=%d want 1（边界样本应触发 LLM）", llm.calls)
	}
}

// TestHumanizeEvalService_Evaluate_LLM_SampledTrigger 低分样本按 sampleRate=1.0 采样触发
func TestHumanizeEvalService_Evaluate_LLM_SampledTrigger(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.65))
	llm := newStubEvaluator(makeResult(0.90))
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, llm, nil, repo, nil)
	svc.WithSampleRate(context.Background(), 1.0)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if !result.Passed {
		t.Errorf("Passed=false want true")
	}
	if result.SampleStrategy != "sampled" {
		t.Errorf("SampleStrategy=%q want sampled", result.SampleStrategy)
	}
	if llm.calls != 1 {
		t.Errorf("llm calls=%d want 1（sampleRate=1.0 应触发）", llm.calls)
	}
}

// TestHumanizeEvalService_Evaluate_LLM_NotSampled 低分样本 sampleRate=0 不触发
func TestHumanizeEvalService_Evaluate_LLM_NotSampled(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.65))
	llm := newStubEvaluator(makeResult(0.90))
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, llm, nil, repo, nil)
	svc.WithSampleRate(context.Background(), 0.0)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result.Passed {
		t.Errorf("Passed=true want false (0.65 < 0.85)")
	}
	if llm.calls != 0 {
		t.Errorf("llm calls=%d want 0（sampleRate=0）", llm.calls)
	}
}

// TestHumanizeEvalService_Evaluate_LLMFail_DegradeToRule LLM 失败降级
func TestHumanizeEvalService_Evaluate_LLMFail_DegradeToRule(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.78))
	llm := newStubEvaluator()
	llm.err = errors.New("LLM service unavailable")
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, llm, nil, repo, nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Passed {
		t.Errorf("Passed=true want false (规则 0.78 < 0.85)")
	}
	if result.TotalScore != 0.78 {
		t.Errorf("TotalScore=%v want 0.78（降级到规则）", result.TotalScore)
	}
	if llm.calls != 1 {
		t.Errorf("llm calls=%d want 1（边界触发但失败）", llm.calls)
	}
}

// TestHumanizeEvalService_Evaluate_RetryPass 重生成后通过
func TestHumanizeEvalService_Evaluate_RetryPass(t *testing.T) {
	rule := newStubEvaluator(
		makeResult(0.80),
		makeResult(0.82),
		makeResult(0.90),
	)
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, nil, nil, repo, nil)
	svc.WithRegenerateFn(context.Background(), func(ctx context.Context, input *dto.HumanizeEvalInput, feedback *dto.HumanizeEvalResult) (string, error) {
		return "regenerated reply", nil
	})
	input := &dto.HumanizeEvalInput{
		AIReply:         "original reply",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if !result.Passed {
		t.Errorf("Passed=false want true（第 3 次应通过）")
	}
	if result.TotalScore != 0.90 {
		t.Errorf("TotalScore=%v want 0.90", result.TotalScore)
	}
	if result.AttemptCount != 3 {
		t.Errorf("AttemptCount=%d want 3", result.AttemptCount)
	}
	if len(result.AllReplies) != 3 {
		t.Errorf("AllReplies len=%d want 3", len(result.AllReplies))
	}
	if rule.calls != 3 {
		t.Errorf("rule calls=%d want 3", rule.calls)
	}
}

// TestHumanizeEvalService_Evaluate_RetryFail 重生成 3 次仍不达标 → 转人工
func TestHumanizeEvalService_Evaluate_RetryFail(t *testing.T) {
	rule := newStubEvaluator(
		makeResult(0.80),
		makeResult(0.82),
		makeResult(0.83),
	)
	repo := newStubScoreRepo()
	collector := newStubSampleCollector()
	svc := newServiceWithStubs(rule, nil, nil, repo, collector)
	svc.WithRegenerateFn(context.Background(), func(ctx context.Context, input *dto.HumanizeEvalInput, feedback *dto.HumanizeEvalResult) (string, error) {
		return "regenerated reply", nil
	})
	input := &dto.HumanizeEvalInput{
		AIReply:         "original reply",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result.Passed {
		t.Errorf("Passed=true want false（3 次都不达标）")
	}
	if result.AttemptCount != 3 {
		t.Errorf("AttemptCount=%d want 3", result.AttemptCount)
	}
	if collector.collected != 1 {
		t.Errorf("collector.collected=%d want 1", collector.collected)
	}
}

// TestHumanizeEvalService_Evaluate_RetryNoRegenerateFn 无重生成回调 → 直接转人工
func TestHumanizeEvalService_Evaluate_RetryNoRegenerateFn(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.80))
	repo := newStubScoreRepo()
	collector := newStubSampleCollector()
	svc := newServiceWithStubs(rule, nil, nil, repo, collector)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result.Passed {
		t.Errorf("Passed=true want false")
	}
	if collector.collected != 1 {
		t.Errorf("collector.collected=%d want 1（应收集低质样本）", collector.collected)
	}
}

// TestHumanizeEvalService_Evaluate_RegenerateFail 重生成回调失败 → 转人工
func TestHumanizeEvalService_Evaluate_RegenerateFail(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.80))
	repo := newStubScoreRepo()
	collector := newStubSampleCollector()
	svc := newServiceWithStubs(rule, nil, nil, repo, collector)
	svc.WithRegenerateFn(context.Background(), func(ctx context.Context, input *dto.HumanizeEvalInput, feedback *dto.HumanizeEvalResult) (string, error) {
		return "", errors.New("regenerate failed")
	})
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result.Passed {
		t.Errorf("Passed=true want false")
	}
	if collector.collected != 1 {
		t.Errorf("collector.collected=%d want 1", collector.collected)
	}
}

// TestHumanizeEvalService_Evaluate_LLM_RetryAfterLLMFail LLM 不达标后重生成
func TestHumanizeEvalService_Evaluate_LLM_RetryAfterLLMFail(t *testing.T) {
	rule := newStubEvaluator(
		makeResult(0.78),
		makeResult(0.92),
	)
	llm := newStubEvaluator(makeResult(0.80))
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, llm, nil, repo, nil)
	svc.WithRegenerateFn(context.Background(), func(ctx context.Context, input *dto.HumanizeEvalInput, feedback *dto.HumanizeEvalResult) (string, error) {
		return "better reply", nil
	})
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if !result.Passed {
		t.Errorf("Passed=false want true")
	}
	if result.TotalScore != 0.92 {
		t.Errorf("TotalScore=%v want 0.92", result.TotalScore)
	}
}

// TestHumanizeEvalService_Evaluate_NilInput nil 输入
func TestHumanizeEvalService_Evaluate_NilInput(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.90))
	svc := newServiceWithStubs(rule, nil, nil, newStubScoreRepo(), nil)
	_, err := svc.Evaluate(context.Background(), nil)
	if err == nil {
		t.Error("nil input 应报错")
	}
}

// TestHumanizeEvalService_Evaluate_EmptyReply 空 AIReply
func TestHumanizeEvalService_Evaluate_EmptyReply(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.90))
	svc := newServiceWithStubs(rule, nil, nil, newStubScoreRepo(), nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "",
		CustomerMessage: "问题",
	}
	_, err := svc.Evaluate(context.Background(), input)
	if err == nil {
		t.Error("空 AIReply 应报错")
	}
}

// TestHumanizeEvalService_Evaluate_NilRuleScorer nil ruleScorer
func TestHumanizeEvalService_Evaluate_NilRuleScorer(t *testing.T) {
	svc := newServiceWithStubs(nil, nil, nil, newStubScoreRepo(), nil)
	input := &dto.HumanizeEvalInput{AIReply: "测试"}
	_, err := svc.Evaluate(context.Background(), input)
	if err == nil {
		t.Error("nil ruleScorer 应报错")
	}
}

// TestHumanizeEvalService_Evaluate_RuleScorerError RuleScorer 报错
func TestHumanizeEvalService_Evaluate_RuleScorerError(t *testing.T) {
	rule := newStubEvaluator()
	rule.err = errors.New("rule scorer internal error")
	svc := newServiceWithStubs(rule, nil, nil, newStubScoreRepo(), nil)
	input := &dto.HumanizeEvalInput{AIReply: "测试"}
	_, err := svc.Evaluate(context.Background(), input)
	if err == nil {
		t.Error("RuleScorer 报错应向上传播")
	}
}

// TestDecideLowQualitySampleType_NaturalnessLow naturalness 最低
func TestDecideLowQualitySampleType_NaturalnessLow(t *testing.T) {
	svc := &HumanizeEvalService{}
	result := &dto.HumanizeEvalResult{
		Scores: []dto.HumanizeDimensionScore{
			{Dimension: dto.HumanizeDimNaturalness, Score: 0.40},
			{Dimension: dto.HumanizeDimConciseness, Score: 0.80},
			{Dimension: dto.HumanizeDimEmpathy, Score: 0.80},
			{Dimension: dto.HumanizeDimProfessionalism, Score: 0.80},
			{Dimension: dto.HumanizeDimPersuasiveness, Score: 0.80},
		},
	}
	got := svc.decideLowQualitySampleType(context.Background(), result)
	if got != "naturalness_low" {
		t.Errorf("type=%q want naturalness_low", got)
	}
}

// TestDecideLowQualitySampleType_PersuasivenessLow persuasiveness 最低
func TestDecideLowQualitySampleType_PersuasivenessLow(t *testing.T) {
	svc := &HumanizeEvalService{}
	result := &dto.HumanizeEvalResult{
		Scores: []dto.HumanizeDimensionScore{
			{Dimension: dto.HumanizeDimNaturalness, Score: 0.80},
			{Dimension: dto.HumanizeDimConciseness, Score: 0.80},
			{Dimension: dto.HumanizeDimEmpathy, Score: 0.80},
			{Dimension: dto.HumanizeDimProfessionalism, Score: 0.80},
			{Dimension: dto.HumanizeDimPersuasiveness, Score: 0.30},
		},
	}
	got := svc.decideLowQualitySampleType(context.Background(), result)
	if got != "persuasiveness_low" {
		t.Errorf("type=%q want persuasiveness_low", got)
	}
}

// TestDecideLowQualitySampleType_OtherDim 其他维度最低 → retry_exhausted
func TestDecideLowQualitySampleType_OtherDim(t *testing.T) {
	svc := &HumanizeEvalService{}
	result := &dto.HumanizeEvalResult{
		Scores: []dto.HumanizeDimensionScore{
			{Dimension: dto.HumanizeDimNaturalness, Score: 0.80},
			{Dimension: dto.HumanizeDimConciseness, Score: 0.30},
			{Dimension: dto.HumanizeDimEmpathy, Score: 0.80},
			{Dimension: dto.HumanizeDimProfessionalism, Score: 0.80},
			{Dimension: dto.HumanizeDimPersuasiveness, Score: 0.80},
		},
	}
	got := svc.decideLowQualitySampleType(context.Background(), result)
	if got != "retry_exhausted" {
		t.Errorf("type=%q want retry_exhausted", got)
	}
}

// TestDecideLowQualitySampleType_NilResult nil 结果
func TestDecideLowQualitySampleType_NilResult(t *testing.T) {
	svc := &HumanizeEvalService{}
	got := svc.decideLowQualitySampleType(context.Background(), nil)
	if got != "retry_exhausted" {
		t.Errorf("type=%q want retry_exhausted", got)
	}
}

// TestDecideLowQualitySampleType_EmptyScores 空 scores
func TestDecideLowQualitySampleType_EmptyScores(t *testing.T) {
	svc := &HumanizeEvalService{}
	result := &dto.HumanizeEvalResult{Scores: nil}
	got := svc.decideLowQualitySampleType(context.Background(), result)
	if got != "retry_exhausted" {
		t.Errorf("type=%q want retry_exhausted", got)
	}
}

// TestHumanizeEvalService_Chaining 链式配置方法
func TestHumanizeEvalService_Chaining(t *testing.T) {
	svc := NewHumanizeEvalService(nil, nil, nil, nil, nil)
	svc.WithThreshold(context.Background(), 0.90).
		WithSampleRate(context.Background(), 0.20).
		WithBoundary(context.Background(), 0.65, 0.80).
		WithMaxRetry(context.Background(), 5)
	if svc.threshold != 0.90 {
		t.Errorf("threshold=%v want 0.90", svc.threshold)
	}
	if svc.sampleRate != 0.20 {
		t.Errorf("sampleRate=%v want 0.20", svc.sampleRate)
	}
	if svc.boundaryLow != 0.65 || svc.boundaryHigh != 0.80 {
		t.Errorf("boundary=[%v,%v] want [0.65,0.80]", svc.boundaryLow, svc.boundaryHigh)
	}
	if svc.maxRetry != 5 {
		t.Errorf("maxRetry=%d want 5", svc.maxRetry)
	}
}

// TestHumanizeEvalService_Chaining_Invalid 无效值不修改
func TestHumanizeEvalService_Chaining_Invalid(t *testing.T) {
	svc := NewHumanizeEvalService(nil, nil, nil, nil, nil)
	origThreshold := svc.threshold
	origSampleRate := svc.sampleRate
	origBoundaryLow := svc.boundaryLow
	origBoundaryHigh := svc.boundaryHigh
	origMaxRetry := svc.maxRetry
	svc.WithThreshold(context.Background(), 0).WithThreshold(context.Background(), -0.1).WithThreshold(context.Background(), 1.5)
	svc.WithSampleRate(context.Background(), -0.1).WithSampleRate(context.Background(), 1.5)
	svc.WithBoundary(context.Background(), -0.1, 0.5).WithBoundary(context.Background(), 0.5, 0.5).WithBoundary(context.Background(), 0.5, 1.5)
	svc.WithMaxRetry(context.Background(), 0).WithMaxRetry(context.Background(), -1)
	if svc.threshold != origThreshold {
		t.Errorf("invalid threshold 不应修改: %v want %v", svc.threshold, origThreshold)
	}
	if svc.sampleRate != origSampleRate {
		t.Errorf("invalid sampleRate 不应修改: %v want %v", svc.sampleRate, origSampleRate)
	}
	if svc.boundaryLow != origBoundaryLow {
		t.Errorf("invalid boundaryLow 不应修改: %v want %v", svc.boundaryLow, origBoundaryLow)
	}
	if svc.boundaryHigh != origBoundaryHigh {
		t.Errorf("invalid boundaryHigh 不应修改: %v want %v", svc.boundaryHigh, origBoundaryHigh)
	}
	if svc.maxRetry != origMaxRetry {
		t.Errorf("invalid maxRetry 不应修改: %d want %d", svc.maxRetry, origMaxRetry)
	}
}

// TestHumanizeEvalService_WithRegenerateFn 设置重生成回调
func TestHumanizeEvalService_WithRegenerateFn(t *testing.T) {
	svc := NewHumanizeEvalService(nil, nil, nil, nil, nil)
	if svc.regenerateFn != nil {
		t.Error("初始 regenerateFn 应为 nil")
	}
	fn := func(ctx context.Context, input *dto.HumanizeEvalInput, feedback *dto.HumanizeEvalResult) (string, error) {
		return "test", nil
	}
	svc.WithRegenerateFn(context.Background(), fn)
	if svc.regenerateFn == nil {
		t.Error("WithRegenerateFn 后不应为 nil")
	}
}

// TestHumanizeEvalService_PersistCalled 评估通过后应调用持久化
func TestHumanizeEvalService_PersistCalled(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.90))
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, nil, nil, repo, nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	_, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if repo.saved < 1 {
		t.Errorf("saved=%d want ≥ 1", repo.saved)
	}
	if repo.lastScore == nil {
		t.Error("lastScore 不应为 nil")
	}
}

// TestHumanizeEvalService_PersistError 持久化失败不应影响主流程
func TestHumanizeEvalService_PersistError(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.90))
	repo := newStubScoreRepo()
	repo.saveErr = errors.New("db connection lost")
	svc := newServiceWithStubs(rule, nil, nil, repo, nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("持久化失败不应导致 Evaluate 报错: %v", err)
	}
	if result == nil {
		t.Fatal("result 不应为 nil")
	}
	if !result.Passed {
		t.Errorf("Passed=false want true（持久化失败不影响业务）")
	}
}

// TestHumanizeEvalService_NilScoreRepo nil scoreRepo 不应 panic
func TestHumanizeEvalService_NilScoreRepo(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.90))
	svc := newServiceWithStubs(rule, nil, nil, nil, nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result == nil {
		t.Fatal("result 不应为 nil")
	}
}

// TestHumanizeEvalService_Evaluate_PersistsEffectiveThreshold 落库阈值必须等于生效阈值
//
// 回归：buildScoreFromResult / buildLowQualitySample 曾硬编码 DefaultThreshold(0.85)，
// 一旦 WithThreshold 定制阈值，落库的 humanize_scores.threshold 与真实判定阈值脱节（数据失真）。
func TestHumanizeEvalService_Evaluate_PersistsEffectiveThreshold(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.92))
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, nil, nil, repo, nil)
	svc.WithThreshold(context.Background(), 0.50)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	if _, err := svc.Evaluate(context.Background(), input); err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if repo.lastScore == nil {
		t.Fatal("lastScore 不应为 nil")
	}
	if repo.lastScore.Threshold != 0.50 {
		t.Errorf("persisted Threshold=%v want 0.50（生效阈值）", repo.lastScore.Threshold)
	}
}

// TestHumanizeEvalService_Evaluate_PersistsDefaultThreshold 未定制时落库默认阈值
func TestHumanizeEvalService_Evaluate_PersistsDefaultThreshold(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.92))
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, nil, nil, repo, nil)
	input := &dto.HumanizeEvalInput{AIReply: "测试", CustomerMessage: "问题"}
	if _, err := svc.Evaluate(context.Background(), input); err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if repo.lastScore == nil {
		t.Fatal("lastScore 不应为 nil")
	}
	if repo.lastScore.Threshold != DefaultThreshold {
		t.Errorf("persisted Threshold=%v want 默认 %.2f", repo.lastScore.Threshold, DefaultThreshold)
	}
}

// TestHumanizeEvalService_NilSampleCollector nil sampleCollector 不应 panic
func TestHumanizeEvalService_NilSampleCollector(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.80))
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, nil, nil, repo, nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result.Passed {
		t.Errorf("Passed=true want false")
	}
}

// TestHumanizeEvalService_CollectError 收集低质样本失败不应影响主流程
func TestHumanizeEvalService_CollectError(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.80))
	repo := newStubScoreRepo()
	collector := newStubSampleCollector()
	collector.collectErr = errors.New("collector unavailable")
	svc := newServiceWithStubs(rule, nil, nil, repo, collector)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("收集失败不应导致 Evaluate 报错: %v", err)
	}
	if result.Passed {
		t.Errorf("Passed=true want false")
	}
}

// TestHumanizeEvalService_BaselineInjection 销冠基线应被查询
func TestHumanizeEvalService_BaselineInjection(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.78))
	llm := newStubEvaluator(makeResult(0.90))
	baseline := &model.ChampionBaseline{
		Persona:         "美妆顾问",
		Industry:        "美妆",
		Intent:          "ask_product",
		Naturalness:     0.85,
		Conciseness:     0.90,
		Empathy:         0.80,
		Professionalism: 0.85,
		Persuasiveness:  0.80,
	}
	baselineRepo := newStubBaselineRepo(baseline)
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, llm, baselineRepo, repo, nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
		Persona:         "美妆顾问",
		Industry:        "美妆",
		Intent:          "ask_product",
	}
	_, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
}

// TestHumanizeEvalService_BaselineFindError 基线查询失败不应影响主流程
func TestHumanizeEvalService_BaselineFindError(t *testing.T) {
	rule := newStubEvaluator(makeResult(0.78))
	llm := newStubEvaluator(makeResult(0.90))
	baselineRepo := newStubBaselineRepo(nil)
	baselineRepo.findErr = errors.New("db error")
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, llm, baselineRepo, repo, nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "测试",
		CustomerMessage: "问题",
		Persona:         "美妆顾问",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("基线查询失败不应导致 Evaluate 报错: %v", err)
	}
	if !result.Passed {
		t.Errorf("Passed=false want true")
	}
}

// TestHumanizeEvalService_E2E_HighQualityReply 高质量回复直接通过
func TestHumanizeEvalService_E2E_HighQualityReply(t *testing.T) {
	rule := NewRuleScorer()
	repo := newStubScoreRepo()
	svc := newServiceWithStubs(rule, nil, nil, repo, nil)
	input := &dto.HumanizeEvalInput{
		AIReply:         "这款精华含烟酰胺，能提亮肤色，配合玻尿酸保湿效果更好，现在下单还有满减优惠哦。",
		CustomerMessage: "这款产品怎么样？",
		Persona:         "美妆顾问",
		Industry:        "美妆",
		Intent:          "ask_product",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	t.Logf("TotalScore=%v Passed=%v Strategy=%v", result.TotalScore, result.Passed, result.SampleStrategy)
	if result.EvaluatorType != "rule" {
		t.Errorf("EvaluatorType=%q want rule", result.EvaluatorType)
	}
}

// TestHumanizeEvalService_E2E_AITraceReply AI 痕迹回复应低分转人工
func TestHumanizeEvalService_E2E_AITraceReply(t *testing.T) {
	rule := NewRuleScorer()
	repo := newStubScoreRepo()
	collector := newStubSampleCollector()
	svc := newServiceWithStubs(rule, nil, nil, repo, collector)
	input := &dto.HumanizeEvalInput{
		AIReply:         "作为 AI，我无法提供具体的购买建议。",
		CustomerMessage: "推荐一下产品",
		Intent:          "ask_product",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	t.Logf("TotalScore=%v Passed=%v", result.TotalScore, result.Passed)
	if result.TotalScore > 0.70 {
		t.Errorf("AI 痕迹回复 TotalScore=%v 应 ≤ 0.70", result.TotalScore)
	}
	if result.Passed {
		t.Errorf("Passed=true want false（AI 痕迹应不达标）")
	}
	if collector.collected != 1 {
		t.Errorf("collector.collected=%d want 1", collector.collected)
	}
}
