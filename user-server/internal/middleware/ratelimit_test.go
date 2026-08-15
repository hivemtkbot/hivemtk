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

	originalConfig := DefaultRateLimitConfig
	defer func() {
		DefaultRateLimitConfig = originalConfig
	}()

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

	DefaultRateLimitConfig = RateLimitConfig{
		RPS:        0.5, 
		BucketSize: 1,   
		Enabled:    true,
	}

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

	InitRateLimiter(DefaultRateLimitConfig)

	remaining, resetAfter := GetRateLimitStatus("test-client")

	if remaining < 0 || remaining > DefaultRateLimitConfig.BucketSize {
		t.Errorf("Expected remaining in [0, %d], got %d", DefaultRateLimitConfig.BucketSize, remaining)
	}
	if remaining != DefaultRateLimitConfig.BucketSize {
		t.Errorf("Expected remaining = %d (full bucket) for new client, got %d",
			DefaultRateLimitConfig.BucketSize, remaining)
	}

	if resetAfter < 0 {
		t.Errorf("Expected resetAfter >= 0, got %f", resetAfter)
	}

	limiter := globalRateLimiter.getLimiter("test-client")
	for i := 0; i < 30; i++ {
		limiter.Allow()
	}
	remaining2, _ := GetRateLimitStatus("test-client")
	if remaining2 >= remaining {
		t.Errorf("After consuming tokens, expected remaining < %d, got %d", remaining, remaining2)
	}

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

	rl.Stop()

	select {
	case <-rl.stopChan:
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

	oldTime := time.Now().Add(-31 * time.Minute)
	rl.clients["old-client"] = &ClientLimiter{
		limiter:  nil,
		lastSeen: oldTime,
	}

	rl.clients["new-client"] = &ClientLimiter{
		limiter:  nil,
		lastSeen: time.Now(),
	}

	rl.cleanupClients()

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

	limiter1 := rl.getLimiter("client-1")
	if limiter1 == nil {
		t.Fatal("Expected non-nil limiter")
	}

	limiter2 := rl.getLimiter("client-1")
	if limiter2 != limiter1 {
		t.Error("Expected same limiter instance")
	}

	limiter3 := rl.getLimiter("client-2")
	if limiter3 == limiter1 {
		t.Error("Expected different limiter instance for different client")
	}
}

func TestInitRateLimiter_DefaultConfig(t *testing.T) {
	InitRateLimiter(RateLimitConfig{
		RPS:        0,
		BucketSize: 0,
		Enabled:    true,
	})

	if globalRateLimiter == nil {
		t.Fatal("Expected globalRateLimiter to be initialized")
	}

	if globalRateLimiter.config.RPS <= 0 {
		t.Errorf("Expected RPS > 0, got %f", globalRateLimiter.config.RPS)
	}
	if globalRateLimiter.config.BucketSize <= 0 {
		t.Errorf("Expected BucketSize > 0, got %d", globalRateLimiter.config.BucketSize)
	}

	globalRateLimiter.Stop()
}

