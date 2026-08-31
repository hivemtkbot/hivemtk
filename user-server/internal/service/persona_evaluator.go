package service

import (
	"context"

	"encoding/json"

	"fmt"

	"math"

	"strings"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/llm"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	"hivemtk-user/internal/repository"
	"regexp"
	"time"
)

type PersonaDimension string

const (
	PersonaDimensionNaturalness PersonaDimension = "naturalness"

	PersonaDimensionRelevance PersonaDimension = "relevance"

	PersonaDimensionPersona PersonaDimension = "persona"

	PersonaDimensionEmotion PersonaDimension = "emotion"

	PersonaDimensionConciseness PersonaDimension = "conciseness"

	PersonaDimensionCompliance PersonaDimension = "compliance"
)

var PersonaDimensionWeight = map[PersonaDimension]float64{
	PersonaDimensionNaturalness: 0.25,
	PersonaDimensionRelevance:   0.20,
	PersonaDimensionPersona:     0.20,
	PersonaDimensionEmotion:     0.15,
	PersonaDimensionConciseness: 0.10,
	PersonaDimensionCompliance:  0.10,
}

var AllPersonaDimensions = []PersonaDimension{
	PersonaDimensionNaturalness,
	PersonaDimensionRelevance,
	PersonaDimensionPersona,
	PersonaDimensionEmotion,
	PersonaDimensionConciseness,
	PersonaDimensionCompliance,
}

type PersonaEvaluationInput struct {
	CustomerID      string
	SessionID       string
	CustomerMessage string
	AIReply         string
	Persona         string
	Industry        string
	Platform        string
	Intent          string
}

type PersonaDimensionScore struct {
	Dimension PersonaDimension `json:"dimension"`
	Score     float64          `json:"score"`
	Reason    string           `json:"reason"`
}

type PersonaEvaluationResult struct {
	Scores       []PersonaDimensionScore `json:"scores"`
	TotalScore   float64                 `json:"total_score"`
	Passed       bool                    `json:"passed"`
	AttemptCount int                     `json:"attempt_count"`
	FinalReply   string                  `json:"final_reply"`
	AllReplies   []string                `json:"all_replies"`
	Input        *PersonaEvaluationInput `json:"input,omitempty"`
}

func (r *PersonaEvaluationResult) ScoreByDimension(ctx context.Context, dim PersonaDimension) (float64, bool) {
	for _, s := range r.Scores {
		if s.Dimension == dim {
			return s.Score, true
		}
	}
	return 0, false
}

type PersonaEvaluator interface {
	Evaluate(ctx context.Context, input *PersonaEvaluationInput) (*PersonaEvaluationResult, error)
}

type PersonaRegenerateFn func(ctx context.Context, input *PersonaEvaluationInput, feedback *PersonaEvaluationResult) (string, error)

type LowQualitySampleCollector interface {
	Collect(ctx context.Context, input *PersonaEvaluationInput, result *PersonaEvaluationResult) error
}

type LLMPersonaEvaluator struct {
	dispatcher *llm.Dispatcher
	threshold  float64
}

func NewLLMPersonaEvaluator(dispatcher *llm.Dispatcher) *LLMPersonaEvaluator {
	return &LLMPersonaEvaluator{
		dispatcher: dispatcher,
		threshold:  0.85,
	}
}

func (e *LLMPersonaEvaluator) WithThreshold(ctx context.Context, t float64) *LLMPersonaEvaluator {
	if t > 0 && t <= 1 {
		e.threshold = t
	}
	return e
}

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
		Scenario:     llm.ScenarioHighQuality,
		Prompt:       prompt,
		SystemPrompt: "你是销售对话质检员，严格按 6 维度评估 AI 回复质量，返回 JSON。维度：naturalness/relevance/persona/emotion/conciseness/compliance。",
		JSONMode:     true,
		MaxTokens:    800,
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

func parsePersonaEvaluationResult(content string) (*PersonaEvaluationResult, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	var raw struct {
		Scores     []PersonaDimensionScore `json:"scores"`
		TotalScore float64                 `json:"total_score"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("unmarshal: %w (content=%s)", err, content)
	}
	if len(raw.Scores) == 0 {
		return nil, fmt.Errorf("empty scores")
	}
	for i := range raw.Scores {
		if raw.Scores[i].Score < 0 {
			raw.Scores[i].Score = 0
		}
		if raw.Scores[i].Score > 1 {
			raw.Scores[i].Score = 1
		}
	}
	totalScore := raw.TotalScore
	if totalScore <= 0 {
		totalScore = computeWeightedScore(raw.Scores)
	}
	return &PersonaEvaluationResult{
		Scores:     raw.Scores,
		TotalScore: totalScore,
	}, nil
}

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
	return math.Round(total*10000) / 10000
}

type RuleBasedPersonaEvaluator struct {
	threshold float64
}

func NewRuleBasedPersonaEvaluator() *RuleBasedPersonaEvaluator {
	return &RuleBasedPersonaEvaluator{threshold: 0.85}
}

func (e *RuleBasedPersonaEvaluator) WithThreshold(ctx context.Context, t float64) *RuleBasedPersonaEvaluator {
	if t > 0 && t <= 1 {
		e.threshold = t
	}
	return e
}

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
		Scores:     scores,
		TotalScore: total,
		Passed:     total >= e.threshold,
		Input:      input,
	}, nil
}

func (e *RuleBasedPersonaEvaluator) scoreNaturalness(ctx context.Context, input *PersonaEvaluationInput) float64 {
	reply := input.AIReply
	score := 0.88
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

func (e *RuleBasedPersonaEvaluator) scoreRelevance(ctx context.Context, input *PersonaEvaluationInput) float64 {
	if input.CustomerMessage == "" {
		return 0.8
	}
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
	intentBoost := computeIntentAlignment(input)
	score := 0.25 + hitRate*0.4 + intentBoost
	return clampScore(score)
}

func computeIntentAlignment(input *PersonaEvaluationInput) float64 {
	customer := input.CustomerMessage
	reply := input.AIReply
	boost := 0.0

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

	if containsAny(customer, []string{"推荐", "建议", "怎么选", "选哪", "用哪"}) {
		if containsAny(reply, []string{"推荐", "建议", "适合", "选择", "可以", "您可以"}) {
			boost += 0.4
		}
	}

	if containsAny(customer, []string{"效果", "有用", "怎么样", "好不好", "管用"}) {
		if containsAny(reply, []string{"效果", "帮助", "改善", "缓解", "提升", "可以"}) {
			boost += 0.25
		}
	}

	if containsAny(customer, []string{"优惠", "活动", "折扣", "便宜", "特价"}) {
		if containsAny(reply, []string{"活动", "优惠", "折扣", "特价", "减", "省", "划算"}) {
			boost += 0.3
		}
	}

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

func (e *RuleBasedPersonaEvaluator) scorePersona(ctx context.Context, input *PersonaEvaluationInput) float64 {
	score := 0.75
	reply := input.AIReply
	if input.Persona != "" && strings.Contains(reply, input.Persona) {
		score += 0.1
	}
	if input.Industry != "" && strings.Contains(reply, input.Industry) {
		score += 0.05
	}
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

func (e *RuleBasedPersonaEvaluator) scoreEmotion(ctx context.Context, input *PersonaEvaluationInput) float64 {
	reply := input.AIReply
	score := 0.8
	empathyWords := []string{"理解", "抱歉", "不好意思", "恭喜", "开心", "放心", "别担心", "明白您的", "感谢"}
	for _, w := range empathyWords {
		if strings.Contains(reply, w) {
			score += 0.15
			break
		}
	}
	politeEmpathy := []string{"您看", "您喜欢", "您考虑", "合适吗", "喜欢吗", "考虑吗"}
	for _, w := range politeEmpathy {
		if strings.Contains(reply, w) {
			score += 0.1
			break
		}
	}
	activeServiceWords := []string{"建议", "推荐", "适合", "帮您", "为您"}
	for _, w := range activeServiceWords {
		if strings.Contains(reply, w) {
			score += 0.05
			break
		}
	}
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

func (e *RuleBasedPersonaEvaluator) scoreConciseness(ctx context.Context, input *PersonaEvaluationInput) float64 {
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

func (e *RuleBasedPersonaEvaluator) scoreCompliance(ctx context.Context, input *PersonaEvaluationInput) float64 {
	reply := input.AIReply
	score := 1.0
	adLawWords := []string{
		"最好", "最佳", "第一", "顶级", "国家级", "世界级",
		"最高级", "唯一", "首个", "独家", "万能", "100%",
	}
	for _, w := range adLawWords {
		if strings.Contains(reply, w) {
			score -= 0.4
		}
	}
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

func clampScore(s float64) float64 {
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return math.Round(s*100) / 100
}

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
	if len(keywords) > 20 {
		keywords = keywords[:20]
	}
	return keywords
}

type LogLowQualitySampleCollector struct{}

// Collect 收集低质样本（仅日志）
func (c *LogLowQualitySampleCollector) Collect(ctx context.Context, input *PersonaEvaluationInput, result *PersonaEvaluationResult) error {
	logger.Infof("[PersonaEvaluator] 低质样本 customer=%s session=%s total_score=%.3f threshold=%.2f attempts=%d final_reply=%s",
		input.CustomerID, input.SessionID, result.TotalScore, 0.85, result.AttemptCount, result.FinalReply)
	return nil
}

type DBLowQualitySampleCollector struct {
	repo repository.PersonaLowQualitySampleRepository
}

func NewDBLowQualitySampleCollector(db *gorm.DB) *DBLowQualitySampleCollector {
	var repo repository.PersonaLowQualitySampleRepository
	if db != nil {
		repo = repository.NewPersonaLowQualitySampleRepositoryWithDB(db)
	}
	return &DBLowQualitySampleCollector{repo: repo}
}

func (c *DBLowQualitySampleCollector) Collect(ctx context.Context, input *PersonaEvaluationInput, result *PersonaEvaluationResult) error {
	if c.repo == nil {
		return nil
	}
	if input == nil || result == nil {
		return nil
	}
	scoresMap := make(map[string]float64, len(result.Scores))
	for _, s := range result.Scores {
		scoresMap[string(s.Dimension)] = s.Score
	}
	scoresJSON, _ := json.Marshal(scoresMap)
	repliesJSON, _ := json.Marshal(result.AllReplies)

	sampleType := model.LowQualitySampleRetryExhausted
	if result.AttemptCount == 1 {
		sampleType = model.LowQualitySamplePersona
	}

	sample := &model.LowQualitySample{
		CustomerID:       input.CustomerID,
		SessionID:        input.SessionID,
		SampleType:       sampleType,
		CustomerMessage:  input.CustomerMessage,
		AIReply:          result.FinalReply,
		Persona:          input.Persona,
		Industry:         input.Industry,
		Platform:         input.Platform,
		Intent:           input.Intent,
		DimensionScores:  string(scoresJSON),
		TotalScore:       result.TotalScore,
		Threshold:        0.85,
		AttemptCount:     result.AttemptCount,
		CandidateReplies: string(repliesJSON),
	}
	return c.repo.Create(ctx, sample)
}

type PersonaEvaluationService struct {
	evaluator       PersonaEvaluator
	threshold       float64
	maxRetry        int
	sampleCollector LowQualitySampleCollector
}

const DefaultPersonaThreshold = 0.85

const DefaultPersonaMaxRetry = 3

func NewPersonaEvaluationService(evaluator PersonaEvaluator) *PersonaEvaluationService {
	return &PersonaEvaluationService{
		evaluator:       evaluator,
		threshold:       DefaultPersonaThreshold,
		maxRetry:        DefaultPersonaMaxRetry,
		sampleCollector: &LogLowQualitySampleCollector{},
	}
}

func (s *PersonaEvaluationService) WithThreshold(ctx context.Context, t float64) *PersonaEvaluationService {
	if t > 0 && t <= 1 {
		s.threshold = t
	}
	return s
}

func (s *PersonaEvaluationService) WithMaxRetry(ctx context.Context, n int) *PersonaEvaluationService {
	if n > 0 {
		s.maxRetry = n
	}
	return s
}

func (s *PersonaEvaluationService) WithSampleCollector(ctx context.Context, c LowQualitySampleCollector) *PersonaEvaluationService {
	if c != nil {
		s.sampleCollector = c
	}
	return s
}

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
			if result != nil {
				break
			}
			return nil, fmt.Errorf("evaluate attempt %d: %w", attempt, err)
		}
		result = r
		result.AttemptCount = attempt
		result.AllReplies = allReplies
		result.Input = input

		if result.TotalScore >= s.threshold {
			result.Passed = true
			result.FinalReply = input.AIReply
			return result, nil
		}

		if regenerateFn == nil {
			result.Passed = false
			result.FinalReply = input.AIReply
			break
		}

		if attempt < s.maxRetry {
			newReply, err := regenerateFn(ctx, input, r)
			if err != nil {
				result.Passed = false
				result.FinalReply = input.AIReply
				break
			}
			if newReply == "" {
				result.Passed = false
				result.FinalReply = input.AIReply
				break
			}
			input.AIReply = newReply
			allReplies = append(allReplies, newReply)
		}
	}

	if result == nil {
		return nil, fmt.Errorf("no evaluation result (lastErr=%v)", lastErr)
	}
	result.Passed = false
	result.FinalReply = input.AIReply
	result.AllReplies = allReplies

	if s.sampleCollector != nil {
		if err := s.sampleCollector.Collect(ctx, input, result); err != nil {
			logger.Errorf("[PersonaEvaluationService] collect low quality sample failed: %v", err)
		}
	}
	return result, nil
}

var (
	_ PersonaEvaluator = (*LLMPersonaEvaluator)(nil)

	_ PersonaEvaluator = (*RuleBasedPersonaEvaluator)(nil)

	_ LowQualitySampleCollector = (*LogLowQualitySampleCollector)(nil)

	_ LowQualitySampleCollector = (*DBLowQualitySampleCollector)(nil)
)

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
		"handled":      true,
		"handled_by":   handler,
		"handled_at":   &now,
		"handled_note": note,
	}).Error
}

// touchUnusedRegex 避免 regexp 包未被引用（保留以备后续复杂规则扩展）
var _ = regexp.MustCompile
