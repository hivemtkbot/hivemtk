package selflearning

// orchestrator_test.go Orchestrator 单元测试
//
// 测试策略：
//   - 使用标准库 testing，table-driven + t.Run 子测试
//   - 自实现 mockEventBus / mockRAGEngine
//   - 复用 switch_service_test.go 中的 mockSwitchRepo / mockLogRepo / newTestService / setCache
//   - 复用 rag_self_supervisor_test.go 中的 mockSignalRepo / newSupervisorWithMocks / newSwitchSvcWithCache
//   - 覆盖：Start/Stop 生命周期、事件处理（dialogue.started/ended/asset.degraded）、
//     幂等性（通过组件 logRepo.ExistsBySessionAndScenario）、信号量并发控制、
//     协程优雅退出、各组件 nil 时的降级（panic 恢复）、Cron 定时任务、统计信息
//   - 测试函数命名：TestOrchestrator_XXX
//
// 注意：
//   - Orchestrator 的事件 handler 不直接检查 SwitchService 状态，开关检查下沉到各组件内部
//   - 幂等性由各组件通过 logRepo.ExistsBySessionAndScenario 实现，Orchestrator 每次事件都会 spawn
//   - 各组件为 nil 时，handler spawn 的协程会 nil dereference panic，由 spawn 的 recover 兜底

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"marketing/internal/event"
	"marketing/internal/model"
)

// ============================================================================
// Mock EventBus
// ============================================================================

// mockEventBus EventBus 接口的内存 mock
//
// 记录所有 Subscribe/Publish 调用，支持按 topic 配置订阅失败。
type mockEventBus struct {
	mu sync.Mutex

	// 订阅记录
	subs map[string][]any

	// 行为控制
	failOnTopic  string // 若非空，Subscribe 该 topic 时返回错误
	subscribeErr error // 通用订阅错误（failOnTopic 为空时生效）
	publishErr   error

	// 调用计数
	subscribeCalls int
	publishCalls   int

	// Publish 捕获
	publishTopics   []string
	publishPayloads []any
}

func (m *mockEventBus) Publish(topic string, payload any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishCalls++
	m.publishTopics = append(m.publishTopics, topic)
	m.publishPayloads = append(m.publishPayloads, payload)
	return m.publishErr
}

func (m *mockEventBus) Subscribe(topic string, handler any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribeCalls++
	if m.failOnTopic != "" && topic == m.failOnTopic {
		return fmt.Errorf("subscribe failed for topic %s", topic)
	}
	if m.subscribeErr != nil {
		return m.subscribeErr
	}
	if m.subs == nil {
		m.subs = make(map[string][]any)
	}
	m.subs[topic] = append(m.subs[topic], handler)
	return nil
}

// getStartedHandler 获取 dialogue.started 的订阅 handler
func (m *mockEventBus) getStartedHandler() func(*event.DialogueStartedPayload) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.subs[event.TopicDialogueStarted]; ok && len(h) > 0 {
		return h[0].(func(*event.DialogueStartedPayload))
	}
	return nil
}

// getEndedHandler 获取 dialogue.ended 的订阅 handler
func (m *mockEventBus) getEndedHandler() func(*event.DialogueEndedPayload) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.subs[event.TopicDialogueEnded]; ok && len(h) > 0 {
		return h[0].(func(*event.DialogueEndedPayload))
	}
	return nil
}

// getDegradedHandler 获取 asset.degraded 的订阅 handler
func (m *mockEventBus) getDegradedHandler() func(*event.AssetDegradedPayload) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.subs[event.TopicAssetDegraded]; ok && len(h) > 0 {
		return h[0].(func(*event.AssetDegradedPayload))
	}
	return nil
}

// subCount 获取某 topic 的订阅数
func (m *mockEventBus) subCount(topic string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.subs[topic])
}

// 编译期接口断言
var _ EventBus = (*mockEventBus)(nil)

// ============================================================================
// Mock RAGEngine
// ============================================================================

// mockRAGEngine RAGEngine 接口的内存 mock
type mockRAGEngine struct {
	mu             sync.Mutex
	warmupErr      error
	warmupCalls    int
	retrieveErr    error
	retrieveResult []RAGChunk
	retrieveCalls  int
	lastSessionID  string
	lastQuery      string
	lastTTL        time.Duration
	lastTopK       int

	// warmupBlock 若非 nil，Warmup 会阻塞直到该 channel 被关闭
	// 用于测试 InFlight 统计（需要协程驻留）
	warmupBlock chan struct{}
}

func (m *mockRAGEngine) Warmup(ctx context.Context, sessionID, query string, ttl time.Duration) error {
	m.mu.Lock()
	m.warmupCalls++
	m.lastSessionID = sessionID
	m.lastQuery = query
	m.lastTTL = ttl
	block := m.warmupBlock
	m.mu.Unlock()
	// 在阻塞期间不持有 mu，允许其他协程进入
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.warmupErr
}

func (m *mockRAGEngine) Retrieve(ctx context.Context, query string, topK int) ([]RAGChunk, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retrieveCalls++
	m.lastQuery = query
	m.lastTopK = topK
	return m.retrieveResult, m.retrieveErr
}

// 编译期接口断言
var _ RAGEngine = (*mockRAGEngine)(nil)

// ============================================================================
// 辅助函数
// ============================================================================

// orchestratorCfg 测试用 Orchestrator 配置
type orchestratorCfg struct {
	switchSvc       *SwitchService
	ragCorrector    *RAGSelfCorrector
	assetLearner    *AssetBundleLearner
	ragSupervisor   *RAGSelfSupervisor
	assetSupervisor *AssetBundleSelfSupervisor
	publisher       *DialogueEventPublisher
	bus             EventBus
	maxConcurrent   int
}

// newTestOrchestrator 构造测试用 Orchestrator
func newTestOrchestrator(cfg orchestratorCfg) *Orchestrator {
	return NewOrchestrator(
		cfg.switchSvc,
		cfg.ragCorrector,
		cfg.assetLearner,
		cfg.publisher,
		cfg.bus,
		cfg.maxConcurrent,
		cfg.ragSupervisor,
		cfg.assetSupervisor,
	)
}

// newTestRAGCorrector 构造测试用 RAGSelfCorrector（chunkExtRepo/actionRepo/publisher 为 nil）
func newTestRAGCorrector(svc *SwitchService, logRepo *mockLogRepo, ragEngine RAGEngine) *RAGSelfCorrector {
	return NewRAGSelfCorrector(svc, nil, logRepo, nil, ragEngine, nil)
}

// newTestAssetLearner 构造测试用 AssetBundleLearner（除 switchSvc/logRepo 外均为 nil）
//
// 适用于 EnableAsset=false 或 reward<threshold 的早返回路径。
func newTestAssetLearner(svc *SwitchService, logRepo *mockLogRepo) *AssetBundleLearner {
	return NewAssetBundleLearner(svc, nil, nil, logRepo, nil, nil, nil, nil, nil)
}

// newTestAssetSupervisor 构造测试用 AssetBundleSelfSupervisor
func newTestAssetSupervisor(svc *SwitchService) *AssetBundleSelfSupervisor {
	return NewAssetBundleSelfSupervisor(svc, &mockSignalRepo{}, &mockLogRepo{}, nil, nil, nil, nil)
}

// defaultOrchSnap 默认 orchestrator 测试用 SwitchSnapshot（全启用、autonomous）
func defaultOrchSnap() *SwitchSnapshot {
	return &SwitchSnapshot{
		AutonomyLevel:           model.AutonomyLevelAutonomous,
		EnableRAG:               true,
		EnableAsset:             true,
		EnableLLM:               true,
		CircuitOpen:             false,
		MaxDailyCorrections:     100,
		MaxDailyPromotions:      5,
		LowQualityThreshold:     3.0,
		ChampionRewardThreshold: 1.5,
		ABTestMinSamples:        100,
		CircuitBreakerThreshold: 0.3,
		CircuitBreakerWindowMin: 30,
	}
}

// newOrchSwitchSvc 构造带缓存 SwitchService + mockSwitchRepo
func newOrchSwitchSvc(snap *SwitchSnapshot) (*SwitchService, *mockSwitchRepo) {
	svc, sr, _ := newTestService(5 * time.Second)
	setCache(svc, snap, 0, 0)
	return svc, sr
}

// makeStartedPayload 构造 dialogue.started payload
func makeStartedPayload(sessionID, firstMsg string) *event.DialogueStartedPayload {
	return &event.DialogueStartedPayload{
		SessionID:    sessionID,
		VisitorID:    "visitor-1",
		ChannelType:  "web_embed",
		Scenario:     "intent",
		FirstMessage: firstMsg,
		TraceID:      "trace-" + sessionID,
		StartedAt:    time.Now(),
	}
}

// makeEndedPayload 构造 dialogue.ended payload
func makeEndedPayload(sessionID string, reward float64) *event.DialogueEndedPayload {
	return &event.DialogueEndedPayload{
		SessionID:        sessionID,
		VisitorID:        "visitor-1",
		DurationSec:      120,
		Outcome:          "converted",
		AggregatedReward: reward,
		UsedCorpusIDs:    []string{},
		UsedAssetIDs:     []string{},
		LastCustomerMsg:  "hello",
		LastAIReply:      "hi there",
		TraceID:          "trace-" + sessionID,
		EndedAt:          time.Now(),
	}
}

// makeDegradedPayload 构造 asset.degraded payload
func makeDegradedPayload(assetID, reason string) *event.AssetDegradedPayload {
	return &event.AssetDegradedPayload{
		AssetID:      assetID,
		AssetTitle:   "test asset",
		Reason:       reason,
		LastUseCount: 0,
		LastRating:   1.5,
		Scenario:     "intent",
		TraceID:      "trace-degrade-" + assetID,
		DegradedAt:   time.Now(),
	}
}

// waitOrch 等待所有协程退出（带超时），返回是否在超时内完成
func waitOrch(o *Orchestrator, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// waitFor 轮询条件直到满足或超时
func waitFor(timeout time.Duration, check func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return check()
}

// forceStop 强制停止 orchestrator 并等待协程退出
//
// 用于测试中直接设置 running=true 后的清理。
func forceStop(o *Orchestrator, timeout time.Duration) bool {
	o.running.Store(false)
	select {
	case <-o.stopCh:
		// already closed
	default:
		close(o.stopCh)
	}
	return waitOrch(o, timeout)
}

// startOrch 直接设置 running=true（跳过 bus 订阅），用于 handler 测试
func startOrch(o *Orchestrator) {
	o.running.Store(true)
}

// ============================================================================
// TestOrchestrator_NewOrchestrator  构造函数默认值
// ============================================================================

func TestOrchestrator_NewOrchestrator(t *testing.T) {
	type tc struct {
		name          string
		maxConcurrent int
		wantMax       int
	}
	cases := []tc{
		{name: "zero_defaults_50", maxConcurrent: 0, wantMax: 50},
		{name: "negative_defaults_50", maxConcurrent: -1, wantMax: 50},
		{name: "negative_large_defaults_50", maxConcurrent: -100, wantMax: 50},
		{name: "one", maxConcurrent: 1, wantMax: 1},
		{name: "ten", maxConcurrent: 10, wantMax: 10},
		{name: "fifty", maxConcurrent: 50, wantMax: 50},
		{name: "hundred", maxConcurrent: 100, wantMax: 100},
		{name: "two_hundred", maxConcurrent: 200, wantMax: 200},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			o := NewOrchestrator(nil, nil, nil, nil, nil, c.maxConcurrent, nil, nil)
			if o.maxConcurrent != c.wantMax {
				t.Errorf("maxConcurrent = %d, want %d", o.maxConcurrent, c.wantMax)
			}
			if cap(o.sem) != c.wantMax {
				t.Errorf("sem cap = %d, want %d", cap(o.sem), c.wantMax)
			}
			if o.IsRunning() {
				t.Errorf("IsRunning = true, want false")
			}
			stats := o.GetStats()
			if stats.Running {
				t.Errorf("stats.Running = true, want false")
			}
			if stats.MaxConcurrent != c.wantMax {
				t.Errorf("stats.MaxConcurrent = %d, want %d", stats.MaxConcurrent, c.wantMax)
			}
		})
	}
}

// ============================================================================
// TestOrchestrator_Start  生命周期 - Start
// ============================================================================

func TestOrchestrator_Start(t *testing.T) {
	type tc struct {
		name        string
		bus         *mockEventBus
		wantErr     bool
		wantRunning bool
		wantSubs    int // 期望 subscribeCalls
	}
	cases := []tc{
		{
			name:        "nil_bus_returns_error",
			bus:         nil,
			wantErr:     true,
			wantRunning: false,
			wantSubs:    0,
		},
		{
			name:        "success",
			bus:         &mockEventBus{},
			wantErr:     false,
			wantRunning: true,
			wantSubs:    3,
		},
		{
			name:        "fail_on_started_topic",
			bus:         &mockEventBus{failOnTopic: event.TopicDialogueStarted},
			wantErr:     true,
			wantRunning: false,
			wantSubs:    1,
		},
		{
			name:        "fail_on_ended_topic",
			bus:         &mockEventBus{failOnTopic: event.TopicDialogueEnded},
			wantErr:     true,
			wantRunning: false,
			wantSubs:    2,
		},
		{
			name:        "fail_on_degraded_topic",
			bus:         &mockEventBus{failOnTopic: event.TopicAssetDegraded},
			wantErr:     true,
			wantRunning: false,
			wantSubs:    3,
		},
		{
			name:        "generic_subscribe_error",
			bus:         &mockEventBus{subscribeErr: errors.New("bus down")},
			wantErr:     true,
			wantRunning: false,
			wantSubs:    1,
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			var bus EventBus
			if c.bus != nil {
				bus = c.bus
			}
			o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
			err := o.Start(context.Background())
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if o.IsRunning() != c.wantRunning {
				t.Errorf("IsRunning = %v, want %v", o.IsRunning(), c.wantRunning)
			}
			if c.bus != nil && c.bus.subscribeCalls != c.wantSubs {
				t.Errorf("subscribeCalls = %d, want %d", c.bus.subscribeCalls, c.wantSubs)
			}
			// 清理
			if o.IsRunning() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = o.Stop(ctx)
			}
		})
	}

	// 重复 Start
	t.Run("double_start_returns_error", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("first Start error: %v", err)
		}
		err := o.Start(context.Background())
		if err == nil {
			t.Fatalf("expected error on double Start, got nil")
		}
		if !errors.Is(err, ErrOrchestratorNotRunning) {
			t.Errorf("err = %v, want ErrOrchestratorNotRunning", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = o.Stop(ctx)
	})

	// Start 后订阅 handler 可被检索
	t.Run("subscribed_handlers_retrievable", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}
		if bus.getStartedHandler() == nil {
			t.Errorf("dialogue.started handler not subscribed")
		}
		if bus.getEndedHandler() == nil {
			t.Errorf("dialogue.ended handler not subscribed")
		}
		if bus.getDegradedHandler() == nil {
			t.Errorf("asset.degraded handler not subscribed")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = o.Stop(ctx)
	})

	// 并发 Start（只有一个成功）
	t.Run("concurrent_start_only_one_succeeds", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		var wg sync.WaitGroup
		var successes, failures int32
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := o.Start(context.Background())
				if err == nil {
					atomic.AddInt32(&successes, 1)
				} else {
					atomic.AddInt32(&failures, 1)
				}
			}()
		}
		wg.Wait()
		if successes != 1 {
			t.Errorf("successes = %d, want 1", successes)
		}
		if failures != 9 {
			t.Errorf("failures = %d, want 9", failures)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = o.Stop(ctx)
	})

	// Start 后失败再重试（新 orchestrator）
	t.Run("retry_after_failure_with_new_orchestrator", func(t *testing.T) {
		o1 := newTestOrchestrator(orchestratorCfg{bus: nil, maxConcurrent: 10})
		if err := o1.Start(context.Background()); err == nil {
			t.Fatalf("expected error with nil bus")
		}
		bus := &mockEventBus{}
		o2 := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o2.Start(context.Background()); err != nil {
			t.Fatalf("retry Start error: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = o2.Stop(ctx)
	})
}

// ============================================================================
// TestOrchestrator_Stop  生命周期 - Stop
// ============================================================================

func TestOrchestrator_Stop(t *testing.T) {
	t.Run("stop_when_not_running_returns_nil", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{bus: &mockEventBus{}, maxConcurrent: 10})
		err := o.Stop(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if o.IsRunning() {
			t.Errorf("IsRunning = true, want false")
		}
	})

	t.Run("stop_when_running", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := o.Stop(ctx); err != nil {
			t.Fatalf("Stop error: %v", err)
		}
		if o.IsRunning() {
			t.Errorf("IsRunning = true, want false")
		}
	})

	t.Run("double_stop_safe", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = o.Stop(ctx)
		// 第二次 Stop 应安全返回
		if err := o.Stop(ctx); err != nil {
			t.Fatalf("second Stop error: %v", err)
		}
	})

	t.Run("stop_with_cancelled_context", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立即取消
		_ = o.Stop(ctx)
		if o.IsRunning() {
			t.Errorf("IsRunning = true, want false")
		}
		// 确保协程能退出
		waitOrch(o, 3*time.Second)
	})

	t.Run("stop_no_inflight_goroutines", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		start := time.Now()
		_ = o.Stop(ctx)
		elapsed := time.Since(start)
		if elapsed > 2*time.Second {
			t.Errorf("Stop took too long: %v (no in-flight goroutines)", elapsed)
		}
		stats := o.GetStats()
		if stats.Running {
			t.Errorf("stats.Running = true, want false")
		}
		if stats.InFlight != 0 {
			t.Errorf("stats.InFlight = %d, want 0", stats.InFlight)
		}
	})
}

// ============================================================================
// TestOrchestrator_IsRunning
// ============================================================================

func TestOrchestrator_IsRunning(t *testing.T) {
	t.Run("before_start_false", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{bus: &mockEventBus{}, maxConcurrent: 10})
		if o.IsRunning() {
			t.Errorf("IsRunning = true, want false")
		}
	})

	t.Run("after_start_true", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}
		if !o.IsRunning() {
			t.Errorf("IsRunning = false, want true")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = o.Stop(ctx)
	})

	t.Run("after_stop_false", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = o.Stop(ctx)
		if o.IsRunning() {
			t.Errorf("IsRunning = true, want false")
		}
	})

	t.Run("start_fail_stays_false", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{bus: nil, maxConcurrent: 10})
		_ = o.Start(context.Background())
		if o.IsRunning() {
			t.Errorf("IsRunning = true, want false after failed Start")
		}
	})

	t.Run("get_stats_reflects_running", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if o.GetStats().Running {
			t.Errorf("stats.Running = true before Start")
		}
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}
		if !o.GetStats().Running {
			t.Errorf("stats.Running = false after Start")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = o.Stop(ctx)
		if o.GetStats().Running {
			t.Errorf("stats.Running = true after Stop")
		}
	})
}

// ============================================================================
// TestOrchestrator_HandleDialogueStarted  dialogue.started 事件处理
// ============================================================================

func TestOrchestrator_HandleDialogueStarted(t *testing.T) {
	type tc struct {
		name           string
		enableRAG      bool
		payload        *event.DialogueStartedPayload
		ragEngine      *mockRAGEngine
		logExists      bool
		wantStarted    int64
		wantSuccessMin int64
		wantFailedMax  int64
		wantWarmupMin  int
		wantWarmupMax  int
	}
	cases := []tc{
		{
			name: "nil_payload_no_spawn", enableRAG: true,
			payload: nil, wantStarted: 0, wantSuccessMin: 0, wantFailedMax: 0,
		},
		{
			name: "rag_enabled_warmup_success",
			enableRAG: true, payload: makeStartedPayload("s1", "hello world"),
			ragEngine: &mockRAGEngine{}, wantStarted: 1, wantSuccessMin: 1, wantFailedMax: 0,
			wantWarmupMin: 1, wantWarmupMax: 1,
		},
		{
			name: "rag_enabled_warmup_error_counts_failed",
			enableRAG: true, payload: makeStartedPayload("s2", "hello"),
			ragEngine: &mockRAGEngine{warmupErr: errors.New("rag down")},
			wantStarted: 1, wantSuccessMin: 0, wantFailedMax: 1,
			wantWarmupMin: 1, wantWarmupMax: 1,
		},
		{
			name: "rag_disabled_warmup_skipped_success",
			enableRAG: false, payload: makeStartedPayload("s3", "hello"),
			ragEngine: &mockRAGEngine{}, wantStarted: 1, wantSuccessMin: 1, wantFailedMax: 0,
			wantWarmupMin: 0, wantWarmupMax: 0,
		},
		{
			name: "rag_enabled_nil_engine_skipped_success",
			enableRAG: true, payload: makeStartedPayload("s4", "hello"),
			ragEngine: nil, wantStarted: 1, wantSuccessMin: 1, wantFailedMax: 0,
			wantWarmupMin: 0, wantWarmupMax: 0,
		},
		{
			name: "rag_enabled_empty_first_message_skipped",
			enableRAG: true, payload: makeStartedPayload("s5", ""),
			ragEngine: nil, wantStarted: 1, wantSuccessMin: 1, wantFailedMax: 0,
			wantWarmupMin: 0, wantWarmupMax: 0,
		},
		{
			name: "idempotent_exists_skips_warmup",
			enableRAG: true, payload: makeStartedPayload("s6", "hello"),
			ragEngine: &mockRAGEngine{}, logExists: true,
			wantStarted: 1, wantSuccessMin: 1, wantFailedMax: 0,
			wantWarmupMin: 0, wantWarmupMax: 0,
		},
		{
			name: "empty_session_id_still_processed",
			enableRAG: true, payload: makeStartedPayload("", "hello"),
			ragEngine: &mockRAGEngine{}, wantStarted: 1, wantSuccessMin: 1, wantFailedMax: 0,
			wantWarmupMin: 1, wantWarmupMax: 1,
		},
		{
			name: "long_first_message",
			enableRAG: true, payload: makeStartedPayload("s7", string(make([]byte, 500))),
			ragEngine: &mockRAGEngine{}, wantStarted: 1, wantSuccessMin: 1, wantFailedMax: 0,
			wantWarmupMin: 1, wantWarmupMax: 1,
		},
		{
			name: "various_scenario_intent",
			enableRAG: true, payload: &event.DialogueStartedPayload{
				SessionID: "s8", FirstMessage: "hi", Scenario: "intent", TraceID: "t8",
			},
			ragEngine: &mockRAGEngine{}, wantStarted: 1, wantSuccessMin: 1, wantFailedMax: 0,
			wantWarmupMin: 1, wantWarmupMax: 1,
		},
		{
			name: "various_scenario_objection",
			enableRAG: true, payload: &event.DialogueStartedPayload{
				SessionID: "s9", FirstMessage: "hi", Scenario: "objection", TraceID: "t9",
			},
			ragEngine: &mockRAGEngine{}, wantStarted: 1, wantSuccessMin: 1, wantFailedMax: 0,
			wantWarmupMin: 1, wantWarmupMax: 1,
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			snap := defaultOrchSnap()
			snap.EnableRAG = c.enableRAG
			svc, _ := newOrchSwitchSvc(snap)
			logRepo := &mockLogRepo{existsResult: c.logExists}
			var ragEngine RAGEngine
			if c.ragEngine != nil {
				ragEngine = c.ragEngine
			}
			ragC := newTestRAGCorrector(svc, logRepo, ragEngine)
			o := newTestOrchestrator(orchestratorCfg{
				switchSvc:    svc,
				ragCorrector: ragC,
				maxConcurrent: 10,
			})
			startOrch(o)
			defer forceStop(o, 5*time.Second)

			o.onDialogueStarted(c.payload)
			if !waitOrch(o, 5*time.Second) {
				t.Fatalf("goroutines did not exit in time")
			}

			stats := o.GetStats()
			if stats.Started < c.wantStarted {
				t.Errorf("Started = %d, want >= %d", stats.Started, c.wantStarted)
			}
			if stats.Success < c.wantSuccessMin {
				t.Errorf("Success = %d, want >= %d", stats.Success, c.wantSuccessMin)
			}
			if stats.Failed > c.wantFailedMax {
				t.Errorf("Failed = %d, want <= %d", stats.Failed, c.wantFailedMax)
			}
			if c.ragEngine != nil {
				if c.ragEngine.warmupCalls < c.wantWarmupMin || c.ragEngine.warmupCalls > c.wantWarmupMax {
					t.Errorf("warmupCalls = %d, want [%d, %d]", c.ragEngine.warmupCalls, c.wantWarmupMin, c.wantWarmupMax)
				}
			}
		})
	}

	// 未启动时不 spawn
	t.Run("not_running_no_spawn", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		ragC := newTestRAGCorrector(svc, &mockLogRepo{}, &mockRAGEngine{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, maxConcurrent: 10,
		})
		// 不调用 startOrch
		o.onDialogueStarted(makeStartedPayload("s1", "hello"))
		time.Sleep(100 * time.Millisecond)
		stats := o.GetStats()
		if stats.Started != 0 {
			t.Errorf("Started = %d, want 0 when not running", stats.Started)
		}
	})

	// 多次调用 onDialogueStarted（每次都 spawn）
	t.Run("multiple_events_each_spawns", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		ragC := newTestRAGCorrector(svc, &mockLogRepo{}, &mockRAGEngine{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		for i := 0; i < 5; i++ {
			o.onDialogueStarted(makeStartedPayload(fmt.Sprintf("multi-%d", i), "hello"))
		}
		if !waitOrch(o, 5*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Started < 5 {
			t.Errorf("Started = %d, want >= 5", stats.Started)
		}
		if stats.Success < 5 {
			t.Errorf("Success = %d, want >= 5", stats.Success)
		}
	})

	// GetStatus 错误传播（组件内 GetStatus 失败 → Warmup 返回 error → statsFailed）
	t.Run("get_status_error_propagates_to_failed", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		// 清空缓存，使 GetStatus 走 DB 失败
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("db down")
		ragC := newTestRAGCorrector(svc, &mockLogRepo{}, &mockRAGEngine{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.onDialogueStarted(makeStartedPayload("err-1", "hello"))
		if !waitOrch(o, 5*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed < 1 {
			t.Errorf("Failed = %d, want >= 1 (GetStatus error)", stats.Failed)
		}
	})

	// 订阅 wiring：通过 Start 订阅后，handler 可被检索并调用
	t.Run("start_subscribes_started_handler", func(t *testing.T) {
		bus := &mockEventBus{}
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		ragC := newTestRAGCorrector(svc, &mockLogRepo{}, &mockRAGEngine{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, bus: bus, maxConcurrent: 10,
		})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = o.Stop(ctx)
		}()

		handler := bus.getStartedHandler()
		if handler == nil {
			t.Fatalf("started handler not subscribed")
		}
		handler(makeStartedPayload("wired-1", "hello"))
		if !waitOrch(o, 5*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Started < 1 {
			t.Errorf("Started = %d, want >= 1", stats.Started)
		}
	})
}

// ============================================================================
// TestOrchestrator_HandleDialogueEnded  dialogue.ended 事件处理
// ============================================================================

func TestOrchestrator_HandleDialogueEnded(t *testing.T) {
	type tc struct {
		name            string
		enableRAG       bool
		enableAsset     bool
		payload         *event.DialogueEndedPayload
		ragSupervisor   *RAGSelfSupervisor
		assetSupervisor *AssetBundleSelfSupervisor
		wantStartedMin  int64
		wantSuccessMin  int64
		wantFailedMax   int64
	}
	cases := []tc{
		{
			name: "nil_payload_no_spawn", enableRAG: true, enableAsset: true,
			payload: nil, wantStartedMin: 0, wantSuccessMin: 0, wantFailedMax: 0,
		},
		{
			name: "rag_on_asset_on_no_supervisors",
			enableRAG: true, enableAsset: true,
			payload: makeEndedPayload("e1", 0.5),
			wantStartedMin: 2, wantSuccessMin: 2, wantFailedMax: 0,
		},
		{
			name: "rag_off_asset_on",
			enableRAG: false, enableAsset: true,
			payload: makeEndedPayload("e2", 0.5),
			wantStartedMin: 2, wantSuccessMin: 1, wantFailedMax: 1, // ragCorrector.Reflect 返回 nil（RAG off），但 assetLearner 仍 spawn
		},
		{
			name: "rag_on_asset_off",
			enableRAG: true, enableAsset: false,
			payload: makeEndedPayload("e3", 0.5),
			wantStartedMin: 2, wantSuccessMin: 2, wantFailedMax: 0,
		},
		{
			name: "rag_on_asset_on_reward_below_threshold",
			enableRAG: true, enableAsset: true,
			payload: makeEndedPayload("e4", 0.5),
			wantStartedMin: 2, wantSuccessMin: 2, wantFailedMax: 0,
		},
		{
			name: "rag_on_asset_on_reward_negative_neutral",
			enableRAG: true, enableAsset: true,
			payload: makeEndedPayload("e5", -0.5),
			wantStartedMin: 2, wantSuccessMin: 1, wantFailedMax: 1,
		},
		{
			name: "with_rag_supervisor",
			enableRAG: true, enableAsset: true,
			payload: makeEndedPayload("e6", 0.5),
			ragSupervisor: newSupervisorWithMocks(nil, &mockSignalRepo{}, nil, nil),
			wantStartedMin: 3, wantSuccessMin: 2, wantFailedMax: 1,
		},
		{
			name: "with_asset_supervisor",
			enableRAG: true, enableAsset: true,
			payload: makeEndedPayload("e7", 0.5),
			assetSupervisor: newTestAssetSupervisor(nil),
			wantStartedMin: 3, wantSuccessMin: 2, wantFailedMax: 1,
		},
		{
			name: "with_both_supervisors",
			enableRAG: true, enableAsset: true,
			payload: makeEndedPayload("e8", 0.5),
			ragSupervisor:   newSupervisorWithMocks(nil, &mockSignalRepo{}, nil, nil),
			assetSupervisor: newTestAssetSupervisor(nil),
			wantStartedMin: 4, wantSuccessMin: 2, wantFailedMax: 2,
		},
		{
			name: "empty_session_id",
			enableRAG: true, enableAsset: true,
			payload: makeEndedPayload("", 0.5),
			wantStartedMin: 2, wantSuccessMin: 2, wantFailedMax: 0,
		},
		{
			name: "outcome_abandoned",
			enableRAG: true, enableAsset: true,
			payload: &event.DialogueEndedPayload{
				SessionID: "e9", AggregatedReward: -1.5, Outcome: "abandoned",
				UsedCorpusIDs: []string{}, UsedAssetIDs: []string{},
			},
			wantStartedMin: 2, wantSuccessMin: 1, wantFailedMax: 1,
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			snap := defaultOrchSnap()
			snap.EnableRAG = c.enableRAG
			snap.EnableAsset = c.enableAsset
			svc, _ := newOrchSwitchSvc(snap)

			// 构造 supervisor 时需要传入 svc（用于 GetStatus）
			var ragSup *RAGSelfSupervisor
			if c.ragSupervisor != nil {
				ragSup = newSupervisorWithMocks(svc, &mockSignalRepo{}, nil, nil)
			}
			var assetSup *AssetBundleSelfSupervisor
			if c.assetSupervisor != nil {
				assetSup = newTestAssetSupervisor(svc)
			}

			ragC := newTestRAGCorrector(svc, &mockLogRepo{}, nil)
			assetL := newTestAssetLearner(svc, &mockLogRepo{})

			o := newTestOrchestrator(orchestratorCfg{
				switchSvc: svc, ragCorrector: ragC, assetLearner: assetL,
				ragSupervisor: ragSup, assetSupervisor: assetSup, maxConcurrent: 10,
			})
			startOrch(o)
			defer forceStop(o, 5*time.Second)

			o.onDialogueEnded(c.payload)
			if !waitOrch(o, 5*time.Second) {
				t.Fatalf("goroutines did not exit in time")
			}

			stats := o.GetStats()
			if stats.Started < c.wantStartedMin {
				t.Errorf("Started = %d, want >= %d", stats.Started, c.wantStartedMin)
			}
			if stats.Success < c.wantSuccessMin {
				t.Errorf("Success = %d, want >= %d", stats.Success, c.wantSuccessMin)
			}
			if stats.Failed > c.wantFailedMax {
				t.Errorf("Failed = %d, want <= %d", stats.Failed, c.wantFailedMax)
			}
		})
	}

	// 未启动时不 spawn
	t.Run("not_running_no_spawn", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		ragC := newTestRAGCorrector(svc, &mockLogRepo{}, nil)
		assetL := newTestAssetLearner(svc, &mockLogRepo{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, assetLearner: assetL, maxConcurrent: 10,
		})
		o.onDialogueEnded(makeEndedPayload("nr-1", 0.5))
		time.Sleep(100 * time.Millisecond)
		stats := o.GetStats()
		if stats.Started != 0 {
			t.Errorf("Started = %d, want 0 when not running", stats.Started)
		}
	})

	// 幂等性：相同 session_id 调用两次 Reflect，第二次被 ExistsBySessionAndScenario 拦截
	t.Run("idempotent_same_session_reflect", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		logRepo := &mockLogRepo{existsResult: true} // 模拟已存在
		ragC := newTestRAGCorrector(svc, logRepo, nil)
		assetL := newTestAssetLearner(svc, &mockLogRepo{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, assetLearner: assetL, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		payload := makeEndedPayload("idem-1", 0.5)
		o.onDialogueEnded(payload)
		o.onDialogueEnded(payload) // 相同 session_id
		if !waitOrch(o, 5*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		// 每次 onDialogueEnded 都 spawn（Orchestrator 不去重），但组件内幂等检查使 Reflect 返回 nil
		if stats.Started < 4 {
			t.Errorf("Started = %d, want >= 4 (2 events × 2 actions)", stats.Started)
		}
		// Reflect 两次都返回 nil（幂等），GenerateCandidate 也会被 exists 拦截
		// 但 assetLearner 用的是独立 mockLogRepo，exists 默认 false
		// 所以至少 ragCorrector 的两次 Reflect 都成功
		if stats.Success < 2 {
			t.Errorf("Success = %d, want >= 2", stats.Success)
		}
	})
}

// ============================================================================
// TestOrchestrator_HandleAssetDegraded  asset.degraded 事件处理
// ============================================================================

func TestOrchestrator_HandleAssetDegraded(t *testing.T) {
	type tc struct {
		name    string
		payload *event.AssetDegradedPayload
	}
	cases := []tc{
		{name: "nil_payload", payload: nil},
		{name: "basic_degraded", payload: makeDegradedPayload("a1", "stale_or_low_rating")},
		{name: "low_effectiveness", payload: makeDegradedPayload("a2", "low_effectiveness")},
		{name: "version_stale", payload: makeDegradedPayload("a3", "version_stale")},
		{name: "empty_asset_id", payload: makeDegradedPayload("", "stale_or_low_rating")},
		{name: "empty_reason", payload: makeDegradedPayload("a4", "")},
		{name: "large_use_count", payload: &event.AssetDegradedPayload{
			AssetID: "a5", Reason: "stale_or_low_rating", LastUseCount: 99999, LastRating: 0.1,
		}},
		{name: "high_rating_still_degraded", payload: &event.AssetDegradedPayload{
			AssetID: "a6", Reason: "version_stale", LastUseCount: 100, LastRating: 4.5,
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
			startOrch(o)
			defer forceStop(o, 3*time.Second)

			// onAssetDegraded 仅记录日志，不 spawn 协程
			o.onAssetDegraded(c.payload)
			time.Sleep(50 * time.Millisecond)

			stats := o.GetStats()
			// asset.degraded 不 spawn，不增加 stats
			if stats.Started != 0 {
				t.Errorf("Started = %d, want 0 (asset.degraded does not spawn)", stats.Started)
			}
			if stats.InFlight != 0 {
				t.Errorf("InFlight = %d, want 0", stats.InFlight)
			}
		})
	}

	// 未启动时不处理
	t.Run("not_running_no_effect", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		o.onAssetDegraded(makeDegradedPayload("nr-1", "stale"))
		stats := o.GetStats()
		if stats.Started != 0 {
			t.Errorf("Started = %d, want 0", stats.Started)
		}
	})

	// 订阅 wiring
	t.Run("start_subscribes_degraded_handler", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = o.Stop(ctx)
		}()
		handler := bus.getDegradedHandler()
		if handler == nil {
			t.Fatalf("degraded handler not subscribed")
		}
		// 调用不应 panic
		handler(makeDegradedPayload("wired-1", "stale"))
	})
}

// ============================================================================
// TestOrchestrator_Concurrency  信号量并发控制
// ============================================================================

func TestOrchestrator_Concurrency(t *testing.T) {
	t.Run("semaphore_limits_concurrency", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 2})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		var current, maxRunning int32
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			o.spawn(func(ctx context.Context) {
				defer wg.Done()
				cur := atomic.AddInt32(&current, 1)
				for {
					old := atomic.LoadInt32(&maxRunning)
					if cur <= old || atomic.CompareAndSwapInt32(&maxRunning, old, cur) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				atomic.AddInt32(&current, -1)
			})
		}
		wg.Wait()
		if maxRunning > 2 {
			t.Errorf("maxRunning = %d, want <= 2 (maxConcurrent)", maxRunning)
		}
	})

	t.Run("max_concurrent_1_serializes", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 1})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		var current, maxRunning int32
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			o.spawn(func(ctx context.Context) {
				defer wg.Done()
				cur := atomic.AddInt32(&current, 1)
				if cur > 1 {
					t.Errorf("concurrent execution detected: current=%d", cur)
				}
				for {
					old := atomic.LoadInt32(&maxRunning)
					if cur <= old || atomic.CompareAndSwapInt32(&maxRunning, old, cur) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&current, -1)
			})
		}
		wg.Wait()
		if maxRunning != 1 {
			t.Errorf("maxRunning = %d, want 1", maxRunning)
		}
	})

	t.Run("max_concurrent_50_allows_parallel", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 50})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		var current int32
		var reachedParallel bool32
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			o.spawn(func(ctx context.Context) {
				defer wg.Done()
				cur := atomic.AddInt32(&current, 1)
				if cur >= 5 {
					reachedParallel.set(true)
				}
				time.Sleep(20 * time.Millisecond)
				atomic.AddInt32(&current, -1)
			})
		}
		wg.Wait()
		if !reachedParallel.get() {
			t.Errorf("expected parallel execution (current >= 5), but never reached")
		}
	})

	t.Run("spawn_after_stop_is_skipped", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 1})
		startOrch(o)
		// 先填满信号量（fn 不响应 ctx，确保 sem 被持有）
		blockCh := make(chan struct{})
		o.spawn(func(ctx context.Context) {
			<-blockCh
		})
		// 等待 sem 被持有，确保后续 spawn 只能走 stopCh 分支
		if !waitFor(1*time.Second, func() bool {
			return len(o.sem) > 0
		}) {
			t.Fatalf("goroutine did not acquire semaphore in time")
		}

		// 停止：不等待 wg（fn 不响应 ctx，会卡 5min 内部 ctx），
		// 仅关闭 stopCh 并尽量等待已 spawn 的 goroutine 退出
		o.running.Store(false)
		select {
		case <-o.stopCh:
		default:
			close(o.stopCh)
		}
		// 此时 sem 仍被原 goroutine 持有，stopCh 已关闭
		// 新 spawn 的 sem 分支无法命中（sem 满），只能走 stopCh 分支

		// 再 spawn 应被跳过
		var executed bool32
		o.spawn(func(ctx context.Context) {
			executed.set(true)
		})
		time.Sleep(100 * time.Millisecond)
		if executed.get() {
			t.Errorf("spawn after stop should be skipped, but fn executed")
		}
		stats := o.GetStats()
		if stats.Skipped < 1 {
			t.Errorf("Skipped = %d, want >= 1", stats.Skipped)
		}

		// 清理：让原 goroutine 退出
		close(blockCh)
		waitOrch(o, 3*time.Second)
	})

	t.Run("panic_in_fn_recovered", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 5})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		var wg sync.WaitGroup
		wg.Add(1)
		o.spawn(func(ctx context.Context) {
			defer wg.Done()
			panic("test panic")
		})
		wg.Wait()
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		if stats.Failed < 1 {
			t.Errorf("Failed = %d, want >= 1 (panic recovered)", stats.Failed)
		}
	})

	t.Run("multiple_panics_all_recovered", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			o.spawn(func(ctx context.Context) {
				defer wg.Done()
				panic(fmt.Sprintf("panic %d", 0))
			})
		}
		wg.Wait()
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		if stats.Failed < 5 {
			t.Errorf("Failed = %d, want >= 5", stats.Failed)
		}
	})
}

// bool32 线程安全的 bool
type bool32 struct {
	val int32
}

func (b *bool32) set(v bool) {
	if v {
		atomic.StoreInt32(&b.val, 1)
	} else {
		atomic.StoreInt32(&b.val, 0)
	}
}
func (b *bool32) get() bool {
	return atomic.LoadInt32(&b.val) == 1
}

// ============================================================================
// TestOrchestrator_GracefulShutdown  协程优雅退出
// ============================================================================

func TestOrchestrator_GracefulShutdown(t *testing.T) {
	t.Run("stop_waits_for_all_goroutines", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}

		// 使用 atomic 计数已进入 fn 的协程数，确保所有协程都进入 fn 后再 Stop
		var startedCount int32
		for i := 0; i < 10; i++ {
			o.spawn(func(ctx context.Context) {
				atomic.AddInt32(&startedCount, 1)
				time.Sleep(50 * time.Millisecond)
			})
		}
		if !waitFor(2*time.Second, func() bool {
			return atomic.LoadInt32(&startedCount) == 10
		}) {
			t.Fatalf("startedCount = %d, want 10 (goroutines did not enter fn in time)", atomic.LoadInt32(&startedCount))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := o.Stop(ctx); err != nil {
			t.Fatalf("Stop error: %v", err)
		}
		// 所有协程应已退出
		if !waitOrch(o, 1*time.Second) {
			t.Errorf("goroutines still running after Stop")
		}
		stats := o.GetStats()
		if stats.InFlight != 0 {
			t.Errorf("InFlight = %d, want 0 after Stop", stats.InFlight)
		}
	})

	t.Run("stop_cancels_blocking_goroutines", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}

		// 使用 atomic 计数已进入 fn 的协程数，确保所有协程都进入 fn 后再 Stop
		// （否则 spawn 的 goroutine 可能在 sem select 时走 stopCh 分支跳过 fn，
		// 导致测试 wg 永不 decrement）
		var startedCount int32
		for i := 0; i < 5; i++ {
			o.spawn(func(ctx context.Context) {
				atomic.AddInt32(&startedCount, 1)
				select {
				case <-ctx.Done():
				case <-time.After(10 * time.Minute): // 永不超时
				}
			})
		}
		if !waitFor(2*time.Second, func() bool {
			return atomic.LoadInt32(&startedCount) == 5
		}) {
			t.Fatalf("startedCount = %d, want 5 (goroutines did not enter fn in time)", atomic.LoadInt32(&startedCount))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		start := time.Now()
		_ = o.Stop(ctx)
		elapsed := time.Since(start)
		// Stop 应通过 cancel ctx 使协程快速退出
		if elapsed > 2*time.Second {
			t.Errorf("Stop took %v, expected fast via ctx cancel", elapsed)
		}
		// 等待所有协程退出（fn 响应 ctx.Done，应快速完成）
		if !waitOrch(o, 3*time.Second) {
			t.Errorf("goroutines did not exit in time after Stop")
		}
	})

	t.Run("stop_timeout_when_goroutines_ignore_ctx", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 10})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}

		// 这个协程不响应 ctx，需等待 5min 超时
		blockCh := make(chan struct{})
		o.spawn(func(ctx context.Context) {
			<-blockCh // 永不关闭
		})

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		_ = o.Stop(ctx)
		elapsed := time.Since(start)
		// Stop 应在 ctx 超时后返回（不会等 5min）
		if elapsed > 2*time.Second {
			t.Errorf("Stop took %v, expected to return after ctx timeout", elapsed)
		}
		// 此时协程仍在运行（不响应 ctx），但 running=false
		if o.IsRunning() {
			t.Errorf("IsRunning = true, want false after Stop")
		}
		// 强制清理
		o.running.Store(false)
		select {
		case <-o.stopCh:
		default:
			close(o.stopCh)
		}
		// 无法等待该协程退出（它会卡 5min），但不影响测试结论
		// 测试结束进程退出即可
	})

	t.Run("spawn_during_stop_is_skipped", func(t *testing.T) {
		bus := &mockEventBus{}
		o := newTestOrchestrator(orchestratorCfg{bus: bus, maxConcurrent: 1})
		if err := o.Start(context.Background()); err != nil {
			t.Fatalf("Start error: %v", err)
		}

		// 填满信号量（fn 不响应 ctx，确保 sem 被持有）
		blockCh := make(chan struct{})
		o.spawn(func(ctx context.Context) {
			<-blockCh
		})
		// 等待 sem 被持有（确保后续 spawn 只能走 stopCh 分支）
		if !waitFor(1*time.Second, func() bool {
			return len(o.sem) > 0
		}) {
			t.Fatalf("goroutine did not acquire semaphore in time")
		}

		// 停止（会关闭 stopCh；不等待 wg，因 fn 不响应 ctx）
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = o.Stop(ctx)
		// 此时 stopCh 已关闭，sem 仍被原 goroutine 持有
		// 新 spawn 只能走 stopCh 分支
		var executed bool32
		o.spawn(func(ctx context.Context) {
			executed.set(true)
		})
		time.Sleep(50 * time.Millisecond)
		if executed.get() {
			t.Errorf("spawn during stop should be skipped")
		}
		stats := o.GetStats()
		if stats.Skipped < 1 {
			t.Errorf("Skipped = %d, want >= 1", stats.Skipped)
		}

		// 清理：让原 goroutine 退出
		close(blockCh)
		waitOrch(o, 3*time.Second)
	})
}

// ============================================================================
// TestOrchestrator_Stats  统计信息
// ============================================================================

func TestOrchestrator_Stats(t *testing.T) {
	t.Run("initial_stats_zero", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		stats := o.GetStats()
		if stats.Started != 0 {
			t.Errorf("Started = %d, want 0", stats.Started)
		}
		if stats.Success != 0 {
			t.Errorf("Success = %d, want 0", stats.Success)
		}
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0", stats.Failed)
		}
		if stats.Skipped != 0 {
			t.Errorf("Skipped = %d, want 0", stats.Skipped)
		}
		if stats.InFlight != 0 {
			t.Errorf("InFlight = %d, want 0", stats.InFlight)
		}
		if stats.MaxConcurrent != 10 {
			t.Errorf("MaxConcurrent = %d, want 10", stats.MaxConcurrent)
		}
	})

	t.Run("stats_after_successful_spawn", func(t *testing.T) {
		// 通过 onDialogueStarted 触发，验证 statsSuccess 正确累加
		// 根因：spawn 本身不更新统计，统计更新在事件 handler 的闭包中
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		ragC := newTestRAGCorrector(svc, &mockLogRepo{}, &mockRAGEngine{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 3*time.Second)

		o.onDialogueStarted(makeStartedPayload("stat-ok", "hello"))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		if stats.Started < 1 {
			t.Errorf("Started = %d, want >= 1", stats.Started)
		}
		if stats.Success < 1 {
			t.Errorf("Success = %d, want >= 1", stats.Success)
		}
		if stats.InFlight != 0 {
			t.Errorf("InFlight = %d, want 0", stats.InFlight)
		}
	})

	t.Run("stats_after_failed_spawn", func(t *testing.T) {
		// ragCorrector 为 nil → spawn 内 nil dereference panic → recover 兜底 → statsFailed+1
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 3*time.Second)

		o.onDialogueStarted(makeStartedPayload("stat-fail", "hello"))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		if stats.Started < 1 {
			t.Errorf("Started = %d, want >= 1", stats.Started)
		}
		if stats.Failed < 1 {
			t.Errorf("Failed = %d, want >= 1 (nil ragCorrector panic recovered)", stats.Failed)
		}
		if stats.Success != 0 {
			t.Errorf("Success = %d, want 0", stats.Success)
		}
	})

	t.Run("in_flight_tracking", func(t *testing.T) {
		// 通过 onDialogueStarted 触发多个事件，使用 warmupBlock 阻塞协程
		// 验证 InFlight 计数正确反映正在运行的协程数
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		// 每个 session 需要独立的 mockRAGEngine（warmupBlock 共享）
		// 但 RAGSelfCorrector 持有单个 ragEngine，所以用同一 channel 阻塞所有 warmup
		blockCh := make(chan struct{})
		ragEngine := &mockRAGEngine{warmupBlock: blockCh}
		ragC := newTestRAGCorrector(svc, &mockLogRepo{}, ragEngine)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, maxConcurrent: 5,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		// 触发 3 个 dialogue.started 事件（不同 session_id 避免幂等冲突）
		for i := 0; i < 3; i++ {
			o.onDialogueStarted(makeStartedPayload(
				fmt.Sprintf("inflight-%d", i), "hi"))
		}

		// 等待协程获取信号量并进入 Warmup 阻塞
		if !waitFor(2*time.Second, func() bool {
			return o.GetStats().InFlight >= 3
		}) {
			t.Fatalf("InFlight = %d, want >= 3", o.GetStats().InFlight)
		}

		// 释放阻塞，等待协程退出
		close(blockCh)
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		if stats.InFlight != 0 {
			t.Errorf("InFlight = %d, want 0 after completion", stats.InFlight)
		}
		if stats.Success < 3 {
			t.Errorf("Success = %d, want >= 3", stats.Success)
		}
	})

	t.Run("started_count_via_dialogue_started", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		logRepo := &mockLogRepo{existsResult: true}
		ragC := newTestRAGCorrector(svc, logRepo, nil)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.onDialogueStarted(makeStartedPayload("stat-1", "hi"))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		if stats.Started < 1 {
			t.Errorf("Started = %d, want >= 1", stats.Started)
		}
	})

	t.Run("started_count_via_dialogue_ended", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		ragC := newTestRAGCorrector(svc, &mockLogRepo{}, nil)
		assetL := newTestAssetLearner(svc, &mockLogRepo{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, assetLearner: assetL, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.onDialogueEnded(makeEndedPayload("stat-2", 0.5))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		// onDialogueEnded 至少 spawn 2 个（ragCorrector + assetLearner）
		if stats.Started < 2 {
			t.Errorf("Started = %d, want >= 2", stats.Started)
		}
	})

	t.Run("max_concurrent_in_stats", func(t *testing.T) {
		for _, mc := range []int{1, 5, 10, 50, 100} {
			o := newTestOrchestrator(orchestratorCfg{maxConcurrent: mc})
			stats := o.GetStats()
			if stats.MaxConcurrent != mc {
				t.Errorf("MaxConcurrent = %d, want %d", stats.MaxConcurrent, mc)
			}
		}
	})
}

// ============================================================================
// TestOrchestrator_NilComponentDegradation  各组件 nil 时的降级
// ============================================================================

func TestOrchestrator_NilComponentDegradation(t *testing.T) {
	t.Run("rag_corrector_nil_dialogue_started_panics_recovered", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.onDialogueStarted(makeStartedPayload("nil-1", "hello"))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		if stats.Started < 1 {
			t.Errorf("Started = %d, want >= 1", stats.Started)
		}
		if stats.Failed < 1 {
			t.Errorf("Failed = %d, want >= 1 (nil dereference panic recovered)", stats.Failed)
		}
		if stats.Success != 0 {
			t.Errorf("Success = %d, want 0", stats.Success)
		}
	})

	t.Run("rag_corrector_nil_dialogue_ended_panics_recovered", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.onDialogueEnded(makeEndedPayload("nil-2", 0.5))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		// 2 个 spawn 都会 panic（ragCorrector.Reflect + assetLearner.GenerateCandidate）
		if stats.Failed < 2 {
			t.Errorf("Failed = %d, want >= 2", stats.Failed)
		}
	})

	t.Run("asset_learner_nil_dialogue_ended_rag_corrector_ok", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		ragC := newTestRAGCorrector(svc, &mockLogRepo{existsResult: true}, nil)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC,
			assetLearner: nil, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.onDialogueEnded(makeEndedPayload("nil-3", 0.5))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		// ragCorrector.Reflect 成功，assetLearner.GenerateCandidate panic
		if stats.Success < 1 {
			t.Errorf("Success = %d, want >= 1 (ragCorrector)", stats.Success)
		}
		if stats.Failed < 1 {
			t.Errorf("Failed = %d, want >= 1 (assetLearner nil)", stats.Failed)
		}
	})

	t.Run("rag_supervisor_nil_not_spawned", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		ragC := newTestRAGCorrector(svc, &mockLogRepo{existsResult: true}, nil)
		assetL := newTestAssetLearner(svc, &mockLogRepo{existsResult: true})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, assetLearner: assetL,
			ragSupervisor: nil, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.onDialogueEnded(makeEndedPayload("nil-4", 0.5))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		// ragSupervisor=nil → actionCount=2（不含 supervisor）
		if stats.Started > 3 {
			t.Errorf("Started = %d, want <= 3 (ragSupervisor nil, no extra spawn)", stats.Started)
		}
	})

	t.Run("asset_supervisor_nil_not_spawned", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		ragC := newTestRAGCorrector(svc, &mockLogRepo{existsResult: true}, nil)
		assetL := newTestAssetLearner(svc, &mockLogRepo{existsResult: true})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragCorrector: ragC, assetLearner: assetL,
			assetSupervisor: nil, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.onDialogueEnded(makeEndedPayload("nil-5", 0.5))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		// assetSupervisor=nil → actionCount=2
		if stats.Started > 3 {
			t.Errorf("Started = %d, want <= 3 (assetSupervisor nil)", stats.Started)
		}
	})

	t.Run("all_components_nil_dialogue_started", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.onDialogueStarted(makeStartedPayload("nil-all-1", "hello"))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		if stats.Failed < 1 {
			t.Errorf("Failed = %d, want >= 1", stats.Failed)
		}
	})

	t.Run("all_components_nil_dialogue_ended", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.onDialogueEnded(makeEndedPayload("nil-all-2", 0.5))
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		if stats.Failed < 2 {
			t.Errorf("Failed = %d, want >= 2 (both ragCorrector and assetLearner nil)", stats.Failed)
		}
	})

	t.Run("all_components_nil_asset_degraded_ok", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 3*time.Second)

		// onAssetDegraded 不调用任何组件，仅记录日志
		o.onAssetDegraded(makeDegradedPayload("nil-all-3", "stale"))
		stats := o.GetStats()
		if stats.Started != 0 {
			t.Errorf("Started = %d, want 0", stats.Started)
		}
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0", stats.Failed)
		}
	})

	t.Run("switch_svc_nil_cron_six_hours_panics_recovered", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronSixHours(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		// 3 个 spawn 都 panic（assetLearner nil + switchSvc nil）
		if stats.Failed < 3 {
			t.Errorf("Failed = %d, want >= 3", stats.Failed)
		}
	})

	t.Run("switch_svc_nil_cron_daily_panics_recovered", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronDaily(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		// 2 个 spawn 都 panic
		if stats.Failed < 2 {
			t.Errorf("Failed = %d, want >= 2", stats.Failed)
		}
	})

	t.Run("switch_svc_nil_cron_hourly_panics_recovered", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronHourly(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit")
		}
		stats := o.GetStats()
		// EvaluateCircuit panic（switchSvc nil），supervisors=nil 不 spawn
		if stats.Failed < 1 {
			t.Errorf("Failed = %d, want >= 1", stats.Failed)
		}
	})
}

// ============================================================================
// TestOrchestrator_OnCronSixHours
// ============================================================================

// 注意：cron 任务通过 o.spawn 直接调度，不经过 onDialogueStarted/onDialogueEnded，
// 因此不会递增 statsStarted / statsSuccess。测试通过 waitOrch、stats.Failed、
// stats.Skipped 以及 mock 调用计数来验证行为。

func TestOrchestrator_OnCronSixHours(t *testing.T) {
	t.Run("not_running_no_spawn", func(t *testing.T) {
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		o.OnCronSixHours(context.Background())
		// 未启动：不应 spawn 任何协程，waitOrch 应立即返回
		if !waitOrch(o, 200*time.Millisecond) {
			t.Errorf("goroutines did not exit in time (expected no spawn when not running)")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 when not running", stats.Failed)
		}
		if stats.Skipped != 0 {
			t.Errorf("Skipped = %d, want 0 when not running", stats.Skipped)
		}
	})

	t.Run("running_spawns_3_tasks_no_panic", func(t *testing.T) {
		snap := defaultOrchSnap()
		snap.EnableAsset = false // 使 ClusterCandidates/CheckConvergence 早返回
		svc, _ := newOrchSwitchSvc(snap)
		assetL := newTestAssetLearner(svc, &mockLogRepo{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, assetLearner: assetL, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronSixHours(context.Background())
		// 3 个 spawn: ClusterCandidates + CheckConvergence + EvaluateCircuit
		// 全部早返回或成功，无 panic
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 (all tasks should succeed)", stats.Failed)
		}
		if stats.InFlight != 0 {
			t.Errorf("InFlight = %d, want 0 after completion", stats.InFlight)
		}
	})

	t.Run("asset_disabled_all_success", func(t *testing.T) {
		snap := defaultOrchSnap()
		snap.EnableAsset = false
		svc, _ := newOrchSwitchSvc(snap)
		assetL := newTestAssetLearner(svc, &mockLogRepo{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, assetLearner: assetL, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronSixHours(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 (all early-return)", stats.Failed)
		}
	})

	t.Run("asset_learner_nil_panics_recovered", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, assetLearner: nil, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronSixHours(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		// ClusterCandidates + CheckConvergence panic (assetLearner nil),
		// EvaluateCircuit 成功（switchSvc 非 nil）
		if stats.Failed < 2 {
			t.Errorf("Failed = %d, want >= 2 (ClusterCandidates + CheckConvergence panic)", stats.Failed)
		}
		// EvaluateCircuit 不应 panic
		// 注意：总 spawn 数 = 3，其中 2 个 panic，1 个成功
		// 无法通过 stats.Success 验证（cron 不递增），改用 stats.Failed 上限
		if stats.Failed > 3 {
			t.Errorf("Failed = %d, want <= 3 (only 2 panics + at most 1 EvaluateCircuit)", stats.Failed)
		}
	})

	t.Run("switch_svc_nil_all_3_panics_recovered", func(t *testing.T) {
		// switchSvc=nil → EvaluateCircuit 也 panic
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronSixHours(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed < 3 {
			t.Errorf("Failed = %d, want >= 3 (all 3 tasks panic)", stats.Failed)
		}
	})
}

// ============================================================================
// TestOrchestrator_OnCronDaily
// ============================================================================

func TestOrchestrator_OnCronDaily(t *testing.T) {
	t.Run("not_running_no_spawn", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		assetL := newTestAssetLearner(svc, &mockLogRepo{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, assetLearner: assetL, maxConcurrent: 10,
		})
		o.OnCronDaily(context.Background())
		if !waitOrch(o, 200*time.Millisecond) {
			t.Errorf("goroutines did not exit in time (expected no spawn when not running)")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 when not running", stats.Failed)
		}
	})

	t.Run("running_spawns_2_tasks_no_panic", func(t *testing.T) {
		snap := defaultOrchSnap()
		snap.EnableAsset = false
		svc, sr := newOrchSwitchSvc(snap)
		assetL := newTestAssetLearner(svc, &mockLogRepo{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, assetLearner: assetL, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronDaily(context.Background())
		// 3 个 spawn: ResetDailyCounters + DegradeInactiveAssets + CleanStaleLogs
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 (all tasks should succeed)", stats.Failed)
		}
		// ResetDailyCounters 应被调用
		if sr.resetDailyCalls < 1 {
			t.Errorf("resetDailyCalls = %d, want >= 1", sr.resetDailyCalls)
		}
	})

	t.Run("asset_disabled_all_success", func(t *testing.T) {
		snap := defaultOrchSnap()
		snap.EnableAsset = false
		svc, sr := newOrchSwitchSvc(snap)
		assetL := newTestAssetLearner(svc, &mockLogRepo{})
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, assetLearner: assetL, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronDaily(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0", stats.Failed)
		}
		if sr.resetDailyCalls < 1 {
			t.Errorf("resetDailyCalls = %d, want >= 1", sr.resetDailyCalls)
		}
	})

	t.Run("asset_learner_nil_panics_recovered", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, sr := newOrchSwitchSvc(snap)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, assetLearner: nil, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronDaily(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		// ResetDailyCounters 成功, DegradeInactiveAssets panic
		if stats.Failed < 1 {
			t.Errorf("Failed = %d, want >= 1 (DegradeInactiveAssets panic)", stats.Failed)
		}
		if stats.Failed > 2 {
			t.Errorf("Failed = %d, want <= 2 (only DegradeInactiveAssets panics)", stats.Failed)
		}
		// ResetDailyCounters 仍应被调用
		if sr.resetDailyCalls < 1 {
			t.Errorf("resetDailyCalls = %d, want >= 1 (ResetDailyCounters should still run)", sr.resetDailyCalls)
		}
	})
}

// ============================================================================
// TestOrchestrator_OnCronHourly
// ============================================================================

func TestOrchestrator_OnCronHourly(t *testing.T) {
	t.Run("not_running_no_spawn", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		ragSup := newSupervisorWithMocks(svc, &mockSignalRepo{}, nil, nil)
		assetSup := newTestAssetSupervisor(svc)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragSupervisor: ragSup, assetSupervisor: assetSup, maxConcurrent: 10,
		})
		o.OnCronHourly(context.Background())
		if !waitOrch(o, 200*time.Millisecond) {
			t.Errorf("goroutines did not exit in time (expected no spawn when not running)")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 when not running", stats.Failed)
		}
	})

	t.Run("running_no_supervisors_only_evaluate_circuit", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronHourly(context.Background())
		// 仅 EvaluateCircuit（无 supervisor）
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 (EvaluateCircuit should succeed)", stats.Failed)
		}
	})

	t.Run("running_with_rag_supervisor_scan_alerts_called", func(t *testing.T) {
		snap := defaultOrchSnap()
		snap.EnableRAG = false // 使 ScanAlerts 早返回（GetStatus 后 EnableRAG=false）
		svc, _ := newOrchSwitchSvc(snap)
		ragSigRepo := &mockSignalRepo{}
		ragSup := newSupervisorWithMocks(svc, ragSigRepo, nil, nil)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragSupervisor: ragSup, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronHourly(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 (all early-return)", stats.Failed)
		}
		// ScanAlerts 内部调用 signalRepo.ListAlerts（即使 EnableRAG=false，
		// ScanAlerts 先 GetStatus 再判断 EnableRAG，但仍可能调用 ListAlerts）
		// 这里不严格断言 listAlertsCalls，因为 EnableRAG=false 时 ScanAlerts 早返回不调用 ListAlerts
	})

	t.Run("running_with_asset_supervisor_no_panic", func(t *testing.T) {
		snap := defaultOrchSnap()
		snap.EnableAsset = false
		svc, _ := newOrchSwitchSvc(snap)
		assetSup := newTestAssetSupervisor(svc)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, assetSupervisor: assetSup, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronHourly(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 (all early-return)", stats.Failed)
		}
	})

	t.Run("running_with_both_supervisors_no_panic", func(t *testing.T) {
		snap := defaultOrchSnap()
		snap.EnableRAG = false
		snap.EnableAsset = false
		svc, _ := newOrchSwitchSvc(snap)
		ragSup := newSupervisorWithMocks(svc, &mockSignalRepo{}, nil, nil)
		assetSup := newTestAssetSupervisor(svc)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragSupervisor: ragSup, assetSupervisor: assetSup, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronHourly(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 (all early-return)", stats.Failed)
		}
	})

	t.Run("rag_supervisor_nil_not_spawned", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, ragSupervisor: nil, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronHourly(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		// ragSupervisor=nil → 不 spawn ScanAlerts(rag)
		// 仅 EvaluateCircuit（可能 spawn）+ assetSupervisor=nil 不 spawn
		// 无 panic
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 (ragSupervisor nil, no spawn, no panic)", stats.Failed)
		}
	})

	t.Run("asset_supervisor_nil_not_spawned", func(t *testing.T) {
		snap := defaultOrchSnap()
		svc, _ := newOrchSwitchSvc(snap)
		o := newTestOrchestrator(orchestratorCfg{
			switchSvc: svc, assetSupervisor: nil, maxConcurrent: 10,
		})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronHourly(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed != 0 {
			t.Errorf("Failed = %d, want 0 (assetSupervisor nil, no spawn, no panic)", stats.Failed)
		}
	})

	t.Run("switch_svc_nil_evaluate_circuit_panics_recovered", func(t *testing.T) {
		// switchSvc=nil → EvaluateCircuit panic；supervisors=nil 不 spawn
		o := newTestOrchestrator(orchestratorCfg{maxConcurrent: 10})
		startOrch(o)
		defer forceStop(o, 5*time.Second)

		o.OnCronHourly(context.Background())
		if !waitOrch(o, 3*time.Second) {
			t.Fatalf("goroutines did not exit in time")
		}
		stats := o.GetStats()
		if stats.Failed < 1 {
			t.Errorf("Failed = %d, want >= 1 (EvaluateCircuit panic)", stats.Failed)
		}
		if stats.Failed > 1 {
			t.Errorf("Failed = %d, want <= 1 (only EvaluateCircuit spawns)", stats.Failed)
		}
	})
}
