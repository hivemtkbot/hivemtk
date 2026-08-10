package tooluse

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

type ToolHandler func(ctx context.Context, args map[string]any) (ToolResult, error)

type ToolDecorator func(next ToolHandler) ToolHandler

type ToolContext struct {
	CallerID    string   // 调用者ID（如智能体ID / 座席ID）
	AgentID     string   // 智能体ID
	CustomerID  string   // 客户ID
	SessionID   string   // 会话ID
	Permissions []string // 调用者拥有的权限列表
	AuditTrace  string   // 审计追踪ID（贯穿整条链路）
	Source      string   // 调用来源（agent/sop/manual/api）
}

type toolContextKey struct{}

func WithToolContext(ctx context.Context, tc *ToolContext) context.Context {
	return context.WithValue(ctx, toolContextKey{}, tc)
}

func GetToolContext(ctx context.Context) *ToolContext {
	if v, ok := ctx.Value(toolContextKey{}).(*ToolContext); ok {
		return v
	}
	return nil
}

type PermissionChecker interface {
	// Check 返回 nil 表示放行，非 nil 表示拒绝
	Check(ctx context.Context, toolName string, tc *ToolContext) error
}

type RateLimiter interface {
	// Acquire 获取令牌；返回 nil 表示放行，ErrRateLimited 表示被限流
	Acquire(ctx context.Context, key string) error
}

type RetryPolicy interface {
	// NextBackoff 返回下一次重试的等待时间；ok=false 表示不再重试
	NextBackoff(attempt int, lastErr error) (delay time.Duration, ok bool)
	// MaxAttempts 最大重试次数（含首次）
	MaxAttempts() int
}

type AuditLogger interface {
	Log(ctx context.Context, entry AuditEntry)
}

type AuditEntry struct {
	TraceID       string        `json:"trace_id,omitempty"` // 贯穿 Agent Loop 的 trace_id
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
	ResultSummary string        `json:"result_summary,omitempty"` // 结果摘要（前 200 字符）
	ExecutedAt    time.Time     `json:"executed_at"`
}

type CostTracker interface {
	// Record 记录一次工具调用的成本
	Record(ctx context.Context, toolName string, success bool, duration time.Duration) error
}

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

type toolNameKey struct{}

func WithToolName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, toolNameKey{}, name)
}

func GetToolName(ctx context.Context) string {
	if v, ok := ctx.Value(toolNameKey{}).(string); ok {
		return v
	}
	return ""
}

type traceIDKey struct{}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

func GetTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}

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

func RetryDecorator(policy RetryPolicy) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (result ToolResult, err error) {
			toolName := GetToolName(ctx)
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
					// 彻底修复：ctx 取消等提前返回路径必须返回完整 ToolResult，
					// 否则会产出「Success=false / err!=nil 但 Error 为空」的零值结果
					//（曾导致 message_trace 出现 abnormal 但 abnormal/error 两列皆空的脏 span）。
					return ErrorResult(toolName, err), err
				}
				// 首次立即执行，后续按 policy 等待
				if attempt > 0 {
					delay, ok := policy.NextBackoff(attempt, lastErr)
					if !ok {
						err = lastErr
						return ensureErrorResult(toolName, result, err), err
					}
					select {
					case <-ctx.Done():
						err = ctx.Err()
						return ErrorResult(toolName, err), err
					case <-time.After(delay):
					}
				}
				// 执行（带 panic 恢复）
				result, err = safeExecute(ctx, next, args)
				if err == nil && result.Success {
					result.Timing.RetryCount = attempt
					return result, nil
				}
				lastErr = err
				if err == nil && !result.Success {
					lastErr = fmt.Errorf("tool returned failure: %s", result.Error)
				}
				// 不可重试错误立即返回（不浪费重试次数）
				if isNonRetryableError(err) || isNonRetryableResult(result) {
					result = ensureErrorResult(toolName, result, lastErr)
					result.Timing.RetryCount = attempt
					return result, err
				}
			}
			// 重试耗尽
			result = ensureErrorResult(toolName, result, fmt.Errorf("重试 %d 次后仍失败：%v", maxAttempts, lastErr))
			result.Timing.RetryCount = maxAttempts - 1
			err = lastErr
			return result, err
		}
	}
}

func ensureErrorResult(toolName string, result ToolResult, err error) ToolResult {
	if result.Success || result.ToolName != "" || result.Error != "" || err == nil {
		return result
	}
	return ErrorResult(toolName, err)
}

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

func safeExecute(ctx context.Context, h ToolHandler, args map[string]any) (result ToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			err = fmt.Errorf("%w: %v\n%s", ErrToolPanic, r, stack)
		}
	}()
	return h(ctx, args)
}

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
					TraceID:    GetTraceID(ctx), // 从 context 取 trace_id
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
				// 记录结果摘要（前 200 字符，避免日志膨胀）
				entry.ResultSummary = summarizeResult(result.Data)
				// 同步写日志（保证顺序，且 logger 实现可以自己异步落盘）
				logger.Log(ctx, entry)
			}

			// 异步记录计费（失败不影响主流程）
			if costTracker != nil {
				_ = costTracker.Record(ctx, toolName, err == nil && result.Success, duration)
			}

			// 记录 Prometheus 指标（ToolCallTotal + ToolCallDuration + ToolCallErrors）
			// 放在审计装饰器中，确保所有工具调用都被统计
			recordToolCallMetrics(toolName, err, result, duration)

			return result, err
		}
	}
}

func recordToolCallMetrics(toolName string, err error, result ToolResult, duration time.Duration) {
	_ = toolName
	_ = err
	_ = result
	_ = duration
}

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

type NoOpPermissionChecker struct{}

func (NoOpPermissionChecker) Check(ctx context.Context, toolName string, tc *ToolContext) error {
	return nil
}

type NoOpRateLimiter struct{}

func (NoOpRateLimiter) Acquire(ctx context.Context, key string) error { return nil }

type NoOpAuditLogger struct{}

func (NoOpAuditLogger) Log(ctx context.Context, entry AuditEntry) {}

type NoOpCostTracker struct{}

func (NoOpCostTracker) Record(ctx context.Context, toolName string, success bool, duration time.Duration) error {
	return nil
}

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
