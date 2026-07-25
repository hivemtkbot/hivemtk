package service

// self_learning_cron.go 对话驱动自我学习三位一体机制定时任务
//
// 五层架构归属: L3 业务层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §6
//
// 3 个定时任务（v1.1 §6 Orchestrator 调度）：
//   1. 每小时（hourly）：监督告警扫描（RAG + 资产包）+ 熔断器评估
//   2. 每 6 小时（6h）：候选聚类升级 + A/B 收敛检查 + 熔断器评估
//   3. 每日 0 点（daily）：每日配额重置 + 不活跃资产包降级
//
// 设计：
//   - 每个任务单独启动 goroutine，sleep + 触发
//   - 失败仅记录日志，不影响主服务
//   - Stop() 关闭 stopCh 优雅退出
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"sync"
	"time"

	"marketing/internal/pkg/utils/logger"
	selflearning "marketing/internal/service/self_learning"
)

// SelfLearningCron 自我学习三位一体机制定时任务
type SelfLearningCron struct {
	orchestrator *selflearning.Orchestrator
	stopCh       chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup
}

// NewSelfLearningCron 创建定时任务
//
// orchestrator: 由 InitSelfLearningComponents 返回的 Orchestrator
func NewSelfLearningCron(orchestrator *selflearning.Orchestrator) *SelfLearningCron {
	if orchestrator == nil {
		return nil
	}
	c := &SelfLearningCron{
		orchestrator: orchestrator,
		stopCh:       make(chan struct{}),
	}
	c.wg.Add(3)
	go c.runHourly(context.Background())
	go c.runSixHourly(context.Background())
	go c.runDaily(context.Background())
	return c
}

// Stop 停止所有 cron
//
// 优雅退出设计：
//   1. close(stopCh) 通知所有 goroutine 退出
//   2. 启动后台 goroutine 等待所有 wg 完成
//   3. 主流程 select 在 {wg 完成} 和 {ctx 超时/取消} 上二选一：
//        - wg 先完成：正常优雅退出，所有协程已清理
//        - ctx 先到期：强制返回，避免调用方长时间阻塞（可能有协程泄漏）
//
// 注意：ctx 到期时仍有 goroutine 在运行，调用方应保证 ctx 的 timeout 足够长
// （建议 ≥ 30s，覆盖单个 cron 任务的最坏执行时间）。
// 强制返回后 goroutine 仍会在下一次 select 到 stopCh 时退出，不会无限运行。
func (c *SelfLearningCron) Stop(ctx context.Context) {
	if c == nil {
		return
	}
	// 防止重复 close（panic: close of closed channel）
	// 使用 sync.Once 确保 close 只执行一次（即使 Stop 被多次调用）
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 所有协程已正常退出
	case <-ctx.Done():
		// ctx 超时或取消，强制返回（可能有协程仍在运行）
		// 日志记录便于运维排查
		logger.Errorf("[self_learning_cron] Stop ctx done before all goroutines exited: %v", ctx.Err())
	}
}

// ----------------------------------------------------------------------------
// 1. 每小时任务：监督告警扫描 + 熔断器评估
// ----------------------------------------------------------------------------

// runHourly 每小时触发一次
//
// 触发 Orchestrator.OnCronHourly：
//   1. SwitchService.EvaluateCircuit - 熔断器评估
//   2. RAGSelfSupervisor.ScanAlerts - RAG 监督告警扫描 + 派发修复
//   3. AssetBundleSelfSupervisor.ScanAlerts - 资产包监督告警扫描 + 派发修复
func (c *SelfLearningCron) runHourly(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	// 启动后立即执行一次（避免首次 1h 延迟）
	c.triggerHourly(ctx)
	for {
		select {
		case <-ticker.C:
			c.triggerHourly(ctx)
		case <-c.stopCh:
			return
		}
	}
}

func (c *SelfLearningCron) triggerHourly(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[self_learning_cron] hourly panic: %v", r)
		}
	}()
	c.orchestrator.OnCronHourly(ctx)
}

// ----------------------------------------------------------------------------
// 2. 每 6 小时任务：候选聚类升级 + A/B 收敛检查 + 熔断器评估
// ----------------------------------------------------------------------------

// runSixHourly 每 6 小时触发一次
//
// 触发 Orchestrator.OnCronSixHours：
//   1. AssetBundleLearner.ClusterCandidates - 候选聚类升级
//   2. AssetBundleLearner.CheckConvergence - A/B 收敛检查
//   3. SwitchService.EvaluateCircuit - 熔断器评估
func (c *SelfLearningCron) runSixHourly(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.triggerSixHourly(ctx)
		case <-c.stopCh:
			return
		}
	}
}

func (c *SelfLearningCron) triggerSixHourly(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[self_learning_cron] 6h panic: %v", r)
		}
	}()
	c.orchestrator.OnCronSixHours(ctx)
}

// ----------------------------------------------------------------------------
// 3. 每日任务：配额重置 + 不活跃资产包降级
// ----------------------------------------------------------------------------

// runDaily 每日 0 点触发
//
// 触发 Orchestrator.OnCronDaily：
//   1. SwitchService.ResetDailyCounters - 重置每日计数器
//   2. AssetBundleLearner.DegradeInactiveAssets - 降级不活跃资产包
func (c *SelfLearningCron) runDaily(ctx context.Context) {
	defer c.wg.Done()
	for {
		// 计算到下一个 0 点的时长
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		wait := next.Sub(now)
		select {
		case <-time.After(wait):
			c.triggerDaily(ctx)
		case <-c.stopCh:
			return
		}
	}
}

func (c *SelfLearningCron) triggerDaily(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[self_learning_cron] daily panic: %v", r)
		}
	}()
	c.orchestrator.OnCronDaily(ctx)
}
