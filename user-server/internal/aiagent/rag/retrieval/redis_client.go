package ragretrieval

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// RedisClient Redis 客户端接口
//
// 仅暴露 ragretrieval 包需要的 Get / Set 方法，避免全量 redis.Cmdable 泄漏。
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

// GoRedisAdapter 适配 github.com/redis/go-redis/v9 *redis.Client
type GoRedisAdapter struct {
	client *redis.Client
}

// NewGoRedisAdapter 创建适配器
//
// client 为 nil 时返回 nil（表示禁用 Redis 缓存）
func NewGoRedisAdapter(client *redis.Client) *GoRedisAdapter {
	if client == nil {
		return nil
	}
	return &GoRedisAdapter{client: client}
}

// Get 实现 RedisClient 接口
func (a *GoRedisAdapter) Get(ctx context.Context, key string) (string, error) {
	if a == nil || a.client == nil {
		return "", redis.Nil
	}
	return a.client.Get(ctx, key).Result()
}

// Set 实现 RedisClient 接口
func (a *GoRedisAdapter) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if a == nil || a.client == nil {
		return nil
	}
	return a.client.Set(ctx, key, value, ttl).Err()
}

// IsRedisNil 判断错误是否为 redis.Nil（key 不存在）
func IsRedisNil(err error) bool {
	return err == redis.Nil
}
