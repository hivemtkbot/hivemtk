package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// VisitorRateLimitConfig 访客限流配置
type VisitorRateLimitConfig struct {
	// 每 IP 每秒允许的请求数
	PerIPRPS float64
	// 每 IP 桶容量（最大突发）
	PerIPBucket int
	// 每 app_key 每秒允许的请求数
	PerChannelRPS float64
	// 每 app_key 桶容量
	PerChannelBucket int
	// 是否启用
	Enabled bool
}

// DefaultVisitorRateLimitConfig 默认访客限流
// 策略：单 IP 20 RPS（防爬虫），单 channel 100 RPS（防脚本滥用）
var DefaultVisitorRateLimitConfig = VisitorRateLimitConfig{
	PerIPRPS:         20,
	PerIPBucket:      40,
	PerChannelRPS:    100,
	PerChannelBucket: 200,
	Enabled:          true,
}

// visitorLimiterEntry 限流器条目
type visitorLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// visitorRateLimiter 访客限流器（双维度：IP + channel）
type visitorRateLimiter struct {
	mu              sync.RWMutex
	ipLimiters      map[string]*visitorLimiterEntry
	channelLimiters map[string]*visitorLimiterEntry
	config          VisitorRateLimitConfig
	stopChan        chan struct{}
}

var globalVisitorLimiter *visitorRateLimiter

// InitVisitorRateLimiter 初始化访客限流器
func InitVisitorRateLimiter(config VisitorRateLimitConfig) {
	if config.PerIPRPS <= 0 {
		config.PerIPRPS = DefaultVisitorRateLimitConfig.PerIPRPS
	}
	if config.PerIPBucket <= 0 {
		config.PerIPBucket = DefaultVisitorRateLimitConfig.PerIPBucket
	}
	if config.PerChannelRPS <= 0 {
		config.PerChannelRPS = DefaultVisitorRateLimitConfig.PerChannelRPS
	}
	if config.PerChannelBucket <= 0 {
		config.PerChannelBucket = DefaultVisitorRateLimitConfig.PerChannelBucket
	}

	globalVisitorLimiter = &visitorRateLimiter{
		ipLimiters:      make(map[string]*visitorLimiterEntry),
		channelLimiters: make(map[string]*visitorLimiterEntry),
		config:          config,
		stopChan:        make(chan struct{}),
	}
	go globalVisitorLimiter.cleanupLoop()
}

func (rl *visitorRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopChan:
			return
		}
	}
}

func (rl *visitorRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	for k, v := range rl.ipLimiters {
		if now.Sub(v.lastSeen) > 30*time.Minute {
			delete(rl.ipLimiters, k)
		}
	}
	for k, v := range rl.channelLimiters {
		if now.Sub(v.lastSeen) > 30*time.Minute {
			delete(rl.channelLimiters, k)
		}
	}
}

func (rl *visitorRateLimiter) getIPLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if e, ok := rl.ipLimiters[ip]; ok {
		e.lastSeen = time.Now()
		return e.limiter
	}
	limiter := rate.NewLimiter(rate.Limit(rl.config.PerIPRPS), rl.config.PerIPBucket)
	rl.ipLimiters[ip] = &visitorLimiterEntry{limiter: limiter, lastSeen: time.Now()}
	return limiter
}

func (rl *visitorRateLimiter) getChannelLimiter(channelID string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if e, ok := rl.channelLimiters[channelID]; ok {
		e.lastSeen = time.Now()
		return e.limiter
	}
	limiter := rate.NewLimiter(rate.Limit(rl.config.PerChannelRPS), rl.config.PerChannelBucket)
	rl.channelLimiters[channelID] = &visitorLimiterEntry{limiter: limiter, lastSeen: time.Now()}
	return limiter
}

// VisitorRateLimitMiddleware 访客限流中间件
//
// 双维度：按 IP 限流 + 按 channel 限流
// 仅在公开 chat API 路由组使用
func VisitorRateLimitMiddleware(config ...VisitorRateLimitConfig) gin.HandlerFunc {
	cfg := DefaultVisitorRateLimitConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	if !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	InitVisitorRateLimiter(cfg)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		ipLimiter := globalVisitorLimiter.getIPLimiter(ip)
		if !ipLimiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "IP 请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		// channel 限流
		if ch, ok := c.Get("chat_channel_id"); ok {
			if channelID, ok2 := ch.(string); ok2 && channelID != "" {
				chLimiter := globalVisitorLimiter.getChannelLimiter(channelID)
				if !chLimiter.Allow() {
					c.JSON(http.StatusTooManyRequests, gin.H{
						"code":    429,
						"message": "渠道请求过于频繁，请稍后再试",
					})
					c.Abort()
					return
				}
			}
		}

		c.Next()
	}
}
