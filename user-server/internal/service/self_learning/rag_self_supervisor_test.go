package selflearning

// rag_self_supervisor_test.go RAGSelfSupervisor 单元测试
//
// 测试策略：
//   - 使用标准库 testing，table-driven + t.Run 子测试
//   - 自实现 mockSignalRepo / mockLLMDispatcher / mockActionRepo（不依赖 DB）
//   - 复用 switch_service_test.go 中已有的 mockSwitchRepo / mockLogRepo / setCache / newTestService
//   - 覆盖：upsertSignal 状态分类边界、4 RAG + 1 Asset 指标采集、
//     ScanAlerts 告警扫描（多目标类型 + 多状态过滤）、LLM-as-Judge 采样、
//     GetDashboard 看板、错误传播、parseScore、computeCoverage、
//     getThreshold、metricDisplayName、SetLLMJudgeSampleRate
//   - 测试函数命名：TestRAGSelfSupervisor_XXX

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"marketing/internal/event"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// ============================================================================
// Mock Repositories / Dependencies
// ============================================================================

// mockSignalRepo SelfSupervisionSignalRepository 内存 mock
//
// UpsertSignal 内部会复刻真实 repo 的 classifyStatus 逻辑，
// 以便测试可以断言最终 status 字段。
type mockSignalRepo struct {
	mu sync.Mutex

	// 捕获的信号
	upsertCalls int
	lastSignal  *model.SelfSupervisionSignal
	allSignals  []*model.SelfSupervisionSignal

	// 行为控制
	upsertErr     error
	alertsList    []*model.SelfSupervisionSignal
	listAlertsErr error
	listMetricRes []*model.SelfSupervisionSignal
	listMetricErr error
	listTargetRes []*model.SelfSupervisionSignal
	listTargetErr error
	aggAvg        float64
	aggCount      int64
	aggErr        error
	getByIDSignal *model.SelfSupervisionSignal
	getByIDErr    error

	// 调用计数
	listAlertsCalls   int
	aggregateCalls    int
	listByTargetCalls int
	listByMetricCalls int
	getByIDCalls      int
}

func (m *mockSignalRepo) UpsertSignal(ctx context.Context, sig *model.SelfSupervisionSignal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upsertCalls++
	if m.upsertErr != nil {
		return m.upsertErr
	}
	cp := *sig
	m.classify(&cp)
	m.lastSignal = &cp
	m.allSignals = append(m.allSignals, &cp)
	return nil
}

// classify 复刻 repository.selfSupervisionSignalRepo.classifyStatus 的逻辑
func (m *mockSignalRepo) classify(sig *model.SelfSupervisionSignal) {
	if sig.Threshold <= 0 || sig.SampleCount == 0 {
		sig.Status = model.SupervisionStatusNormal
		return
	}
	if sig.Value >= sig.Threshold {
		sig.Status = model.SupervisionStatusAlert
	} else if sig.Value >= sig.Threshold*0.8 {
		sig.Status = model.SupervisionStatusWarning
	} else {
		sig.Status = model.SupervisionStatusNormal
	}
}

func (m *mockSignalRepo) GetByID(ctx context.Context, signalID string) (*model.SelfSupervisionSignal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getByIDCalls++
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	return m.getByIDSignal, nil
}

func (m *mockSignalRepo) ListByMetric(ctx context.Context, targetType model.SupervisionTargetType, metricName string, from, to time.Time, limit int) ([]*model.SelfSupervisionSignal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listByMetricCalls++
	return m.listMetricRes, m.listMetricErr
}

func (m *mockSignalRepo) ListAlerts(ctx context.Context, since time.Time, limit int) ([]*model.SelfSupervisionSignal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listAlertsCalls++
	if m.listAlertsErr != nil {
		return nil, m.listAlertsErr
	}
	return m.alertsList, nil
}

func (m *mockSignalRepo) ListByTarget(ctx context.Context, targetType model.SupervisionTargetType, targetID string, from, to time.Time, limit int) ([]*model.SelfSupervisionSignal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listByTargetCalls++
	return m.listTargetRes, m.listTargetErr
}

func (m *mockSignalRepo) AggregateByRange(ctx context.Context, targetType model.SupervisionTargetType, metricName string, from, to time.Time) (float64, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aggregateCalls++
	if m.aggErr != nil {
		return 0, 0, m.aggErr
	}
	return m.aggAvg, m.aggCount, nil
}

// mockLLMDispatcher LLMDispatcher 接口的内存 mock
type mockLLMDispatcher struct {
	mu            sync.Mutex
	dispatchCalls int
	content       string
	returnModel   string
	err           error
	lastScenario  string
	lastPrompt    string
	lastSystem    string
	lastJSONMode  bool
	lastMaxTokens int
}

func (m *mockLLMDispatcher) Dispatch(ctx context.Context, scenario string, prompt, systemPrompt string, jsonMode bool, maxTokens int) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dispatchCalls++
	m.lastScenario = scenario
	m.lastPrompt = prompt
	m.lastSystem = systemPrompt
	m.lastJSONMode = jsonMode
	m.lastMaxTokens = maxTokens
	return m.content, m.returnModel, m.err
}

// mockActionRepo SelfCorrectionActionRepository 内存 mock（供构造真实 SelfCorrectionDispatcher 使用）
type mockActionRepo struct {
	mu                sync.Mutex
	createErr         error
	createCalls       int
	getByIDAction     *model.SelfCorrectionAction
	getByIDErr        error
	updateErr         error
	updateCalls       int
	listPendingRes    []*model.SelfCorrectionAction
	listPendingErr    error
	listByTargetRes   []*model.SelfCorrectionAction
	listByTargetErr   error
	listByTriggerRes  []*model.SelfCorrectionAction
	listByTriggerErr  error
	listByFilterRes   []*model.SelfCorrectionAction
	listByFilterTotal int64
	listByFilterErr   error
	countTodayRes     map[model.CorrectionActionType]int64
	countTodayErr     error
}

func (m *mockActionRepo) Create(ctx context.Context, a *model.SelfCorrectionAction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	return m.createErr
}

func (m *mockActionRepo) GetByID(ctx context.Context, actionID string) (*model.SelfCorrectionAction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	return m.getByIDAction, nil
}

func (m *mockActionRepo) UpdateStatus(ctx context.Context, actionID string, status model.CorrectionActionStatus, extraUpdates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	return m.updateErr
}

func (m *mockActionRepo) ListPending(ctx context.Context, limit int) ([]*model.SelfCorrectionAction, error) {
	return m.listPendingRes, m.listPendingErr
}

func (m *mockActionRepo) ListByTarget(ctx context.Context, targetType, targetID string, limit int) ([]*model.SelfCorrectionAction, error) {
	return m.listByTargetRes, m.listByTargetErr
}

func (m *mockActionRepo) ListByTriggerLog(ctx context.Context, triggerLogID string, limit int) ([]*model.SelfCorrectionAction, error) {
	return m.listByTriggerRes, m.listByTriggerErr
}

func (m *mockActionRepo) ListByFilter(ctx context.Context, filter repository.CorrectionActionFilter) ([]*model.SelfCorrectionAction, int64, error) {
	return m.listByFilterRes, m.listByFilterTotal, m.listByFilterErr
}

func (m *mockActionRepo) CountToday(ctx context.Context) (map[model.CorrectionActionType]int64, error) {
	return m.countTodayRes, m.countTodayErr
}

// 编译期接口断言
var (
	_ repository.SelfSupervisionSignalRepository = (*mockSignalRepo)(nil)
	_ repository.SelfCorrectionActionRepository  = (*mockActionRepo)(nil)
	_ LLMDispatcher                              = (*mockLLMDispatcher)(nil)
)

// ============================================================================
// 辅助函数
// ============================================================================

// newSupervisorWithMocks 构造一个 RAGSelfSupervisor，所有依赖均为 mock
//
// 注意：当 llm 为 nil 时，必须传 nil 接口（而非含 nil 指针的接口），
// 否则 supervisor 中 `s.llmDispatcher != nil` 判断会因 typed-nil 陷阱而为真。
func newSupervisorWithMocks(
	switchSvc *SwitchService,
	signalRepo *mockSignalRepo,
	llm *mockLLMDispatcher,
	dispatcher *SelfCorrectionDispatcher,
) *RAGSelfSupervisor {
	var llmIface LLMDispatcher
	if llm != nil {
		llmIface = llm
	}
	return NewRAGSelfSupervisor(switchSvc, signalRepo, &mockLogRepo{}, dispatcher, llmIface)
}

// newSwitchSvcWithCache 构造一个 SwitchService 并预填缓存
func newSwitchSvcWithCache(snap *SwitchSnapshot) (*SwitchService, *mockSwitchRepo) {
	svc, sr, _ := newTestService(5 * time.Second)
	setCache(svc, snap, 0, 0)
	return svc, sr
}

// defaultSupervisorSnap 默认 supervisor 测试用 SwitchSnapshot
func defaultSupervisorSnap() *SwitchSnapshot {
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

// newDispatcherWithMocks 构造一个真实的 SelfCorrectionDispatcher，所有依赖均为 mock
func newDispatcherWithMocks(switchSvc *SwitchService, actionRepo *mockActionRepo) *SelfCorrectionDispatcher {
	return NewSelfCorrectionDispatcher(
		switchSvc,
		actionRepo,
		&mockLogRepo{},
		&mockSignalRepo{},
		nil, // ragCorrector
		nil, // assetLearner
		nil, // llmCorrector
	)
}

// countSignalsByMetric 统计 mock 中捕获的某 metric 信号数量
func countSignalsByMetric(signals []*model.SelfSupervisionSignal, metric string) int {
	n := 0
	for _, s := range signals {
		if s.MetricName == metric {
			n++
		}
	}
	return n
}

// findSignalByMetric 找到 mock 中捕获的某 metric 的第一个信号
func findSignalByMetric(signals []*model.SelfSupervisionSignal, metric string) *model.SelfSupervisionSignal {
	for _, s := range signals {
		if s.MetricName == metric {
			return s
		}
	}
	return nil
}

// ============================================================================
// TestRAGSelfSupervisor_UpsertSignal_StatusClassification
// 状态分类边界测试：normal / warning / alert
//   - threshold*0.8 边界 → warning
//   - threshold 边界 → alert
//   - value=0 → normal
//   - sample_count=0 → normal
//   - threshold=0 → normal
//   - 不同 SupervisionTargetType（rag/asset/llm/hybrid）
// ============================================================================

func TestRAGSelfSupervisor_UpsertSignal_StatusClassification(t *testing.T) {
	type tc struct {
		name        string
		targetType  model.SupervisionTargetType
		targetID    string
		metricName  string
		value       float64
		threshold   float64
		sampleCount int
		upsertErr   error
		wantStatus  model.SupervisionSignalStatus
		wantErr     bool
	}

	metrics := []string{
		model.SupervisionMetricRecallPrecision,
		model.SupervisionMetricRecallCoverage,
		model.SupervisionMetricGenerationFidelity,
		model.SupervisionMetricAnswerRelevance,
		model.SupervisionMetricAssetEffectiveness,
	}
	targets := []model.SupervisionTargetType{
		model.SupervisionTargetRAG,
		model.SupervisionTargetAsset,
		model.SupervisionTargetLLM,
		model.SupervisionTargetHybrid,
	}

	// 价值-阈值组合：覆盖 normal/warning/alert 三态 + 边界 + value=0 + sample=0 + threshold=0
	//
	// 注意：threshold*0.8 是 float64 运行时运算（如 0.8*0.8 = 0.6400000000000001），
	// 与编译期常量 0.64（= float64(0.64) ≈ 0.64000000000000001）不同。
	// 因此 at_0.8x_boundary 用 0.8*0.8 表达式（运行时与 classify 一致），
	// 而 literal_0.64 用字面量 0.64 验证 float 算术差异下的真实行为。
	threshold08 := 0.8
	warningBoundary := threshold08 * 0.8 // 运行时计算，与 classify 中 sig.Threshold*0.8 完全一致
	valueThresholdCases := []struct {
		value       float64
		threshold   float64
		sampleCount int
		wantStatus  model.SupervisionSignalStatus
		desc        string
	}{
		{0.0, 0.8, 1, model.SupervisionStatusNormal, "value_zero"},             // value=0
		{0.5, 0.8, 1, model.SupervisionStatusNormal, "below_0.8x"},             // 0.5 < 0.64
		{0.6399, 0.8, 1, model.SupervisionStatusNormal, "just_below_0.8x"},     // 0.6399 < 0.64
		{0.64, 0.8, 1, model.SupervisionStatusNormal, "literal_0.64_below_0.8x0.8"}, // 字面量 0.64 < 0.8*0.8
		{warningBoundary, 0.8, 1, model.SupervisionStatusWarning, "at_0.8x_boundary"}, // value == threshold*0.8 (运行时计算)
		{0.7, 0.8, 1, model.SupervisionStatusWarning, "warning_mid"},          // 0.7 in [0.64, 0.8)
		{0.7999, 0.8, 1, model.SupervisionStatusWarning, "just_below_threshold"},
		{0.8, 0.8, 1, model.SupervisionStatusAlert, "at_threshold_boundary"},   // 0.8 == 0.8
		{0.9, 0.8, 1, model.SupervisionStatusAlert, "above_threshold"},
		{1.0, 0.8, 1, model.SupervisionStatusAlert, "value_one"},
		{0.5, 0.8, 0, model.SupervisionStatusNormal, "sample_zero"},           // sample=0
		{0.9, 0.0, 1, model.SupervisionStatusNormal, "threshold_zero"},        // threshold=0
		{0.9, -0.1, 1, model.SupervisionStatusNormal, "threshold_negative"},   // threshold<0
	}

	var cases []tc
	// 5 metrics × 12 value/threshold combos = 60 cases
	for _, metric := range metrics {
		for i, vt := range valueThresholdCases {
			cases = append(cases, tc{
				name:        fmt.Sprintf("%s/%s", metric, vt.desc),
				targetType:  targets[i%len(targets)],
				targetID:    "",
				metricName:  metric,
				value:       vt.value,
				threshold:   vt.threshold,
				sampleCount: vt.sampleCount,
				wantStatus:  vt.wantStatus,
			})
		}
	}
	// 不同 SupervisionTargetType × 同一 metric 的状态分类
	for _, tt := range targets {
		cases = append(cases, tc{
			name:        fmt.Sprintf("target_type_%s/alert", tt),
			targetType:  tt,
			targetID:    "tgt-1",
			metricName:  model.SupervisionMetricRecallPrecision,
			value:       0.95,
			threshold:   0.8,
			sampleCount: 5,
			wantStatus:  model.SupervisionStatusAlert,
		})
		cases = append(cases, tc{
			name:        fmt.Sprintf("target_type_%s/warning", tt),
			targetType:  tt,
			targetID:    "tgt-2",
			metricName:  model.SupervisionMetricRecallPrecision,
			value:       0.7,
			threshold:   0.8,
			sampleCount: 3,
			wantStatus:  model.SupervisionStatusWarning,
		})
		cases = append(cases, tc{
			name:        fmt.Sprintf("target_type_%s/normal", tt),
			targetType:  tt,
			targetID:    "tgt-3",
			metricName:  model.SupervisionMetricRecallPrecision,
			value:       0.3,
			threshold:   0.8,
			sampleCount: 2,
			wantStatus:  model.SupervisionStatusNormal,
		})
	}
	// upsert 错误传播
	cases = append(cases, tc{
		name:        "upsert_error",
		targetType:  model.SupervisionTargetRAG,
		metricName:  model.SupervisionMetricRecallPrecision,
		value:       0.9,
		threshold:   0.8,
		sampleCount: 1,
		upsertErr:   errors.New("db upsert failed"),
		wantErr:     true,
	})

	if len(cases) < 60 {
		t.Fatalf("expected >=60 upsert cases, got %d", len(cases))
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			sr := &mockSignalRepo{upsertErr: c.upsertErr}
			svc := newSupervisorWithMocks(nil, sr, nil, nil)
			bucket := time.Now().Truncate(time.Hour)
			err := svc.upsertSignal(context.Background(), c.targetType, c.targetID, c.metricName, bucket, c.value, c.threshold, c.sampleCount, "trace-test")

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if err.Error() != c.upsertErr.Error() {
					t.Fatalf("err = %v, want %v", err, c.upsertErr)
				}
				if sr.upsertCalls != 1 {
					t.Errorf("upsertCalls = %d, want 1", sr.upsertCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sr.upsertCalls != 1 {
				t.Errorf("upsertCalls = %d, want 1", sr.upsertCalls)
			}
			if sr.lastSignal == nil {
				t.Fatalf("lastSignal is nil")
			}
			got := sr.lastSignal
			if got.Status != c.wantStatus {
				t.Errorf("Status = %v, want %v (value=%.4f threshold=%.4f sample=%d)",
					got.Status, c.wantStatus, c.value, c.threshold, c.sampleCount)
			}
			if got.Value != c.value {
				t.Errorf("Value = %v, want %v", got.Value, c.value)
			}
			if got.Threshold != c.threshold {
				t.Errorf("Threshold = %v, want %v", got.Threshold, c.threshold)
			}
			if got.SampleCount != int64(c.sampleCount) {
				t.Errorf("SampleCount = %d, want %d", got.SampleCount, c.sampleCount)
			}
			if got.MetricName != c.metricName {
				t.Errorf("MetricName = %v, want %v", got.MetricName, c.metricName)
			}
			if got.TargetType != c.targetType {
				t.Errorf("TargetType = %v, want %v", got.TargetType, c.targetType)
			}
			if got.TargetID != c.targetID {
				t.Errorf("TargetID = %v, want %v", got.TargetID, c.targetID)
			}
			if got.Baseline != 0.5 {
				t.Errorf("Baseline = %v, want 0.5", got.Baseline)
			}
			if len(got.TraceIDs) != 1 || got.TraceIDs[0] != "trace-test" {
				t.Errorf("TraceIDs = %v, want [trace-test]", got.TraceIDs)
			}
			if got.SignalID == "" {
				t.Errorf("SignalID is empty")
			}
		})
	}
}

// ============================================================================
// TestRAGSelfSupervisor_CollectMetrics
// 4 RAG 指标 + asset_effectiveness 采集路径
// ============================================================================

func TestRAGSelfSupervisor_CollectMetrics(t *testing.T) {
	type tc struct {
		name           string
		payload        *event.DialogueEndedPayload
		enableRAG      bool
		enableAsset    bool
		llmRate        float64
		llmDispatcher  *mockLLMDispatcher
		wantErr        bool
		wantUpsertMin  int // 至少多少次 upsert
		wantUpsertMax  int // 至多多少次 upsert
		wantRPrecision float64
		wantCoverage   float64
		wantAssetEff   float64
		wantFidelity   float64
		wantRelevance  float64
	}

	// 默认 payload（含 corpus + asset）
	makeFullPayload := func() *event.DialogueEndedPayload {
		return &event.DialogueEndedPayload{
			SessionID:        "sess-1",
			TraceID:          "trace-1",
			AggregatedReward: 1.0,
			UsedCorpusIDs:    []string{"c1", "c2"},
			UsedAssetIDs:     []string{"a1"},
			LastCustomerMsg:  "hello world product",
			LastAIReply:      "hello world product info",
			Outcome:          "converted",
		}
	}

	cases := []tc{
		{
			name: "nil_payload",
			payload: nil,
			enableRAG: true, enableAsset: true,
			wantErr: true,
		},
		{
			name: "rag_off_asset_off_no_calls",
			payload: makeFullPayload(),
			enableRAG: false, enableAsset: false,
			wantUpsertMin: 0, wantUpsertMax: 0,
		},
		{
			name: "rag_off_asset_on_with_assets",
			payload: makeFullPayload(),
			enableRAG: false, enableAsset: true,
			wantUpsertMin: 1, wantUpsertMax: 1,
			wantAssetEff: 0.85, // outcome=converted
		},
		{
			name: "rag_on_asset_off_no_corpus_reward_positive",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				AggregatedReward: 1.5,
			},
			enableRAG: true, enableAsset: false,
			wantUpsertMin: 1, wantUpsertMax: 1,
			wantRPrecision: 0.85,
		},
		{
			name: "rag_on_reward_negative",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				AggregatedReward: -1.0,
			},
			enableRAG: true, enableAsset: false,
			wantUpsertMin: 1, wantUpsertMax: 1,
			wantRPrecision: 0.4,
		},
		{
			name: "rag_on_reward_zero",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				AggregatedReward: 0.0,
			},
			enableRAG: true, enableAsset: false,
			wantUpsertMin: 1, wantUpsertMax: 1,
			wantRPrecision: 0.5,
		},
		{
			name: "rag_on_with_corpus_no_llm_judge",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedCorpusIDs:    []string{"c1"},
				LastCustomerMsg:  "product info",
				LastAIReply:      "product info here",
				AggregatedReward: 0,
			},
			enableRAG: true, enableAsset: false,
			llmRate: 0.0,
			wantUpsertMin: 2, wantUpsertMax: 2,
			wantRPrecision: 0.5,
			wantCoverage:   1.0, // "product" "info" 都在 AI 回复中
		},
		{
			name: "rag_on_with_corpus_llm_rate_1_dispatcher_set",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedCorpusIDs:    []string{"c1"},
				LastCustomerMsg:  "hello",
				LastAIReply:      "hello world",
				AggregatedReward: 0,
			},
			enableRAG: true, enableAsset: false,
			llmRate:       1.0,
			llmDispatcher: &mockLLMDispatcher{content: "0.85"},
			wantUpsertMin: 4, wantUpsertMax: 4,
			wantFidelity:  0.85,
			wantRelevance: 0.85,
		},
		{
			name: "rag_on_llm_rate_1_dispatcher_nil",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedCorpusIDs:    []string{"c1"},
				LastCustomerMsg:  "hello",
				LastAIReply:      "hello world",
				AggregatedReward: 0,
			},
			enableRAG: true, enableAsset: false,
			llmRate:       1.0,
			llmDispatcher: nil,
			wantUpsertMin: 2, wantUpsertMax: 2, // 仅 recall_precision + recall_coverage
		},
		{
			name: "rag_on_llm_rate_1_dispatcher_error_silent",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedCorpusIDs:    []string{"c1"},
				LastCustomerMsg:  "hello",
				LastAIReply:      "hello world",
				AggregatedReward: 0,
			},
			enableRAG: true, enableAsset: false,
			llmRate:       1.0,
			llmDispatcher: &mockLLMDispatcher{err: errors.New("llm down")},
			wantUpsertMin: 2, wantUpsertMax: 2, // LLM 失败静默吞掉
		},
		{
			name: "rag_on_llm_rate_1_dispatcher_garbage_content",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedCorpusIDs:    []string{"c1"},
				LastCustomerMsg:  "hello",
				LastAIReply:      "hello world",
				AggregatedReward: 0,
			},
			enableRAG: true, enableAsset: false,
			llmRate:       1.0,
			llmDispatcher: &mockLLMDispatcher{content: "not-a-number"},
			wantUpsertMin: 2, wantUpsertMax: 2, // parseScore 失败，跳过 upsert
		},
		{
			name: "asset_on_outcome_converted",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedAssetIDs: []string{"a1", "a2"},
				Outcome:      "converted",
			},
			enableRAG: false, enableAsset: true,
			wantUpsertMin: 2, wantUpsertMax: 2,
			wantAssetEff: 0.85,
		},
		{
			name: "asset_on_outcome_abandoned",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedAssetIDs: []string{"a1"},
				Outcome:      "abandoned",
			},
			enableRAG: false, enableAsset: true,
			wantUpsertMin: 1, wantUpsertMax: 1,
			wantAssetEff: 0.35,
		},
		{
			name: "asset_on_outcome_other",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedAssetIDs: []string{"a1"},
				Outcome:      "transferred",
			},
			enableRAG: false, enableAsset: true,
			wantUpsertMin: 1, wantUpsertMax: 1,
			wantAssetEff: 0.5,
		},
		{
			name: "asset_on_no_used_assets",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedAssetIDs: []string{},
				Outcome:      "converted",
			},
			enableRAG: false, enableAsset: true,
			wantUpsertMin: 0, wantUpsertMax: 0,
		},
		{
			name: "rag_on_asset_on_full_payload_with_llm",
			payload: makeFullPayload(),
			enableRAG: true, enableAsset: true,
			llmRate:       1.0,
			llmDispatcher: &mockLLMDispatcher{content: "0.9"},
			wantUpsertMin: 5, wantUpsertMax: 5, // recall_precision + recall_coverage + fidelity + relevance + 1 asset
			wantRPrecision: 0.85,
			wantFidelity:   0.9,
			wantRelevance:  0.9,
			wantAssetEff:   0.85,
		},
		{
			name: "rag_on_no_corpus_ids_skips_coverage",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedCorpusIDs:    nil,
				AggregatedReward: 1.0,
			},
			enableRAG: true, enableAsset: false,
			llmRate: 0.0,
			wantUpsertMin: 1, wantUpsertMax: 1, // 仅 recall_precision
			wantRPrecision: 0.85,
		},
		{
			name: "coverage_empty_customer_msg",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				UsedCorpusIDs:    []string{"c1"},
				LastCustomerMsg:  "",
				LastAIReply:      "hello",
				AggregatedReward: 0,
			},
			enableRAG: true, enableAsset: false,
			llmRate: 0.0,
			wantUpsertMin: 2, wantUpsertMax: 2,
			wantCoverage: 0,
		},
		{
			name: "upsert_error_does_not_propagate",
			payload: &event.DialogueEndedPayload{
				SessionID: "s", TraceID: "t",
				AggregatedReward: 1.0,
			},
			enableRAG: true, enableAsset: false,
			wantUpsertMin: 1, wantUpsertMax: 1, // upsert 仍被调用，但错误被吞掉
			wantErr: false,
		},
	}

	if len(cases) < 15 {
		t.Fatalf("expected >=15 collect metrics cases, got %d", len(cases))
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			snap := defaultSupervisorSnap()
			snap.EnableRAG = c.enableRAG
			snap.EnableAsset = c.enableAsset
			svc, _ := newSwitchSvcWithCache(snap)
			sr := &mockSignalRepo{upsertErr: nil}
			// 为了测试 upsert 错误路径，在子测试中单独处理
			if c.name == "upsert_error_does_not_propagate" {
				sr.upsertErr = errors.New("db error")
			}
			sup := newSupervisorWithMocks(svc, sr, c.llmDispatcher, nil)
			sup.SetLLMJudgeSampleRate(c.llmRate)

			err := sup.CollectMetrics(context.Background(), c.payload)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sr.upsertCalls < c.wantUpsertMin || sr.upsertCalls > c.wantUpsertMax {
				t.Errorf("upsertCalls = %d, want [%d, %d]", sr.upsertCalls, c.wantUpsertMin, c.wantUpsertMax)
			}
			// 校验具体指标值
			if c.wantRPrecision > 0 {
				sig := findSignalByMetric(sr.allSignals, model.SupervisionMetricRecallPrecision)
				if sig == nil {
					t.Errorf("recall_precision signal not captured")
				} else if sig.Value != c.wantRPrecision {
					t.Errorf("recall_precision value = %v, want %v", sig.Value, c.wantRPrecision)
				}
			}
			if c.wantCoverage > 0 {
				sig := findSignalByMetric(sr.allSignals, model.SupervisionMetricRecallCoverage)
				if sig == nil {
					t.Errorf("recall_coverage signal not captured")
				} else if sig.Value != c.wantCoverage {
					t.Errorf("recall_coverage value = %v, want %v", sig.Value, c.wantCoverage)
				}
			}
			if c.wantFidelity > 0 {
				sig := findSignalByMetric(sr.allSignals, model.SupervisionMetricGenerationFidelity)
				if sig == nil {
					t.Errorf("generation_fidelity signal not captured")
				} else if sig.Value != c.wantFidelity {
					t.Errorf("generation_fidelity value = %v, want %v", sig.Value, c.wantFidelity)
				}
			}
			if c.wantRelevance > 0 {
				sig := findSignalByMetric(sr.allSignals, model.SupervisionMetricAnswerRelevance)
				if sig == nil {
					t.Errorf("answer_relevance signal not captured")
				} else if sig.Value != c.wantRelevance {
					t.Errorf("answer_relevance value = %v, want %v", sig.Value, c.wantRelevance)
				}
			}
			if c.wantAssetEff > 0 {
				sig := findSignalByMetric(sr.allSignals, model.SupervisionMetricAssetEffectiveness)
				if sig == nil {
					t.Errorf("asset_effectiveness signal not captured")
				} else if sig.Value != c.wantAssetEff {
					t.Errorf("asset_effectiveness value = %v, want %v", sig.Value, c.wantAssetEff)
				}
			}
		})
	}

	// Switch GetStatus 错误传播
	t.Run("get_status_error", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("db down")
		sup := newSupervisorWithMocks(svc, &mockSignalRepo{}, nil, nil)
		err := sup.CollectMetrics(context.Background(), &event.DialogueEndedPayload{SessionID: "s"})
		if err == nil || err.Error() != "db down" {
			t.Fatalf("err = %v, want db down", err)
		}
	})
}

// ============================================================================
// TestRAGSelfSupervisor_ScanAlerts
// 告警扫描：ListAlerts 路径、状态过滤、目标类型过滤、dispatcher 调用
// ============================================================================

func TestRAGSelfSupervisor_ScanAlerts(t *testing.T) {
	type tc struct {
		name              string
		enableRAG         bool
		alerts            []*model.SelfSupervisionSignal
		listAlertsErr     error
		dispatcher        *SelfCorrectionDispatcher
		actionRepo        *mockActionRepo
		wantErr           bool
		wantDispatched    int
		wantListAlertsN   int
	}

	// 构造 alert 信号辅助函数
	mkAlert := func(targetType model.SupervisionTargetType, metric string, status model.SupervisionSignalStatus, signalID string) *model.SelfSupervisionSignal {
		return &model.SelfSupervisionSignal{
			SignalID:   signalID,
			TargetType: targetType,
			TargetID:   "tgt-" + signalID,
			MetricName: metric,
			Status:     status,
			Value:      0.9,
			Threshold:  0.8,
			BucketHour: time.Now(),
		}
	}

	// 基线 snap，autonomous + RAG on
	defaultSnap := func() *SwitchSnapshot {
		s := defaultSupervisorSnap()
		s.EnableRAG = true
		return s
	}

	cases := []tc{
		{
			name:        "rag_disabled_returns_zero",
			enableRAG:   false,
			alerts:      nil,
			wantDispatched: 0,
		},
		{
			name:          "empty_alerts",
			enableRAG:     true,
			alerts:        nil,
			wantDispatched: 0,
		},
		{
			name:          "list_alerts_error_propagates",
			enableRAG:     true,
			listAlertsErr: errors.New("list alerts db error"),
			wantErr:       true,
		},
		{
			name: "dispatcher_nil_no_dispatch",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusAlert, "s1"),
			},
			dispatcher: nil,
			wantDispatched: 0,
		},
		{
			name: "rag_alert_dispatched",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusAlert, "s1"),
			},
			dispatcher:     newDispatcherWithMocks(nil, &mockActionRepo{}), // 用 nil switchSvc 占位，下面会替换
			actionRepo:     &mockActionRepo{},
			wantDispatched: 1,
		},
		{
			name: "asset_alert_dispatched",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetAsset, model.SupervisionMetricAssetEffectiveness, model.SupervisionStatusAlert, "s2"),
			},
			actionRepo:     &mockActionRepo{},
			wantDispatched: 1,
		},
		{
			name: "llm_alert_skipped",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetLLM, model.SupervisionMetricGenerationFidelity, model.SupervisionStatusAlert, "s3"),
			},
			actionRepo:     &mockActionRepo{},
			wantDispatched: 0,
		},
		{
			name: "hybrid_alert_skipped",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetHybrid, model.SupervisionMetricAnswerRelevance, model.SupervisionStatusAlert, "s4"),
			},
			actionRepo:     &mockActionRepo{},
			wantDispatched: 0,
		},
		{
			name: "warning_status_skipped",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusWarning, "s5"),
			},
			actionRepo:     &mockActionRepo{},
			wantDispatched: 0,
		},
		{
			name: "normal_status_skipped",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusNormal, "s6"),
			},
			actionRepo:     &mockActionRepo{},
			wantDispatched: 0,
		},
		{
			name: "mixed_statuses_only_alert_dispatched",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusNormal, "n1"),
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusWarning, "w1"),
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusAlert, "a1"),
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusAlert, "a2"),
			},
			actionRepo:     &mockActionRepo{},
			wantDispatched: 2,
		},
		{
			name: "mixed_target_types_only_rag_asset_dispatched",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusAlert, "r1"),
				mkAlert(model.SupervisionTargetAsset, model.SupervisionMetricAssetEffectiveness, model.SupervisionStatusAlert, "a1"),
				mkAlert(model.SupervisionTargetLLM, model.SupervisionMetricGenerationFidelity, model.SupervisionStatusAlert, "l1"),
				mkAlert(model.SupervisionTargetHybrid, model.SupervisionMetricAnswerRelevance, model.SupervisionStatusAlert, "h1"),
			},
			actionRepo:     &mockActionRepo{},
			wantDispatched: 2,
		},
		{
			name: "dispatcher_error_continues_silently",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusAlert, "s1"),
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusAlert, "s2"),
			},
			actionRepo:     &mockActionRepo{createErr: errors.New("action create failed")},
			wantDispatched: 0,
		},
		{
			name: "asset_ab_converge_no_action_type_dispatch_error",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetAsset, model.SupervisionMetricAssetABConverge, model.SupervisionStatusAlert, "s1"),
			},
			actionRepo:     &mockActionRepo{},
			wantDispatched: 0, // lookupActionType 返回 ""，DispatchFromSignal 返回 error，跳过
		},
		{
			name: "multiple_alerts_partial_success",
			enableRAG: true,
			alerts: []*model.SelfSupervisionSignal{
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, model.SupervisionStatusAlert, "ok1"),
				mkAlert(model.SupervisionTargetAsset, model.SupervisionMetricAssetABConverge, model.SupervisionStatusAlert, "err1"),
				mkAlert(model.SupervisionTargetRAG, model.SupervisionMetricRecallCoverage, model.SupervisionStatusAlert, "ok2"),
			},
			actionRepo:     &mockActionRepo{},
			wantDispatched: 2,
		},
	}

	if len(cases) < 12 {
		t.Fatalf("expected >=12 scan alerts cases, got %d", len(cases))
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			snap := defaultSnap()
			snap.EnableRAG = c.enableRAG
			svc, _ := newSwitchSvcWithCache(snap)
			sr := &mockSignalRepo{
				alertsList:    c.alerts,
				listAlertsErr: c.listAlertsErr,
			}

			// 构造 dispatcher（如果需要）
			var dispatcher *SelfCorrectionDispatcher
			if c.actionRepo != nil {
				dispatcher = newDispatcherWithMocks(svc, c.actionRepo)
			} else if c.dispatcher != nil {
				dispatcher = c.dispatcher
			}

			sup := newSupervisorWithMocks(svc, sr, nil, dispatcher)
			count, err := sup.ScanAlerts(context.Background())

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if count != 0 {
					t.Errorf("count = %d, want 0 on error", count)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if count != c.wantDispatched {
				t.Errorf("dispatchedCount = %d, want %d", count, c.wantDispatched)
			}
			// RAG 禁用时 ScanAlerts 提前返回，不调用 ListAlerts
			if c.enableRAG && !c.wantErr {
				if sr.listAlertsCalls != 1 {
					t.Errorf("listAlertsCalls = %d, want 1", sr.listAlertsCalls)
				}
			} else if !c.enableRAG {
				if sr.listAlertsCalls != 0 {
					t.Errorf("listAlertsCalls = %d, want 0 (rag disabled)", sr.listAlertsCalls)
				}
			}
		})
	}

	// Switch GetStatus 错误传播
	t.Run("get_status_error", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("db down")
		sup := newSupervisorWithMocks(svc, &mockSignalRepo{}, nil, nil)
		count, err := sup.ScanAlerts(context.Background())
		if err == nil || err.Error() != "db down" {
			t.Fatalf("err = %v, want db down", err)
		}
		if count != 0 {
			t.Errorf("count = %d, want 0", count)
		}
	})
}

// ============================================================================
// TestRAGSelfSupervisor_LLMJudge_GenerationFidelity
// LLM-as-Judge 生成忠实度评估
// ============================================================================

func TestRAGSelfSupervisor_LLMJudge_GenerationFidelity(t *testing.T) {
	type tc struct {
		name        string
		dispatcher  *mockLLMDispatcher
		wantScore   float64
		wantErr     bool
		wantErrSub  string
	}

	cases := []tc{
		{name: "nil_dispatcher_returns_default", dispatcher: nil, wantScore: 0.5},
		{name: "llm_returns_0.85", dispatcher: &mockLLMDispatcher{content: "0.85"}, wantScore: 0.85},
		{name: "llm_returns_1.0", dispatcher: &mockLLMDispatcher{content: "1.0"}, wantScore: 1.0},
		{name: "llm_returns_0", dispatcher: &mockLLMDispatcher{content: "0"}, wantScore: 0.0},
		{name: "llm_returns_0.5", dispatcher: &mockLLMDispatcher{content: "0.5"}, wantScore: 0.5},
		{name: "llm_returns_1.5_clamped_to_1", dispatcher: &mockLLMDispatcher{content: "1.5"}, wantScore: 1.0},
		{name: "llm_returns_-0.5_clamped_to_0", dispatcher: &mockLLMDispatcher{content: "-0.5"}, wantScore: 0.0},
		{name: "llm_returns_garbage", dispatcher: &mockLLMDispatcher{content: "garbage"}, wantScore: 0.5, wantErr: true},
		{name: "llm_returns_empty", dispatcher: &mockLLMDispatcher{content: ""}, wantScore: 0.5, wantErr: true},
		{name: "llm_returns_whitespace_trimmed", dispatcher: &mockLLMDispatcher{content: "  0.7  "}, wantScore: 0.7},
		{name: "llm_returns_0.999", dispatcher: &mockLLMDispatcher{content: "0.999"}, wantScore: 0.999},
		{name: "llm_returns_1.001_clamped", dispatcher: &mockLLMDispatcher{content: "1.001"}, wantScore: 1.0},
		{name: "llm_dispatcher_error", dispatcher: &mockLLMDispatcher{err: errors.New("llm timeout")}, wantScore: 0.5, wantErr: true, wantErrSub: "llm timeout"},
	}

	if len(cases) < 10 {
		t.Fatalf("expected >=10 generation fidelity cases, got %d", len(cases))
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			sup := newSupervisorWithMocks(nil, &mockSignalRepo{}, c.dispatcher, nil)
			score, err := sup.judgeGenerationFidelity(context.Background(), "customer msg", "ai reply", []string{"c1"})

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if c.wantErrSub != "" && !strings.Contains(err.Error(), c.wantErrSub) {
					t.Errorf("err = %v, want contains %q", err, c.wantErrSub)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if score != c.wantScore {
				t.Errorf("score = %v, want %v", score, c.wantScore)
			}
			if c.dispatcher != nil {
				if c.dispatcher.dispatchCalls != 1 {
					t.Errorf("dispatchCalls = %d, want 1", c.dispatcher.dispatchCalls)
				}
				if c.dispatcher.lastScenario != "rag_supervision" {
					t.Errorf("lastScenario = %q, want rag_supervision", c.dispatcher.lastScenario)
				}
				if c.dispatcher.lastJSONMode != false {
					t.Errorf("lastJSONMode = %v, want false", c.dispatcher.lastJSONMode)
				}
				if c.dispatcher.lastMaxTokens != 100 {
					t.Errorf("lastMaxTokens = %d, want 100", c.dispatcher.lastMaxTokens)
				}
			}
		})
	}
}

// ============================================================================
// TestRAGSelfSupervisor_LLMJudge_AnswerRelevance
// LLM-as-Judge 答案相关性评估
// ============================================================================

func TestRAGSelfSupervisor_LLMJudge_AnswerRelevance(t *testing.T) {
	type tc struct {
		name       string
		dispatcher *mockLLMDispatcher
		wantScore  float64
		wantErr    bool
	}

	cases := []tc{
		{name: "nil_dispatcher_returns_default", dispatcher: nil, wantScore: 0.5},
		{name: "llm_returns_0.9", dispatcher: &mockLLMDispatcher{content: "0.9"}, wantScore: 0.9},
		{name: "llm_returns_1.0", dispatcher: &mockLLMDispatcher{content: "1.0"}, wantScore: 1.0},
		{name: "llm_returns_0", dispatcher: &mockLLMDispatcher{content: "0"}, wantScore: 0},
		{name: "llm_returns_1.2_clamped", dispatcher: &mockLLMDispatcher{content: "1.2"}, wantScore: 1.0},
		{name: "llm_returns_-0.3_clamped", dispatcher: &mockLLMDispatcher{content: "-0.3"}, wantScore: 0},
		{name: "llm_returns_garbage", dispatcher: &mockLLMDispatcher{content: "garbage"}, wantScore: 0.5, wantErr: true},
		{name: "llm_returns_empty", dispatcher: &mockLLMDispatcher{content: ""}, wantScore: 0.5, wantErr: true},
		{name: "llm_returns_whitespace", dispatcher: &mockLLMDispatcher{content: "  0.6  "}, wantScore: 0.6},
		{name: "llm_dispatcher_error", dispatcher: &mockLLMDispatcher{err: errors.New("network error")}, wantScore: 0.5, wantErr: true},
		{name: "llm_returns_0.55", dispatcher: &mockLLMDispatcher{content: "0.55"}, wantScore: 0.55},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			sup := newSupervisorWithMocks(nil, &mockSignalRepo{}, c.dispatcher, nil)
			score, err := sup.judgeAnswerRelevance(context.Background(), "customer msg", "ai reply")

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if score != c.wantScore {
				t.Errorf("score = %v, want %v", score, c.wantScore)
			}
			if c.dispatcher != nil && c.dispatcher.dispatchCalls != 1 {
				t.Errorf("dispatchCalls = %d, want 1", c.dispatcher.dispatchCalls)
			}
		})
	}
}

// ============================================================================
// TestRAGSelfSupervisor_GetDashboard
// 看板查询：3 种 range + 默认 + 聚合错误 + 告警错误
// ============================================================================

func TestRAGSelfSupervisor_GetDashboard(t *testing.T) {
	type tc struct {
		name          string
		rangeStr      string
		aggAvg        float64
		aggCount      int64
		aggErr        error
		alerts        []*model.SelfSupervisionSignal
		listAlertsErr error
		wantRange     string
		wantRAGCount  int // 期望 RAG 指标数（4 个）
		wantAssetCount int
		wantAlertCount int
	}

	cases := []tc{
		{
			name: "range_24h",
			rangeStr: "24h", aggAvg: 0.85, aggCount: 100,
			wantRange: "24h", wantRAGCount: 4, wantAssetCount: 1,
		},
		{
			name: "range_7d",
			rangeStr: "7d", aggAvg: 0.7, aggCount: 500,
			wantRange: "7d", wantRAGCount: 4, wantAssetCount: 1,
		},
		{
			name: "range_30d",
			rangeStr: "30d", aggAvg: 0.6, aggCount: 2000,
			wantRange: "30d", wantRAGCount: 4, wantAssetCount: 1,
		},
		{
			name: "range_empty_defaults_24h",
			rangeStr: "", aggAvg: 0.5, aggCount: 10,
			wantRange: "24h", wantRAGCount: 4, wantAssetCount: 1,
		},
		{
			name: "range_invalid_defaults_24h",
			rangeStr: "invalid", aggAvg: 0.5, aggCount: 10,
			wantRange: "24h", wantRAGCount: 4, wantAssetCount: 1,
		},
		{
			name: "aggregate_error_metrics_skipped",
			rangeStr: "24h", aggErr: errors.New("aggregate failed"),
			// RAG metrics 用 continue 跳过 → 0 个；Asset metrics 用 _ 忽略错误，仍 append → 1 个
			wantRange: "24h", wantRAGCount: 0, wantAssetCount: 1,
		},
		{
			name: "with_alerts_warning_and_critical",
			rangeStr: "24h", aggAvg: 0.7, aggCount: 10,
			alerts: []*model.SelfSupervisionSignal{
				{SignalID: "sig-1", MetricName: model.SupervisionMetricRecallPrecision, Status: model.SupervisionStatusWarning, Value: 0.7, Threshold: 0.8, TargetID: "rag-1", BucketHour: time.Now()},
				{SignalID: "sig-2", MetricName: model.SupervisionMetricRecallCoverage, Status: model.SupervisionStatusAlert, Value: 0.9, Threshold: 0.8, TargetID: "rag-2", BucketHour: time.Now()},
			},
			wantRange: "24h", wantRAGCount: 4, wantAssetCount: 1, wantAlertCount: 2,
		},
		{
			name: "list_alerts_error_returns_empty_alerts",
			rangeStr: "24h", aggAvg: 0.7, aggCount: 10,
			listAlertsErr: errors.New("list alerts failed"),
			wantRange: "24h", wantRAGCount: 4, wantAssetCount: 1, wantAlertCount: 0,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			snap := defaultSupervisorSnap()
			svc, _ := newSwitchSvcWithCache(snap)
			sr := &mockSignalRepo{
				aggAvg:        c.aggAvg,
				aggCount:      c.aggCount,
				aggErr:        c.aggErr,
				alertsList:    c.alerts,
				listAlertsErr: c.listAlertsErr,
			}
			sup := newSupervisorWithMocks(svc, sr, nil, nil)

			resp, err := sup.GetDashboard(context.Background(), c.rangeStr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Range != c.wantRange {
				t.Errorf("Range = %q, want %q", resp.Range, c.wantRange)
			}
			if len(resp.RAGMetrics) != c.wantRAGCount {
				t.Errorf("RAGMetrics count = %d, want %d", len(resp.RAGMetrics), c.wantRAGCount)
			}
			if len(resp.AssetMetrics) != c.wantAssetCount {
				t.Errorf("AssetMetrics count = %d, want %d", len(resp.AssetMetrics), c.wantAssetCount)
			}
			if len(resp.Alerts) != c.wantAlertCount {
				t.Errorf("Alerts count = %d, want %d", len(resp.Alerts), c.wantAlertCount)
			}
			// 校验告警 severity
			if c.wantAlertCount > 0 {
				for i, wantStatus := range []model.SupervisionSignalStatus{model.SupervisionStatusWarning, model.SupervisionStatusAlert} {
					if i >= len(resp.Alerts) {
						break
					}
					wantSeverity := "warning"
					if wantStatus == model.SupervisionStatusAlert {
						wantSeverity = "critical"
					}
					if resp.Alerts[i].Severity != wantSeverity {
						t.Errorf("Alerts[%d].Severity = %q, want %q", i, resp.Alerts[i].Severity, wantSeverity)
					}
				}
			}
		})
	}
}

// ============================================================================
// TestRAGSelfSupervisor_SetLLMJudgeSampleRate
// 采样比例设置：0/0.5/1.0 合法，负数 / >1 被忽略
// ============================================================================

func TestRAGSelfSupervisor_SetLLMJudgeSampleRate(t *testing.T) {
	type tc struct {
		name    string
		rate    float64
		want    float64
	}
	cases := []tc{
		{name: "zero", rate: 0.0, want: 0.0},
		{name: "half", rate: 0.5, want: 0.5},
		{name: "one", rate: 1.0, want: 1.0},
		{name: "negative_ignored", rate: -0.1, want: 0.1}, // 保持默认 0.1
		{name: "above_one_ignored", rate: 1.1, want: 0.1},
		{name: "two_ignored", rate: 2.0, want: 0.1},
		{name: "negative_one_ignored", rate: -1.0, want: 0.1},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			sup := newSupervisorWithMocks(nil, &mockSignalRepo{}, nil, nil)
		sup.SetLLMJudgeSampleRate(c.rate)
		if sup.GetLLMJudgeSampleRate() != c.want {
			t.Errorf("llmJudgeSampleRate = %v, want %v", sup.GetLLMJudgeSampleRate(), c.want)
		}
		})
	}
}

// ============================================================================
// TestRAGSelfSupervisor_parseScore
// parseScore 解析逻辑
// ============================================================================

func TestRAGSelfSupervisor_parseScore(t *testing.T) {
	type tc struct {
		name      string
		input     string
		wantScore float64
		wantErr   bool
	}
	cases := []tc{
		{name: "0.85", input: "0.85", wantScore: 0.85},
		{name: "1.0", input: "1.0", wantScore: 1.0},
		{name: "0", input: "0", wantScore: 0},
		{name: "0.5", input: "0.5", wantScore: 0.5},
		{name: "1.5_clamped", input: "1.5", wantScore: 1.0},
		{name: "negative_clamped", input: "-0.5", wantScore: 0},
		{name: "garbage", input: "garbage", wantScore: 0.5, wantErr: true},
		{name: "empty", input: "", wantScore: 0.5, wantErr: true},
		{name: "whitespace_trimmed", input: "  0.7  ", wantScore: 0.7},
		{name: "0.999", input: "0.999", wantScore: 0.999},
		{name: "1.001_clamped", input: "1.001", wantScore: 1.0},
		{name: "scientific_notation", input: "1e-1", wantScore: 0.1},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			score, err := parseScore(c.input)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if score != c.wantScore {
				t.Errorf("score = %v, want %v", score, c.wantScore)
			}
		})
	}
}

// ============================================================================
// TestRAGSelfSupervisor_computeCoverage
// 召回覆盖计算
// ============================================================================

func TestRAGSelfSupervisor_computeCoverage(t *testing.T) {
	type tc struct {
		name       string
		customer   string
		aiReply    string
		want       float64
	}
	cases := []tc{
		{name: "empty_customer", customer: "", aiReply: "hello", want: 0},
		{name: "empty_ai", customer: "hello", aiReply: "", want: 0},
		{name: "both_empty", customer: "", aiReply: "", want: 0},
		{name: "single_word_customer_skipped", customer: "a", aiReply: "a", want: 0}, // len<2 跳过
		{name: "full_match", customer: "product info", aiReply: "product info here", want: 1.0},
		{name: "partial_match", customer: "product info details", aiReply: "product info", want: 2.0 / 3.0},
		{name: "no_match", customer: "product info", aiReply: "completely different", want: 0},
		{name: "case_insensitive", customer: "Product Info", aiReply: "product info here", want: 1.0},
		{name: "punctuation_not_split", customer: "hello, world", aiReply: "hello world", want: 0.5}, // strings.Fields 只按空格分割，"hello," 带逗号不匹配，"world" 匹配 → 1/2
		{name: "long_customer_short_ai", customer: "alpha beta gamma delta epsilon", aiReply: "alpha beta", want: 2.0 / 5.0},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			sup := newSupervisorWithMocks(nil, &mockSignalRepo{}, nil, nil)
			got := sup.computeCoverage(c.customer, c.aiReply)
			if got != c.want {
				t.Errorf("computeCoverage(%q, %q) = %v, want %v", c.customer, c.aiReply, got, c.want)
			}
		})
	}
}

// ============================================================================
// TestRAGSelfSupervisor_GetThreshold
// 阈值获取：已知指标 / 未知指标
// ============================================================================

func TestRAGSelfSupervisor_GetThreshold(t *testing.T) {
	type tc struct {
		name       string
		metricName string
		want       float64
	}
	cases := []tc{
		{name: "recall_precision", metricName: model.SupervisionMetricRecallPrecision, want: 0.8},
		{name: "recall_coverage", metricName: model.SupervisionMetricRecallCoverage, want: 0.7},
		{name: "generation_fidelity", metricName: model.SupervisionMetricGenerationFidelity, want: 0.85},
		{name: "answer_relevance", metricName: model.SupervisionMetricAnswerRelevance, want: 0.8},
		{name: "asset_effectiveness", metricName: model.SupervisionMetricAssetEffectiveness, want: 0.6},
		{name: "unknown_metric_defaults_0.7", metricName: "unknown_metric", want: 0.7},
		{name: "empty_metric_defaults_0.7", metricName: "", want: 0.7},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			sup := newSupervisorWithMocks(nil, &mockSignalRepo{}, nil, nil)
			got := sup.getThreshold(c.metricName, defaultSupervisorSnap())
			if got != c.want {
				t.Errorf("getThreshold(%q) = %v, want %v", c.metricName, got, c.want)
			}
		})
	}
}

// ============================================================================
// TestRAGSelfSupervisor_metricDisplayName
// 指标中文名映射
// ============================================================================

func TestRAGSelfSupervisor_metricDisplayName(t *testing.T) {
	type tc struct {
		metric string
		want   string
	}
	cases := []tc{
		{model.SupervisionMetricRecallPrecision, "召回精度"},
		{model.SupervisionMetricRecallCoverage, "召回覆盖"},
		{model.SupervisionMetricGenerationFidelity, "生成忠实度"},
		{model.SupervisionMetricAnswerRelevance, "答案相关性"},
		{model.SupervisionMetricAssetEffectiveness, "资产包效能"},
		{"unknown_metric", "unknown_metric"},
		{"", ""},
	}

	for _, c := range cases {
		c := c
		t.Run(fmt.Sprintf("metric_%s", c.metric), func(t *testing.T) {
			got := metricDisplayName(c.metric)
			if got != c.want {
				t.Errorf("metricDisplayName(%q) = %q, want %q", c.metric, got, c.want)
			}
		})
	}
}

// ============================================================================
// TestRAGSelfSupervisor_ErrorPropagation
// 各类错误传播：Repo 错误 / LLMDispatcher 错误
// ============================================================================

func TestRAGSelfSupervisor_ErrorPropagation(t *testing.T) {
	t.Run("CollectMetrics_GetStatus_error", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("switch db down")
		sup := newSupervisorWithMocks(svc, &mockSignalRepo{}, nil, nil)
		err := sup.CollectMetrics(context.Background(), &event.DialogueEndedPayload{SessionID: "s"})
		if err == nil || err.Error() != "switch db down" {
			t.Fatalf("err = %v, want switch db down", err)
		}
	})

	t.Run("ScanAlerts_GetStatus_error", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("switch db down")
		sup := newSupervisorWithMocks(svc, &mockSignalRepo{}, nil, nil)
		count, err := sup.ScanAlerts(context.Background())
		if err == nil || err.Error() != "switch db down" {
			t.Fatalf("err = %v, want switch db down", err)
		}
		if count != 0 {
			t.Errorf("count = %d, want 0", count)
		}
	})

	t.Run("ScanAlerts_ListAlerts_error", func(t *testing.T) {
		svc, _ := newSwitchSvcWithCache(defaultSupervisorSnap())
		sr := &mockSignalRepo{listAlertsErr: errors.New("list alerts failed")}
		sup := newSupervisorWithMocks(svc, sr, nil, nil)
		count, err := sup.ScanAlerts(context.Background())
		if err == nil || err.Error() != "list alerts failed" {
			t.Fatalf("err = %v, want list alerts failed", err)
		}
		if count != 0 {
			t.Errorf("count = %d, want 0", count)
		}
	})

	t.Run("CollectMetrics_upsert_error_does_not_propagate", func(t *testing.T) {
		svc, _ := newSwitchSvcWithCache(defaultSupervisorSnap())
		sr := &mockSignalRepo{upsertErr: errors.New("upsert failed")}
		sup := newSupervisorWithMocks(svc, sr, nil, nil)
		err := sup.CollectMetrics(context.Background(), &event.DialogueEndedPayload{SessionID: "s", AggregatedReward: 1.0})
		// upsert 错误仅 log，不传播
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sr.upsertCalls == 0 {
			t.Errorf("upsertCalls = 0, want > 0")
		}
	})

	t.Run("LLMDispatcher_error_in_judgeGenerationFidelity", func(t *testing.T) {
		dispatcher := &mockLLMDispatcher{err: errors.New("llm unavailable")}
		sup := newSupervisorWithMocks(nil, &mockSignalRepo{}, dispatcher, nil)
		score, err := sup.judgeGenerationFidelity(context.Background(), "msg", "reply", []string{"c1"})
		if err == nil || err.Error() != "llm unavailable" {
			t.Fatalf("err = %v, want llm unavailable", err)
		}
		if score != 0.5 {
			t.Errorf("score = %v, want 0.5 on error", score)
		}
	})

	t.Run("LLMDispatcher_error_in_judgeAnswerRelevance", func(t *testing.T) {
		dispatcher := &mockLLMDispatcher{err: errors.New("llm unavailable")}
		sup := newSupervisorWithMocks(nil, &mockSignalRepo{}, dispatcher, nil)
		score, err := sup.judgeAnswerRelevance(context.Background(), "msg", "reply")
		if err == nil || err.Error() != "llm unavailable" {
			t.Fatalf("err = %v, want llm unavailable", err)
		}
		if score != 0.5 {
			t.Errorf("score = %v, want 0.5 on error", score)
		}
	})

	t.Run("GetDashboard_aggregate_error_does_not_propagate", func(t *testing.T) {
		svc, _ := newSwitchSvcWithCache(defaultSupervisorSnap())
		sr := &mockSignalRepo{aggErr: errors.New("aggregate failed")}
		sup := newSupervisorWithMocks(svc, sr, nil, nil)
		resp, err := sup.GetDashboard(context.Background(), "24h")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatalf("resp is nil")
		}
		// 聚合错误时 RAG metrics 应被跳过
		if len(resp.RAGMetrics) != 0 {
			t.Errorf("RAGMetrics count = %d, want 0 on aggregate error", len(resp.RAGMetrics))
		}
	})

	t.Run("GetDashboard_listAlerts_error_does_not_propagate", func(t *testing.T) {
		svc, _ := newSwitchSvcWithCache(defaultSupervisorSnap())
		sr := &mockSignalRepo{
			aggAvg: 0.7, aggCount: 10,
			listAlertsErr: errors.New("list alerts failed"),
		}
		sup := newSupervisorWithMocks(svc, sr, nil, nil)
		resp, err := sup.GetDashboard(context.Background(), "24h")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Alerts) != 0 {
			t.Errorf("Alerts count = %d, want 0 on listAlerts error", len(resp.Alerts))
		}
	})
}

// ============================================================================
// TestRAGSelfSupervisor_ShouldLLMJudge
// 采样逻辑：rate=0 → false, rate=1 → true
// ============================================================================

func TestRAGSelfSupervisor_ShouldLLMJudge(t *testing.T) {
	t.Run("rate_zero_always_false", func(t *testing.T) {
		sup := newSupervisorWithMocks(nil, &mockSignalRepo{}, nil, nil)
		sup.SetLLMJudgeSampleRate(0.0)
		for i := 0; i < 50; i++ {
			if sup.shouldLLMJudge() {
				t.Errorf("shouldLLMJudge returned true at iteration %d, want always false", i)
				break
			}
		}
	})
	t.Run("rate_one_always_true", func(t *testing.T) {
		sup := newSupervisorWithMocks(nil, &mockSignalRepo{}, nil, nil)
		sup.SetLLMJudgeSampleRate(1.0)
		for i := 0; i < 50; i++ {
			if !sup.shouldLLMJudge() {
				t.Errorf("shouldLLMJudge returned false at iteration %d, want always true", i)
				break
			}
		}
	})
}

// ============================================================================
// TestRAGSelfSupervisor_NewRAGSelfSupervisor_Defaults
// 构造函数默认值
// ============================================================================

func TestRAGSelfSupervisor_NewRAGSelfSupervisor_Defaults(t *testing.T) {
	sup := NewRAGSelfSupervisor(nil, &mockSignalRepo{}, &mockLogRepo{}, nil, nil)
	if sup.GetLLMJudgeSampleRate() != 0.1 {
		t.Errorf("default llmJudgeSampleRate = %v, want 0.1", sup.GetLLMJudgeSampleRate())
	}
	if len(sup.defaultThresholds) != 5 {
		t.Errorf("defaultThresholds len = %d, want 5", len(sup.defaultThresholds))
	}
	if sup.defaultThresholds[model.SupervisionMetricRecallPrecision] != 0.8 {
		t.Errorf("recall_precision threshold = %v, want 0.8", sup.defaultThresholds[model.SupervisionMetricRecallPrecision])
	}
	if sup.defaultThresholds[model.SupervisionMetricRecallCoverage] != 0.7 {
		t.Errorf("recall_coverage threshold = %v, want 0.7", sup.defaultThresholds[model.SupervisionMetricRecallCoverage])
	}
	if sup.defaultThresholds[model.SupervisionMetricGenerationFidelity] != 0.85 {
		t.Errorf("generation_fidelity threshold = %v, want 0.85", sup.defaultThresholds[model.SupervisionMetricGenerationFidelity])
	}
	if sup.defaultThresholds[model.SupervisionMetricAnswerRelevance] != 0.8 {
		t.Errorf("answer_relevance threshold = %v, want 0.8", sup.defaultThresholds[model.SupervisionMetricAnswerRelevance])
	}
	if sup.defaultThresholds[model.SupervisionMetricAssetEffectiveness] != 0.6 {
		t.Errorf("asset_effectiveness threshold = %v, want 0.6", sup.defaultThresholds[model.SupervisionMetricAssetEffectiveness])
	}
}
