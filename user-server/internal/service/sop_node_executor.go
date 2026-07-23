package service

// ============================================================================
// SOP 节点执行器接口与注册中心（P0-1 SOP 节点执行器完善设计）
// ----------------------------------------------------------------------------
// 设计依据：docs/核心链路优化.md 第十三章 §13.2.1
// 私域独立部署：无 merchant_id 字段
// 五层架构：本文件位于 L3 业务层（Service），被 sop_dispatcher.go 调用
//
// 设计要点（Strategy 模式 + DB 状态字段 + Outbox 事件流混合）：
//   - 每种节点类型一个 NodeExecutor 实现，注册表分发（开闭原则）
//   - 14 种节点类型已固化在 sop_service.go SOPNodeType* 常量中
//   - 长任务（wait/timer）通过 sop_timers 表 + OutboxDispatcher 实现
//   - 事件日志（sop_exec_events）支持审计与幂等性
// ============================================================================

import (
	"context"
	"fmt"
	"sync"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// 节点执行结果状态
const (
	NodeStatusCompleted = "completed" // 节点执行完成，可推进下一节点
	NodeStatusWaiting   = "waiting"   // 节点进入等待态（wait/timer 节点）
	NodeStatusFailed    = "failed"    // 节点执行失败
	NodeStatusSkipped   = "skipped"   // 节点被跳过（如幂等命中）
)

// 节点事件类型（写入 sop_exec_events.event_type）
const (
	NodeEventStarted   = "started"   // 节点开始执行
	NodeEventExecuted  = "executed"  // 节点执行完毕
	NodeEventCompleted = "completed" // 节点完成后推进
	NodeEventFailed    = "failed"    // 节点执行失败
	NodeEventWaiting   = "waiting"   // 节点进入等待
	NodeEventRetried   = "retried"   // 节点重试
)

// 等待事件类型（写入 sop_timers.wait_event 与 sop_executions.wait_event）
const (
	WaitEventTimer         = "timer"          // 定时唤醒（wait_seconds/wait_until）
	WaitEventCustomerReply = "customer_reply" // 客户回复唤醒
	WaitEventExternal      = "external"       // 外部事件唤醒
)

// NodeExecutor 节点执行器接口（Strategy 模式）
//
// 每种 SOP 节点类型实现该接口，由 NodeExecutorRegistry 注册并分发。
// 实现方应保证 Execute 方法幂等（同一 ExecutionContext 多次执行结果一致），
// 重试由 SOPExecutionDispatcher 调度，执行器通过 SideEffects 列表标识已发生的副作用。
type NodeExecutor interface {
	// NodeType 返回节点类型字符串（与 dto.SOPNode.Type 对应）
	NodeType() string

	// Execute 执行节点逻辑
	//   - ctx: 链路追踪上下文（携带 trace_id）
	//   - execCtx: 节点执行上下文（包含 Execution/Node/Graph 等）
	//   - 返回 NodeExecResult 描述执行结果与下一节点路由
	Execute(ctx context.Context, execCtx *ExecutionContext) (*NodeExecResult, error)

	// IsAsync 是否异步执行
	//   - true: 调度器不阻塞 Worker Pool，立即派发下一任务（如 wait 节点）
	//   - false: 调度器等待 Execute 返回后再处理结果（默认）
	IsAsync() bool
}

// ExecutionContext 节点执行上下文
//
// 封装节点执行所需的所有上下文信息，避免执行器直接访问数据库。
// 由 SOPExecutionDispatcher 在派发任务时构造。
type ExecutionContext struct {
	Execution     *model.SOPExecution // 当前执行记录
	Node          *dto.SOPNode        // 待执行节点
	Graph         *dto.SOPGraph       // SOP 图（用于查找下一节点）
	CustomerID    string              // 客户 ID
	SessionID     string              // 会话 ID（用于消息持久化与 WS 推送）
	Variant       string              // A/B 测试 variant 名称
	Input         model.JSONMap       // 节点输入（来自上一节点 Output 或 ExecutionData）
	ExecutionData model.JSONMap       // 执行数据（全局共享，节点可读写）
	TraceID       string              // 链路追踪 ID
	StartedAt     time.Time           // 节点开始执行时间
	Attempt       int                 // 当前重试次数（0=首次执行）
}

// NodeExecResult 节点执行结果
type NodeExecResult struct {
	Status       string        // completed / waiting / failed / skipped
	Output       model.JSONMap // 节点输出（合并到 ExecutionData）
	NextNodeID   string        // 显式指定下一节点（覆盖 nextNode 默认逻辑，空表示用默认流转）
	WaitUntil    *time.Time    // Status=waiting 时有效：定时唤醒时间
	WaitEvent    string        // Status=waiting 时有效：timer / customer_reply / external
	ErrorMessage string        // Status=failed 时的错误信息
	Retryable    bool          // 是否可重试（失败时由调度器按 RetryPolicy 决定是否重试）
	SideEffects  []string      // 副作用标识列表（幂等性检查与回滚）
	TokensUsed   int           // LLM token 消耗（用于成本统计）
}

// NodeExecutorRegistry 节点执行器注册中心
//
// 全局唯一实例，启动时通过 Register 注册所有节点执行器。
// 调度器通过 MustGet 获取执行器，未注册类型返回 NoopExecutor 兜底。
type NodeExecutorRegistry struct {
	mu        sync.RWMutex
	executors map[string]NodeExecutor
}

// NewNodeExecutorRegistry 创建注册中心
func NewNodeExecutorRegistry() *NodeExecutorRegistry {
	return &NodeExecutorRegistry{
		executors: make(map[string]NodeExecutor),
	}
}

// Register 注册节点执行器
//
// 同一节点类型重复注册会 panic（启动期错误，及早暴露配置问题）。
func (r *NodeExecutorRegistry) Register(ctx context.Context, e NodeExecutor)  {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.executors[e.NodeType()]; exists {
		panic(fmt.Sprintf("node executor already registered: %s", e.NodeType()))
	}
	r.executors[e.NodeType()] = e
}

// Get 获取节点执行器
func (r *NodeExecutorRegistry) Get(ctx context.Context, nodeType string)  (NodeExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.executors[nodeType]
	if !ok {
		return nil, fmt.Errorf("node executor not found: %s", nodeType)
	}
	return e, nil
}

// MustGet 获取节点执行器，未注册时返回 NoopExecutor 兜底
//
// 兜底策略保证 SOP 流程不因未知节点类型中断，
// NoopExecutor 会记录 warn 日志并将节点标记为 completed 推进下一节点。
func (r *NodeExecutorRegistry) MustGet(ctx context.Context, nodeType string)  NodeExecutor {
	e, err := r.Get(context.Background(), nodeType)
	if err != nil {
		logger.Warnf("node executor not found, using noop: %s", nodeType)
		return &NoopExecutor{nodeType: nodeType}
	}
	return e
}

// AllRegistered 返回所有已注册的节点类型（用于调试与启动校验）
func (r *NodeExecutorRegistry) AllRegistered(ctx context.Context)  []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.executors))
	for k := range r.executors {
		out = append(out, k)
	}
	return out
}

// AllExecutors 返回所有已注册的节点执行器（用于运行时依赖注入）
//
// 调用方应自行使用类型断言过滤关心的执行器类型（如 *MessageNodeBase）。
// 返回的切片在调用瞬间是注册中心的一份快照，后续注册/反注册不影响其内容。
func (r *NodeExecutorRegistry) AllExecutors(ctx context.Context)  []NodeExecutor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]NodeExecutor, 0, len(r.executors))
	for _, e := range r.executors {
		out = append(out, e)
	}
	return out
}

// NoopExecutor 空操作执行器（兜底）
//
// 用于未注册的节点类型：记录 warn 日志，节点标记为 completed，
// 按默认 Next[0] 流转，保证 SOP 流程不中断。
type NoopExecutor struct {
	nodeType string
}

// NodeType 返回节点类型
func (n *NoopExecutor) NodeType() string { return n.nodeType }

// Execute 空操作：返回 completed
func (n *NoopExecutor) Execute(ctx context.Context, execCtx *ExecutionContext) (*NodeExecResult, error) {
	logger.Ctx(ctx).Warn().
		Str("node_type", n.nodeType).
		Str("node_id", execCtx.Node.ID).
		Str("execution_id", fmt.Sprintf("%d", execCtx.Execution.ID)).
		Msg("noop executor: node type not registered, skipping")
	return &NodeExecResult{
		Status: NodeStatusCompleted,
		Output: model.JSONMap{},
	}, nil
}

// IsAsync 同步执行
func (n *NoopExecutor) IsAsync() bool { return false }

// hasSideEffect 检查执行记录中是否已存在指定副作用标识
//
// 用于节点级幂等性保障：如 "message_sent:{execID}:{nodeID}" 标识消息已发送，
// 重试时跳过实际发送，直接返回 completed。
func hasSideEffect(exec *model.SOPExecution, effect string) bool {
	if exec == nil {
		return false
	}
	sideEffects := extractSideEffects(exec.ExecutionData)
	for _, e := range sideEffects {
		if e == effect {
			return true
		}
	}
	return false
}

// extractSideEffects 从 ExecutionData 中提取已发生的副作用列表
//
// 副作用列表存储在 ExecutionData["_side_effects"] 字段。
// 兼容两种类型：
//   - []interface{}：JSON 反序列化后的标准类型（GORM Scan 路径）
//   - []string：直接构造的内存类型（appendSideEffect 路径，单元测试场景）
func extractSideEffects(data model.JSONMap) []string {
	if data == nil {
		return nil
	}
	raw, ok := data["_side_effects"]
	if !ok {
		return nil
	}
	// 优先匹配 []interface{}（JSON 反序列化路径）
	if arr, ok := raw.([]any); ok {
		out := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	// 兼容 []string（直接构造路径）
	if arr, ok := raw.([]string); ok {
		out := make([]string, 0, len(arr))
		out = append(out, arr...)
		return out
	}
	return nil
}

// appendSideEffect 向 ExecutionData 追加副作用标识（去重）
func appendSideEffect(data model.JSONMap, effect string) model.JSONMap {
	if data == nil {
		data = model.JSONMap{}
	}
	existing := extractSideEffects(data)
	for _, e := range existing {
		if e == effect {
			return data // 已存在，跳过
		}
	}
	existing = append(existing, effect)
	data["_side_effects"] = existing
	return data
}
