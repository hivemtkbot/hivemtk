package tooluse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/trace"
	"marketing/internal/pkg/tracing"
)

// executor.go 工具执行引擎（PRD §5.2 G3）
//
// 设计目标：
//  1. 从 ToolRegistry 取工具 → 包装为 handler → 应用 5 装饰器链 → 执行
//  2. 缓存装饰后 handler（避免每次重建链）
//  3. 支持工具级别配置覆盖（如某些工具需要更长超时 / 不同重试策略）
//  4. 支持批量执行（顺序 / 并发）
//  5. 集成 LLM Function Calling：DispatchByLLMToolCall 接收 OpenAI tool_call 格式

// ===== 可观测性 observer（与追踪系统解耦：默认 nil，由 router 在启动时接线） =====
//
// 设计意图：工具执行引擎暴露一个可选 observer 钩子，事件类型来自 tracing 包。
// router 启动时接线：tooluse.ToolTraceSink = tracing.ReportToolCall。
// 从而以「观察者模式」自动记录 agent 多轮（agent_turn）/ 多工具（tool_call）调用，
// 业务代码与工具调用点均无需手写追踪埋点。
var ToolTraceSink func(ctx context.Context, ev tracing.ToolTraceEvent)

// turnIndexKey / turnCounters 用于在单次 Agent Loop 内为每一轮 LLM 推理分配自增序号。
type turnIndexKey struct{}

var turnCounters sync.Map // trace_id -> *int64

// WithTurnIndex 把当前 agent 轮次序号注入 context，供其下所有 tool_call 继承。
func WithTurnIndex(ctx context.Context, idx int) context.Context {
	return context.WithValue(ctx, turnIndexKey{}, idx)
}

// GetTurnIndex 取出当前 agent 轮次序号。
func GetTurnIndex(ctx context.Context) int {
	if v, ok := ctx.Value(turnIndexKey{}).(int); ok {
		return v
	}
	return 0
}

// nextTurnIndex 基于 trace_id 自增返回本轮序号（1-based）。
func nextTurnIndex(ctx context.Context) int {
	key := trace.TraceIDFromContext(ctx)
	if key == "" {
		key = "default"
	}
	v, _ := turnCounters.LoadOrStore(key, new(int64))
	return int(atomic.AddInt64(v.(*int64), 1))
}

// ===== 配置类型 =====

// ToolExecutorConfig 执行器全局配置
type ToolExecutorConfig struct {
	DefaultTimeout time.Duration // 默认单次执行超时
	// 可选：全局默认装饰器依赖（也可通过 ToolOverride 覆盖）
	PermissionChecker PermissionChecker
	RateLimiter       RateLimiter
	RetryPolicy       RetryPolicy
	AuditLogger       AuditLogger
	CostTracker       CostTracker
	// 熔断器注册中心（可选，nil 表示不启用熔断）
	// 当配置非 nil 时，工具执行链会插入 CircuitBreakerDecorator
	CircuitBreaker *CircuitBreakerRegistry
}

// ToolOverride 工具级别配置覆盖
type ToolOverride struct {
	ToolName   string        // 工具名
	Timeout    time.Duration // 覆盖超时（0 表示用默认）
	MaxRetries int           // 覆盖重试次数（< 0 表示用默认）
	BaseDelay  time.Duration // 重试基础延迟
	Disabled   bool          // 是否禁用该工具
}

// ===== 工具执行器 =====

// ToolExecutor 工具执行引擎
// 线程安全；缓存每个工具的装饰后 handler
type ToolExecutor struct {
	registry *ToolRegistry
	config   ToolExecutorConfig

	mu        sync.RWMutex
	overrides map[string]ToolOverride // toolName → override
	cache     map[string]ToolHandler  // toolName → 装饰后 handler（缓存）
}

// NewToolExecutor 创建工具执行器
func NewToolExecutor(registry *ToolRegistry, config ToolExecutorConfig) *ToolExecutor {
	if config.DefaultTimeout <= 0 {
		config.DefaultTimeout = 30 * time.Second
	}
	return &ToolExecutor{
		registry:  registry,
		config:    config,
		overrides: make(map[string]ToolOverride),
		cache:     make(map[string]ToolHandler),
	}
}

// SetOverride 设置工具级别覆盖
// 设置后会清除该工具的 handler 缓存，下次执行时重建
func (e *ToolExecutor) SetOverride(override ToolOverride) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.overrides[override.ToolName] = override
	delete(e.cache, override.ToolName)
}

// GetOverride 获取工具级别覆盖
func (e *ToolExecutor) GetOverride(toolName string) (ToolOverride, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	o, ok := e.overrides[toolName]
	return o, ok
}

// ClearOverride 清除工具级别覆盖
func (e *ToolExecutor) ClearOverride(toolName string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.overrides, toolName)
	delete(e.cache, toolName)
}

// ClearCache 清空所有 handler 缓存（用于配置变更后）
func (e *ToolExecutor) ClearCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache = make(map[string]ToolHandler)
}

// Registry 返回关联的注册中心
func (e *ToolExecutor) Registry() *ToolRegistry {
	return e.registry
}

// ===== 核心执行入口 =====

// ExecuteRequest 执行请求
type ExecuteRequest struct {
	ToolName string         `json:"tool_name"`
	Args     map[string]any `json:"args"`
	// 可选：执行上下文（不传则使用空 ToolContext）
	ToolCtx *ToolContext `json:"-"`
}

// ExecuteResult 执行结果
type ExecuteResult struct {
	ToolResult
	Err error `json:"-"`
}

// Execute 执行单个工具
// 流程：
//  1. 从 registry 取工具
//  2. 检查 override.Disabled
//  3. 取出（或构造并缓存）装饰后 handler
//  4. 注入 toolName + toolCtx 到 context
//  5. 调用 handler
func (e *ToolExecutor) Execute(ctx context.Context, req ExecuteRequest) ExecuteResult {
	if req.ToolName == "" {
		return ExecuteResult{
			ToolResult: ErrorResult("", fmt.Errorf("tool_name 不能为空")),
			Err:        fmt.Errorf("tool_name 不能为空"),
		}
	}
	// 1. 取工具
	tool, err := e.registry.Get(req.ToolName)
	if err != nil {
		return ExecuteResult{
			ToolResult: ErrorResult(req.ToolName, err),
			Err:        err,
		}
	}
	// 2. 检查 disabled
	if o, ok := e.GetOverride(req.ToolName); ok && o.Disabled {
		err := fmt.Errorf("tool %s is disabled", req.ToolName)
		return ExecuteResult{
			ToolResult: ErrorResult(req.ToolName, err),
			Err:        err,
		}
	}
	// 3. 取出（或构造）装饰后 handler
	handler := e.getOrBuildHandler(tool)
	// 4. 注入 context
	execCtx := WithToolName(ctx, req.ToolName)
	if req.ToolCtx != nil {
		execCtx = WithToolContext(execCtx, req.ToolCtx)
	}
	// 5. 执行
	start := time.Now()
	result, err := handler(execCtx, req.Args)
	// 补全 Timing.DurationMs（装饰器内部各自计算耗时，外层再做一次总耗时统计）
	if result.Timing.DurationMs == 0 {
		result.Timing.DurationMs = time.Since(start).Milliseconds()
	}
	if result.AuditTrace == "" && req.ToolCtx != nil {
		result.AuditTrace = req.ToolCtx.AuditTrace
	}
	// 错误数据契约对齐（彻底修复脏数据根因，而非仅在 trace 层兜底）：
	// 当 Go 层 err 非空或执行失败时，保证返回的 ToolResult 自身完整
	// （ToolName / Error / Success 一致）。否则工具或装饰器在 ctx 取消等提前返回路径
	// 可能产出「Success=false / err!=nil 但 Error 为空」的零值结果，导致监控出现
	// status=abnormal 却 abnormal/error 两列皆空的脏 span，且调用方也拿不到错误详情。
	if err != nil || !result.Success {
		if result.ToolName == "" {
			result.ToolName = req.ToolName
		}
		if result.Error == "" {
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Error = "tool execution failed without error detail"
			}
		}
		result.Success = false
	}
	// 可观测性：把单次工具调用作为 tool_call 子 span 上报（observer 默认 nil，非阻塞）。
	if ToolTraceSink != nil {
		status := "ok"
		if !result.Success || err != nil {
			status = "abnormal"
		}
		// 此时 result.Error 已在边界对齐为完整错误详情（优先工具业务错误，回退 Go 层 err），
		// 不再需要在 trace 层单独兜底。
		ev := tracing.ToolTraceEvent{
			Kind:       model.SpanKindToolCall,
			TraceID:    trace.TraceIDFromContext(execCtx),
			ToolName:   req.ToolName,
			TurnIndex:  GetTurnIndex(execCtx),
			Input:      req.Args,
			Output:     result.Data,
			Error:      result.Error,
			DurationMs: result.Timing.DurationMs,
			Status:     status,
		}
		if tc := GetToolContext(execCtx); tc != nil {
			ev.AgentID = tc.AgentID
			ev.SessionID = tc.SessionID
			ev.CustomerID = tc.CustomerID
			ev.CallerID = tc.CallerID
		}
		ToolTraceSink(execCtx, ev)
	}
	return ExecuteResult{ToolResult: result, Err: err}
}

// getOrBuildHandler 取出（或构造并缓存）装饰后 handler
func (e *ToolExecutor) getOrBuildHandler(tool Tool) ToolHandler {
	name := tool.Name()
	e.mu.RLock()
	if h, ok := e.cache[name]; ok {
		e.mu.RUnlock()
		return h
	}
	e.mu.RUnlock()

	e.mu.Lock()
	defer e.mu.Unlock()
	// 双检
	if h, ok := e.cache[name]; ok {
		return h
	}
	h := e.buildHandler(tool)
	e.cache[name] = h
	return h
}

// buildHandler 构造装饰后 handler
func (e *ToolExecutor) buildHandler(tool Tool) ToolHandler {
	// 原始 handler：调用 tool.Execute
	raw := func(ctx context.Context, args map[string]any) (ToolResult, error) {
		start := time.Now()
		r, err := tool.Execute(ctx, args)
		dur := time.Since(start)
		// 补全 ToolResult 字段
		if r.ToolName == "" {
			r.ToolName = tool.Name()
		}
		if r.ExecutedAt.IsZero() {
			r.ExecutedAt = start
		}
		if r.Timing.DurationMs == 0 {
			r.Timing.DurationMs = dur.Milliseconds()
		}
		return r, err
	}

	// 解析该工具的 override
	timeout := e.config.DefaultTimeout
	policy := e.config.RetryPolicy
	if o, ok := e.overrides[tool.Name()]; ok {
		if o.Timeout > 0 {
			timeout = o.Timeout
		}
		if o.MaxRetries >= 0 {
			// 用 override 的重试策略
			policy = NewExponentialBackoffPolicy(
				o.MaxRetries+1, // MaxRetries 是"重试次数"，MaxAttempts = 重试次数 + 1（首次）
				func() time.Duration {
					if o.BaseDelay > 0 {
						return o.BaseDelay
					}
					return 100 * time.Millisecond
				}(),
				10*time.Second,
			)
		}
	}

	return BuildChainWithCircuitBreaker(raw,
		e.config.PermissionChecker,
		e.config.RateLimiter,
		e.config.CircuitBreaker,
		policy,
		timeout,
		e.config.AuditLogger,
		e.config.CostTracker,
	)
}

// ===== 批量执行 =====

// BatchExecuteRequest 批量执行请求
type BatchExecuteRequest struct {
	Requests []ExecuteRequest `json:"requests"`
	// 是否并发执行（false = 顺序执行）
	Parallel bool `json:"parallel"`
	// 并发执行时的最大并发数（0 = 不限）
	MaxConcurrency int `json:"max_concurrency,omitempty"`
	// 是否遇到第一个错误就停止（仅顺序模式生效）
	StopOnError bool `json:"stop_on_error,omitempty"`
}

// BatchExecuteResponse 批量执行响应
type BatchExecuteResponse struct {
	Results []ExecuteResult `json:"results"`
	// 成功数 / 失败数
	SuccessCount int `json:"success_count"`
	FailedCount  int `json:"failed_count"`
	// 总耗时
	TotalDurationMs int64 `json:"total_duration_ms"`
}

// BatchExecute 批量执行工具
func (e *ToolExecutor) BatchExecute(ctx context.Context, req BatchExecuteRequest) BatchExecuteResponse {
	start := time.Now()
	n := len(req.Requests)
	if n == 0 {
		return BatchExecuteResponse{}
	}

	results := make([]ExecuteResult, n)

	if req.Parallel {
		// 并发模式
		var sem chan struct{}
		if req.MaxConcurrency > 0 {
			sem = make(chan struct{}, req.MaxConcurrency)
		}
		var wg sync.WaitGroup
		for i, r := range req.Requests {
			wg.Add(1)
			if sem != nil {
				sem <- struct{}{}
			}
			go func(idx int, er ExecuteRequest) {
				defer wg.Done()
				if sem != nil {
					defer func() { <-sem }()
				}
				results[idx] = e.Execute(ctx, er)
			}(i, r)
		}
		wg.Wait()
	} else {
		// 顺序模式
		for i, r := range req.Requests {
			results[i] = e.Execute(ctx, r)
			if req.StopOnError && results[i].Err != nil {
				// 填充剩余为空结果
				for j := i + 1; j < n; j++ {
					results[j] = ExecuteResult{
						ToolResult: ErrorResult(r.ToolName, fmt.Errorf("skipped due to previous error")),
						Err:        fmt.Errorf("skipped due to previous error"),
					}
				}
				break
			}
		}
	}

	resp := BatchExecuteResponse{
		Results:         results,
		TotalDurationMs: time.Since(start).Milliseconds(),
	}
	for _, r := range results {
		if r.Err == nil && r.Success {
			resp.SuccessCount++
		} else {
			resp.FailedCount++
		}
	}
	return resp
}

// ===== LLM Function Calling 集成 =====

// LLMToolCall LLM 返回的 tool_call（OpenAI 兼容格式）
type LLMToolCall struct {
	ID       string          `json:"id"` // 调用 ID（用于回传 tool result 给 LLM）
	Function LLMToolFunction `json:"function"`
}

// LLMToolFunction LLM 工具调用 function 部分
type LLMToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// LLMToolResult 工具执行结果（回传给 LLM 的格式）
type LLMToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Content    string          `json:"content"` // 工具结果文本（通常是 ToolResult.Data 的 JSON）
	Success    bool            `json:"success"`
	Card       *model.RichCard `json:"card,omitempty"` // 工具产出的结构化富卡片
}

// DispatchByLLMToolCall 根据 LLM 返回的 tool_call 调度执行
// 一次接收多个 tool_call，并发执行，返回每个 tool_call 的结果
//
// 并发上限控制（默认 5）
// 设计依据：防止 LLM 一次返回 10+ tool_calls 时打爆下游服务
// 5 并发是经过权衡的平衡值：足够覆盖常见业务场景（如 customer.search + order.query 并行），
// 又能保护下游 DB/外部 API 不被突发流量打垮
func (e *ToolExecutor) DispatchByLLMToolCall(ctx context.Context, toolCalls []LLMToolCall, toolCtx *ToolContext) []LLMToolResult {
	if len(toolCalls) == 0 {
		return nil
	}
	// 本轮 LLM 推理序号（1-based），其下所有 tool_call 继承该序号，便于 UI 归并到同一 agent_turn。
	turnIdx := nextTurnIndex(ctx)
	dispatchCtx := WithTurnIndex(ctx, turnIdx)
	agentID := ""
	if toolCtx != nil {
		agentID = toolCtx.AgentID
	}
	start := time.Now()
	results := make([]LLMToolResult, len(toolCalls))

	// semaphore 控制并发上限
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for i, call := range toolCalls {
		wg.Add(1)
		sem <- struct{}{} // 获取信号量（阻塞时等待）
		go func(idx int, c LLMToolCall) {
			defer wg.Done()
			defer func() { <-sem }() // 释放信号量
			results[idx] = e.executeSingleLLMToolCall(dispatchCtx, c, toolCtx)
		}(i, call)
	}
	wg.Wait()
	// agent_turn 子 span：覆盖整轮 LLM 工具编排耗时（observer 默认 nil，非阻塞）。
	// 真实状态（彻底修复观测缺口）：上下文被取消（客户端断开 / turn 截止）或任一工具失败
	// 都标记 abnormal 并带上首个错误详情，否则 monitor 会把「失败的 turn」误报为 ok，
	// 导致 node_abnormal 对 agent_turn 维度永远为 0、隐藏真实异常 turn。
	turnStatus := tracing.StatusOk
	turnErr := ""
	if dispatchCtx.Err() != nil {
		turnStatus = tracing.StatusAbnormal
		turnErr = dispatchCtx.Err().Error()
	} else {
		for _, r := range results {
			if !r.Success {
				turnStatus = tracing.StatusAbnormal
				break
			}
		}
	}
	if ToolTraceSink != nil {
		ev := tracing.ToolTraceEvent{
			Kind:       model.SpanKindAgentTurn,
			TraceID:    trace.TraceIDFromContext(dispatchCtx),
			AgentID:    agentID,
			TurnIndex:  turnIdx,
			Input:      map[string]any{"tool_calls": len(toolCalls)},
			DurationMs: time.Since(start).Milliseconds(),
			Status:     turnStatus,
			Error:      turnErr,
		}
		// 工具失败导致的异常 turn：把首个失败工具的错误详情带给监控，便于定位根因。
		if turnStatus == tracing.StatusAbnormal && turnErr == "" {
			for _, r := range results {
				if !r.Success {
					if er := extractToolError(r); er != "" {
						ev.Error = er
					}
					break
				}
			}
		}
		ToolTraceSink(dispatchCtx, ev)
	}
	return results
}

// extractToolError 从 LLMToolResult.Content（ToolResult 的 JSON 序列化）中提取 error 字段，
// 供 agent_turn 异常 span 携带首个失败工具的错误详情，便于监控定位根因。
func extractToolError(r LLMToolResult) string {
	if r.Content == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(r.Content), &m); err != nil {
		return ""
	}
	if e, ok := m["error"]; ok {
		if s, ok := e.(string); ok {
			return s
		}
	}
	return ""
}

// executeSingleLLMToolCall 执行单个 LLM tool_call
func (e *ToolExecutor) executeSingleLLMToolCall(ctx context.Context, call LLMToolCall, toolCtx *ToolContext) LLMToolResult {
	// 解析 arguments JSON
	args := make(map[string]any)
	if call.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return LLMToolResult{
				ToolCallID: call.ID,
				Content:    fmt.Sprintf(`{"error":"arguments JSON 解析失败：%s"}`, err.Error()),
				Success:    false,
			}
		}
	}
	// 执行
	execResult := e.Execute(ctx, ExecuteRequest{
		ToolName: call.Function.Name,
		Args:     args,
		ToolCtx:  toolCtx,
	})
	// 序列化结果为 JSON 字符串
	content, _ := json.Marshal(execResult.ToolResult)
	contentStr := string(content)

	// 工具结果长度截断
	// 设计依据：GPT-3.5 context 限制 4K-16K tokens，单个工具结果不应超过 4000 字符（约 1000 tokens）
	// 截断后追加省略号 + 原始长度，让 LLM 知道数据被截断
	const maxContentLen = 4000
	if len(contentStr) > maxContentLen {
		originalLen := len(contentStr)
		contentStr = contentStr[:maxContentLen] + fmt.Sprintf(`...[truncated, original_size=%d]`, originalLen)
	}
	return LLMToolResult{
		ToolCallID: call.ID,
		Content:    contentStr,
		Success:    execResult.Err == nil && execResult.Success,
		Card:       execResult.ToolResult.Card,
	}
}

// ===== 便捷方法 =====

// ExecuteByName 便捷执行：直接传 toolName + args
func (e *ToolExecutor) ExecuteByName(ctx context.Context, toolName string, args map[string]any) (ToolResult, error) {
	r := e.Execute(ctx, ExecuteRequest{
		ToolName: toolName,
		Args:     args,
	})
	return r.ToolResult, r.Err
}

// ExecuteByNameWithCtx 便捷执行：带 ToolContext
func (e *ToolExecutor) ExecuteByNameWithCtx(ctx context.Context, toolName string, args map[string]any, toolCtx *ToolContext) (ToolResult, error) {
	r := e.Execute(ctx, ExecuteRequest{
		ToolName: toolName,
		Args:     args,
		ToolCtx:  toolCtx,
	})
	return r.ToolResult, r.Err
}

// ListAvailableTools 列出所有可用工具（排除被 disabled 的）
func (e *ToolExecutor) ListAvailableTools() []Tool {
	all := e.registry.List()
	out := make([]Tool, 0, len(all))
	for _, t := range all {
		if o, ok := e.GetOverride(t.Name()); ok && o.Disabled {
			continue
		}
		out = append(out, t)
	}
	return out
}

// ListAvailableLLMFunctions 列出所有可用工具的 LLM Function Calling 格式
func (e *ToolExecutor) ListAvailableLLMFunctions() []LLMFunction {
	tools := e.ListAvailableTools()
	out := make([]LLMFunction, 0, len(tools))
	for _, t := range tools {
		out = append(out, ToLLMFunction(t))
	}
	return out
}

// ===== 全局执行器（可选使用） =====

var (
	globalExecutor     *ToolExecutor
	globalExecutorOnce sync.Once
)

// InitGlobalExecutor 初始化全局执行器（应用启动时调用一次）
func InitGlobalExecutor(registry *ToolRegistry, config ToolExecutorConfig) {
	globalExecutorOnce.Do(func() {
		globalExecutor = NewToolExecutor(registry, config)
	})
}

// GetGlobalExecutor 获取全局执行器
// 必须先调用 InitGlobalExecutor 初始化，否则返回 nil
func GetGlobalExecutor() *ToolExecutor {
	return globalExecutor
}

// SetGlobalExecutor 替换全局执行器（用于测试 / 热重载）
func SetGlobalExecutor(exec *ToolExecutor) {
	globalExecutor = exec
}
