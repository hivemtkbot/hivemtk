package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

// rfmRetryBackoff 默认退避序列（M2）：失败后最多重试 3 次，间隔 1m / 5m / 15m
var rfmRetryBackoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}

// CustomerRFMCron 铁律级补漏：RFM 全量计算定时任务。
//
// v7 审计修复：ComputeAll 此前仅有手动端点触发（content_routes.go），
// 无任何 cron 注册，RFM 分层/流失预警实际永不更新。
// 现每日 04:00（北京时间，避开夜间禁发与业务高峰）全量重算一次。
//
// M2 修复：计算失败按指数退避重试 3 次（1m/5m/15m），全部耗尽后
// 通过既有 system_alerts SSE 通道发布告警（运维侧可实时感知）。
type CustomerRFMCron struct {
	svc *CustomerRFMService

	// computeFn 单次全量计算入口（默认 svc.ComputeAll；测试可注入 mock）
	computeFn func(ctx context.Context) (int, error)
	// retryBackoff 失败重试退避序列（长度 N = 首次执行后最多再重试 N 次）
	retryBackoff []time.Duration
	// onFinalFailure 重试耗尽后的告警回调（默认发 system_alerts SSE + 错误日志；测试可注入捕获）
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

// runWithRetry 单次调度执行：失败按 retryBackoff 退避重试，耗尽后告警（供测试直接调用）
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

// computeOnce 带 panic 隔离的单次计算
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

// sleep 可中断的退避等待；返回 false 表示 cron 已停止
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

// alertFinalFailure 最终失败告警：走既有 system_alerts SSE 通道（PublishSSEEvent 对未初始化 Hub 安全）
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
