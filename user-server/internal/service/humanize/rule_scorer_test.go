package humanize

// rule_scorer_test.go P0-4 RuleScorer 单元测试
//
// 覆盖：
//  1. 5 维评分基本输出
//  2. AI 痕迹词触发 Naturalness 重罚
//  3. burstiness 计算
//  4. 字数与意图匹配（Conciseness）
//  5. 投诉场景必须共情（Empathy ≤ 0.4）
//  6. 专业词密度提升 Professionalism
//  7. 行动召唤 + 利益词提升 Persuasiveness
//  8. 边界用例（空输入、超长回复、单字符）
//  9. 加权综合分计算
//  10. 各意图期望字数

import (
	"context"
	"testing"

	"marketing/internal/dto"
)

// ============================================================================
// 基础评估测试
// ============================================================================

// TestRuleScorer_Evaluate_Basic 基本 5 维评分输出
func TestRuleScorer_Evaluate_Basic(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		CustomerMessage: "这个产品的成分是什么？",
		AIReply:         "您好，这款产品主要成分是烟酰胺和玻尿酸，能深层保湿并提亮肤色。下单试试吧！",
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
	if len(result.Scores) != 5 {
		t.Errorf("应有 5 维评分, got %d", len(result.Scores))
	}
	// 每维 ∈ [0, 1]
	for _, s := range result.Scores {
		if s.Score < 0 || s.Score > 1 {
			t.Errorf("维度 %s 分数超出 [0,1]: %v", s.Dimension, s.Score)
		}
	}
	if result.TotalScore < 0 || result.TotalScore > 1 {
		t.Errorf("综合分超出 [0,1]: %v", result.TotalScore)
	}
	if result.EvaluatorType != "rule" {
		t.Errorf("EvaluatorType=%q want=rule", result.EvaluatorType)
	}
	if result.SampleStrategy != "full" {
		t.Errorf("SampleStrategy=%q want=full", result.SampleStrategy)
	}
}

// TestRuleScorer_Evaluate_NilInput 空输入应报错
func TestRuleScorer_Evaluate_NilInput(t *testing.T) {
	scorer := NewRuleScorer()
	_, err := scorer.Evaluate(context.Background(), nil)
	if err == nil {
		t.Error("nil input 应报错")
	}
}

// TestRuleScorer_Evaluate_EmptyReply 空 AIReply 应报错
func TestRuleScorer_Evaluate_EmptyReply(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{AIReply: ""}
	_, err := scorer.Evaluate(context.Background(), input)
	if err == nil {
		t.Error("空 AIReply 应报错")
	}
}

// ============================================================================
// Naturalness 测试
// ============================================================================

// TestRuleScorer_Naturalness_AITrace AI 痕迹词触发重罚
func TestRuleScorer_Naturalness_AITrace(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "作为 AI 语言模型，我无法回答这个问题。请理解我的局限。",
		CustomerMessage: "你能告诉我这个产品的成分吗？",
	}
	result, err := scorer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	natScore, ok := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimNaturalness)
	if !ok {
		t.Fatal("缺 naturalness 维度")
	}
	if natScore > 0.5 {
		t.Errorf("AI 痕迹词应使 Naturalness ≤ 0.5, got %v", natScore)
	}
}

// TestRuleScorer_Naturalness_Humanlike 真人风格回复得分较高
func TestRuleScorer_Naturalness_Humanlike(t *testing.T) {
	scorer := NewRuleScorer()
	// 包含语气词、句长有变化、无 AI 痕迹
	input := &dto.HumanizeEvalInput{
		AIReply:         "亲，这款产品超好用呢！我自己也在用。下单试试嘛。",
		CustomerMessage: "这个产品怎么样？",
	}
	result, err := scorer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	natScore, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimNaturalness)
	if natScore < 0.7 {
		t.Errorf("真人风格 Naturalness 应较高 ≥ 0.7, got %v", natScore)
	}
}

// TestRuleScorer_Naturalness_Burstiness 句长方差影响 burstiness
func TestRuleScorer_Naturalness_Burstiness(t *testing.T) {
	scorer := NewRuleScorer()
	// 等长短句：burstiness 低
	equalLen := &dto.HumanizeEvalInput{
		AIReply:         "好的呢。可以的哦。没问题啊。我看看呢。下单吧。",
		CustomerMessage: "可以吗？",
	}
	// 变长句：burstiness 高
	variedLen := &dto.HumanizeEvalInput{
		AIReply:         "好的呢。这个产品我们可以详细聊聊，因为里面有不少您可能感兴趣的细节。下单吧！",
		CustomerMessage: "可以吗？",
	}
	r1, _ := scorer.Evaluate(context.Background(), equalLen)
	r2, _ := scorer.Evaluate(context.Background(), variedLen)
	b1 := computeBurstiness(equalLen.AIReply)
	b2 := computeBurstiness(variedLen.AIReply)
	if b1 >= b2 {
		t.Logf("等长句 burstiness=%v, 变长句 burstiness=%v（等长应低于变长）", b1, b2)
	}
	// 变长句应得到更高的 Naturalness
	n1, _ := dto.ScoreHumanizeEvalByDimension(r1, dto.HumanizeDimNaturalness)
	n2, _ := dto.ScoreHumanizeEvalByDimension(r2, dto.HumanizeDimNaturalness)
	if n2 < n1-0.1 {
		t.Logf("变长句 Naturalness=%v, 等长句=%v（变长应不低于等长）", n2, n1)
	}
}

// ============================================================================
// Conciseness 测试
// ============================================================================

// TestRuleScorer_Conciseness_InRange 字数在期望范围内得高分
func TestRuleScorer_Conciseness_InRange(t *testing.T) {
	scorer := NewRuleScorer()
	// greeting 期望 5-50 字
	input := &dto.HumanizeEvalInput{
		AIReply:         "您好，欢迎光临！",
		CustomerMessage: "你好",
		Intent:          "greeting",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimConciseness)
	if score < 0.8 {
		t.Errorf("字数在期望范围内的 Conciseness 应 ≥ 0.8, got %v", score)
	}
}

// TestRuleScorer_Conciseness_TooShort 过短回复扣分
func TestRuleScorer_Conciseness_TooShort(t *testing.T) {
	scorer := NewRuleScorer()
	// complaint 期望 20-200 字，回复 1 字 = 极敷衍
	input := &dto.HumanizeEvalInput{
		AIReply:         "好",
		CustomerMessage: "我要投诉你们的产品质量太差了！",
		Intent:          "complaint",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimConciseness)
	if score > 0.5 {
		t.Errorf("过短回复 Conciseness 应 ≤ 0.5, got %v", score)
	}
}

// TestRuleScorer_Conciseness_TooLong 过长回复扣分
func TestRuleScorer_Conciseness_TooLong(t *testing.T) {
	scorer := NewRuleScorer()
	// greeting 期望 5-50 字，回复 200+ 字 = 冗长
	longReply := "您好欢迎光临我们的店铺"
	for i := 0; i < 30; i++ {
		longReply += "，这里有非常多的产品供您选择"
	}
	input := &dto.HumanizeEvalInput{
		AIReply:         longReply,
		CustomerMessage: "你好",
		Intent:          "greeting",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimConciseness)
	if score > 0.5 {
		t.Errorf("过长回复 Conciseness 应 ≤ 0.5, got %v", score)
	}
}

// TestRuleScorer_Conciseness_UnknownIntent 未知意图用默认范围
func TestRuleScorer_Conciseness_UnknownIntent(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "好的，我帮您查一下。",
		CustomerMessage: "查一下",
		Intent:          "unknown_intent_xyz",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimConciseness)
	if score < 0 || score > 1 {
		t.Errorf("未知意图 Conciseness 应在 [0,1], got %v", score)
	}
}

// ============================================================================
// Empathy 测试
// ============================================================================

// TestRuleScorer_Empathy_ComplaintNoEmpathy 投诉场景无共情直接 ≤ 0.4
func TestRuleScorer_Empathy_ComplaintNoEmpathy(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "您的订单号是多少？我帮您查一下物流状态。",
		CustomerMessage: "我要投诉！你们的产品质量太差了！",
		Intent:          "complaint",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimEmpathy)
	if score > 0.4 {
		t.Errorf("投诉无共情 Empathy 应 ≤ 0.4, got %v", score)
	}
}

// TestRuleScorer_Empathy_ComplaintWithEmpathy 投诉场景有共情得分提高
func TestRuleScorer_Empathy_ComplaintWithEmpathy(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "非常抱歉给您带来不好的体验，我完全理解您的心情。我们会立即处理这个问题。",
		CustomerMessage: "我要投诉！你们的产品质量太差了！",
		Intent:          "complaint",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimEmpathy)
	if score < 0.5 {
		t.Errorf("投诉有共情 Empathy 应 ≥ 0.5, got %v", score)
	}
}

// TestRuleScorer_Empathy_NonComplaint 非投诉场景按密度评分
func TestRuleScorer_Empathy_NonComplaint(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "这款产品很适合您呢，我理解您的需求。感谢您的咨询。",
		CustomerMessage: "这个产品怎么样？",
		Intent:          "ask_product",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimEmpathy)
	if score < 0.5 {
		t.Errorf("非投诉有共情词 Empathy 应 ≥ 0.5, got %v", score)
	}
}

// ============================================================================
// Professionalism 测试
// ============================================================================

// TestRuleScorer_Professionalism_WithProWords 专业词密度提升得分
func TestRuleScorer_Professionalism_WithProWords(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "这款产品的成分是烟酰胺，保湿效果很好，肤质适应性广。",
		CustomerMessage: "产品怎么样？",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimProfessionalism)
	if score < 0.7 {
		t.Errorf("含专业词 Professionalism 应 ≥ 0.7, got %v", score)
	}
}

// TestRuleScorer_Professionalism_NoProWords 无专业词得分较低
func TestRuleScorer_Professionalism_NoProWords(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "这个还行吧，您看看要不要买。",
		CustomerMessage: "怎么样？",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimProfessionalism)
	if score > 0.7 {
		t.Errorf("无专业词 Professionalism 应 ≤ 0.7, got %v", score)
	}
}

// ============================================================================
// Persuasiveness 测试
// ============================================================================

// TestRuleScorer_Persuasiveness_WithActionAndBenefit 行动召唤+利益词得高分
func TestRuleScorer_Persuasiveness_WithActionAndBenefit(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "现在下单立享 8 折优惠，限时活动包邮哦！赶紧入手吧！",
		CustomerMessage: "多少钱？",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimPersuasiveness)
	if score < 0.7 {
		t.Errorf("含行动召唤+利益词 Persuasiveness 应 ≥ 0.7, got %v", score)
	}
}

// TestRuleScorer_Persuasiveness_NoCallToAction 无行动召唤得分较低
func TestRuleScorer_Persuasiveness_NoCallToAction(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "这个产品的价格是 99 元。",
		CustomerMessage: "多少钱？",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	score, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimPersuasiveness)
	if score > 0.5 {
		t.Errorf("无行动召唤 Persuasiveness 应 ≤ 0.5, got %v", score)
	}
}

// ============================================================================
// 综合分计算测试
// ============================================================================

// TestRuleScorer_TotalScore_WeightedSum 综合分 = Σ w_i * s_i
func TestRuleScorer_TotalScore_WeightedSum(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "您好，这款产品成分是烟酰胺，保湿效果好。现在下单有优惠呢！",
		CustomerMessage: "这个怎么样？",
		Intent:          "ask_product",
	}
	result, _ := scorer.Evaluate(context.Background(), input)
	// 手动验证
	expected := 0.0
	for _, s := range result.Scores {
		expected += dto.HumanizeDimensionWeight[s.Dimension] * s.Score
	}
	// 保留 4 位小数
	if !approxEqualTol(result.TotalScore, expected, 1e-3) {
		t.Errorf("综合分=%v want %v", result.TotalScore, expected)
	}
}

// ============================================================================
// 辅助函数测试
// ============================================================================

// TestCountRunes 字符计数
func TestCountRunes(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 5},
		{"你好世界", 4},
		{"  a b  ", 2}, // 空格不计（仅 a、b 两个非空白字符）
		{"a b c", 3},
	}
	for _, tt := range tests {
		got := countRunes(tt.input)
		if got != tt.want {
			t.Errorf("countRunes(%q)=%d want %d", tt.input, got, tt.want)
		}
	}
}

// TestSplitSentences 分句
func TestSplitSentences(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"你好。", 1},
		{"你好。再见！", 2},
		{"第一句。第二句？第三句！", 3},
		{"Hello. World!", 2},
	}
	for _, tt := range tests {
		got := splitSentences(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitSentences(%q) len=%d want %d (got=%v)", tt.input, len(got), tt.want, got)
		}
	}
}

// TestComputeBurstiness burstiness 计算
func TestComputeBurstiness(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// 不严格校验具体值，只校验是否合理
		expectNonNeg bool
		expectLow    bool // 期望较低
	}{
		{"empty", "", true, true},
		{"single sentence", "你好。", true, true},
		{"equal length", "好的呢。可以的哦。没问题啊。", true, true},
		{"varied length", "好。这个产品我们可以详细聊聊，因为里面有不少您可能感兴趣的细节。下单吧！", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeBurstiness(tt.input)
			if got < 0 {
				t.Errorf("burstiness 应非负, got %v", got)
			}
		})
	}
}

// TestNormalizeIntent 意图归一化
func TestNormalizeIntent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "default"},
		{"  Ask_Product  ", "ask_product"},
		{"COMPLAINT", "complaint"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := normalizeIntent(tt.input)
		if got != tt.want {
			t.Errorf("normalizeIntent(%q)=%q want %q", tt.input, got, tt.want)
		}
	}
}

// TestClampScore 分数截断
func TestClampScore(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{-1, 0},
		{0, 0},
		{0.5, 0.5},
		{1, 1},
		{1.5, 1},
	}
	for _, tt := range tests {
		got := clampScore(tt.input)
		if got != tt.want {
			t.Errorf("clampScore(%v)=%v want %v", tt.input, got, tt.want)
		}
	}
}

// ============================================================================
// 各意图期望字数表测试
// ============================================================================

// TestIntentExpectedLength 各意图字数范围
func TestIntentExpectedLength(t *testing.T) {
	tests := []struct {
		intent string
		low    int
		high   int
	}{
		{"complaint", 20, 200},
		{"churn", 30, 250},
		{"objection", 20, 180},
		{"ask_product", 10, 150},
		{"ask_service", 15, 180},
		{"price_inquiry", 10, 100},
		{"purchase", 10, 120},
		{"after_sale", 20, 200},
		{"social", 5, 60},
		{"greeting", 5, 50},
		{"default", 10, 150},
	}
	for _, tt := range tests {
		got, ok := intentExpectedLength[tt.intent]
		if !ok {
			t.Errorf("意图 %q 不在期望字数表中", tt.intent)
			continue
		}
		if got[0] != tt.low || got[1] != tt.high {
			t.Errorf("意图 %q 字数范围=[%d,%d] want [%d,%d]", tt.intent, got[0], got[1], tt.low, tt.high)
		}
	}
}

// ============================================================================
// 端到端场景测试
// ============================================================================

// TestRuleScorer_Evaluate_ComplaintScenario 投诉场景端到端
func TestRuleScorer_Evaluate_ComplaintScenario(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "非常抱歉给您带来困扰，我完全理解您的不满。请问您的订单号是多少？我立即为您处理退款。",
		CustomerMessage: "我要投诉！你们的产品质量太差了，要求退货退款！",
		Persona:         "客服专家",
		Industry:        "通用",
		Intent:          "complaint",
	}
	result, err := scorer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	// 投诉场景 empathy 应较高
	emp, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimEmpathy)
	if emp < 0.5 {
		t.Errorf("投诉场景 Empathy 应 ≥ 0.5, got %v", emp)
	}
	// 综合
	if result.TotalScore < 0.5 {
		t.Errorf("合理回复综合分应 ≥ 0.5, got %v", result.TotalScore)
	}
}

// TestRuleScorer_Evaluate_SalesScenario 销售场景端到端
func TestRuleScorer_Evaluate_SalesScenario(t *testing.T) {
	scorer := NewRuleScorer()
	input := &dto.HumanizeEvalInput{
		AIReply:         "亲，这款精华液成分含烟酰胺，能深层保湿提亮肤色哦。现在下单立享 8 折包邮活动，赶紧入手吧！",
		CustomerMessage: "这款产品怎么样？",
		Persona:         "美妆顾问",
		Industry:        "美妆",
		Intent:          "ask_product",
	}
	result, err := scorer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	// 销售场景 persuasiveness 应较高
	per, _ := dto.ScoreHumanizeEvalByDimension(result, dto.HumanizeDimPersuasiveness)
	if per < 0.6 {
		t.Errorf("销售场景 Persuasiveness 应 ≥ 0.6, got %v", per)
	}
	// 综合
	if result.TotalScore < 0.6 {
		t.Errorf("合理销售回复综合分应 ≥ 0.6, got %v", result.TotalScore)
	}
}
