package service

// feedback_learning_cron.go G7 反馈学习闭环定时任务
//
// 五层架构归属: L3 业务层
// 设计依据: PRD §5.2 G7 系统自我进化 —— 对话/反馈驱动 AI 持续进化
//
// 职责（打通此前孤儿服务 FeedbackLearningService 的自学习闭环）：
//   1. 周期调用 ExtractProfile 从反馈记录提取销冠画像 5 维度（持久化为快照）
//   2. 遍历所有 SOP，调用 AnalyzeNodeConversion 分析节点转化率
//   3. 对低转化瓶颈节点调用 GenerateOptimizationSuggestions 生成优化建议（落库）
//
// 数据流闭环：
//   SalesEngine.recordFeedback → FeedbackLearner.RecordFeedback（INSERT feedback_records）
//     → [本 cron 周期] FeedbackLearningService.ExtractProfile / AnalyzeNodeConversion
//       → GenerateOptimizationSuggestions（INSERT optimization_suggestions）
//     → 运营审核 → ReviewSuggestion（approve/apply）
//
// 设计：
//   - 单 goroutine + ticker，失败仅记录日志，不影响主服务
//   - Stop() 关闭 stopCh 优雅退出（与 SelfLearningCron.Stop 同模式）

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/repository"
	"marketing/internal/pkg/utils/logger"
)

// FeedbackLearningCron G7 反馈学习闭环定时任务
type FeedbackLearningCron struct {
	svc    *FeedbackLearningService
	sopRepo *repository.SopAgentRepository
	stopCh  chan struct{}
	stopOnce sync.Once
	wg      sync.WaitGroup
}

// NewFeedbackLearningCron 创建 G7 闭环定时任务
//
// db 非空时构造 FeedbackLearningService 与 SOP repository；db 为空返回 nil
// （不启动，避免无 DB 环境下空转报错）。
func NewFeedbackLearningCron(db *gorm.DB) *FeedbackLearningCron {
	if db == nil {
		return nil
	}
	c := &FeedbackLearningCron{
		svc:     NewFeedbackLearningService(db),
		sopRepo: repository.NewSopAgentRepository(db),
		stopCh:  make(chan struct{}),
	}
	c.wg.Add(1)
	go c.runDaily(context.Background())
	return c
}

// Stop 优雅停止
func (c *FeedbackLearningCron) Stop(ctx context.Context) {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() { close(c.stopCh) })
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		logger.Errorf("[feedback_learning_cron] Stop ctx done before goroutine exited: %v", ctx.Err())
	}
}

// runDaily 每日触发一次（首次启动后立即执行一次）
func (c *FeedbackLearningCron) runDaily(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	c.trigger(ctx)
	for {
		select {
		case <-ticker.C:
			c.trigger(ctx)
		case <-c.stopCh:
			return
		}
	}
}

// trigger 执行一次闭环：提取画像 + 遍历 SOP 生成优化建议
func (c *FeedbackLearningCron) trigger(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[feedback_learning_cron] trigger panic: %v", r)
		}
	}()
	if c.svc == nil {
		return
	}

	// 1. 提取销冠画像（系统级智能体，场景 ai_champion，最近 30 天）
	periodEnd := time.Now()
	periodStart := periodEnd.AddDate(0, 0, -30)
	if _, err := c.svc.ExtractProfile(ctx, 0, "系统智能体", "ai_champion", periodStart, periodEnd); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Msg("[feedback_learning_cron] ExtractProfile failed")
	}

	// 2. 遍历所有 SOP：分析节点转化 + 生成优化建议
	if c.sopRepo == nil {
		return
	}
	sops, err := c.sopRepo.ListAll(ctx)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Msg("[feedback_learning_cron] list sops failed")
		return
	}
	for _, sop := range sops {
		stats, aerr := c.svc.AnalyzeNodeConversion(ctx, sop.ID, "")
		if aerr != nil {
			logger.Ctx(ctx).Warn().Err(aerr).Uint("sop_id", sop.ID).
				Msg("[feedback_learning_cron] AnalyzeNodeConversion failed")
			continue
		}
		if _, gerr := c.svc.GenerateOptimizationSuggestions(ctx, OptimizationSuggestionInput{
			SOPID:          sop.ID,
			SOPName:        sop.Name,
			NodeConversion: stats,
		}); gerr != nil {
			logger.Ctx(ctx).Warn().Err(gerr).Uint("sop_id", sop.ID).
				Msg("[feedback_learning_cron] GenerateOptimizationSuggestions failed")
		}
	}
}
