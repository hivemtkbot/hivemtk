package translation

import (
	"context"
	"math/rand"
	"sync/atomic"
	"time"

	"hivemtk-user/internal/aiagent/eval"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)


// EvalLogRepository 评估日志仓储接口（由 repository 层实现）。
type EvalLogRepository interface {
	ListRecentCalls(ctx context.Context, lang string, limit int) ([]*model.LLMRoutingLog, error)
	UpdateQualityScore(ctx context.Context, logID int64, score float64, issues string) error
}

// EvalService 多语言质量评估服务。
type EvalService struct {
	chrF       *eval.ChrFEvaluator
	judge      eval.LLMJudge
	logRepo    EvalLogRepository
	sampleRate float64 
	enabled    bool

	sampledCount   int64 
	evaluatedCount int64 
}

// NewEvalService 创建评估服务。
//
// chrF 为 nil 时使用默认参数。judge 为 nil 时仅做 chrF 评估。
// repo 为 nil 时跳过回填（仅做评估，不持久化）。
func NewEvalService(chrF *eval.ChrFEvaluator, judge eval.LLMJudge, repo EvalLogRepository) *EvalService {
	if chrF == nil {
		chrF = eval.NewChrFEvaluator()
	}
	return &EvalService{
		chrF:       chrF,
		judge:      judge,
		logRepo:    repo,
		sampleRate: 0.05,
		enabled:    true,
	}
}

// SetEnabled 启用/禁用评估服务。
func (s *EvalService) SetEnabled(enabled bool) {
	s.enabled = enabled
}

// SetSampleRate 设置抽样率（0.0 ~ 1.0），越界自动夹紧。
func (s *EvalService) SetSampleRate(rate float64) {
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	s.sampleRate = rate
}

// Stats 返回观测统计（sampled=抽样命中次数, evaluated=评估完成次数）。
func (s *EvalService) Stats() (sampled, evaluated int64) {
	return atomic.LoadInt64(&s.sampledCount), atomic.LoadInt64(&s.evaluatedCount)
}

// MaybeEvaluate 异步抽样评估（在 LLM 调用后触发）。
//
// 行为：
//   - 服务未启用 → 直接返回
//   - log 为 nil 或 candidate/reference 为空 → 直接返回
//   - 按.sampleRate 抽样，未命中 → 直接返回
//   - 命中 → 启动 goroutine 异步执行 evaluate（不阻塞主流程）
//
// 注意：使用 context.Background() 而非入参 ctx，避免主流程 ctx 取消后
// 异步评估被中断。
func (s *EvalService) MaybeEvaluate(ctx context.Context, log *model.LLMRoutingLog, query, candidate, reference string) {
	if !s.enabled {
		return
	}
	if log == nil {
		return
	}
	if candidate == "" || reference == "" {
		return
	}
	if s.sampleRate <= 0 || rand.Float64() > s.sampleRate {
		return
	}
	atomic.AddInt64(&s.sampledCount, 1)
	go s.evaluate(context.Background(), log, query, candidate, reference)
}

// evaluate 执行同步评估（在 goroutine 中运行）。
//
// 流程：
//  1. chrF++ 评估
//  2. LLM-as-Judge（若 judge 已注入且启用）
//  3. 综合分计算
//  4. 回填到 llm_routing_logs（log.ID > 0 时）
//
// 安全：defer recover 兜底 panic；30s 超时保护避免 goroutine 泄漏。
func (s *EvalService) evaluate(ctx context.Context, log *model.LLMRoutingLog, query, candidate, reference string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("eval service: panic recovered: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	chrFScore := s.chrF.Score(candidate, reference)

	// 2. LLM-as-Judge（若注入）
	var judgeScore float64
	var issues string
	if s.judge != nil {
		result, err := s.judge.Judge(ctx, eval.JudgeRequest{
			Query:      query,
			Reference:  reference,
			Candidate:  candidate,
			TargetLang: log.TargetLang,
		})
		if err != nil {
			logger.Warnf("eval service: llm judge failed (log_id=%d): %v", log.ID, err)
		} else if result != nil {
			judgeScore = result.OverallScore
			if result.Explanation != "" {
				issues = result.Explanation
			}
		}
	}

	finalScore := chrFScore
	if judgeScore > 0 {
		finalScore = (chrFScore + judgeScore) / 2
	}

	if s.logRepo != nil && log.ID > 0 {
		if err := s.logRepo.UpdateQualityScore(ctx, log.ID, finalScore, issues); err != nil {
			logger.Warnf("eval service: update quality score failed (log_id=%d): %v", log.ID, err)
		}
	}

	atomic.AddInt64(&s.evaluatedCount, 1)
}

