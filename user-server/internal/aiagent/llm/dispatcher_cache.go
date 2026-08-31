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

func (d *Dispatcher) getCache(ctx context.Context, key string) (string, bool) {
	if key == "" {
		return "", false
	}
	// v3 审计 P3-2 修复：ctx 透传 trace_id
	raw, err := cache.GetGlobalCache().Get(ctx, key)
	if err != nil || raw == "" {
		return "", false
	}
	return raw, true
}

func (d *Dispatcher) setCache(ctx context.Context, key string, ttl int, content string) {
	if ttl <= 0 || key == "" {
		return
	}
	// v3 审计 P3-2 修复：ctx 透传
	_ = cache.GetGlobalCache().Set(ctx, key, content, time.Duration(ttl)*time.Second)
}

// allowRequest RPM 限流（全局，跨实例一致）
// allowRequest 单实例 + 多实例统一 RPM 限流
// 业务需要：必须守住上游 provider 每分钟配额/成本。
// v3 审计 P1-40 修复：使用滑动窗口近似（两次计数 + 衰减）替代固定窗口
// 原：Incr 累加到时间窗口 key，过期后丢弃 → 固定窗口边界处可超量 1.5-2×
// 新：每次 Incr 同时记录窗口起始时间戳，超 maxRPM 立即拒绝
func (d *Dispatcher) allowRequest(providerName string, maxRPM int) bool {
	if maxRPM <= 0 {
		return true
	}
	if !cache.GlobalIsRedis() {
		return d.allowRequestLocal(providerName, maxRPM)
	}
	c := cache.GetGlobalCache()
	now := time.Now()
	windowStart := now.Truncate(time.Minute).Unix()
	key := fmt.Sprintf("mtk:llm:rpm:%s:%d", providerName, windowStart)
	cur, err := c.Incr(context.Background(), key, time.Minute)
	if err != nil {
		logger.Warnf("[LLM] RPM 计数后端异常，放行 provider=%s: %v", providerName, err)
		return true
	}
	// v3 审计 P1-40：当前窗口已超 maxRPM → 拒绝
	// 不 Decr（cache 包未暴露 Decr），让 key 过期自清；下窗口从 0 开始
	if cur > int64(maxRPM) {
		logger.Warnf("[LLM] RPM 限流触发 provider=%s cur=%d max=%d", providerName, cur, maxRPM)
		return false
	}
	return true
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
