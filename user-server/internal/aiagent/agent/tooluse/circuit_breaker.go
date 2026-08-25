package tooluse

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	FailureThreshold    int
	BaseCooldown        time.Duration
	MaxCooldown         time.Duration
	BackoffMultiplier   float64
	HalfOpenMaxAttempts int
}

// DefaultCircuitBreakerConfig 默认熔断器配置（含指数退避）
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    5,
		BaseCooldown:        30 * time.Second,
		MaxCooldown:         5 * time.Minute,
		BackoffMultiplier:   2.0,
		HalfOpenMaxAttempts: 1,
	}
}

// toolCircuit 单工具的熔断器实例
type toolCircuit struct {
	state            atomic.Int32
	consecutiveFails atomic.Int32
	openedAt         atomic.Int64
	halfOpenAttempts atomic.Int32
	openCount        atomic.Int32
}

func newToolCircuit() *toolCircuit {
	c := &toolCircuit{}
	c.state.Store(int32(CircuitClosed))
	return c
}

// calculateCooldown 计算指数退避冷却时间
func calculateCooldown(cfg CircuitBreakerConfig, openCount int32) time.Duration {
	if openCount <= 1 {
		return cfg.BaseCooldown
	}
	cooldown := float64(cfg.BaseCooldown) * math.Pow(cfg.BackoffMultiplier, float64(openCount-1))
	if cooldown > float64(cfg.MaxCooldown) {
		cooldown = float64(cfg.MaxCooldown)
	}
	return time.Duration(cooldown)
}

// Allow 判断是否允许请求通过
func (c *toolCircuit) Allow(now time.Time, cfg CircuitBreakerConfig) bool {
	state := CircuitState(c.state.Load())

	switch state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		openedAt := time.Unix(0, c.openedAt.Load())
		openCount := c.openCount.Load()
		cooldown := calculateCooldown(cfg, openCount)
		if now.Sub(openedAt) >= cooldown {
			if c.state.CompareAndSwap(int32(CircuitOpen), int32(CircuitHalfOpen)) {
				c.halfOpenAttempts.Store(0)
			}
			state = CircuitState(c.state.Load())
			if state == CircuitOpen {
				return false
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
func (c *toolCircuit) RecordSuccess() {
	c.consecutiveFails.Store(0)
	state := CircuitState(c.state.Load())
	if state == CircuitHalfOpen {
		c.state.Store(int32(CircuitClosed))
		c.halfOpenAttempts.Store(0)
		c.openCount.Store(0)
	}
}

// RecordFailure 记录一次失败
func (c *toolCircuit) RecordFailure(now time.Time, cfg CircuitBreakerConfig) {
	state := CircuitState(c.state.Load())

	if state == CircuitHalfOpen {
		c.state.Store(int32(CircuitOpen))
		c.openedAt.Store(now.UnixNano())
		c.openCount.Add(1)
		return
	}

	fails := c.consecutiveFails.Add(1)
	if int(fails) >= cfg.FailureThreshold {
		if c.state.CompareAndSwap(int32(CircuitClosed), int32(CircuitOpen)) {
			c.openedAt.Store(now.UnixNano())
			c.openCount.Add(1)
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

// OpenCount 获取熔断次数
func (c *toolCircuit) OpenCount() int32 {
	return c.openCount.Load()
}

// CircuitBreakerRegistry 熔断器注册中心
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
	if cfg.BackoffMultiplier < 1.0 {
		cfg.BackoffMultiplier = 2.0
	}
	if cfg.MaxCooldown <= 0 {
		cfg.MaxCooldown = 5 * time.Minute
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

// ToolCircuitInfo 工具熔断信息
type ToolCircuitInfo struct {
	State            CircuitState `json:"state"`
	ConsecutiveFails int32         `json:"consecutive_fails"`
	OpenCount        int32         `json:"open_count"`
}

// AllStates 查询所有工具的熔断状态
func (r *CircuitBreakerRegistry) AllStates() map[string]ToolCircuitInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]ToolCircuitInfo, len(r.circuits))
	for name, c := range r.circuits {
		out[name] = ToolCircuitInfo{
			State:            c.State(),
			ConsecutiveFails: c.ConsecutiveFails(),
			OpenCount:        c.OpenCount(),
		}
	}
	return out
}

// Config 返回当前配置
func (r *CircuitBreakerRegistry) Config() CircuitBreakerConfig {
	return r.cfg
}

// CircuitBreakerDecorator 熔断器装饰器
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

// NoOpCircuitBreakerRegistry 空操作熔断器
type NoOpCircuitBreakerRegistry struct{}

func (NoOpCircuitBreakerRegistry) Allow(toolName string) bool                { return true }
func (NoOpCircuitBreakerRegistry) RecordSuccess(toolName string)             {}
func (NoOpCircuitBreakerRegistry) RecordFailure(toolName string)             {}
func (NoOpCircuitBreakerRegistry) State(toolName string) CircuitState        { return CircuitClosed }
func (NoOpCircuitBreakerRegistry) AllStates() map[string]ToolCircuitInfo    { return nil }
func (NoOpCircuitBreakerRegistry) Config() CircuitBreakerConfig              { return DefaultCircuitBreakerConfig() }
