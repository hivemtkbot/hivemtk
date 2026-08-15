package llm

import (
	"context"

	"fmt"

	"strings"

	"sync"

	"time"

	"hivemtk-user/internal/cache"

	"hash/fnv"
	"hivemtk-user/internal/pkg/utils/logger"
)

type rpmBucket struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

func (d *Dispatcher) getCache(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	raw, err := cache.GetGlobalCache().Get(context.Background(), key)
	if err != nil || raw == "" {
		return "", false
	}
	return raw, true
}

func (d *Dispatcher) setCache(key string, ttl int, content string) {
	if ttl <= 0 || key == "" {
		return
	}
	_ = cache.GetGlobalCache().Set(context.Background(), key, content, time.Duration(ttl)*time.Second)
}

// allowRequest RPM 限流（全局，跨实例一致）
// 业务需要：必须守住上游 provider 每分钟配额/成本。多实例下若各持独立计数，
// 全局实际允许量被放大为 N×单实例配额，可能击穿上游限流/资费。
// 实现：REDIS_HOST 配置时走全局缓存固定窗口计数（Redis 共享，各实例累计同一配额）；
// 未配置 Redis 时回退进程内计数（单实例安全、零额外开销）。后端异常一律放行（可用性优先）。
func (d *Dispatcher) allowRequest(providerName string, maxRPM int) bool {
	if maxRPM <= 0 {
		return true
	}
	if !cache.GlobalIsRedis() {
		return d.allowRequestLocal(providerName, maxRPM)
	}
	c := cache.GetGlobalCache()
	key := fmt.Sprintf("mtk:llm:rpm:%s:%d", providerName, time.Now().Truncate(time.Minute).Unix())
	cur, err := c.Incr(context.Background(), key, time.Minute)
	if err != nil {
		logger.Warnf("[LLM] RPM 计数后端异常，放行 provider=%s: %v", providerName, err)
		return true
	}
	return cur <= int64(maxRPM)
}

// allowRequestLocal 单实例 RPM 限流（未配置 Redis 时走进程内固定窗口计数）
func (d *Dispatcher) allowRequestLocal(providerName string, maxRPM int) bool {
	d.mu.Lock()
	bucket, ok := d.rpmCounter[providerName]
	if !ok {
		bucket = &rpmBucket{resetAt: time.Now().Add(time.Minute)}
		d.rpmCounter[providerName] = bucket
	}
	d.mu.Unlock()

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()
	if now.After(bucket.resetAt) {
		bucket.count = 0
		bucket.resetAt = now.Add(time.Minute)
	}
	if bucket.count >= maxRPM {
		return false
	}
	bucket.count++
	return true
}

// CacheKey 生成缓存 key
func CacheKey(scenario DispatchScenario, prompt string) string {
	h := fnv.New64a()
	h.Write([]byte(string(scenario)))
	h.Write([]byte(strings.TrimSpace(prompt)))
	return fmt.Sprintf("llm:dispatch:%s:%x", scenario, h.Sum64())
}

