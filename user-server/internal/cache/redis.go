package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache Redis 缓存实现
type RedisCache struct {
	client       *redis.Client
	sharedClient bool
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

// NewRedisCache 创建 Redis 缓存
func NewRedisCache(config RedisConfig) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Host, config.Port),
		Password: config.Password,
		DB:       config.DB,
		PoolSize: config.PoolSize,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisCache{
		client: client,
	}, nil
}

// Get 获取缓存
func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

// Set 设置缓存
func (r *RedisCache) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}

// SetNX 仅在 key 不存在时设置
func (r *RedisCache) SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	return r.client.SetNX(ctx, key, value, expiration).Result()
}

// ReleaseLock 仅当 key 的值等于 token 时原子删除（Lua 脚本），安全释放 SetNX 获取的锁，
// 避免误删其他持有者的锁。返回 true 表示成功释放。
func (r *RedisCache) ReleaseLock(ctx context.Context, key, token string) (bool, error) {
	script := redis.NewScript(`if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end`)
	n, err := script.Run(ctx, r.client, []string{key}, token).Int64()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// LPush 向列表头部推入
func (r *RedisCache) LPush(ctx context.Context, key string, value any, expiration time.Duration) error {
	if err := r.client.LPush(ctx, key, value).Err(); err != nil {
		return err
	}
	if expiration > 0 {
		r.client.Expire(ctx, key, expiration)
	}
	return nil
}

// RPush 向列表尾部推入
func (r *RedisCache) RPush(ctx context.Context, key string, value any, expiration time.Duration) error {
	if err := r.client.RPush(ctx, key, value).Err(); err != nil {
		return err
	}
	if expiration > 0 {
		r.client.Expire(ctx, key, expiration)
	}
	return nil
}

// LPop 从列表头部弹出
func (r *RedisCache) LPop(ctx context.Context, key string) (string, error) {
	return r.client.LPop(ctx, key).Result()
}

// LRange 获取列表区间
func (r *RedisCache) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.LRange(ctx, key, start, stop).Result()
}

// PopAll 原子取出并删除整个 list（Lua 脚本保证 LRange+DEL 之间无窗口）。
func (r *RedisCache) PopAll(ctx context.Context, key string) ([]string, error) {
	const popAllLua = `local v = redis.call('LRANGE', KEYS[1], 0, -1)
redis.call('DEL', KEYS[1])
return v`
	res, err := r.client.Eval(ctx, popAllLua, []string{key}).Result()
	if err != nil {
		return nil, err
	}
	vals, _ := res.([]any)
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// LLen 列表长度
func (r *RedisCache) LLen(ctx context.Context, key string) (int64, error) {
	return r.client.LLen(ctx, key).Result()
}

// Delete 删除缓存
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// Incr 原子自增并返回新值（首次 value=1，expiration>0 时作为 TTL 应用）
func (r *RedisCache) Incr(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	n, err := r.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 && expiration > 0 {
		if err := r.client.Expire(ctx, key, expiration).Err(); err != nil {
			return n, err
		}
	}
	return n, nil
}

// Exists 检查缓存是否存在
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// GetJSON 获取 JSON 缓存并反序列化
func (r *RedisCache) GetJSON(ctx context.Context, key string, dest any) error {
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// SetJSON 设置 JSON 缓存
func (r *RedisCache) SetJSON(ctx context.Context, key string, value any, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, expiration).Err()
}

// Clear 清空缓存
func (r *RedisCache) Clear(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

// NewRedisCacheWithClient 基于已存在的（共享）Redis 客户端创建缓存实现。
// 不拥有 client 生命周期：Close 为 no-op，由调用方（main）负责关闭连接。
// 用于全局缓存单例复用 main 构建的同一 *redis.Client，避免重复连接。
func NewRedisCacheWithClient(client *redis.Client) *RedisCache {
	return &RedisCache{client: client, sharedClient: true}
}

// Close 关闭连接
func (r *RedisCache) Close() error {
	if r.sharedClient {
		return nil
	}
	return r.client.Close()
}
