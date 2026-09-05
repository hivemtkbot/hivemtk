package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/pkg/utils/logger"
)

// ===== D14-T15b：GCRA 平滑限流（redis_rate）=====
//
// 决策 D14：三层频控的 QPS 桶层替换为 go-redis/redis_rate（GCRA 漏桶变体），
// 换来平滑输出（防突刺触发渠道风控）；key 维度沿用 channel:account（单 key 语义等价替换）。
// Redis 不可用时降级进程内 MemorySendRateLimiter（与 D14 全局频控的降级语义一致），
// 降级只告警一次（degraded atomic 标志），恢复后允许重新探测。

// RedisGCRARateLimiter 基于 redis_rate 的 GCRA 实现。
type RedisGCRARateLimiter struct {
	client  *redis.Client
	limiter *redis_rate.Limiter

	// fallback：Redis 出错时退化的进程内限流器（懒创建，与生产语义对齐）
	fallback   SendRateLimiter
	fallOnce   *sync.Once
	degraded   atomic.Bool
	lastErrAt  atomic.Int64 // unix nano；retryAfter 内不重复探测降级恢复
	retryAfter time.Duration
}

// NewRedisGCRARateLimiter 构造。client 为 nil 时 panic（调用方保证）；
// fallback 未传则使用 NewMemorySendRateLimiter。
func NewRedisGCRARateLimiter(client *redis.Client, fallback SendRateLimiter) *RedisGCRARateLimiter {
	if fallback == nil {
		fallback = NewMemorySendRateLimiter()
	}
	return &RedisGCRARateLimiter{
		client:     client,
		limiter:    redis_rate.NewLimiter(client),
		fallback:   fallback,
		fallOnce:   &sync.Once{},
		retryAfter: 30 * time.Second,
	}
}

// Allow 实现 SendRateLimiter。GCRA：突发 burst 一次性放行后按 1/qps 平滑补充。
// redis_rate 不支持"多 key 混合 QPS+DailyQuota"，日配额层维持自研（D14 决策 ②），
// 本层只处理 QPS/Burst。
func (l *RedisGCRARateLimiter) Allow(ctx context.Context, key string, limit RateLimitSpec) bool {
	if limit.QPS <= 0 && limit.Burst <= 0 {
		return true
	}
	// 降级态：直接走进程内（每 retryAfter 重试一次 Redis 探测恢复）
	if l.degraded.Load() {
		if time.Now().UnixNano()-l.lastErrAt.Load() < int64(l.retryAfter) {
			return l.fallback.Allow(ctx, key, limit)
		}
		if err := l.client.Ping(ctx).Err(); err != nil {
			l.lastErrAt.Store(time.Now().UnixNano())
			return l.fallback.Allow(ctx, key, limit)
		}
		l.degraded.Store(false)
		logger.Infof("[Reach] GCRA Redis 恢复，退出降级（进程内限流→GCRA）")
	}

	res, err := l.limiter.AllowN(ctx, key,
		redis_rate.Limit{Rate: limit.QPS, Burst: limit.Burst, Period: time.Second}, 1)
	if err != nil {
		l.enterDegraded(ctx, err)
		return l.fallback.Allow(ctx, key, limit)
	}
	return res.Allowed > 0
}

// Reset 清除指定 key 的 GCRA 状态（对应 MemorySendRateLimiter.Reset）。
// redis_rate 内部 key 前缀为 "rate:"（v10 redisPrefix）。
func (l *RedisGCRARateLimiter) Reset(ctx context.Context, key string) {
	_ = l.client.Del(ctx, "rate:"+key).Err()
}

func (l *RedisGCRARateLimiter) enterDegraded(ctx context.Context, err error) {
	if l.degraded.CompareAndSwap(false, true) {
		logger.Ctx(ctx).Warn().Err(err).
			Msg("[Reach] GCRA Redis 不可用，降级进程内 QPS 限流（每 30s 探测恢复）")
	}
	l.lastErrAt.Store(time.Now().UnixNano())
}

// Compile-time 断言
var _ SendRateLimiter = (*RedisGCRARateLimiter)(nil)

// NewGCRARateLimiterFromGlobalCache 从全局缓存单例解析底层 redis.Client 构造 GCRA 限流器；
// 全局缓存未初始化为 Redis（GlobalIsRedis()==false）时返回 nil，调用方回退 MemorySendRateLimiter。
// 复用 T15 预留的 RedisCache.Client() 通道（redis.go:194）。
func NewGCRARateLimiterFromGlobalCache() *RedisGCRARateLimiter {
	if !cache.GlobalIsRedis() {
		return nil
	}
	c := cache.GetGlobalCache()
	rc, ok := c.(*cache.RedisCache)
	if !ok {
		return nil
	}
	client := rc.Client()
	if client == nil {
		return nil
	}
	return NewRedisGCRARateLimiter(client, nil)
}
