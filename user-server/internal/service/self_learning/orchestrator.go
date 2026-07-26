package selflearning

// orchestrator.go 自我学习三位一体主调度器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §2.4 §6
//
// 职责：
//   1. 订阅 EventBus 事件（dialogue.started / dialogue.ended / cron.*）
//   2. 协程调度：每个事件触发一个独立 goroutine
//   3. 信号量限流：防止 goroutine 失控（默认 max=50）
//   4. 幂等保证：通过 self_learning_logs 的 UNIQUE(session_id, scenario) 保证
//   5. 优雅关闭：context cancel 后等待所有协程退出
//
// 用户开启即全自动执行（v1.1 §7.4）：
//   - 启动时由 main.go 调用 Orchestrator.Start(ctx)
//   - 订阅事件后，事件触发即自动执行
//   - 用户调用 SwitchService.UpdateSwitch(manual, false, false, false) 关闭后，
//     Orchestrator 仍订阅事件，但每个 handler 内部检查 SwitchService 状态会直接返回

import (
	"context"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"marketing/internal/event"
)

const (
	// defaultMaxConcurrent 默认最大并发协程数
	defaultMaxConcurrent = 50
	// goroutineTimeout 单个协程的最大执行时长
	goroutineTimeout = 5 * time.Minute
	// stopTimeout Stop 等待所有协程退出的超时时间
	stopTimeout = 30 * time.Second
	// stackTraceMaxLen panic 栈最大字节数
	stackTraceMaxLen = 1024
)

// Orchestrator 自我学习主调度器
type Orchestrator struct {
	switchSvc       *SwitchService
	ragCorrector    *RAGSelfCorrector
	assetLearner    *AssetBundleLearner
	ragSupervisor   *RAGSelfSupervisor
	assetSupervisor *AssetBundleSelfSupervisor
	publisher       *DialogueEventPublisher
	bus             EventBus

	// 协程控制
	maxConcurrent int
	sem           chan struct{} // 信号量
	wg            sync.WaitGroup

	// 运行状态
	running  atomic.Bool
	stopCh   chan struct{}
	stopOnce sync.Once

	// 统计（仅供看板展示）
	statsStarted atomic.Int64
	statsSuccess atomic.Int64
	statsFailed  atomic.Int64
	statsSkipped atomic.Int64
	statsRunning atomic.Int64
}

// NewOrchestrator 创建主调度器
//
// maxConcurrent: 最大并发协程数（建议 50，防止 goroutine 失控）
// ragSupervisor / assetSupervisor: 三位一体监督器（v1.1 §7.2 扩展）
//   - 可为 nil（未启用监督告警派发时）
//   - 非 nil 时，onDialogueEnded 会并发触发监督指标采集
func NewOrchestrator(
	switchSvc *SwitchService,
	ragCorrector *RAGSelfCorrector,
	assetLearner *AssetBundleLearner,
	publisher *DialogueEventPublisher,
	bus EventBus,
	maxConcurrent int,
	ragSupervisor *RAGSelfSupervisor,
	assetSupervisor *AssetBundleSelfSupervisor,
) *Orchestrator {
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrent
	}
	return &Orchestrator{
		switchSvc:       switchSvc,
		ragCorrector:    ragCorrector,
		assetLearner:    assetLearner,
		ragSupervisor:   ragSupervisor,
		assetSupervisor: assetSupervisor,
		publisher:       publisher,
		bus:             bus,
		maxConcurrent:   maxConcurrent,
		sem:             make(chan struct{}, maxConcurrent),
		stopCh:          make(chan struct{}),
	}
}

// ============================================================================
// Start / Stop 生命周期
// ============================================================================

// Start 启动调度器（订阅事件）
//
// 必须由 main.go 在依赖注入完成后调用
// 调用后立即返回，事件处理在后台 goroutine 中执行
func (o *Orchestrator) Start(ctx context.Context) error {
	// CAS 原子抢占：只有一个调用方能成功将 running 从 false 翻转为 true
	if !o.running.CompareAndSwap(false, true) {
		return ErrOrchestratorNotRunning
	}
	if o.bus == nil {
		o.running.Store(false)
		return ErrEventBusNil
	}
	log.Printf("[orchestrator] starting self-learning orchestrator (max_concurrent=%d)", o.maxConcurrent)

	// 订阅 dialogue.started
	if err := o.bus.Subscribe(event.TopicDialogueStarted, o.onDialogueStarted); err != nil {
		o.running.Store(false)
		return err
	}
	// 订阅 dialogue.ended
	if err := o.bus.Subscribe(event.TopicDialogueEnded, o.onDialogueEnded); err != nil {
		o.running.Store(false)
		return err
	}
	// 订阅 asset.degraded（用于触发新候选生成）
	if err := o.bus.Subscribe(event.TopicAssetDegraded, o.onAssetDegraded); err != nil {
		o.running.Store(false)
		return err
	}

	log.Printf("[orchestrator] subscribed: dialogue.started, dialogue.ended, asset.degraded")
	return nil
}

// Stop 停止调度器（等待所有协程退出）
//
// 由 main.go 优雅关闭时调用
func (o *Orchestrator) Stop(ctx context.Context) error {
	if !o.running.Load() {
		return nil
	}
	log.Printf("[orchestrator] stopping, waiting for in-flight goroutines...")
	o.running.Store(false)
	// 使用 sync.Once 确保 stopCh 只关闭一次，避免并发 Stop 导致 double-close panic
	o.stopOnce.Do(func() {
		close(o.stopCh)
	})

	// 等待所有协程退出（带超时）
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Printf("[orchestrator] stopped gracefully")
	case <-time.After(stopTimeout):
		log.Printf("[orchestrator] stop timeout, %d goroutines still running", o.statsRunning.Load())
	case <-ctx.Done():
		log.Printf("[orchestrator] stop context cancelled")
	}
	return nil
}

// IsRunning 是否运行中
func (o *Orchestrator) IsRunning() bool {
	return o.running.Load()
}

// ============================================================================
// 事件处理器
// ============================================================================

// onDialogueStarted 处理 dialogue.started 事件
//
// 触发：对话开始（WS 连接建立 + 首条访客消息）
// 动作：调用 RAGSelfCorrector.Warmup 预热 RAG 缓存
func (o *Orchestrator) onDialogueStarted(payload *event.DialogueStartedPayload) {
	if !o.running.Load() || payload == nil {
		return
	}
	o.statsStarted.Add(1)
	o.spawn(func(ctx context.Context) {
		o.statsRunning.Add(1)
		defer o.statsRunning.Add(-1)
		if err := o.ragCorrector.Warmup(ctx, payload); err != nil {
			log.Printf("[orchestrator] rag warmup failed: session=%s err=%v", payload.SessionID, err)
			o.statsFailed.Add(1)
			return
		}
		o.statsSuccess.Add(1)
	})
}

// onDialogueEnded 处理 dialogue.ended 事件
//
// 触发：对话结束（status=closed/resolved）
// 动作：
//  1. RAGSelfCorrector.Reflect - RAG 反思（销冠补录 / 低质标记）
//  2. AssetBundleLearner.GenerateCandidate - 资产包候选生成（若 reward ≥ 阈值）
//  3. RAGSelfSupervisor.CollectMetrics - RAG 5 维监督指标采集
//  4. AssetBundleSelfSupervisor.CollectMetrics - 资产包 5 维专属监督指标采集
//
// 四个动作可并发执行，但都受信号量限流
func (o *Orchestrator) onDialogueEnded(payload *event.DialogueEndedPayload) {
	if !o.running.Load() || payload == nil {
		return
	}
	actionCount := 2
	if o.ragSupervisor != nil {
		actionCount++
	}
	if o.assetSupervisor != nil {
		actionCount++
	}
	o.statsStarted.Add(int64(actionCount))

	// 1. RAG 反思（独立协程）
	o.spawn(func(ctx context.Context) {
		o.statsRunning.Add(1)
		defer o.statsRunning.Add(-1)
		if err := o.ragCorrector.Reflect(ctx, payload); err != nil {
			log.Printf("[orchestrator] rag reflect failed: session=%s err=%v", payload.SessionID, err)
			o.statsFailed.Add(1)
			return
		}
		o.statsSuccess.Add(1)
	})

	// 2. 资产包候选生成（独立协程）
	o.spawn(func(ctx context.Context) {
		o.statsRunning.Add(1)
		defer o.statsRunning.Add(-1)
		if _, err := o.assetLearner.GenerateCandidate(ctx, payload); err != nil {
			log.Printf("[orchestrator] asset generate candidate failed: session=%s err=%v", payload.SessionID, err)
			o.statsFailed.Add(1)
			return
		}
		o.statsSuccess.Add(1)
	})

	// 3. RAG 5 维监督指标采集（独立协程）
	if o.ragSupervisor != nil {
		o.spawn(func(ctx context.Context) {
			o.statsRunning.Add(1)
			defer o.statsRunning.Add(-1)
			if err := o.ragSupervisor.CollectMetrics(ctx, payload); err != nil {
				log.Printf("[orchestrator] rag supervision collect failed: session=%s err=%v", payload.SessionID, err)
				o.statsFailed.Add(1)
				return
			}
			o.statsSuccess.Add(1)
		})
	}

	// 4. 资产包 5 维专属监督指标采集（独立协程）
	if o.assetSupervisor != nil {
		o.spawn(func(ctx context.Context) {
			o.statsRunning.Add(1)
			defer o.statsRunning.Add(-1)
			if err := o.assetSupervisor.CollectMetrics(ctx, payload); err != nil {
				log.Printf("[orchestrator] asset supervision collect failed: session=%s err=%v", payload.SessionID, err)
				o.statsFailed.Add(1)
				return
			}
			o.statsSuccess.Add(1)
		})
	}
}

// onAssetDegraded 处理 asset.degraded 事件
//
// 触发：资产包被降级（连续 30 天 use_count=0）
// 动作：触发新一轮候选生成（查找近 7 天高 reward 对话）
//
// 简化实现：仅记录日志，候选生成由 cron.6h 周期触发
// 后续可扩展为立即触发
func (o *Orchestrator) onAssetDegraded(payload *event.AssetDegradedPayload) {
	if !o.running.Load() || payload == nil {
		return
	}
	log.Printf("[orchestrator] asset degraded: asset=%s reason=%s", payload.AssetID, payload.Reason)
	// 仅记录，候选生成由 cron 触发
}

// ============================================================================
// Cron 定时任务（由 main.go 的 cron 调度器调用）
// ============================================================================

// OnCronSixHours 每 6 小时定时任务
//
// 由 cron 调度器调用，触发：
//  1. AssetBundleLearner.ClusterCandidates - 候选聚类升级
//  2. AssetBundleLearner.CheckConvergence - A/B 收敛检查
//  3. SwitchService.EvaluateCircuit - 熔断器评估
func (o *Orchestrator) OnCronSixHours(ctx context.Context) {
	if !o.running.Load() {
		return
	}
	o.spawn(func(ctx context.Context) {
		// 聚类升级
		if n, err := o.assetLearner.ClusterCandidates(ctx); err != nil {
			log.Printf("[orchestrator] cron.6h cluster failed: %v", err)
		} else if n > 0 {
			log.Printf("[orchestrator] cron.6h cluster promoted %d candidates", n)
		}
	})
	o.spawn(func(ctx context.Context) {
		// 收敛检查
		if n, err := o.assetLearner.CheckConvergence(ctx); err != nil {
			log.Printf("[orchestrator] cron.6h convergence failed: %v", err)
		} else if n > 0 {
			log.Printf("[orchestrator] cron.6h convergence processed %d experiments", n)
		}
	})
	o.spawn(func(ctx context.Context) {
		// 熔断器评估
		if err := o.switchSvc.EvaluateCircuit(ctx); err != nil {
			log.Printf("[orchestrator] cron.6h circuit eval failed: %v", err)
		}
	})
}

// OnCronDaily 每日定时任务（0 点）
//
// 触发：
//  1. SwitchService.ResetDailyCounters - 重置每日计数器
//  2. AssetBundleLearner.DegradeInactiveAssets - 降级不活跃资产包
//  3. CleanStaleLogs - 清理 7 天前的 failed/running 日志（孤儿数据治理）
func (o *Orchestrator) OnCronDaily(ctx context.Context) {
	if !o.running.Load() {
		return
	}
	o.spawn(func(ctx context.Context) {
		if err := o.switchSvc.ResetDailyCounters(ctx); err != nil {
			log.Printf("[orchestrator] cron.daily reset counters failed: %v", err)
		}
	})
	o.spawn(func(ctx context.Context) {
		if n, err := o.assetLearner.DegradeInactiveAssets(ctx); err != nil {
			log.Printf("[orchestrator] cron.daily degrade failed: %v", err)
		} else if n > 0 {
			log.Printf("[orchestrator] cron.daily degraded %d assets", n)
		}
	})
	// 孤儿数据治理：将 7 天前的 failed/running 日志降级为 skipped
	// - failed：长期未重试的失败日志，占据看板"最近失败"列表
	// - running：协程崩溃/进程 OOM 等导致日志卡在 running 状态
	// 7 天窗口足够运维介入排查，超期则自动清理（保留记录，仅状态降级）
	o.spawn(func(ctx context.Context) {
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		if n, err := o.switchSvc.CleanStaleLogs(ctx, cutoff); err != nil {
			log.Printf("[orchestrator] cron.daily clean stale logs failed: %v", err)
		} else if n > 0 {
			log.Printf("[orchestrator] cron.daily cleaned %d stale logs", n)
		}
	})
}

// OnCronHourly 每小时定时任务
//
// 触发：
//  1. SwitchService.EvaluateCircuit - 熔断器评估（更频繁）
//  2. RAGSelfSupervisor.ScanAlerts - RAG 监督告警扫描 + 派发修复
//  3. AssetBundleSelfSupervisor.ScanAlerts - 资产包监督告警扫描 + 派发修复
func (o *Orchestrator) OnCronHourly(ctx context.Context) {
	if !o.running.Load() {
		return
	}
	o.spawn(func(ctx context.Context) {
		if err := o.switchSvc.EvaluateCircuit(ctx); err != nil {
			log.Printf("[orchestrator] cron.hourly circuit eval failed: %v", err)
		}
	})
	if o.ragSupervisor != nil {
		o.spawn(func(ctx context.Context) {
			if n, err := o.ragSupervisor.ScanAlerts(ctx); err != nil {
				log.Printf("[orchestrator] cron.hourly rag scan alerts failed: %v", err)
			} else if n > 0 {
				log.Printf("[orchestrator] cron.hourly rag dispatched %d corrections", n)
			}
		})
	}
	if o.assetSupervisor != nil {
		o.spawn(func(ctx context.Context) {
			if n, err := o.assetSupervisor.ScanAlerts(ctx); err != nil {
				log.Printf("[orchestrator] cron.hourly asset scan alerts failed: %v", err)
			} else if n > 0 {
				log.Printf("[orchestrator] cron.hourly asset dispatched %d corrections", n)
			}
		})
	}
}

// ============================================================================
// 协程调度
// ============================================================================

// spawn 启动一个受信号量限流的 goroutine
//
// 若信号量已满（达到 maxConcurrent），则阻塞等待
// 协程内部 panic 不会影响其他协程（recover 兜底）
func (o *Orchestrator) spawn(fn func(ctx context.Context)) {
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		// 信号量获取
		select {
		case o.sem <- struct{}{}:
			defer func() { <-o.sem }()
		case <-o.stopCh:
			o.statsSkipped.Add(1)
			return
		}
		// panic 兜底
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[orchestrator] goroutine panic: %v\n%s", r, debugStack())
				o.statsFailed.Add(1)
			}
		}()
		// 短超时 context（防止协程卡死）
		ctx, cancel := context.WithTimeout(context.Background(), goroutineTimeout)
		defer cancel()
		// 监听 stopCh
		go func() {
			select {
			case <-o.stopCh:
				cancel()
			case <-ctx.Done():
			}
		}()
		fn(ctx)
	}()
}

// debugStack 获取调用栈（限制最大长度）
func debugStack() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	if n > stackTraceMaxLen {
		n = stackTraceMaxLen
	}
	return string(buf[:n])
}

// ============================================================================
// 统计信息（供看板展示）
// ============================================================================

// OrchestratorStats 调度器统计
type OrchestratorStats struct {
	Running       bool  `json:"running"`
	MaxConcurrent int   `json:"max_concurrent"`
	Started       int64 `json:"started"`
	Success       int64 `json:"success"`
	Failed        int64 `json:"failed"`
	Skipped       int64 `json:"skipped"`
	InFlight      int64 `json:"in_flight"`
}

// GetStats 获取统计
func (o *Orchestrator) GetStats() OrchestratorStats {
	return OrchestratorStats{
		Running:       o.running.Load(),
		MaxConcurrent: o.maxConcurrent,
		Started:       o.statsStarted.Load(),
		Success:       o.statsSuccess.Load(),
		Failed:        o.statsFailed.Load(),
		Skipped:       o.statsSkipped.Load(),
		InFlight:      o.statsRunning.Load(),
	}
}
