package tooluse

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"marketing/internal/pkg/metrics"
)

// decorator.go 工具执行装饰器链（PRD §5.2 P0-3 G3）
//
// 设计目标：
//  1. 5 装饰器链按固定顺序执行：权限 → 限流 → 重试 → 超时 → 审计计费 → 实际工具执行
//  2. 每个装饰器独立可测试
//  3. 依赖通过接口注入（PermissionChecker/RateLimiter/RetryPolicy/AuditLogger/CostTracker）
//  4. 提供 NoOp 默认实现以便单测
//  5. 装饰器支持 panic 恢复（防止单个工具异常影响整个智能体链路）

// ===== 核心类型 =====

// ToolHandler 工具执行处理器（被装饰的目标）
// 由 ToolExecutor 包装 Tool.Execute 得到
type ToolHandler func(ctx context.Context, args map[string]any) (ToolResult, error)

// ToolDecorator 装饰器（函数式）
// 接收下一个 handler，返回增强后的 handler
type ToolDecorator func(next ToolHandler) ToolHandler

// ToolContext 工具执行上下文（贯穿整个装饰器链）
// 通过 context.Value 传递，避免污染 ToolHandler 签名
type ToolContext struct {
	CallerID    string   // 调用者ID（如智能体ID / 座席ID）
	AgentID     string   // 智能体ID
	CustomerID  string   // 客户ID
	SessionID   string   // 会话ID
	Permissions []string // 调用者拥有的权限列表
	AuditTrace  string   // 审计追踪ID（贯穿整条链路）
	Source      string   // 调用来源（agent/sop/manual/api）
}

// toolContextKey context.Value 的 key 类型（避免冲突）
type toolContextKey struct{}

// WithToolContext 将 ToolContext 注入 context
func WithToolContext(ctx context.Context, tc *ToolContext) context.Context {
	return context.WithValue(ctx, toolContextKey{}, tc)
}

// GetToolContext 从 context 取出 ToolContext
func GetToolContext(ctx context.Context) *ToolContext {
	if v, ok := ctx.Value(toolContextKey{}).(*ToolContext); ok {
		return v
	}
	return nil
}

// ===== 依赖接口 =====

// PermissionChecker 权限校验接口
type PermissionChecker interface {
	// Check 返回 nil 表示放行，非 nil 表示拒绝
	Check(ctx context.Context, toolName string, tc *ToolContext) error
}

// RateLimiter 限流接口
type RateLimiter interface {
	// Acquire 获取令牌；返回 nil 表示放行，ErrRateLimited 表示被限流
	Acquire(ctx context.Context, key string) error
}

// RetryPolicy 重试策略接口
type RetryPolicy interface {
	// NextBackoff 返回下一次重试的等待时间；ok=false 表示不再重试
	NextBackoff(attempt int, lastErr error) (delay time.Duration, ok bool)
	// MaxAttempts 最大重试次数（含首次）
	MaxAttempts() int
}

// AuditLogger 审计日志接口
type AuditLogger interface {
	Log(ctx context.Context, entry AuditEntry)
}

// AuditEntry 审计日志条目
//
// P1-B：新增 TraceID 字段，贯穿整个 Agent Loop 的多次工具调用
// 同一个 Agent Loop 内所有 AuditEntry 共享同一 TraceID，便于关联分析
type AuditEntry struct {
	TraceID       string        `json:"trace_id,omitempty"` // P1-B：贯穿 Agent Loop 的 trace_id
	ToolName      string        `json:"tool_name"`
	CallerID      string        `json:"caller_id"`
	AgentID       string        `json:"agent_id,omitempty"`
	CustomerID    string        `json:"customer_id,omitempty"`
	SessionID     string        `json:"session_id,omitempty"`
	Success       bool          `json:"success"`
	Error         string        `json:"error,omitempty"`
	Duration      time.Duration `json:"duration_ms"`
	RetryCount    int           `json:"retry_count"`
	AuditTrace    string        `json:"audit_trace,omitempty"`
	ArgsSummary   string        `json:"args_summary,omitempty"`   // 参数摘要（脱敏后）
	ResultSummary string        `json:"result_summary,omitempty"` // P1-B：结果摘要（前 200 字符）
	ExecutedAt    time.Time     `json:"executed_at"`
}

// CostTracker 计费接口
type CostTracker interface {
	// Record 记录一次工具调用的成本
	Record(ctx context.Context, toolName string, success bool, duration time.Duration) error
}

// ===== 错误定义 =====

var (
	// ErrPermissionDenied 权限拒绝
	ErrPermissionDenied = fmt.Errorf("permission denied")
	// ErrRateLimited 被限流
	ErrRateLimited = fmt.Errorf("rate limited")
	// ErrToolTimeout 工具执行超时
	ErrToolTimeout = fmt.Errorf("tool execution timeout")
	// ErrToolPanic 工具 panic
	ErrToolPanic = fmt.Errorf("tool panic")
)

// ===== 装饰器 1：权限校验 =====

// PermissionDecorator 权限校验装饰器
// 在工具执行前校验调用者是否拥有调用该工具的权限
func PermissionDecorator(checker PermissionChecker) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			if checker == nil {
				// nil checker = 放行（用于不需要权限校验的场景）
				return next(ctx, args)
			}
			// 从 context 取出工具名和调用者信息
			// 注意：toolName 通过特殊 context value 传递（由 Executor 注入）
			toolName, _ := ctx.Value(toolNameKey{}).(string)
			tc := GetToolContext(ctx)
			if err := checker.Check(ctx, toolName, tc); err != nil {
				return ErrorResult(toolName, fmt.Errorf("%w: %v", ErrPermissionDenied, err)), ErrPermissionDenied
			}
			return next(ctx, args)
		}
	}
}

// toolNameKey context.Value 的 key（用于在装饰器链中传递工具名）
type toolNameKey struct{}

// WithToolName 将工具名注入 context（供装饰器使用）
func WithToolName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, toolNameKey{}, name)
}

// GetToolName 从 context 取出工具名
func GetToolName(ctx context.Context) string {
	if v, ok := ctx.Value(toolNameKey{}).(string); ok {
		return v
	}
	return ""
}

// traceIDKey context.Value 的 key（用于在装饰器链中传递 trace_id）
type traceIDKey struct{}

// WithTraceID 将 trace_id 注入 context（供装饰器使用）
// P1-B：由 Agent Loop 调用方注入，贯穿整个 loop 的所有工具调用
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// GetTraceID 从 context 取出 trace_id
func GetTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}

// ===== 装饰器 2：限流 =====

// RateLimitDecorator 限流装饰器
// key 由限流器内部决定（通常按 caller_id + tool_name 维度限流）
func RateLimitDecorator(limiter RateLimiter) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			if limiter == nil {
				return next(ctx, args)
			}
			toolName := GetToolName(ctx)
			tc := GetToolContext(ctx)
			// 限流 key = caller_id + ":" + tool_name
			key := toolName
			if tc != nil && tc.CallerID != "" {
				key = tc.CallerID + ":" + toolName
			}
			if err := limiter.Acquire(ctx, key); err != nil {
				return ErrorResult(toolName, fmt.Errorf("%w: %v", ErrRateLimited, err)), ErrRateLimited
			}
			return next(ctx, args)
		}
	}
}

// ===== 装饰器 3：重试 =====

// RetryDecorator 重试装饰器
// 失败时按 RetryPolicy 重试；成功或重试次数耗尽后返回
// panic 也算失败，会触发重试
//
// P2-F: 错误类型分类
//   - 可重试错误：网络抖动、超时、5xx 服务端错误、panic（瞬时故障）
//   - 不可重试错误：权限拒绝、限流、熔断开启、context 取消、参数校验失败（确定性故障）
//   - 不可重试错误立即返回，不浪费重试次数
func RetryDecorator(policy RetryPolicy) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (result ToolResult, err error) {
			if policy == nil {
				return next(ctx, args)
			}
			maxAttempts := policy.MaxAttempts()
			if maxAttempts < 1 {
				maxAttempts = 1
			}
			var lastErr error
			for attempt := 0; attempt < maxAttempts; attempt++ {
				// 检查 context 是否已取消
				if ctx.Err() != nil {
					err = ctx.Err()
					return
				}
				// 首次立即执行，后续按 policy 等待
				if attempt > 0 {
					delay, ok := policy.NextBackoff(attempt, lastErr)
					if !ok {
						err = lastErr
						return
					}
					select {
					case <-ctx.Done():
						err = ctx.Err()
						return
					case <-time.After(delay):
					}
				}
				// 执行（带 panic 恢复）
				result, err = safeExecute(ctx, next, args)
				if err == nil && result.Success {
					result.Timing.RetryCount = attempt
					return
				}
				lastErr = err
				if err == nil && !result.Success {
					lastErr = fmt.Errorf("tool returned failure: %s", result.Error)
				}
				// P2-F: 不可重试错误立即返回（不浪费重试次数）
				if isNonRetryableError(err) || isNonRetryableResult(result) {
					if result.Success == false && result.ToolName == "" {
						toolName := GetToolName(ctx)
						result = ErrorResult(toolName, lastErr)
					}
					result.Timing.RetryCount = attempt
					return
				}
			}
			// 重试耗尽
			if result.Success == false && result.ToolName == "" {
				toolName := GetToolName(ctx)
				result = ErrorResult(toolName, fmt.Errorf("重试 %d 次后仍失败：%v", maxAttempts, lastErr))
			}
			result.Timing.RetryCount = maxAttempts - 1
			err = lastErr
			return
		}
	}
}

// isNonRetryableError 判断错误是否不可重试（确定性故障）
//
// 不可重试错误类型：
//   - ErrPermissionDenied: 权限不足，重试也不会通过
//   - ErrRateLimited: 已被限流，重试只会加剧拥塞
//   - ErrCircuitOpen: 熔断开启，重试无意义
//   - ErrLoopDetected: 工具调用循环被检测到，重试只会再次触发循环
//     （依据 loop_guard.go L34 注释承诺：ErrLoopDetected 是不可重试错误）
//   - context.Canceled: 客户端主动取消
//   - context.DeadlineExceeded: 总超时（不是单次超时），重试无意义
//   - 参数校验错误（业务错误，重试不会改变结果）
func isNonRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrPermissionDenied) ||
		errors.Is(err, ErrRateLimited) ||
		errors.Is(err, ErrCircuitOpen) ||
		errors.Is(err, ErrLoopDetected) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// 参数校验错误（含 "invalid argument" / "validation failed"）
	errMsg := err.Error()
	if strings.Contains(errMsg, "invalid argument") ||
		strings.Contains(errMsg, "validation failed") ||
		strings.Contains(errMsg, "参数校验失败") ||
		strings.Contains(errMsg, "参数无效") {
		return true
	}
	return false
}

// isNonRetryableResult 判断结果是否不可重试
//
// 业务错误（如 customer_not_found / order_already_exists）不应重试
// 这些错误由工具自身返回 Success=false + Error 字段
func isNonRetryableResult(result ToolResult) bool {
	if result.Success {
		return false
	}
	errMsg := result.Error
	// 业务确定性错误（重试不会改变结果）
	nonRetryablePatterns := []string{
		"not_found",         // 客户/订单/优惠券等不存在
		"already_exists",    // 资源已存在
		"already_used",      // 优惠券已使用
		"expired",           // 优惠券/活动已过期
		"insufficient_",     // 余额/库存不足
		"permission_denied", // 业务层权限不足
		"invalid_argument",  // 参数校验失败
		"validation_failed", // 校验失败
		"参数无效",              // 中文：参数无效
		"参数校验失败",            // 中文：参数校验失败
		"不存在",               // 中文：资源不存在
		"已存在",               // 中文：资源已存在
		"已使用",               // 中文：优惠券已使用
		"已过期",               // 中文：优惠券已过期
	}
	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	return false
}

// safeExecute 带 panic 恢复的执行
func safeExecute(ctx context.Context, h ToolHandler, args map[string]any) (result ToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			err = fmt.Errorf("%w: %v\n%s", ErrToolPanic, r, stack)
		}
	}()
	return h(ctx, args)
}

// ===== 装饰器 4：超时 =====

// TimeoutDecorator 超时装饰器
// 单次工具执行最长允许 duration 时间
func TimeoutDecorator(duration time.Duration) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			if duration <= 0 {
				return next(ctx, args)
			}
			childCtx, cancel := context.WithTimeout(ctx, duration)
			defer cancel()
			// 用 channel + goroutine 实现，避免 next 内部不响应 context 取消
			type ret struct {
				r ToolResult
				e error
			}
			ch := make(chan ret, 1)
			go func() {
				defer func() {
					if rec := recover(); rec != nil {
						ch <- ret{ErrorResult(GetToolName(childCtx), fmt.Errorf("%w: %v", ErrToolPanic, rec)), ErrToolPanic}
					}
				}()
				r, e := next(childCtx, args)
				ch <- ret{r, e}
			}()
			select {
			case <-childCtx.Done():
				toolName := GetToolName(ctx)
				if childCtx.Err() == context.DeadlineExceeded {
					return ErrorResult(toolName, ErrToolTimeout), ErrToolTimeout
				}
				return ErrorResult(toolName, childCtx.Err()), childCtx.Err()
			case out := <-ch:
				return out.r, out.e
			}
		}
	}
}

// ===== 装饰器 5：审计 + 计费 =====

// AuditDecorator 审计 + 计费装饰器
// 在工具执行前后记录审计日志、累计计费
func AuditDecorator(logger AuditLogger, costTracker CostTracker) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			toolName := GetToolName(ctx)
			tc := GetToolContext(ctx)
			start := time.Now()

			result, err := next(ctx, args)
			duration := time.Since(start)

			// 异步写审计日志（避免阻塞主流程）
			if logger != nil {
				entry := AuditEntry{
					ToolName:   toolName,
					TraceID:    GetTraceID(ctx), // P1-B：从 context 取 trace_id
					CallerID:   "",
					AgentID:    "",
					CustomerID: "",
					SessionID:  "",
					Success:    err == nil && result.Success,
					Duration:   duration,
					RetryCount: result.Timing.RetryCount,
					ExecutedAt: start,
				}
				if err != nil {
					entry.Error = err.Error()
				} else if !result.Success {
					entry.Error = result.Error
				}
				if tc != nil {
					entry.CallerID = tc.CallerID
					entry.AgentID = tc.AgentID
					entry.CustomerID = tc.CustomerID
					entry.SessionID = tc.SessionID
					entry.AuditTrace = tc.AuditTrace
				}
				entry.ArgsSummary = summarizeArgs(args)
				// P1-B：记录结果摘要（前 200 字符，避免日志膨胀）
				entry.ResultSummary = summarizeResult(result.Data)
				// 同步写日志（保证顺序，且 logger 实现可以自己异步落盘）
				logger.Log(ctx, entry)
			}

			// 异步记录计费（失败不影响主流程）
			if costTracker != nil {
				_ = costTracker.Record(ctx, toolName, err == nil && result.Success, duration)
			}

			// P1-A：记录 Prometheus 指标（ToolCallTotal + ToolCallDuration + ToolCallErrors）
			// 放在审计装饰器中，确保所有工具调用都被统计
			recordToolCallMetrics(toolName, err, result, duration)

			return result, err
		}
	}
}

// recordToolCallMetrics 记录工具调用 Prometheus 指标
// P1-A：深度审查第二轮新增
//
// 指标维度：
//   - ToolCallTotal: tool_name|result(success|failed|panic)
//   - ToolCallDuration: tool_name（sums + counts）
//   - ToolCallErrors: tool_name|error_type(permission|ratelimit|timeout|panic|internal)
//
// 设计说明：直接引用 metrics.GlobalMetrics，避免引入复杂抽象
// metrics 包仅依赖基础类型，无 import cycle 风险
func recordToolCallMetrics(toolName string, err error, result ToolResult, duration time.Duration) {
	m := metrics.GlobalMetrics
	if m == nil || m.ToolCallTotal == nil {
		return
	}

	// 1. 总调用数（按 result 分类）
	resultLabel := "success"
	if err != nil || !result.Success {
		resultLabel = "failed"
	}
	if err != nil && errors.Is(err, ErrToolPanic) {
		resultLabel = "panic"
	}
	m.ToolCallTotal.Inc(fmt.Sprintf("%s|%s", toolName, resultLabel))

	// 2. 耗时直方图（按 tool_name 分组）
	if m.ToolCallDuration != nil {
		m.ToolCallDuration.Observe(toolName, duration.Seconds())
	}

	// 3. 错误分类计数（仅失败时记录）
	if err != nil || !result.Success {
		errType := "internal"
		if err != nil {
			switch {
			case errors.Is(err, ErrPermissionDenied):
				errType = "permission"
			case errors.Is(err, ErrRateLimited):
				errType = "ratelimit"
			case errors.Is(err, ErrToolTimeout):
				errType = "timeout"
			case errors.Is(err, ErrToolPanic):
				errType = "panic"
			}
		}
		if m.ToolCallErrors != nil {
			m.ToolCallErrors.Inc(fmt.Sprintf("%s|%s", toolName, errType))
		}
	}
}

// summarizeArgs 参数脱敏摘要（截断 + 移除敏感字段）
func summarizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	sensitiveKeys := map[string]bool{
		"password": true, "token": true, "secret": true,
		"api_key": true, "apikey": true, "phone": true,
		"id_card": true, "bank_card": true,
	}
	out := ""
	for k, v := range args {
		if sensitiveKeys[k] {
			out += fmt.Sprintf("%s=***,", k)
			continue
		}
		s := fmt.Sprintf("%v", v)
		if len(s) > 50 {
			s = s[:50] + "..."
		}
		out += fmt.Sprintf("%s=%s,", k, s)
	}
	if len(out) > 200 {
		out = out[:200] + "..."
	}
	return out
}

// summarizeResult 结果摘要（前 200 字符，避免日志膨胀）
// 接受 ToolResult.Data（any 类型），序列化为字符串后截断
func summarizeResult(data any) string {
	if data == nil {
		return ""
	}
	s := fmt.Sprintf("%v", data)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// ===== 装饰器链构造 =====

// ChainDecorators 串联多个装饰器
// 执行顺序：decorators[0] → decorators[1] → ... → handler
// 即 decorators[0] 最先执行其前置逻辑，最后执行其后置逻辑
//
// 例：ChainDecorators(handler, PermissionDecorator(c), RateLimitDecorator(r))
// 等价于：PermissionDecorator(c)(RateLimitDecorator(r)(handler))
// 调用流：permission.pre → ratelimit.pre → handler → ratelimit.post → permission.post
func ChainDecorators(handler ToolHandler, decorators ...ToolDecorator) ToolHandler {
	// 从后往前包裹
	for i := len(decorators) - 1; i >= 0; i-- {
		if decorators[i] == nil {
			continue
		}
		handler = decorators[i](handler)
	}
	return handler
}

// BuildDefaultChain 按默认顺序构造 5 装饰器链
// 顺序：权限 → 限流 → 熔断 → 重试 → 超时 → 审计计费 → handler
//
// P2-B：新增熔断器装饰器（位于限流之后、重试之前）
//   - 限流在熔断之前：先过滤掉超频请求，再判断熔断状态
//   - 熔断在重试之前：避免对已熔断的工具进行重试（浪费资源）
//   - 超时在审计之前：超时由 TimeoutDecorator 兜底，审计记录真实耗时
func BuildDefaultChain(
	handler ToolHandler,
	checker PermissionChecker,
	limiter RateLimiter,
	policy RetryPolicy,
	timeout time.Duration,
	logger AuditLogger,
	costTracker CostTracker,
) ToolHandler {
	return ChainDecorators(handler,
		PermissionDecorator(checker),
		RateLimitDecorator(limiter),
		RetryDecorator(policy),
		TimeoutDecorator(timeout),
		AuditDecorator(logger, costTracker),
	)
}

// BuildChainWithCircuitBreaker 按顺序构造 7 装饰器链（含熔断器 + 参数校验）
// 顺序：权限 → 限流 → 熔断 → 参数校验 → 重试 → 超时 → 审计计费 → handler
//
// P2-B：新增熔断器装饰器，用于生产环境的工具执行链
// P2-E：新增参数校验装饰器，提前拒绝非法参数（避免无效重试）
// 当 circuitBreaker 为 nil 时退化为 BuildDefaultChain
func BuildChainWithCircuitBreaker(
	handler ToolHandler,
	checker PermissionChecker,
	limiter RateLimiter,
	circuitBreaker *CircuitBreakerRegistry,
	policy RetryPolicy,
	timeout time.Duration,
	logger AuditLogger,
	costTracker CostTracker,
) ToolHandler {
	if circuitBreaker == nil {
		return BuildDefaultChain(handler, checker, limiter, policy, timeout, logger, costTracker)
	}
	return ChainDecorators(handler,
		PermissionDecorator(checker),
		RateLimitDecorator(limiter),
		CircuitBreakerDecorator(circuitBreaker),
		RetryDecorator(policy),
		TimeoutDecorator(timeout),
		AuditDecorator(logger, costTracker),
	)
}

// BuildChainWithCircuitBreakerAndValidator 按顺序构造 7 装饰器链（含熔断器 + 参数校验）
// 顺序：权限 → 限流 → 熔断 → 参数校验 → 重试 → 超时 → 审计计费 → handler
//
// P2-E：新增参数校验装饰器（位于熔断之后、重试之前）
//   - 参数校验在重试之前：避免无效参数被重试
//   - 参数校验在熔断之后：避免对已熔断工具做无效校验
func BuildChainWithCircuitBreakerAndValidator(
	handler ToolHandler,
	checker PermissionChecker,
	limiter RateLimiter,
	circuitBreaker *CircuitBreakerRegistry,
	registry *ToolRegistry,
	policy RetryPolicy,
	timeout time.Duration,
	logger AuditLogger,
	costTracker CostTracker,
) ToolHandler {
	chain := []ToolDecorator{
		PermissionDecorator(checker),
		RateLimitDecorator(limiter),
	}
	if circuitBreaker != nil {
		chain = append(chain, CircuitBreakerDecorator(circuitBreaker))
	}
	if registry != nil {
		chain = append(chain, ParamValidatorDecorator(registry))
	}
	chain = append(chain, RetryDecorator(policy), TimeoutDecorator(timeout), AuditDecorator(logger, costTracker))
	return ChainDecorators(handler, chain...)
}

// ===== 默认 NoOp 实现（用于测试 / 默认放行） =====

// NoOpPermissionChecker 始终放行
type NoOpPermissionChecker struct{}

func (NoOpPermissionChecker) Check(ctx context.Context, toolName string, tc *ToolContext) error {
	return nil
}

// NoOpRateLimiter 始终放行
type NoOpRateLimiter struct{}

func (NoOpRateLimiter) Acquire(ctx context.Context, key string) error { return nil }

// NoOpAuditLogger 不写日志
type NoOpAuditLogger struct{}

func (NoOpAuditLogger) Log(ctx context.Context, entry AuditEntry) {}

// NoOpCostTracker 不计费
type NoOpCostTracker struct{}

func (NoOpCostTracker) Record(ctx context.Context, toolName string, success bool, duration time.Duration) error {
	return nil
}

// ===== 内置 RateLimiter 实现：令牌桶（按 caller+tool 维度） =====

// TokenBucketLimiter 令牌桶限流器
// 每个 key（caller_id:tool_name）独立一个桶
type TokenBucketLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64 // 每秒生成令牌数
	burst   int     // 桶容量
}

type tokenBucket struct {
	tokens  float64
	lastRef time.Time
}

// NewTokenBucketLimiter 创建令牌桶限流器
// rate: 每秒生成令牌数（如 10 表示 10 QPS）
// burst: 桶容量（允许瞬时突发）
func NewTokenBucketLimiter(rate float64, burst int) *TokenBucketLimiter {
	if burst < 1 {
		burst = 1
	}
	return &TokenBucketLimiter{
		buckets: make(map[string]*tokenBucket),
		rate:    rate,
		burst:   burst,
	}
}

// Acquire 获取一个令牌
func (l *TokenBucketLimiter) Acquire(ctx context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &tokenBucket{tokens: float64(l.burst), lastRef: now}
		l.buckets[key] = b
	}
	// 按时间间隔补充令牌
	elapsed := now.Sub(b.lastRef).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}
	b.lastRef = now
	if b.tokens < 1.0 {
		return ErrRateLimited
	}
	b.tokens -= 1.0
	return nil
}

// ===== 内置 RetryPolicy 实现：指数退避 =====

// ExponentialBackoffPolicy 指数退避重试策略
type ExponentialBackoffPolicy struct {
	MaxAttemptsValue int           // 最大重试次数（含首次）
	BaseDelay        time.Duration // 基础延迟
	MaxDelay         time.Duration // 最大延迟
	Jitter           bool          // 是否添加随机抖动
}

// NewExponentialBackoffPolicy 创建指数退避策略
func NewExponentialBackoffPolicy(maxAttempts int, baseDelay, maxDelay time.Duration) *ExponentialBackoffPolicy {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}
	if maxDelay <= 0 {
		maxDelay = 10 * time.Second
	}
	return &ExponentialBackoffPolicy{
		MaxAttemptsValue: maxAttempts,
		BaseDelay:        baseDelay,
		MaxDelay:         maxDelay,
		Jitter:           true,
	}
}

// MaxAttempts 最大重试次数
func (p *ExponentialBackoffPolicy) MaxAttempts() int {
	return p.MaxAttemptsValue
}

// NextBackoff 下一次重试延迟
// attempt 从 1 开始（第 1 次重试的延迟）
// delay = baseDelay * 2^(attempt-1)，最大不超过 maxDelay
func (p *ExponentialBackoffPolicy) NextBackoff(attempt int, lastErr error) (time.Duration, bool) {
	if attempt < 1 {
		return 0, false
	}
	if attempt > p.MaxAttemptsValue {
		return 0, false
	}
	// 指数退避
	delay := float64(p.BaseDelay) * math.Pow(2, float64(attempt-1))
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	// 添加抖动（±20%）
	if p.Jitter {
		jitter := (rand.Float64() - 0.5) * 0.4 * delay
		delay += jitter
		if delay < 0 {
			delay = float64(p.BaseDelay)
		}
	}
	return time.Duration(delay), true
}

// ===== 内置 AuditLogger 实现：内存记录（用于测试 / 默认审计） =====

// MemoryAuditLogger 内存审计日志（用于测试 / 单机审计）
type MemoryAuditLogger struct {
	mu      sync.Mutex
	entries []AuditEntry
	maxSize int
}

// NewMemoryAuditLogger 创建内存审计日志
// maxSize: 最大保留条数（超出后滚动覆盖最旧条目）
func NewMemoryAuditLogger(maxSize int) *MemoryAuditLogger {
	if maxSize < 1 {
		maxSize = 1000
	}
	return &MemoryAuditLogger{
		entries: make([]AuditEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// Log 记录审计日志
func (l *MemoryAuditLogger) Log(ctx context.Context, entry AuditEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) >= l.maxSize {
		// 滚动覆盖最旧条目
		l.entries = l.entries[1:]
	}
	l.entries = append(l.entries, entry)
}

// Entries 返回所有审计日志副本
func (l *MemoryAuditLogger) Entries() []AuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]AuditEntry, len(l.entries))
	copy(out, l.entries)
	return out
}

// Count 返回审计日志条数
func (l *MemoryAuditLogger) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// Reset 清空审计日志
func (l *MemoryAuditLogger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = l.entries[:0]
}

// ===== 内置 CostTracker 实现：内存计费（用于测试 / 统计） =====

// MemoryCostTracker 内存计费统计
type MemoryCostTracker struct {
	mu      sync.Mutex
	records map[string]*costRecord
}

type costRecord struct {
	TotalCalls      int64
	SuccessCalls    int64
	FailedCalls     int64
	TotalDurationMs int64
}

// NewMemoryCostTracker 创建内存计费统计
func NewMemoryCostTracker() *MemoryCostTracker {
	return &MemoryCostTracker{
		records: make(map[string]*costRecord),
	}
}

// Record 记录一次工具调用的成本
func (t *MemoryCostTracker) Record(ctx context.Context, toolName string, success bool, duration time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.records[toolName]
	if !ok {
		rec = &costRecord{}
		t.records[toolName] = rec
	}
	atomic.AddInt64(&rec.TotalCalls, 1)
	if success {
		atomic.AddInt64(&rec.SuccessCalls, 1)
	} else {
		atomic.AddInt64(&rec.FailedCalls, 1)
	}
	atomic.AddInt64(&rec.TotalDurationMs, duration.Milliseconds())
	return nil
}

// CostStats 计费统计快照
type CostStats struct {
	ToolName        string  `json:"tool_name"`
	TotalCalls      int64   `json:"total_calls"`
	SuccessCalls    int64   `json:"success_calls"`
	FailedCalls     int64   `json:"failed_calls"`
	TotalDurationMs int64   `json:"total_duration_ms"`
	SuccessRate     float64 `json:"success_rate"`
	AvgDurationMs   float64 `json:"avg_duration_ms"`
}

// Stats 返回所有工具的计费统计
func (t *MemoryCostTracker) Stats() []CostStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]CostStats, 0, len(t.records))
	for name, rec := range t.records {
		stats := CostStats{
			ToolName:        name,
			TotalCalls:      atomic.LoadInt64(&rec.TotalCalls),
			SuccessCalls:    atomic.LoadInt64(&rec.SuccessCalls),
			FailedCalls:     atomic.LoadInt64(&rec.FailedCalls),
			TotalDurationMs: atomic.LoadInt64(&rec.TotalDurationMs),
		}
		if stats.TotalCalls > 0 {
			stats.SuccessRate = float64(stats.SuccessCalls) / float64(stats.TotalCalls)
			stats.AvgDurationMs = float64(stats.TotalDurationMs) / float64(stats.TotalCalls)
		}
		out = append(out, stats)
	}
	return out
}

// Reset 清空计费统计
func (t *MemoryCostTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = make(map[string]*costRecord)
}
