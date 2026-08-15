package tooluse

import (
	"context"
	"fmt"
)

type PermissionChecker interface {
	Check(ctx context.Context, toolName string, tc *ToolContext) error
}

var (
	ErrPermissionDenied = fmt.Errorf("permission denied")
	ErrRateLimited = fmt.Errorf("rate limited")
	ErrToolTimeout = fmt.Errorf("tool execution timeout")
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

