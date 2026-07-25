package i18n

import (
	"context"
	"math/rand"
	"sync/atomic"
	"time"

	"marketing/internal/aiagent/eval"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// EvalService 多语言质量评估服务（v1.2 出海多语言方案 P2-1）
// ----------------------------------------------------------------------------
// 职责：
//   - 异步抽样（默认 5%）LLM 调用进行 chrF++ + LLM-as-Judge 评估
//   - 综合分回填到 llm_routing_logs.quality_score
//   - 不阻塞主流程（goroutine + recover + 超时保护）
//
// 五层架构归属：L2 业务服务层。组合 aiagent/eval（评估能力）+
// EvalLogRepository（仓储接口），不直接访问 db。
//
// 综合分规则：
//   - 仅 chrF 可用：finalScore = chrFScore
//   - chrF + judge 都可用：finalScore = (chrFScore + judgeScore) / 2
//   - judge 失败：降级为仅 chrF
// ============================================================================

// EvalLogRepository 评估日志仓储接口（由 repository 层实现）。
type EvalLogRepository interface {
	// ListRecentCalls 拉取指定目标语言的最近 N 条 LLM 调用日志（离线评估用）。
	ListRecentCalls(ctx context.Context, lang string, limit int) ([]*model.LLMRoutingLog, error)
	// UpdateQualityScore 回填质量分到 llm_routing_logs.quality_score。
	// issues 为评分理由 / 问题摘要（写入 validation_issues 或独立字段，由实现决定）。
	UpdateQualityScore(ctx context.Context, logID int64, score float64, issues string) error
}

// EvalService 多语言质量评估服务。
type EvalService struct {
	chrF       *eval.ChrFEvaluator
	judge      eval.LLMJudge
	logRepo    EvalLogRepository
	sampleRate float64 // 抽样率，0.05 = 5%
	enabled    bool

	// 观测统计（原子读写，无需加锁）
	sampledCount    int64 // 抽样命中次数
	evaluatedCount  int64 // 评估完成次数
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

	// 1. chrF++ 评估
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

	// 3. 综合分（chrF 50% + judge 50%；judge 缺失时全用 chrF）
	finalScore := chrFScore
	if judgeScore > 0 {
		finalScore = (chrFScore + judgeScore) / 2
	}

	// 4. 回填到 llm_routing_logs（仅有真实 log ID 时）
	if s.logRepo != nil && log.ID > 0 {
		if err := s.logRepo.UpdateQualityScore(ctx, log.ID, finalScore, issues); err != nil {
			logger.Warnf("eval service: update quality score failed (log_id=%d): %v", log.ID, err)
		}
	}

	atomic.AddInt64(&s.evaluatedCount, 1)
}
