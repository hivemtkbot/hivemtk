package ragretrieval

// redis_client.go Redis 客户端抽象与 go-redis 适配
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十四章 §14.4.6 / §14.4.10
//
// 设计原则:
//   - L4 不直接依赖 *redis.Client 具体类型，通过 RedisClient 接口解耦
//   - 测试可注入 mock；生产用 GoRedisAdapter 包装 github.com/redis/go-redis/v9 *redis.Client
//   - nil Redis 客户端表示禁用缓存（直接走 DB / TEI）

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// RedisClient Redis 客户端接口
//
// 仅暴露 ragretrieval 包需要的 Get / Set 方法，避免全量 redis.Cmdable 泄漏。
type RedisClient interface {
	// Get 读取字符串缓存；key 不存在时返回 ("", redis.Nil)
	Get(ctx context.Context, key string) (string, error)
	// Set 写入字符串缓存带 TTL
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

// Compile-time 接口断言
