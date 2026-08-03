package browser

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"marketing/internal/cache"
	"marketing/internal/pkg/utils/logger"
)

// RateLimitEntry 限流条目
type RateLimitEntry struct {
	HourCount     int
	DayCount      int
	LastReply     time.Time
	HourResetTime time.Time
	DayResetTime  time.Time
}

// RateLimiter 频率控制器
// 防止触发平台风控的核心机制：
//   - 每小时回复上限
//   - 每日回复上限
//   - 回复冷却时间（最小间隔）
//   - 随机延迟抖动
//
// 多副本部署：计数状态存放在全局缓存（cache.GetGlobalCache）中。
// 配置 Redis 时由各副本共享（分布式限流），未配置 Redis 时退化为进程内缓存，行为与原先一致。
type RateLimiter struct {
	mu     sync.RWMutex // 进程内串行化；跨进程共享状态由全局缓存（Redis）保证
	config RateLimitConfig
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	HourlyLimit   int           // 每小时最大回复数，默认 20
	DailyLimit    int           // 每日最大回复数，默认 100
	CoolDownMin   time.Duration // 最小回复间隔，默认 30s
	CoolDownMax   time.Duration // 最大回复间隔，默认 90s
	JitterPercent float64       // 抖动百分比，默认 0.3 (30%)
	Enabled       bool          // 是否启用
}

// DefaultRateLimitConfig 默认限流配置（安全值）
var DefaultRateLimitConfig = RateLimitConfig{
	HourlyLimit:   20,
	DailyLimit:    100,
	CoolDownMin:   30 * time.Second,
	CoolDownMax:   90 * time.Second,
	JitterPercent: 0.3,
	Enabled:       true,
}

// NewRateLimiter 创建限流器
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		config: cfg,
	}
}

func (rl *RateLimiter) cacheKey(key string) string {
	return "reach:rl:" + key
}

// loadEntry 从全局缓存读取限流条目（Redis 共享；未配置 Redis 时退化为进程内缓存）。
func (rl *RateLimiter) loadEntry(key string) *RateLimitEntry {
	now := time.Now()
	entry := &RateLimitEntry{
		HourResetTime: now.Truncate(time.Hour).Add(time.Hour),
		DayResetTime:  now.Truncate(24 * time.Hour).Add(24 * time.Hour),
	}
	if c := cache.GetGlobalCache(); c != nil {
		_ = c.GetJSON(context.Background(), rl.cacheKey(key), entry)
	}
	return entry
}

// saveEntry 将限流条目写回全局缓存，TTL 持续到当日计数重置时刻。
func (rl *RateLimiter) saveEntry(key string, entry *RateLimitEntry) {
	c := cache.GetGlobalCache()
	if c == nil {
		return
	}
	ttl := time.Until(entry.DayResetTime)
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	_ = c.SetJSON(context.Background(), rl.cacheKey(key), entry, ttl)
}

// Allow 检查是否允许发送回复
// 返回 true 表示可以发送，false 表示需要等待
func (rl *RateLimiter) Allow(key string) (bool, string) {
	if !rl.config.Enabled {
		return true, ""
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry := rl.loadEntry(key)
	now := time.Now()

	// 检查是否需要重置计数器
	if now.After(entry.HourResetTime) {
		entry.HourCount = 0
		entry.HourResetTime = now.Truncate(time.Hour).Add(time.Hour)
	}
	if now.After(entry.DayResetTime) {
		entry.DayCount = 0
		entry.DayResetTime = now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	}

	// 检查日上限
	if entry.DayCount >= rl.config.DailyLimit {
		return false, fmt.Sprintf("日回复上限已达 (%d/%d)", entry.DayCount, rl.config.DailyLimit)
	}

	// 检查时上限
	if entry.HourCount >= rl.config.HourlyLimit {
		return false, fmt.Sprintf("小时回复上限已达 (%d/%d)", entry.HourCount, rl.config.HourlyLimit)
	}

	// 检查冷却时间
	if !entry.LastReply.IsZero() {
		elapsed := now.Sub(entry.LastReply)
		if elapsed < rl.config.CoolDownMin {
			remaining := rl.config.CoolDownMin - elapsed
			return false, fmt.Sprintf("冷却中，剩余 %.0f 秒", remaining.Seconds())
		}
	}

	return true, ""
}

// Record 记录一次回复
func (rl *RateLimiter) Record(key string) {
	if !rl.config.Enabled {
		return
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry := rl.loadEntry(key)
	now := time.Now()

	if now.After(entry.HourResetTime) {
		entry.HourCount = 0
		entry.HourResetTime = now.Truncate(time.Hour).Add(time.Hour)
	}
	if now.After(entry.DayResetTime) {
		entry.DayCount = 0
		entry.DayResetTime = now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	}

	entry.HourCount++
	entry.DayCount++
	entry.LastReply = now

	rl.saveEntry(key, entry)
}

// Wait 等待直到允许发送（阻塞）
func (rl *RateLimiter) Wait(key string) {
	for {
		allowed, reason := rl.Allow(key)
		if allowed {
			// 随机抖动
			jitter := time.Duration(float64(rl.config.CoolDownMin) * rl.config.JitterPercent * (rand.Float64()*2 - 1))
			waitTime := rl.config.CoolDownMin + jitter
			if waitTime > 0 {
				logger.Infof("[限流] %s: 等待 %.0f 秒后发送", key, waitTime.Seconds())
				time.Sleep(waitTime)
			}
			return
		}
		logger.Infof("[限流] %s: %s, 等待 10s 后重试", key, reason)
		time.Sleep(10 * time.Second)
	}
}

// Stats 获取统计信息
func (rl *RateLimiter) Stats(key string) map[string]any {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	entry := rl.loadEntry(key)

	return map[string]any{
		"hour_count":   entry.HourCount,
		"day_count":    entry.DayCount,
		"last_reply":   entry.LastReply,
		"hour_reset":   entry.HourResetTime,
		"day_reset":    entry.DayResetTime,
		"hourly_limit": rl.config.HourlyLimit,
		"daily_limit":  rl.config.DailyLimit,
	}
}

// GenerateCooldown 生成随机冷却时间
func (rl *RateLimiter) GenerateCooldown() time.Duration {
	base := rl.config.CoolDownMin
	range_ := rl.config.CoolDownMax - rl.config.CoolDownMin
	if range_ <= 0 {
		return base
	}
	return base + time.Duration(rand.Int63n(int64(range_)))
}
