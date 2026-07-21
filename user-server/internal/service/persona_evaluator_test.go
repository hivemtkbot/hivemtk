package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"marketing/internal/model"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupPersonaTestDB 初始化测试 DB（含 LowQualitySample 表）
func setupPersonaTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.LowQualitySample{},
	)
}

// =================== RuleBasedPersonaEvaluator 测试 ===================

func TestRuleBasedPersonaEvaluator_HighQualityReply(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		CustomerMessage: "这个产品多少钱？",
		AIReply:         "亲，这款产品原价 299，今天活动价 199，您看合适吗？",
		Persona:         "美妆顾问",
		Industry:        "美妆",
		Platform:        "wechat",
		Intent:          "price_inquiry",
	}
	result, err := e.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result.TotalScore < 0.85 {
		t.Errorf("expected high quality score >= 0.85, got %.3f", result.TotalScore)
		t.Logf("scores: %+v", result.Scores)
	}
	if !result.Passed {
		t.Errorf("expected pass, got fail (score=%.3f)", result.TotalScore)
	}
}

func TestRuleBasedPersonaEvaluator_AITracesLowNaturalness(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		CustomerMessage: "推荐一下产品",
		AIReply:         "作为 AI 助手，我无法直接推荐产品，但我可以给您一些建议。",
	}
	result, _ := e.Evaluate(context.Background(), input)
	nat, ok := result.ScoreByDimension(PersonaDimensionNaturalness)
	if !ok {
		t.Fatal("expected naturalness score")
	}
	if nat >= 0.6 {
		t.Errorf("expected low naturalness (< 0.6) for AI traces, got %.3f", nat)
	}
}

func TestRuleBasedPersonaEvaluator_AdLawWordsLowCompliance(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		CustomerMessage: "这个产品怎么样？",
		AIReply:         "这是全网最好的产品，销量第一，国家级认证！",
	}
	result, _ := e.Evaluate(context.Background(), input)
	comp, _ := result.ScoreByDimension(PersonaDimensionCompliance)
	if comp >= 0.4 {
		t.Errorf("expected low compliance (< 0.4) for ad law words, got %.3f", comp)
	}
}

func TestRuleBasedPersonaEvaluator_FalsePromiseLowCompliance(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		CustomerMessage: "用了会有效果吗？",
		AIReply:         "绝对有效，保证见效，包治百病！",
	}
	result, _ := e.Evaluate(context.Background(), input)
	comp, _ := result.ScoreByDimension(PersonaDimensionCompliance)
	if comp >= 0.4 {
		t.Errorf("expected low compliance for false promise, got %.3f", comp)
	}
}

func TestRuleBasedPersonaEvaluator_LongReplyLowConciseness(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	longReply := strings.Repeat("这是一段很长的回复内容。", 30) // > 250 字
	input := &PersonaEvaluationInput{
		AIReply: longReply,
	}
	result, _ := e.Evaluate(context.Background(), input)
	conc, _ := result.ScoreByDimension(PersonaDimensionConciseness)
	if conc > 0.3 {
		t.Errorf("expected low conciseness (<= 0.3) for long reply, got %.3f", conc)
	}
}

func TestRuleBasedPersonaEvaluator_ShortReplyHighConciseness(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		AIReply: "好的，马上给您安排。",
	}
	result, _ := e.Evaluate(context.Background(), input)
	conc, _ := result.ScoreByDimension(PersonaDimensionConciseness)
	if conc < 0.9 {
		t.Errorf("expected high conciseness (>= 0.9) for short reply, got %.3f", conc)
	}
}

func TestRuleBasedPersonaEvaluator_LowRelevance(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		CustomerMessage: "请问这款手机多少钱？支持 5G 吗？",
		AIReply:         "今天天气真好，适合出门散步。",
	}
	result, _ := e.Evaluate(context.Background(), input)
	rel, _ := result.ScoreByDimension(PersonaDimensionRelevance)
	if rel > 0.4 {
		t.Errorf("expected low relevance (<= 0.4) for off-topic, got %.3f", rel)
	}
}

func TestRuleBasedPersonaEvaluator_HighRelevance(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		CustomerMessage: "这款手机多少钱？支持 5G 吗？",
		AIReply:         "这款手机 5G 版本售价 3999，支持双模 5G，您喜欢吗？",
	}
	result, _ := e.Evaluate(context.Background(), input)
	rel, _ := result.ScoreByDimension(PersonaDimensionRelevance)
	if rel < 0.7 {
		t.Errorf("expected high relevance (>= 0.7), got %.3f", rel)
	}
}

func TestRuleBasedPersonaEvaluator_ComplaintNeedEmpathy(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		CustomerMessage: "你们的产品坏了，我要投诉！",
		AIReply:         "好的，我帮您处理。",
		Intent:          "complaint",
	}
	result, _ := e.Evaluate(context.Background(), input)
	emo, _ := result.ScoreByDimension(PersonaDimensionEmotion)
	if emo > 0.5 {
		t.Errorf("expected low emotion (<= 0.5) for complaint without empathy, got %.3f", emo)
	}
}

func TestRuleBasedPersonaEvaluator_ComplaintWithEmpathy(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		CustomerMessage: "你们的产品坏了，我要投诉！",
		AIReply:         "非常抱歉给您带来不便，我理解您的感受，立刻帮您处理。",
		Intent:          "complaint",
	}
	result, _ := e.Evaluate(context.Background(), input)
	emo, _ := result.ScoreByDimension(PersonaDimensionEmotion)
	if emo < 0.8 {
		t.Errorf("expected high emotion (>= 0.8) with empathy, got %.3f", emo)
	}
}

func TestRuleBasedPersonaEvaluator_PersonaMatch(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		AIReply:  "作为美妆顾问，我建议您选择适合干皮的粉底液。",
		Persona:  "美妆顾问",
		Industry: "美妆",
	}
	result, _ := e.Evaluate(context.Background(), input)
	per, _ := result.ScoreByDimension(PersonaDimensionPersona)
	if per < 0.85 {
		t.Errorf("expected high persona score with match, got %.3f", per)
	}
}

func TestRuleBasedPersonaEvaluator_AllDimensionsPresent(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		AIReply: "好的，亲",
	}
	result, _ := e.Evaluate(context.Background(), input)
	if len(result.Scores) != len(AllPersonaDimensions) {
		t.Errorf("expected %d dimensions, got %d", len(AllPersonaDimensions), len(result.Scores))
	}
	// 所有维度都应有得分
	for _, dim := range AllPersonaDimensions {
		if _, ok := result.ScoreByDimension(dim); !ok {
			t.Errorf("missing dimension: %s", dim)
		}
	}
}

func TestRuleBasedPersonaEvaluator_WeightedScore(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	// 全 1 分（理想回复）应得 1.0
	input := &PersonaEvaluationInput{
		AIReply:         "嗯，亲，我理解您的心情，这款产品适合您，建议购买。",
		CustomerMessage: "我想要适合的产品",
		Persona:         "顾问",
		Industry:        "零售",
	}
	result, _ := e.Evaluate(context.Background(), input)
	// 综合分应在合理范围
	if result.TotalScore < 0 || result.TotalScore > 1 {
		t.Errorf("total score out of [0,1]: %.3f", result.TotalScore)
	}
	// 验证加权计算
	expectedTotal := 0.0
	for _, s := range result.Scores {
		expectedTotal += PersonaDimensionWeight[s.Dimension] * s.Score
	}
	// 四舍五入到 4 位
	expectedTotal = float64(int(expectedTotal*10000)) / 10000
	if abs(result.TotalScore-expectedTotal) > 0.001 {
		t.Errorf("weighted score mismatch: expected %.4f, got %.4f", expectedTotal, result.TotalScore)
	}
}

func TestRuleBasedPersonaEvaluator_NilInput(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	_, err := e.Evaluate(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil input")
	}
}

func TestRuleBasedPersonaEvaluator_EmptyReply(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	_, err := e.Evaluate(context.Background(), &PersonaEvaluationInput{})
	if err == nil {
		t.Error("expected error for empty reply")
	}
}

func TestRuleBasedPersonaEvaluator_CustomThreshold(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator().WithThreshold(0.95)
	input := &PersonaEvaluationInput{
		AIReply: "好的，亲",
	}
	result, _ := e.Evaluate(context.Background(), input)
	// 设置高阈值后，普通回复不应通过
	if result.Passed {
		t.Errorf("expected fail with threshold 0.95, got pass (score=%.3f)", result.TotalScore)
	}
}

// =================== PersonaEvaluationService 测试 ===================

func TestPersonaEvaluationService_Evaluate_HighQualityPass(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator())
	input := &PersonaEvaluationInput{
		CustomerMessage: "这个多少钱？",
		AIReply:         "亲，这款 199 元，今天有活动，您看合适吗？",
		Persona:         "美妆顾问",
		Industry:        "美妆",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass, got fail (score=%.3f)", result.TotalScore)
	}
	if result.AttemptCount != 1 {
		t.Errorf("expected attempt 1, got %d", result.AttemptCount)
	}
}

func TestPersonaEvaluationService_Evaluate_LowQualityFail(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator())
	input := &PersonaEvaluationInput{
		AIReply: "这是全网最好的产品，销量第一，绝对保证见效！",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result.Passed {
		t.Errorf("expected fail for low quality, got pass (score=%.3f)", result.TotalScore)
	}
}

func TestPersonaEvaluationService_EvaluateWithRetry_PassOnFirstAttempt(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator())
	input := &PersonaEvaluationInput{
		AIReply:         "嗯，亲，这款适合您，建议购买。",
		CustomerMessage: "推荐一下",
		Persona:         "顾问",
	}
	regenerateCalls := 0
	regenerateFn := func(ctx context.Context, input *PersonaEvaluationInput, feedback *PersonaEvaluationResult) (string, error) {
		regenerateCalls++
		return "更好的回复", nil
	}
	result, err := svc.EvaluateWithRetry(context.Background(), input, regenerateFn)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass on first attempt, got fail")
	}
	if regenerateCalls != 0 {
		t.Errorf("expected 0 regenerate calls (passed first), got %d", regenerateCalls)
	}
	if result.AttemptCount != 1 {
		t.Errorf("expected attempt 1, got %d", result.AttemptCount)
	}
}

func TestPersonaEvaluationService_EvaluateWithRetry_PassOnSecondAttempt(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator())
	// 第一次低质量，重生成后高质量
	input := &PersonaEvaluationInput{
		AIReply:         "全网最好的产品，绝对保证",
		CustomerMessage: "推荐产品",
		Persona:         "顾问",
	}
	regenerateCalls := 0
	regenerateFn := func(ctx context.Context, input *PersonaEvaluationInput, feedback *PersonaEvaluationResult) (string, error) {
		regenerateCalls++
		return "亲，这款产品适合您，价格 199 元，您看合适吗？", nil
	}
	result, err := svc.EvaluateWithRetry(context.Background(), input, regenerateFn)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass on second attempt, got fail (score=%.3f)", result.TotalScore)
		t.Logf("scores: %+v", result.Scores)
	}
	if regenerateCalls != 1 {
		t.Errorf("expected 1 regenerate call, got %d", regenerateCalls)
	}
	if result.AttemptCount != 2 {
		t.Errorf("expected attempt 2, got %d", result.AttemptCount)
	}
	if len(result.AllReplies) != 2 {
		t.Errorf("expected 2 replies, got %d", len(result.AllReplies))
	}
}

func TestPersonaEvaluationService_EvaluateWithRetry_ExhaustedAndCollect(t *testing.T) {
	db := setupPersonaTestDB(t)
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator()).
		WithSampleCollector(NewDBLowQualitySampleCollector(db))
	// 始终低质量
	input := &PersonaEvaluationInput{
		AIReply:         "全网最好的产品，绝对保证",
		CustomerMessage: "推荐产品",
		CustomerID:      "c-exhausted",
		SessionID:       "s-1",
	}
	regenerateFn := func(ctx context.Context, input *PersonaEvaluationInput, feedback *PersonaEvaluationResult) (string, error) {
		return "依然最好的产品，绝对保证", nil // 重生成还是低质量
	}
	result, err := svc.EvaluateWithRetry(context.Background(), input, regenerateFn)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result.Passed {
		t.Errorf("expected fail after exhaustion, got pass")
	}
	if result.AttemptCount != DefaultPersonaMaxRetry {
		t.Errorf("expected %d attempts, got %d", DefaultPersonaMaxRetry, result.AttemptCount)
	}
	// 验证低质样本已收集
	var count int64
	db.Model(&model.LowQualitySample{}).Where("customer_id = ?", "c-exhausted").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 low quality sample, got %d", count)
	}
	// 验证样本字段
	var sample model.LowQualitySample
	db.First(&sample, "customer_id = ?", "c-exhausted")
	if sample.AttemptCount != DefaultPersonaMaxRetry {
		t.Errorf("expected sample attempts %d, got %d", DefaultPersonaMaxRetry, sample.AttemptCount)
	}
	if sample.TotalScore >= svc.threshold {
		t.Errorf("expected sample total < threshold, got %.3f", sample.TotalScore)
	}
	if sample.SampleType != model.LowQualitySampleRetryExhausted {
		t.Errorf("expected retry_exhausted type, got %s", sample.SampleType)
	}
}

func TestPersonaEvaluationService_EvaluateWithRetry_NoRegenerateFn(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator())
	input := &PersonaEvaluationInput{
		AIReply: "全网最好的产品",
	}
	// regenerateFn 为 nil，应退化为单次评估
	result, err := svc.EvaluateWithRetry(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result.AttemptCount != 1 {
		t.Errorf("expected 1 attempt (no regenerate), got %d", result.AttemptCount)
	}
}

func TestPersonaEvaluationService_EvaluateWithRetry_RegenerateError(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator())
	input := &PersonaEvaluationInput{
		AIReply: "全网最好的产品",
	}
	regenerateFn := func(ctx context.Context, input *PersonaEvaluationInput, feedback *PersonaEvaluationResult) (string, error) {
		return "", fmt.Errorf("regenerate failed")
	}
	// 重生成错误不应导致整个评估崩溃
	result, err := svc.EvaluateWithRetry(context.Background(), input, regenerateFn)
	if err != nil {
		t.Fatalf("expected graceful degradation, got error: %v", err)
	}
	if result.Passed {
		t.Errorf("expected fail (low quality), got pass")
	}
}

func TestPersonaEvaluationService_EvaluateWithRetry_EmptyReply(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator())
	_, err := svc.EvaluateWithRetry(context.Background(), &PersonaEvaluationInput{}, nil)
	if err == nil {
		t.Error("expected error for empty reply")
	}
}

func TestPersonaEvaluationService_EvaluateWithRetry_NilInput(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator())
	_, err := svc.EvaluateWithRetry(context.Background(), nil, nil)
	if err == nil {
		t.Error("expected error for nil input")
	}
}

func TestPersonaEvaluationService_WithThreshold(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator()).WithThreshold(0.5)
	input := &PersonaEvaluationInput{
		AIReply: "好的",
	}
	result, _ := svc.Evaluate(context.Background(), input)
	if !result.Passed {
		t.Errorf("expected pass with low threshold 0.5, got fail (score=%.3f)", result.TotalScore)
	}
}

func TestPersonaEvaluationService_WithMaxRetry(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator()).WithMaxRetry(5)
	input := &PersonaEvaluationInput{
		AIReply: "全网最好的产品",
	}
	regenerateFn := func(ctx context.Context, input *PersonaEvaluationInput, feedback *PersonaEvaluationResult) (string, error) {
		return "依然最好的产品", nil
	}
	result, _ := svc.EvaluateWithRetry(context.Background(), input, regenerateFn)
	if result.AttemptCount != 5 {
		t.Errorf("expected 5 attempts, got %d", result.AttemptCount)
	}
}

func TestPersonaEvaluationService_AllRepliesCollected(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator())
	input := &PersonaEvaluationInput{
		AIReply: "全网最好的产品",
	}
	regenerateFn := func(ctx context.Context, input *PersonaEvaluationInput, feedback *PersonaEvaluationResult) (string, error) {
		return "依然最好的产品，绝对保证", nil
	}
	result, _ := svc.EvaluateWithRetry(context.Background(), input, regenerateFn)
	// AllReplies 应包含原始 + 2 次重生成（3 次尝试 = 1 原始 + 2 重生成）
	if len(result.AllReplies) != DefaultPersonaMaxRetry {
		t.Errorf("expected %d replies, got %d", DefaultPersonaMaxRetry, len(result.AllReplies))
	}
}

// =================== LLMPersonaEvaluator 输入校验测试 ===================

func TestLLMPersonaEvaluator_NilDispatcher(t *testing.T) {
	e := &LLMPersonaEvaluator{dispatcher: nil}
	_, err := e.Evaluate(context.Background(), &PersonaEvaluationInput{AIReply: "测试"})
	if err == nil {
		t.Error("expected error for nil dispatcher")
	}
}

func TestLLMPersonaEvaluator_NilInput(t *testing.T) {
	e := &LLMPersonaEvaluator{}
	_, err := e.Evaluate(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil input")
	}
}

func TestLLMPersonaEvaluator_EmptyReply(t *testing.T) {
	e := &LLMPersonaEvaluator{}
	_, err := e.Evaluate(context.Background(), &PersonaEvaluationInput{})
	if err == nil {
		t.Error("expected error for empty reply")
	}
}

// =================== parsePersonaEvaluationResult 测试 ===================

func TestParsePersonaEvaluationResult_ValidJSON(t *testing.T) {
	content := `{"scores":[{"dimension":"naturalness","score":0.9,"reason":"口语化"},{"dimension":"relevance","score":0.85,"reason":"相关"},{"dimension":"persona","score":0.8,"reason":"符合人设"},{"dimension":"emotion","score":0.7,"reason":"有共情"},{"dimension":"conciseness","score":0.95,"reason":"简洁"},{"dimension":"compliance","score":1.0,"reason":"合规"}],"total_score":0.87}`
	result, err := parsePersonaEvaluationResult(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Scores) != 6 {
		t.Errorf("expected 6 scores, got %d", len(result.Scores))
	}
	if result.TotalScore != 0.87 {
		t.Errorf("expected 0.87, got %.3f", result.TotalScore)
	}
}

func TestParsePersonaEvaluationResult_JSONWithCodeBlock(t *testing.T) {
	content := "```json\n" + `{"scores":[{"dimension":"naturalness","score":0.9,"reason":""}],"total_score":0.9}` + "\n```"
	result, err := parsePersonaEvaluationResult(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Scores) != 1 {
		t.Errorf("expected 1 score, got %d", len(result.Scores))
	}
}

func TestParsePersonaEvaluationResult_EmptyScores(t *testing.T) {
	content := `{"scores":[],"total_score":0.5}`
	_, err := parsePersonaEvaluationResult(content)
	if err == nil {
		t.Error("expected error for empty scores")
	}
}

func TestParsePersonaEvaluationResult_InvalidJSON(t *testing.T) {
	_, err := parsePersonaEvaluationResult("not a json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParsePersonaEvaluationResult_MissingTotalScore(t *testing.T) {
	// 缺失 total_score 时应自动按权重计算
	content := `{"scores":[{"dimension":"naturalness","score":1.0,"reason":""},{"dimension":"relevance","score":1.0,"reason":""},{"dimension":"persona","score":1.0,"reason":""},{"dimension":"emotion","score":1.0,"reason":""},{"dimension":"conciseness","score":1.0,"reason":""},{"dimension":"compliance","score":1.0,"reason":""}]}`
	result, err := parsePersonaEvaluationResult(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if abs(result.TotalScore-1.0) > 0.001 {
		t.Errorf("expected auto-computed 1.0, got %.3f", result.TotalScore)
	}
}

func TestParsePersonaEvaluationResult_ScoreClamp(t *testing.T) {
	// 越界得分应被裁剪到 [0,1]
	content := `{"scores":[{"dimension":"naturalness","score":1.5,"reason":""},{"dimension":"relevance","score":-0.5,"reason":""},{"dimension":"persona","score":0.5,"reason":""},{"dimension":"emotion","score":0.5,"reason":""},{"dimension":"conciseness","score":0.5,"reason":""},{"dimension":"compliance","score":0.5,"reason":""}],"total_score":0.5}`
	result, err := parsePersonaEvaluationResult(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	nat, _ := result.ScoreByDimension(PersonaDimensionNaturalness)
	if nat != 1.0 {
		t.Errorf("expected clamp to 1.0, got %.3f", nat)
	}
	rel, _ := result.ScoreByDimension(PersonaDimensionRelevance)
	if rel != 0.0 {
		t.Errorf("expected clamp to 0.0, got %.3f", rel)
	}
}

// =================== 低质样本收集器测试 ===================

func TestDBLowQualitySampleCollector_HappyPath(t *testing.T) {
	db := setupPersonaTestDB(t)
	c := NewDBLowQualitySampleCollector(db)
	input := &PersonaEvaluationInput{
		CustomerID:      "c-1",
		SessionID:       "s-1",
		CustomerMessage: "你好",
		AIReply:         "差的回复",
		Persona:         "顾问",
		Industry:        "美妆",
		Platform:        "wechat",
		Intent:          "price_inquiry",
	}
	result := &PersonaEvaluationResult{
		Scores: []PersonaDimensionScore{
			{Dimension: PersonaDimensionNaturalness, Score: 0.5},
			{Dimension: PersonaDimensionRelevance, Score: 0.6},
			{Dimension: PersonaDimensionPersona, Score: 0.4},
			{Dimension: PersonaDimensionEmotion, Score: 0.3},
			{Dimension: PersonaDimensionConciseness, Score: 0.9},
			{Dimension: PersonaDimensionCompliance, Score: 0.2},
		},
		TotalScore:   0.5,
		FinalReply:   "差的回复",
		AttemptCount: 3,
		AllReplies:   []string{"差的回复", "依然差", "还是差"},
	}
	if err := c.Collect(context.Background(), input, result); err != nil {
		t.Fatalf("collect: %v", err)
	}
	var sample model.LowQualitySample
	if err := db.First(&sample, "customer_id = ?", "c-1").Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if sample.TotalScore != 0.5 {
		t.Errorf("expected 0.5, got %.3f", sample.TotalScore)
	}
	if sample.AttemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", sample.AttemptCount)
	}
	if sample.AIReply != "差的回复" {
		t.Errorf("expected '差的回复', got %s", sample.AIReply)
	}
	if sample.Persona != "顾问" {
		t.Errorf("expected '顾问', got %s", sample.Persona)
	}
	if !strings.Contains(sample.DimensionScores, "naturalness") {
		t.Errorf("expected dimension_scores to contain naturalness, got %s", sample.DimensionScores)
	}
	if !strings.Contains(sample.CandidateReplies, "差的回复") {
		t.Errorf("expected candidate_replies to contain reply, got %s", sample.CandidateReplies)
	}
	if sample.Handled {
		t.Errorf("expected Handled=false by default")
	}
}

func TestDBLowQualitySampleCollector_NilDB(t *testing.T) {
	c := &DBLowQualitySampleCollector{db: nil}
	err := c.Collect(context.Background(), &PersonaEvaluationInput{AIReply: "x"}, &PersonaEvaluationResult{})
	if err != nil {
		t.Errorf("expected nil err for nil db, got %v", err)
	}
}

func TestDBLowQualitySampleCollector_NilInput(t *testing.T) {
	db := setupPersonaTestDB(t)
	c := NewDBLowQualitySampleCollector(db)
	err := c.Collect(context.Background(), nil, nil)
	if err != nil {
		t.Errorf("expected nil err for nil input, got %v", err)
	}
}

func TestLogLowQualitySampleCollector_NoError(t *testing.T) {
	c := &LogLowQualitySampleCollector{}
	err := c.Collect(context.Background(), &PersonaEvaluationInput{AIReply: "x"}, &PersonaEvaluationResult{})
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

// =================== ListLowQualitySamples / MarkLowQualitySampleHandled 测试 ===================

func TestListLowQualitySamples_HappyPath(t *testing.T) {
	db := setupPersonaTestDB(t)
	// 插入 3 条
	for i := 0; i < 3; i++ {
		db.Create(&model.LowQualitySample{
			CustomerID:   fmt.Sprintf("c-%d", i),
			SampleType:   model.LowQualitySampleRetryExhausted,
			AIReply:      "差回复",
			TotalScore:   0.5,
			AttemptCount: 3,
		})
	}
	list, total, err := ListLowQualitySamples(db, nil, "", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 items, got %d", len(list))
	}
}

func TestListLowQualitySamples_FilterByHandled(t *testing.T) {
	db := setupPersonaTestDB(t)
	db.Create(&model.LowQualitySample{CustomerID: "c-1", AIReply: "差", Handled: false})
	db.Create(&model.LowQualitySample{CustomerID: "c-2", AIReply: "差", Handled: true})
	handled := false
	list, total, _ := ListLowQualitySamples(db, &handled, "", 10, 0)
	if total != 1 {
		t.Errorf("expected 1 unhandled, got %d", total)
	}
	if len(list) != 1 || list[0].CustomerID != "c-1" {
		t.Errorf("expected c-1, got %+v", list)
	}
}

func TestListLowQualitySamples_FilterByType(t *testing.T) {
	db := setupPersonaTestDB(t)
	db.Create(&model.LowQualitySample{CustomerID: "c-1", AIReply: "差", SampleType: model.LowQualitySampleRetryExhausted})
	db.Create(&model.LowQualitySample{CustomerID: "c-2", AIReply: "差", SampleType: model.LowQualitySamplePersona})
	list, total, _ := ListLowQualitySamples(db, nil, string(model.LowQualitySamplePersona), 10, 0)
	if total != 1 {
		t.Errorf("expected 1 persona type, got %d", total)
	}
	if len(list) != 1 || list[0].CustomerID != "c-2" {
		t.Errorf("expected c-2, got %+v", list)
	}
}

func TestListLowQualitySamples_NilDB(t *testing.T) {
	_, _, err := ListLowQualitySamples(nil, nil, "", 10, 0)
	if err == nil {
		t.Error("expected error for nil db")
	}
}

func TestMarkLowQualitySampleHandled(t *testing.T) {
	db := setupPersonaTestDB(t)
	sample := &model.LowQualitySample{CustomerID: "c-1", AIReply: "差", Handled: false}
	db.Create(sample)
	if err := MarkLowQualitySampleHandled(db, sample.ID, "operator-1", "已人工修正"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	var updated model.LowQualitySample
	db.First(&updated, sample.ID)
	if !updated.Handled {
		t.Errorf("expected Handled=true")
	}
	if updated.HandledBy != "operator-1" {
		t.Errorf("expected HandledBy=operator-1, got %s", updated.HandledBy)
	}
	if updated.HandledNote != "已人工修正" {
		t.Errorf("expected HandledNote=已人工修正, got %s", updated.HandledNote)
	}
	if updated.HandledAt == nil {
		t.Errorf("expected non-nil HandledAt")
	}
}

func TestMarkLowQualitySampleHandled_NilDB(t *testing.T) {
	err := MarkLowQualitySampleHandled(nil, 1, "op", "note")
	if err == nil {
		t.Error("expected error for nil db")
	}
}

// =================== 辅助函数测试 ===================

func TestComputeWeightedScore_AllMax(t *testing.T) {
	scores := []PersonaDimensionScore{}
	for _, dim := range AllPersonaDimensions {
		scores = append(scores, PersonaDimensionScore{Dimension: dim, Score: 1.0})
	}
	total := computeWeightedScore(scores)
	if abs(total-1.0) > 0.001 {
		t.Errorf("expected 1.0 for all max, got %.3f", total)
	}
}

func TestComputeWeightedScore_AllMin(t *testing.T) {
	scores := []PersonaDimensionScore{}
	for _, dim := range AllPersonaDimensions {
		scores = append(scores, PersonaDimensionScore{Dimension: dim, Score: 0.0})
	}
	total := computeWeightedScore(scores)
	if total != 0.0 {
		t.Errorf("expected 0.0 for all min, got %.3f", total)
	}
}

func TestComputeWeightedScore_Partial(t *testing.T) {
	// 仅自然度 1.0，其他 0.0，应得 0.25
	scores := []PersonaDimensionScore{
		{Dimension: PersonaDimensionNaturalness, Score: 1.0},
		{Dimension: PersonaDimensionRelevance, Score: 0.0},
		{Dimension: PersonaDimensionPersona, Score: 0.0},
		{Dimension: PersonaDimensionEmotion, Score: 0.0},
		{Dimension: PersonaDimensionConciseness, Score: 0.0},
		{Dimension: PersonaDimensionCompliance, Score: 0.0},
	}
	total := computeWeightedScore(scores)
	if abs(total-0.25) > 0.001 {
		t.Errorf("expected 0.25 (naturalness only), got %.3f", total)
	}
}

func TestClampScore(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{-0.5, 0},
		{0, 0},
		{0.5, 0.5},
		{1, 1},
		{1.5, 1},
		{0.555, 0.56}, // 四舍五入到 2 位
	}
	for _, tc := range tests {
		got := clampScore(tc.input)
		if abs(got-tc.expected) > 0.001 {
			t.Errorf("clampScore(%f) = %f, expected %f", tc.input, got, tc.expected)
		}
	}
}

func TestExtractKeywords(t *testing.T) {
	kws := extractKeywords("这款手机多少钱")
	if len(kws) == 0 {
		t.Error("expected non-empty keywords")
	}
	// 应包含 "这款"/"款手"/"手机" 等 2 字词
	foundPhone := false
	for _, kw := range kws {
		if kw == "手机" {
			foundPhone = true
		}
	}
	if !foundPhone {
		t.Errorf("expected to find '手机' in keywords: %v", kws)
	}
}

func TestExtractKeywords_Empty(t *testing.T) {
	if kws := extractKeywords(""); kws != nil {
		t.Errorf("expected nil for empty, got %v", kws)
	}
	if kws := extractKeywords("   "); kws != nil {
		t.Errorf("expected nil for whitespace, got %v", kws)
	}
}

func TestExtractKeywords_ShortText(t *testing.T) {
	kws := extractKeywords("好")
	if len(kws) != 1 || kws[0] != "好" {
		t.Errorf("expected ['好'], got %v", kws)
	}
}

func TestBuildPersonaEvaluationPrompt(t *testing.T) {
	input := &PersonaEvaluationInput{
		CustomerMessage: "你好",
		AIReply:         "您好",
		Persona:         "顾问",
		Industry:        "美妆",
		Platform:        "wechat",
		Intent:          "greeting",
	}
	prompt := buildPersonaEvaluationPrompt(input)
	if !strings.Contains(prompt, "naturalness") {
		t.Error("expected prompt to contain naturalness")
	}
	if !strings.Contains(prompt, "【客户消息】你好") {
		t.Error("expected prompt to contain customer message")
	}
	if !strings.Contains(prompt, "【AI 回复】您好") {
		t.Error("expected prompt to contain AI reply")
	}
	if !strings.Contains(prompt, "【销冠人设】顾问") {
		t.Error("expected prompt to contain persona")
	}
	if !strings.Contains(prompt, "【行业】美妆") {
		t.Error("expected prompt to contain industry")
	}
}

func TestPersonaEvaluationResult_ScoreByDimension(t *testing.T) {
	r := &PersonaEvaluationResult{
		Scores: []PersonaDimensionScore{
			{Dimension: PersonaDimensionNaturalness, Score: 0.9},
			{Dimension: PersonaDimensionCompliance, Score: 1.0},
		},
	}
	nat, ok := r.ScoreByDimension(PersonaDimensionNaturalness)
	if !ok || nat != 0.9 {
		t.Errorf("expected 0.9 naturalness, got %f (ok=%v)", nat, ok)
	}
	_, ok = r.ScoreByDimension(PersonaDimensionEmotion)
	if ok {
		t.Error("expected not found for missing dimension")
	}
}

// =================== PRD 验收测试 ===================

// TestPersonaEvaluation_PRDAcceptance_RetryAndCollect PRD §5.2 P1-2 G6 验收：
// "重生成循环正确触发" + "低质样本自动收集用于后续训练"
func TestPersonaEvaluation_PRDAcceptance_RetryAndCollect(t *testing.T) {
	db := setupPersonaTestDB(t)
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator()).
		WithSampleCollector(NewDBLowQualitySampleCollector(db))

	// 模拟始终低质量回复
	input := &PersonaEvaluationInput{
		CustomerID:      "c-prd",
		SessionID:       "s-prd",
		CustomerMessage: "这个产品怎么样",
		AIReply:         "这是全网最好的产品，绝对保证见效，第一品牌",
		Persona:         "美妆顾问",
		Industry:        "美妆",
		Platform:        "wechat",
		Intent:          "product_inquiry",
	}
	regenerateFn := func(ctx context.Context, input *PersonaEvaluationInput, feedback *PersonaEvaluationResult) (string, error) {
		// 重生成依然是低质量（含广告法极限词）
		return "依然是最好的产品，绝对保证，国家级认证", nil
	}
	result, err := svc.EvaluateWithRetry(context.Background(), input, regenerateFn)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	// 验收 1：3 次仍不达标 → 转人工
	if result.Passed {
		t.Errorf("PRD 验收失败：3 次仍不达标应转人工，got pass (score=%.3f)", result.TotalScore)
	}
	if result.AttemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", result.AttemptCount)
	}

	// 验收 2：低质样本自动收集
	var sample model.LowQualitySample
	if err := db.First(&sample, "customer_id = ?", "c-prd").Error; err != nil {
		t.Fatalf("PRD 验收失败：低质样本未收集: %v", err)
	}
	if sample.SampleType != model.LowQualitySampleRetryExhausted {
		t.Errorf("expected retry_exhausted, got %s", sample.SampleType)
	}
	if sample.TotalScore >= svc.threshold {
		t.Errorf("expected sample total < threshold, got %.3f", sample.TotalScore)
	}
}

// TestPersonaEvaluation_PRDAcceptance_HighQualityPass PRD §5.2 P1-2 G6 验收：
// "综合分 ≥ 0.85 → 通过"
func TestPersonaEvaluation_PRDAcceptance_HighQualityPass(t *testing.T) {
	svc := NewPersonaEvaluationService(NewRuleBasedPersonaEvaluator())
	// 高质量回复应通过
	input := &PersonaEvaluationInput{
		CustomerMessage: "这个多少钱？",
		AIReply:         "亲，这款产品 199 元，今天活动价 99 元，您看合适吗？",
		Persona:         "美妆顾问",
		Industry:        "美妆",
		Platform:        "wechat",
		Intent:          "price_inquiry",
	}
	result, err := svc.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !result.Passed {
		t.Errorf("PRD 验收失败：综合分 ≥ 0.85 应通过，got fail (score=%.3f)", result.TotalScore)
		t.Logf("scores: %+v", result.Scores)
	}
}

// TestPersonaEvaluation_PRDAcceptance_SixDimensions PRD §5.2 P1-2 G6 验收：
// "6 维度评分准确"
func TestPersonaEvaluation_PRDAcceptance_SixDimensions(t *testing.T) {
	e := NewRuleBasedPersonaEvaluator()
	input := &PersonaEvaluationInput{
		AIReply:         "嗯，亲，我理解您的心情，这款适合您，建议购买。",
		CustomerMessage: "我想要适合的产品",
		Persona:         "顾问",
		Industry:        "零售",
		Intent:          "product_inquiry",
	}
	result, _ := e.Evaluate(context.Background(), input)

	// 必须有 6 个维度
	expectedDims := map[PersonaDimension]bool{
		PersonaDimensionNaturalness: false,
		PersonaDimensionRelevance:   false,
		PersonaDimensionPersona:     false,
		PersonaDimensionEmotion:     false,
		PersonaDimensionConciseness: false,
		PersonaDimensionCompliance:  false,
	}
	for _, s := range result.Scores {
		if _, ok := expectedDims[s.Dimension]; ok {
			expectedDims[s.Dimension] = true
		}
	}
	for dim, found := range expectedDims {
		if !found {
			t.Errorf("PRD 验收失败：缺少维度 %s", dim)
		}
	}

	// 权重总和应为 1.0
	totalWeight := 0.0
	for _, w := range PersonaDimensionWeight {
		totalWeight += w
	}
	if abs(totalWeight-1.0) > 0.001 {
		t.Errorf("PRD 验收失败：权重总和应为 1.0, got %.3f", totalWeight)
	}
}
