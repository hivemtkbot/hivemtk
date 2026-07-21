package humanize

// llm_scorer_test.go P0-4 LLMScorer 单元测试
//
// 覆盖：
//  1. 基本评估流程（含 self-consistency N=3）
//  2. Prompt 构建（G-Eval 风格）
//  3. LLM 返回解析（含 ```json 包裹与 CoT 前缀）
//  4. 中位数结果选取（pickMedianResult）
//  5. 加权欧氏距离计算
//  6. 采样决策（decideLLMSample）
//  7. LLM 失败降级
//  8. 边界用例（空输入、dispatcher nil）

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"marketing/internal/dto"
)

// ============================================================================
// 基本评估测试
// ============================================================================

// TestLLMScorer_Evaluate_Basic 基本 LLM 评估
func TestLLMScorer_Evaluate_Basic(t *testing.T) {
	// 3 次返回相同结果
	responses := []string{
		`{"scores":[{"dimension":"naturalness","score":0.85,"reason":"good"},{"dimension":"conciseness","score":0.9,"reason":"good"},{"dimension":"empathy","score":0.8,"reason":"good"},{"dimension":"professionalism","score":0.85,"reason":"good"},{"dimension":"persuasiveness","score":0.8,"reason":"good"}],"total_score":0.84,"distance_to_champion":0.1}`,
		`{"scores":[{"dimension":"naturalness","score":0.85,"reason":"good"},{"dimension":"conciseness","score":0.9,"reason":"good"},{"dimension":"empathy","score":0.8,"reason":"good"},{"dimension":"professionalism","score":0.85,"reason":"good"},{"dimension":"persuasiveness","score":0.8,"reason":"good"}],"total_score":0.84,"distance_to_champion":0.1}`,
		`{"scores":[{"dimension":"naturalness","score":0.85,"reason":"good"},{"dimension":"conciseness","score":0.9,"reason":"good"},{"dimension":"empathy","score":0.8,"reason":"good"},{"dimension":"professionalism","score":0.85,"reason":"good"},{"dimension":"persuasiveness","score":0.8,"reason":"good"}],"total_score":0.84,"distance_to_champion":0.1}`,
	}
	dispatcher := newStubLLMDispatcher(responses)
	scorer := NewLLMScorer(dispatcher)

	input := &dto.HumanizeEvalInput{
		CustomerMessage: "这个产品怎么样？",
		AIReply:         "这款产品成分是烟酰胺，保湿效果好。",
		Persona:         "美妆顾问",
		Industry:        "美妆",
		Intent:          "ask_product",
	}
	result, err := scorer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.TotalScore < 0.8 || result.TotalScore > 0.9 {
		t.Errorf("TotalScore=%v 应在 [0.8, 0.9]", result.TotalScore)
	}
	if result.EvaluatorType != "llm" {
		t.Errorf("EvaluatorType=%q want=llm", result.EvaluatorType)
	}
	if result.SampleStrategy != "sampled" {
		t.Errorf("SampleStrategy=%q want=sampled", result.SampleStrategy)
	}
	if result.LLMModel != "stub-llm-v1" {
		t.Errorf("LLMModel=%q want=stub-llm-v1", result.LLMModel)
	}
	if dispatcher.calls != 3 {
		t.Errorf("self-consistency 应调用 3 次, got %d", dispatcher.calls)
	}
}

// TestLLMScorer_Evaluate_SelfConsistencyMedian 中位数选取
func TestLLMScorer_Evaluate_SelfConsistencyMedian(t *testing.T) {
	// 3 次返回不同分数：0.70, 0.85, 0.95，中位数 = 0.85
	responses := []string{
		`{"scores":[{"dimension":"naturalness","score":0.7},{"dimension":"conciseness","score":0.7},{"dimension":"empathy","score":0.7},{"dimension":"professionalism","score":0.7},{"dimension":"persuasiveness","score":0.7}],"total_score":0.70}`,
		`{"scores":[{"dimension":"naturalness","score":0.85},{"dimension":"conciseness","score":0.85},{"dimension":"empathy","score":0.85},{"dimension":"professionalism","score":0.85},{"dimension":"persuasiveness","score":0.85}],"total_score":0.85}`,
		`{"scores":[{"dimension":"naturalness","score":0.95},{"dimension":"conciseness","score":0.95},{"dimension":"empathy","score":0.95},{"dimension":"professionalism","score":0.95},{"dimension":"persuasiveness","score":0.95}],"total_score":0.95}`,
	}
	dispatcher := newStubLLMDispatcher(responses)
	scorer := NewLLMScorer(dispatcher)
	input := &dto.HumanizeEvalInput{AIReply: "测试回复"}
	result, err := scorer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if !approxEqualTol(result.TotalScore, 0.85, 1e-3) {
		t.Errorf("中位数 TotalScore=%v want ≈ 0.85", result.TotalScore)
	}
}

// TestLLMScorer_Evaluate_NilInput 空输入报错
func TestLLMScorer_Evaluate_NilInput(t *testing.T) {
	scorer := NewLLMScorer(newStubLLMDispatcher(nil))
	_, err := scorer.Evaluate(context.Background(), nil)
	if err == nil {
		t.Error("nil input 应报错")
	}
}

// TestLLMScorer_Evaluate_NilDispatcher dispatcher nil 报错
func TestLLMScorer_Evaluate_NilDispatcher(t *testing.T) {
	scorer := NewLLMScorer(nil)
	input := &dto.HumanizeEvalInput{AIReply: "测试"}
	_, err := scorer.Evaluate(context.Background(), input)
	if err == nil {
		t.Error("dispatcher nil 应报错")
	}
}

// TestLLMScorer_Evaluate_AllFail 所有 self-consistency 失败应报错
func TestLLMScorer_Evaluate_AllFail(t *testing.T) {
	dispatcher := newStubLLMDispatcher(nil)
	dispatcher.err = errors.New("LLM service unavailable")
	dispatcher.failOn = 1 // 第 1 次失败，由于 responses 为空，所有调用都失败
	scorer := NewLLMScorer(dispatcher)
	input := &dto.HumanizeEvalInput{AIReply: "测试"}
	_, err := scorer.Evaluate(context.Background(), input)
	if err == nil {
		t.Error("全部失败应报错")
	}
}

// TestLLMScorer_Evaluate_PartialFail 部分失败仍可返回中位数
func TestLLMScorer_Evaluate_PartialFail(t *testing.T) {
	// 第 1 次失败，第 2、3 次成功
	responses := []string{
		"", // 占位，第 1 次因 failOn 失败
		`{"scores":[{"dimension":"naturalness","score":0.8},{"dimension":"conciseness","score":0.8},{"dimension":"empathy","score":0.8},{"dimension":"professionalism","score":0.8},{"dimension":"persuasiveness","score":0.8}],"total_score":0.8}`,
		`{"scores":[{"dimension":"naturalness","score":0.9},{"dimension":"conciseness","score":0.9},{"dimension":"empathy","score":0.9},{"dimension":"professionalism","score":0.9},{"dimension":"persuasiveness","score":0.9}],"total_score":0.9}`,
	}
	dispatcher := newStubLLMDispatcher(responses)
	dispatcher.failOn = 1
	dispatcher.err = errors.New("first call failed")
	scorer := NewLLMScorer(dispatcher)
	input := &dto.HumanizeEvalInput{AIReply: "测试"}
	result, err := scorer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("部分失败应返回结果, got err: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// ============================================================================
// Prompt 构建测试
// ============================================================================

// TestBuildHumanizeLLMPrompt_Basic 基本 prompt 构建
func TestBuildHumanizeLLMPrompt_Basic(t *testing.T) {
	input := &dto.HumanizeEvalInput{
		CustomerMessage: "产品怎么样？",
		AIReply:         "很好用。",
		Persona:         "美妆顾问",
		Industry:        "美妆",
		Platform:        "wechat",
		Intent:          "ask_product",
	}
	prompt := buildHumanizeLLMPrompt(input, nil)
	if prompt == "" {
		t.Fatal("prompt 不应为空")
	}
	// 必须包含关键字段
	mustContain := []string{
		"产品怎么样？",
		"很好用。",
		"美妆顾问",
		"美妆",
		"wechat",
		"ask_product",
		"naturalness",
		"conciseness",
		"empathy",
		"professionalism",
		"persuasiveness",
		"评估步骤",
		"输出 JSON",
	}
	for _, kw := range mustContain {
		if !stringContains(prompt, kw) {
			t.Errorf("prompt 应包含 %q", kw)
		}
	}
}

// TestBuildHumanizeLLMPrompt_WithBaseline 含销冠基线
func TestBuildHumanizeLLMPrompt_WithBaseline(t *testing.T) {
	input := &dto.HumanizeEvalInput{
		CustomerMessage: "test",
		AIReply:         "test",
	}
	baseline := &dto.ChampionBaselineDTO{
		Naturalness:     0.85,
		Conciseness:     0.90,
		Empathy:         0.80,
		Professionalism: 0.85,
		Persuasiveness:  0.80,
	}
	prompt := buildHumanizeLLMPrompt(input, baseline)
	if !stringContains(prompt, "销冠基线分") {
		t.Error("prompt 应包含销冠基线分")
	}
	if !stringContains(prompt, "naturalness=0.850") {
		t.Error("prompt 应包含 naturalness=0.850")
	}
}

// ============================================================================
// 解析测试
// ============================================================================

// TestParseHumanizeEvalResult_PlainJSON 纯 JSON
func TestParseHumanizeEvalResult_PlainJSON(t *testing.T) {
	content := `{"scores":[{"dimension":"naturalness","score":0.85,"reason":"good"}],"total_score":0.85,"distance_to_champion":0.1}`
	result, err := parseHumanizeEvalResult(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Scores) != 1 {
		t.Errorf("Scores len=%d want 1", len(result.Scores))
	}
	if result.Scores[0].Score != 0.85 {
		t.Errorf("score=%v want 0.85", result.Scores[0].Score)
	}
	if result.TotalScore != 0.85 {
		t.Errorf("TotalScore=%v want 0.85", result.TotalScore)
	}
}

// TestParseHumanizeEvalResult_CodeFenced ```json 包裹
func TestParseHumanizeEvalResult_CodeFenced(t *testing.T) {
	content := "```json\n{\"scores\":[{\"dimension\":\"naturalness\",\"score\":0.9}],\"total_score\":0.9}\n```"
	result, err := parseHumanizeEvalResult(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.TotalScore != 0.9 {
		t.Errorf("TotalScore=%v want 0.9", result.TotalScore)
	}
}

// TestParseHumanizeEvalResult_WithCoT 含 CoT 前缀
func TestParseHumanizeEvalResult_WithCoT(t *testing.T) {
	content := `评估步骤：
1. 无 AI 痕迹词
2. 字数 30 字
3. 共情良好
4. 含专业词
5. 有行动召唤
6. 与基线差距 0.05

{"scores":[{"dimension":"naturalness","score":0.88}],"total_score":0.88}`
	result, err := parseHumanizeEvalResult(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.TotalScore != 0.88 {
		t.Errorf("TotalScore=%v want 0.88", result.TotalScore)
	}
}

// TestParseHumanizeEvalResult_EmptyScores 空分数报错
func TestParseHumanizeEvalResult_EmptyScores(t *testing.T) {
	content := `{"scores":[],"total_score":0.5}`
	_, err := parseHumanizeEvalResult(content)
	if err == nil {
		t.Error("空 scores 应报错")
	}
}

// TestParseHumanizeEvalResult_InvalidJSON 无效 JSON
func TestParseHumanizeEvalResult_InvalidJSON(t *testing.T) {
	content := `not a json at all`
	_, err := parseHumanizeEvalResult(content)
	if err == nil {
		t.Error("无效 JSON 应报错")
	}
}

// TestParseHumanizeEvalResult_ScoreClamp 分数截断
func TestParseHumanizeEvalResult_ScoreClamp(t *testing.T) {
	content := `{"scores":[{"dimension":"naturalness","score":1.5},{"dimension":"conciseness","score":-0.3}],"total_score":0.6}`
	result, err := parseHumanizeEvalResult(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.Scores[0].Score != 1.0 {
		t.Errorf("1.5 应截断为 1.0, got %v", result.Scores[0].Score)
	}
	if result.Scores[1].Score != 0.0 {
		t.Errorf("-0.3 应截断为 0.0, got %v", result.Scores[1].Score)
	}
}

// TestParseHumanizeEvalResult_AutoTotal 自动计算综合分
func TestParseHumanizeEvalResult_AutoTotal(t *testing.T) {
	// total_score=0 时自动按加权计算
	content := `{"scores":[{"dimension":"naturalness","score":0.85},{"dimension":"conciseness","score":0.85},{"dimension":"empathy","score":0.85},{"dimension":"professionalism","score":0.85},{"dimension":"persuasiveness","score":0.85}],"total_score":0}`
	result, err := parseHumanizeEvalResult(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	// 5 维都 0.85，加权 = 0.85
	if !approxEqualTol(result.TotalScore, 0.85, 1e-3) {
		t.Errorf("自动计算 TotalScore=%v want 0.85", result.TotalScore)
	}
}

// ============================================================================
// pickMedianResult 测试
// ============================================================================

// TestPickMedianResult_Odd 奇数取中位数
func TestPickMedianResult_Odd(t *testing.T) {
	results := []*dto.HumanizeEvalResult{
		{TotalScore: 0.7},
		{TotalScore: 0.85},
		{TotalScore: 0.95},
	}
	picked := pickMedianResult(results)
	if !approxEqual(picked.TotalScore, 0.85) {
		t.Errorf("中位数=%v want 0.85", picked.TotalScore)
	}
}

// TestPickMedianResult_Even 偶数取中间偏右
func TestPickMedianResult_Even(t *testing.T) {
	results := []*dto.HumanizeEvalResult{
		{TotalScore: 0.7},
		{TotalScore: 0.8},
		{TotalScore: 0.9},
		{TotalScore: 0.95},
	}
	picked := pickMedianResult(results)
	// len/2 = 2，排序后 [0.7, 0.8, 0.9, 0.95]，索引 2 = 0.9
	if !approxEqual(picked.TotalScore, 0.9) {
		t.Errorf("偶数中位数=%v want 0.9", picked.TotalScore)
	}
}

// TestPickMedianResult_Single 单元素
func TestPickMedianResult_Single(t *testing.T) {
	results := []*dto.HumanizeEvalResult{
		{TotalScore: 0.5},
	}
	picked := pickMedianResult(results)
	if !approxEqual(picked.TotalScore, 0.5) {
		t.Errorf("单元素=%v want 0.5", picked.TotalScore)
	}
}

// ============================================================================
// 加权欧氏距离测试
// ============================================================================

// TestWeightedEuclideanDistance_Basic 基本距离计算
func TestWeightedEuclideanDistance_Basic(t *testing.T) {
	scores := []dto.HumanizeDimensionScore{
		{Dimension: dto.HumanizeDimNaturalness, Score: 0.85},
		{Dimension: dto.HumanizeDimConciseness, Score: 0.90},
		{Dimension: dto.HumanizeDimEmpathy, Score: 0.80},
		{Dimension: dto.HumanizeDimProfessionalism, Score: 0.85},
		{Dimension: dto.HumanizeDimPersuasiveness, Score: 0.80},
	}
	baseline := &dto.ChampionBaselineDTO{
		Naturalness:     0.85,
		Conciseness:     0.90,
		Empathy:         0.80,
		Professionalism: 0.85,
		Persuasiveness:  0.80,
	}
	dist := weightedEuclideanDistance(scores, baseline)
	if !approxEqual(dist, 0.0) {
		t.Errorf("完全相同的距离=%v want 0", dist)
	}
}

// TestWeightedEuclideanDistance_NonZero 有差距
func TestWeightedEuclideanDistance_NonZero(t *testing.T) {
	scores := []dto.HumanizeDimensionScore{
		{Dimension: dto.HumanizeDimNaturalness, Score: 0.75}, // diff 0.10
		{Dimension: dto.HumanizeDimConciseness, Score: 0.90},
		{Dimension: dto.HumanizeDimEmpathy, Score: 0.80},
		{Dimension: dto.HumanizeDimProfessionalism, Score: 0.85},
		{Dimension: dto.HumanizeDimPersuasiveness, Score: 0.80},
	}
	baseline := &dto.ChampionBaselineDTO{
		Naturalness:     0.85,
		Conciseness:     0.90,
		Empathy:         0.80,
		Professionalism: 0.85,
		Persuasiveness:  0.80,
	}
	// 只有 naturalness 差 0.10，权重 0.25
	// D = sqrt(0.25 * 0.10^2) = sqrt(0.0025) = 0.05
	dist := weightedEuclideanDistance(scores, baseline)
	if !approxEqualTol(dist, 0.05, 1e-3) {
		t.Errorf("加权欧氏距离=%v want ≈ 0.05", dist)
	}
}

// TestWeightedEuclideanDistance_NilBaseline nil 基线返回 0
func TestWeightedEuclideanDistance_NilBaseline(t *testing.T) {
	scores := []dto.HumanizeDimensionScore{
		{Dimension: dto.HumanizeDimNaturalness, Score: 0.85},
	}
	dist := weightedEuclideanDistance(scores, nil)
	if dist != 0 {
		t.Errorf("nil baseline 距离=%v want 0", dist)
	}
}

// ============================================================================
// 采样决策测试
// ============================================================================

// TestDecideLLMSample_Boundary 边界样本 100% 触发
func TestDecideLLMSample_Boundary(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	// score ∈ [0.70, 0.85) → boundary
	for i := 0; i < 20; i++ {
		dec := decideLLMSample(0.75, 0.85, 0.70, 0.85, 0.10, r)
		if !dec.NeedLLM {
			t.Errorf("边界样本应 100%% 触发 LLM, iteration %d", i)
		}
		if dec.Strategy != "boundary" {
			t.Errorf("边界策略=%q want=boundary", dec.Strategy)
		}
	}
}

// TestDecideLLMSample_BelowThreshold_Low 低于边界低值 10% 采样
func TestDecideLLMSample_BelowThreshold_Low(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	triggerCount := 0
	totalRuns := 1000
	for i := 0; i < totalRuns; i++ {
		dec := decideLLMSample(0.50, 0.85, 0.70, 0.85, 0.10, r)
		if dec.NeedLLM {
			triggerCount++
		}
	}
	// 10% 采样，1000 次中应约 100 次（允许 [60, 140]）
	if triggerCount < 60 || triggerCount > 140 {
		t.Errorf("10%% 采样触发次数=%d 应在 [60, 140]", triggerCount)
	}
}

// TestDecideLLMSample_AboveThreshold 高分 1% 监控采样
func TestDecideLLMSample_AboveThreshold(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	triggerCount := 0
	totalRuns := 1000
	for i := 0; i < totalRuns; i++ {
		dec := decideLLMSample(0.90, 0.85, 0.70, 0.85, 0.10, r)
		if dec.NeedLLM {
			triggerCount++
			if dec.Strategy != "sampled_monitor" {
				t.Errorf("高分采样策略=%q want=sampled_monitor", dec.Strategy)
			}
		}
	}
	// 1% 监控采样，1000 次中应约 10 次（允许 [3, 25]）
	if triggerCount < 3 || triggerCount > 25 {
		t.Errorf("1%% 监控采样触发次数=%d 应在 [3, 25]", triggerCount)
	}
}

// TestDecideLLMSample_ExactBoundaryLow 刚好等于 boundaryLow
func TestDecideLLMSample_ExactBoundaryLow(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	dec := decideLLMSample(0.70, 0.85, 0.70, 0.85, 0.10, r)
	if !dec.NeedLLM {
		t.Error("score=boundaryLow 应触发 boundary")
	}
	if dec.Strategy != "boundary" {
		t.Errorf("策略=%q want=boundary", dec.Strategy)
	}
}

// TestDecideLLMSample_ExactBoundaryHigh 刚好低于 boundaryHigh
func TestDecideLLMSample_ExactBoundaryHigh(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	dec := decideLLMSample(0.849, 0.85, 0.70, 0.85, 0.10, r)
	if !dec.NeedLLM {
		t.Error("score < boundaryHigh 应触发 boundary")
	}
	if dec.Strategy != "boundary" {
		t.Errorf("策略=%q want=boundary", dec.Strategy)
	}
}

// TestDecideLLMSample_EqualBoundaryHigh 等于 boundaryHigh 不触发 boundary
func TestDecideLLMSample_EqualBoundaryHigh(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	// score=0.85 = boundaryHigh = threshold
	// 不属于 [0.70, 0.85)，也不属于 < threshold
	// 走 >= threshold 分支，1% 监控
	totalBoundary := 0
	for i := 0; i < 100; i++ {
		dec := decideLLMSample(0.85, 0.85, 0.70, 0.85, 0.10, r)
		if dec.Strategy == "boundary" {
			totalBoundary++
		}
	}
	if totalBoundary > 0 {
		t.Errorf("score=boundaryHigh 不应触发 boundary, 触发次数=%d", totalBoundary)
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// stringContains 字符串包含（避免引入 strings 包）
func stringContains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

// indexOf 子串索引
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
