package service


import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/websocket"
)

// SOPDispatcherConfig 调度器配置
type SOPDispatcherConfig struct {
	WorkerCount       int           
	QueueCapacity     int           
	LLMConcurrency    int           
	MaxAttempts       int           
	InitialBackoff    time.Duration 
	MaxBackoff        time.Duration 
	BackoffMultiplier float64       
}

// DefaultSOPDispatcherConfig 默认配置
func DefaultSOPDispatcherConfig() *SOPDispatcherConfig {
	return &SOPDispatcherConfig{
		WorkerCount:       16,
		QueueCapacity:     1000,
		LLMConcurrency:    4,
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// dispatchTask 调度任务
type dispatchTask struct {
	ExecutionID uint
	NodeID      string
	Attempt     int
	TraceID     string
}

// SOPExecutionDispatcher SOP 执行调度器
//
// 通过 Worker Pool 并发执行 SOP 节点，全局唯一实例。
// 由 SOPService.Execute / Step 派发任务，由 OutboxDispatcher 派发 timer 唤醒任务。
type SOPExecutionDispatcher struct {
	registry    *NodeExecutorRegistry
	execRepo    *repository.SopExecutionRepository
	agentRepo   *repository.SopAgentRepository
	eventRepo   *repository.SOPExecEventRepository
	msgRepo     *repository.SessionMessageRepository
	sessionRepo *repository.CustomerSessionRepository
	sopService  *SOPService

	dispatchQueue chan *dispatchTask
	workerCount   int
	llmSem        chan struct{}

	retryPolicy *SOPRetryPolicy

	wg      sync.WaitGroup
	stopCh  chan struct{}
	runMu   sync.Mutex
	running bool

	// v3 审计 P0-12 修复：跟踪所有重试 timer 以便 Stop 时全部释放
	retryTimersMu sync.Mutex
	retryTimers   map[*time.Timer]struct{}
}

// registerRetryTimer 注册 timer（v3 审计 P0-12 修复）
func (d *SOPExecutionDispatcher) registerRetryTimer(t *time.Timer) {
	d.retryTimersMu.Lock()
	defer d.retryTimersMu.Unlock()
	if d.retryTimers == nil {
		d.retryTimers = make(map[*time.Timer]struct{})
	}
	d.retryTimers[t] = struct{}{}
}

// unregisterRetryTimer 注销 timer（v3 审计 P0-12 修复）
func (d *SOPExecutionDispatcher) unregisterRetryTimer(t *time.Timer) {
	d.retryTimersMu.Lock()
	defer d.retryTimersMu.Unlock()
	if d.retryTimers != nil {
		delete(d.retryTimers, t)
	}
}

// stopAllRetryTimers 停止所有 timer（v3 审计 P0-12 修复）
func (d *SOPExecutionDispatcher) stopAllRetryTimers() {
	d.retryTimersMu.Lock()
	defer d.retryTimersMu.Unlock()
	for t := range d.retryTimers {
		t.Stop()
	}
	d.retryTimers = make(map[*time.Timer]struct{})
}

// SOPRetryPolicy SOP 节点执行指数退避重试策略
//
// 注意：reach_pipeline.go 已有同名 RetryPolicy 类型（用于触达 pipeline），
// 本结构体专为 SOP 节点执行器设计，故加 SOP 前缀避免冲突。
type SOPRetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
}

// DefaultSOPRetryPolicy 默认 SOP 重试策略
func DefaultSOPRetryPolicy() *SOPRetryPolicy {
	return &SOPRetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
	}
}

// Backoff 计算第 attempt 次重试的退避时间（attempt 从 1 开始）
//
// 标准指数退避：Backoff(N) = InitialBackoff * Multiplier^(N-1)，被 MaxBackoff 封顶。
//   - attempt=1 → InitialBackoff
//   - attempt=2 → InitialBackoff * Multiplier
//   - attempt=3 → InitialBackoff * Multiplier^2
//   - attempt<=0 视为 1
func (p *SOPRetryPolicy) Backoff(ctx context.Context, attempt int) time.Duration {
	if attempt <= 1 {
		return p.InitialBackoff
	}
	d := p.InitialBackoff
	for i := 1; i < attempt; i++ {
		d = time.Duration(float64(d) * p.Multiplier)
		if d >= p.MaxBackoff {
			return p.MaxBackoff
		}
	}
	if d > p.MaxBackoff {
		return p.MaxBackoff
	}
	return d
}

// NewSOPExecutionDispatcher 创建调度器
//
// 不会自动启动 Worker，需调用 Start 启动。
func NewSOPExecutionDispatcher(db *gorm.DB, sopSvc *SOPService, registry *NodeExecutorRegistry, cfg *SOPDispatcherConfig) *SOPExecutionDispatcher {
	if cfg == nil {
		cfg = DefaultSOPDispatcherConfig()
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 16
	}
	if cfg.QueueCapacity <= 0 {
		cfg.QueueCapacity = 1000
	}
	if cfg.LLMConcurrency <= 0 {
		cfg.LLMConcurrency = 4
	}

	d := &SOPExecutionDispatcher{
		registry:      registry,
		execRepo:      repository.NewSopExecutionRepository(db),
		agentRepo:     repository.NewSopAgentRepository(db),
		eventRepo:     repository.NewSOPExecEventRepository(db),
		msgRepo:       repository.NewSessionMessageRepository(),
		sessionRepo:   repository.NewCustomerSessionRepository(),
		sopService:    sopSvc,
		dispatchQueue: make(chan *dispatchTask, cfg.QueueCapacity),
		workerCount:   cfg.WorkerCount,
		llmSem:        make(chan struct{}, cfg.LLMConcurrency),
		retryPolicy: &SOPRetryPolicy{
			MaxAttempts:    cfg.MaxAttempts,
			InitialBackoff: cfg.InitialBackoff,
			MaxBackoff:     cfg.MaxBackoff,
			Multiplier:     cfg.BackoffMultiplier,
		},
		stopCh: make(chan struct{}),
	}

	deps := &SOPNodeExecutorDeps{
		DB:          db,
		WSHub:       nil, 
		MsgRepo:     d.msgRepo,
		SessionRepo: d.sessionRepo,
		LLMSem:      d.llmSem,
	}
	if sopSvc != nil {
		deps.Dispatcher = sopSvc.dispatcher
	}
	RegisterAllNodeExecutors(registry, deps)

	return d
}

// SetWSHub 注入 WebSocket Hub（避免循环依赖，启动后注入）
//
// 由 main.go 在装配 WebSocket Hub 后调用。内部遍历 registry 中所有已注册的
// 消息发送类执行器，调用其 SetWSHub 真正注入到执行器（修复原 #6 不一致点）。
func (d *SOPExecutionDispatcher) SetWSHub(ctx context.Context, hub *websocket.Hub) {
	if d == nil || hub == nil {
		return
	}
	d.replaceMessageExecutorHub(context.Background(), hub)
}

// replaceMessageExecutorHub 替换所有 MessageNodeBase 的 WS Hub
//
// 通过类型断言直接调用 MessageNodeBase.SetWSHub（编译期安全）。
// 遍历 registry.executors，过滤出 *MessageNodeBase 类型。
func (d *SOPExecutionDispatcher) replaceMessageExecutorHub(ctx context.Context, hub *websocket.Hub) {
	if d == nil || d.registry == nil {
		return
	}
	for _, exec := range d.registry.AllExecutors(context.Background()) {
		if mb, ok := exec.(*MessageNodeBase); ok {
			mb.SetWSHub(context.Background(), hub)
		}
	}
}

// Start 启动 Worker Pool
func (d *SOPExecutionDispatcher) Start(ctx context.Context) {
	d.runMu.Lock()
	defer d.runMu.Unlock()
	if d.running {
		return
	}
	d.running = true
	d.stopCh = make(chan struct{})

	for i := 0; i < d.workerCount; i++ {
		d.wg.Add(1)
		go d.worker(context.Background(), i)
	}
	logger.GetLogger().Info().
		Int("worker_count", d.workerCount).
		Int("queue_capacity", cap(d.dispatchQueue)).
		Int("llm_concurrency", cap(d.llmSem)).
		Msg("[SOPExecutionDispatcher] started")
}

// Stop 停止 Worker Pool（等待所有任务完成）
func (d *SOPExecutionDispatcher) Stop(ctx context.Context) {
	d.runMu.Lock()
	if !d.running {
		d.runMu.Unlock()
		return
	}
	d.running = false
	close(d.stopCh)
	d.runMu.Unlock()

	// v3 审计 P0-12 修复：停止所有重试 timer
	d.stopAllRetryTimers()

	d.wg.Wait()
	logger.GetLogger().Info().Msg("[SOPExecutionDispatcher] stopped")
}

// Dispatch 派发任务到调度队列
//
// 队列满时返回错误（背压），调用方应处理（如重试或记录日志）。
// 停止信号（stopCh 关闭）优先于入队，确保停止语义明确。
func (d *SOPExecutionDispatcher) Dispatch(ctx context.Context, task *dispatchTask) error {
	select {
	case <-d.stopCh:
		return fmt.Errorf("dispatcher stopped")
	default:
	}
	select {
	case d.dispatchQueue <- task:
		return nil
	case <-d.stopCh:
		return fmt.Errorf("dispatcher stopped")
	default:
		return fmt.Errorf("dispatch queue full (capacity=%d)", cap(d.dispatchQueue))
	}
}

// DispatchOrLog 派发任务，失败时记录日志（不阻塞调用方）
func (d *SOPExecutionDispatcher) DispatchOrLog(task *dispatchTask) {
	if err := d.Dispatch(context.Background(), task); err != nil {
		logger.GetLogger().Error().Err(err).
			Uint("execution_id", task.ExecutionID).
			Str("node_id", task.NodeID).
			Msg("dispatch task failed")
	}
}

// worker Worker 主循环
func (d *SOPExecutionDispatcher) worker(ctx context.Context, id int) {
	defer d.wg.Done()
	logger.GetLogger().Debug().Int("worker_id", id).Msg("[worker] started")
	for {
		select {
		case <-d.stopCh:
			logger.GetLogger().Debug().Int("worker_id", id).Msg("[worker] stopped")
			return
		case task := <-d.dispatchQueue:
			d.processTask(context.Background(), id, task)
		}
	}
}

// processTask 处理单个调度任务
func (d *SOPExecutionDispatcher) processTask(ctx context.Context, workerID int, task *dispatchTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ctx = logger.WithTraceID(ctx, task.TraceID)
	ctx = logger.WithModule(ctx, "sop_dispatcher")

	start := time.Now()
	logger.Ctx(ctx).Info().
		Int("worker_id", workerID).
		Uint("execution_id", task.ExecutionID).
		Str("node_id", task.NodeID).
		Int("attempt", task.Attempt).
		Msg("[worker] processing task")

	exec, err := d.loadExecution(ctx, task.ExecutionID)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).
			Uint("execution_id", task.ExecutionID).
			Msg("[worker] load execution failed")
		return
	}
	if exec.Status != SOPStatusRunning {
		logger.Ctx(ctx).Info().
			Str("status", exec.Status).
			Msg("[worker] execution not running, skip")
		return
	}

	graph, err := d.loadGraph(ctx, exec)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[worker] load sop graph failed")
		d.handleExecutionError(ctx, exec, err, task)
		return
	}
	node := findNodeByID(graph, task.NodeID)
	if node == nil {
		logger.Ctx(ctx).Error().Str("node_id", task.NodeID).Msg("[worker] node not found in graph")
		d.handleExecutionError(ctx, exec, fmt.Errorf("node not found: %s", task.NodeID), task)
		return
	}

	d.writeExecEvent(ctx, exec, node, NodeEventStarted, task.Attempt, nil, nil, "")

	execCtx := &ExecutionContext{
		Execution:     exec,
		Node:          node,
		Graph:         graph,
		CustomerID:    exec.CustomerID,
		SessionID:     exec.SessionID,
		Variant:       exec.Variant,
		Input:         exec.ExecutionData,
		ExecutionData: exec.ExecutionData,
		TraceID:       task.TraceID,
		StartedAt:     start,
		Attempt:       task.Attempt,
	}

	executor := d.registry.MustGet(ctx, node.Type)
	result, err := executor.Execute(ctx, execCtx)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil || result == nil {
		d.handleNodeFailure(ctx, exec, node, task, err, latencyMs)
		return
	}

	d.writeExecEvent(ctx, exec, node, NodeEventExecuted, task.Attempt, result.Output, result.SideEffects, "")

	switch result.Status {
	case NodeStatusCompleted, NodeStatusSkipped:
		d.handleNodeSuccess(ctx, exec, node, graph, result, task, latencyMs)
	case NodeStatusWaiting:
		d.handleNodeWaiting(ctx, exec, node, result, latencyMs)
	case NodeStatusFailed:
		err := fmt.Errorf("%s", result.ErrorMessage)
		d.handleNodeFailure(ctx, exec, node, task, err, latencyMs)
	default:
		logger.Ctx(ctx).Warn().
			Str("status", result.Status).
			Msg("[worker] unknown node status, treating as completed")
		d.handleNodeSuccess(ctx, exec, node, graph, result, task, latencyMs)
	}
}

// loadExecution 加载 Execution 记录
func (d *SOPExecutionDispatcher) loadExecution(ctx context.Context, execID uint) (*model.SOPExecution, error) {
	return d.execRepo.GetByID(ctx, execID)
}

// loadGraph 加载 SOP 图
func (d *SOPExecutionDispatcher) loadGraph(ctx context.Context, exec *model.SOPExecution) (*dto.SOPGraph, error) {
	if d.sopService == nil {
		return nil, fmt.Errorf("sop service not configured")
	}
	agent, err := d.sopService.Get(ctx, exec.SOPID)
	if err != nil {
		return nil, err
	}
	// 解析 variant graph ID
	var variantGraphID uint
	if exec.Variant != "" {
		cfg := ParseSOPABTestConfig(agent.ABTestConfig)
		if cfg.Enabled {
			for _, v := range cfg.Variants {
				if v.Name == exec.Variant {
					variantGraphID = v.SOPGraphID
					break
				}
			}
		}
	}
	graph, err := d.sopService.loadSOPGraph(context.Background(), agent, variantGraphID)
	if err != nil {
		return nil, err
	}
	return &graph, nil
}

// handleNodeSuccess 处理节点执行成功
func (d *SOPExecutionDispatcher) handleNodeSuccess(ctx context.Context, exec *model.SOPExecution, node *dto.SOPNode, graph *dto.SOPGraph, result *NodeExecResult, task *dispatchTask, latencyMs int64) {
	if exec.ExecutionData == nil {
		exec.ExecutionData = model.JSONMap{}
	}
	for k, v := range result.Output {
		exec.ExecutionData[k] = v
	}
	for _, effect := range result.SideEffects {
		exec.ExecutionData = appendSideEffect(exec.ExecutionData, effect)
	}

	nextNodeID := result.NextNodeID
	if nextNodeID == "" {
		nextNode := nextNode(graph, node, exec.ExecutionData)
		if nextNode == nil {
			d.completeExecution(ctx, exec)
			return
		}
		nextNodeID = nextNode.ID
	} else if nextNodeID == "_end_" {
		d.completeExecution(ctx, exec)
		return
	}

	now := time.Now()
	exec.CurrentNode = nextNodeID
	exec.LastEventAt = &now
	exec.AttemptCount = 0
	exec.WaitEvent = ""
	for i, n := range graph.Nodes {
		if n.ID == nextNodeID {
			exec.CurrentNodeIdx = i
			break
		}
	}
	if err := d.execRepo.Save(ctx, exec); err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[worker] save execution failed")
		return
	}

	d.writeExecEvent(ctx, exec, node, NodeEventCompleted, task.Attempt, result.Output, result.SideEffects, "")

	d.DispatchOrLog(&dispatchTask{
		ExecutionID: exec.ID,
		NodeID:      nextNodeID,
		Attempt:     0,
		TraceID:     task.TraceID,
	})

	logger.Ctx(ctx).Info().
		Str("node_id", node.ID).
		Str("next_node_id", nextNodeID).
		Int64("latency_ms", latencyMs).
		Msg("[worker] node completed, dispatched next")
}

// handleNodeWaiting 处理节点进入等待态
func (d *SOPExecutionDispatcher) handleNodeWaiting(ctx context.Context, exec *model.SOPExecution, node *dto.SOPNode, result *NodeExecResult, latencyMs int64) {
	now := time.Now()
	exec.LastEventAt = &now
	exec.WaitEvent = result.WaitEvent
	exec.AttemptCount = 0
	if err := d.execRepo.Save(ctx, exec); err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[worker] save execution (waiting) failed")
		return
	}

	d.writeExecEvent(ctx, exec, node, NodeEventWaiting, 0, result.Output, nil, "")

	logger.Ctx(ctx).Info().
		Str("node_id", node.ID).
		Str("wait_event", result.WaitEvent).
		Int64("latency_ms", latencyMs).
		Msg("[worker] node waiting")
}

// handleNodeFailure 处理节点执行失败
func (d *SOPExecutionDispatcher) handleNodeFailure(ctx context.Context, exec *model.SOPExecution, node *dto.SOPNode, task *dispatchTask, err error, latencyMs int64) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	d.writeExecEvent(ctx, exec, node, NodeEventFailed, task.Attempt, nil, nil, errMsg)

	if task.Attempt+1 < d.retryPolicy.MaxAttempts {
		backoff := d.retryPolicy.Backoff(context.Background(), task.Attempt+1)
		logger.Ctx(ctx).Warn().
			Str("node_id", node.ID).
			Int("attempt", task.Attempt).
			Int("next_attempt", task.Attempt+1).
			Dur("backoff", backoff).
			Err(err).
			Msg("[worker] node failed, will retry")

		exec.AttemptCount = task.Attempt + 1
		if err := d.execRepo.UpdateAttemptCount(ctx, exec.ID, exec.AttemptCount); err != nil {
			logger.Ctx(ctx).Warn().
				Uint("exec_id", exec.ID).
				Int("attempt_count", exec.AttemptCount).
				Err(err).
				Msg("[worker] update attempt_count failed")
		}

		d.writeExecEvent(ctx, exec, node, NodeEventRetried, task.Attempt+1, nil, nil, errMsg)

		// v3 审计 P0-12 修复：timer 关联 ctx 实现可停止
		// 原：time.AfterFunc(backoff, func() { d.DispatchOrLog(...) })
		//      Stop 后仍 dispatch；timer 资源不释放
		// 新：time.NewTimer + Stop()，并把 stop 注册到 dispatcher
		retryTimer := time.NewTimer(backoff)
		d.registerRetryTimer(retryTimer)
		go func(t *time.Timer) {
			defer d.unregisterRetryTimer(t)
			select {
			case <-t.C:
				d.DispatchOrLog(&dispatchTask{
					ExecutionID: exec.ID,
					NodeID:      node.ID,
					Attempt:     task.Attempt + 1,
					TraceID:     task.TraceID,
				})
			case <-d.stopCh:
				t.Stop()
				return
			}
		}(retryTimer)
		return
	}

	logger.Ctx(ctx).Error().
		Str("node_id", node.ID).
		Int("attempts", task.Attempt+1).
		Err(err).
		Msg("[worker] node failed after max attempts, marking execution as failed")
	d.failExecution(ctx, exec, fmt.Sprintf("node %s failed after %d attempts: %s", node.ID, task.Attempt+1, errMsg))
}

// handleExecutionError 处理 Execution 级别错误（如加载失败）
func (d *SOPExecutionDispatcher) handleExecutionError(ctx context.Context, exec *model.SOPExecution, err error, task *dispatchTask) {
	d.failExecution(ctx, exec, fmt.Sprintf("execution error: %v", err))
}

// completeExecution 标记 Execution 成功完成
func (d *SOPExecutionDispatcher) completeExecution(ctx context.Context, exec *model.SOPExecution) {
	now := time.Now()
	exec.Status = SOPStatusSuccess
	exec.CompletedAt = &now
	exec.LastEventAt = &now
	exec.WaitEvent = ""
	if err := d.execRepo.Save(ctx, exec); err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[worker] mark execution success failed")
		return
	}
	_ = d.agentRepo.IncrementSuccessCount(ctx, exec.SOPID)

	logger.Ctx(ctx).Info().
		Uint("execution_id", exec.ID).
		Msg("[worker] execution completed successfully")
}

// failExecution 标记 Execution 失败
func (d *SOPExecutionDispatcher) failExecution(ctx context.Context, exec *model.SOPExecution, errMsg string) {
	now := time.Now()
	exec.Status = SOPStatusFailed
	exec.CompletedAt = &now
	exec.LastEventAt = &now
	exec.ErrorMessage = errMsg
	exec.WaitEvent = ""
	_ = d.execRepo.Save(ctx, exec)
	logger.Ctx(ctx).Error().
		Uint("execution_id", exec.ID).
		Str("error", errMsg).
		Msg("[worker] execution marked as failed")
}

// writeExecEvent 写入 sop_exec_events 事件日志
//
// 事件日志的幂等性由唯一约束 (execution_id, node_id, attempt) 保证，
// 同一 attempt 重复写入会被数据库拒绝（忽略错误，仅记录日志）。
func (d *SOPExecutionDispatcher) writeExecEvent(ctx context.Context, exec *model.SOPExecution, node *dto.SOPNode, eventType string, attempt int, output model.JSONMap, sideEffects []string, errMsg string) {
	if d.eventRepo == nil {
		return
	}
	event := &model.SOPExecEvent{
		ExecutionID:  exec.ID,
		SOPID:        exec.SOPID,
		NodeID:       node.ID,
		NodeType:     node.Type,
		EventType:    eventType,
		Attempt:      attempt,
		Status:       eventType,
		Input:        exec.ExecutionData,
		Output:       output,
		SideEffects:  sopToJSONArray(sideEffects),
		ErrorMessage: errMsg,
		TraceID:      logger.TraceIDFromContext(ctx),
	}
	if err := d.eventRepo.Create(ctx, event); err != nil {
		logger.Ctx(ctx).Debug().Err(err).
			Str("node_id", node.ID).
			Str("event_type", eventType).
			Msg("[worker] write exec event failed (may be duplicate)")
	}
}

// sopToJSONArray 将 []string 转为 model.JSONArray
//
// 注意：reach_pipeline.go 已有 toJSONArray 函数（参数为 []byte），
// 本函数专为 SOP 节点执行器的 []string 副作用列表设计，故加 sop 前缀避免冲突。
func sopToJSONArray(s []string) model.JSONArray {
	if len(s) == 0 {
		return nil
	}
	out := make(model.JSONArray, 0, len(s))
	for _, v := range s {
		out = append(out, v)
	}
	return out
}


var (
	globalSOPDispatcher *SOPExecutionDispatcher
	sopDispatcherOnce   sync.Once
)

// InitSOPExecutionDispatcher 初始化全局调度器
func InitSOPExecutionDispatcher(db *gorm.DB, sopSvc *SOPService, cfg *SOPDispatcherConfig) *SOPExecutionDispatcher {
	sopDispatcherOnce.Do(func() {
		registry := NewNodeExecutorRegistry()
		globalSOPDispatcher = NewSOPExecutionDispatcher(db, sopSvc, registry, cfg)
		globalSOPDispatcher.Start(context.Background())
	})
	return globalSOPDispatcher
}

// GetSOPExecutionDispatcher 获取全局调度器
func GetSOPExecutionDispatcher() *SOPExecutionDispatcher {
	return globalSOPDispatcher
}

