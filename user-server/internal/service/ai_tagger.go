package service

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// AI 谈单后自动打标签 + 订单意向提取（Auto Tagger + Order Intent Extractor）
// ----------------------------------------------------------------------------
// 商业市场需求：销售每天接触 50+ 客户，手动打标签必然遗漏。AI 自动从对话
// 中提取标签和订单意向，赋能"数据驱动决策"。
// ============================================================================

// AITagger AI 谈单后自动打标签
type AITagger struct {
	mu sync.RWMutex
	// 客户-标签索引
	customerTags map[string]map[string]TagInfo
	// 标签分类
	tagTaxonomy map[string][]string
}

// TagInfo 标签信息
type TagInfo struct {
	Tag        string    `json:"tag"`
	Category   string    `json:"category"`
	Source     string    `json:"source"` // 来源: ai_chat/manual/order/system
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
}

// NewAITagger 创建 AI 谈单自动打标签器
func NewAITagger() *AITagger {
	return &AITagger{
		customerTags: make(map[string]map[string]TagInfo),
		tagTaxonomy: map[string][]string{
			"interest":  {"beauty", "education", "ecommerce", "fitness", "medical", "real_estate"},
			"price":     {"price_sensitive", "high_end", "mid_range", "budget"},
			"behavior":  {"urgent", "comparing", "hesitant", "ready_to_buy", "researching", "decisive"},
			"lifecycle": {"new_customer", "repeat_customer", "vip", "churn_risk", "dormant"},
			"channel":   {"wechat", "douyin", "xiaohongshu", "xianyu", "tiktok", "offline"},
		},
	}
}

// TagFromSalesResponse 从 AI 谈单响应中自动提取标签
func (t *AITagger) TagFromSalesResponse(ctx context.Context, customerID string, resp *SalesResponse)  []TagInfo {
	if resp == nil {
		return nil
	}
	tags := make([]TagInfo, 0)
	now := time.Now()

	// 1. 基于意图打标签
	if resp.Intent != nil {
		// 行为标签（置信度 = 基础值 + 意图置信度权重，按 0.05 精度取整以避免 0.7/0.91 漂移）
		baseConf := 0.5
		confBoost := resp.Intent.Confidence * 0.4
		if resp.Intent.Confidence < 0.3 {
			confBoost = 0
		}
		// 标签置信度：baseConf + confBoost，并四舍五入到 0.05
		tagConf := roundTo(baseConf+confBoost, 0.05)
		tagConfLow := roundTo(baseConf-0.1+confBoost, 0.05)
		tagConfHigh := roundTo(baseConf+0.15+confBoost, 0.05)
		tagConfStrong := roundTo(baseConf+0.2+confBoost, 0.05)
		switch resp.Intent.IntentType {
		case IntentPriceInquiry:
			tags = append(tags, TagInfo{Tag: "behavior:price_sensitive", Category: "behavior", Source: "ai_chat", Confidence: tagConf, CreatedAt: now})
			tags = append(tags, TagInfo{Tag: "behavior:researching", Category: "behavior", Source: "ai_chat", Confidence: tagConfLow, CreatedAt: now})
		case IntentObjectionPrice:
			tags = append(tags, TagInfo{Tag: "behavior:price_sensitive", Category: "behavior", Source: "ai_chat", Confidence: roundTo(baseConf+0.1+confBoost, 0.05), CreatedAt: now})
			tags = append(tags, TagInfo{Tag: "behavior:hesitant", Category: "behavior", Source: "ai_chat", Confidence: tagConf, CreatedAt: now})
		case IntentAskProduct:
			tags = append(tags, TagInfo{Tag: "behavior:researching", Category: "behavior", Source: "ai_chat", Confidence: tagConf, CreatedAt: now})
		case IntentPurchase:
			tags = append(tags, TagInfo{Tag: "behavior:ready_to_buy", Category: "behavior", Source: "ai_chat", Confidence: tagConfHigh, CreatedAt: now})
		case IntentChurn:
			tags = append(tags, TagInfo{Tag: "lifecycle:churn_risk", Category: "lifecycle", Source: "ai_chat", Confidence: tagConfStrong, CreatedAt: now})
		case IntentComplaint:
			tags = append(tags, TagInfo{Tag: "lifecycle:churn_risk", Category: "lifecycle", Source: "ai_chat", Confidence: tagConfStrong, CreatedAt: now})
		case IntentGreeting, IntentSocial:
			tags = append(tags, TagInfo{Tag: "behavior:friendly", Category: "behavior", Source: "ai_chat", Confidence: roundTo(0.4+confBoost, 0.05), CreatedAt: now})
		}
		// 兴趣标签（基于意图名）
		if interest := matchInterestFromIntent(resp.Intent.IntentName); interest != "" {
			tags = append(tags, TagInfo{Tag: "interest:" + interest, Category: "interest", Source: "ai_chat", Confidence: roundTo(0.5+confBoost, 0.05), CreatedAt: now})
		}
	}

	// 2. 基于转人工判断
	if resp.TransferredToHuman {
		tags = append(tags, TagInfo{Tag: "behavior:needs_human", Category: "behavior", Source: "ai_chat", Confidence: 0.9, CreatedAt: now})
	}

	// 3. 基于脚本模板打标签
	if resp.ScriptTemplate != nil {
		for _, tag := range resp.ScriptTemplate.Tags {
			tags = append(tags, TagInfo{Tag: "interest:" + tag, Category: "interest", Source: "script_match", Confidence: 0.5, CreatedAt: now})
		}
	}

	// 4. 基于消息内容正则提取
	if resp.Memory != nil {
		facts := factsToString(resp.Memory.KeyFacts)
		if strings.Contains(facts, "VIP") || strings.Contains(facts, "高端") {
			tags = append(tags, TagInfo{Tag: "lifecycle:vip", Category: "lifecycle", Source: "ai_chat", Confidence: 0.7, CreatedAt: now})
			tags = append(tags, TagInfo{Tag: "price:high_end", Category: "price", Source: "ai_chat", Confidence: 0.6, CreatedAt: now})
		}
		if strings.Contains(facts, "新客户") || strings.Contains(facts, "首次") {
			tags = append(tags, TagInfo{Tag: "lifecycle:new_customer", Category: "lifecycle", Source: "ai_chat", Confidence: 0.7, CreatedAt: now})
		}
	}

	// 5. 落地标签
	for _, tag := range tags {
		t.applyTag(ctx, customerID, tag)
	}
	return tags
}

// applyTag 应用标签
func (t *AITagger) applyTag(ctx context.Context, customerID string, tag TagInfo)  {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.customerTags[customerID]; !ok {
		t.customerTags[customerID] = make(map[string]TagInfo)
	}
	// 如果已存在且置信度更高则跳过
	if existing, ok := t.customerTags[customerID][tag.Tag]; ok {
		if existing.Confidence >= tag.Confidence {
			return
		}
	}
	t.customerTags[customerID][tag.Tag] = tag
}

// GetTags 获取客户所有标签
func (t *AITagger) GetTags(ctx context.Context, customerID string)  []TagInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	tags := make([]TagInfo, 0)
	if m, ok := t.customerTags[customerID]; ok {
		for _, tag := range m {
			tags = append(tags, tag)
		}
	}
	return tags
}

// GetByCategory 按类别获取标签
func (t *AITagger) GetByCategory(ctx context.Context, customerID, category string) []TagInfo {
	all := t.GetTags(ctx, customerID)
	result := make([]TagInfo, 0)
	for _, tag := range all {
		if tag.Category == category {
			result = append(result, tag)
		}
	}
	return result
}

// matchInterestFromIntent 从意图名匹配兴趣
func matchInterestFromIntent(intentName string) string {
	lower := strings.ToLower(intentName)
	interestMap := map[string]string{
		"美容": "beauty", "医美": "medical", "光子": "medical",
		"教育": "education", "课程": "education", "培训": "education",
		"健身": "fitness", "瑜伽": "fitness",
		"房产": "real_estate", "租房": "real_estate",
		"商品": "ecommerce", "购买": "ecommerce",
	}
	for k, v := range interestMap {
		if strings.Contains(lower, k) {
			return v
		}
	}
	return ""
}

// ============================================================================
// 订单意向提取器（Order Intent Extractor）
// ----------------------------------------------------------------------------
// 商业市场需求：销售对话中客户说"我想要 XX" / "价格多少" / "怎么付款"，
// 必须自动同步为订单意向，赋能给销售跟进。
// ============================================================================

// OrderIntent 订单意向
type OrderIntent struct {
	CustomerID  string         `json:"customer_id"`
	OneID       string         `json:"one_id"`
	ProductName string         `json:"product_name"`
	ProductID   string         `json:"product_id,omitempty"`
	Category    string         `json:"category"`
	Quantity    int            `json:"quantity"`
	UnitPrice   float64        `json:"unit_price"`
	TotalAmount float64        `json:"total_amount"`
	Confidence  float64        `json:"confidence"`
	Source      string         `json:"source"`   // ai_chat / manual
	RawText     string         `json:"raw_text"` // 原始对话
	ExtractedAt time.Time      `json:"extracted_at"`
	Status      string         `json:"status"` // pending/confirmed/converted
	Metadata    map[string]any `json:"metadata"`
}

// OrderIntentExtractor 订单意向提取器
type OrderIntentExtractor struct {
	priceRegex   *regexp.Regexp
	productRegex *regexp.Regexp
	qtyRegex     *regexp.Regexp
}

// NewOrderIntentExtractor 创建订单意向提取器
func NewOrderIntentExtractor() *OrderIntentExtractor {
	return &OrderIntentExtractor{
		priceRegex:   regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:元|块|RMB|¥)`),
		productRegex: regexp.MustCompile(`(光子嫩肤|水光针|玻尿酸|瘦脸针|双眼皮|隆鼻|抽脂|脱毛|祛斑|美牙|隐形矫正|种植牙|补牙|洁牙|体检套餐|SPA|瑜伽课|私教课|舞蹈课|绘画课|钢琴课|编程课|早教|幼小衔接|K12|留学咨询|语培)([a-zA-Z0-9一-龥]*)`),
		qtyRegex:     regexp.MustCompile(`(\d+)\s*(?:次|件|个|台|套|支|瓶|盒|节|期)`),
	}
}

// ExtractFromText 从对话文本中提取订单意向
func (e *OrderIntentExtractor) ExtractFromText(ctx context.Context, customerID, text string)  []OrderIntent {
	intents := make([]OrderIntent, 0)
	if text == "" {
		return intents
	}
	now := time.Now()

	// 1. 提取产品名
	products := e.productRegex.FindAllStringSubmatch(text, -1)
	// 2. 提取价格
	prices := e.priceRegex.FindAllStringSubmatch(text, -1)
	// 3. 提取数量
	qtys := e.qtyRegex.FindAllStringSubmatch(text, -1)

	if len(products) == 0 {
		return intents
	}
	for i, prod := range products {
		if len(prod) < 2 {
			continue
		}
		productName := prod[1]
		category := categorizeProduct(productName)
		var unitPrice float64
		if i < len(prices) {
			// parse price
			priceStr := strings.TrimRight(prices[i][1], " 元块RMB¥")
			unitPrice = atof(priceStr)
		}
		var qty int = 1
		if i < len(qtys) {
			qty = atoi(qtys[i][1])
		}
		intent := OrderIntent{
			CustomerID:  customerID,
			ProductName: productName,
			Category:    category,
			Quantity:    qty,
			UnitPrice:   unitPrice,
			TotalAmount: unitPrice * float64(qty),
			Confidence:  0.7,
			Source:      "ai_chat",
			RawText:     text,
			ExtractedAt: now,
			Status:      "pending",
			Metadata:    make(map[string]any),
		}
		if unitPrice == 0 {
			intent.Confidence = 0.5 // 价格未知，置信度低
		}
		intents = append(intents, intent)
	}
	return intents
}

// ExtractFromSalesResponse 从 AI 谈单响应提取订单意向
func (e *OrderIntentExtractor) ExtractFromSalesResponse(ctx context.Context, customerID string, resp *SalesResponse)  []OrderIntent {
	if resp == nil {
		return nil
	}
	// 从话术模板匹配的产品 + RAG 召回的文档中提取
	text := resp.Reply
	if resp.ScriptTemplate != nil {
		text += " " + resp.ScriptTemplate.Content
	}
	for _, chunk := range resp.RAGChunks {
		text += " " + chunk.Content
	}
	return e.ExtractFromText(ctx, customerID, text)
}

// categorizeProduct 分类产品
func categorizeProduct(name string) string {
	beauty := []string{"光子嫩肤", "水光针", "玻尿酸", "瘦脸针", "双眼皮", "隆鼻", "抽脂", "脱毛", "祛斑", "美牙"}
	medical := []string{"隐形矫正", "种植牙", "补牙", "洁牙", "体检套餐"}
	education := []string{"编程课", "早教", "幼小衔接", "K12", "留学咨询", "语培", "钢琴课", "绘画课", "舞蹈课"}
	fitness := []string{"瑜伽课", "私教课", "SPA"}
	for _, p := range beauty {
		if strings.Contains(name, p) {
			return "beauty"
		}
	}
	for _, p := range medical {
		if strings.Contains(name, p) {
			return "medical"
		}
	}
	for _, p := range education {
		if strings.Contains(name, p) {
			return "education"
		}
	}
	for _, p := range fitness {
		if strings.Contains(name, p) {
			return "fitness"
		}
	}
	return "other"
}

// helpers
func atof(s string) float64 {
	var f float64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			f = f*10 + float64(c-'0')
		} else if c == '.' {
			// 简化处理：忽略小数
			break
		}
	}
	return f
}

func atoi(s string) int {
	var i int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			i = i*10 + int(c-'0')
		}
	}
	if i == 0 {
		i = 1
	}
	return i
}

// factsToString 把 KeyFacts（map）转成可搜索字符串
func factsToString(facts map[string]any) string {
	if len(facts) == 0 {
		return ""
	}
	var sb strings.Builder
	for k, v := range facts {
		sb.WriteString(k)
		sb.WriteString(":")
		switch val := v.(type) {
		case string:
			sb.WriteString(val)
		case float64, int, bool:
			sb.WriteString(fmtSprint(val))
		default:
			sb.WriteString(fmtSprint(val))
		}
		sb.WriteString(";")
	}
	return sb.String()
}

func fmtSprint(v any) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(
		strings.ReplaceAll(stringifyVal(v), "\n", " "), "  ", " "), "  ", " "))
}

func stringifyVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// roundTo 四舍五入到指定精度（如 0.05）
func roundTo(value, precision float64) float64 {
	if precision == 0 {
		return value
	}
	return float64(int64(value/precision+0.5)) * precision
}
