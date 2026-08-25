package service

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

// CustomerRFMCron 铁律级补漏：RFM 全量计算定时任务。
//
// v7 审计修复：ComputeAll 此前仅有手动端点触发（content_routes.go），
// 无任何 cron 注册，RFM 分层/流失预警实际永不更新。
// 现每日 04:00（北京时间，避开夜间禁发与业务高峰）全量重算一次。
type CustomerRFMCron struct {
	svc       *CustomerRFMService
	stop      chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
}

// NewCustomerRFMCron 构造（svc 为 nil 时使用全局默认构造）
func NewCustomerRFMCron(svc *CustomerRFMService) *CustomerRFMCron {
	if svc == nil {
		svc = NewCustomerRFMService()
	}
	return &CustomerRFMCron{
		svc:  svc,
		stop: make(chan struct{}),
	}
}

// Start 启动每日调度
func (c *CustomerRFMCron) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		c.wg.Add(1)
		go c.loop(ctx)
		logger.Info("[CustomerRFMCron] 已启动（每日 04:00 CST 全量重算 RFM）")
	})
}

// Stop 停止
func (c *CustomerRFMCron) Stop(_ context.Context) {
	select {
	case <-c.stop:
		return
	default:
		close(c.stop)
	}
	c.wg.Wait()
	logger.Info("[CustomerRFMCron] 已停止")
}

func (c *CustomerRFMCron) loop(ctx context.Context) {
	defer c.wg.Done()
	cst := time.FixedZone("CST", 8*3600)
	for {
		next := time.Now().In(cst).Add(24 * time.Hour)
		next = time.Date(next.Year(), next.Month(), next.Day(), 4, 0, 0, 0, cst)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-c.stop:
			timer.Stop()
			return
		case <-timer.C:
			count, err := c.svc.ComputeAll(ctx, 0)
			if err != nil {
				logger.Ctx(ctx).Error().Err(err).Int("computed", count).
					Msg("[CustomerRFMCron] RFM 全量计算失败")
			} else {
				logger.Ctx(ctx).Info().Int("computed", count).
					Msg("[CustomerRFMCron] RFM 全量计算完成")
			}
		}
	}
}
