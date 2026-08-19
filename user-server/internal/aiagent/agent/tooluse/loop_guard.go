package tooluse

import (
	"context"
	"fmt"
	"sync"
	"time"
)



// ErrLoopDetected 工具调用循环被检测到
var ErrLoopDetected = fmt.Errorf("tool loop detected: same tool called too many times with equivalent args")


// LoopGuardConfig 循环检测配置
type LoopGuardConfig struct {
	MaxRepeatCount int
	WindowSize time.Duration
	MaxTraces int
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


// LoopGuard 工具调用循环检测器
//
// 线程安全；按 trace_id 维度跟踪调用历史
type LoopGuard struct {
	mu     sync.Mutex
	traces map[string]*traceHistory 
	config LoopGuardConfig
}

type traceHistory struct {
	calls    []callRecord 
	lastSeen time.Time    
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

	argsHash := structuralFingerprint(args)
	key := traceID
	if key == "" {
		key = "_no_trace_:" + toolName
	}

	now := time.Now()
	windowStart := now.Add(-g.config.WindowSize)

	g.mu.Lock()
	defer g.mu.Unlock()

	history, ok := g.traces[key]
	if !ok {
		history = &traceHistory{calls: make([]callRecord, 0, 8)}
		g.traces[key] = history

		if len(g.traces) > g.config.MaxTraces {
			g.evictOldestLocked()
		}
	}
	history.lastSeen = now

	dedupeIdx := 0
	for i, c := range history.calls {
		if c.timestamp.After(windowStart) {
			history.calls[dedupeIdx] = history.calls[i]
			dedupeIdx++
		}
	}
	history.calls = history.calls[:dedupeIdx]

	repeatCount := 0
	for _, c := range history.calls {
		if c.toolName == toolName && c.argsHash == argsHash {
			repeatCount++
		}
	}

	if repeatCount >= g.config.MaxRepeatCount {
		return ErrLoopDetected
	}

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
	ActiveTraces int `json:"active_traces"` 
	TotalRecords int `json:"total_records"` 
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

// hashArgs 保留为旧名（不导出），实际委托给 structuralFingerprint。
// 删去实现以避免分叉；外部如需 hash 调用 structuralFingerprint。
// 本函数保留仅为二进制兼容（被同包内其它代码引用）。
func hashArgs(args map[string]any) string {
	return structuralFingerprint(args)
}


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

			if err := guard.CheckAndRecord(traceID, toolName, args); err != nil {
				return ErrorResult(toolName, err), err
			}

			return next(ctx, args)
		}
	}
}

