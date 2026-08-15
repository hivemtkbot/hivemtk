package cache

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
)

var (
	globalMu    sync.RWMutex
	globalCache Cache
	globalRedis bool
	defaultOnce  sync.Once
	defaultCache Cache
)

// InitGlobalCache 初始化全局缓存单例。
//
// client 非 nil 时使用 Redis 后端（共享同一 *redis.Client，避免重复连接）；
// 否则回退进程内内存缓存（向后兼容单实例部署）。
// 返回最终选中的实现，供调用方持有/关闭。
//
// 必须在服务启动的业务装配（router.Setup、RAG 栈构造等）之前调用，
// 以便 message_hub 会话幂等、RAG 检索缓存等共享同一后端。
func InitGlobalCache(client *redis.Client) Cache {
	var c Cache
	if client != nil {
		c = NewRedisCacheWithClient(client)
		globalRedis = true
	} else {
		c = NewMemoryCache()
		globalRedis = false
	}
	globalMu.Lock()
	globalCache = c
	globalMu.Unlock()
	return c
}

// GetGlobalCache 获取全局缓存。
// 未初始化时返回惰性创建的「稳定」内存缓存单例（首次调用创建后复用），
// 保证同进程内读写一致，避免 nil panic 与每次新建实例导致的不一致。
func GetGlobalCache() Cache {
	globalMu.RLock()
	c := globalCache
	globalMu.RUnlock()
	if c != nil {
		return c
	}
	defaultOnce.Do(func() {
		defaultCache = NewMemoryCache()
	})
	return defaultCache
}

// GlobalIsRedis 报告全局缓存是否以 Redis 为后端。
func GlobalIsRedis() bool {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalRedis
}

// cacheCloser 可选关闭接口：内存缓存实现 Close() 停止清理协程。
// Cache 接口本身未包含 Close，故通过类型断言按需调用，避免改动接口契约。
type cacheCloser interface {
	Close()
}

// CloseGlobalCache 关闭全局缓存。Redis 后端共享 main 持有的 client，
// 此处不关闭连接（由 main 负责）；内存后端停止清理协程。
func CloseGlobalCache(_ context.Context) error {
	globalMu.RLock()
	c := globalCache
	globalMu.RUnlock()
	if c == nil {
		return nil
	}
	if cl, ok := c.(cacheCloser); ok {
		cl.Close()
	}
	return nil
}

