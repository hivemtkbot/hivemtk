package tooluse

import (
	"context"
	"fmt"
	"time"
)

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
