package ragretrieval

import (
	"context"
	"encoding/json"
	"time"

	"marketing/internal/cache"
)

// cacheEnvelope 在 Redis / 全局缓存中保存类型标签，使反序列化后能还原为具体 Go 类型，
// 从而兼容 RAG 检索热路径中对返回值的类型断言（如 cached.([]SearchResult)）。
type cacheEnvelope struct {
	Type  string          `json:"t"`
	Value json.RawMessage `json:"v"`
}

const (
	envTypeSearchResultSlice = "SearchResultSlice"
	envTypeAny               = "Any"
)

// RedisBackedCache 将注入的 CacheInterface 适配到共享的 cache.Cache
// （Redis 或内存单例）。通过 JSON 信封保留类型信息，无需改动 RAG 检索
// 热路径中的类型断言代码即可实现跨实例缓存共享。
type RedisBackedCache struct {
	backend cache.Cache
}

// NewRedisBackedCache 以共享 cache.Cache 为后端构造 RAG 检索缓存。
func NewRedisBackedCache(backend cache.Cache) CacheInterface {
	return &RedisBackedCache{backend: backend}
}

func (c *RedisBackedCache) Get(key string) (any, bool) {
	raw, err := c.backend.Get(context.Background(), key)
	if err != nil || raw == "" {
		return nil, false
	}
	var env cacheEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return nil, false
	}
	switch env.Type {
	case envTypeSearchResultSlice:
		var v []SearchResult
		if err := json.Unmarshal(env.Value, &v); err != nil {
			return nil, false
		}
		return v, true
	default:
		var v any
		if err := json.Unmarshal(env.Value, &v); err != nil {
			return nil, false
		}
		return v, true
	}
}

func (c *RedisBackedCache) Set(key string, value any, ttl time.Duration) {
	typ := envTypeAny
	if _, ok := value.([]SearchResult); ok {
		typ = envTypeSearchResultSlice
	}
	b, err := json.Marshal(value)
	if err != nil {
		return
	}
	env := cacheEnvelope{Type: typ, Value: b}
	out, err := json.Marshal(env)
	if err != nil {
		return
	}
	_ = c.backend.Set(context.Background(), key, string(out), ttl)
}

func (c *RedisBackedCache) Delete(key string) {
	_ = c.backend.Delete(context.Background(), key)
}
