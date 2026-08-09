package tooluse

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/tracing"
)

// fakeResultTool 返回预设的 ToolResult 与 err，用于精确复现各类错误/成功路径。
type fakeResultTool struct {
	BaseTool
	res ToolResult
	err error
}

func (t *fakeResultTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	return t.res, t.err
}

func newTestTool(name string, res ToolResult, err error) *fakeResultTool {
	return &fakeResultTool{
		BaseTool: BaseTool{
			NameVal:        name,
			CategoryVal:    ToolCategory("test"),
			DescriptionVal: "regression-test",
			ParamsVal:      ToolParameters{Type: "object"},
		},
		res: res,
		err: err,
	}
}

// runToolAndCapture 注册工具、接线 trace sink、执行并返回结果与捕获到的 trace 事件。
func runToolAndCapture(t *testing.T, tool Tool, cfg ToolExecutorConfig, ctx context.Context, req ExecuteRequest) (ExecuteResult, *tracing.ToolTraceEvent) {
	t.Helper()
	reg := NewToolRegistry()
	if err := reg.Register(tool); err != nil {
		t.Fatalf("register: %v", err)
	}
	exec := NewToolExecutor(reg, cfg)
	var captured *tracing.ToolTraceEvent
	prev := ToolTraceSink
	ToolTraceSink = func(_ context.Context, ev tracing.ToolTraceEvent) {
		e := ev
		captured = &e
	}
	defer func() { ToolTraceSink = prev }()
	if req.ToolName == "" {
		req.ToolName = tool.Name()
	}
	return exec.Execute(ctx, req), captured
}

// TestExecute_ErrorResultContract 回归（根因场景 A：工具直返 (零值, err)）：
// 当 err!=nil 但 result.Error 为空时，Execute 必须在边界把返回的 ToolResult 补全为完整错误结果，
// 使监控与调用方都能拿到错误详情——彻底修复「status=abnormal 但 abnormal/error 皆空」脏数据。
func TestExecute_ErrorResultContract(t *testing.T) {
	sentinel := errors.New("boom: context canceled")
	tool := newTestTool("test.no_detail", ToolResult{}, sentinel)
	res, captured := runToolAndCapture(t, tool, ToolExecutorConfig{}, context.Background(), ExecuteRequest{Args: map[string]any{}})

	if res.ToolResult.Success {
		t.Fatal("失败调用应标记 Success=false")
	}
	if res.ToolResult.ToolName != tool.Name() {
		t.Fatalf("ToolResult.ToolName 应为 %q，实际 %q", tool.Name(), res.ToolResult.ToolName)
	}
	if res.ToolResult.Error != sentinel.Error() {
		t.Fatalf("ToolResult.Error 应为 %q，实际 %q", sentinel.Error(), res.ToolResult.Error)
	}
	if captured == nil {
		t.Fatal("未收到 tool_call trace 事件")
	}
	if captured.Status != "abnormal" {
		t.Fatalf("异常工具应标记 abnormal，实际 %q", captured.Status)
	}
	if captured.Error != sentinel.Error() {
		t.Fatalf("Error 应为 %q，实际 %q", sentinel.Error(), captured.Error)
	}
}

// TestRetryDecorator_ContextCancelBeforeExecute 回归（根因场景 B：调用前 ctx 已取消）：
// RetryDecorator 的循环入口 ctx 检查必须返回完整 ToolResult（含 Error），而非零值结果。
func TestRetryDecorator_ContextCancelBeforeExecute(t *testing.T) {
	tool := newTestTool("test.ctx_cancel_before", ToolResult{}, nil)
	cfg := ToolExecutorConfig{RetryPolicy: NewExponentialBackoffPolicy(3, 50*time.Millisecond, 200*time.Millisecond)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 调用前即取消

	res, captured := runToolAndCapture(t, tool, cfg, ctx, ExecuteRequest{Args: map[string]any{}})

	if res.Err == nil {
		t.Fatal("ctx 取消应返回非 nil err")
	}
	if res.ToolResult.Success {
		t.Fatal("ctx 取消结果应 Success=false")
	}
	if res.ToolResult.ToolName != tool.Name() {
		t.Fatalf("ToolResult.ToolName 应为 %q，实际 %q", tool.Name(), res.ToolResult.ToolName)
	}
	if res.ToolResult.Error == "" {
		t.Fatal("根因未修复：ctx 取消仍返回零值结果（Error 为空）")
	}
	if res.ToolResult.Error != context.Canceled.Error() {
		t.Fatalf("Error 应为 %q，实际 %q", context.Canceled.Error(), res.ToolResult.Error)
	}
	if captured == nil || captured.Error != context.Canceled.Error() {
		t.Fatalf("trace 应记录 ctx 取消错误详情")
	}
}

// TestRetryDecorator_ContextCancelDuringBackoff 回归（根因场景 C：退避等待中 ctx 取消）：
// RetryDecorator 在 backoff 等待的 select 中捕获 ctx.Done 也必须返回完整 ToolResult，而非零值。
func TestRetryDecorator_ContextCancelDuringBackoff(t *testing.T) {
	// 首次（attempt 0）返回「可重试」错误，进入 attempt 1 的退避等待；期间取消 ctx。
	retryable := errors.New("transient network blip")
	tool := newTestTool("test.ctx_cancel_backoff", ToolResult{}, retryable)
	cfg := ToolExecutorConfig{RetryPolicy: NewExponentialBackoffPolicy(3, 2*time.Second, 2*time.Second)}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	res, captured := runToolAndCapture(t, tool, cfg, ctx, ExecuteRequest{Args: map[string]any{}})

	if res.Err == nil {
		t.Fatal("退避中取消应返回非 nil err")
	}
	if res.ToolResult.Error == "" {
		t.Fatal("根因未修复：退避等待中 ctx 取消仍返回零值结果（Error 为空）")
	}
	if res.ToolResult.Error != context.Canceled.Error() {
		t.Fatalf("Error 应为 %q，实际 %q", context.Canceled.Error(), res.ToolResult.Error)
	}
	if res.ToolResult.Success {
		t.Fatal("应 Success=false")
	}
	if captured == nil || captured.Error != context.Canceled.Error() {
		t.Fatalf("trace 应记录 ctx 取消错误详情")
	}
}

// TestExecute_BusinessFailureWithoutError 回归（场景 D：业务失败但 Error 为空、err 为 nil）：
// 边界契约必须兜底填充占位错误详情，避免「abnormal 但 error 为空」。
func TestExecute_BusinessFailureWithoutError(t *testing.T) {
	tool := newTestTool("test.biz_no_error", ToolResult{Success: false, Error: ""}, nil)
	res, captured := runToolAndCapture(t, tool, ToolExecutorConfig{}, context.Background(), ExecuteRequest{Args: map[string]any{}})

	if res.ToolResult.Success {
		t.Fatal("应 Success=false")
	}
	if res.ToolResult.Error == "" {
		t.Fatal("业务失败且无错误详情时，边界必须填充占位错误，否则仍产生脏数据")
	}
	if res.ToolResult.Error != "tool execution failed without error detail" {
		t.Fatalf("占位错误不符，实际 %q", res.ToolResult.Error)
	}
	if captured == nil || captured.Error != res.ToolResult.Error {
		t.Fatalf("trace 应同步占位错误详情")
	}
}

// TestExecute_BusinessErrorPreserved 规格（场景 E：工具自带业务错误必须被保留，不得被边界覆盖）。
func TestExecute_BusinessErrorPreserved(t *testing.T) {
	const bizErr = "business: customer_not_found"
	tool := newTestTool("test.biz_preserved", ToolResult{Success: false, Error: bizErr, ToolName: "test.biz_preserved"}, nil)
	res, captured := runToolAndCapture(t, tool, ToolExecutorConfig{}, context.Background(), ExecuteRequest{Args: map[string]any{}})

	if res.ToolResult.Error != bizErr {
		t.Fatalf("业务错误应被原样保留，实际 %q", res.ToolResult.Error)
	}
	if captured == nil || captured.Error != bizErr {
		t.Fatalf("trace 应保留业务错误，实际 %q", captured.Error)
	}
}

// TestExecute_SuccessWithErrForcedFalse 规格（场景 F：Success=true 却带 err 的矛盾态）：
// 边界应强制 Success=false 并补全 Error，防止调用方误判成功。
func TestExecute_SuccessWithErrForcedFalse(t *testing.T) {
	sentinel := errors.New("weird: success-but-err")
	tool := newTestTool("test.success_with_err", ToolResult{Success: true}, sentinel)
	res, _ := runToolAndCapture(t, tool, ToolExecutorConfig{}, context.Background(), ExecuteRequest{Args: map[string]any{}})

	if res.ToolResult.Success {
		t.Fatal("带 err 的结果应被强制 Success=false")
	}
	if res.ToolResult.Error != sentinel.Error() {
		t.Fatalf("Error 应为 %q，实际 %q", sentinel.Error(), res.ToolResult.Error)
	}
}

// TestExecute_SuccessPathUnchanged 规格（场景 G：成功路径不得被误标为 abnormal）：
// 确保边界补偿逻辑不会把正常成功调用污染。
func TestExecute_SuccessPathUnchanged(t *testing.T) {
	tool := newTestTool("test.ok", ToolResult{Success: true, Data: "done"}, nil)
	_, captured := runToolAndCapture(t, tool, ToolExecutorConfig{}, context.Background(), ExecuteRequest{Args: map[string]any{}})

	if captured == nil {
		t.Fatal("应收到 trace 事件")
	}
	if captured.Status != "ok" {
		t.Fatalf("成功调用应标记 ok，实际 %q", captured.Status)
	}
	if captured.Error != "" {
		t.Fatalf("成功调用 Error 应为空，实际 %q", captured.Error)
	}
}

// TestRetry_PreservesBusinessErrorUnderRetry 规格（场景 H：重试链下业务错误须保留）：
// 工具返回完整业务错误（非重试）时，RetryDecorator 的 ensureErrorResult 不得覆盖。
func TestRetry_PreservesBusinessErrorUnderRetry(t *testing.T) {
	const bizErr = "business: order_already_exists"
	tool := newTestTool("test.retry_biz", ToolResult{Success: false, Error: bizErr, ToolName: "test.retry_biz"}, nil)
	cfg := ToolExecutorConfig{RetryPolicy: NewExponentialBackoffPolicy(3, 10*time.Millisecond, 50*time.Millisecond)}
	res, captured := runToolAndCapture(t, tool, cfg, context.Background(), ExecuteRequest{Args: map[string]any{}})

	if res.ToolResult.Error != bizErr {
		t.Fatalf("重试链下业务错误应被保留，实际 %q", res.ToolResult.Error)
	}
	if res.ToolResult.Timing.RetryCount != 0 {
		t.Fatalf("非重试错误不应消耗重试次数，实际 %d", res.ToolResult.Timing.RetryCount)
	}
	if captured == nil || captured.Error != bizErr {
		t.Fatalf("trace 应保留业务错误，实际 %q", captured.Error)
	}
}

// ===== agent_turn span 状态准确性（彻底修复观测缺口：原永远写 ok）=====

// runDispatchAndCaptureTurn 注册工具、接线 trace sink、执行整轮调度，并抽出 agent_turn 事件。
func runDispatchAndCaptureTurn(t *testing.T, tool Tool, cfg ToolExecutorConfig, ctx context.Context, calls []LLMToolCall) ([]LLMToolResult, *tracing.ToolTraceEvent) {
	t.Helper()
	reg := NewToolRegistry()
	if err := reg.Register(tool); err != nil {
		t.Fatalf("register: %v", err)
	}
	exec := NewToolExecutor(reg, cfg)
	var events []tracing.ToolTraceEvent
	var mu sync.Mutex
	prev := ToolTraceSink
	ToolTraceSink = func(_ context.Context, ev tracing.ToolTraceEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}
	defer func() { ToolTraceSink = prev }()
	results := exec.DispatchByLLMToolCall(ctx, calls, nil)
	var turn *tracing.ToolTraceEvent
	for i := range events {
		if events[i].Kind == model.SpanKindAgentTurn {
			e := events[i]
			turn = &e
		}
	}
	return results, turn
}

// ctxAwareTool 在 ctx 已取消时返回 context.Canceled（精确复现「客户端断开 / turn 截止」）。
type ctxAwareTool struct {
	BaseTool
}

func (t *ctxAwareTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if ctx.Err() != nil {
		return ErrorResult(t.Name(), ctx.Err()), ctx.Err()
	}
	return SuccessResult(t.Name(), "ok"), nil
}

func newCtxAwareTool(name string) *ctxAwareTool {
	return &ctxAwareTool{BaseTool: BaseTool{
		NameVal:        name,
		CategoryVal:    ToolCategory("test"),
		DescriptionVal: "regression-test",
		ParamsVal:      ToolParameters{Type: "object"},
	}}
}

// TestDispatch_AgentTurnStatusOK 全部工具成功时 agent_turn 必须标记 ok（不被误标异常）。
func TestDispatch_AgentTurnStatusOK(t *testing.T) {
	tool := newTestTool("test.turn_ok", SuccessResult("test.turn_ok", "done"), nil)
	calls := []LLMToolCall{
		{ID: "c1", Function: LLMToolFunction{Name: "test.turn_ok", Arguments: "{}"}},
		{ID: "c2", Function: LLMToolFunction{Name: "test.turn_ok", Arguments: "{}"}},
	}
	results, turn := runDispatchAndCaptureTurn(t, tool, ToolExecutorConfig{}, context.Background(), calls)
	for _, r := range results {
		if !r.Success {
			t.Fatalf("工具应成功，实际 %+v", r)
		}
	}
	if turn == nil {
		t.Fatal("未收到 agent_turn trace 事件")
	}
	if turn.Status != tracing.StatusOk {
		t.Fatalf("全成功应标记 ok，实际 %q", turn.Status)
	}
	if turn.Error != "" {
		t.Fatalf("成功 turn Error 应为空，实际 %q", turn.Error)
	}
}

// TestDispatch_AgentTurnStatusAbnormalOnToolFailure 任一工具失败时 agent_turn 必须标记 abnormal
// 并携带首个失败工具的错误详情（monitor 才能定位根因，而非误报 ok）。
func TestDispatch_AgentTurnStatusAbnormalOnToolFailure(t *testing.T) {
	const bizErr = "business: downstream_timeout"
	failTool := newTestTool("test.turn_fail", ToolResult{Success: false, Error: bizErr, ToolName: "test.turn_fail"}, nil)
	okTool := newTestTool("test.turn_ok2", SuccessResult("test.turn_ok2", "done"), nil)
	reg := NewToolRegistry()
	if err := reg.Register(failTool); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(okTool); err != nil {
		t.Fatal(err)
	}
	// 用同一 registry 但 DispatchByLLMToolCall 内部按 call.Function.Name 查找 —— 需构造带两工具的 executor。
	exec := NewToolExecutor(reg, ToolExecutorConfig{})
	var events []tracing.ToolTraceEvent
	var mu sync.Mutex
	prev := ToolTraceSink
	ToolTraceSink = func(_ context.Context, ev tracing.ToolTraceEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}
	defer func() { ToolTraceSink = prev }()
	calls := []LLMToolCall{
		{ID: "c1", Function: LLMToolFunction{Name: "test.turn_fail", Arguments: "{}"}},
		{ID: "c2", Function: LLMToolFunction{Name: "test.turn_ok2", Arguments: "{}"}},
	}
	exec.DispatchByLLMToolCall(context.Background(), calls, nil)
	var turn *tracing.ToolTraceEvent
	for i := range events {
		if events[i].Kind == model.SpanKindAgentTurn {
			e := events[i]
			turn = &e
		}
	}
	if turn == nil {
		t.Fatal("未收到 agent_turn trace 事件")
	}
	if turn.Status != tracing.StatusAbnormal {
		t.Fatalf("含失败工具应标记 abnormal，实际 %q", turn.Status)
	}
	if turn.Error != bizErr {
		t.Fatalf("异常 turn 应携带首个失败错误 %q，实际 %q", bizErr, turn.Error)
	}
}

// TestDispatch_AgentTurnStatusAbnormalOnCancel 整轮调度 ctx 被取消（客户端断开）时
// agent_turn 必须标记 abnormal 并带 context canceled（原代码永远写 ok，会隐藏断开）。
func TestDispatch_AgentTurnStatusAbnormalOnCancel(t *testing.T) {
	tool := newCtxAwareTool("test.turn_cancel")
	calls := []LLMToolCall{
		{ID: "c1", Function: LLMToolFunction{Name: "test.turn_cancel", Arguments: "{}"}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 调度前即取消
	_, turn := runDispatchAndCaptureTurn(t, tool, ToolExecutorConfig{}, ctx, calls)
	if turn == nil {
		t.Fatal("未收到 agent_turn trace 事件")
	}
	if turn.Status != tracing.StatusAbnormal {
		t.Fatalf("ctx 取消应标记 abnormal，实际 %q", turn.Status)
	}
	if turn.Error != context.Canceled.Error() {
		t.Fatalf("异常 turn 应携带 %q，实际 %q", context.Canceled.Error(), turn.Error)
	}
}
