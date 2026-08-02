package humanize

// service.go 拟人度评估主编排服务
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十六章 §16.4.10
//
// 职责：
//  1. 调用 RuleScorer 全量评估
//  2. 边界样本 / 10% 采样 → 调用 LLMScorer
//  3. 不达标 → 重生成（最多 maxRetry 次）
//  4. 3 次仍不达标 → 转人工 + 收集低质样本
//  5. 持久化到 humanize_scores 表

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// DefaultThreshold 默认达标阈值（PRD §5.2 G6：≥ 0.85）
const DefaultThreshold = 0.85

// DefaultBoundaryLow 边界样本下界（含）
const DefaultBoundaryLow = 0.70

// DefaultBoundaryHigh 边界样本上界（不含）
const DefaultBoundaryHigh = 0.85

// DefaultSampleRate 不达标样本 LLM 采样率
const DefaultSampleRate = 0.10

// DefaultMaxRetry 最大重生成次数
const DefaultMaxRetry = 3

// HumanizeEvalService 主编排服务
type HumanizeEvalService struct {
	ruleScorer      HumanizeEvaluator
	llmScorer       HumanizeEvaluator
	baselineRepo    ChampionBaselineRepository
	scoreRepo       HumanizeScoreRepository
	sampleCollector LowQualitySampleCollector
	threshold       float64
	sampleRate      float64
	boundaryLow     float64
	boundaryHigh    float64
	maxRetry        int
	regenerateFn    HumanizeRegenerateFn
	rng             *rand.Rand
}

// NewHumanizeEvalService 构造
func NewHumanizeEvalService(
	rule, llm HumanizeEvaluator,
	baselineRepo ChampionBaselineRepository,
	scoreRepo HumanizeScoreRepository,
	sampleCollector LowQualitySampleCollector,
) *HumanizeEvalService {
	return &HumanizeEvalService{
		ruleScorer:      rule,
		llmScorer:       llm,
		baselineRepo:    baselineRepo,
		scoreRepo:       scoreRepo,
		sampleCollector: sampleCollector,
		threshold:       DefaultThreshold,
		sampleRate:      DefaultSampleRate,
		boundaryLow:     DefaultBoundaryLow,
		boundaryHigh:    DefaultBoundaryHigh,
		maxRetry:        DefaultMaxRetry,
		rng:             rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// WithThreshold 设置达标阈值
func (s *HumanizeEvalService) WithThreshold(ctx context.Context, t float64) *HumanizeEvalService {
	if t > 0 && t <= 1 {
		s.threshold = t
	}
	return s
}

// WithSampleRate 设置 LLM 采样率
func (s *HumanizeEvalService) WithSampleRate(ctx context.Context, r float64) *HumanizeEvalService {
	if r >= 0 && r <= 1 {
		s.sampleRate = r
	}
	return s
}

// WithBoundary 设置边界区间
func (s *HumanizeEvalService) WithBoundary(ctx context.Context, low, high float64) *HumanizeEvalService {
	if low >= 0 && high > low && high <= 1 {
		s.boundaryLow = low
		s.boundaryHigh = high
	}
	return s
}

// WithMaxRetry 设置最大重试次数
func (s *HumanizeEvalService) WithMaxRetry(ctx context.Context, n int) *HumanizeEvalService {
	if n > 0 {
		s.maxRetry = n
	}
	return s
}

// WithRegenerateFn 设置重生成回调
func (s *HumanizeEvalService) WithRegenerateFn(ctx context.Context, fn HumanizeRegenerateFn) *HumanizeEvalService {
	s.regenerateFn = fn
	return s
}

// Evaluate 主入口
//
// 被外部 SalesEngine 第 7.5 步调用
func (s *HumanizeEvalService) Evaluate(ctx context.Context, input *dto.HumanizeEvalInput) (*dto.HumanizeEvalResult, error) {
	if input == nil || input.AIReply == "" {
		return nil, fmt.Errorf("%w: input or ai_reply empty", ErrInvalidInput)
	}
	if s.ruleScorer == nil {
		return nil, ErrAggregatorNotInitialized
	}

	// 1. RuleScorer 全量评估
	ruleResult, err := s.ruleScorer.Evaluate(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("rule scorer: %w", err)
	}
	ruleResult.SampleStrategy = "full"

	// 2. 决定是否触发 LLMScorer
	decision := decideLLMSample(
		ruleResult.TotalScore, s.threshold,
		s.boundaryLow, s.boundaryHigh, s.sampleRate, s.rng,
	)

	if decision.NeedLLM && s.llmScorer != nil {
		// 注入销冠基线（如可用）
		if s.baselineRepo != nil && input.Persona != "" {
			baseline, berr := s.baselineRepo.FindByPersonaIndustryIntent(ctx, input.Persona, input.Industry, input.Intent)
			if berr == nil && baseline != nil {
				if ls, ok := s.llmScorer.(*LLMScorerImpl); ok {
					ls.WithBaseline(baseline)
				}
			}
		}
		llmResult, lerr := s.llmScorer.Evaluate(ctx, input)
		if lerr == nil && llmResult != nil {
			llmResult.SampleStrategy = decision.Strategy
			llmResult.EvaluatorType = "hybrid"
			s.persist(ctx, llmResult)
			if llmResult.TotalScore < s.threshold {
				return s.evaluateWithRetry(ctx, input, llmResult)
			}
			llmResult.Passed = true
			llmResult.FinalReply = input.AIReply
			return llmResult, nil
		}
		// LLM 失败则降级到规则结果
	}

	// 3. 持久化规则结果
	s.persist(ctx, ruleResult)
	if ruleResult.TotalScore < s.threshold {
		return s.evaluateWithRetry(ctx, input, ruleResult)
	}
	ruleResult.Passed = true
	ruleResult.FinalReply = input.AIReply
	return ruleResult, nil
}

// evaluateWithRetry 含重生成循环（最多 maxRetry 次）
func (s *HumanizeEvalService) evaluateWithRetry(ctx context.Context, input *dto.HumanizeEvalInput, lastResult *dto.HumanizeEvalResult) (*dto.HumanizeEvalResult, error) {
	result := lastResult
	allReplies := []string{input.AIReply}
	for attempt := 1; attempt <= s.maxRetry; attempt++ {
		result.AttemptCount = attempt
		if result.TotalScore >= s.threshold {
			result.Passed = true
			result.FinalReply = input.AIReply
			result.AllReplies = allReplies
			s.persist(ctx, result)
			return result, nil
		}
		if s.regenerateFn == nil || attempt == s.maxRetry {
			break
		}
		newReply, err := s.regenerateFn(ctx, input, result)
		if err != nil || newReply == "" {
			break
		}
		input.AIReply = newReply
		allReplies = append(allReplies, newReply)
		// 重评估（用 RuleScorer 快速评估）
		newResult, err := s.ruleScorer.Evaluate(ctx, input)
		if err != nil {
			break
		}
		result = newResult
		result.SampleStrategy = "full"
	}
	// 仍不达标 → 转人工 + 收集低质样本
	result.Passed = false
	result.FinalReply = input.AIReply
	result.AllReplies = allReplies
	result.AttemptCount = s.maxRetry
	// 收集低质样本（复用 接口）
	if s.sampleCollector != nil {
		sampleType := s.decideLowQualitySampleType(ctx, result)
		sample := s.buildLowQualitySample(ctx, input, result, sampleType)
		if err := s.sampleCollector.Collect(ctx, sample); err != nil {
			logger.Errorf("[HumanizeEval] collect low quality sample: %v", err)
		}
	}
	s.persist(ctx, result)
	return result, nil
}

// decideLowQualitySampleType 根据最低分维度决定低质样本类型
func (s *HumanizeEvalService) decideLowQualitySampleType(ctx context.Context, result *dto.HumanizeEvalResult) string {
	if result == nil || len(result.Scores) == 0 {
		return "retry_exhausted"
	}
	// 找最低分维度
	minScore := 1.0
	minDim := ""
	for _, sc := range result.Scores {
		if sc.Score < minScore {
			minScore = sc.Score
			minDim = string(sc.Dimension)
		}
	}
	switch minDim {
	case "naturalness":
		return "naturalness_low"
	case "persuasiveness":
		return "persuasiveness_low"
	default:
		return "retry_exhausted"
	}
}

// persist 持久化到 humanize_scores 表
func (s *HumanizeEvalService) persist(ctx context.Context, r *dto.HumanizeEvalResult) {
	if s.scoreRepo == nil || r == nil {
		return
	}
	score, dimensions := s.buildScoreFromResult(ctx, r)
	if err := s.scoreRepo.Save(ctx, score, dimensions); err != nil {
		logger.Errorf("[HumanizeEval] persist score: %v", err)
	}
}

// buildScoreFromResult 将 dto.HumanizeEvalResult 转换为 model.HumanizeScore + 维度明细
// 五层架构：dto→model 转换在 service 层完成，repository 仅负责持久化
func (s *HumanizeEvalService) buildScoreFromResult(ctx context.Context, r *dto.HumanizeEvalResult) (*model.HumanizeScore, []model.HumanizeDimensionRecord) {
	score := &model.HumanizeScore{
		TotalScore:         r.TotalScore,
		Threshold:          DefaultThreshold,
		DistanceToChampion: r.DistanceToChampion,
		Passed:             r.Passed,
		AttemptCount:       r.AttemptCount,
		LLMModel:           r.LLMModel,
		LLMLatencyMs:       r.LLMLatencyMs,
		EvaluatorType:      model.HumanizeEvaluatorType(r.EvaluatorType),
		SampleStrategy:     model.HumanizeSampleStrategy(r.SampleStrategy),
		FinalReply:         r.FinalReply,
	}
	if r.Input != nil {
		score.SessionID = r.Input.SessionID
		score.CustomerID = r.Input.CustomerID
		score.MessageID = r.Input.MessageID
		score.Persona = r.Input.Persona
		score.Industry = r.Input.Industry
		score.Platform = r.Input.Platform
		score.Intent = r.Input.Intent
		score.CustomerMessage = r.Input.CustomerMessage
		score.AIReply = r.Input.AIReply
	}
	// 填充 5 维得分 + 构建维度明细
	var dimensions []model.HumanizeDimensionRecord
	reasonMap := make(map[string]string, len(r.Scores))
	for _, sc := range r.Scores {
		reasonMap[string(sc.Dimension)] = sc.Reason
		switch sc.Dimension {
		case dto.HumanizeDimNaturalness:
			score.Naturalness = sc.Score
		case dto.HumanizeDimConciseness:
			score.Conciseness = sc.Score
		case dto.HumanizeDimEmpathy:
			score.Empathy = sc.Score
		case dto.HumanizeDimProfessionalism:
			score.Professionalism = sc.Score
		case dto.HumanizeDimPersuasiveness:
			score.Persuasiveness = sc.Score
		}
	}
	if len(r.Scores) > 0 {
		dimensions = make([]model.HumanizeDimensionRecord, 0, len(r.Scores))
		for _, sc := range r.Scores {
			dimensions = append(dimensions, model.HumanizeDimensionRecord{
				Dimension: string(sc.Dimension),
				Score:     sc.Score,
				Weight:    dto.HumanizeDimensionWeight[sc.Dimension],
				Reason:    sc.Reason,
			})
		}
	}
	reasonJSON, _ := json.Marshal(reasonMap)
	score.ReasonJSON = string(reasonJSON)
	return score, dimensions
}

// buildLowQualitySample 将 dto 评估结果转换为 model.LowQualitySample
// 包含序列化维度得分和候选回复（原 repository 层逻辑，上移到 service）
func (s *HumanizeEvalService) buildLowQualitySample(ctx context.Context, input *dto.HumanizeEvalInput, result *dto.HumanizeEvalResult, sampleType string) *model.LowQualitySample {
	if input == nil || result == nil {
		return nil
	}
	// 校验 sampleType
	validTypes := map[string]bool{
		"persona": true, "compliance": true, "naturalness": true, "relevance": true,
		"manual_review": true, "retry_exhausted": true,
		"naturalness_low": true, "persuasiveness_low": true,
		"champion_distance": true, "ab_test_loser": true,
	}
	if !validTypes[sampleType] {
		sampleType = "retry_exhausted"
	}
	// 序列化维度得分
	scoresMap := make(map[string]float64, len(result.Scores))
	for _, sc := range result.Scores {
		scoresMap[string(sc.Dimension)] = sc.Score
	}
	scoresJSON, _ := json.Marshal(scoresMap)
	repliesJSON, _ := json.Marshal(result.AllReplies)
	return &model.LowQualitySample{
		CustomerID:       input.CustomerID,
		SessionID:        input.SessionID,
		SampleType:       model.LowQualitySampleType(sampleType),
		CustomerMessage:  input.CustomerMessage,
		AIReply:          result.FinalReply,
		Persona:          input.Persona,
		Industry:         input.Industry,
		Platform:         input.Platform,
		Intent:           input.Intent,
		DimensionScores:  string(scoresJSON),
		TotalScore:       result.TotalScore,
		Threshold:        DefaultThreshold,
		AttemptCount:     result.AttemptCount,
		CandidateReplies: string(repliesJSON),
	}
}

// 编译时接口实现检查
var (
	_ HumanizeEvaluator = (*RuleScorerImpl)(nil)
	_ HumanizeEvaluator = (*LLMScorerImpl)(nil)
)
