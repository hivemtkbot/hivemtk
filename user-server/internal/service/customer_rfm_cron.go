package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

var rfmRetryBackoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}

type CustomerRFMCron struct {
	svc *CustomerRFMService

	computeFn func(ctx context.Context) (int, error)

	retryBackoff []time.Duration

	onFinalFailure func(ctx context.Context, attempts int, err error)

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
		svc:          svc,
		retryBackoff: append([]time.Duration(nil), rfmRetryBackoff...),
		stop:         make(chan struct{}),
	}
}

// Start 启动每日调度（幂等：重复调用仅启动一次）
func (c *CustomerRFMCron) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		c.wg.Add(1)
		go c.loop(ctx)
		logger.Info("[CustomerRFMCron] 已启动（每日 04:00 CST 全量重算 RFM，失败重试 1m/5m/15m）")
	})
}

// Stop 停止（幂等：重复调用安全返回；会中断进行中的退避等待）
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
			c.runWithRetry(ctx)
		}
	}
}

func (c *CustomerRFMCron) runWithRetry(ctx context.Context) {
	backoff := c.retryBackoff
	if len(backoff) == 0 {
		backoff = rfmRetryBackoff
	}
	attempts := len(backoff) + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			logger.Ctx(ctx).Warn().Err(lastErr).
				Int("failed_attempt", attempt-1).
				Dur("backoff", backoff[attempt-2]).
				Msg("[CustomerRFMCron] RFM 计算失败，退避后将重试")
			if !c.sleep(backoff[attempt-2]) {
				logger.Info("[CustomerRFMCron] 退避等待期间收到停止信号，放弃本次调度")
				return
			}
		}
		count, err := c.computeOnce(ctx)
		if err == nil {
			logger.Ctx(ctx).Info().Int("computed", count).Int("attempt", attempt).
				Msg("[CustomerRFMCron] RFM 全量计算完成")
			return
		}
		lastErr = err
	}
	c.alertFinalFailure(ctx, attempts, lastErr)
}

func (c *CustomerRFMCron) computeOnce(ctx context.Context) (count int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("RFM 计算 panic: %v", r)
		}
	}()
	fn := c.computeFn
	if fn == nil {
		fn = func(ctx context.Context) (int, error) { return c.svc.ComputeAll(ctx, 0) }
	}
	return fn(ctx)
}

func (c *CustomerRFMCron) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-c.stop:
		return false
	case <-t.C:
		return true
	}
}

func (c *CustomerRFMCron) alertFinalFailure(ctx context.Context, attempts int, err error) {
	if c.onFinalFailure != nil {
		c.onFinalFailure(ctx, attempts, err)
		return
	}
	logger.Ctx(ctx).Error().Err(err).Int("attempts", attempts).
		Msg("[CustomerRFMCron] RFM 全量计算重试耗尽，触发告警")
	PublishSSEEvent(SSETopicSystemAlerts, "cron_failed", map[string]any{
		"job":      "customer_rfm_compute_all",
		"attempts": attempts,
		"error":    err.Error(),
	}, "")
}
