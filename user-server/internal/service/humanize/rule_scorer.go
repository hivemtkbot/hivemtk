package humanize

// rule_scorer.go P0-4 规则评估器（RuleScorerImpl）
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十六章 §16.4.2
//
// 5 维评分体系（纯 Go，无 LLM 调用，<1ms）：
//  1. Naturalness:     1 - AI 痕迹词惩罚 - burstiness 惩罚
//  2. Conciseness:     字数与意图匹配（投诉/价格/购买等不同意图期望字数不同）
//  3. Empathy:         投诉场景必须共情；其他场景按共情词密度
//  4. Professionalism: 行业专业词密度 + 人设一致性
//  5. Persuasiveness:  行动召唤 + 利益词密度

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"

	"marketing/internal/dto"
)

// ============================================================================
// 词典（行业无关通用词典，可后续扩展按行业分词典）
// ============================================================================

// aiTraces AI 痕迹词（命中则 Naturalness 重罚）
var aiTraces = []string{
	"作为 AI", "作为人工智能", "我是一个 AI", "我是一个人工智能",
	"作为语言模型", "作为大语言模型", "我无法", "我不能",
	"作为助手", "我是一个助手", "我是一个智能助手",
	"我没有情感", "我没有真正的情感", "我没有实际情感",
	"我的训练数据", "我的训练数据中", "我被训练",
	"as an ai", "as a language model", "i am an ai",
	"i cannot", "i can't help with that",
	"according to my training", "my training data",
}

// particles 语气词（口语化标志，提升 Naturalness）
var particles = []string{
	"啊", "呢", "啦", "呀", "嘛", "哦", "哎", "嗨",
	"哟", "嘎", "咯", "喔", "嗷", "吼",
}

// empathyWords 共情词
//
// 注意：仅包含明确的情感共情词，不包含"帮您"/"为您"等功能性词汇
// （功能性词汇在查询/操作场景会被误命中）
var empathyWords = []string{
	"理解", "明白", "抱歉", "对不起", "恭喜", "放心",
	"感同身受", "确实", "没错", "遗憾", "欣慰",
	"辛苦", "陪伴", "倾听", "懂你", "知道你",
	"谅解", "体谅", "包容", "宽慰",
}

// professionalWords 行业专业词（多行业混合，后续可按 industry 拆分）
var professionalWords = []string{
	// 美妆
	"成分", "肤质", "保湿", "补水", "美白", "抗老", "维C", "烟酰胺",
	"玻尿酸", "修护", "防晒", "清洁", "控油",
	// 3C
	"续航", "性能", "处理器", "内存", "屏幕", "像素", "快充", "电池",
	"参数", "配置", "型号", "保修", "售后",
	// 服饰
	"面料", "版型", "尺码", "剪裁", "质地", "透气", "垂坠",
	// 通用
	"专业", "认证", "权威", "标准", "工艺",
}

// salesWords 销售推进词
var salesWords = []string{
	"下单", "拍下", "入手", "试试", "咨询", "联系", "回复",
	"立即", "马上", "现在", "赶紧", "快",
	"专属", "定制", "推荐", "适合", "选择",
}

// actionCalls 行动召唤词
var actionCalls = []string{
	"下单", "拍下", "入手", "试试", "咨询", "联系", "回复",
	"点击", "领取", "抢购", "预约", "购买", "加入",
}

// benefitWords 利益词
var benefitWords = []string{
	"优惠", "折扣", "活动", "省", "划算", "限时", "包邮",
	"赠送", "赠品", "满减", "立减", "立省", "免邮",
}

// complaintKeywords 投诉关键词（触发共情强制要求）
var complaintKeywords = []string{
	"投诉", "差评", "退款", "退货", "举报", "骗子", "假货",
	"维权", "315", "12315", "工商", "客服", "经理",
	"不满", "失望", "气愤", "愤怒", "恶心", "糟糕",
}

// intentExpectedLength 各意图期望字数范围（用于 Conciseness 评分）
var intentExpectedLength = map[string][2]int{
	"complaint":     {20, 200}, // 投诉需要充分共情，字数较多
	"churn":         {30, 250}, // 流失挽回需要详细解释
	"objection":     {20, 180}, // 异议处理
	"ask_product":   {10, 150}, // 产品咨询
	"ask_service":   {15, 180}, // 服务咨询
	"price_inquiry": {10, 100}, // 价格咨询简洁
	"purchase":      {10, 120}, // 购买引导
	"after_sale":    {20, 200}, // 售后
	"social":        {5, 60},   // 闲聊极简
	"greeting":      {5, 50},   // 问候极简
	"default":       {10, 150}, // 默认
}

// RuleScorerImpl 规则评估器实现
//
// 全量执行 <1ms，无 LLM 调用
type RuleScorerImpl struct {
	// 可扩展字段：按行业加载不同词典
}

// NewRuleScorer 构造规则评估器
func NewRuleScorer() *RuleScorerImpl {
	return &RuleScorerImpl{}
}

// Evaluate 单次规则评估
func (s *RuleScorerImpl) Evaluate(ctx context.Context, input *dto.HumanizeEvalInput) (*dto.HumanizeEvalResult, error) {
	if input == nil || input.AIReply == "" {
		return nil, fmt.Errorf("%w: input or ai_reply empty", ErrInvalidInput)
	}

	reply := input.AIReply
	intent := normalizeIntent(input.Intent)

	scores := []dto.HumanizeDimensionScore{
		{Dimension: dto.HumanizeDimNaturalness, Score: s.computeNaturalness(reply)},
		{Dimension: dto.HumanizeDimConciseness, Score: s.computeConciseness(reply, intent)},
		{Dimension: dto.HumanizeDimEmpathy, Score: s.computeEmpathy(reply, input.CustomerMessage)},
		{Dimension: dto.HumanizeDimProfessionalism, Score: s.computeProfessionalism(reply)},
		{Dimension: dto.HumanizeDimPersuasiveness, Score: s.computePersuasiveness(reply)},
	}

	total := computeHumanizeWeightedScore(scores)
	// 保留 4 位小数
	total = math.Round(total*10000) / 10000

	return &dto.HumanizeEvalResult{
		Scores:         scores,
		TotalScore:     total,
		EvaluatorType:  "rule",
		SampleStrategy: "full",
		Input:          input,
		CalculatedAt:   time.Now(),
	}, nil
}

// ============================================================================
// 5 维评分实现
// ============================================================================

// computeNaturalness 自然度评分
//
// 评分公式：
//
//	base = 0.85
//	- AI 痕迹词每命中一次扣 0.30（最多扣到 0.0）
//	- burstiness < 0.3 扣 0.15（句长过于均匀 = 机械感）
//	- 语气词密度奖励：每 50 字 1 个语气词 +0.05（最多 +0.10）
func (s *RuleScorerImpl) computeNaturalness(reply string) float64 {
	score := 0.85
	lowered := strings.ToLower(reply)
	for _, trace := range aiTraces {
		if strings.Contains(reply, trace) || strings.Contains(lowered, strings.ToLower(trace)) {
			score -= 0.30
		}
	}
	burstiness := computeBurstiness(reply)
	if burstiness < 0.3 {
		score -= 0.15
	}
	// 语气词密度奖励
	runeCount := countRunes(reply)
	if runeCount > 0 {
		particleCount := 0
		for _, p := range particles {
			particleCount += strings.Count(reply, p)
		}
		density := float64(particleCount) / float64(runeCount) * 50.0
		bonus := math.Min(density*0.05, 0.10)
		score += bonus
	}
	return clampScore(score)
}

// computeConciseness 简洁性评分
//
// 评分公式：
//
//	字数在期望范围内 → 0.85-1.00
//	字数低于下限的 50% → 扣分（敷衍）
//	字数超过上限的 200% → 扣分（冗长）
func (s *RuleScorerImpl) computeConciseness(reply, intent string) float64 {
	runeCount := countRunes(reply)
	expected, ok := intentExpectedLength[intent]
	if !ok {
		expected = intentExpectedLength["default"]
	}
	low, high := expected[0], expected[1]

	var score float64
	switch {
	case runeCount >= low && runeCount <= high:
		// 范围内：0.85-1.00（按位置插值）
		if high == low {
			score = 0.95
		} else {
			ratio := float64(runeCount-low) / float64(high-low)
			score = 0.85 + 0.15*(1-math.Abs(ratio-0.5)*2) // 中间值最优
		}
	case runeCount < low:
		// 太短：按比例扣分
		ratio := float64(runeCount) / float64(low)
		if ratio < 0.5 {
			score = 0.30 // 极敷衍
		} else {
			score = 0.30 + 0.55*(ratio-0.5)*2 // 0.30 - 0.85
		}
	default:
		// 太长：按比例扣分
		ratio := float64(runeCount) / float64(high)
		if ratio > 2.0 {
			score = 0.30
		} else {
			score = 0.85 - 0.55*(ratio-1.0) // 0.85 - 0.30
		}
	}
	return clampScore(score)
}

// computeEmpathy 共情度评分
//
// 评分规则：
//   - 投诉场景：必须有共情词，否则直接 ≤ 0.40（A-04 标准）
//   - 非投诉场景：按共情词密度评分
func (s *RuleScorerImpl) computeEmpathy(reply, customerMessage string) float64 {
	empathyCount := 0
	for _, w := range empathyWords {
		empathyCount += strings.Count(reply, w)
	}
	runeCount := countRunes(reply)
	if runeCount == 0 {
		return 0
	}
	density := float64(empathyCount) / float64(runeCount) * 100.0

	// 投诉场景强制共情
	isComplaint := false
	lowered := strings.ToLower(customerMessage)
	for _, kw := range complaintKeywords {
		if strings.Contains(customerMessage, kw) || strings.Contains(lowered, strings.ToLower(kw)) {
			isComplaint = true
			break
		}
	}
	if isComplaint {
		if empathyCount == 0 {
			return 0.30 // 投诉无共情直接低分
		}
		// 投诉场景共情权重更高
		score := 0.40 + math.Min(density*0.20, 0.60)
		return clampScore(score)
	}
	// 非投诉场景：按密度评分
	score := 0.50 + math.Min(density*0.15, 0.50)
	return clampScore(score)
}

// computeProfessionalism 专业度评分
//
// 评分公式：
//
//	base = 0.50
//	+ 专业词密度 * 0.30（每 50 字 1 个专业词加 0.10，最多 +0.30）
//	+ 销售推进词密度 * 0.20
func (s *RuleScorerImpl) computeProfessionalism(reply string) float64 {
	runeCount := countRunes(reply)
	if runeCount == 0 {
		return 0
	}
	profCount := 0
	for _, w := range professionalWords {
		profCount += strings.Count(reply, w)
	}
	salesCount := 0
	for _, w := range salesWords {
		salesCount += strings.Count(reply, w)
	}
	profDensity := float64(profCount) / float64(runeCount) * 50.0
	salesDensity := float64(salesCount) / float64(runeCount) * 50.0

	score := 0.50 + math.Min(profDensity*0.10, 0.30) + math.Min(salesDensity*0.05, 0.20)
	return clampScore(score)
}

// computePersuasiveness 说服力评分
//
// 评分公式：
//
//	base = 0.40
//	+ 行动召唤命中 +0.30
//	+ 利益词密度 * 0.30
func (s *RuleScorerImpl) computePersuasiveness(reply string) float64 {
	runeCount := countRunes(reply)
	if runeCount == 0 {
		return 0
	}
	actionCount := 0
	for _, w := range actionCalls {
		if strings.Contains(reply, w) {
			actionCount++
		}
	}
	benefitCount := 0
	for _, w := range benefitWords {
		benefitCount += strings.Count(reply, w)
	}
	benefitDensity := float64(benefitCount) / float64(runeCount) * 50.0

	score := 0.40
	if actionCount > 0 {
		score += math.Min(float64(actionCount)*0.15, 0.30)
	}
	score += math.Min(benefitDensity*0.10, 0.30)
	return clampScore(score)
}

// ============================================================================
// 辅助函数
// ============================================================================

// normalizeIntent 归一化意图标签
func normalizeIntent(intent string) string {
	intent = strings.TrimSpace(strings.ToLower(intent))
	if intent == "" {
		return "default"
	}
	return intent
}

// countRunes 统计字符数（不含空白）
func countRunes(s string) int {
	count := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			count++
		}
	}
	return count
}

// splitSentences 分句（按。！？.!? 换行）
var sentenceEndRegex = regexp.MustCompile(`[。！？.!?!\n]+`)

func splitSentences(s string) []string {
	if s == "" {
		return nil
	}
	parts := sentenceEndRegex.Split(s, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// computeBurstiness 计算 burstiness（句长方差/均值）
//
// 业界参考：AI 文本 burstiness < 人类文本（A-07 标准）
// 公式：variance(sentence_lengths) / mean(sentence_lengths)
func computeBurstiness(s string) float64 {
	sentences := splitSentences(s)
	if len(sentences) < 2 {
		return 0
	}
	lengths := make([]float64, len(sentences))
	sum := 0.0
	for i, sent := range sentences {
		lengths[i] = float64(countRunes(sent))
		sum += lengths[i]
	}
	mean := sum / float64(len(sentences))
	if mean == 0 {
		return 0
	}
	variance := 0.0
	for _, l := range lengths {
		diff := l - mean
		variance += diff * diff
	}
	variance /= float64(len(sentences))
	return math.Sqrt(variance) / mean
}
