package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)


// fakeChecker 模拟健康检查器
type fakeChecker struct {
	mu      sync.Mutex
	latency int64
	err     error
	calls   map[string]int
}

func newFakeChecker(latency int64, err error) *fakeChecker {
	return &fakeChecker{
		latency: latency,
		err:     err,
		calls:   make(map[string]int),
	}
}

func (f *fakeChecker) Ping(ctx context.Context, provider *ProviderConfig, config FailoverConfig) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[provider.Name]++
	return f.latency, f.err
}

// setupFailoverDB 构建测试 DB（仅 system_kv_config 表）
func setupFailoverDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t)
}

// newTestFailover 构建测试用 ProviderFailover
func newTestFailover(t *testing.T, dispatcher *Dispatcher, db *gorm.DB) *ProviderFailover {
	f := NewProviderFailover(dispatcher, db)
	return f
}


// 1. 默认配置 - 字段正确
func TestDefaultFailoverConfig(t *testing.T) {
	cfg := DefaultFailoverConfig()
	if cfg.HealthCheckInterval != 30 {
		t.Errorf("expected 30, got %d", cfg.HealthCheckInterval)
	}
	if cfg.FailureThreshold != 5 {
		t.Errorf("expected 5, got %d", cfg.FailureThreshold)
	}
	if cfg.CircuitOpenDuration != 60 {
		t.Errorf("expected 60, got %d", cfg.CircuitOpenDuration)
	}
	if cfg.LocalFallbackProvider != "default" {
		t.Errorf("expected default, got %s", cfg.LocalFallbackProvider)
	}
	if cfg.TemplateReply == "" {
		t.Error("expected non-empty template reply")
	}
}

// 2. 默认策略 - 包含所有场景
func TestDefaultFailoverPolicy(t *testing.T) {
	p := DefaultFailoverPolicy()
	if len(p.Scenarios) == 0 {
		t.Fatal("expected non-empty scenarios")
	}
	list, ok := p.Scenarios["intent_recognize"]
	if !ok || len(list) == 0 {
		t.Fatal("expected intent_recognize scenario")
	}
	if list[0] != "default" {
		t.Errorf("expected default first, got %s", list[0])
	}
}

// 3. 健康检查成功 - 状态 up
func TestHealthCheckSuccess(t *testing.T) {
	d := newTestDispatcher()
	d.AddProvider(ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true})
	f := newTestFailover(t, d, nil)
	f.SetHealthChecker(newFakeChecker(100, nil))
	f.checkOne(context.Background(), &ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true}, f.Config())
	h := f.GetHealth("p1")
	if h == nil {
		t.Fatal("expected health record")
	}
	if h.Status != ProviderStatusUp {
		t.Errorf("expected up, got %s", h.Status)
	}
	if h.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 failures, got %d", h.ConsecutiveFailures)
	}
	if h.LatencyP95Ms != 100 {
		t.Errorf("expected 100ms, got %d", h.LatencyP95Ms)
	}
}

// 4. 健康检查失败 - 累计失败次数
func TestHealthCheckFailure(t *testing.T) {
	d := newTestDispatcher()
	d.AddProvider(ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true})
	f := newTestFailover(t, d, nil)
	f.SetHealthChecker(newFakeChecker(0, errors.New("connect refused")))
	for i := 0; i < 3; i++ {
		f.checkOne(context.Background(), &ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true}, f.Config())
	}
	h := f.GetHealth("p1")
	if h.Status != ProviderStatusDegraded {
		t.Errorf("expected degraded, got %s", h.Status)
	}
	if h.ConsecutiveFailures != 3 {
		t.Errorf("expected 3 failures, got %d", h.ConsecutiveFailures)
	}
}

// 5. 连续 5 次失败 - 触发熔断
func TestCircuitOpen(t *testing.T) {
	d := newTestDispatcher()
	d.AddProvider(ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true})
	f := newTestFailover(t, d, nil)
	f.SetHealthChecker(newFakeChecker(0, errors.New("timeout")))
	for i := 0; i < 5; i++ {
		f.checkOne(context.Background(), &ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true}, f.Config())
	}
	h := f.GetHealth("p1")
	if h.Status != ProviderStatusDown {
		t.Errorf("expected down, got %s", h.Status)
	}
	if h.CircuitOpenUntil.IsZero() {
		t.Error("expected circuit open")
	}
	if !f.IsCircuitOpen("p1") {
		t.Error("expected circuit open")
	}
}

// 6. 熔断重置 - ResetCircuit
func TestResetCircuit(t *testing.T) {
	d := newTestDispatcher()
	d.AddProvider(ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true})
	f := newTestFailover(t, d, nil)
	f.SetHealthChecker(newFakeChecker(0, errors.New("timeout")))
	for i := 0; i < 5; i++ {
		f.checkOne(context.Background(), &ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true}, f.Config())
	}
	if !f.IsCircuitOpen("p1") {
		t.Fatal("expected circuit open")
	}
	if !f.ResetCircuit("p1") {
		t.Fatal("expected reset success")
	}
	if f.IsCircuitOpen("p1") {
		t.Error("expected circuit closed after reset")
	}
	h := f.GetHealth("p1")
	if h.Status != ProviderStatusUp {
		t.Errorf("expected up, got %s", h.Status)
	}
}

// 7. ResetCircuit 不存在的 provider
func TestResetCircuitNonExist(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	if f.ResetCircuit("non-exist") {
		t.Error("expected false for non-exist provider")
	}
}

// 8. 延迟超过阈值 - degraded
func TestDegradedByLatency(t *testing.T) {
	d := newTestDispatcher()
	d.AddProvider(ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true})
	f := newTestFailover(t, d, nil)
	cfg := f.Config()
	cfg.DegradedLatencyMs = 500
	f.ApplyConfig(cfg)
	f.SetHealthChecker(newFakeChecker(800, nil))
	f.checkOne(context.Background(), &ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true}, f.Config())
	h := f.GetHealth("p1")
	if h.Status != ProviderStatusDegraded {
		t.Errorf("expected degraded, got %s", h.Status)
	}
}

// 9. RecordSuccess 重置失败计数
func TestRecordSuccess(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	f.RecordFailure("p1", errors.New("err1"))
	f.RecordFailure("p1", errors.New("err2"))
	f.RecordSuccess("p1", 200)
	h := f.GetHealth("p1")
	if h.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 failures, got %d", h.ConsecutiveFailures)
	}
	if h.Status != ProviderStatusUp {
		t.Errorf("expected up, got %s", h.Status)
	}
}

// 10. RecordFailure 累计
func TestRecordFailure(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	f.RecordFailure("p1", errors.New("err1"))
	h := f.GetHealth("p1")
	if h.ConsecutiveFailures != 1 {
		t.Errorf("expected 1, got %d", h.ConsecutiveFailures)
	}
	if h.Status != ProviderStatusDegraded {
		t.Errorf("expected degraded, got %s", h.Status)
	}
}

// 11. 熔断时间过期 - 自动恢复探测
func TestCircuitExpiry(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	f.RecordFailure("p1", errors.New("err"))
	h := f.GetHealth("p1")
	h.CircuitOpenUntil = time.Now().Add(-time.Second)
	if f.IsCircuitOpen("p1") {
		t.Error("expected circuit closed after expiry")
	}
}

// 12. LoadPolicy 无 DB 时使用默认策略
func TestLoadPolicyNoDB(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	p := f.LoadPolicy(context.Background())
	if len(p.Scenarios) == 0 {
		t.Fatal("expected default scenarios")
	}
}

// 13. LoadPolicy 从 DB 读取
func TestLoadPolicyFromDB(t *testing.T) {
	db := setupFailoverDB(t)
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS system_kv_config (
		id BIGSERIAL PRIMARY KEY,
		key VARCHAR(128) NOT NULL UNIQUE,
		value TEXT NOT NULL,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	customPolicy := `{"config":{"health_check_interval":60,"failure_threshold":3,"circuit_open_duration":120,"degraded_latency_ms":5000,"local_fallback_provider":"my_local","template_reply":"custom reply","health_check_path":"/v1/health"},"scenarios":{"intent_recognize":["default","custom"]}}`
	if err := db.Exec(`INSERT INTO system_kv_config (key, value) VALUES ('llm_provider_failover', $1)`, customPolicy).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	d := newTestDispatcher()
	f := newTestFailover(t, d, db)
	p := f.LoadPolicy(context.Background())
	if p.Config.HealthCheckInterval != 60 {
		t.Errorf("expected 60, got %d", p.Config.HealthCheckInterval)
	}
	if p.Config.FailureThreshold != 3 {
		t.Errorf("expected 3, got %d", p.Config.FailureThreshold)
	}
	if p.Config.LocalFallbackProvider != "my_local" {
		t.Errorf("expected my_local, got %s", p.Config.LocalFallbackProvider)
	}
	if p.Config.TemplateReply != "custom reply" {
		t.Errorf("expected custom reply, got %s", p.Config.TemplateReply)
	}
	list := p.Scenarios["intent_recognize"]
	if len(list) != 2 || list[1] != "custom" {
		t.Errorf("expected [default, custom], got %v", list)
	}
}

// 14. buildCandidates 包含本地兜底
func TestBuildCandidatesIncludesFallback(t *testing.T) {
	d := newTestDispatcher()
	d.SetRoute(ScenarioRoute{Scenario: ScenarioIntentRecognize, Provider: "deepseek", Fallbacks: []string{"qwen"}})
	f := newTestFailover(t, d, nil)
	policy := DefaultFailoverPolicy()
	list := f.buildCandidates(ScenarioIntentRecognize, policy)
	found := false
	for _, n := range list {
		if n == "default" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected default in candidates, got %v", list)
	}
}

// 15. IsDegraded 判断降级响应
func TestIsDegraded(t *testing.T) {
	if IsDegraded(nil) {
		t.Error("nil should not be degraded")
	}
	r := &DispatchResult{Provider: "degraded", Model: "template"}
	if !IsDegraded(r) {
		t.Error("expected degraded")
	}
	r2 := &DispatchResult{Provider: "deepseek", Model: "deepseek-chat"}
	if IsDegraded(r2) {
		t.Error("normal result should not be degraded")
	}
}

// 16. HTTPHealthChecker 无 BaseURL 视为本地（成功）
func TestHTTPHealthCheckerLocal(t *testing.T) {
	c := NewHTTPHealthChecker()
	latency, err := c.Ping(context.Background(), &ProviderConfig{Name: "local"}, DefaultFailoverConfig())
	if err != nil {
		t.Errorf("expected nil err for local, got %v", err)
	}
	if latency != 0 {
		t.Errorf("expected 0 latency for local, got %d", latency)
	}
}

// 17. HTTPHealthChecker nil provider
func TestHTTPHealthCheckerNil(t *testing.T) {
	c := NewHTTPHealthChecker()
	_, err := c.Ping(context.Background(), nil, DefaultFailoverConfig())
	if err == nil {
		t.Error("expected error for nil provider")
	}
}

// 18. GetHealth 不存在的 provider 返回 nil
func TestGetHealthNonExist(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	if h := f.GetHealth("non-exist"); h != nil {
		t.Errorf("expected nil, got %v", h)
	}
}

// 19. GetAllHealth 返回所有
func TestGetAllHealth(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	f.RecordSuccess("p1", 100)
	f.RecordSuccess("p2", 200)
	all := f.GetAllHealth()
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}
}

// 20. ApplyConfig 修改配置
func TestApplyConfig(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	cfg := f.Config()
	cfg.FailureThreshold = 10
	f.ApplyConfig(cfg)
	if f.Config().FailureThreshold != 10 {
		t.Errorf("expected 10, got %d", f.Config().FailureThreshold)
	}
}

// 21. DispatchWithFailover 全部失败 → 降级响应
func TestDispatchWithFailoverAllFailed(t *testing.T) {
	d := newTestDispatcher()
	d.AddProvider(ProviderConfig{Name: "p1", BaseURL: "http://nonexist.invalid", Enabled: true})
	f := newTestFailover(t, d, nil)
	result, err := f.DispatchWithFailover(context.Background(), DispatchRequest{
		Scenario: ScenarioIntentRecognize,
		Prompt:   "hello",
	})
	if err != nil {
		t.Fatalf("expected nil err (degraded), got %v", err)
	}
	if !IsDegraded(result) {
		t.Errorf("expected degraded result, got provider=%s model=%s content=%s",
			result.Provider, result.Model, result.Content)
	}
	if result.Content == "" {
		t.Error("degraded response should have non-empty content")
	}
}

// 22. Stop 不重复关闭
func TestFailoverStop(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	f.Stop()
	f.Stop() 
}

// 23. Start 后 Stop 优雅关闭
func TestFailoverStartStop(t *testing.T) {
	d := newTestDispatcher()
	d.AddProvider(ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true})
	f := newTestFailover(t, d, nil)
	f.SetHealthChecker(newFakeChecker(50, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	f.Stop()
}

// 24. LoadPolicy 异常 JSON 使用默认
func TestLoadPolicyBadJSON(t *testing.T) {
	db := setupFailoverDB(t)
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS system_kv_config (
		id BIGSERIAL PRIMARY KEY,
		key VARCHAR(128) NOT NULL UNIQUE,
		value TEXT NOT NULL,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec(`INSERT INTO system_kv_config (key, value) VALUES ('llm_provider_failover', 'not-json')
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	d := newTestDispatcher()
	f := newTestFailover(t, d, db)
	p := f.LoadPolicy(context.Background())
	if p.Config.HealthCheckInterval != 30 {
		t.Errorf("expected default 30, got %d", p.Config.HealthCheckInterval)
	}
}

// 25. interval 默认值
func TestIntervalDefault(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	if d := f.interval(); d != 30*time.Second {
		t.Errorf("expected 30s, got %v", d)
	}
}

// 26. SetHealthChecker 注入 nil 不影响默认
func TestSetHealthCheckerNil(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	f.SetHealthChecker(nil) 
	latency, err := f.checker.Ping(context.Background(), &ProviderConfig{Name: "local"}, f.Config())
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
	if latency != 0 {
		t.Errorf("expected 0, got %d", latency)
	}
}

// 27. 健康检查成功后状态恢复
func TestRecoverFromDegraded(t *testing.T) {
	d := newTestDispatcher()
	f := newTestFailover(t, d, nil)
	f.SetHealthChecker(newFakeChecker(0, errors.New("err")))
	f.checkOne(context.Background(), &ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true}, f.Config())
	if f.GetHealth("p1").Status != ProviderStatusDegraded {
		t.Fatal("expected degraded")
	}
	f.SetHealthChecker(newFakeChecker(100, nil))
	f.checkOne(context.Background(), &ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true}, f.Config())
	if f.GetHealth("p1").Status != ProviderStatusUp {
		t.Errorf("expected up, got %s", f.GetHealth("p1").Status)
	}
}

// 28. 熔断期间跳过该 provider
func TestCircuitOpenSkipsProvider(t *testing.T) {
	d := newTestDispatcher()
	d.AddProvider(ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true})
	f := newTestFailover(t, d, nil)
	f.mu.Lock()
	f.health["p1"] = &ProviderHealth{
		ProviderName:     "p1",
		Status:           ProviderStatusDown,
		CircuitOpenUntil: time.Now().Add(1 * time.Hour),
	}
	f.mu.Unlock()
	if !f.IsCircuitOpen("p1") {
		t.Fatal("expected circuit open")
	}
}

// 29. 健康检查跳过 disabled provider
func TestCheckAllSkipsDisabled(t *testing.T) {
	d := newTestDispatcher()
	d.AddProvider(ProviderConfig{Name: "p1", BaseURL: "http://example.com", Enabled: true})
	d.AddProvider(ProviderConfig{Name: "p2", BaseURL: "http://example.com", Enabled: false})
	f := newTestFailover(t, d, nil)
	f.SetHealthChecker(newFakeChecker(100, nil))
	f.checkAll(context.Background())
	if f.GetHealth("p1") == nil {
		t.Error("expected p1 checked")
	}
	if f.GetHealth("p2") != nil {
		t.Error("expected p2 skipped")
	}
}

// 30. DefaultFailoverConfig HealthCheckPath
func TestDefaultHealthCheckPath(t *testing.T) {
	cfg := DefaultFailoverConfig()
	if cfg.HealthCheckPath != "/health" {
		t.Errorf("expected /health, got %s", cfg.HealthCheckPath)
	}
}

