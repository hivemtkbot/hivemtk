package tooluse

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)



var (
	ErrCircuitOpen = fmt.Errorf("circuit breaker open")
)


// CircuitState 熔断器状态
type CircuitState int32

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// String 状态字符串表示
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half_open"
	}
	return "unknown"
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	FailureThreshold int
	Cooldown time.Duration
	HalfOpenMaxAttempts int
}

// DefaultCircuitBreakerConfig 默认熔断器配置
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    5,
		Cooldown:            30 * time.Second,
		HalfOpenMaxAttempts: 1,
	}
}


// toolCircuit 单工具的熔断器实例
//
// 状态字段使用 atomic 操作保证线程安全
type toolCircuit struct {
	state            atomic.Int32 
	consecutiveFails atomic.Int32 
	openedAt         atomic.Int64 
	halfOpenAttempts atomic.Int32 
}

func newToolCircuit() *toolCircuit {
	c := &toolCircuit{}
	c.state.Store(int32(CircuitClosed))
	return c
}

// Allow 判断是否允许请求通过
//
// 返回：
//   - true: 允许通过（CLOSED 或 HALF_OPEN 试探配额未用尽）
//   - false: 拒绝（OPEN 且冷却时间未到，或 HALF_OPEN 配额已用尽）
func (c *toolCircuit) Allow(now time.Time, cfg CircuitBreakerConfig) bool {
	state := CircuitState(c.state.Load())

	switch state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		openedAt := time.Unix(0, c.openedAt.Load())
		if now.Sub(openedAt) >= cfg.Cooldown {
			if c.state.CompareAndSwap(int32(CircuitOpen), int32(CircuitHalfOpen)) {
				c.halfOpenAttempts.Store(0)
			}
		} else {
			return false
		}
		fallthrough

	case CircuitHalfOpen:
		if c.halfOpenAttempts.Add(1) > int32(cfg.HalfOpenMaxAttempts) {
			return false
		}
		return true
	}

	return true
}

// RecordSuccess 记录一次成功
// CLOSED: 重置失败计数
// HALF_OPEN: 切换回 CLOSED
func (c *toolCircuit) RecordSuccess() {
	c.consecutiveFails.Store(0)
	state := CircuitState(c.state.Load())
	if state == CircuitHalfOpen {
		c.state.Store(int32(CircuitClosed))
		c.halfOpenAttempts.Store(0)
	}
}

// RecordFailure 记录一次失败
// CLOSED: 失败计数 +1，达到阈值则切换到 OPEN
// HALF_OPEN: 立即切换回 OPEN
func (c *toolCircuit) RecordFailure(now time.Time, cfg CircuitBreakerConfig) {
	state := CircuitState(c.state.Load())

	if state == CircuitHalfOpen {
		c.state.Store(int32(CircuitOpen))
		c.openedAt.Store(now.UnixNano())
		return
	}

	fails := c.consecutiveFails.Add(1)
	if int(fails) >= cfg.FailureThreshold {
		if c.state.CompareAndSwap(int32(CircuitClosed), int32(CircuitOpen)) {
			c.openedAt.Store(now.UnixNano())
		}
	}
}

// State 获取当前状态
func (c *toolCircuit) State() CircuitState {
	return CircuitState(c.state.Load())
}

// ConsecutiveFails 获取当前连续失败次数
func (c *toolCircuit) ConsecutiveFails() int32 {
	return c.consecutiveFails.Load()
}


// CircuitBreakerRegistry 熔断器注册中心（按 tool_name 独立熔断）
type CircuitBreakerRegistry struct {
	mu       sync.RWMutex
	circuits map[string]*toolCircuit
	cfg      CircuitBreakerConfig
}

// NewCircuitBreakerRegistry 创建熔断器注册中心
func NewCircuitBreakerRegistry(cfg CircuitBreakerConfig) *CircuitBreakerRegistry {
	if cfg.FailureThreshold <= 0 {
		cfg = DefaultCircuitBreakerConfig()
	}
	return &CircuitBreakerRegistry{
		circuits: make(map[string]*toolCircuit),
		cfg:      cfg,
	}
}

// GetCircuit 获取（或创建）指定工具的熔断器
func (r *CircuitBreakerRegistry) GetCircuit(toolName string) *toolCircuit {
	r.mu.RLock()
	c, ok := r.circuits[toolName]
	r.mu.RUnlock()
	if ok {
		return c
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.circuits[toolName]; ok {
		return c
	}
	c = newToolCircuit()
	r.circuits[toolName] = c
	return c
}

// Allow 判断指定工具是否允许请求通过
func (r *CircuitBreakerRegistry) Allow(toolName string) bool {
	c := r.GetCircuit(toolName)
	return c.Allow(time.Now(), r.cfg)
}

// RecordSuccess 记录指定工具调用成功
func (r *CircuitBreakerRegistry) RecordSuccess(toolName string) {
	c := r.GetCircuit(toolName)
	c.RecordSuccess()
}

// RecordFailure 记录指定工具调用失败
func (r *CircuitBreakerRegistry) RecordFailure(toolName string) {
	c := r.GetCircuit(toolName)
	c.RecordFailure(time.Now(), r.cfg)
}

// State 查询指定工具的熔断状态
func (r *CircuitBreakerRegistry) State(toolName string) CircuitState {
	c := r.GetCircuit(toolName)
	return c.State()
}

// AllStates 查询所有工具的熔断状态（用于 /metrics endpoint 暴露）
func (r *CircuitBreakerRegistry) AllStates() map[string]CircuitState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]CircuitState, len(r.circuits))
	for name, c := range r.circuits {
		out[name] = c.State()
	}
	return out
}

// Config 返回当前配置
func (r *CircuitBreakerRegistry) Config() CircuitBreakerConfig {
	return r.cfg
}


// CircuitBreakerDecorator 熔断器装饰器
//
// 在工具执行前检查熔断状态，工具执行后记录成功/失败
// 装饰器链位置：权限 → 限流 → 熔断 → 重试 → 超时 → 审计
// （熔断在重试之前：避免重试已熔断的工具）
func CircuitBreakerDecorator(registry *CircuitBreakerRegistry) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			if registry == nil {
				return next(ctx, args)
			}
			toolName := GetToolName(ctx)

			if !registry.Allow(toolName) {
				return ErrorResult(toolName, fmt.Errorf("%w: tool=%s state=%s",
					ErrCircuitOpen, toolName, registry.State(toolName))), ErrCircuitOpen
			}

			result, err := next(ctx, args)

			if err != nil || !result.Success {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return result, err
				}
				registry.RecordFailure(toolName)
			} else {
				registry.RecordSuccess(toolName)
			}

			return result, err
		}
	}
}


// NoOpCircuitBreakerRegistry 空操作熔断器（不进行任何熔断）
// 用于不需要熔断的场景（如单元测试、低频调用工具）
type NoOpCircuitBreakerRegistry struct{}

func (NoOpCircuitBreakerRegistry) Allow(toolName string) bool         { return true }
func (NoOpCircuitBreakerRegistry) RecordSuccess(toolName string)      {}
func (NoOpCircuitBreakerRegistry) RecordFailure(toolName string)      {}
func (NoOpCircuitBreakerRegistry) State(toolName string) CircuitState { return CircuitClosed }
func (NoOpCircuitBreakerRegistry) AllStates() map[string]CircuitState { return nil }
func (NoOpCircuitBreakerRegistry) Config() CircuitBreakerConfig       { return DefaultCircuitBreakerConfig() }

