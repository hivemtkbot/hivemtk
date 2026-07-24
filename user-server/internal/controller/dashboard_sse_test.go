package controller

// dashboard_sse_test.go 实时驾驶舱 SSE 单元测试
//
// 覆盖范围：
//   - roundTo 浮点保留
//   - 缓存机制（cacheTTL 内不重复查询）
//   - Snapshot 输出结构完整性
//   - 离线场景（db=nil）安全降级
//   - EventStream 响应头设置
//   - 订阅者计数

import (
	"bufio"
	"context"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"marketing/internal/service"
)

func TestRoundTo(t *testing.T) {
	cases := []struct {
		input    float64
		n        int
		expected float64
	}{
		{1.234567, 2, 1.23},
		{1.235567, 2, 1.24},
		{0.0, 3, 0.0},
		{math.NaN(), 2, 0.0},
		{math.Inf(1), 2, 0.0},
		{0.9999, 3, 1.0},
	}
	for _, c := range cases {
		got := roundTo(c.input, c.n)
		if math.Abs(got-c.expected) > 0.001 {
			t.Errorf("roundTo(%v, %d) = %v, want %v", c.input, c.n, got, c.expected)
		}
	}
}

func TestNewDashboardSSEController_Defaults(t *testing.T) {
	c := NewDashboardSSEController(nil)
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
	if c.cacheTTL != 2*time.Second {
		t.Errorf("expected cacheTTL 2s, got %v", c.cacheTTL)
	}
	if c.subscriberCount.Load() != 0 {
		t.Errorf("expected 0 subscribers, got %d", c.subscriberCount.Load())
	}
}

func TestCollectSnapshot_NoDB(t *testing.T) {
	c := NewDashboardSSEController(nil)
	snap := c.collectSnapshot(context.Background())
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.OnlineSessions != 0 {
		t.Errorf("expected 0 online sessions, got %d", snap.OnlineSessions)
	}
	if snap.Funnel == nil {
		t.Error("expected non-nil Funnel")
	}
	if snap.HumanizeDistribution.WindowHours != 1 {
		t.Errorf("expected 1h window, got %d", snap.HumanizeDistribution.WindowHours)
	}
	if snap.LLMMetrics == nil {
		t.Error("expected non-nil LLMMetrics")
	}
}

func TestCollectSnapshot_CacheHit(t *testing.T) {
	c := NewDashboardSSEController(nil)
	c.cacheTTL = 1 * time.Hour
	snap1 := c.collectSnapshot(context.Background())
	c.cacheMu.Lock()
	c.lastUpdateAt = time.Now()
	c.lastSnapshot.GeneratedAt = time.Now().Add(-1 * time.Minute)
	c.cacheMu.Unlock()
	snap2 := c.collectSnapshot(context.Background())
	if snap1 != snap2 {
		t.Error("expected cache hit (same pointer)")
	}
}

func TestCollectSnapshot_CacheMiss(t *testing.T) {
	c := NewDashboardSSEController(nil)
	c.cacheTTL = 1 * time.Millisecond
	snap1 := c.collectSnapshot(context.Background())
	time.Sleep(2 * time.Millisecond)
	snap2 := c.collectSnapshot(context.Background())
	if snap1 == snap2 {
		t.Error("expected cache miss (new pointer)")
	}
}

func TestCollectFunnel_NoDB(t *testing.T) {
	// collectFunnel 已下沉到 service.DashboardStatsService.CollectFunnel，
	// controller 离线模式下通过 collectSnapshot 返回空 FunnelProgress。
	// 这里通过 collectSnapshot 验证离线场景的 Funnel 默认值。
	c := NewDashboardSSEController(nil)
	snap := c.collectSnapshot(context.Background())
	if snap.Funnel == nil {
		t.Fatal("expected non-nil funnel")
	}
	if len(snap.Funnel.Stages) != 0 {
		t.Errorf("expected empty stages when db=nil, got %d", len(snap.Funnel.Stages))
	}
}

func TestCollectHumanizeDistribution_NoDB(t *testing.T) {
	// collectHumanizeDistribution 已下沉到 service.DashboardStatsService.CollectHumanizeDistribution，
	// controller 离线模式下通过 collectSnapshot 返回默认 HumanizeDistribution。
	// 这里通过 collectSnapshot 验证离线场景的分布默认值。
	c := NewDashboardSSEController(nil)
	snap := c.collectSnapshot(context.Background())
	if snap.HumanizeDistribution.WindowHours != 1 {
		t.Errorf("expected 1h window, got %d", snap.HumanizeDistribution.WindowHours)
	}
	if snap.HumanizeDistribution.TotalScored != 0 {
		t.Errorf("expected 0 total, got %d", snap.HumanizeDistribution.TotalScored)
	}
}

func TestCollectLLMMetrics_NoDB(t *testing.T) {
	c := NewDashboardSSEController(nil)
	m := c.collectLLMMetrics(context.Background())
	if m == nil {
		t.Fatal("expected non-nil LLM metrics")
	}
}

func TestSnapshot_HTTPEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewDashboardSSEController(nil)
	r := gin.New()
	r.GET("/snapshot", c.Snapshot)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/snapshot", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Code    string                     `json:"code"`
		Data    *service.DashboardSnapshot `json:"data"`
		Message string                     `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data == nil {
		t.Fatal("expected non-nil data")
	}
	if resp.Code == "" {
		t.Error("expected non-empty code")
	}
}

func TestMetrics_HTTPEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewDashboardSSEController(nil)
	r := gin.New()
	r.GET("/metrics", c.Metrics)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestEventStream_HeadersAndLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewDashboardSSEController(nil)
	r := gin.New()
	r.GET("/stream", c.StreamEventStream)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	req = req.WithContext(ctx)
	r.ServeHTTP(w, req)

	headers := w.Header()
	if got := headers.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", got)
	}
	if got := headers.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %s", got)
	}
	if got := headers.Get("Connection"); got != "keep-alive" {
		t.Errorf("expected Connection keep-alive, got %s", got)
	}
	if got := headers.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("expected X-Accel-Buffering no, got %s", got)
	}

	body := w.Body.String()
	if !contains(body, "event:") {
		t.Error("expected 'event:' in SSE body")
	}
	if !contains(body, "data:") {
		t.Error("expected 'data:' in SSE body")
	}
}

func TestEventStream_ConcurrentSubscribers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewDashboardSSEController(nil)
	r := gin.New()
	r.GET("/stream", c.StreamEventStream)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/stream", nil)
			req = req.WithContext(ctx)
			r.ServeHTTP(w, req)
		}()
	}
	wg.Wait()
	if got := c.subscriberCount.Load(); got != 0 {
		t.Errorf("expected 0 subscribers after teardown, got %d", got)
	}
}

func TestWriteDashboardEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// httptest.ResponseRecorder 已实现 gin.ResponseWriter 大部分方法
	// 我们用包装器补齐 CloseNotify / Hijack / Flush
	c.Writer = &ginTestResponseWriter{ResponseRecorder: w, closeNotify: make(chan bool)}

	err := writeDashboardEvent(c, "test_event", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	body := w.Body.String()
	if !contains(body, "event: test_event") {
		t.Errorf("expected 'event: test_event' in body, got %s", body)
	}
	if !contains(body, `"key":"value"`) {
		t.Errorf("expected JSON data in body, got %s", body)
	}
}

// ginTestResponseWriter 包装 httptest.ResponseRecorder，补齐 gin.ResponseWriter 缺失的方法
type ginTestResponseWriter struct {
	*httptest.ResponseRecorder
	closeNotify chan bool
}

func (g *ginTestResponseWriter) CloseNotify() <-chan bool {
	return g.closeNotify
}

func (g *ginTestResponseWriter) Flush() {
	// httptest.ResponseRecorder 已实现 Flush
}

func (g *ginTestResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	// 测试环境不实现真实 hijack
	return nil, nil, nil
}

func (g *ginTestResponseWriter) Pusher() http.Pusher {
	// 测试环境不实现 http2 push
	return nil
}

func (g *ginTestResponseWriter) Size() int {
	return g.ResponseRecorder.Body.Len()
}

func (g *ginTestResponseWriter) Status() int {
	return g.ResponseRecorder.Code
}

func (g *ginTestResponseWriter) WriteString(s string) (int, error) {
	return g.ResponseRecorder.Body.WriteString(s)
}

func (g *ginTestResponseWriter) Written() bool {
	return g.ResponseRecorder.Code != http.StatusOK || g.ResponseRecorder.Body.Len() > 0
}

func (g *ginTestResponseWriter) WriteHeaderNow() {
	// 立即写入响应头（httptest.ResponseRecorder 已在 WriteHeader 时完成）
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
