package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"marketing/internal/aiagent/llm"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// P1-2 G6 拟人度 6 维度评估器
// ----------------------------------------------------------------------------
// 对应 PRD §5.2 P1-2 G6：严格商用标准 ≥ 0.85
//
// 6 维度权重：
//   自然度 0.25 | 相关性 0.20 | 人设 0.20 | 情绪 0.15 | 简洁性 0.10 | 合规 0.10
//
// 评估流程：
//   1. LLM 评估 6 维度得分（0-1）
//   2. 加权计算综合分
//   3. 综合分 ≥ 0.85 → 通过
//   4. 综合分 < 0.85 → 重生成（最多 3 次）
//   5. 3 次仍不达标 → 转人工 + 记录低质样本
//
// 设计：
//   - PersonaEvaluator 接口：单次评估（LLM / 规则两种实现）
//   - PersonaEvaluationService：上层服务（重生成循环 + 低质样本收集）
//   - LowQualitySampleCollector：低质样本持久化（DB / 日志两种实现）
// ============================================================================

// PersonaDimension 拟人度评估维度
type PersonaDimension string

const (
	PersonaDimensionNaturalness	PersonaDimension	= "naturalness"	// 自然度：口语化、无机械感
	PersonaDimensionRelevance	PersonaDimension	= "relevance"	// 相关性：答非所问检测
	PersonaDimensionPersona		PersonaDimension	= "persona"	// 人设：销冠角色 + 行业专业度
	PersonaDimensionEmotion		PersonaDimension	= "emotion"	// 情绪：共情客户情绪
	PersonaDimensionConciseness	PersonaDimension	= "conciseness"	// 简洁性：字数控制
	PersonaDimensionCompliance	PersonaDimension	= "compliance"	// 合规：广告法 + 虚假承诺
)

// PersonaDimensionWeight 6 维度权重（对应 PRD §5.2 P1-2 G6）
var PersonaDimensionWeight = map[PersonaDimension]float64{
	PersonaDimensionNaturalness:	0.25,
	PersonaDimensionRelevance:	0.20,
	PersonaDimensionPersona:	0.20,
	PersonaDimensionEmotion:	0.15,
	PersonaDimensionConciseness:	0.10,
	PersonaDimensionCompliance:	0.10,
}

// AllPersonaDimensions 全部 6 维度（用于遍历）
var AllPersonaDimensions = []PersonaDimension{
	PersonaDimensionNaturalness,
	PersonaDimensionRelevance,
	PersonaDimensionPersona,
	PersonaDimensionEmotion,
	PersonaDimensionConciseness,
	PersonaDimensionCompliance,
}

// PersonaEvaluationInput 评估输入
type PersonaEvaluationInput struct {
	CustomerID	string	// 客户 ID（用于低质样本追溯）
	SessionID	string	// 会话 ID
	CustomerMessage	string	// 客户消息
	AIReply		string	// AI 回复
	Persona		string	// 销冠人设（如"美妆顾问"/"3C 数码专家"）
	Industry	string	// 行业（如"美妆"/"3C"/"服饰"）
	Platform	string	// 平台（wechat/douyin/xiaohongshu/...）
	Intent		string	// 意图（price_inquiry/complaint/...）
}

// PersonaDimensionScore 单维度得分
type PersonaDimensionScore struct {
	Dimension	PersonaDimension	`json:"dimension"`
	Score		float64			`json:"score"`	// 0-1
	Reason		string			`json:"reason"`
}

// PersonaEvaluationResult 评估结果
type PersonaEvaluationResult struct {
	Scores		[]PersonaDimensionScore	`json:"scores"`
	TotalScore	float64			`json:"total_score"`	// 加权综合分
	Passed		bool			`json:"passed"`		// 是否 ≥ threshold
	AttemptCount	int			`json:"attempt_count"`
	FinalReply	string			`json:"final_reply"`	// 最终回复
	AllReplies	[]string		`json:"all_replies"`	// 所有候选回复（调试用）
	Input		*PersonaEvaluationInput	`json:"input,omitempty"`
}

// ScoreByDimension 按维度查询得分
func (r *PersonaEvaluationResult) ScoreByDimension(ctx context.Context, dim PersonaDimension)  (float64, bool) {
	for _, s := range r.Scores {
		if s.Dimension == dim {
			return s.Score, true
		}
	}
	return 0, false
}

// PersonaEvaluator 单次评估器接口
type PersonaEvaluator interface {
	Evaluate(ctx context.Context, input *PersonaEvaluationInput) (*PersonaEvaluationResult, error)
}

// PersonaRegenerateFn 重生成回调
// 调用方提供：根据上次评估结果重新生成回复
type PersonaRegenerateFn func(ctx context.Context, input *PersonaEvaluationInput, feedback *PersonaEvaluationResult) (string, error)

// LowQualitySampleCollector 低质样本收集器接口
type LowQualitySampleCollector interface {
	Collect(ctx context.Context, input *PersonaEvaluationInput, result *PersonaEvaluationResult) error
}

// =================== LLM 评估器（生产路径） ===================

// LLMPersonaEvaluator 基于 LLM 的拟人度评估器
type LLMPersonaEvaluator struct {
	dispatcher	*llm.Dispatcher
	threshold	float64	// 默认 0.85
}

// NewLLMPersonaEvaluator 构造 LLM 评估器
func NewLLMPersonaEvaluator(dispatcher *llm.Dispatcher) *LLMPersonaEvaluator {
	return &LLMPersonaEvaluator{
		dispatcher:	dispatcher,
		threshold:	0.85,
	}
}

// Threshold 设置阈值（链式）
func (e *LLMPersonaEvaluator) WithThreshold(ctx context.Context, t float64)  *LLMPersonaEvaluator {
	if t > 0 && t <= 1 {
		e.threshold = t
	}
	return e
}

// Evaluate 单次评估
func (e *LLMPersonaEvaluator) Evaluate(ctx context.Context, input *PersonaEvaluationInput) (*PersonaEvaluationResult, error) {
	if e.dispatcher == nil {
		return nil, fmt.Errorf("dispatcher not configured")
	}
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.AIReply == "" {
		return nil, fmt.Errorf("ai_reply cannot be empty")
	}

	prompt := buildPersonaEvaluationPrompt(input)
	req := llm.DispatchRequest{
		Scenario:	llm.ScenarioHighQuality,
		Prompt:		prompt,
		SystemPrompt:	"你是销售对话质检员，严格按 6 维度评估 AI 回复质量，返回 JSON。维度：naturalness/relevance/persona/emotion/conciseness/compliance。",
		JSONMode:	true,
		MaxTokens:	800,
	}
	result, err := e.dispatcher.Dispatch(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("dispatch persona evaluation: %w", err)
	}
	parsed, err := parsePersonaEvaluationResult(result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse persona evaluation: %w", err)
	}
	parsed.Passed = parsed.TotalScore >= e.threshold
	parsed.Input = input
	return parsed, nil
}

// buildPersonaEvaluationPrompt 构建 LLM 评估 prompt
func buildPersonaEvaluationPrompt(input *PersonaEvaluationInput) string {
	var sb strings.Builder
	sb.WriteString("请按以下 6 维度评估 智能体回复的拟人度，每项 0-1 分（1 分完美，0 分极差）：\n\n")
	sb.WriteString("维度说明：\n")
	sb.WriteString("- naturalness（自然度 0.25）：口语化、无机械感，无'作为 AI'等套话\n")
	sb.WriteString("- relevance（相关性 0.20）：是否答非所问，是否回应客户问题\n")
	sb.WriteString("- persona（人设 0.20）：是否符合销冠角色 + 行业专业度\n")
	sb.WriteString("- emotion（情绪 0.15）：是否共情客户情绪\n")
	sb.WriteString("- conciseness（简洁性 0.10）：字数是否合适（建议 ≤80 字）\n")
	sb.WriteString("- compliance（合规 0.10）：是否违反广告法极限词 / 虚假承诺\n\n")
	sb.WriteString(fmt.Sprintf("【客户消息】%s\n", input.CustomerMessage))
	sb.WriteString(fmt.Sprintf("【AI 回复】%s\n", input.AIReply))
	if input.Persona != "" {
		sb.WriteString(fmt.Sprintf("【销冠人设】%s\n", input.Persona))
	}
	if input.Industry != "" {
		sb.WriteString(fmt.Sprintf("【行业】%s\n", input.Industry))
	}
	if input.Platform != "" {
		sb.WriteString(fmt.Sprintf("【平台】%s\n", input.Platform))
	}
	if input.Intent != "" {
		sb.WriteString(fmt.Sprintf("【意图】%s\n", input.Intent))
	}
	sb.WriteString("\n返回 JSON 格式：\n")
	sb.WriteString(`{"scores":[{"dimension":"naturalness","score":0.85,"reason":"..."},{"dimension":"relevance","score":0.9,"reason":"..."},{"dimension":"persona","score":0.8,"reason":"..."},{"dimension":"emotion","score":0.7,"reason":"..."},{"dimension":"conciseness","score":0.95,"reason":"..."},{"dimension":"compliance","score":1.0,"reason":"..."}],"total_score":0.85}`)
	return sb.String()
}

// parsePersonaEvaluationResult 解析 LLM 返回的 JSON
func parsePersonaEvaluationResult(content string) (*PersonaEvaluationResult, error) {
	content = strings.TrimSpace(content)
	// 兼容 ```json ... ``` 包裹
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	var raw struct {
		Scores		[]PersonaDimensionScore	`json:"scores"`
		TotalScore	float64			`json:"total_score"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("unmarshal: %w (content=%s)", err, content)
	}
	if len(raw.Scores) == 0 {
		return nil, fmt.Errorf("empty scores")
	}
	// 归一化得分到 [0,1]
	for i := range raw.Scores {
		if raw.Scores[i].Score < 0 {
			raw.Scores[i].Score = 0
		}
		if raw.Scores[i].Score > 1 {
			raw.Scores[i].Score = 1
		}
	}
	// 如果 total_score 缺失或为 0，按权重重新计算
	totalScore := raw.TotalScore
	if totalScore <= 0 {
		totalScore = computeWeightedScore(raw.Scores)
	}
	return &PersonaEvaluationResult{
		Scores:		raw.Scores,
		TotalScore:	totalScore,
	}, nil
}

// computeWeightedScore 按权重计算综合分
func computeWeightedScore(scores []PersonaDimensionScore) float64 {
	scoreMap := make(map[PersonaDimension]float64, len(scores))
	for _, s := range scores {
		scoreMap[s.Dimension] = s.Score
	}
	var total float64
	for _, dim := range AllPersonaDimensions {
		w := PersonaDimensionWeight[dim]
		s := scoreMap[dim]
		total += w * s
	}
	// 四舍五入到小数点 4 位
	return math.Round(total*10000) / 10000
}

// =================== 规则评估器（测试 / 降级路径） ===================

// RuleBasedPersonaEvaluator 基于规则的拟人度评估器
// 不依赖 LLM，使用关键词匹配 + 字数统计评分
// 用途：
//   - 单元测试（无网络、无 API Key）
//   - LLM 不可用时的降级
//   - LLM 评估前的快速预筛选
type RuleBasedPersonaEvaluator struct {
	threshold float64
}

// NewRuleBasedPersonaEvaluator 构造规则评估器
func NewRuleBasedPersonaEvaluator() *RuleBasedPersonaEvaluator {
	return &RuleBasedPersonaEvaluator{threshold: 0.85}
}

// WithThreshold 设置阈值（链式）
func (e *RuleBasedPersonaEvaluator) WithThreshold(ctx context.Context, t float64)  *RuleBasedPersonaEvaluator {
	if t > 0 && t <= 1 {
		e.threshold = t
	}
	return e
}

// Evaluate 单次评估（规则评分）
func (e *RuleBasedPersonaEvaluator) Evaluate(ctx context.Context, input *PersonaEvaluationInput) (*PersonaEvaluationResult, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.AIReply == "" {
		return nil, fmt.Errorf("ai_reply cannot be empty")
	}
	scores := []PersonaDimensionScore{
		{Dimension: PersonaDimensionNaturalness, Score: e.scoreNaturalness(ctx, input), Reason: "规则评分"},
		{Dimension: PersonaDimensionRelevance, Score: e.scoreRelevance(ctx, input), Reason: "规则评分"},
		{Dimension: PersonaDimensionPersona, Score: e.scorePersona(ctx, input), Reason: "规则评分"},
		{Dimension: PersonaDimensionEmotion, Score: e.scoreEmotion(ctx, input), Reason: "规则评分"},
		{Dimension: PersonaDimensionConciseness, Score: e.scoreConciseness(ctx, input), Reason: "规则评分"},
		{Dimension: PersonaDimensionCompliance, Score: e.scoreCompliance(ctx, input), Reason: "规则评分"},
	}
	total := computeWeightedScore(scores)
	return &PersonaEvaluationResult{
		Scores:		scores,
		TotalScore:	total,
		Passed:		total >= e.threshold,
		Input:		input,
	}, nil
}

// scoreNaturalness 自然度评分：
//   - 包含 AI 痕迹词 → 严重扣分
//   - 口语化词（"嗯"/"哦"/"呢"等）→ 加分
//   - 基础分 0.88：纯中文、正常表达的回复默认较自然
func (e *RuleBasedPersonaEvaluator) scoreNaturalness(ctx context.Context, input *PersonaEvaluationInput)  float64 {
	reply := input.AIReply
	score := 0.88	// 基础分
	// AI 痕迹词（每个 -0.3）
	aiTraces := []string{
		"作为 AI", "作为人工智能", "我是 AI", "我是人工智能",
		"很抱歉，我无法", "作为一个", "我是一个 AI", "我的能力有限",
		"根据您提供的信息", "作为您的销售顾问",
	}
	for _, trace := range aiTraces {
		if strings.Contains(reply, trace) {
			score -= 0.3
		}
	}
	// 口语化词（每个 +0.02，上限 +0.1）
	// "亲" 是中文销售场景的常用称呼，体现自然口语化
	particles := []string{"嗯", "哦", "呢", "啦", "呀", "哈哈", "对了", "亲"}
	particleBonus := 0.0
	for _, p := range particles {
		if strings.Contains(reply, p) {
			particleBonus += 0.02
		}
	}
	if particleBonus > 0.1 {
		particleBonus = 0.1
	}
	score += particleBonus
	return clampScore(score)
}

// scoreRelevance 相关性评分：
//   - 客户消息的关键词是否在 AI 回复中体现
//   - 完全无关联 → 低分
//   - 语义意图对齐（价格问询/推荐等场景）→ 显著加分
func (e *RuleBasedPersonaEvaluator) scoreRelevance(ctx context.Context, input *PersonaEvaluationInput)  float64 {
	if input.CustomerMessage == "" {
		return 0.8	// 无客户消息上下文，给中等分
	}
	// 提取客户消息的关键词（中文按 2-3 字分词简化）
	keywords := extractKeywords(input.CustomerMessage)
	if len(keywords) == 0 {
		return 0.8
	}
	hitCount := 0
	for _, kw := range keywords {
		if strings.Contains(input.AIReply, kw) {
			hitCount++
		}
	}
	hitRate := float64(hitCount) / float64(len(keywords))
	// 语义意图加权：识别客户意图与 AI 回复是否一致
	intentBoost := computeIntentAlignment(input)
	// 计算得分：
	//   关键词命中提供基础分（hit_rate 0 → 0.25, hit_rate 1 → 0.65）
	//   意图对齐提供额外奖励（最高 +0.4）
	//   完全无关（无关键词命中且无意图对齐）应低于 0.4
	score := 0.25 + hitRate*0.4 + intentBoost
	return clampScore(score)
}

// computeIntentAlignment 意图对齐评分
// 识别客户消息的意图类别，与 AI 回复中是否包含对应的语义词
func computeIntentAlignment(input *PersonaEvaluationInput) float64 {
	customer := input.CustomerMessage
	reply := input.AIReply
	boost := 0.0

	// 价格问询：客户提到"多少钱/价格/贵不贵"，AI 提到数字/价格
	if containsAny(customer, []string{"多少钱", "价格", "贵不贵", "怎么卖", "几钱"}) {
		hasNumber := false
		for _, r := range reply {
			if r >= '0' && r <= '9' {
				hasNumber = true
				break
			}
		}
		if hasNumber || containsAny(reply, []string{"价", "元", "块", "￥", "$"}) {
			boost += 0.3
		}
	}

	// 推荐请求：客户提到"推荐/建议/怎么选"，AI 提到"推荐/建议/适合"
	if containsAny(customer, []string{"推荐", "建议", "怎么选", "选哪", "用哪"}) {
		if containsAny(reply, []string{"推荐", "建议", "适合", "选择", "可以", "您可以"}) {
			boost += 0.4
		}
	}

	// 功效问询：客户提到"效果/有用吗/怎么样"，AI 提到效果描述
	if containsAny(customer, []string{"效果", "有用", "怎么样", "好不好", "管用"}) {
		if containsAny(reply, []string{"效果", "帮助", "改善", "缓解", "提升", "可以"}) {
			boost += 0.25
		}
	}

	// 优惠/活动问询
	if containsAny(customer, []string{"优惠", "活动", "折扣", "便宜", "特价"}) {
		if containsAny(reply, []string{"活动", "优惠", "折扣", "特价", "减", "省", "划算"}) {
			boost += 0.3
		}
	}

	// 通用：包含核心词"产品/商品/东西"通常意味着对产品的讨论
	if containsAny(customer, []string{"产品", "商品", "东西", "这个"}) {
		if containsAny(reply, []string{"产品", "商品", "款", "这一款", "这个"}) {
			boost += 0.15
		}
	}

	if boost > 0.5 {
		boost = 0.5
	}
	return boost
}

func containsAny(text string, words []string) bool {
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

// scorePersona 人设评分：
//   - 是否包含人设关键词
//   - 是否包含行业专业词
//   - 基础分 0.75：合理的销售话术默认人设表现良好
func (e *RuleBasedPersonaEvaluator) scorePersona(ctx context.Context, input *PersonaEvaluationInput)  float64 {
	score := 0.75	// 基础分
	reply := input.AIReply
	if input.Persona != "" && strings.Contains(reply, input.Persona) {
		score += 0.1
	}
	if input.Industry != "" && strings.Contains(reply, input.Industry) {
		score += 0.05
	}
	// 销冠常用语（多个词命中累计加分，上限 0.1）
	salesWords := []string{"亲", "您", "建议", "推荐", "适合", "专属", "可以", "您看"}
	salesBonus := 0.0
	for _, w := range salesWords {
		if strings.Contains(reply, w) {
			salesBonus += 0.025
		}
	}
	if salesBonus > 0.1 {
		salesBonus = 0.1
	}
	score += salesBonus
	return clampScore(score)
}

// scoreEmotion 情绪评分：
//   - 共情词（"理解"/"抱歉"/"恭喜"/"开心"等）→ 加分
//   - 投诉场景必须共情，否则扣分
//   - 基础分 0.75：中性专业语气（不冷漠也不过度热情）作为正常基线
func (e *RuleBasedPersonaEvaluator) scoreEmotion(ctx context.Context, input *PersonaEvaluationInput)  float64 {
	reply := input.AIReply
	score := 0.8	// 基础分：中性专业语气作为正常基线
	empathyWords := []string{"理解", "抱歉", "不好意思", "恭喜", "开心", "放心", "别担心", "明白您的", "感谢"}
	for _, w := range empathyWords {
		if strings.Contains(reply, w) {
			score += 0.15
			break
		}
	}
	// 礼貌询问也算轻微共情（询问客户意见，体现尊重）
	politeEmpathy := []string{"您看", "您喜欢", "您考虑", "合适吗", "喜欢吗", "考虑吗"}
	for _, w := range politeEmpathy {
		if strings.Contains(reply, w) {
			score += 0.1
			break
		}
	}
	// 积极销售词（体现主动服务意识）
	activeServiceWords := []string{"建议", "推荐", "适合", "帮您", "为您"}
	for _, w := range activeServiceWords {
		if strings.Contains(reply, w) {
			score += 0.05
			break
		}
	}
	// 投诉场景必须共情（无共情词时严厉扣分，"帮您"等服务词不能替代共情）
	if input.Intent == "complaint" {
		hasEmpathy := false
		for _, w := range empathyWords {
			if strings.Contains(reply, w) {
				hasEmpathy = true
				break
			}
		}
		if !hasEmpathy {
			score -= 0.4
		}
	}
	return clampScore(score)
}

// scoreConciseness 简洁性评分：
//   - ≤60 字 → 满分
//   - 60-100 字 → 0.92
//   - 100-180 字 → 0.7
//   - 180-300 字 → 0.25
//   - >300 字 → 0.1
func (e *RuleBasedPersonaEvaluator) scoreConciseness(ctx context.Context, input *PersonaEvaluationInput)  float64 {
	length := len([]rune(input.AIReply))
	switch {
	case length <= 60:
		return 1.0
	case length <= 100:
		return 0.92
	case length <= 180:
		return 0.7
	case length <= 300:
		return 0.25
	default:
		return 0.1
	}
}

// scoreCompliance 合规评分：
//   - 广告法极限词（"最好"/"第一"/"国家级"等）→ 严重扣分
//   - 虚假承诺词（"100%"/"绝对"/"保证"等）→ 扣分
func (e *RuleBasedPersonaEvaluator) scoreCompliance(ctx context.Context, input *PersonaEvaluationInput)  float64 {
	reply := input.AIReply
	score := 1.0
	// 广告法极限词（每个 -0.4）
	adLawWords := []string{
		"最好", "最佳", "第一", "顶级", "国家级", "世界级",
		"最高级", "唯一", "首个", "独家", "万能", "100%",
	}
	for _, w := range adLawWords {
		if strings.Contains(reply, w) {
			score -= 0.4
		}
	}
	// 虚假承诺（每个 -0.3）
	falsePromiseWords := []string{
		"绝对", "保证", "包治", "根治", "100%有效", "无风险",
	}
	for _, w := range falsePromiseWords {
		if strings.Contains(reply, w) {
			score -= 0.3
		}
	}
	return clampScore(score)
}

// clampScore 限制得分在 [0, 1]
func clampScore(s float64) float64 {
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return math.Round(s*100) / 100
}

// extractKeywords 简化关键词提取（中文 2-4 字滑窗）
// 仅用于规则评分的相关性判断，非通用分词
func extractKeywords(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) < 2 {
		return []string{text}
	}
	// 取 2-3 字滑窗
	var keywords []string
	seen := make(map[string]bool)
	for size := 2; size <= 3; size++ {
		for i := 0; i+size <= len(runes); i++ {
			kw := string(runes[i : i+size])
			if !seen[kw] {
				seen[kw] = true
				keywords = append(keywords, kw)
			}
		}
	}
	// 限制关键词数量，避免过多导致命中率虚低
	if len(keywords) > 20 {
		keywords = keywords[:20]
	}
	return keywords
}

// =================== 低质样本收集器 ===================

// LogLowQualitySampleCollector 日志收集器（默认）
// 仅打印日志，不持久化。用于不需要 DB 的场景。
type LogLowQualitySampleCollector struct{}

// Collect 收集低质样本（仅日志）
func (c *LogLowQualitySampleCollector) Collect(ctx context.Context, input *PersonaEvaluationInput, result *PersonaEvaluationResult) error {
	logger.Infof("[PersonaEvaluator] 低质样本 customer=%s session=%s total_score=%.3f threshold=%.2f attempts=%d final_reply=%s",
		input.CustomerID, input.SessionID, result.TotalScore, 0.85, result.AttemptCount, result.FinalReply)
	return nil
}

// DBLowQualitySampleCollector 数据库收集器
type DBLowQualitySampleCollector struct {
	db *gorm.DB
}

// NewDBLowQualitySampleCollector 构造 DB 收集器
func NewDBLowQualitySampleCollector(db *gorm.DB) *DBLowQualitySampleCollector {
	return &DBLowQualitySampleCollector{db: db}
}

// Collect 收集低质样本到数据库
func (c *DBLowQualitySampleCollector) Collect(ctx context.Context, input *PersonaEvaluationInput, result *PersonaEvaluationResult) error {
	if c.db == nil {
		return nil
	}
	if input == nil || result == nil {
		return nil
	}
	// 序列化维度得分
	scoresMap := make(map[string]float64, len(result.Scores))
	for _, s := range result.Scores {
		scoresMap[string(s.Dimension)] = s.Score
	}
	scoresJSON, _ := json.Marshal(scoresMap)
	repliesJSON, _ := json.Marshal(result.AllReplies)

	// 判断样本类型
	sampleType := model.LowQualitySampleRetryExhausted
	if result.AttemptCount == 1 {
		// 单次评估未达标但未被重试（说明调用方只评估了一次）
		sampleType = model.LowQualitySamplePersona
	}

	sample := &model.LowQualitySample{
		CustomerID:		input.CustomerID,
		SessionID:		input.SessionID,
		SampleType:		sampleType,
		CustomerMessage:	input.CustomerMessage,
		AIReply:		result.FinalReply,
		Persona:		input.Persona,
		Industry:		input.Industry,
		Platform:		input.Platform,
		Intent:			input.Intent,
		DimensionScores:	string(scoresJSON),
		TotalScore:		result.TotalScore,
		Threshold:		0.85,
		AttemptCount:		result.AttemptCount,
		CandidateReplies:	string(repliesJSON),
	}
	if err := c.db.WithContext(ctx).Create(sample).Error; err != nil {
		return fmt.Errorf("save low quality sample: %w", err)
	}
	return nil
}

// =================== 评估服务（含重生成循环） ===================

// PersonaEvaluationService 拟人度评估服务
// 职责：组合单次评估器 + 重生成循环 + 低质样本收集
type PersonaEvaluationService struct {
	evaluator	PersonaEvaluator
	threshold	float64
	maxRetry	int
	sampleCollector	LowQualitySampleCollector
}

// DefaultPersonaThreshold 默认阈值（PRD：≥ 0.85）
const DefaultPersonaThreshold = 0.85

// DefaultPersonaMaxRetry 默认最大重试次数（PRD：最多 3 次）
const DefaultPersonaMaxRetry = 3

// NewPersonaEvaluationService 构造评估服务
func NewPersonaEvaluationService(evaluator PersonaEvaluator) *PersonaEvaluationService {
	return &PersonaEvaluationService{
		evaluator:		evaluator,
		threshold:		DefaultPersonaThreshold,
		maxRetry:		DefaultPersonaMaxRetry,
		sampleCollector:	&LogLowQualitySampleCollector{},
	}
}

// WithThreshold 设置阈值
func (s *PersonaEvaluationService) WithThreshold(ctx context.Context, t float64) *PersonaEvaluationService {
	if t > 0 && t <= 1 {
		s.threshold = t
	}
	return s
}

// WithMaxRetry 设置最大重试次数
func (s *PersonaEvaluationService) WithMaxRetry(ctx context.Context, n int) *PersonaEvaluationService {
	if n > 0 {
		s.maxRetry = n
	}
	return s
}

// WithSampleCollector 设置低质样本收集器
func (s *PersonaEvaluationService) WithSampleCollector(ctx context.Context, c LowQualitySampleCollector) *PersonaEvaluationService {
	if c != nil {
		s.sampleCollector = c
	}
	return s
}

// Evaluate 单次评估（不重生成）
func (s *PersonaEvaluationService) Evaluate(ctx context.Context, input *PersonaEvaluationInput) (*PersonaEvaluationResult, error) {
	result, err := s.evaluator.Evaluate(ctx, input)
	if err != nil {
		return nil, err
	}
	result.Passed = result.TotalScore >= s.threshold
	result.AttemptCount = 1
	result.FinalReply = input.AIReply
	result.AllReplies = []string{input.AIReply}
	result.Input = input
	return result, nil
}

// EvaluateWithRetry 含重生成循环
// 流程：
//  1. 评估 input.AIReply
//  2. 综合分 ≥ threshold → 通过
//  3. 综合分 < threshold → 调 regenerateFn 重新生成（最多 maxRetry 次）
//  4. maxRetry 次仍不达标 → 转人工 + 记录低质样本
//
// regenerateFn 为 nil 时退化为单次评估（不重生成）
func (s *PersonaEvaluationService) EvaluateWithRetry(ctx context.Context, input *PersonaEvaluationInput, regenerateFn PersonaRegenerateFn) (*PersonaEvaluationResult, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.AIReply == "" {
		return nil, fmt.Errorf("ai_reply cannot be empty")
	}

	var result *PersonaEvaluationResult
	var lastErr error
	allReplies := []string{input.AIReply}

	for attempt := 1; attempt <= s.maxRetry; attempt++ {
		r, err := s.evaluator.Evaluate(ctx, input)
		if err != nil {
			lastErr = err
			// 评估失败不立即返回，尝试用上次结果（如果有）
			if result != nil {
				break
			}
			// 第一次评估就失败，无法继续
			return nil, fmt.Errorf("evaluate attempt %d: %w", attempt, err)
		}
		result = r
		result.AttemptCount = attempt
		result.AllReplies = allReplies
		result.Input = input

		// 通过阈值
		if result.TotalScore >= s.threshold {
			result.Passed = true
			result.FinalReply = input.AIReply
			return result, nil
		}

		// 未达标：无重生成函数 → 立即结束（退化为单次评估）
		if regenerateFn == nil {
			result.Passed = false
			result.FinalReply = input.AIReply
			break
		}

		// 未达标，准备重生成
		if attempt < s.maxRetry {
			newReply, err := regenerateFn(ctx, input, r)
			if err != nil {
				// 重生成失败，用当前结果
				result.Passed = false
				result.FinalReply = input.AIReply
				break
			}
			if newReply == "" {
				// 空回复，用当前结果
				result.Passed = false
				result.FinalReply = input.AIReply
				break
			}
			input.AIReply = newReply
			allReplies = append(allReplies, newReply)
		}
	}

	// 达到 maxRetry 仍不达标 → 转人工 + 记录低质样本
	if result == nil {
		return nil, fmt.Errorf("no evaluation result (lastErr=%v)", lastErr)
	}
	result.Passed = false
	result.FinalReply = input.AIReply
	result.AllReplies = allReplies

	// 收集低质样本（不阻塞返回，仅记录错误日志）
	if s.sampleCollector != nil {
		if err := s.sampleCollector.Collect(ctx, input, result); err != nil {
			logger.Errorf("[PersonaEvaluationService] collect low quality sample failed: %v", err)
		}
	}
	return result, nil
}

// Compile-time interface compliance checks
var (
	_	PersonaEvaluator		= (*LLMPersonaEvaluator)(nil)
	_	PersonaEvaluator		= (*RuleBasedPersonaEvaluator)(nil)
	_	LowQualitySampleCollector	= (*LogLowQualitySampleCollector)(nil)
	_	LowQualitySampleCollector	= (*DBLowQualitySampleCollector)(nil)
)

// =================== 辅助：低质样本查询（用于 UI 展示） ===================

// ListLowQualitySamples 列出低质样本
func ListLowQualitySamples(db *gorm.DB, handled *bool, sampleType string, limit, offset int) ([]model.LowQualitySample, int64, error) {
	if db == nil {
		return nil, 0, fmt.Errorf("db not configured")
	}
	if limit <= 0 {
		limit = 20
	}
	q := db.Model(&model.LowQualitySample{})
	if handled != nil {
		q = q.Where("handled = ?", *handled)
	}
	if sampleType != "" {
		q = q.Where("sample_type = ?", sampleType)
	}
	var total int64
	q.Count(&total)
	var list []model.LowQualitySample
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// MarkLowQualitySampleHandled 标记低质样本已处理
func MarkLowQualitySampleHandled(db *gorm.DB, id uint64, handler, note string) error {
	if db == nil {
		return fmt.Errorf("db not configured")
	}
	now := time.Now()
	return db.Model(&model.LowQualitySample{}).Where("id = ?", id).Updates(map[string]any{
		"handled":	true,
		"handled_by":	handler,
		"handled_at":	&now,
		"handled_note":	note,
	}).Error
}

// touchUnusedRegex 避免 regexp 包未被引用（保留以备后续复杂规则扩展）
var _ = regexp.MustCompile
