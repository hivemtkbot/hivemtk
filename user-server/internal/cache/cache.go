package cache

import (
	"context"
	"time"
)

// Cache 缓存接口
type Cache interface {
	// Get 获取缓存
	Get(ctx context.Context, key string) (string, error)
	// Set 设置缓存
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	// Delete 删除缓存
	Delete(ctx context.Context, key string) error
	// Exists 检查缓存是否存在
	Exists(ctx context.Context, key string) (bool, error)
	// GetJSON 获取 JSON 缓存并反序列化
	GetJSON(ctx context.Context, key string, dest any) error
	// SetJSON 设置 JSON 缓存
	SetJSON(ctx context.Context, key string, value any, expiration time.Duration) error
	// Clear 清空缓存
	Clear(ctx context.Context) error
}
