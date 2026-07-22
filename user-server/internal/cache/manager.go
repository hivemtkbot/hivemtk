package cache

import (
	"context"
	"marketing/internal/pkg/utils/logger"
	"time"
)

// CacheManager 缓存管理器
type CacheManager struct {
	cache Cache
}

// CacheConfig 缓存配置
type CacheConfig struct {
	Type       string        `yaml:"type"` // memory 或 redis
	Redis      RedisConfig   `yaml:"redis"`
	DefaultTTL time.Duration `yaml:"default_ttl"`
}

// NewCacheManager 创建缓存管理器
func NewCacheManager(config CacheConfig) (*CacheManager, error) {
	var cache Cache
	var err error

	switch config.Type {
	case "redis":
		cache, err = NewRedisCache(config.Redis)
		if err != nil {
			// Redis 连接失败，回退到内存缓存
			logger.Errorf("Redis 连接失败，使用内存缓存：%v", err)
			cache = NewMemoryCache()
		}
	case "memory", "":
		cache = NewMemoryCache()
	default:
		cache = NewMemoryCache()
	}

	return &CacheManager{
		cache: cache,
	}, nil
}

// Get 获取缓存
func (m *CacheManager) Get(ctx context.Context, key string) (string, error) {
	return m.cache.Get(ctx, key)
}

// Set 设置缓存
func (m *CacheManager) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	if expiration == 0 {
		expiration = m.getDefaultTTL()
	}
	return m.cache.Set(ctx, key, value, expiration)
}

// SetNX 仅在 key 不存在时设置
func (m *CacheManager) SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	if expiration == 0 {
		expiration = m.getDefaultTTL()
	}
	return m.cache.SetNX(ctx, key, value, expiration)
}

// LPush 向列表头部推入
func (m *CacheManager) LPush(ctx context.Context, key string, value any, expiration time.Duration) error {
	if expiration == 0 {
		expiration = m.getDefaultTTL()
	}
	return m.cache.LPush(ctx, key, value, expiration)
}

// RPush 向列表尾部推入
func (m *CacheManager) RPush(ctx context.Context, key string, value any, expiration time.Duration) error {
	if expiration == 0 {
		expiration = m.getDefaultTTL()
	}
	return m.cache.RPush(ctx, key, value, expiration)
}

// LPop 从列表头部弹出
func (m *CacheManager) LPop(ctx context.Context, key string) (string, error) {
	return m.cache.LPop(ctx, key)
}

// LRange 获取列表区间
func (m *CacheManager) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return m.cache.LRange(ctx, key, start, stop)
}

// LLen 列表长度
func (m *CacheManager) LLen(ctx context.Context, key string) (int64, error) {
	return m.cache.LLen(ctx, key)
}

// Delete 删除缓存
func (m *CacheManager) Delete(ctx context.Context, key string) error {
	return m.cache.Delete(ctx, key)
}

// Exists 检查缓存是否存在
func (m *CacheManager) Exists(ctx context.Context, key string) (bool, error) {
	return m.cache.Exists(ctx, key)
}

// GetJSON 获取 JSON 缓存并反序列化
func (m *CacheManager) GetJSON(ctx context.Context, key string, dest any) error {
	return m.cache.GetJSON(ctx, key, dest)
}

// SetJSON 设置 JSON 缓存
func (m *CacheManager) SetJSON(ctx context.Context, key string, value any, expiration time.Duration) error {
	if expiration == 0 {
		expiration = m.getDefaultTTL()
	}
	return m.cache.SetJSON(ctx, key, value, expiration)
}

// Clear 清空缓存
func (m *CacheManager) Clear(ctx context.Context) error {
	return m.cache.Clear(ctx)
}

// getDefaultTTL 获取默认 TTL
func (m *CacheManager) getDefaultTTL() time.Duration {
	// 默认 30 分钟
	return 30 * time.Minute
}

// Close 关闭缓存连接
func (m *CacheManager) Close() error {
	if redisCache, ok := m.cache.(*RedisCache); ok {
		return redisCache.Close()
	}
	return nil
}
