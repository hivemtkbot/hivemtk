package service

// ============================================================================
// SOP Outbox 调度器与卡死检测器（P0-1 SOP 节点执行器完善设计）
// ----------------------------------------------------------------------------
// 设计依据：docs/核心链路优化.md 第十三章 §13.2.4 / §13.2.5
// 私域独立部署：无 merchant_id 字段
// 五层架构：本文件位于 L3 业务层（Service）
//
// 设计要点：
//   - OutboxDispatcher：独立 goroutine 轮询 sop_timers 表（5s 周期），
//     扫描到期的 timer 后派发任务给 SOPExecutionDispatcher
//   - StuckExecutionDetector：独立 goroutine 周期扫描（60s 周期），
//     检测卡死的 Execution（24h 无 event 且非 waiting 态），自动恢复
//   - 两个组件均由 SOPScheduler 启动，与 SOPScheduler.tick 解耦
// ============================================================================

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// SOPDispatchSender 调度任务派发接口
//
// 抽象自 SOPExecutionDispatcher.DispatchOrLog，便于测试注入 mock，
// 同时解耦 Outbox/StuckDetector 与具体调度器实现。
type SOPDispatchSender interface {
	DispatchOrLog(task *dispatchTask)
}

// ============================================================================
// OutboxDispatcher
// ============================================================================

// SOPOutboxDispatcher Outbox 调度器
//
// 独立 goroutine 轮询 sop_timers 表，扫描到期的 timer 后派发任务给 SOPExecutionDispatcher。
// 与 SOPScheduler 解耦：SOPScheduler 触发新执行，OutboxDispatcher 处理执行内事件。
type SOPOutboxDispatcher struct {
	db             *gorm.DB
	execDispatcher SOPDispatchSender
	tickInterval   time.Duration // 轮询周期（默认 5s）
	batchSize      int           // 单次扫描批量（默认 100）
	stopCh         chan struct{}
	wg             sync.WaitGroup
	runMu          sync.Mutex
	running        bool
}

// NewSOPOutboxDispatcher 创建 Outbox 调度器
func NewSOPOutboxDispatcher(db *gorm.DB, execDispatcher SOPDispatchSender) *SOPOutboxDispatcher {
	return &SOPOutboxDispatcher{
		db:             db,
		execDispatcher: execDispatcher,
		tickInterval:   5 * time.Second,
		batchSize:      100,
		stopCh:         make(chan struct{}),
	}
}

// SetTickInterval 设置轮询周期（用于测试）
func (o *SOPOutboxDispatcher) SetTickInterval(d time.Duration) {
	o.tickInterval = d
}

// Start 启动 Outbox 调度器
func (o *SOPOutboxDispatcher) Start() {
	o.runMu.Lock()
	defer o.runMu.Unlock()
	if o.running {
		return
	}
	o.running = true
	o.stopCh = make(chan struct{})
	o.wg.Add(1)
	go o.loop()
	logger.GetLogger().Info().
		Dur("tick_interval", o.tickInterval).
		Int("batch_size", o.batchSize).
		Msg("[SOPOutboxDispatcher] started")
}

// Stop 停止 Outbox 调度器
func (o *SOPOutboxDispatcher) Stop() {
	o.runMu.Lock()
	if !o.running {
		o.runMu.Unlock()
		return
	}
	o.running = false
	close(o.stopCh)
	o.runMu.Unlock()
	o.wg.Wait()
	logger.GetLogger().Info().Msg("[SOPOutboxDispatcher] stopped")
}

// loop 主循环
func (o *SOPOutboxDispatcher) loop() {
	defer o.wg.Done()
	ticker := time.NewTicker(o.tickInterval)
	defer ticker.Stop()

	// 启动时立即执行一次
	o.processDueTimers()

	for {
		select {
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.processDueTimers()
		}
	}
}

// processDueTimers 扫描到期 timer 并派发任务
//
// 幂等性保障：通过 WHERE status='pending' 原子更新为 'fired'，
// 多实例并发时只有第一个实例能成功更新，其他实例 RowsAffected=0 直接跳过。
func (o *SOPOutboxDispatcher) processDueTimers() {
	if o.db == nil || o.execDispatcher == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = logger.WithModule(ctx, "sop_outbox")

	now := time.Now()
	var timers []model.SOPTimer
	if err := o.db.Where("status = ? AND wait_until <= ?", "pending", now).
		Order("wait_until ASC").
		Limit(o.batchSize).
		Find(&timers).Error; err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[outbox] query due timers failed")
		return
	}

	if len(timers) == 0 {
		return
	}

	firedCount := 0
	for _, t := range timers {
		// 原子标记 fired（防多实例重复处理）
		res := o.db.Model(&model.SOPTimer{}).
			Where("id = ? AND status = ?", t.ID, "pending").
			Updates(map[string]any{
				"status":   "fired",
				"fired_at": &now,
			})
		if res.Error != nil {
			logger.Ctx(ctx).Error().Err(res.Error).
				Uint("timer_id", t.ID).
				Msg("[outbox] mark timer fired failed")
			continue
		}
		if res.RowsAffected == 0 {
			// 已被其他实例处理，跳过
			continue
		}

		// 派发任务到 SOPExecutionDispatcher
		traceID := fmt.Sprintf("timer-%d", t.ID)
		o.execDispatcher.DispatchOrLog(&dispatchTask{
			ExecutionID: t.ExecutionID,
			NodeID:      t.NodeID,
			Attempt:     0,
			TraceID:     traceID,
		})
		firedCount++

		logger.Ctx(ctx).Info().
			Uint("timer_id", t.ID).
			Uint("execution_id", t.ExecutionID).
			Str("node_id", t.NodeID).
			Str("wait_event", t.WaitEvent).
			Msg("[outbox] timer fired, dispatched task")
	}

	if firedCount > 0 {
		logger.Ctx(ctx).Info().
			Int("fired_count", firedCount).
			Int("scanned_count", len(timers)).
			Msg("[outbox] processed due timers")
	}
}

// ============================================================================
// StuckExecutionDetector
// ============================================================================

// SOPStuckDetector 卡死执行检测器
//
// 独立 goroutine 周期扫描（默认 60s），检测卡死的 Execution：
//   - status='running' AND started_at < now()-24h（执行超时）
//   - 最近 30min 无 sop_exec_events（节点级卡死）
//   - 且无 pending timer（wait 节点不算卡死）
//
// 恢复策略：
//   - 找到当前节点重新派发任务（attempt=0）
//   - 若仍失败，标记 Execution 为 failed
type SOPStuckDetector struct {
	db             *gorm.DB
	execDispatcher SOPDispatchSender
	maxIdleTime    time.Duration // 节点级卡死阈值（默认 30min）
	maxExecTime    time.Duration // Execution 级超时阈值（默认 24h）
	tickInterval   time.Duration // 扫描周期（默认 60s）
	stopCh         chan struct{}
	wg             sync.WaitGroup
	runMu          sync.Mutex
	running        bool
}

// NewSOPStuckDetector 创建卡死检测器
func NewSOPStuckDetector(db *gorm.DB, execDispatcher SOPDispatchSender) *SOPStuckDetector {
	return &SOPStuckDetector{
		db:             db,
		execDispatcher: execDispatcher,
		maxIdleTime:    30 * time.Minute,
		maxExecTime:    24 * time.Hour,
		tickInterval:   60 * time.Second,
		stopCh:         make(chan struct{}),
	}
}

// SetTickInterval 设置扫描周期（用于测试）
func (d *SOPStuckDetector) SetTickInterval(interval time.Duration) {
	d.tickInterval = interval
}

// SetMaxIdleTime 设置节点级卡死阈值（用于测试）
func (d *SOPStuckDetector) SetMaxIdleTime(t time.Duration) {
	d.maxIdleTime = t
}

// Start 启动卡死检测器
func (d *SOPStuckDetector) Start() {
	d.runMu.Lock()
	defer d.runMu.Unlock()
	if d.running {
		return
	}
	d.running = true
	d.stopCh = make(chan struct{})
	d.wg.Add(1)
	go d.loop()
	logger.GetLogger().Info().
		Dur("tick_interval", d.tickInterval).
		Dur("max_idle_time", d.maxIdleTime).
		Dur("max_exec_time", d.maxExecTime).
		Msg("[SOPStuckDetector] started")
}

// Stop 停止卡死检测器
func (d *SOPStuckDetector) Stop() {
	d.runMu.Lock()
	if !d.running {
		d.runMu.Unlock()
		return
	}
	d.running = false
	close(d.stopCh)
	d.runMu.Unlock()
	d.wg.Wait()
	logger.GetLogger().Info().Msg("[SOPStuckDetector] stopped")
}

// loop 主循环
func (d *SOPStuckDetector) loop() {
	defer d.wg.Done()
	ticker := time.NewTicker(d.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.scanStuckExecutions()
		}
	}
}

// scanStuckExecutions 扫描卡死执行
//
// 扫描条件：
//  1. status='running'
//  2. (last_event_at IS NULL OR last_event_at < now()-maxIdleTime)
//     即最近 maxIdleTime 时间内无节点事件
//  3. NOT EXISTS (SELECT 1 FROM sop_timers WHERE execution_id=sop_executions.id AND status='pending')
//     即无 pending timer（wait 节点不算卡死）
//
// 恢复策略：重新派发当前节点任务（attempt=0）
func (d *SOPStuckDetector) scanStuckExecutions() {
	if d.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = logger.WithModule(ctx, "sop_stuck_detector")

	now := time.Now()
	idleThreshold := now.Add(-d.maxIdleTime)
	execThreshold := now.Add(-d.maxExecTime)

	// 扫描卡死执行（无 pending timer 的 running 执行，且最近无事件）
	var execs []model.SOPExecution
	err := d.db.Where(
		"status = ? AND started_at < ? AND (last_event_at IS NULL OR last_event_at < ?)",
		SOPStatusRunning, execThreshold, idleThreshold,
	).Limit(50).Find(&execs).Error
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[stuck] query stuck executions failed")
		return
	}

	if len(execs) == 0 {
		return
	}

	recoveredCount := 0
	for _, exec := range execs {
		// 检查是否有 pending timer（wait 节点不算卡死）
		var pendingTimerCount int64
		d.db.Model(&model.SOPTimer{}).
			Where("execution_id = ? AND status = ?", exec.ID, "pending").
			Count(&pendingTimerCount)
		if pendingTimerCount > 0 {
			// 有 pending timer，不算卡死
			continue
		}

		// 检查最近是否有 sop_exec_events
		var recentEventCount int64
		d.db.Model(&model.SOPExecEvent{}).
			Where("execution_id = ? AND created_at > ?", exec.ID, idleThreshold).
			Count(&recentEventCount)
		if recentEventCount > 0 {
			// 最近有事件，不算卡死
			continue
		}

		// 真正卡死，尝试恢复
		logger.Ctx(ctx).Warn().
			Uint("execution_id", exec.ID).
			Str("current_node", exec.CurrentNode).
			Time("started_at", exec.StartedAt).
			Msg("[stuck] detected stuck execution, attempting recovery")

		// 重新派发当前节点
		if d.execDispatcher != nil && exec.CurrentNode != "" {
			traceID := fmt.Sprintf("stuck-recovery-%d-%d", exec.ID, now.Unix())
			d.execDispatcher.DispatchOrLog(&dispatchTask{
				ExecutionID: exec.ID,
				NodeID:      exec.CurrentNode,
				Attempt:     0,
				TraceID:     traceID,
			})
			recoveredCount++
		}
	}

	if recoveredCount > 0 {
		logger.Ctx(ctx).Info().
			Int("recovered_count", recoveredCount).
			Int("scanned_count", len(execs)).
			Msg("[stuck] recovered stuck executions")
	}
}

// ============================================================================
// 全局实例
// ============================================================================

var (
	globalOutboxDispatcher *SOPOutboxDispatcher
	globalStuckDetector    *SOPStuckDetector
	outboxOnce             sync.Once
	stuckOnce              sync.Once
)

// InitSOPOutboxDispatcher 初始化全局 Outbox 调度器
func InitSOPOutboxDispatcher(db *gorm.DB, execDispatcher *SOPExecutionDispatcher) *SOPOutboxDispatcher {
	outboxOnce.Do(func() {
		globalOutboxDispatcher = NewSOPOutboxDispatcher(db, execDispatcher)
		globalOutboxDispatcher.Start()
	})
	return globalOutboxDispatcher
}

// GetSOPOutboxDispatcher 获取全局 Outbox 调度器
func GetSOPOutboxDispatcher() *SOPOutboxDispatcher {
	return globalOutboxDispatcher
}

// InitSOPStuckDetector 初始化全局卡死检测器
func InitSOPStuckDetector(db *gorm.DB, execDispatcher *SOPExecutionDispatcher) *SOPStuckDetector {
	stuckOnce.Do(func() {
		globalStuckDetector = NewSOPStuckDetector(db, execDispatcher)
		globalStuckDetector.Start()
	})
	return globalStuckDetector
}

// GetSOPStuckDetector 获取全局卡死检测器
func GetSOPStuckDetector() *SOPStuckDetector {
	return globalStuckDetector
}
