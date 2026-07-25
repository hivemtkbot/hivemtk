package tooluse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// loop_guard.go P3-B: 工具级循环检测装饰器
//
// 设计目标：
//   检测 LLM Agent Loop 中工具调用陷入死循环的场景，
//   如 LLM 反复调用同一工具（相同参数）N 次仍期望不同结果。
//   当检测到循环时，返回 ErrLoopDetected 错误中断当前工具调用，
//   上层 Agent Loop 据此跳出循环或降级到文本回复。
//
// 设计要点：
//   1. 按 trace_id 维度跟踪调用历史（同一 Agent Loop 内的所有工具调用）
//      trace_id 为空时退化为按 tool_name 维度（兜底）
//   2. 仅记录窗口期内的调用（默认 60s），过期自动清理
//   3. 检测规则：相同 (tool_name, args_hash) 在窗口内出现超过 MaxRepeatCount 次
//      默认 MaxRepeatCount=3（LLM 调用同一工具 3 次仍失败，认为陷入循环）
//   4. 内存实现（生产环境可替换为 Redis 分布式实现）
//   5. 线程安全；并发调用安全
//
// 装饰器链位置（实际执行顺序，外→内）：
//   死信 → 反馈回流 → 循环检测 → 权限 → 限流 → 熔断 → 参数校验 → 缓存 → 重试 → 超时 → 审计计费 → handler
//   （循环检测位于重试之外：避免重试触发的多次调用被误判为循环）
//   （循环检测位于反馈回流之内：循环命中时反馈仍能记录失败事件）
//   （循环检测位于死信之内：ErrLoopDetected 是不可重试错误，死信队列自动跳过）
//
// 与 Agent Loop 的 maxIterations 区别：
//   - agentLoopMaxIterations 限制 LLM 调用轮数（粗粒度）
//   - LoopGuard 限制同一工具相同参数的重复调用（细粒度，精准识别死循环）
//   - 两者协同：LoopGuard 提前中断无效循环，避免浪费 LLM 调用配额

// ===== 错误定义 =====

// ErrLoopDetected 工具调用循环被检测到
var ErrLoopDetected = fmt.Errorf("tool loop detected: same tool called too many times with identical args")

// ===== 配置 =====

// LoopGuardConfig 循环检测配置
type LoopGuardConfig struct {
	// MaxRepeatCount 同一 (tool_name, args_hash) 在窗口内允许的最大重复次数
	// 默认 3（第 4 次调用会被拦截）
	MaxRepeatCount int
	// WindowSize 滑动窗口大小（超过此时间的调用记录自动清理）
	// 默认 60s
	WindowSize time.Duration
	// MaxTraces 最大追踪 trace 数（防止内存无限增长）
	// 默认 10000
	MaxTraces int
	// Enabled 是否启用循环检测
	// false 时所有调用直接放行（零开销）
	Enabled bool
}

// DefaultLoopGuardConfig 默认循环检测配置
func DefaultLoopGuardConfig() LoopGuardConfig {
	return LoopGuardConfig{
		MaxRepeatCount: 3,
		WindowSize:     60 * time.Second,
		MaxTraces:      10000,
		Enabled:        true,
	}
}

// ===== 循环检测器 =====

// LoopGuard 工具调用循环检测器
//
// 线程安全；按 trace_id 维度跟踪调用历史
type LoopGuard struct {
	mu     sync.Mutex
	traces map[string]*traceHistory // trace_id → 调用历史
	config LoopGuardConfig
}

type traceHistory struct {
	calls    []callRecord // 调用记录（按时间顺序）
	lastSeen time.Time    // 最后调用时间（用于过期清理）
}

type callRecord struct {
	toolName  string
	argsHash  string
	timestamp time.Time
}

// NewLoopGuard 创建循环检测器
func NewLoopGuard(config LoopGuardConfig) *LoopGuard {
	if config.MaxRepeatCount <= 0 {
		config.MaxRepeatCount = 3
	}
	if config.WindowSize <= 0 {
		config.WindowSize = 60 * time.Second
	}
	if config.MaxTraces <= 0 {
		config.MaxTraces = 10000
	}
	return &LoopGuard{
		traces: make(map[string]*traceHistory),
		config: config,
	}
}

// CheckAndRecord 检查是否允许调用，并记录本次调用
//
// 返回值：
//   - nil: 允许调用（已记录到历史）
//   - ErrLoopDetected: 检测到循环，拒绝调用（不记录）
func (g *LoopGuard) CheckAndRecord(traceID, toolName string, args map[string]any) error {
	if g == nil || !g.config.Enabled {
		return nil
	}

	argsHash := hashArgs(args)
	key := traceID
	if key == "" {
		// trace_id 为空时退化为按 tool_name 聚合（兜底，避免漏检）
		key = "_no_trace_:" + toolName
	}

	now := time.Now()
	windowStart := now.Add(-g.config.WindowSize)

	g.mu.Lock()
	defer g.mu.Unlock()

	// 过期清理（懒清理：每次调用时清理当前 key 的过期记录）
	history, ok := g.traces[key]
	if !ok {
		history = &traceHistory{calls: make([]callRecord, 0, 8)}
		g.traces[key] = history

		// 容量保护：超过 MaxTraces 时清理最旧的 trace
		if len(g.traces) > g.config.MaxTraces {
			g.evictOldestLocked()
		}
	}
	history.lastSeen = now

	// 清理窗口外的记录
	dedupeIdx := 0
	for i, c := range history.calls {
		if c.timestamp.After(windowStart) {
			history.calls[dedupeIdx] = history.calls[i]
			dedupeIdx++
		}
	}
	history.calls = history.calls[:dedupeIdx]

	// 统计当前 (tool_name, args_hash) 在窗口内的出现次数
	repeatCount := 0
	for _, c := range history.calls {
		if c.toolName == toolName && c.argsHash == argsHash {
			repeatCount++
		}
	}

	// 检测：超过阈值则拒绝
	if repeatCount >= g.config.MaxRepeatCount {
		return ErrLoopDetected
	}

	// 记录本次调用
	history.calls = append(history.calls, callRecord{
		toolName:  toolName,
		argsHash:  argsHash,
		timestamp: now,
	})
	return nil
}

// evictOldestLocked 清理最旧的 trace（已持锁）
func (g *LoopGuard) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	for k, v := range g.traces {
		if oldestKey == "" || v.lastSeen.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.lastSeen
		}
	}
	if oldestKey != "" {
		delete(g.traces, oldestKey)
	}
}

// Reset 清空所有调用历史
func (g *LoopGuard) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.traces = make(map[string]*traceHistory)
}

// Stats 返回当前追踪统计（用于监控/调试）
type LoopGuardStats struct {
	ActiveTraces int `json:"active_traces"` // 活跃 trace 数
	TotalRecords int `json:"total_records"` // 总调用记录数
}

// Stats 返回统计信息
func (g *LoopGuard) Stats() LoopGuardStats {
	g.mu.Lock()
	defer g.mu.Unlock()
	total := 0
	for _, h := range g.traces {
		total += len(h.calls)
	}
	return LoopGuardStats{
		ActiveTraces: len(g.traces),
		TotalRecords: total,
	}
}

// hashArgs 计算参数哈希（用于检测相同参数的重复调用）
func hashArgs(args map[string]any) string {
	if len(args) == 0 {
		return "empty"
	}
	b, _ := json.Marshal(args)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:16]) // 前 16 字节足够
}

// ===== 装饰器 =====

// LoopGuardDecorator 工具级循环检测装饰器
//
// 在工具执行前检查是否陷入循环，若陷入则返回 ErrLoopDetected。
// guard 为 nil 时直接放行（零开销）。
//
// 装饰器链位置：重试之外（避免重试触发的多次调用被误判为循环）
//   - 位于重试之外：重试是同一逻辑调用的多次尝试，不应触发循环检测
//   - 位于反馈回流之内：循环命中时反馈仍能记录失败事件
//   - 位于死信之内：ErrLoopDetected 是不可重试错误，死信队列自动跳过
func LoopGuardDecorator(guard *LoopGuard) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			if guard == nil {
				return next(ctx, args)
			}

			toolName := GetToolName(ctx)
			traceID := GetTraceID(ctx)

			// 检查是否陷入循环
			if err := guard.CheckAndRecord(traceID, toolName, args); err != nil {
				// 循环检测命中：返回错误结果
				return ErrorResult(toolName, err), err
			}

			return next(ctx, args)
		}
	}
}
