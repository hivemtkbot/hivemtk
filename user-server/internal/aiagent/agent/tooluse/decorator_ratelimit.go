package tooluse

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type RateLimiter interface {
	Acquire(ctx context.Context, key string) error
}

func RateLimitDecorator(limiter RateLimiter) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			if limiter == nil {
				return next(ctx, args)
			}
			toolName := GetToolName(ctx)
			tc := GetToolContext(ctx)

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

type NoOpRateLimiter struct{}

type TokenBucketLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64 
	burst   int     
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

