// Package cron 承载跨域装配的轻量定时任务。
//
// M1 修复：WeComAccountHealthService.ResetDailyQuota 此前仅有手动端点触发，
// 现由 WeComQuotaResetCron 每日 00:05（CST）全量重置一次。
// 惯例与 service.CustomerRFMCron 一致：幂等启动（startOnce）、panic 隔离、可 Stop。
package cron

import (
	"context"
	"fmt"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"
)

// WeComQuotaResetCron 企微账号日配额每日重置定时任务（M1）。
type WeComQuotaResetCron struct {
	svc *service.WeComAccountHealthService

	// executeFn 单次执行入口（默认 svc.ResetAllDailyQuotas；测试可注入 mock）
	executeFn func(ctx context.Context) (int64, error)

	stop      chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
}

// NewWeComQuotaResetCron 构造（svc 为 nil 时回退全局实例）
func NewWeComQuotaResetCron(svc *service.WeComAccountHealthService) *WeComQuotaResetCron {
	if svc == nil {
		svc = service.GetWeComAccountHealthService()
	}
	return &WeComQuotaResetCron{svc: svc, stop: make(chan struct{})}
}

// Start 启动每日调度（幂等：重复调用仅启动一次）
func (c *WeComQuotaResetCron) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		c.wg.Add(1)
		go c.loop(ctx)
		logger.Info("[WeComQuotaResetCron] 已启动（每日 00:05 CST 全量重置企微日配额）")
	})
}

// Stop 停止（幂等：重复调用安全返回）
func (c *WeComQuotaResetCron) Stop(_ context.Context) {
	select {
	case <-c.stop:
		return
	default:
		close(c.stop)
	}
	c.wg.Wait()
	logger.Info("[WeComQuotaResetCron] 已停止")
}

func (c *WeComQuotaResetCron) loop(ctx context.Context) {
	defer c.wg.Done()
	cst := time.FixedZone("CST", 8*3600)
	for {
		next := nextDailyTick(time.Now(), 0, 5, cst)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-c.stop:
			timer.Stop()
			return
		case <-timer.C:
			n, err := c.runOnce(ctx)
			if err != nil {
				logger.Ctx(ctx).Error().Err(err).Int64("reset", n).
					Msg("[WeComQuotaResetCron] 日配额全量重置失败")
			} else {
				logger.Ctx(ctx).Info().Int64("reset", n).
					Msg("[WeComQuotaResetCron] 日配额全量重置完成")
			}
		}
	}
}

// runOnce 单次执行（panic 隔离，供测试直接调用调度触发逻辑）
func (c *WeComQuotaResetCron) runOnce(ctx context.Context) (n int64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("日配额重置 panic: %v", r)
		}
	}()
	fn := c.executeFn
	if fn == nil {
		fn = c.svc.ResetAllDailyQuotas
	}
	return fn(ctx)
}

// nextDailyTick 计算 now 之后最近的每日 hour:minute（loc 时区）触发点；
// 若今日该时刻尚未过去则返回今日时刻（首次启动即补跑当日窗口）。
func nextDailyTick(now time.Time, hour, minute int, loc *time.Location) time.Time {
	n := now.In(loc)
	next := time.Date(n.Year(), n.Month(), n.Day(), hour, minute, 0, 0, loc)
	if !next.After(n) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
