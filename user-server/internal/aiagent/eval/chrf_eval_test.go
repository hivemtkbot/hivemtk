package eval

import (
	"context"
	"errors"
	"testing"
)

// ============================================================================
// chrF++ 单元测试（最小集）
// ----------------------------------------------------------------------------
// 覆盖：
//   - 完全相同的字符串 chrF≈1.0
//   - 完全不同的字符串 chrF<0.3
//   - 部分匹配的字符串 0.3<chrF<0.9
//   - 空字符串处理（candidate / reference 任一为空 → 0）
//   - 中英文混合
//   - 批量计算
//   - LLMJudge 解析容错（含 markdown / 0-100 分归一化）
//   - Evaluator 批量 / 单条
// ============================================================================

func TestChrF_IdenticalStrings(t *testing.T) {
	e := NewChrFEvaluator()
	score := e.Score("hello world", "hello world")
	if score < 0.99 || score > 1.01 {
		t.Errorf("identical strings should score ~1.0, got %f", score)
	}
}

func TestChrF_CompletelyDifferent(t *testing.T) {
	e := NewChrFEvaluator()
	// 完全不重叠的字符集
	score := e.Score("abcdefg", "xyzwvu")
	if score >= 0.3 {
		t.Errorf("completely different strings should score <0.3, got %f", score)
	}
}

func TestChrF_PartialMatch(t *testing.T) {
	e := NewChrFEvaluator()
	// 部分匹配：共享 "hello" 前缀
	score := e.Score("hello world", "hello there")
	if score < 0.3 || score > 0.9 {
		t.Errorf("partial match should score 0.3-0.9, got %f", score)
	}
}

func TestChrF_EmptyStrings(t *testing.T) {
	e := NewChrFEvaluator()
	if score := e.Score("", ""); score != 0 {
		t.Errorf("empty/empty should score 0, got %f", score)
	}
	if score := e.Score("hello", ""); score != 0 {
		t.Errorf("empty reference should score 0, got %f", score)
	}
	if score := e.Score("", "hello"); score != 0 {
		t.Errorf("empty candidate should score 0, got %f", score)
	}
}

func TestChrF_MixedChineseEnglish(t *testing.T) {
	e := NewChrFEvaluator()
	// 中英文混合，完全相同应得 1.0
	score := e.Score("Hello 世界 hello", "Hello 世界 hello")
	if score < 0.99 {
		t.Errorf("identical mixed zh/en should score ~1.0, got %f", score)
	}
	// 中英文混合，部分匹配应得中间值
	score2 := e.Score("Hello 世界", "Hello 地球")
	if score2 <= 0 || score2 >= 1.0 {
		t.Errorf("partial mixed zh/en should score 0-1 (exclusive), got %f", score2)
	}
}

func TestChrF_ScoreBatch(t *testing.T) {
	e := NewChrFEvaluator()
	// 等长 batch
	score := e.ScoreBatch(
		[]string{"hello", "world"},
		[]string{"hello", "world"},
	)
	if score < 0.99 {
		t.Errorf("identical batch should score ~1.0, got %f", score)
	}
	// 空 batch
	if score := e.ScoreBatch(nil, nil); score != 0 {
		t.Errorf("empty batch should score 0, got %f", score)
	}
	// 不等长 batch（取较小者）
	score = e.ScoreBatch(
		[]string{"a", "b", "c"},
		[]string{"a", "b"},
	)
	if score < 0.99 {
		t.Errorf("first 2 identical should score ~1.0, got %f", score)
	}
}

func TestChrF_CustomParams(t *testing.T) {
	// 自定义参数不应 panic，且完全相同的字符串仍得 1.0
	e := &ChrFEvaluator{CharN: 3, WordN: 1, Beta: 1.0}
	score := e.Score("the quick brown fox", "the quick brown fox")
	if score < 0.99 {
		t.Errorf("identical with custom params should score ~1.0, got %f", score)
	}
}

func TestChrF_InvalidParamsFallback(t *testing.T) {
	// 0/负数参数应兜底为默认值，不 panic
	e := &ChrFEvaluator{CharN: 0, WordN: -1, Beta: 0}
	score := e.Score("hello", "hello")
	if score < 0.99 {
		t.Errorf("identical with invalid params (fallback) should score ~1.0, got %f", score)
	}
}

// ============================================================================
// LLMJudge 测试
// ============================================================================

// mockLLMService 模拟 LLM 服务，返回预设响应。
type mockLLMService struct {
	resp string
	err  error
}

func (m *mockLLMService) Generate(ctx context.Context, config any, prompt string) (string, error) {
	return m.resp, m.err
}

func TestLLMJudge_Disabled(t *testing.T) {
	j := NewDefaultLLMJudge(&mockLLMService{resp: "{}"})
	j.SetEnabled(false)
	_, err := j.Judge(context.Background(), JudgeRequest{
		Query: "q", Reference: "r", Candidate: "c", TargetLang: "en",
	})
	if err == nil {
		t.Errorf("disabled judge should return error")
	}
	if err.Error() != "llm judge disabled" {
		t.Errorf("disabled judge should return 'llm judge disabled', got %v", err)
	}
}

func TestLLMJudge_NoService(t *testing.T) {
	j := NewDefaultLLMJudge(nil)
	_, err := j.Judge(context.Background(), JudgeRequest{
		Query: "q", Reference: "r", Candidate: "c", TargetLang: "en",
	})
	if err == nil {
		t.Errorf("judge without service should return error")
	}
}

func TestLLMJudge_ParseJSON(t *testing.T) {
	// 纯 JSON
	j := NewDefaultLLMJudge(&mockLLMService{
		resp: `{"overall_score": 0.85, "dimension_scores": {"fluency": 0.9, "accuracy": 0.8}, "explanation": "good", "issues": ["minor typo"]}`,
	})
	result, err := j.Judge(context.Background(), JudgeRequest{
		Query: "q", Reference: "r", Candidate: "c", TargetLang: "en",
	})
	if err != nil {
		t.Fatalf("parse plain JSON failed: %v", err)
	}
	if result.OverallScore != 0.85 {
		t.Errorf("overall_score should be 0.85, got %f", result.OverallScore)
	}
	if len(result.Issues) != 1 {
		t.Errorf("issues should have 1 item, got %d", len(result.Issues))
	}
}

func TestLLMJudge_ParseMarkdownWrapped(t *testing.T) {
	// LLM 返回带 markdown 代码块的 JSON
	j := NewDefaultLLMJudge(&mockLLMService{
		resp: "```json\n{\"overall_score\": 0.7, \"dimension_scores\": {}, \"explanation\": \"\", \"issues\": []}\n```\nSome extra text.",
	})
	result, err := j.Judge(context.Background(), JudgeRequest{
		Query: "q", Reference: "r", Candidate: "c", TargetLang: "en",
	})
	if err != nil {
		t.Fatalf("parse markdown-wrapped JSON failed: %v", err)
	}
	if result.OverallScore != 0.7 {
		t.Errorf("overall_score should be 0.7, got %f", result.OverallScore)
	}
}

func TestLLMJudge_NormalizePercentScore(t *testing.T) {
	// 0-100 打分应归一化为 0-1
	j := NewDefaultLLMJudge(&mockLLMService{
		resp: `{"overall_score": 85, "dimension_scores": {}, "explanation": "", "issues": []}`,
	})
	result, err := j.Judge(context.Background(), JudgeRequest{
		Query: "q", Reference: "r", Candidate: "c", TargetLang: "en",
	})
	if err != nil {
		t.Fatalf("parse percent score failed: %v", err)
	}
	if result.OverallScore != 0.85 {
		t.Errorf("85 should normalize to 0.85, got %f", result.OverallScore)
	}
}

func TestLLMJudge_EmptyResponse(t *testing.T) {
	j := NewDefaultLLMJudge(&mockLLMService{resp: ""})
	_, err := j.Judge(context.Background(), JudgeRequest{
		Query: "q", Reference: "r", Candidate: "c", TargetLang: "en",
	})
	if err == nil {
		t.Errorf("empty response should return error")
	}
}

func TestLLMJudge_NoJSON(t *testing.T) {
	j := NewDefaultLLMJudge(&mockLLMService{resp: "no json here at all"})
	_, err := j.Judge(context.Background(), JudgeRequest{
		Query: "q", Reference: "r", Candidate: "c", TargetLang: "en",
	})
	if err == nil {
		t.Errorf("response without JSON should return error")
	}
}

func TestLLMJudge_GenerateError(t *testing.T) {
	j := NewDefaultLLMJudge(&mockLLMService{err: errors.New("network timeout")})
	_, err := j.Judge(context.Background(), JudgeRequest{
		Query: "q", Reference: "r", Candidate: "c", TargetLang: "en",
	})
	if err == nil {
		t.Errorf("LLM generate error should propagate")
	}
}

func TestExtractJSON_Nested(t *testing.T) {
	// 嵌套 JSON 应正确提取外层对象
	input := `prefix {"a": {"b": 1}, "c": "}"} suffix`
	out := extractJSON(input)
	if out == "" {
		t.Fatalf("extractJSON returned empty for nested JSON")
	}
	// 应提取到完整外层对象（包含嵌套大括号）
	if out != `{"a": {"b": 1}, "c": "}"}` {
		t.Errorf("extractJSON mismatch, got: %s", out)
	}
}

func TestExtractJSON_NoBrace(t *testing.T) {
	out := extractJSON("no braces here")
	if out != "" {
		t.Errorf("extractJSON should return empty when no brace, got %s", out)
	}
}

// ============================================================================
// Evaluator 测试
// ============================================================================

func TestEvaluator_EvaluateBatch(t *testing.T) {
	e := NewEvaluator(nil, nil)
	result, err := e.EvaluateBatch(
		[]string{"hello world", "foo bar"},
		[]string{"hello world", "foo baz"},
		"zh-en",
	)
	if err != nil {
		t.Fatalf("EvaluateBatch failed: %v", err)
	}
	if result.SampleCount != 2 {
		t.Errorf("SampleCount should be 2, got %d", result.SampleCount)
	}
	if result.LangPair != "zh-en" {
		t.Errorf("LangPair should be zh-en, got %s", result.LangPair)
	}
	if len(result.PerSample) != 2 {
		t.Errorf("PerSample should have 2 items, got %d", len(result.PerSample))
	}
	// 第一条完全相同 → 1.0
	if result.PerSample[0].ChrF < 0.99 {
		t.Errorf("first sample (identical) should score ~1.0, got %f", result.PerSample[0].ChrF)
	}
	// 平均分应介于 0 和 1 之间
	if result.ChrF <= 0 || result.ChrF > 1.0 {
		t.Errorf("avg ChrF should be (0,1], got %f", result.ChrF)
	}
}

func TestEvaluator_EvaluateBatch_LengthMismatch(t *testing.T) {
	e := NewEvaluator(nil, nil)
	_, err := e.EvaluateBatch(
		[]string{"a", "b"},
		[]string{"a"},
		"zh-en",
	)
	if err == nil {
		t.Errorf("length mismatch should return error")
	}
}

func TestEvaluator_EvaluateBatch_Empty(t *testing.T) {
	e := NewEvaluator(nil, nil)
	result, err := e.EvaluateBatch(nil, nil, "zh-en")
	if err != nil {
		t.Fatalf("empty batch should not error: %v", err)
	}
	if result.SampleCount != 0 {
		t.Errorf("empty batch SampleCount should be 0, got %d", result.SampleCount)
	}
}

func TestEvaluator_EvaluateSingle(t *testing.T) {
	e := NewEvaluator(nil, nil)
	score, err := e.EvaluateSingle("hello", "hello", "hi")
	if err != nil {
		t.Fatalf("EvaluateSingle failed: %v", err)
	}
	if score < 0.99 {
		t.Errorf("identical single should score ~1.0, got %f", score)
	}
}

func TestNewEvaluator_DefaultChrF(t *testing.T) {
	// chrF 为 nil 时应自动构造默认评估器
	e := NewEvaluator(nil, nil)
	if e.ChrF() == nil {
		t.Errorf("ChrF should not be nil after NewEvaluator(nil, nil)")
	}
	if e.Judge() != nil {
		t.Errorf("Judge should be nil when not provided")
	}
	// 验证可用
	score := e.ChrF().Score("a", "a")
	if score < 0.99 {
		t.Errorf("default ChrF should work, got %f", score)
	}
}
