package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimitMiddleware_Enabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 保存原始配置
	originalConfig := DefaultRateLimitConfig
	defer func() {
		DefaultRateLimitConfig = originalConfig
	}()

	// 设置低限流以便测试
	DefaultRateLimitConfig = RateLimitConfig{
		RPS:        1,
		BucketSize: 2,
		Enabled:    true,
	}

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	req, _ := http.NewRequest("GET", "http://test.com/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	ctx.Request = req

	middleware := RateLimitMiddleware()
	middleware(ctx)

	// 第一个请求应该成功
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_Disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	req, _ := http.NewRequest("GET", "http://test.com/test", nil)
	ctx.Request = req

	config := RateLimitConfig{
		RPS:        1,
		BucketSize: 1,
		Enabled:    false,
	}

	middleware := RateLimitMiddleware(config)
	middleware(ctx)

	// 禁用时应该直接放行
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_WithAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalConfig := DefaultRateLimitConfig
	defer func() {
		DefaultRateLimitConfig = originalConfig
	}()

	DefaultRateLimitConfig = RateLimitConfig{
		RPS:        10,
		BucketSize: 100,
		Enabled:    true,
	}

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	req, _ := http.NewRequest("GET", "http://test.com/test", nil)
	req.Header.Set("X-API-KEY", "test-api-key-12345")
	ctx.Request = req

	middleware := RateLimitMiddleware()
	middleware(ctx)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_RateLimitExceeded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalConfig := DefaultRateLimitConfig
	defer func() {
		DefaultRateLimitConfig = originalConfig
	}()

	// 设置非常小的限流以便快速测试
	DefaultRateLimitConfig = RateLimitConfig{
		RPS:        0.5, // 每 2 秒 1 个请求
		BucketSize: 1,   // 最大突发 1 个请求
		Enabled:    true,
	}

	// 第一次请求
	w1 := httptest.NewRecorder()
	ctx1, _ := gin.CreateTestContext(w1)
	req1, _ := http.NewRequest("GET", "http://test.com/test", nil)
	req1.Header.Set("X-Forwarded-For", "10.0.0.1")
	ctx1.Request = req1

	middleware := RateLimitMiddleware()
	middleware(ctx1)

	if w1.Code != http.StatusOK {
		t.Errorf("First request: Expected status 200, got %d", w1.Code)
	}

	// 立即第二次请求，应该被限流
	w2 := httptest.NewRecorder()
	ctx2, _ := gin.CreateTestContext(w2)
	req2, _ := http.NewRequest("GET", "http://test.com/test", nil)
	req2.Header.Set("X-Forwarded-For", "10.0.0.1")
	ctx2.Request = req2

	middleware(ctx2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("Second request: Expected status 429, got %d", w2.Code)
	}
}

func TestGetRateLimitStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalConfig := DefaultRateLimitConfig
	defer func() {
		DefaultRateLimitConfig = originalConfig
	}()

	DefaultRateLimitConfig = RateLimitConfig{
		RPS:        10,
		BucketSize: 100,
		Enabled:    true,
	}

	// 初始化限流器
	InitRateLimiter(DefaultRateLimitConfig)

	// 获取限流状态
	remaining, resetAfter := GetRateLimitStatus("test-client")

	// remaining 应为实际可用令牌数（0 ~ BucketSize 之间）
	// 新创建的限流器初始即为满桶，因此 remaining 应等于 BucketSize
	if remaining < 0 || remaining > DefaultRateLimitConfig.BucketSize {
		t.Errorf("Expected remaining in [0, %d], got %d", DefaultRateLimitConfig.BucketSize, remaining)
	}
	// 新建客户端首次查询时应为满桶（或接近满桶）
	if remaining != DefaultRateLimitConfig.BucketSize {
		t.Errorf("Expected remaining = %d (full bucket) for new client, got %d",
			DefaultRateLimitConfig.BucketSize, remaining)
	}

	// resetAfter 应该 >= 0（满桶时为 0）
	if resetAfter < 0 {
		t.Errorf("Expected resetAfter >= 0, got %f", resetAfter)
	}

	// 消耗部分令牌后再查询，remaining 应减少
	limiter := globalRateLimiter.getLimiter("test-client")
	for i := 0; i < 30; i++ {
		limiter.Allow()
	}
	// 等待极短时间让令牌计数刷新（limiter 内部基于时间计算）
	remaining2, _ := GetRateLimitStatus("test-client")
	if remaining2 >= remaining {
		t.Errorf("After consuming tokens, expected remaining < %d, got %d", remaining, remaining2)
	}

	// 未初始化限流器时返回 -1
	originalGlobal := globalRateLimiter
	globalRateLimiter = nil
	defer func() { globalRateLimiter = originalGlobal }()
	remaining3, resetAfter3 := GetRateLimitStatus("any-client")
	if remaining3 != -1 {
		t.Errorf("Expected remaining = -1 when limiter not initialized, got %d", remaining3)
	}
	if resetAfter3 != 0 {
		t.Errorf("Expected resetAfter = 0 when limiter not initialized, got %f", resetAfter3)
	}
}

func TestRateLimiter_Stop(t *testing.T) {
	rl := &RateLimiter{
		clients:  make(map[string]*ClientLimiter),
		config:   DefaultRateLimitConfig,
		cleanup:  5 * time.Minute,
		stopChan: make(chan struct{}),
	}

	// 停止限流器
	rl.Stop()

	// 验证 stopChan 已关闭
	select {
	case <-rl.stopChan:
		// 正常关闭
	default:
		t.Error("Expected stopChan to be closed")
	}
}

func TestRateLimiter_cleanupClients(t *testing.T) {
	rl := &RateLimiter{
		clients:  make(map[string]*ClientLimiter),
		config:   DefaultRateLimitConfig,
		cleanup:  5 * time.Minute,
		stopChan: make(chan struct{}),
	}

	// 添加一个旧的客户端
	oldTime := time.Now().Add(-31 * time.Minute)
	rl.clients["old-client"] = &ClientLimiter{
		limiter:  nil,
		lastSeen: oldTime,
	}

	// 添加一个新的客户端
	rl.clients["new-client"] = &ClientLimiter{
		limiter:  nil,
		lastSeen: time.Now(),
	}

	// 清理
	rl.cleanupClients()

	// 验证旧客户端被删除，新客户端保留
	if _, exists := rl.clients["old-client"]; exists {
		t.Error("Expected old-client to be deleted")
	}
	if _, exists := rl.clients["new-client"]; !exists {
		t.Error("Expected new-client to exist")
	}
}

func TestRateLimiter_getLimiter_Cache(t *testing.T) {
	rl := &RateLimiter{
		clients:  make(map[string]*ClientLimiter),
		config:   DefaultRateLimitConfig,
		cleanup:  5 * time.Minute,
		stopChan: make(chan struct{}),
	}

	// 第一次获取，应该创建新的限流器
	limiter1 := rl.getLimiter("client-1")
	if limiter1 == nil {
		t.Fatal("Expected non-nil limiter")
	}

	// 第二次获取，应该返回同一个限流器
	limiter2 := rl.getLimiter("client-1")
	if limiter2 != limiter1 {
		t.Error("Expected same limiter instance")
	}

	// 获取不同客户端的限流器，应该是不同的实例
	limiter3 := rl.getLimiter("client-2")
	if limiter3 == limiter1 {
		t.Error("Expected different limiter instance for different client")
	}
}

func TestInitRateLimiter_DefaultConfig(t *testing.T) {
	// 使用零配置初始化
	InitRateLimiter(RateLimitConfig{
		RPS:        0,
		BucketSize: 0,
		Enabled:    true,
	})

	if globalRateLimiter == nil {
		t.Fatal("Expected globalRateLimiter to be initialized")
	}

	// 验证使用了默认配置
	if globalRateLimiter.config.RPS <= 0 {
		t.Errorf("Expected RPS > 0, got %f", globalRateLimiter.config.RPS)
	}
	if globalRateLimiter.config.BucketSize <= 0 {
		t.Errorf("Expected BucketSize > 0, got %d", globalRateLimiter.config.BucketSize)
	}

	// 停止限流器以便下一个测试
	globalRateLimiter.Stop()
}
