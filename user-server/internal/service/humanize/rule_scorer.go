package humanize

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"

	"hivemtk-user/internal/dto"
)

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

var particles = []string{
	"啊", "呢", "啦", "呀", "嘛", "哦", "哎", "嗨",
	"哟", "嘎", "咯", "喔", "嗷", "吼",
}

var empathyWords = []string{
	"理解", "明白", "抱歉", "对不起", "恭喜", "放心",
	"感同身受", "确实", "没错", "遗憾", "欣慰",
	"辛苦", "陪伴", "倾听", "懂你", "知道你",
	"谅解", "体谅", "包容", "宽慰",
}

var professionalWords = []string{
	"成分", "肤质", "保湿", "补水", "美白", "抗老", "维C", "烟酰胺",
	"玻尿酸", "修护", "防晒", "清洁", "控油",
	"续航", "性能", "处理器", "内存", "屏幕", "像素", "快充", "电池",
	"参数", "配置", "型号", "保修", "售后",
	"面料", "版型", "尺码", "剪裁", "质地", "透气", "垂坠",
	"专业", "认证", "权威", "标准", "工艺",
}

var salesWords = []string{
	"下单", "拍下", "入手", "试试", "咨询", "联系", "回复",
	"立即", "马上", "现在", "赶紧", "快",
	"专属", "定制", "推荐", "适合", "选择",
}

var actionCalls = []string{
	"下单", "拍下", "入手", "试试", "咨询", "联系", "回复",
	"点击", "领取", "抢购", "预约", "购买", "加入",
}

var benefitWords = []string{
	"优惠", "折扣", "活动", "省", "划算", "限时", "包邮",
	"赠送", "赠品", "满减", "立减", "立省", "免邮",
}

var complaintKeywords = []string{
	"投诉", "差评", "退款", "退货", "举报", "骗子", "假货",
	"维权", "315", "12315", "工商", "客服", "经理",
	"不满", "失望", "气愤", "愤怒", "恶心", "糟糕",
}

var intentExpectedLength = map[string][2]int{
	"complaint":     {20, 200},
	"churn":         {30, 250},
	"objection":     {20, 180},
	"ask_product":   {10, 150},
	"ask_service":   {15, 180},
	"price_inquiry": {10, 100},
	"purchase":      {10, 120},
	"after_sale":    {20, 200},
	"social":        {5, 60},
	"greeting":      {5, 50},
	"default":       {10, 150},
}

// RuleScorerImpl 规则评估器实现
//
// 全量执行 <1ms，无 LLM 调用
type RuleScorerImpl struct {
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
		if high == low {
			score = 0.95
		} else {
			ratio := float64(runeCount-low) / float64(high-low)
			score = 0.85 + 0.15*(1-math.Abs(ratio-0.5)*2)
		}
	case runeCount < low:
		ratio := float64(runeCount) / float64(low)
		if ratio < 0.5 {
			score = 0.30
		} else {
			score = 0.30 + 0.55*(ratio-0.5)*2
		}
	default:
		ratio := float64(runeCount) / float64(high)
		if ratio > 2.0 {
			score = 0.30
		} else {
			score = 0.85 - 0.55*(ratio-1.0)
		}
	}
	return clampScore(score)
}

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
			return 0.30
		}
		score := 0.40 + math.Min(density*0.20, 0.60)
		return clampScore(score)
	}
	score := 0.50 + math.Min(density*0.15, 0.50)
	return clampScore(score)
}

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

func normalizeIntent(intent string) string {
	intent = strings.TrimSpace(strings.ToLower(intent))
	if intent == "" {
		return "default"
	}
	return intent
}

func countRunes(s string) int {
	count := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			count++
		}
	}
	return count
}

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
