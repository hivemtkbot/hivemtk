package tooluse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// D08: sentinel → error_code 映射
func TestD08_ClassifyToolError(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("%w: missing x", ErrParamValidationFailed), ToolErrInvalidParams},
		{fmt.Errorf("%w: too fast", ErrRateLimited), ToolErrRateLimited},
		{fmt.Errorf("%w: no role", ErrPermissionDenied), ToolErrPermissionDenied},
		{fmt.Errorf("%w: 3s", ErrToolTimeout), ToolErrTimeout},
		{context.DeadlineExceeded, ToolErrTimeout},
		{fmt.Errorf("%w: boom", ErrToolPanic), ToolErrPanic},
		{fmt.Errorf("%w: need approval", ErrApprovalDenied), ToolErrApprovalDenied},
		{fmt.Errorf("%w: customer on dnc", ErrDNCBlocked), ToolErrDNCBlocked},
		{fmt.Errorf("%w: open 30s", ErrCircuitOpen), ToolErrCircuitOpen},
		{context.Canceled, ToolErrInternal},
		{ErrLoopDetected, ToolErrInternal},
		{errors.New("some random failure"), ToolErrInternal},
	}
	for _, c := range cases {
		if got := ClassifyToolError(c.err); got != c.want {
			t.Errorf("ClassifyToolError(%v)=%s want=%s", c.err, got, c.want)
		}
	}
	if got := ClassifyToolError(nil); got != "" {
		t.Errorf("nil error 应返回空串, got %s", got)
	}
}

// D08: 两层 %w 包装穿透
func TestD08_WrappedChain(t *testing.T) {
	inner := fmt.Errorf("db down: %w", ErrCircuitOpen)
	outer := fmt.Errorf("call failed: %w", inner)
	if got := ClassifyToolError(outer); got != ToolErrCircuitOpen {
		t.Errorf("want TOOL_CIRCUIT_OPEN, got %s", got)
	}
}

// D08: ErrorResult 填充 error_code；成功结果不含
func TestD08_ErrorResultHasCode(t *testing.T) {
	er := ErrorResult("query_order", fmt.Errorf("%w: denied by role", ErrPermissionDenied))
	if er.ErrorCode != ToolErrPermissionDenied {
		t.Errorf("want %s, got %s", ToolErrPermissionDenied, er.ErrorCode)
	}
	b, _ := json.Marshal(er)
	if !jsonContains(string(b), `"error_code":"TOOL_PERMISSION_DENIED"`) {
		t.Errorf("serialized missing error_code: %s", b)
	}
	sr := SuccessResult("query_order", map[string]any{"ok": true})
	b2, _ := json.Marshal(sr)
	if jsonContains(string(b2), "error_code") {
		t.Errorf("success result should omit error_code: %s", b2)
	}
}

// D08: executeSingleLLMToolCall 的手写失败 Content 合法且带码
func TestD08_LLMToolResultContentValidJSON(t *testing.T) {
	e := &ToolExecutor{}
	res := e.executeSingleLLMToolCall(context.Background(), LLMToolCall{
		ID: "call-1",
		Function: LLMToolFunction{
			Name:      "any_tool",
			Arguments: `{bad json`,
		},
	}, &ToolContext{})
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("Content 非法 JSON: %v (%s)", err, res.Content)
	}
	if m["error_code"] != ToolErrInvalidParams {
		t.Errorf("error_code=%v want %s", m["error_code"], ToolErrInvalidParams)
	}
}

// D08: isNonRetryableError 基于 ClassifyToolError 判定后语义不回退
func TestD08_NonRetryableAlignment(t *testing.T) {
	if !isNonRetryableError(ErrPermissionDenied) || !isNonRetryableError(ErrCircuitOpen) ||
		!isNonRetryableError(ErrLoopDetected) || !isNonRetryableError(context.Canceled) {
		t.Error("强失败/取消/循环应不可重试")
	}
	if isNonRetryableError(fmt.Errorf("%w: x", ErrToolTimeout)) || isNonRetryableError(ErrToolPanic) ||
		isNonRetryableError(errors.New("random")) {
		t.Error("timeout/panic/random 应保持可重试")
	}
}

func jsonContains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
