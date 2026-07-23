package tooluse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// dead_letter.go P2-C: 工具调用死信队列
//
// 设计目标：
//   当工具调用最终失败（重试耗尽、超时、panic 等）时，将失败请求持久化到死信队列，
//   便于后续人工介入或自动重放。避免失败请求被默默丢弃，导致业务数据丢失。
//
// 适用场景：
//   - 写入类工具失败（order.create 失败需人工对账）
//   - 外部 API 调用失败（飞书/企微 send_message 失败需重放）
//   - 关键业务工具失败（order.create 失败需排查）
//
// 设计要点：
//   1. 内存实现（生产环境可替换为 Redis/DB 持久化）
//   2. 容量上限（默认 10000 条，超过丢弃最旧）
//   3. 按 tool_name 分组查询（便于按业务排查）
//   4. TTL 过期清理（默认 7 天）
//   5. 支持重放（重新执行失败的 tool_call）

// ===== 死信条目 =====

// DeadLetterEntry 死信条目
type DeadLetterEntry struct {
	ID         string           `json:"id"`          // 唯一 ID（用于重放）
	ToolName   string           `json:"tool_name"`   // 工具名
	Args       map[string]any   `json:"args"`        // 调用参数（用于重放）
	Error      string           `json:"error"`       // 错误信息
	TraceID    string           `json:"trace_id"`    // 追踪 ID（贯穿 Agent Loop）
	ToolCtx    *ToolContext     `json:"tool_ctx"`    // 工具上下文（含 agent_id/session_id 等）
	RetryCount int              `json:"retry_count"` // 已重试次数
	Status     DeadLetterStatus `json:"status"`      // 处理状态
	CreatedAt  time.Time        `json:"created_at"`  // 创建时间
	UpdatedAt  time.Time        `json:"updated_at"`  // 最后更新时间
}

// DeadLetterStatus 死信处理状态
type DeadLetterStatus string

const (
	// DeadLetterPending 待处理
	DeadLetterPending DeadLetterStatus = "pending"
	// DeadLetterReplaying 重放中
	DeadLetterReplaying DeadLetterStatus = "replaying"
	// DeadLetterReplayed 已重放成功
	DeadLetterReplayed DeadLetterStatus = "replayed"
	// DeadLetterReplayFailed 重放失败
	DeadLetterReplayFailed DeadLetterStatus = "replay_failed"
	// DeadLetterDiscarded 已丢弃（人工标记不处理）
	DeadLetterDiscarded DeadLetterStatus = "discarded"
)

// ===== 死信队列 =====

// DeadLetterQueue 死信队列（内存实现）
//
// 线程安全；生产环境可替换为持久化实现（Redis/DB）
type DeadLetterQueue struct {
	mu      sync.Mutex
	entries []*DeadLetterEntry
	byID    map[string]*DeadLetterEntry
	byTool  map[string][]*DeadLetterEntry // 按工具名分组索引
	maxSize int
	ttl     time.Duration
	idSeq   atomic.Uint64
}

// NewDeadLetterQueue 创建死信队列
//
// 参数：
//   - maxSize: 最大条目数（默认 10000）
//   - ttl: 条目过期时间（默认 7 天）
func NewDeadLetterQueue(maxSize int, ttl time.Duration) *DeadLetterQueue {
	if maxSize <= 0 {
		maxSize = 10000
	}
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return &DeadLetterQueue{
		entries: make([]*DeadLetterEntry, 0, maxSize),
		byID:    make(map[string]*DeadLetterEntry),
		byTool:  make(map[string][]*DeadLetterEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Push 将失败的 tool_call 推入死信队列
//
// 参数：
//   - toolName: 工具名
//   - args: 调用参数
//   - err: 失败错误
//   - traceID: 追踪 ID
//   - toolCtx: 工具上下文
//   - retryCount: 已重试次数
//
// 返回：死信条目 ID（用于后续查询/重放）
func (q *DeadLetterQueue) Push(
	toolName string,
	args map[string]any,
	err error,
	traceID string,
	toolCtx *ToolContext,
	retryCount int,
) string {
	q.mu.Lock()
	defer q.mu.Unlock()

	id := fmt.Sprintf("dlq_%d_%d", time.Now().UnixNano(), q.idSeq.Add(1))
	entry := &DeadLetterEntry{
		ID:         id,
		ToolName:   toolName,
		Args:       args,
		Error:      "",
		TraceID:    traceID,
		ToolCtx:    toolCtx,
		RetryCount: retryCount,
		Status:     DeadLetterPending,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err != nil {
		entry.Error = err.Error()
	}

	q.entries = append(q.entries, entry)
	q.byID[id] = entry
	q.byTool[toolName] = append(q.byTool[toolName], entry)

	// LRU 淘汰：超过 maxSize 时删除最旧
	if len(q.entries) > q.maxSize {
		oldest := q.entries[0]
		q.entries = q.entries[1:]
		delete(q.byID, oldest.ID)
		// 从 byTool 索引删除
		if toolList, ok := q.byTool[oldest.ToolName]; ok && len(toolList) > 0 {
			q.byTool[oldest.ToolName] = toolList[1:]
		}
	}

	return id
}

// Get 按 ID 查询死信
func (q *DeadLetterQueue) Get(id string) (*DeadLetterEntry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, ok := q.byID[id]
	return entry, ok
}

// ListByTool 按工具名查询死信列表
func (q *DeadLetterQueue) ListByTool(toolName string) []*DeadLetterEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]*DeadLetterEntry, len(q.byTool[toolName]))
	copy(out, q.byTool[toolName])
	return out
}

// ListPending 查询所有待处理死信
func (q *DeadLetterQueue) ListPending() []*DeadLetterEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]*DeadLetterEntry, 0, len(q.entries))
	for _, e := range q.entries {
		if e.Status == DeadLetterPending {
			out = append(out, e)
		}
	}
	return out
}

// ListAll 查询所有死信（按时间倒序）
func (q *DeadLetterQueue) ListAll() []*DeadLetterEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]*DeadLetterEntry, len(q.entries))
	copy(out, q.entries)
	// 倒序（最新在前）
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// UpdateStatus 更新死信状态
func (q *DeadLetterQueue) UpdateStatus(id string, status DeadLetterStatus) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, ok := q.byID[id]
	if !ok {
		return fmt.Errorf("dead letter entry not found: %s", id)
	}
	entry.Status = status
	entry.UpdatedAt = time.Now()
	return nil
}

// Cleanup 清理过期条目
func (q *DeadLetterQueue) Cleanup() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	removed := 0
	newEntries := make([]*DeadLetterEntry, 0, len(q.entries))
	for _, e := range q.entries {
		if now.Sub(e.CreatedAt) > q.ttl {
			delete(q.byID, e.ID)
			removed++
			continue
		}
		newEntries = append(newEntries, e)
	}
	q.entries = newEntries
	// 重建 byTool 索引
	q.byTool = make(map[string][]*DeadLetterEntry)
	for _, e := range q.entries {
		q.byTool[e.ToolName] = append(q.byTool[e.ToolName], e)
	}
	return removed
}

// Stats 返回死信队列统计
func (q *DeadLetterQueue) Stats() DeadLetterStats {
	q.mu.Lock()
	defer q.mu.Unlock()
	stats := DeadLetterStats{
		Total:    len(q.entries),
		ByTool:   make(map[string]int),
		ByStatus: make(map[DeadLetterStatus]int),
		MaxSize:  q.maxSize,
	}
	for _, e := range q.entries {
		stats.ByTool[e.ToolName]++
		stats.ByStatus[e.Status]++
	}
	return stats
}

// DeadLetterStats 死信队列统计
type DeadLetterStats struct {
	Total    int                      `json:"total"`
	ByTool   map[string]int           `json:"by_tool"`
	ByStatus map[DeadLetterStatus]int `json:"by_status"`
	MaxSize  int                      `json:"max_size"`
}

// ===== 装饰器：死信队列装饰器 =====

// DeadLetterQueueDecorator 死信队列装饰器
//
// 在工具调用最终失败时（重试耗尽、超时、panic 等）推入死信队列
// 装饰器链位置：权限 → 限流 → 熔断 → 参数校验 → 缓存 → 重试 → 超时 → 审计 → 死信队列
//   - 死信队列在最后：仅捕获最终失败结果
//   - 已成功的调用不推入死信队列
//
// 设计说明：
//
//	死信队列装饰器位于审计之后，确保失败结果先被审计记录，
//	再由死信队列持久化便于后续重放。
//	但为了简化实现，本装饰器直接在装饰器链最后（即最接近 handler 的位置），
//	避免与其他装饰器（重试/超时）冲突。
func DeadLetterQueueDecorator(queue *DeadLetterQueue) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			result, err := next(ctx, args)

			// 仅在最终失败时推入死信队列
			if queue == nil {
				return result, err
			}
			if err == nil && result.Success {
				return result, err
			}

			// 跳过不可重试的错误（业务错误不需要死信重放）
			if isNonRetryableError(err) || isNonRetryableResult(result) {
				return result, err
			}

			// 推入死信队列
			toolName := GetToolName(ctx)
			traceID := GetTraceID(ctx)
			toolCtx := GetToolContext(ctx)
			queue.Push(toolName, args, err, traceID, toolCtx, result.Timing.RetryCount)

			return result, err
		}
	}
}

// ===== 内存死信队列（NoOp）=====

// NoOpDeadLetterQueue 空操作死信队列
// 用于不需要死信的场景（如单元测试、查询类工具）
type NoOpDeadLetterQueue struct{}

func (NoOpDeadLetterQueue) Push(toolName string, args map[string]any, err error, traceID string, toolCtx *ToolContext, retryCount int) string {
	return ""
}
func (NoOpDeadLetterQueue) Get(id string) (*DeadLetterEntry, bool)                { return nil, false }
func (NoOpDeadLetterQueue) ListByTool(toolName string) []*DeadLetterEntry         { return nil }
func (NoOpDeadLetterQueue) ListPending() []*DeadLetterEntry                       { return nil }
func (NoOpDeadLetterQueue) ListAll() []*DeadLetterEntry                           { return nil }
func (NoOpDeadLetterQueue) UpdateStatus(id string, status DeadLetterStatus) error { return nil }
func (NoOpDeadLetterQueue) Cleanup() int                                          { return 0 }
func (NoOpDeadLetterQueue) Stats() DeadLetterStats                                { return DeadLetterStats{} }

// ===== 死信重放器 =====

// DeadLetterReplayer 死信重放器
//
// 从死信队列读取待处理条目，通过 ToolExecutor 重新执行
type DeadLetterReplayer struct {
	queue    *DeadLetterQueue
	executor *ToolExecutor
}

// NewDeadLetterReplayer 创建死信重放器
func NewDeadLetterReplayer(queue *DeadLetterQueue, executor *ToolExecutor) *DeadLetterReplayer {
	return &DeadLetterReplayer{queue: queue, executor: executor}
}

// Replay 重放指定死信
//
// 流程：
//  1. 标记死信为 replaying
//  2. 通过 ToolExecutor 重新执行
//  3. 成功：标记为 replayed；失败：标记为 replay_failed
func (r *DeadLetterReplayer) Replay(ctx context.Context, id string) error {
	// 标记为 replaying
	if err := r.queue.UpdateStatus(id, DeadLetterReplaying); err != nil {
		return fmt.Errorf("update status to replaying failed: %w", err)
	}

	entry, ok := r.queue.Get(id)
	if !ok {
		return fmt.Errorf("dead letter entry not found: %s", id)
	}

	// 重新执行
	execResult := r.executor.Execute(ctx, ExecuteRequest{
		ToolName: entry.ToolName,
		Args:     entry.Args,
		ToolCtx:  entry.ToolCtx,
	})

	// 更新状态
	if execResult.Err == nil && execResult.Success {
		_ = r.queue.UpdateStatus(id, DeadLetterReplayed)
		return nil
	}
	_ = r.queue.UpdateStatus(id, DeadLetterReplayFailed)
	if execResult.Err != nil {
		return fmt.Errorf("replay failed: %w", execResult.Err)
	}
	return fmt.Errorf("replay failed: %s", execResult.Error)
}

// ReplayAll 重放所有待处理死信
//
// 返回：成功数、失败数
func (r *DeadLetterReplayer) ReplayAll(ctx context.Context) (succeeded, failed int) {
	pending := r.queue.ListPending()
	for _, entry := range pending {
		if err := r.Replay(ctx, entry.ID); err != nil {
			failed++
		} else {
			succeeded++
		}
	}
	return
}

// ===== JSON 序列化（用于持久化）=====

// MarshalJSON 序列化死信条目
func (e *DeadLetterEntry) MarshalJSON() ([]byte, error) {
	type alias DeadLetterEntry
	return json.Marshal((*alias)(e))
}
