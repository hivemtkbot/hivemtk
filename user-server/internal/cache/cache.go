package cache

import (
	"context"
	"time"
)

// Cache 缓存接口
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key, token string) (bool, error)
	Incr(ctx context.Context, key string, expiration time.Duration) (int64, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	GetJSON(ctx context.Context, key string, dest any) error
	SetJSON(ctx context.Context, key string, value any, expiration time.Duration) error
	LPush(ctx context.Context, key string, value any, expiration time.Duration) error
	RPush(ctx context.Context, key string, value any, expiration time.Duration) error
	LPop(ctx context.Context, key string) (string, error)
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	// PopAll 原子取出并删除整个 list（T5 防抖窗口消费用）。
	// 必须原子：LRange+Delete 分离实现存在「新消息 LPush 落在两步之间被 Delete
	// 抹掉」的丢消息窗口（二次审查 S2 发现）。
	PopAll(ctx context.Context, key string) ([]string, error)
	LLen(ctx context.Context, key string) (int64, error)
	Clear(ctx context.Context) error
}

