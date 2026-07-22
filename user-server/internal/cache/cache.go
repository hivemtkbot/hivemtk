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
	// SetNX 原子性：仅在 key 不存在时设置值，返回 true 表示设置成功，false 表示 key 已存在
	// 用途：分布式锁 / 单飞去重
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error)
	// Delete 删除缓存
	Delete(ctx context.Context, key string) error
	// Exists 检查缓存是否存在
	Exists(ctx context.Context, key string) (bool, error)
	// GetJSON 获取 JSON 缓存并反序列化
	GetJSON(ctx context.Context, key string, dest any) error
	// SetJSON 设置 JSON 缓存
	SetJSON(ctx context.Context, key string, value any, expiration time.Duration) error
	// LPush 向列表头部推入一个元素（消息待处理队列 / 串行化防抖）
	LPush(ctx context.Context, key string, value any, expiration time.Duration) error
	// RPush 向列表尾部推入一个元素
	RPush(ctx context.Context, key string, value any, expiration time.Duration) error
	// LPop 从列表头部弹出一个元素（阻塞语义由调用方控制）
	LPop(ctx context.Context, key string) (string, error)
	// LRange 获取列表区间元素
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	// LLen 列表长度
	LLen(ctx context.Context, key string) (int64, error)
	// Clear 清空缓存
	Clear(ctx context.Context) error
}
