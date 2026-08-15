package service


import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

// FeedbackLearningCron G7 反馈学习闭环定时任务
type FeedbackLearningCron struct {
	svc      *FeedbackLearningService
	sopRepo  *repository.SopAgentRepository
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
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

	periodEnd := time.Now()
	periodStart := periodEnd.AddDate(0, 0, -30)
	if _, err := c.svc.ExtractProfile(ctx, 0, "系统智能体", "ai_champion", periodStart, periodEnd); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Msg("[feedback_learning_cron] ExtractProfile failed")
	}

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

