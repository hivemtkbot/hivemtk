package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	// 每秒请求数（令牌补充速率）
	RPS float64
	// 桶容量（最大突发请求数）
	BucketSize int
	// 是否启用
	Enabled bool
}

// DefaultRateLimitConfig 默认限流配置
var DefaultRateLimitConfig = RateLimitConfig{
	RPS:        10,   // 每秒 10 个请求
	BucketSize: 100,  // 最大突发 100 个请求
	Enabled:    true, // 默认启用
}

// ClientLimiter 客户端限流器
type ClientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter 全局限流器
type RateLimiter struct {
	mu       sync.RWMutex
	clients  map[string]*ClientLimiter
	config   RateLimitConfig
	cleanup  time.Duration
	stopChan chan struct{}
}

// 全局限流器实例
var globalRateLimiter *RateLimiter

// InitRateLimiter 初始化全局限流器
func InitRateLimiter(config RateLimitConfig) {
	if config.RPS <= 0 {
		config.RPS = DefaultRateLimitConfig.RPS
	}
	if config.BucketSize <= 0 {
		config.BucketSize = DefaultRateLimitConfig.BucketSize
	}

	globalRateLimiter = &RateLimiter{
		clients:  make(map[string]*ClientLimiter),
		config:   config,
		cleanup:  5 * time.Minute,
		stopChan: make(chan struct{}),
	}

	// 启动定期清理 goroutine
	go globalRateLimiter.cleanupLoop()
}

// cleanupLoop 定期清理不活跃的客户端限流器
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanupClients()
		case <-rl.stopChan:
			return
		}
	}
}

// cleanupClients 清理超过 30 分钟未使用的客户端
func (rl *RateLimiter) cleanupClients() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, client := range rl.clients {
		if now.Sub(client.lastSeen) > 30*time.Minute {
			delete(rl.clients, ip)
		}
	}
}

// getLimiter 获取或创建客户端限流器
func (rl *RateLimiter) getLimiter(clientKey string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if client, exists := rl.clients[clientKey]; exists {
		client.lastSeen = time.Now()
		return client.limiter
	}

	// 创建新的限流器
	limiter := rate.NewLimiter(rate.Limit(rl.config.RPS), rl.config.BucketSize)
	rl.clients[clientKey] = &ClientLimiter{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

// Stop 停止限流器
func (rl *RateLimiter) Stop() {
	close(rl.stopChan)
}

// RateLimitMiddleware 限流中间件
// 基于 IP 地址进行限流，防止 DDoS 和暴力请求
func RateLimitMiddleware(config ...RateLimitConfig) gin.HandlerFunc {
	cfg := DefaultRateLimitConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	// 如果未启用，直接跳过
	if !cfg.Enabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	// 初始化全局限流器
	InitRateLimiter(cfg)

	return func(c *gin.Context) {
		// 获取客户端标识（优先使用 API Key，其次使用 IP）
		clientKey := c.GetHeader("X-API-KEY")
		if clientKey == "" {
			clientKey = c.ClientIP()
		}

		// 获取限流器
		limiter := globalRateLimiter.getLimiter(clientKey)

		// 检查是否允许请求
		if !limiter.Allow() {
			c.JSON(429, gin.H{
				"code":        429,
				"msg":         "请求过于频繁，请稍后再试",
				"retry_after": int(limiter.Reserve().Delay().Seconds()),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// GetRateLimitStatus 获取限流状态（用于监控）
// 返回值：
//   - remaining: 剩余可用令牌数（向下取整）。-1 表示限流器未初始化。
//   - resetAfter: 桶填满所需秒数（0 表示已满或限流器未初始化）。
func GetRateLimitStatus(clientKey string) (remaining int, resetAfter float64) {
	if globalRateLimiter == nil {
		return -1, 0
	}

	limiter := globalRateLimiter.getLimiter(clientKey)

	// 当前可用令牌数（浮点，向下取整为剩余请求数）
	tokens := limiter.Tokens()
	remaining = int(tokens)
	if remaining < 0 {
		remaining = 0
	}

	// 桶填满所需时间：(Burst - tokens) / RPS
	burst := float64(limiter.Burst())
	limit := float64(limiter.Limit())
	if limit <= 0 {
		// 限速为 0 视为永不补充，resetAfter 无意义
		return remaining, 0
	}
	deficit := burst - tokens
	if deficit <= 0 {
		return remaining, 0
	}
	resetAfter = deficit / limit

	return remaining, resetAfter
}
