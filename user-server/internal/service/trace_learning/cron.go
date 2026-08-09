package trace_learning

import (
	"context"
	"time"

	"marketing/internal/pkg/utils/logger"
)

// Cron 自学习定时任务：周期扫描未评估 trace 批量打分+调权。
//
// 设计：每小时运行一次（启动延迟 5 分钟避免与装配争抢）；用全局单例 Service 的
// RunBatch（sinceHours/batchSize 取默认配置），LLM 调用失败仅告警不中断。
type Cron struct {
	svc    *Service
	ticker *time.Ticker
	stop   chan struct{}
}

// NewCron 构造定时任务
func NewCron(svc *Service) *Cron {
	return &Cron{svc: svc, stop: make(chan struct{})}
}

// Start 启动周期任务
func (c *Cron) Start(ctx context.Context) {
	c.ticker = time.NewTicker(1 * time.Hour)
	go func() {
		select {
		case <-time.After(5 * time.Minute):
		case <-c.stop:
			return
		}
		for {
			select {
			case <-c.stop:
				return
			case <-c.ticker.C:
				res, err := c.svc.RunBatch(ctx, 0, 0, false)
				if err != nil {
					logger.Warnf("[trace_learning] cron 批量评估失败: %v", err)
					continue
				}
				n := 0
				if res != nil {
					n = res.Processed
				}
				logger.Infof("[trace_learning] cron 批量评估完成 processed=%d", n)
			}
		}
	}()
}

// Stop 停止定时任务
func (c *Cron) Stop(ctx context.Context) {
	if c.ticker != nil {
		c.ticker.Stop()
	}
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
}
