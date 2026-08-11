package tooluse

import (
	"context"
	"fmt"
)

type PermissionChecker interface {
	// Check 返回 nil 表示放行，非 nil 表示拒绝
	Check(ctx context.Context, toolName string, tc *ToolContext) error
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

				return next(ctx, args)
			}

			toolName, _ := ctx.Value(toolNameKey{}).(string)
			tc := GetToolContext(ctx)
			if err := checker.Check(ctx, toolName, tc); err != nil {
				return ErrorResult(toolName, fmt.Errorf("%w: %v", ErrPermissionDenied, err)), ErrPermissionDenied
			}
			return next(ctx, args)
		}
	}
}

type NoOpPermissionChecker struct{}
