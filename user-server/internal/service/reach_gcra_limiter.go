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

// RedisGCRARateLimiter 基于 redis_rate 的 GCRA 实现。
type RedisGCRARateLimiter struct {
	client  *redis.Client
	limiter *redis_rate.Limiter

	fallback   SendRateLimiter
	fallOnce   *sync.Once
	degraded   atomic.Bool
	lastErrAt  atomic.Int64
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

var _ SendRateLimiter = (*RedisGCRARateLimiter)(nil)

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
