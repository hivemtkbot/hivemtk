package service

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

// FeedbackLoopCron 反馈学习闭环定时任务
type FeedbackLoopCron struct {
	components *FeedbackLoopComponents
	stopCh     chan struct{}
	wg         sync.WaitGroup
	sopRepo    *repository.SopAgentRepository
	abTestRepo *repository.FeedbackLoopRepository
}

// NewFeedbackLoopCron 创建定时任务
//
// db 参数保留以维持调用方签名兼容；当 db 非空时构造对应 repository，
// 为空时 repos 保持 nil（cron 各任务会跳过 nil repo 对应的 DB 操作）。
func NewFeedbackLoopCron(db *gorm.DB, components *FeedbackLoopComponents) *FeedbackLoopCron {
	c := &FeedbackLoopCron{
		components: components,
		stopCh:     make(chan struct{}),
	}
	if db != nil {
		c.sopRepo = repository.NewSopAgentRepository(db)
		c.abTestRepo = repository.NewFeedbackLoopRepositoryWithDB(db)
	}
	c.wg.Add(6)
	go c.runChampionBaselineMonthly(context.Background(), db)
	go c.runChampionDialogueWeekly(context.Background())
	go c.runPromptIteratorDaily(context.Background())
	go c.runBanditConvergence(context.Background())
	go c.runOptimizerCycle(context.Background())
	go c.runBanditRewardReflux(context.Background(), db)
	return c
}

// Stop 停止所有 cron
func (c *FeedbackLoopCron) Stop(ctx context.Context) {
	close(c.stopCh)
	c.wg.Wait()
}

func (c *FeedbackLoopCron) runChampionBaselineMonthly(ctx context.Context, _ *gorm.DB) {
	defer c.wg.Done()
	for {
		next := nextMonthlyRun(1, 2, 0)
		select {
		case <-c.stopCh:
			return
		case <-time.After(time.Until(next)):
		}
		ctx, cancel := context.WithTimeout(context.Background(), utils.CronVeryLongTimeout)

		report, err := c.components.Analyzer.AnalyzePipeline(ctx, time.Now().AddDate(0, -1, 0))
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[cron] monthly champion analyze pipeline failed")
		} else {
			logger.Ctx(ctx).Info().
				Int("candidates", report.CandidateCount).
				Int("clusters", report.ClusterCount).
				Int("persisted", report.PersistedCount).
				Int("scripts", len(report.ExtractedScripts)).
				Msg("[cron] monthly champion analyze pipeline done")
		}

		periodEnd := time.Now()
		periodStart := periodEnd.AddDate(0, -1, 0)
		rowsAffected := c.refreshChampionBaselineRows(ctx, "default", "default", "all", periodStart, periodEnd)
		logger.Ctx(ctx).Info().Int64("rows_affected", rowsAffected).Msg("[cron] monthly champion baseline refresh done")
		cancel()
	}
}

func (c *FeedbackLoopCron) refreshChampionBaselineRows(
	ctx context.Context,
	persona, industry, intent string,
	periodStart, periodEnd time.Time,
) int64 {
	if c.sopRepo == nil {
		return 0
	}
	scoreRepo := repository.NewHumanizeScoreRepository()
	row, err := scoreRepo.AggregateBaselineMetrics(ctx, periodStart, periodEnd)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[cron] refresh champion baseline: aggregate query failed")
		return 0
	}
	if row.Count == 0 {
		logger.Ctx(ctx).Info().Msg("[cron] refresh champion baseline: no scores in period, skip")
		return 0
	}

	baselineRepo := repository.NewChampionBaselineRepository()
	maxVersion, err := baselineRepo.MaxVersion(ctx, persona, industry, intent)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[cron] refresh champion baseline: query max version failed")
		return 0
	}

	baseline := model.ChampionBaseline{
		Persona:         persona,
		Industry:        industry,
		Intent:          intent,
		Naturalness:     row.AvgN,
		Conciseness:     row.AvgC,
		Empathy:         row.AvgE,
		Professionalism: row.AvgP,
		Persuasiveness:  row.AvgR,
		SampleCount:     int(row.Count),
		SampleStddev:    row.Stddev,
		PeriodStart:     periodStart,
		PeriodEnd:       periodEnd,
		Version:         maxVersion + 1,
		Enabled:         true,
		CreatedAt:       time.Now(),
	}
	if err := baselineRepo.CreateBaseline(ctx, &baseline); err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[cron] refresh champion baseline: insert failed")
		return 0
	}

	if maxVersion > 0 {
		if err := baselineRepo.DisableOldVersions(ctx, persona, industry, intent, maxVersion+1); err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[cron] refresh champion baseline: disable old versions failed")
		}
	}
	return 1
}

func (c *FeedbackLoopCron) runChampionDialogueWeekly(ctx context.Context) {
	defer c.wg.Done()
	for {
		next := nextWeeklyRun(time.Sunday, 3, 0)
		select {
		case <-c.stopCh:
			return
		case <-time.After(time.Until(next)):
		}
		ctx, cancel := context.WithTimeout(context.Background(), utils.CronExtraLongTimeout)
		_, err := c.components.Analyzer.AnalyzePipeline(ctx, time.Now().AddDate(0, 0, -7))
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[cron] weekly champion dialogue analyze failed")
		} else {
			logger.Ctx(ctx).Info().Msg("[cron] weekly champion dialogue analyze done")
		}
		cancel()
	}
}

func (c *FeedbackLoopCron) runPromptIteratorDaily(ctx context.Context) {
	defer c.wg.Done()
	for {
		next := nextDailyRun(4, 0)
		select {
		case <-c.stopCh:
			return
		case <-time.After(time.Until(next)):
		}
		ctx, cancel := context.WithTimeout(context.Background(), utils.CronLongTimeout)
		if c.components == nil || c.components.Iterator == nil {
			cancel()
			continue
		}
		if c.sopRepo == nil {
			cancel()
			continue
		}
		sops, err := c.sopRepo.ListByUseBandit(ctx, true)
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[cron] prompt iterator: load sops failed")
			cancel()
			continue
		}
		totalCandidates := 0
		for _, sop := range sops {
			nodes := extractSOPNodesFromGraph(sop.SOPGraph)
			for _, n := range nodes {
				if n.ID == "" {
					continue
				}
				if !isPromptableNodeType(n.Type) {
					continue
				}
				candidates, err := c.components.Iterator.IterateForNode(ctx, sop.ID, n.ID)
				if err != nil {
					logger.Ctx(ctx).Error().Err(err).Uint("sop_id", sop.ID).Str("node_id", n.ID).Msg("[cron] prompt iterator iterate failed")
					continue
				}
				totalCandidates += len(candidates)
			}
		}
		logger.Ctx(ctx).Info().Int("sop_count", len(sops)).Int("candidates", totalCandidates).Msg("[cron] daily prompt iterator done")
		cancel()
	}
}

type sopGraphNode struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

func extractSOPNodesFromGraph(graph model.JSONMap) []sopGraphNode {
	if len(graph) == 0 {
		return nil
	}
	for _, key := range []string{"nodes", "steps", "node_list"} {
		raw, ok := graph[key]
		if !ok {
			continue
		}
		arr, ok := raw.([]any)
		if !ok {
			continue
		}
		out := make([]sopGraphNode, 0, len(arr))
		for _, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			node := sopGraphNode{
				ID:   stringOf(m["id"]),
				Type: stringOf(m["type"]),
			}
			if node.ID != "" {
				out = append(out, node)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func stringOf(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func isPromptableNodeType(t string) bool {
	switch t {
	case "llm", "generation", "prompt", "ai_response", "ai_message":
		return true
	default:
		return false
	}
}

func (c *FeedbackLoopCron) runBanditConvergence(ctx context.Context) {
	defer c.wg.Done()
	for {
		select {
		case <-c.stopCh:
			return
		case <-time.After(6 * time.Hour):
		}
		ctx, cancel := context.WithTimeout(context.Background(), utils.CronShortTimeout)
		if c.components == nil || c.components.Bandit == nil {
			cancel()
			continue
		}
		var experiments []model.PromptABTest
		if c.abTestRepo != nil {
			var err error
			experiments, err = c.abTestRepo.ListRunningABTests(ctx)
			if err != nil {
				logger.Ctx(ctx).Error().Err(err).Msg("[cron] bandit convergence: load experiments failed")
				cancel()
				continue
			}
		}
		converged, promoted := 0, 0
		for _, exp := range experiments {
			if exp.ExperimentID == "" {
				continue
			}
			winnerKey, ok := c.components.Bandit.CheckConvergence(ctx, exp.ExperimentID)
			if !ok || winnerKey == "" {
				continue
			}
			converged++
			if err := c.components.Bandit.PromoteArm(ctx, exp.ExperimentID, winnerKey); err != nil {
				logger.Ctx(ctx).Error().Err(err).Str("experiment_id", exp.ExperimentID).Msg("[cron] bandit promote failed")
				continue
			}
			promoted++
			if c.abTestRepo != nil {
				utils.WarnErrKV("feedbackloop.cron.promoteAbTest", c.abTestRepo.UpdateABTestByExperimentID(ctx, exp.ExperimentID, map[string]any{
					"status":       model.PromptABTestStatusCompleted,
					"winner_arm":   winnerKey,
					"completed_at": time.Now(),
				}), "experiment_id", exp.ExperimentID, "winner_arm", winnerKey)
			}
		}
		logger.Ctx(ctx).Info().Int("total", len(experiments)).Int("converged", converged).Int("promoted", promoted).Msg("[cron] bandit convergence check done")
		cancel()
	}
}

func (c *FeedbackLoopCron) runOptimizerCycle(ctx context.Context) {
	defer c.wg.Done()
	for {
		select {
		case <-c.stopCh:
			return
		case <-time.After(6 * time.Hour):
		}
		if c.components == nil || c.components.Optimizer == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), utils.CronShortTimeout)
		report, err := c.components.Optimizer.ProcessPendingSuggestions(ctx)
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[cron] sop optimizer cycle failed")
		} else {
			logger.Ctx(ctx).Info().
				Int("pending", report.PendingCount).
				Int("applied", report.AppliedCount).
				Int("failed", report.FailedCount).
				Int("rolled_back", report.RolledBackCount).
				Msg("[cron] sop optimizer cycle done")
		}
		cancel()
	}
}

func nextMonthlyRun(day, hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), day, hour, minute, 0, 0, now.Location())
	if next.Before(now) {
		next = next.AddDate(0, 1, 0)
	}
	return next
}

func nextWeeklyRun(weekday time.Weekday, hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	daysUntil := int(weekday - now.Weekday())
	if daysUntil < 0 || (daysUntil == 0 && next.Before(now)) {
		daysUntil += 7
	}
	next = next.AddDate(0, 0, daysUntil)
	return next
}

func nextDailyRun(hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if next.Before(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
