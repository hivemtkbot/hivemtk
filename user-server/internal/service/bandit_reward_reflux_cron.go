package service

import (
	"context"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service/feedback_loop"
)

// runBanditRewardReflux cron worker：每小时回流一次（D04，挂载于 FeedbackLoopCron 第 6 worker）。
// 起始 cursor = 启动时间-1h；重启后 cursor 归零重扫由 bandit_reflux_log 幂等兜底
// （event_id 唯一索引 + session/sop/signal 转化去重唯一索引，迁移 v3.30.0）。
func (c *FeedbackLoopCron) runBanditRewardReflux(ctx context.Context, db *gorm.DB) {
	defer c.wg.Done()
	cursor := time.Now().Add(-time.Hour)
	reflux := feedbackloop.NewBanditRewardReflux(db, c.components.Bandit)
	for {
		select {
		case <-c.stopCh:
			return
		case <-time.After(time.Hour):
		}
		if c.components == nil || c.components.Bandit == nil || db == nil {
			continue
		}
		now := time.Now()
		wctx, cancel := context.WithTimeout(context.Background(), utils.CronShortTimeout)
		stats, err := reflux.RefluxOnce(wctx, cursor, now)
		cancel()
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[cron] bandit reward reflux failed")
			continue
		}
		cursor = now
		logger.Ctx(ctx).Info().
			Int("scanned", stats.Scanned).
			Int("refluxed", stats.Refluxed).
			Int("skipped", stats.Skipped).
			Int("duped", stats.Duped).
			Int("failed", stats.Failed).
			Msg("[cron] bandit reward reflux done")
	}
}
