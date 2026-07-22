package agent_runtime

import (
	"context"
	"strings"
	"time"
)

// ============================================================================
// Perception Stage 感知阶段实现
// ----------------------------------------------------------------------------
// 文档依据：方向4 阶段1 情绪感知 + 阶段2 意图识别
// 合并为单一阶段，编排器在 InferenceCycle 视为一个 stage
//
// 顺序：
//  1. SentimentAnalyzer.Analyze → SentimentScore
//  2. IntentRecognizer.Recognize → IntentResult
// ============================================================================

// DefaultPerceptionStage 默认感知阶段（基于规则 + 关键词）
//
// 设计原则：
//  - 纯规则 fallback，不依赖外部 LLM
//  - 真实生产可注入 SentimentAnalyzer / IntentRecognizer
//  - 即使没有 AI 服务，本阶段也能产出合理结果
type DefaultPerceptionStage struct {
	Sentiment SentimentAnalyzer
	Intent    IntentRecognizer
}

// NewDefaultPerceptionStage 构造默认感知阶段
func NewDefaultPerceptionStage() *DefaultPerceptionStage {
	return &DefaultPerceptionStage{
		Sentiment: NewKeywordSentimentAnalyzer(),
		Intent:    NewKeywordIntentRecognizer(),
	}
}

// NewDefaultPerceptionStageWith 自定义感知阶段
func NewDefaultPerceptionStageWith(s SentimentAnalyzer, i IntentRecognizer) *DefaultPerceptionStage {
	return &DefaultPerceptionStage{Sentiment: s, Intent: i}
}

// Name 阶段名
func (p *DefaultPerceptionStage) Name() string {
	return "perception"
}

// Execute 执行感知
func (p *DefaultPerceptionStage) Execute(ctx context.Context, ic *InferenceContext) StageResult {
	start := time.Now()

	// 1. 情绪分析
	ic.Sentiment = p.Sentiment.Analyze(ctx, ic.Payload.Content)

	// 2. 意图识别
	hint := map[string]string{
		"channel":    ic.Payload.ChannelType,
		"account_id": ic.Payload.AccountID,
	}
	if ic.AgentCtx != nil {
		hint["agent_code"] = ic.AgentCtx.AgentCode
	}
	ic.Intent = p.Intent.Recognize(ctx, ic.Payload.Content, hint)

	// 3. 记录决策
	ic.Stages = append(ic.Stages, StageDecision{
		Stage:    p.Name(),
		Action:   "analyze",
		Reason:   "sentiment=" + string(ic.Sentiment.Label) + " intent=" + string(ic.Intent.Primary),
		Duration: time.Since(start),
		Success:  true,
	})

	return ContinueResult()
}

// ============================================================================
// KeywordSentimentAnalyzer 基于关键词的情绪分析器
// ============================================================================

// KeywordSentimentAnalyzer 关键词情绪分析器
//
// 字典驱动，覆盖中英文常见情绪词
// 真实生产可替换为 LLM-based 或 BERT 模型
type KeywordSentimentAnalyzer struct {
	// 词 → 权重
	AngryWords    map[string]float64
	AnxiousWords  map[string]float64
	AppreciWords  map[string]float64
	ConfusedWords map[string]float64
	CalmWords     map[string]float64
}

// NewKeywordSentimentAnalyzer 构造关键词情绪分析器
func NewKeywordSentimentAnalyzer() *KeywordSentimentAnalyzer {
	return &KeywordSentimentAnalyzer{
		AngryWords: map[string]float64{
			"气死": 0.9, "生气": 0.8, "愤怒": 0.9, "投诉": 0.7, "差评": 0.7,
			"骗子": 0.95, "骗人": 0.95, "欺诈": 0.95, "无语": 0.6, "垃圾": 0.85,
			"退款": 0.5, "起诉": 0.95, "曝光": 0.85,
			"angry": 0.9, "furious": 0.95, "scam": 0.9, "fraud": 0.95, "stupid": 0.85,
		},
		AnxiousWords: map[string]float64{
			"急": 0.6, "着急": 0.7, "焦虑": 0.8, "担心": 0.7, "紧张": 0.6,
			"怎么办": 0.6, "会不会": 0.5, "求助": 0.6, "麻烦": 0.4,
			"urgent": 0.7, "worried": 0.7, "anxious": 0.8, "help": 0.5,
		},
		AppreciWords: map[string]float64{
			"谢谢": 0.7, "感谢": 0.8, "非常好": 0.85, "棒": 0.6, "满意": 0.8,
			"喜欢": 0.7, "赞": 0.7, "辛苦": 0.5,
			"thanks": 0.7, "thank you": 0.8, "great": 0.8, "awesome": 0.9, "perfect": 0.85,
		},
		ConfusedWords: map[string]float64{
			"不懂": 0.7, "不明白": 0.7, "啥": 0.4, "什么意思": 0.6, "怎么用": 0.5,
			"为什么": 0.4,
			"confused": 0.7, "what": 0.4, "how": 0.4, "why": 0.4,
		},
		CalmWords: map[string]float64{
			"你好": 0.5, "在吗": 0.4, "请问": 0.3, "ok": 0.4, "好的": 0.4,
		},
	}
}

// Analyze 分析情绪
func (k *KeywordSentimentAnalyzer) Analyze(ctx context.Context, text string) SentimentScore {
	text = strings.ToLower(text)
	scores := map[Sentiment]float64{
		SentimentCalm:     0.1,
		SentimentAnxious:  0,
		SentimentAngry:    0,
		SentimentAppreci:  0,
		SentimentConfused: 0,
	}

	// 累加每类关键词
	for word, weight := range k.AngryWords {
		if strings.Contains(text, word) {
			scores[SentimentAngry] += weight
		}
	}
	for word, weight := range k.AnxiousWords {
		if strings.Contains(text, word) {
			scores[SentimentAnxious] += weight
		}
	}
	for word, weight := range k.AppreciWords {
		if strings.Contains(text, word) {
			scores[SentimentAppreci] += weight
		}
	}
	for word, weight := range k.ConfusedWords {
		if strings.Contains(text, word) {
			scores[SentimentConfused] += weight
		}
	}
	for word, weight := range k.CalmWords {
		if strings.Contains(text, word) {
			scores[SentimentCalm] += weight
		}
	}

	// 找主情绪
	var primary Sentiment = SentimentUnknown
	maxScore := 0.0
	for label, score := range scores {
		if score > maxScore {
			primary = label
			maxScore = score
		}
	}

	// 归一化
	if maxScore > 1.0 {
		maxScore = 1.0
	}

	return SentimentScore{
		Label:  primary,
		Score:  maxScore,
		Detail: scores,
	}
}

// ============================================================================
// KeywordIntentRecognizer 基于关键词的意图识别器
// ============================================================================

// KeywordIntentRecognizer 关键词意图识别器
type KeywordIntentRecognizer struct {
	IntentKeywords map[Intent][]string
	SlotPatterns   map[string]*SlotPattern
}

// SlotPattern 槽位正则模式
type SlotPattern struct {
	Key string
	// 简化：词表匹配（真实场景用正则）
	Words []string
}

// NewKeywordIntentRecognizer 构造关键词意图识别器
func NewKeywordIntentRecognizer() *KeywordIntentRecognizer {
	return &KeywordIntentRecognizer{
		IntentKeywords: map[Intent][]string{
			IntentGreeting:       {"你好", "hi", "hello", "在吗", "hey", "您好"},
			IntentFarewell:       {"再见", "bye", "拜拜", "88", "回头聊", "goodbye"},
			IntentChitchat:       {"聊聊", "干嘛", "怎么样", "最近", "how are you"},
			IntentInquiry:        {"多少钱", "价格", "怎么卖", "报价", "费用", "price", "cost", "how much"},
			IntentOrderStatus:    {"订单", "查单", "我的订单", "物流", "快递", "order", "shipping", "delivery", "发货", "没收到", "没到"},
			IntentComplaint:      {"投诉", "不满", "差", "失望", "生气", "complain", "terrible"},
			IntentRefund:         {"退款", "退钱", "退货", "退订", "refund", "return"},
			IntentAfterSales:     {"售后", "维修", "保修", "换货", "warranty", "repair"},
			IntentSalesLead:      {"想买", "需要", "采购", "合作", "buy", "purchase", "need"},
			IntentHandoffToHuman: {"找人工", "人工客服", "转人工", "真人", "人工服务", "human agent", "real person", "speak to human", "转接", "人工"},
			IntentFAQ:            {"怎么用", "怎么操作", "如何使用", "怎么开通", "怎么开启", "怎么打开", "怎么登录", "教程", "how to", "how do i"},
		},
		SlotPatterns: map[string]*SlotPattern{
			"product": {Key: "product", Words: []string{"手机", "电脑", "衣服", "鞋子", "phone", "laptop"}},
		},
	}
}

// Recognize 识别意图
func (r *KeywordIntentRecognizer) Recognize(ctx context.Context, text string, hint map[string]string) IntentResult {
	textLower := strings.ToLower(text)

	matches := map[Intent]int{}
	for intent, kws := range r.IntentKeywords {
		for _, kw := range kws {
			if strings.Contains(textLower, strings.ToLower(kw)) {
				matches[intent]++
			}
		}
	}

	var primary Intent = IntentUnknown
	maxCount := 0
	var secondaries []Intent
	for intent, count := range matches {
		if count > maxCount {
			if primary != IntentUnknown {
				secondaries = append(secondaries, primary)
			}
			primary = intent
			maxCount = count
		} else if count > 0 {
			secondaries = append(secondaries, intent)
		}
	}

	// 简单 fallback：包含 "?" 默认为 inquiry
	if primary == IntentUnknown && (strings.Contains(text, "?") || strings.Contains(text, "？")) {
		primary = IntentInquiry
		maxCount = 1
	}

	// 槽位抽取
	tags := map[string]string{}
	for _, p := range r.SlotPatterns {
		for _, w := range p.Words {
			if strings.Contains(textLower, w) {
				tags[p.Key] = w
				break
			}
		}
	}

	// 置信度：粗略归一化
	score := 0.5
	if maxCount > 0 {
		score = 0.5 + float64(maxCount)*0.15
		if score > 0.99 {
			score = 0.99
		}
	}

	return IntentResult{
		Primary:   primary,
		Secondary: secondaries,
		Score:     score,
		Tags:      tags,
	}
}
