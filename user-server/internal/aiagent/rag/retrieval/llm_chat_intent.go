package ragretrieval


import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/aiagent/llm"
)

// KeyIntentType 7 个核心精细意图类型
type KeyIntentType string

const (
	KeyIntentPriceObjection      KeyIntentType = "price_objection"      
	KeyIntentQualityObjection    KeyIntentType = "quality_objection"    
	KeyIntentPurchaseIntent      KeyIntentType = "purchase_intent"      
	KeyIntentTrustObjection      KeyIntentType = "trust_objection"      
	KeyIntentCompetitorObjection KeyIntentType = "competitor_objection" 
	KeyIntentDiscountRequest     KeyIntentType = "discount_request"     
	KeyIntentRefundRequest       KeyIntentType = "refund_request"       
)

// KeyIntentDescription 7 子类中文名映射
var KeyIntentDescription = map[KeyIntentType]string{
	KeyIntentPriceObjection:      "价格异议（客户嫌贵）",
	KeyIntentQualityObjection:    "质量异议（产品/服务质量不满）",
	KeyIntentPurchaseIntent:      "购买意向（准备下单）",
	KeyIntentTrustObjection:      "信任异议（怀疑品牌/服务）",
	KeyIntentCompetitorObjection: "竞品异议（对比/提到别家）",
	KeyIntentDiscountRequest:     "折扣请求（索要优惠）",
	KeyIntentRefundRequest:       "退款请求（要求退货）",
}

// AllKeyIntents 全部 7 个核心意图（用于遍历）
var AllKeyIntents = []KeyIntentType{
	KeyIntentPriceObjection,
	KeyIntentQualityObjection,
	KeyIntentPurchaseIntent,
	KeyIntentTrustObjection,
	KeyIntentCompetitorObjection,
	KeyIntentDiscountRequest,
	KeyIntentRefundRequest,
}

// KeyIntentResult 精细意图识别结果
type KeyIntentResult struct {
	Intent KeyIntentType `json:"intent"`
	Confidence float64 `json:"confidence"`
	Evidence string `json:"evidence,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
	Method string `json:"method"`
	LatencyMs int `json:"latency_ms,omitempty"`
}

// keyIntentKeywords 7 子类规则关键词（用于快速规则匹配）
var keyIntentKeywords = map[KeyIntentType][]string{
	KeyIntentPriceObjection:      {"太贵", "价格高", "有点高", "贵了", "不划算", "不值", "价格有点高", "价格贵"},
	KeyIntentQualityObjection:    {"质量差", "假货", "次品", "质量不行", "做工差", "不好用", "质量有问题"},
	KeyIntentPurchaseIntent:      {"怎么买", "怎么付款", "下单", "购买", "要了", "买一个", "来一个", "付款方式"},
	KeyIntentTrustObjection:      {"不放心", "骗人", "靠谱吗", "是真的吗", "信不过", "骗子", "跑路"},
	KeyIntentCompetitorObjection: {"别家", "竞品", "其他品牌", "友商", "比别家", "他家用"},
	KeyIntentDiscountRequest:     {"优惠", "折扣", "便宜点", "减免", "满减", "活动", "促销", "优惠券"},
	KeyIntentRefundRequest:       {"退款", "退货", "退钱", "退换", "退订", "要退", "能退"},
}

// ClassifyKeyIntent 精细意图识别入口
//
// 流程：
//  1. 规则匹配（快速、零 LLM 调用）：返回 KeyIntentResult（method=rule）
//  2. 规则未命中：调用 LLM 识别（method=llm）；chat 为 nil 时直接返回 no_match
//  3. chat 调用失败时：返回 rule 降级结果（method=rule, confidence=0.4）
//
// chat 必传 LLMChatClient（业务层注入）；nil 时仅走规则匹配。
func ClassifyKeyIntent(ctx context.Context, text string, chat LLMChatClient) KeyIntentResult {
	if strings.TrimSpace(text) == "" {
		return KeyIntentResult{Method: "rule", Confidence: 0.0}
	}
	if r := matchKeyIntentByRule(text); r != nil {
		return *r
	}
	if chat == nil {
		return KeyIntentResult{Method: "rule", Confidence: 0.4}
	}
	llmResult, err := classifyKeyIntentByLLM(ctx, text, chat)
	if err != nil || llmResult == nil {
		return KeyIntentResult{Method: "rule", Confidence: 0.4}
	}
	return *llmResult
}

// IsKeyIntent 判定文本是否命中指定核心意图
// 规则匹配即可，命中返回 true；规则未命中且 chat 非 nil 时调用 LLM 二次判定
func IsKeyIntent(ctx context.Context, text string, target KeyIntentType, chat LLMChatClient) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	if r := matchKeyIntentByRule(text); r != nil {
		return r.Intent == target
	}
	if chat == nil {
		return false
	}
	res, err := classifyKeyIntentByLLM(ctx, text, chat)
	if err != nil || res == nil {
		return false
	}
	return res.Intent == target
}

// ExtractKeyIntents 批量识别多条消息的核心意图
// 返回每条消息的 KeyIntentResult（顺序与输入一致）
func ExtractKeyIntents(ctx context.Context, texts []string, chat LLMChatClient) []KeyIntentResult {
	out := make([]KeyIntentResult, len(texts))
	for i, t := range texts {
		out[i] = ClassifyKeyIntent(ctx, t, chat)
	}
	return out
}

// matchKeyIntentByRule 规则匹配（仅基于关键词）
// 返回得分最高且 > 0 的意图
func matchKeyIntentByRule(text string) *KeyIntentResult {
	bestIntent := KeyIntentType("")
	bestScore := 0
	bestEvidence := ""
	for intent, kws := range keyIntentKeywords {
		score := 0
		var matched string
		for _, kw := range kws {
			if strings.Contains(text, kw) {
				score += len(kw)
				if matched == "" || len(kw) > len(matched) {
					matched = kw
				}
			}
		}
		if score > bestScore {
			bestScore = score
			bestIntent = intent
			bestEvidence = matched
		}
	}
	if bestIntent == "" {
		return nil
	}
	conf := 0.6 + float64(bestScore)*0.02
	if conf > 0.92 {
		conf = 0.92
	}
	return &KeyIntentResult{
		Intent:     bestIntent,
		Confidence: conf,
		Evidence:   bestEvidence,
		Method:     "rule",
	}
}

// classifyKeyIntentByLLM 调用 LLM 进行精细意图识别
func classifyKeyIntentByLLM(ctx context.Context, text string, chat LLMChatClient) (*KeyIntentResult, error) {
	if chat == nil {
		return nil, fmt.Errorf("LLM chat client is nil")
	}
	prompt := buildKeyIntentPrompt(text)
	resp, err := chat.Chat(ctx, prompt, LLMChatOptions{
		Temperature:  0.2,
		MaxTokens:    300,
		SystemPrompt: "你是销售对话意图识别专家。严格按 JSON 格式输出，不要添加其他内容。",
	})
	if err != nil {
		return nil, fmt.Errorf("LLM chat: %w", err)
	}
	var parsed struct {
		Intent     string  `json:"intent"`
		Confidence float64 `json:"confidence"`
		Evidence   string  `json:"evidence"`
		Reasoning  string  `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(extractJSONFromStr(resp)), &parsed); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}
	intent := KeyIntentType(parsed.Intent)
	if !isValidKeyIntent(intent) {
		return nil, fmt.Errorf("invalid key intent: %s", parsed.Intent)
	}
	if parsed.Confidence <= 0 {
		parsed.Confidence = 0.5
	}
	if parsed.Confidence > 1 {
		parsed.Confidence = 1
	}
	return &KeyIntentResult{
		Intent:     intent,
		Confidence: parsed.Confidence,
		Evidence:   parsed.Evidence,
		Reasoning:  parsed.Reasoning,
		Method:     "llm",
	}, nil
}

// buildKeyIntentPrompt 构造 LLM prompt
func buildKeyIntentPrompt(text string) string {
	var sb strings.Builder
	sb.WriteString("客户消息：")
	sb.WriteString(text)
	sb.WriteString("\n\n从以下 7 个精细意图中选择最匹配的 1 个：\n")
	for _, k := range AllKeyIntents {
		sb.WriteString(fmt.Sprintf("- %s (%s)\n", k, KeyIntentDescription[k]))
	}
	sb.WriteString(`
输出要求（严格 JSON）：
{
  "intent": "7 个 key 之一",
  "confidence": 0.0-1.0,
  "evidence": "从客户消息中提取的关键词或短语",
  "reasoning": "选择该意图的简短理由（不超过 30 字）"
}`)
	return sb.String()
}

// isValidKeyIntent 校验 key intent 合法性
func isValidKeyIntent(intent KeyIntentType) bool {
	for _, k := range AllKeyIntents {
		if k == intent {
			return true
		}
	}
	return false
}

// extractJSONFromStr 从字符串中提取 JSON 子串（与 intent_recognition_fine.go 兼容）
func extractJSONFromStr(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return s
	}
	depth := 0
	for i := start; i < len(s); i++ {
		if s[i] == '{' {
			depth++
		} else if s[i] == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s[start:]
}

// ClassifyKeyIntentWithDispatcher 便捷方法：直接使用 *llm.Dispatcher 包装成 LLMChatClient
//
// 用法：
//
//	result := ClassifyKeyIntentWithDispatcher(ctx, text, dispatcher)
func ClassifyKeyIntentWithDispatcher(ctx context.Context, text string, dispatcher *llm.Dispatcher) KeyIntentResult {
	adapter := NewDispatcherChatAdapter(dispatcher)
	return ClassifyKeyIntent(ctx, text, adapter)
}

