package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/logger"
)

// Saga 补偿状态
//
// 业界依据：Garcia-Molina, H. & Salem, K. (1987). "Sagas".
//   - 经典 SAGA 模式：每个 Activity 都有对应的 Compensation
//   - 失败时按反向顺序执行 Compensation，最终一致性
//   - 本实现是 saga 在 SOP 引擎内的轻量化版本

// 补偿状态常量
const (
	CompensationStatusPending   = "pending"   // 待执行
	CompensationStatusRunning   = "running"   // 执行中
	CompensationStatusCompleted = "completed" // 已完成
	CompensationStatusFailed    = "failed"    // 失败
	CompensationStatusSkipped   = "skipped"   // 跳过（无补偿逻辑）
)

// Compensable 节点支持 Saga 补偿的可选接口
//
// 实现者：需要"撤销/回滚"的节点（如发短信 → 补偿删除消息；写 DB → 补偿删除记录；
// 调外部 API → 补偿取消订单）。
//
// Compensate 设计原则：
//   - 必须幂等（可能重试）
//   - 必须有界（不能无限阻塞）
//   - 失败允许（失败不阻断其他补偿，但记日志）
//
// 与 Execute 的区别：Execute 是"做"，Compensate 是"撤销"。
// 业界 SAGA 经典：每个 Activity 都有 Compensation，定义在同一个接口里。
type Compensable interface {
	// Compensate 执行补偿
	// 返回 nil 表示补偿成功；返回 error 表示失败（可重试）
	// ctx 可能已被 cancel（worker 关闭时）
	Compensate(ctx context.Context, execCtx *ExecutionContext) error
}

// CompensationRecord 单个节点的补偿记录
type CompensationRecord struct {
	NodeID     string    `json:"node_id"`
	NodeType   string    `json:"node_type"`
	Status     string    `json:"status"`
	Attempt    int       `json:"attempt"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Error      string    `json:"error,omitempty"`
	TraceID    string    `json:"trace_id,omitempty"`
}

// CompensationPlan 一次完整补偿计划
type CompensationPlan struct {
	ExecutionID uint                 `json:"execution_id"`
	StartedAt   time.Time            `json:"started_at"`
	FinishedAt  time.Time            `json:"finished_at,omitempty"`
	Records     []CompensationRecord `json:"records"`
	Status      string               `json:"status"`
}

// CompensationManager Saga 补偿管理器
//
// 线程安全：支持多 Execution 并发补偿
type CompensationManager struct {
	mu     sync.RWMutex
	plans  map[uint]*CompensationPlan // executionID -> plan
	config CompensationConfig
}

// CompensationConfig 补偿配置
type CompensationConfig struct {
	MaxAttempts        int           // 单节点最大补偿尝试次数
	PerCompensationTTL time.Duration // 单节点补偿超时
	TotalTimeout       time.Duration // 整个补偿流程总超时
}

// DefaultCompensationConfig 默认配置
//
// 业界依据：
//   - 单节点补偿应比原 Activity 执行时间短（撤销操作通常更快）
//   - 总超时留 5 分钟（与 execute 总超时同数量级）
func DefaultCompensationConfig() CompensationConfig {
	return CompensationConfig{
		MaxAttempts:        3,
		PerCompensationTTL: 30 * time.Second,
		TotalTimeout:       5 * time.Minute,
	}
}

// NewCompensationManager 构造补偿管理器
func NewCompensationManager(cfg CompensationConfig) *CompensationManager {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.PerCompensationTTL <= 0 {
		cfg.PerCompensationTTL = 30 * time.Second
	}
	if cfg.TotalTimeout <= 0 {
		cfg.TotalTimeout = 5 * time.Minute
	}
	return &CompensationManager{
		plans:  make(map[uint]*CompensationPlan),
		config: cfg,
	}
}

// Plan 构造补偿计划：按 executed 节点的反向顺序构造
//
// inputs:
//   - executedNodes: 按执行顺序的节点列表（成功完成或被跳过）
//   - failedNode: 失败节点（不参与补偿，但其前序节点要补偿）
//
// 返回：反向顺序的节点列表（先补偿最后执行的，最后补偿最早执行的）
func (m *CompensationManager) Plan(executedNodes []CompensationRecord) []CompensationRecord {
	// 反向：拷贝避免修改原 slice
	reversed := make([]CompensationRecord, len(executedNodes))
	for i, n := range executedNodes {
		reversed[len(executedNodes)-1-i] = n
	}
	return reversed
}

// CompensateNode 补偿单个节点（带重试和超时）
//
// 业界 pattern（来自 Temporal / Cadence）：补偿是 activity-level 操作，
// 失败可重试；总超时防止补偿卡死整个 execution。
func (m *CompensationManager) CompensateNode(
	ctx context.Context,
	execCtx *ExecutionContext,
	executor NodeExecutor,
) CompensationRecord {
	rec := CompensationRecord{
		NodeID:    execCtx.Node.ID,
		NodeType:  execCtx.Node.Type,
		Status:    CompensationStatusRunning,
		StartedAt: time.Now(),
		TraceID:   execCtx.TraceID,
	}

	// 检查是否实现 Compensable
	comp, ok := executor.(Compensable)
	if !ok {
		rec.Status = CompensationStatusSkipped
		rec.FinishedAt = time.Now()
		logger.Ctx(ctx).Info().
			Str("node_id", rec.NodeID).
			Str("node_type", rec.NodeType).
			Msg("[Compensation] node not compensable, skipped")
		return rec
	}

	// 重试循环
	var lastErr error
	for attempt := 1; attempt <= m.config.MaxAttempts; attempt++ {
		rec.Attempt = attempt

		// 单次补偿超时
		compCtx, cancel := context.WithTimeout(ctx, m.config.PerCompensationTTL)
		err := comp.Compensate(compCtx, execCtx)
		cancel()

		if err == nil {
			rec.Status = CompensationStatusCompleted
			rec.FinishedAt = time.Now()
			logger.Ctx(ctx).Info().
				Str("node_id", rec.NodeID).
				Str("node_type", rec.NodeType).
				Int("attempt", attempt).
				Dur("duration", time.Since(rec.StartedAt)).
				Msg("[Compensation] node compensated")
			return rec
		}

		lastErr = err
		logger.Ctx(ctx).Warn().
			Err(err).
			Str("node_id", rec.NodeID).
			Int("attempt", attempt).
			Int("max_attempts", m.config.MaxAttempts).
			Msg("[Compensation] node compensate failed, will retry")

		// 重试间隔（指数退避）
		if attempt < m.config.MaxAttempts {
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				rec.Status = CompensationStatusFailed
				rec.Error = "context cancelled"
				rec.FinishedAt = time.Now()
				return rec
			}
		}
	}

	rec.Status = CompensationStatusFailed
	rec.Error = lastErr.Error()
	rec.FinishedAt = time.Now()
	return rec
}

// Run 启动一次完整补偿流程
//
// inputs:
//   - executionID: SOP execution ID
//   - plan: 补偿节点列表（应已按反向排序）
//   - getExecutor: 通过 nodeType 获取 executor（与 NodeExecutorRegistry 兼容）
//   - execCtxFor: 通过 nodeID 构造 ExecutionContext
//
// 返回：完成的 CompensationPlan
//
// 业界特性：
//   - 单节点失败不阻断其他补偿（best-effort）
//   - 总超时：超时强制结束，防止永久卡死
//   - 终态记录：所有尝试都可在 plan 中回放
func (m *CompensationManager) Run(
	ctx context.Context,
	executionID uint,
	plan []CompensationRecord,
	getExecutor func(nodeType string) NodeExecutor,
	execCtxFor func(nodeID string) *ExecutionContext,
) *CompensationPlan {
	totalCtx, cancel := context.WithTimeout(ctx, m.config.TotalTimeout)
	defer cancel()

	result := &CompensationPlan{
		ExecutionID: executionID,
		StartedAt:   time.Now(),
		Records:     make([]CompensationRecord, 0, len(plan)),
		Status:      CompensationStatusRunning,
	}

	m.mu.Lock()
	m.plans[executionID] = result
	m.mu.Unlock()

	defer func() {
		result.FinishedAt = time.Now()
		// 计算整体状态
		hasFailed := false
		hasCompleted := false
		for _, r := range result.Records {
			if r.Status == CompensationStatusFailed {
				hasFailed = true
			}
			if r.Status == CompensationStatusCompleted {
				hasCompleted = true
			}
		}
		if hasFailed && !hasCompleted {
			result.Status = CompensationStatusFailed
		} else if hasFailed {
			result.Status = "partial" // 部分成功
		} else {
			result.Status = CompensationStatusCompleted
		}
		logger.Ctx(ctx).Info().
			Uint("execution_id", executionID).
			Str("status", result.Status).
			Int("records", len(result.Records)).
			Msg("[Compensation] run finished")
	}()

	for _, planned := range plan {
		if totalCtx.Err() != nil {
			logger.Ctx(ctx).Warn().Msg("[Compensation] total timeout, abort remaining")
			// 记录被跳过的节点（运维可观察）
			result.Records = append(result.Records, CompensationRecord{
				NodeID:     planned.NodeID,
				NodeType:   planned.NodeType,
				Status:     CompensationStatusSkipped,
				Error:      "aborted: " + totalCtx.Err().Error(),
				FinishedAt: time.Now(),
			})
			continue
		}

		execCtx := execCtxFor(planned.NodeID)
		if execCtx == nil {
			result.Records = append(result.Records, CompensationRecord{
				NodeID:     planned.NodeID,
				Status:     CompensationStatusFailed,
				Error:      "no execution context available",
				FinishedAt: time.Now(),
			})
			continue
		}

		executor := getExecutor(planned.NodeType)
		if executor == nil {
			result.Records = append(result.Records, CompensationRecord{
				NodeID:     planned.NodeID,
				NodeType:   planned.NodeType,
				Status:     CompensationStatusSkipped,
				FinishedAt: time.Now(),
			})
			continue
		}

		rec := m.CompensateNode(totalCtx, execCtx, executor)
		result.Records = append(result.Records, rec)
	}

	return result
}

// GetPlan 查询补偿计划
func (m *CompensationManager) GetPlan(executionID uint) *CompensationPlan {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.plans[executionID]
}

// Summary 输出补偿计划摘要（用于监控/调试）
type CompensationSummary struct {
	TotalPlans     int `json:"total_plans"`
	CompletedPlans int `json:"completed_plans"`
	FailedPlans    int `json:"failed_plans"`
	PartialPlans   int `json:"partial_plans"`
	TotalNodes     int `json:"total_nodes"`
	FailedNodes    int `json:"failed_nodes"`
}

// Summary 全局补偿摘要
func (m *CompensationManager) Summary() CompensationSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	summary := CompensationSummary{TotalPlans: len(m.plans)}
	for _, p := range m.plans {
		summary.TotalNodes += len(p.Records)
		switch p.Status {
		case CompensationStatusCompleted:
			summary.CompletedPlans++
		case CompensationStatusFailed:
			summary.FailedPlans++
		case "partial":
			summary.PartialPlans++
		}
		for _, r := range p.Records {
			if r.Status == CompensationStatusFailed {
				summary.FailedNodes++
			}
		}
	}
	return summary
}

// 编译器断言：确保 dto.SOPNode 与我们的 NodeType 字段匹配
// （非必需但提高代码可读性）
var _ = func() *dto.SOPNode {
	return &dto.SOPNode{Type: "compensable_test"}
}

// dummy use of fmt to keep import
var _ = fmt.Sprintf
