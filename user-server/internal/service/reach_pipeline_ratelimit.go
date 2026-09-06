package service

// reach_pipeline_ratelimit.go 频控与配额：令牌桶（进程内）、每日配额（Redis 优先、
// 进程内降级）、单用户频控、全局跨流水线单用户每日上限及配额运维接口。

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

type rateBucket struct {
	mu       sync.Mutex
	tokens   float64
	lastFill time.Time
	burst    int
	qps      int
}

func (b *rateBucket) allow(ctx context.Context) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens = math.Min(float64(b.burst), b.tokens+elapsed*float64(b.qps))
	b.lastFill = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

type dailyCounter struct {
	date  string
	count int
}

func (s *ReachPipelineService) checkRateLimit(ctx context.Context, channel, accountID, customerID string, rl *RateLimitConfig, transactional bool) bool {
	if rl.DailyQuota > 0 {
		if !s.checkDailyQuota(ctx, channel, rl.DailyQuota) {
			return false
		}
	}

	if !s.checkGlobalPerUserDaily(ctx, customerID, transactional) {
		return false
	}
	perUserLimit := s.resolvePerUserLimit(ctx, rl.PerUserLimit)
	if perUserLimit > 0 && customerID != "" && !transactional {
		if !s.checkPerUser(ctx, customerID, perUserLimit, time.Duration(rl.CooldownSecs)*time.Second) {
			return false
		}
	}
	if rl.QPS > 0 || rl.Burst > 0 {
		key := channel + ":" + accountID
		s.rateMu.Lock()
		b, ok := s.rateState[key]
		if !ok {
			b = &rateBucket{
				tokens:   float64(rl.Burst),
				lastFill: time.Now(),
				burst:    rl.Burst,
				qps:      rl.QPS,
			}
			s.rateState[key] = b
		}
		if rl.Burst > 0 && b.burst != rl.Burst {
			b.burst = rl.Burst
		}
		if rl.QPS > 0 && b.qps != rl.QPS {
			b.qps = rl.QPS
		}
		s.rateMu.Unlock()
		if !b.allow(ctx) {
			return false
		}
	}
	return true
}

// SetRateCache 注入跨实例共享的频控/配额缓存后端（R-5/R-6，通常为 Redis 实现）
func (s *ReachPipelineService) SetRateCache(c cache.Cache) {
	s.rateCache = c
}

func (s *ReachPipelineService) rateCacheOrGlobal() cache.Cache {
	if s.rateCache != nil {
		return s.rateCache
	}
	if cache.GlobalIsRedis() {
		return cache.GetGlobalCache()
	}
	return nil
}

func (s *ReachPipelineService) warnRedisDegraded(component string, err error) {
	if _, loaded := s.redisDegradedWarned.LoadOrStore(component, true); loaded {
		return
	}
	logger.Warnf("[R-5/R-6] Redis 不可用，%s 降级为进程内计数（多实例配额语义失效）: %v", component, err)
}

func isTransactionalPayload(job *model.ReachJob) bool {
	if job == nil || job.Payload == nil {
		return false
	}
	v, ok := job.Payload["transactional"]
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	default:
		return false
	}
}

const (
	reachPerUserLimitConfigKey = "reach_per_user_limit"

	defaultPerUserLimit  = 3
	perUserLimitCacheTTL = time.Minute
)

func (s *ReachPipelineService) resolvePerUserLimit(ctx context.Context, configured int) int {
	if configured > 0 {
		return configured
	}
	s.perUserLimitMu.Lock()
	defer s.perUserLimitMu.Unlock()
	if s.perUserLimitVal > 0 && time.Since(s.perUserLimitLoadAt) < perUserLimitCacheTTL {
		return s.perUserLimitVal
	}
	val := defaultPerUserLimit
	s.kvRepoOnce.Do(func() { s.kvRepo = repository.NewSystemConfigKVRepository() })
	if s.kvRepo != nil {

		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Debugf("[R-5] 读取 %s 时全局 DB 不可用（使用默认值 %d）", reachPerUserLimitConfigKey, defaultPerUserLimit)
				}
			}()
			if raw, err := s.kvRepo.Get(ctx, reachPerUserLimitConfigKey); err == nil && raw != "" {
				if n, perr := strconv.Atoi(strings.TrimSpace(raw)); perr == nil && n >= 0 {
					val = n
				}
			} else if err != nil {
				logger.Debugf("[R-5] 读取 %s 失败（使用默认值 %d）: %v", reachPerUserLimitConfigKey, defaultPerUserLimit, err)
			}
		}()
	}
	s.perUserLimitVal = val
	s.perUserLimitLoadAt = time.Now()
	return val
}

func dailyQuotaRedisKey(channel, day string) string {
	return fmt.Sprintf("reach:dailyquota:%s:%s", channel, day)
}

func nextCSTMidnight(t time.Time) time.Duration {
	local := t.In(cstZone)
	mid := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, cstZone).AddDate(0, 0, 1)
	return mid.Sub(local)
}

// ConsumeDailyQuota 手动消耗每日配额
func (s *ReachPipelineService) ConsumeDailyQuota(ctx context.Context, channel string) bool {
	return s.consumeDailyQuota(ctx, channel, 1)
}

func (s *ReachPipelineService) checkDailyQuota(ctx context.Context, channel string, quota int) bool {
	today := time.Now().In(cstZone).Format("2006-01-02")
	key := dailyQuotaRedisKey(channel, today)
	if c := s.rateCacheOrGlobal(); c != nil {
		n, err := c.Incr(ctx, key, nextCSTMidnight(time.Now()))
		if err != nil {
			s.warnRedisDegraded("daily_quota", err)
		} else {
			return quota <= 0 || n <= int64(quota)
		}
	}

	s.dailyQuotaMu.Lock()
	defer s.dailyQuotaMu.Unlock()
	if s.dailyQuota == nil {
		s.dailyQuota = make(map[string]*dailyCounter)
	}
	dq, ok := s.dailyQuota[key]
	if !ok || dq.date != today {
		s.dailyQuota[key] = &dailyCounter{date: today, count: 0}
		dq = s.dailyQuota[key]
	}
	if dq.count >= quota {
		return false
	}
	dq.count++
	return true
}

func (s *ReachPipelineService) consumeDailyQuota(ctx context.Context, channel string, n int) bool {
	today := time.Now().In(cstZone).Format("2006-01-02")
	key := dailyQuotaRedisKey(channel, today)
	if c := s.rateCacheOrGlobal(); c != nil {
		ttl := nextCSTMidnight(time.Now())
		var lastErr error
		for i := 0; i < n; i++ {
			if _, err := c.Incr(ctx, key, ttl); err != nil {
				lastErr = err
				break
			}
			lastErr = nil
		}
		if lastErr == nil {
			return true
		}
		s.warnRedisDegraded("daily_quota", lastErr)
	}

	s.dailyQuotaMu.Lock()
	defer s.dailyQuotaMu.Unlock()
	if s.dailyQuota == nil {
		s.dailyQuota = make(map[string]*dailyCounter)
	}
	c2, ok := s.dailyQuota[key]
	if !ok || c2.date != today {
		c2 = &dailyCounter{date: today, count: 0}
		s.dailyQuota[key] = c2
	}
	c2.count += n
	return true
}

func (s *ReachPipelineService) checkPerUser(ctx context.Context, customerID string, limit int, cooldown time.Duration) bool {

	if cooldown <= 0 {
		return true
	}
	if c := s.rateCacheOrGlobal(); c != nil {
		key := "reach:peruser:" + customerID
		n, err := c.Incr(ctx, key, cooldown)
		if err == nil {
			return n <= int64(limit)
		}
		s.warnRedisDegraded("per_user_rate_limit", err)
	}

	now := time.Now()
	s.perUserMu.Lock()
	defer s.perUserMu.Unlock()
	if s.perUserHits == nil {
		s.perUserHits = make(map[string][]time.Time)
	}
	hits := s.perUserHits[customerID]
	cutoff := now.Add(-cooldown)
	newHits := hits[:0]
	for _, h := range hits {
		if h.After(cutoff) {
			newHits = append(newHits, h)
		}
	}
	if len(newHits) >= limit {
		s.perUserHits[customerID] = newHits
		return false
	}
	newHits = append(newHits, now)
	s.perUserHits[customerID] = newHits
	return true
}

// ResetRateLimit 重置限流状态（用于测试或运维）
func (s *ReachPipelineService) ResetRateLimit(ctx context.Context, channel string) {
	prefix := channel
	s.rateMu.Lock()
	for k := range s.rateState {
		if strings.HasPrefix(k, prefix) {
			delete(s.rateState, k)
		}
	}
	s.rateMu.Unlock()
	s.dailyQuotaMu.Lock()
	delete(s.dailyQuota, prefix)

	delete(s.dailyQuota, dailyQuotaRedisKey(prefix, time.Now().In(cstZone).Format("2006-01-02")))
	s.dailyQuotaMu.Unlock()

	if c := s.rateCacheOrGlobal(); c != nil {
		today := time.Now().In(cstZone).Format("2006-01-02")
		_ = c.Delete(ctx, dailyQuotaRedisKey(channel, today))
	}
}

func (s *ReachPipelineService) checkGlobalPerUserDaily(ctx context.Context, customerID string, transactional bool) bool {
	limit := 3
	if s.globalLimitFn != nil {
		limit = s.globalLimitFn(ctx)
	}
	if limit <= 0 || customerID == "" || transactional {
		return true
	}
	day := time.Now().In(cstLoc()).Format("2006-01-02")
	key := "reach:global:" + customerID + ":" + day

	if c := s.rateCacheOrGlobal(); c != nil {
		n, err := c.Incr(ctx, key, nextCSTMidnight(time.Now()))
		if err == nil {
			if n > int64(limit) {
				logger.Ctx(ctx).Warn().Str("customer", customerID).Int64("count", n).Int("limit", limit).
					Msg("[Reach] global per-user daily limit exceeded (cross-pipeline), suppressed")
				return false
			}
			return true
		}
		s.warnRedisDegraded("global_per_user", err)
	}

	s.globalMu.Lock()
	defer s.globalMu.Unlock()
	if s.globalHits == nil {
		s.globalHits = map[string]int{}
	}
	ck := key

	s.globalHits[ck]++
	return s.globalHits[ck] <= limit
}
