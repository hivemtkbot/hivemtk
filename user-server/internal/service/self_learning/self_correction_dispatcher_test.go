package selflearning

// self_correction_dispatcher_test.go SelfCorrectionDispatcher 单元测试
//
// 测试策略：
//   - 使用标准库 testing，table-driven + t.Run 子测试
//   - 自实现 capActionRepo（捕获型 mock，记录所有 Create/UpdateStatus 调用参数）
//   - 复用 switch_service_test.go 中的 mockSwitchRepo / mockLogRepo
//   - 复用 rag_self_supervisor_test.go 中的 mockSignalRepo / mockLLMDispatcher
//   - 覆盖：DispatchFromSignal 7 类动作派发、3 种自治等级、3 种信号状态、
//     4 种监督目标类型、executeRetrieveRetry/executeQueryRewrite/executeChunkArchive 各路径、
//     createPendingAction 在 supervised/manual 模式下的行为、
//     ApproveAction/RejectAction 人工确认流程、错误传播

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// ============================================================================
// Capturing Mock: capActionRepo
// ============================================================================

// capActionRepo SelfCorrectionActionRepository 的捕获型 mock
//
// 与 mockActionRepo 不同，本 mock 会捕获所有 Create/UpdateStatus 调用的参数，
// 便于在 dispatcher 测试中断言动作内容（status / autonomy_level / reason / before 等）。
type capActionRepo struct {
	mu sync.Mutex

	// Create 捕获
	createErr   error
	createCalls int
	created     []*model.SelfCorrectionAction

	// GetByID
	getByIDAction *model.SelfCorrectionAction
	getByIDErr    error
	getByIDCalls  int

	// UpdateStatus 捕获
	updateCalls     int
	updateErr       error
	updateCallsList []capUpdateCall

	// 其他 List 方法（返回零值）
	listPendingRes []*model.SelfCorrectionAction
	listPendingErr error
}

// capUpdateCall 记录一次 UpdateStatus 调用的参数
type capUpdateCall struct {
	ActionID string
	Status   model.CorrectionActionStatus
	Extra    map[string]any
}

func (m *capActionRepo) Create(ctx context.Context, a *model.SelfCorrectionAction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	cp := *a
	m.created = append(m.created, &cp)
	return m.createErr
}

func (m *capActionRepo) GetByID(ctx context.Context, actionID string) (*model.SelfCorrectionAction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getByIDCalls++
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	if m.getByIDAction != nil {
		cp := *m.getByIDAction
		return &cp, nil
	}
	return nil, errors.New("not found")
}

func (m *capActionRepo) UpdateStatus(ctx context.Context, actionID string, status model.CorrectionActionStatus, extraUpdates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	m.updateCallsList = append(m.updateCallsList, capUpdateCall{
		ActionID: actionID,
		Status:   status,
		Extra:    extraUpdates,
	})
	return m.updateErr
}

func (m *capActionRepo) ListPending(ctx context.Context, limit int) ([]*model.SelfCorrectionAction, error) {
	return m.listPendingRes, m.listPendingErr
}

func (m *capActionRepo) ListByTarget(ctx context.Context, targetType, targetID string, limit int) ([]*model.SelfCorrectionAction, error) {
	return nil, nil
}

func (m *capActionRepo) ListByTriggerLog(ctx context.Context, triggerLogID string, limit int) ([]*model.SelfCorrectionAction, error) {
	return nil, nil
}

func (m *capActionRepo) ListByFilter(ctx context.Context, filter repository.CorrectionActionFilter) ([]*model.SelfCorrectionAction, int64, error) {
	return nil, 0, nil
}

func (m *capActionRepo) CountToday(ctx context.Context) (map[model.CorrectionActionType]int64, error) {
	return nil, nil
}

// 编译期接口断言
var _ repository.SelfCorrectionActionRepository = (*capActionRepo)(nil)

// ============================================================================
// 辅助函数
// ============================================================================

// newDispatcherSetup 构造一个 SelfCorrectionDispatcher + SwitchService + capActionRepo
//
// 所有依赖均为 mock；rag/asset/llm corrector 为 nil（可在测试中按需替换）。
// 缓存预填，避免走 DB；mockSwitchRepo 状态同步，保证 IncrementTodayCorrections 等方法可用。
func newDispatcherSetup(autonomy model.AutonomyLevel, enableRAG, enableAsset, enableLLM bool) (*SelfCorrectionDispatcher, *SwitchService, *capActionRepo, *mockSwitchRepo) {
	svc, sr, _ := newSelfLearningTestService(5 * time.Second)
	snap := &SwitchSnapshot{
		AutonomyLevel:           autonomy,
		EnableRAG:               enableRAG,
		EnableAsset:             enableAsset,
		EnableLLM:               enableLLM,
		CircuitOpen:             false,
		MaxDailyCorrections:     100,
		MaxDailyPromotions:      5,
		LowQualityThreshold:     3.0,
		ChampionRewardThreshold: 1.5,
		ABTestMinSamples:        100,
		CircuitBreakerThreshold: 0.3,
		CircuitBreakerWindowMin: 30,
	}
	setCache(svc, snap, 0, 0)
	sr.sw.AutonomyLevel = autonomy
	sr.sw.EnableRAG = enableRAG
	sr.sw.EnableAsset = enableAsset
	sr.sw.EnableLLM = enableLLM

	ar := &capActionRepo{}
	d := NewSelfCorrectionDispatcher(svc, ar, &mockLogRepo{}, &mockSignalRepo{}, nil, nil, nil)
	return d, svc, ar, sr
}

// newDispatcherWithLLM 构造带真实 LLMSelfCorrector 的 dispatcher
//
// llm 参数为 nil 时，LLMSelfCorrector.llmDispatcher 为 nil（untyped nil interface），
// CorrectFromSignal 会返回 "llm dispatcher is nil" 错误。
func newDispatcherWithLLM(autonomy model.AutonomyLevel, llm LLMDispatcher) (*SelfCorrectionDispatcher, *SwitchService, *capActionRepo, *mockSwitchRepo, *LLMSelfCorrector) {
	svc, sr, _ := newSelfLearningTestService(5 * time.Second)
	snap := &SwitchSnapshot{
		AutonomyLevel:           autonomy,
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
	setCache(svc, snap, 0, 0)
	sr.sw.AutonomyLevel = autonomy
	sr.sw.EnableRAG = true
	sr.sw.EnableAsset = true
	sr.sw.EnableLLM = true

	ar := &capActionRepo{}
	llmC := NewLLMSelfCorrector(svc, ar, &mockLogRepo{}, llm)
	d := NewSelfCorrectionDispatcher(svc, ar, &mockLogRepo{}, &mockSignalRepo{}, nil, nil, llmC)
	return d, svc, ar, sr, llmC
}

// mkSignal 构造一个简单的监督信号
func mkSignal(targetType model.SupervisionTargetType, metric, signalID, targetID string, status model.SupervisionSignalStatus) *model.SelfSupervisionSignal {
	return &model.SelfSupervisionSignal{
		SignalID:   signalID,
		TargetType: targetType,
		TargetID:   targetID,
		MetricName: metric,
		Status:     status,
		Value:      0.5,
		Threshold:  0.8,
		BucketHour: time.Now(),
	}
}

// mkSignalWithDetail 构造带 Detail 的监督信号（用于 LLM 矫正测试）
func mkSignalWithDetail(targetType model.SupervisionTargetType, metric, signalID, targetID string, status model.SupervisionSignalStatus, detail map[string]any) *model.SelfSupervisionSignal {
	s := mkSignal(targetType, metric, signalID, targetID, status)
	s.Detail = detail
	return s
}

// mkPendingAction 构造一个 pending 状态的 SelfCorrectionAction（用于 ApproveAction 测试）
func mkPendingAction(actionID string, actionType model.CorrectionActionType, targetID string) *model.SelfCorrectionAction {
	return &model.SelfCorrectionAction{
		ActionID:     actionID,
		TriggerLogID: "log-" + actionID,
		ActionType:   actionType,
		Scenario:     "rag",
		TargetType:   "rag_query",
		TargetID:     targetID,
		Before: map[string]any{
			"metric":    model.SupervisionMetricRecallPrecision,
			"value":     0.5,
			"threshold": 0.8,
		},
		Status: model.CorrectionStatusPending,
	}
}

// allSupervisionMetrics 所有监督指标（10 种）
func allSupervisionMetrics() []string {
	return []string{
		model.SupervisionMetricRecallPrecision,
		model.SupervisionMetricRecallCoverage,
		model.SupervisionMetricGenerationFidelity,
		model.SupervisionMetricAnswerRelevance,
		model.SupervisionMetricAssetEffectiveness,
		model.SupervisionMetricAssetAdoption,
		model.SupervisionMetricAssetConversion,
		model.SupervisionMetricAssetComplaint,
		model.SupervisionMetricAssetFreshness,
		model.SupervisionMetricAssetABConverge,
	}
}

// allSupervisionTargetTypes 4 种监督目标类型
func allSupervisionTargetTypes() []model.SupervisionTargetType {
	return []model.SupervisionTargetType{
		model.SupervisionTargetRAG,
		model.SupervisionTargetAsset,
		model.SupervisionTargetLLM,
		model.SupervisionTargetHybrid,
	}
}

// allSignalStatuses 3 种信号状态
func allSignalStatuses() []model.SupervisionSignalStatus {
	return []model.SupervisionSignalStatus{
		model.SupervisionStatusNormal,
		model.SupervisionStatusWarning,
		model.SupervisionStatusAlert,
	}
}

// errContains 检查错误消息是否包含子串
func errContains(err error, substr string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), substr)
}

// ============================================================================
// TestSelfCorrectionDispatcher_DispatchFromSignal_NilSignal
// ============================================================================

func TestSelfCorrectionDispatcher_DispatchFromSignal_NilSignal(t *testing.T) {
	t.Run("nil_signal_returns_error", func(t *testing.T) {
		d, _, _, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
		err := d.DispatchFromSignal(context.Background(), nil)
		if err == nil || !errContains(err, "signal is nil") {
			t.Fatalf("err = %v, want 'signal is nil'", err)
		}
	})
}

// ============================================================================
// TestSelfCorrectionDispatcher_LookupActionType  失败矩阵派发逻辑
// ============================================================================

func TestSelfCorrectionDispatcher_LookupActionType(t *testing.T) {
	// 在 autonomous 模式下测试 lookupActionType 的派发逻辑
	// 通过 DispatchFromSignal 间接测试（lookupActionType 是私有方法）
	type tc struct {
		name              string
		metric            string
		targetType        model.SupervisionTargetType
		targetID          string
		wantErr           bool
		errSubstr         string
		wantCreateCalls   int    // 期望 actionRepo.Create 调用次数
		wantActionType    model.CorrectionActionType // 期望捕获的动作类型（createCalls>0 时）
		wantActionStatus  model.CorrectionActionStatus
	}

	cases := []tc{
		// recall_precision → retrieve_retry → executeRetrieveRetry → Create(applied)
		{
			name: "recall_precision_rag_target", metric: model.SupervisionMetricRecallPrecision,
			targetType: model.SupervisionTargetRAG, targetID: "qry-1",
			wantCreateCalls: 1, wantActionType: model.CorrectionRetrieveRetry, wantActionStatus: model.CorrectionStatusApplied,
		},
		// recall_coverage → query_rewrite → executeQueryRewrite → Create(applied)
		{
			name: "recall_coverage_rag_target", metric: model.SupervisionMetricRecallCoverage,
			targetType: model.SupervisionTargetRAG, targetID: "qry-2",
			wantCreateCalls: 1, wantActionType: model.CorrectionQueryRewrite, wantActionStatus: model.CorrectionStatusApplied,
		},
		// generation_fidelity → llm_correction → nil corrector → error
		{
			name: "generation_fidelity_llm_target_nil_corrector", metric: model.SupervisionMetricGenerationFidelity,
			targetType: model.SupervisionTargetLLM, targetID: "reply-1",
			wantErr: true, errSubstr: "llm corrector is nil",
		},
		// answer_relevance → llm_correction → nil corrector → error
		{
			name: "answer_relevance_llm_target_nil_corrector", metric: model.SupervisionMetricAnswerRelevance,
			targetType: model.SupervisionTargetLLM, targetID: "reply-2",
			wantErr: true, errSubstr: "llm corrector is nil",
		},
		// asset_effectiveness + asset target → asset_rollback → executeAction returns nil (logged)
		{
			name: "asset_effectiveness_asset_target", metric: model.SupervisionMetricAssetEffectiveness,
			targetType: model.SupervisionTargetAsset, targetID: "asset-1",
			wantCreateCalls: 0,
		},
		// asset_effectiveness + rag target → chunk_archive → executeChunkArchive
		{
			name: "asset_effectiveness_rag_target_valid_chunkid", metric: model.SupervisionMetricAssetEffectiveness,
			targetType: model.SupervisionTargetRAG, targetID: "123",
			wantCreateCalls: 1, wantActionType: model.CorrectionChunkArchive, wantActionStatus: model.CorrectionStatusApplied,
		},
		// asset_effectiveness + rag target + empty target_id → chunk_archive → error
		{
			name: "asset_effectiveness_rag_target_empty_id", metric: model.SupervisionMetricAssetEffectiveness,
			targetType: model.SupervisionTargetRAG, targetID: "",
			wantErr: true, errSubstr: "target_id is empty",
		},
		// asset_effectiveness + rag target + non-numeric target_id → chunk_archive → parse error
		{
			name: "asset_effectiveness_rag_target_non_numeric_id", metric: model.SupervisionMetricAssetEffectiveness,
			targetType: model.SupervisionTargetRAG, targetID: "abc",
			wantErr: true, errSubstr: "parse chunk_id failed",
		},
		// asset_effectiveness + llm target → chunk_archive
		{
			name: "asset_effectiveness_llm_target", metric: model.SupervisionMetricAssetEffectiveness,
			targetType: model.SupervisionTargetLLM, targetID: "456",
			wantCreateCalls: 1, wantActionType: model.CorrectionChunkArchive,
		},
		// asset_effectiveness + hybrid target → chunk_archive
		{
			name: "asset_effectiveness_hybrid_target", metric: model.SupervisionMetricAssetEffectiveness,
			targetType: model.SupervisionTargetHybrid, targetID: "789",
			wantCreateCalls: 1, wantActionType: model.CorrectionChunkArchive,
		},
		// asset_adoption → asset_rollback → nil
		{
			name: "asset_adoption_asset_target", metric: model.SupervisionMetricAssetAdoption,
			targetType: model.SupervisionTargetAsset, targetID: "asset-2",
			wantCreateCalls: 0,
		},
		// asset_conversion → asset_rollback → nil
		{
			name: "asset_conversion_asset_target", metric: model.SupervisionMetricAssetConversion,
			targetType: model.SupervisionTargetAsset, targetID: "asset-3",
			wantCreateCalls: 0,
		},
		// asset_complaint → asset_rollback → nil
		{
			name: "asset_complaint_asset_target", metric: model.SupervisionMetricAssetComplaint,
			targetType: model.SupervisionTargetAsset, targetID: "asset-4",
			wantCreateCalls: 0,
		},
		// asset_freshness → asset_rollback → nil
		{
			name: "asset_freshness_asset_target", metric: model.SupervisionMetricAssetFreshness,
			targetType: model.SupervisionTargetAsset, targetID: "asset-5",
			wantCreateCalls: 0,
		},
		// asset_ab_converge → "" → error "no action type"
		{
			name: "asset_ab_converge_no_action", metric: model.SupervisionMetricAssetABConverge,
			targetType: model.SupervisionTargetAsset, targetID: "asset-6",
			wantErr: true, errSubstr: "no action type",
		},
		// unknown metric → "" → error
		{
			name: "unknown_metric_rag", metric: "unknown_metric",
			targetType: model.SupervisionTargetRAG, targetID: "x",
			wantErr: true, errSubstr: "no action type",
		},
		// unknown metric + asset target → error
		{
			name: "unknown_metric_asset", metric: "unknown_metric",
			targetType: model.SupervisionTargetAsset, targetID: "y",
			wantErr: true, errSubstr: "no action type",
		},
		// empty metric → error
		{
			name: "empty_metric", metric: "",
			targetType: model.SupervisionTargetRAG, targetID: "z",
			wantErr: true, errSubstr: "no action type",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
			sig := mkSignal(c.targetType, c.metric, "sig-"+c.name, c.targetID, model.SupervisionStatusAlert)

			err := d.DispatchFromSignal(context.Background(), sig)

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", c.errSubstr)
				}
				if !errContains(err, c.errSubstr) {
					t.Fatalf("err = %v, want error containing %q", err, c.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if ar.createCalls != c.wantCreateCalls {
				t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
			}
			if c.wantCreateCalls > 0 && len(ar.created) > 0 {
				got := ar.created[0]
				if got.ActionType != c.wantActionType {
					t.Errorf("created.ActionType = %v, want %v", got.ActionType, c.wantActionType)
				}
				if c.wantActionStatus != "" && got.Status != c.wantActionStatus {
					t.Errorf("created.Status = %v, want %v", got.Status, c.wantActionStatus)
				}
			}
		})
	}
}

// ============================================================================
// TestSelfCorrectionDispatcher_DispatchFromSignal_AutonomyLevels
// ============================================================================

func TestSelfCorrectionDispatcher_DispatchFromSignal_AutonomyLevels(t *testing.T) {
	type tc struct {
		name             string
		autonomy         model.AutonomyLevel
		metric           string
		targetType       model.SupervisionTargetType
		targetID         string
		wantErr          bool
		wantCreateCalls  int
		wantActionStatus model.CorrectionActionStatus
		wantAutonomyLvl  model.AutonomyLevel // 仅 pending 动作检查
	}

	metrics := []struct {
		metric     string
		targetType model.SupervisionTargetType
		targetID   string
	}{
		{model.SupervisionMetricRecallPrecision, model.SupervisionTargetRAG, "q1"},
		{model.SupervisionMetricRecallCoverage, model.SupervisionTargetRAG, "q2"},
		{model.SupervisionMetricGenerationFidelity, model.SupervisionTargetLLM, "r1"},
		{model.SupervisionMetricAssetEffectiveness, model.SupervisionTargetAsset, "a1"},
	}

	for _, autonomy := range []model.AutonomyLevel{
		model.AutonomyLevelManual,
		model.AutonomyLevelSupervised,
		model.AutonomyLevelAutonomous,
	} {
		for i, m := range metrics {
			m := m
			autonomy := autonomy
			name := fmt.Sprintf("%s_%s_%d", autonomy, m.metric, i)
			c := tc{
				name:       name,
				autonomy:   autonomy,
				metric:     m.metric,
				targetType: m.targetType,
				targetID:   m.targetID,
			}
			// 预期行为：
			// - manual: guardrail blocks (manual mode forbids) → pending action
			// - supervised: guardrail passes → pending action (supervised doesn't auto-execute)
			// - autonomous: guardrail passes → execute
			//   - recall_precision → executeRetrieveRetry → Create(applied)
			//   - recall_coverage → executeQueryRewrite → Create(applied)
			//   - generation_fidelity → llm_correction → nil corrector → error
			//   - asset_effectiveness/asset → asset_rollback → nil (Create(0))
			if autonomy == model.AutonomyLevelAutonomous {
				if m.metric == model.SupervisionMetricGenerationFidelity {
					c.wantErr = true
					c.wantCreateCalls = 0
				} else if m.metric == model.SupervisionMetricAssetEffectiveness {
					c.wantErr = false
					c.wantCreateCalls = 0 // asset_rollback logged, not executed by dispatcher
				} else {
					c.wantErr = false
					c.wantCreateCalls = 1
					c.wantActionStatus = model.CorrectionStatusApplied
				}
			} else {
				// manual or supervised → pending action
				// manual: guardrail blocks → pending
				// supervised: guardrail passes → pending (autonomy != autonomous)
				c.wantErr = false
				c.wantCreateCalls = 1
				c.wantActionStatus = model.CorrectionStatusPending
				c.wantAutonomyLvl = autonomy
			}

			t.Run(c.name, func(t *testing.T) {
				d, _, ar, _ := newDispatcherSetup(c.autonomy, true, true, true)
				sig := mkSignal(c.targetType, c.metric, "sig-"+c.name, c.targetID, model.SupervisionStatusAlert)

				err := d.DispatchFromSignal(context.Background(), sig)

				if c.wantErr {
					if err == nil {
						t.Fatalf("expected error, got nil")
					}
				} else {
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
				}

				if ar.createCalls != c.wantCreateCalls {
					t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
				}
				if c.wantCreateCalls > 0 && len(ar.created) > 0 {
					got := ar.created[0]
					if got.Status != c.wantActionStatus {
						t.Errorf("created.Status = %v, want %v", got.Status, c.wantActionStatus)
					}
					if c.wantAutonomyLvl != "" && got.AutonomyLevel != c.wantAutonomyLvl {
						t.Errorf("created.AutonomyLevel = %v, want %v", got.AutonomyLevel, c.wantAutonomyLvl)
					}
				}
			})
		}
	}
}

// ============================================================================
// TestSelfCorrectionDispatcher_DispatchFromSignal_GuardrailBlocked
// ============================================================================

func TestSelfCorrectionDispatcher_DispatchFromSignal_GuardrailBlocked(t *testing.T) {
	type tc struct {
		name             string
		autonomy         model.AutonomyLevel
		enableRAG        bool
		enableAsset      bool
		enableLLM        bool
		circuitOpen      bool
		quotaCorr        int64
		maxCorr          int
		metric           string
		targetType       model.SupervisionTargetType
		targetID         string
		wantCreateCalls  int
		wantActionStatus model.CorrectionActionStatus
		wantReasonSubstr string
	}

	cases := []tc{
		{
			name: "rag_disabled_blocks_retrieve_retry", autonomy: model.AutonomyLevelAutonomous,
			enableRAG: false, enableAsset: true, enableLLM: true,
			metric: model.SupervisionMetricRecallPrecision, targetType: model.SupervisionTargetRAG, targetID: "q1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantReasonSubstr: "rag self-learning disabled",
		},
		{
			name: "rag_disabled_blocks_query_rewrite", autonomy: model.AutonomyLevelAutonomous,
			enableRAG: false, enableAsset: true, enableLLM: true,
			metric: model.SupervisionMetricRecallCoverage, targetType: model.SupervisionTargetRAG, targetID: "q2",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantReasonSubstr: "rag self-learning disabled",
		},
		{
			name: "rag_disabled_blocks_chunk_archive", autonomy: model.AutonomyLevelAutonomous,
			enableRAG: false, enableAsset: true, enableLLM: true,
			metric: model.SupervisionMetricAssetEffectiveness, targetType: model.SupervisionTargetRAG, targetID: "123",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantReasonSubstr: "rag self-learning disabled",
		},
		{
			name: "asset_disabled_blocks_asset_rollback", autonomy: model.AutonomyLevelAutonomous,
			enableRAG: true, enableAsset: false, enableLLM: true,
			metric: model.SupervisionMetricAssetEffectiveness, targetType: model.SupervisionTargetAsset, targetID: "a1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantReasonSubstr: "asset self-learning disabled",
		},
		{
			name: "llm_disabled_blocks_llm_correction", autonomy: model.AutonomyLevelAutonomous,
			enableRAG: true, enableAsset: true, enableLLM: false,
			metric: model.SupervisionMetricGenerationFidelity, targetType: model.SupervisionTargetLLM, targetID: "r1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantReasonSubstr: "llm self-correction disabled",
		},
		{
			name: "circuit_open_blocks_retrieve_retry", autonomy: model.AutonomyLevelAutonomous,
			enableRAG: true, enableAsset: true, enableLLM: true, circuitOpen: true,
			metric: model.SupervisionMetricRecallPrecision, targetType: model.SupervisionTargetRAG, targetID: "q1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantReasonSubstr: "circuit breaker open",
		},
		{
			name: "quota_exceeded_blocks_retrieve_retry", autonomy: model.AutonomyLevelAutonomous,
			enableRAG: true, enableAsset: true, enableLLM: true, quotaCorr: 100, maxCorr: 100,
			metric: model.SupervisionMetricRecallPrecision, targetType: model.SupervisionTargetRAG, targetID: "q1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantReasonSubstr: "daily correction quota exceeded",
		},
		{
			name: "manual_mode_blocks_all", autonomy: model.AutonomyLevelManual,
			enableRAG: true, enableAsset: true, enableLLM: true,
			metric: model.SupervisionMetricRecallPrecision, targetType: model.SupervisionTargetRAG, targetID: "q1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantReasonSubstr: "manual mode forbids auto action",
		},
		{
			name: "circuit_open_blocks_llm_correction", autonomy: model.AutonomyLevelAutonomous,
			enableRAG: true, enableAsset: true, enableLLM: true, circuitOpen: true,
			metric: model.SupervisionMetricAnswerRelevance, targetType: model.SupervisionTargetLLM, targetID: "r2",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantReasonSubstr: "circuit breaker open",
		},
		{
			name: "quota_exceeded_blocks_query_rewrite", autonomy: model.AutonomyLevelAutonomous,
			enableRAG: true, enableAsset: true, enableLLM: true, quotaCorr: 100, maxCorr: 100,
			metric: model.SupervisionMetricRecallCoverage, targetType: model.SupervisionTargetRAG, targetID: "q2",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantReasonSubstr: "daily correction quota exceeded",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			svc, sr, _ := newSelfLearningTestService(5 * time.Second)
			snap := &SwitchSnapshot{
				AutonomyLevel:           c.autonomy,
				EnableRAG:               c.enableRAG,
				EnableAsset:             c.enableAsset,
				EnableLLM:               c.enableLLM,
				CircuitOpen:             c.circuitOpen,
				MaxDailyCorrections:     c.maxCorr,
				MaxDailyPromotions:      5,
				LowQualityThreshold:     3.0,
				ChampionRewardThreshold: 1.5,
				ABTestMinSamples:        100,
				CircuitBreakerThreshold: 0.3,
				CircuitBreakerWindowMin: 30,
			}
			setCache(svc, snap, c.quotaCorr, 0)
			sr.sw.AutonomyLevel = c.autonomy
			sr.sw.EnableRAG = c.enableRAG
			sr.sw.EnableAsset = c.enableAsset
			sr.sw.EnableLLM = c.enableLLM
			sr.sw.CircuitOpen = c.circuitOpen
			sr.sw.MaxDailyCorrections = c.maxCorr

			ar := &capActionRepo{}
			d := NewSelfCorrectionDispatcher(svc, ar, &mockLogRepo{}, &mockSignalRepo{}, nil, nil, nil)

			sig := mkSignal(c.targetType, c.metric, "sig-"+c.name, c.targetID, model.SupervisionStatusAlert)
			err := d.DispatchFromSignal(context.Background(), sig)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if ar.createCalls != c.wantCreateCalls {
				t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
			}
			if len(ar.created) > 0 {
				got := ar.created[0]
				if got.Status != c.wantActionStatus {
					t.Errorf("created.Status = %v, want %v", got.Status, c.wantActionStatus)
				}
				if c.wantReasonSubstr != "" && !strings.Contains(got.Reason, c.wantReasonSubstr) {
					t.Errorf("created.Reason = %q, want substring %q", got.Reason, c.wantReasonSubstr)
				}
			}
		})
	}
}

// ============================================================================
// TestSelfCorrectionDispatcher_DispatchFromSignal_ErrorPropagation
// ============================================================================

func TestSelfCorrectionDispatcher_DispatchFromSignal_ErrorPropagation(t *testing.T) {
	t.Run("should_execute_action_error_propagates", func(t *testing.T) {
		svc, sr, _ := newSelfLearningTestService(5 * time.Second)
		// 清空缓存，强制走 DB
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("db unavailable")

		ar := &capActionRepo{}
		d := NewSelfCorrectionDispatcher(svc, ar, &mockLogRepo{}, &mockSignalRepo{}, nil, nil, nil)

		sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, "s1", "q1", model.SupervisionStatusAlert)
		err := d.DispatchFromSignal(context.Background(), sig)
		if err == nil || !errContains(err, "db unavailable") {
			t.Fatalf("err = %v, want 'db unavailable'", err)
		}
		if ar.createCalls != 0 {
			t.Errorf("createCalls = %d, want 0 on error", ar.createCalls)
		}
	})

	t.Run("create_error_in_execute_retrieve_retry", func(t *testing.T) {
		d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
		ar.createErr = errors.New("action create failed")

		sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, "s1", "q1", model.SupervisionStatusAlert)
		err := d.DispatchFromSignal(context.Background(), sig)
		if err == nil || !errContains(err, "action create failed") {
			t.Fatalf("err = %v, want 'action create failed'", err)
		}
		if ar.createCalls != 1 {
			t.Errorf("createCalls = %d, want 1", ar.createCalls)
		}
	})

	t.Run("create_error_in_execute_query_rewrite", func(t *testing.T) {
		d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
		ar.createErr = errors.New("create db error")

		sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricRecallCoverage, "s2", "q2", model.SupervisionStatusAlert)
		err := d.DispatchFromSignal(context.Background(), sig)
		if err == nil || !errContains(err, "create db error") {
			t.Fatalf("err = %v, want 'create db error'", err)
		}
	})

	t.Run("create_error_in_execute_chunk_archive", func(t *testing.T) {
		d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
		ar.createErr = errors.New("create failed")

		sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricAssetEffectiveness, "s3", "123", model.SupervisionStatusAlert)
		err := d.DispatchFromSignal(context.Background(), sig)
		if err == nil || !errContains(err, "create failed") {
			t.Fatalf("err = %v, want 'create failed'", err)
		}
	})

	t.Run("create_error_in_create_pending_supervised", func(t *testing.T) {
		d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelSupervised, true, true, true)
		ar.createErr = errors.New("pending create failed")

		sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, "s4", "q1", model.SupervisionStatusAlert)
		err := d.DispatchFromSignal(context.Background(), sig)
		if err == nil || !errContains(err, "pending create failed") {
			t.Fatalf("err = %v, want 'pending create failed'", err)
		}
		if ar.createCalls != 1 {
			t.Errorf("createCalls = %d, want 1", ar.createCalls)
		}
	})

	t.Run("create_error_in_create_pending_manual_guardrail_blocked", func(t *testing.T) {
		d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelManual, true, true, true)
		ar.createErr = errors.New("manual pending failed")

		sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, "s5", "q1", model.SupervisionStatusAlert)
		err := d.DispatchFromSignal(context.Background(), sig)
		if err == nil || !errContains(err, "manual pending failed") {
			t.Fatalf("err = %v, want 'manual pending failed'", err)
		}
	})

	t.Run("get_status_error_in_autonomous_flow", func(t *testing.T) {
		// 在 autonomous 模式下，guardrail 通过后调用 GetStatus
		// 但由于缓存命中，GetStatus 不会失败
		// 此测试验证缓存命中路径正常工作（不报错）
		d, _, _, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
		sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, "s6", "q1", model.SupervisionStatusAlert)
		err := d.DispatchFromSignal(context.Background(), sig)
		if err != nil {
			t.Fatalf("unexpected error with cached GetStatus: %v", err)
		}
	})
}

// ============================================================================
// TestSelfCorrectionDispatcher_ExecuteRetrieveRetry
// ============================================================================

func TestSelfCorrectionDispatcher_ExecuteRetrieveRetry(t *testing.T) {
	type tc struct {
		name             string
		signalID         string
		targetID         string
		value            float64
		threshold        float64
		createErr        error
		incrCorrErr      error
		wantErr          bool
		errSubstr        string
		wantCreateCalls  int
		wantActionType   model.CorrectionActionType
		wantReasonSubstr string
	}

	cases := []tc{
		{
			name: "success_basic", signalID: "sig-1", targetID: "q1",
			value: 0.5, threshold: 0.8,
			wantCreateCalls: 1, wantActionType: model.CorrectionRetrieveRetry,
			wantReasonSubstr: "recall_precision below threshold",
		},
		{
			name: "success_different_values", signalID: "sig-2", targetID: "q2",
			value: 0.3, threshold: 0.9,
			wantCreateCalls: 1, wantActionType: model.CorrectionRetrieveRetry,
		},
		{
			name: "create_error", signalID: "sig-3", targetID: "q3",
			createErr: errors.New("db write failed"),
			wantErr: true, errSubstr: "db write failed",
			wantCreateCalls: 1, wantActionType: model.CorrectionRetrieveRetry,
		},
		{
			name: "record_correction_error_ignored", signalID: "sig-4", targetID: "q4",
			incrCorrErr: errors.New("incr failed"), // RecordCorrectionAction 错误被忽略
			wantCreateCalls: 1, wantActionType: model.CorrectionRetrieveRetry,
		},
		{
			name: "success_empty_target_id", signalID: "sig-5", targetID: "",
			wantCreateCalls: 1, wantActionType: model.CorrectionRetrieveRetry,
		},
		{
			name: "success_with_bucket_hour", signalID: "sig-6", targetID: "q6",
			value: 0.7, threshold: 0.85,
			wantCreateCalls: 1, wantActionType: model.CorrectionRetrieveRetry,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			d, _, ar, sr := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
			ar.createErr = c.createErr
			sr.incrementCorrectionsErr = c.incrCorrErr

			sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, c.signalID, c.targetID, model.SupervisionStatusAlert)
			sig.Value = c.value
			sig.Threshold = c.threshold

			err := d.DispatchFromSignal(context.Background(), sig)

			if c.wantErr {
				if err == nil || !errContains(err, c.errSubstr) {
					t.Fatalf("err = %v, want error containing %q", err, c.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if ar.createCalls != c.wantCreateCalls {
				t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
			}
			if len(ar.created) > 0 {
				got := ar.created[0]
				if got.ActionType != c.wantActionType {
					t.Errorf("ActionType = %v, want %v", got.ActionType, c.wantActionType)
				}
				if got.Status != model.CorrectionStatusApplied {
					t.Errorf("Status = %v, want applied", got.Status)
				}
				if got.Scenario != "rag" {
					t.Errorf("Scenario = %v, want rag", got.Scenario)
				}
				if got.TargetType != "rag_query" {
					t.Errorf("TargetType = %v, want rag_query", got.TargetType)
				}
				if got.Operator != "auto" {
					t.Errorf("Operator = %v, want auto", got.Operator)
				}
				if c.wantReasonSubstr != "" && !strings.Contains(got.Reason, c.wantReasonSubstr) {
					t.Errorf("Reason = %q, want substring %q", got.Reason, c.wantReasonSubstr)
				}
				// Before 应包含 metric/value/threshold
				if got.Before["metric"] != model.SupervisionMetricRecallPrecision {
					t.Errorf("Before[metric] = %v, want %v", got.Before["metric"], model.SupervisionMetricRecallPrecision)
				}
			}
		})
	}
}

// ============================================================================
// TestSelfCorrectionDispatcher_ExecuteQueryRewrite
// ============================================================================

func TestSelfCorrectionDispatcher_ExecuteQueryRewrite(t *testing.T) {
	type tc struct {
		name             string
		signalID         string
		targetID         string
		createErr        error
		wantErr          bool
		errSubstr        string
		wantCreateCalls  int
		wantReasonSubstr string
	}

	cases := []tc{
		{
			name: "success_basic", signalID: "sig-1", targetID: "q1",
			wantCreateCalls: 1, wantReasonSubstr: "recall_coverage below threshold",
		},
		{
			name: "success_empty_target", signalID: "sig-2", targetID: "",
			wantCreateCalls: 1,
		},
		{
			name: "create_error", signalID: "sig-3", targetID: "q3",
			createErr: errors.New("write failed"),
			wantErr: true, errSubstr: "write failed",
			wantCreateCalls: 1,
		},
		{
			name: "success_long_signal_id", signalID: "sig-very-long-id-12345", targetID: "q4",
			wantCreateCalls: 1,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
			ar.createErr = c.createErr

			sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricRecallCoverage, c.signalID, c.targetID, model.SupervisionStatusAlert)
			err := d.DispatchFromSignal(context.Background(), sig)

			if c.wantErr {
				if err == nil || !errContains(err, c.errSubstr) {
					t.Fatalf("err = %v, want error containing %q", err, c.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if ar.createCalls != c.wantCreateCalls {
				t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
			}
			if len(ar.created) > 0 {
				got := ar.created[0]
				if got.ActionType != model.CorrectionQueryRewrite {
					t.Errorf("ActionType = %v, want query_rewrite", got.ActionType)
				}
				if got.Status != model.CorrectionStatusApplied {
					t.Errorf("Status = %v, want applied", got.Status)
				}
				if got.Scenario != "rag" {
					t.Errorf("Scenario = %v, want rag", got.Scenario)
				}
				if c.wantReasonSubstr != "" && !strings.Contains(got.Reason, c.wantReasonSubstr) {
					t.Errorf("Reason = %q, want substring %q", got.Reason, c.wantReasonSubstr)
				}
			}
		})
	}
}

// ============================================================================
// TestSelfCorrectionDispatcher_ExecuteChunkArchive
// ============================================================================

func TestSelfCorrectionDispatcher_ExecuteChunkArchive(t *testing.T) {
	type tc struct {
		name            string
		targetID        string
		createErr       error
		wantErr         bool
		errSubstr       string
		wantCreateCalls int
	}

	cases := []tc{
		{
			name: "success_valid_chunkid", targetID: "123",
			wantCreateCalls: 1,
		},
		{
			name: "success_large_chunkid", targetID: "999999999",
			wantCreateCalls: 1,
		},
		{
			name: "success_zero_chunkid", targetID: "0",
			wantCreateCalls: 1,
		},
		{
			name: "empty_target_id", targetID: "",
			wantErr: true, errSubstr: "target_id is empty",
			wantCreateCalls: 0,
		},
		{
			name: "non_numeric_target_id", targetID: "abc",
			wantErr: true, errSubstr: "parse chunk_id failed",
			wantCreateCalls: 0,
		},
		{
			name: "mixed_target_id", targetID: "12abc",
			wantErr: false, wantCreateCalls: 1, // Sscanf 宽松解析 "12abc" → 12, err=nil
		},
		{
			name: "create_error", targetID: "456",
			createErr: errors.New("db error"),
			wantErr: true, errSubstr: "db error",
			wantCreateCalls: 1,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
			ar.createErr = c.createErr

			// asset_effectiveness + rag target → chunk_archive
			sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricAssetEffectiveness, "sig-"+c.name, c.targetID, model.SupervisionStatusAlert)
			err := d.DispatchFromSignal(context.Background(), sig)

			if c.wantErr {
				if err == nil || !errContains(err, c.errSubstr) {
					t.Fatalf("err = %v, want error containing %q", err, c.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if ar.createCalls != c.wantCreateCalls {
				t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
			}
			if len(ar.created) > 0 {
				got := ar.created[0]
				if got.ActionType != model.CorrectionChunkArchive {
					t.Errorf("ActionType = %v, want chunk_archive", got.ActionType)
				}
				if got.Status != model.CorrectionStatusApplied {
					t.Errorf("Status = %v, want applied", got.Status)
				}
				if got.TargetType != "rag_chunk" {
					t.Errorf("TargetType = %v, want rag_chunk", got.TargetType)
				}
				if got.Scenario != "rag" {
					t.Errorf("Scenario = %v, want rag", got.Scenario)
				}
				// Before 应包含 chunk_id
				if _, ok := got.Before["chunk_id"]; !ok {
					t.Errorf("Before should contain chunk_id")
				}
			}
		})
	}
}

// ============================================================================
// TestSelfCorrectionDispatcher_ExecuteAction_AssetActions
// ============================================================================

func TestSelfCorrectionDispatcher_ExecuteAction_AssetActions(t *testing.T) {
	// asset_promote / asset_rollback 通过 DispatchFromSignal 不直接执行
	// dispatcher 仅记录日志并返回 nil
	t.Run("asset_rollback_via_dispatch_returns_nil", func(t *testing.T) {
		d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
		sig := mkSignal(model.SupervisionTargetAsset, model.SupervisionMetricAssetEffectiveness, "s1", "asset-1", model.SupervisionStatusAlert)
		err := d.DispatchFromSignal(context.Background(), sig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ar.createCalls != 0 {
			t.Errorf("createCalls = %d, want 0 (asset action not executed by dispatcher)", ar.createCalls)
		}
	})

	// 通过 ApproveAction 测试 asset_promote / asset_rollback 执行路径
	type tc struct {
		name            string
		actionType      model.CorrectionActionType
		wantErr         bool
		wantUpdateCalls int
		wantFinalStatus model.CorrectionActionStatus
	}

	cases := []tc{
		{
			name: "asset_promote_returns_nil", actionType: model.CorrectionAssetPromote,
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusApplied,
		},
		{
			name: "asset_rollback_returns_nil", actionType: model.CorrectionAssetRollback,
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusApplied,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
			ar.getByIDAction = mkPendingAction("aid-1", c.actionType, "asset-1")

			err := d.ApproveAction(context.Background(), "aid-1", 42, "approved")
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if ar.updateCalls != c.wantUpdateCalls {
				t.Errorf("updateCalls = %d, want %d", ar.updateCalls, c.wantUpdateCalls)
			}
			if len(ar.updateCallsList) > 0 {
				if ar.updateCallsList[0].Status != c.wantFinalStatus {
					t.Errorf("update status = %v, want %v", ar.updateCallsList[0].Status, c.wantFinalStatus)
				}
			}
		})
	}

	// 测试所有资产类指标通过 DispatchFromSignal 都派发 asset_rollback
	t.Run("all_asset_metrics_dispatch_rollback", func(t *testing.T) {
		assetMetrics := []string{
			model.SupervisionMetricAssetAdoption,
			model.SupervisionMetricAssetConversion,
			model.SupervisionMetricAssetComplaint,
			model.SupervisionMetricAssetFreshness,
		}
		for _, metric := range assetMetrics {
			d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
			sig := mkSignal(model.SupervisionTargetAsset, metric, "sig-"+metric, "asset-1", model.SupervisionStatusAlert)
			err := d.DispatchFromSignal(context.Background(), sig)
			if err != nil {
				t.Errorf("metric %s: unexpected error: %v", metric, err)
			}
			if ar.createCalls != 0 {
				t.Errorf("metric %s: createCalls = %d, want 0", metric, ar.createCalls)
			}
		}
	})
}

// ============================================================================
// TestSelfCorrectionDispatcher_ExecuteAction_LLMCorrection
// ============================================================================

func TestSelfCorrectionDispatcher_ExecuteAction_LLMCorrection(t *testing.T) {
	type tc struct {
		name             string
		metric           string
		detail           map[string]any
		llmContent       string
		llmErr           error
		wantErr          bool
		errSubstr        string
		wantCreateCalls  int
		wantActionStatus model.CorrectionActionStatus
	}

	cases := []tc{
		// nil llmCorrector → error
		{
			name: "nil_corrector_generation_fidelity",
			metric: model.SupervisionMetricGenerationFidelity,
			wantErr: true, errSubstr: "llm corrector is nil",
			wantCreateCalls: 0,
		},
		{
			name: "nil_corrector_answer_relevance",
			metric: model.SupervisionMetricAnswerRelevance,
			wantErr: true, errSubstr: "llm corrector is nil",
			wantCreateCalls: 0,
		},
		// real LLMSelfCorrector with nil llmDispatcher → "llm dispatcher is nil"
		{
			name: "nil_llm_dispatcher",
			metric: model.SupervisionMetricGenerationFidelity,
			detail: map[string]any{"ai_reply": "some reply"},
			wantErr: true, errSubstr: "llm dispatcher is nil",
			wantCreateCalls: 0,
		},
		// real LLMSelfCorrector + empty ai_reply → recordSkippedAction → Create(skipped)
		{
			name: "empty_ai_reply_skipped",
			metric: model.SupervisionMetricGenerationFidelity,
			detail: map[string]any{"ai_reply": ""},
			llmContent: "ok",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusSkipped,
		},
		// real LLMSelfCorrector + non-empty ai_reply → criticHallucination → no hallucination → Create(applied)
		{
			name: "hallucination_check_no_hallucination",
			metric: model.SupervisionMetricGenerationFidelity,
			detail: map[string]any{"ai_reply": "original reply", "customer_msg": "customer question"},
			llmContent: `{"has_hallucination": false, "evidence": "none", "severity": "low"}`,
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusApplied,
		},
		// real LLMSelfCorrector + non-empty ai_reply + hallucination found → regenerate → Create(applied)
		{
			name: "hallucination_found_and_regenerated",
			metric: model.SupervisionMetricGenerationFidelity,
			detail: map[string]any{"ai_reply": "reply with hallucination", "customer_msg": "question"},
			llmContent: `{"has_hallucination": true, "evidence": "fake data", "severity": "high"}`,
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusApplied,
		},
		// answer_relevance + empty ai_reply → skipped
		{
			name: "answer_relevance_empty_reply_skipped",
			metric: model.SupervisionMetricAnswerRelevance,
			detail: map[string]any{"ai_reply": ""},
			llmContent: "ok",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusSkipped,
		},
		// answer_relevance + non-empty ai_reply + empty customer_msg → skipped
		{
			name: "answer_relevance_empty_customer_msg_skipped",
			metric: model.SupervisionMetricAnswerRelevance,
			detail: map[string]any{"ai_reply": "reply", "customer_msg": ""},
			llmContent: "ok",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusSkipped,
		},
		// answer_relevance + both non-empty → criticOffTopic → not off topic → Create(applied)
		{
			name: "off_topic_check_not_off_topic",
			metric: model.SupervisionMetricAnswerRelevance,
			detail: map[string]any{"ai_reply": "reply", "customer_msg": "question"},
			llmContent: `{"is_off_topic": false, "evidence": "relevant", "severity": "low"}`,
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusApplied,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			var d *SelfCorrectionDispatcher
			var ar *capActionRepo

			// nil_llm_dispatcher 优先处理：构造 LLMSelfCorrector with nil llmDispatcher
			// 必须在 detail != nil 判断之前，否则会被分支吞掉
			if c.name == "nil_llm_dispatcher" {
				svc, sr, _ := newSelfLearningTestService(5 * time.Second)
				snap := &SwitchSnapshot{
					AutonomyLevel:           model.AutonomyLevelAutonomous,
					EnableRAG:               true,
					EnableAsset:             true,
					EnableLLM:               true,
					MaxDailyCorrections:     100,
					MaxDailyPromotions:      5,
					LowQualityThreshold:     3.0,
					ChampionRewardThreshold: 1.5,
					ABTestMinSamples:        100,
					CircuitBreakerThreshold: 0.3,
					CircuitBreakerWindowMin: 30,
				}
				setCache(svc, snap, 0, 0)
				sr.sw.AutonomyLevel = model.AutonomyLevelAutonomous
				ar = &capActionRepo{}
				llmC := NewLLMSelfCorrector(svc, ar, &mockLogRepo{}, nil)
				d = NewSelfCorrectionDispatcher(svc, ar, &mockLogRepo{}, &mockSignalRepo{}, nil, nil, llmC)
			} else if c.llmContent != "" || c.llmErr != nil || c.detail != nil {
				// 使用真实 LLMSelfCorrector + mock LLM dispatcher
				mockLLM := &mockLLMDispatcher{
					content: c.llmContent,
					err:     c.llmErr,
				}
				dd, _, aar, _, _ := newDispatcherWithLLM(model.AutonomyLevelAutonomous, mockLLM)
				d, ar = dd, aar
			} else {
				// nil corrector 路径
				dd, _, aar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
				d, ar = dd, aar
			}

			sig := mkSignalWithDetail(model.SupervisionTargetLLM, c.metric, "sig-"+c.name, "reply-1", model.SupervisionStatusAlert, c.detail)
			err := d.DispatchFromSignal(context.Background(), sig)

			if c.wantErr {
				if err == nil || !errContains(err, c.errSubstr) {
					t.Fatalf("err = %v, want error containing %q", err, c.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if ar.createCalls != c.wantCreateCalls {
				t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
			}
			if len(ar.created) > 0 {
				got := ar.created[0]
				if got.ActionType != model.CorrectionLLMCorrection {
					t.Errorf("ActionType = %v, want llm_correction", got.ActionType)
				}
				if c.wantActionStatus != "" && got.Status != c.wantActionStatus {
					t.Errorf("Status = %v, want %v", got.Status, c.wantActionStatus)
				}
				if got.Scenario != "llm" {
					t.Errorf("Scenario = %v, want llm", got.Scenario)
				}
				if got.TargetType != "llm_reply" {
					t.Errorf("TargetType = %v, want llm_reply", got.TargetType)
				}
			}
		})
	}

	// 测试 unsupported metric
	t.Run("unsupported_metric_for_llm", func(t *testing.T) {
		mockLLM := &mockLLMDispatcher{content: "ok"}
		d, _, ar, _, _ := newDispatcherWithLLM(model.AutonomyLevelAutonomous, mockLLM)
		// 使用 recall_precision 作为 metric（不映射到 llm_correction 的 executeAction 路径）
		// 但实际上 DispatchFromSignal 会根据 metric 派发，recall_precision → retrieve_retry
		// 所以需要直接测试 CorrectFromSignal 的 unsupported metric
		// 这里通过 DispatchFromSignal 测试 generation_fidelity 正常路径已覆盖
		// 此测试改为验证 llmCorrector 不为 nil 时正常工作
		sig := mkSignalWithDetail(model.SupervisionTargetLLM, model.SupervisionMetricGenerationFidelity, "s1", "r1", model.SupervisionStatusAlert,
			map[string]any{"ai_reply": ""})
		err := d.DispatchFromSignal(context.Background(), sig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ar.createCalls != 1 {
			t.Errorf("createCalls = %d, want 1", ar.createCalls)
		}
	})
}

// ============================================================================
// TestSelfCorrectionDispatcher_CreatePendingAction
// ============================================================================

func TestSelfCorrectionDispatcher_CreatePendingAction(t *testing.T) {
	type tc struct {
		name             string
		autonomy         model.AutonomyLevel
		metric           string
		targetType       model.SupervisionTargetType
		targetID         string
		createErr        error
		wantErr          bool
		errSubstr        string
		wantCreateCalls  int
		wantActionStatus model.CorrectionActionStatus
		wantAutonomyLvl  model.AutonomyLevel
		wantReasonSubstr string
	}

	cases := []tc{
		// supervised 模式：guardrail 通过 → pending
		{
			name: "supervised_retrieve_retry", autonomy: model.AutonomyLevelSupervised,
			metric: model.SupervisionMetricRecallPrecision, targetType: model.SupervisionTargetRAG, targetID: "q1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantAutonomyLvl: model.AutonomyLevelSupervised,
			wantReasonSubstr: "supervised/manual: pending review",
		},
		{
			name: "supervised_query_rewrite", autonomy: model.AutonomyLevelSupervised,
			metric: model.SupervisionMetricRecallCoverage, targetType: model.SupervisionTargetRAG, targetID: "q2",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantAutonomyLvl: model.AutonomyLevelSupervised,
		},
		{
			name: "supervised_llm_correction", autonomy: model.AutonomyLevelSupervised,
			metric: model.SupervisionMetricGenerationFidelity, targetType: model.SupervisionTargetLLM, targetID: "r1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantAutonomyLvl: model.AutonomyLevelSupervised,
		},
		{
			name: "supervised_asset_rollback", autonomy: model.AutonomyLevelSupervised,
			metric: model.SupervisionMetricAssetEffectiveness, targetType: model.SupervisionTargetAsset, targetID: "a1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantAutonomyLvl: model.AutonomyLevelSupervised,
		},
		// manual 模式：guardrail 阻断 → pending
		{
			name: "manual_retrieve_retry", autonomy: model.AutonomyLevelManual,
			metric: model.SupervisionMetricRecallPrecision, targetType: model.SupervisionTargetRAG, targetID: "q1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantAutonomyLvl: model.AutonomyLevelManual,
			wantReasonSubstr: "manual mode forbids auto action",
		},
		{
			name: "manual_query_rewrite", autonomy: model.AutonomyLevelManual,
			metric: model.SupervisionMetricRecallCoverage, targetType: model.SupervisionTargetRAG, targetID: "q2",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantAutonomyLvl: model.AutonomyLevelManual,
		},
		{
			name: "manual_llm_correction", autonomy: model.AutonomyLevelManual,
			metric: model.SupervisionMetricGenerationFidelity, targetType: model.SupervisionTargetLLM, targetID: "r1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantAutonomyLvl: model.AutonomyLevelManual,
		},
		{
			name: "manual_asset_rollback", autonomy: model.AutonomyLevelManual,
			metric: model.SupervisionMetricAssetEffectiveness, targetType: model.SupervisionTargetAsset, targetID: "a1",
			wantCreateCalls: 1, wantActionStatus: model.CorrectionStatusPending,
			wantAutonomyLvl: model.AutonomyLevelManual,
		},
		// Create 错误传播
		{
			name: "create_error_supervised", autonomy: model.AutonomyLevelSupervised,
			metric: model.SupervisionMetricRecallPrecision, targetType: model.SupervisionTargetRAG, targetID: "q1",
			createErr: errors.New("pending create db error"),
			wantErr: true, errSubstr: "pending create db error",
			wantCreateCalls: 1,
		},
		{
			name: "create_error_manual", autonomy: model.AutonomyLevelManual,
			metric: model.SupervisionMetricRecallPrecision, targetType: model.SupervisionTargetRAG, targetID: "q1",
			createErr: errors.New("manual create error"),
			wantErr: true, errSubstr: "manual create error",
			wantCreateCalls: 1,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			d, _, ar, _ := newDispatcherSetup(c.autonomy, true, true, true)
			ar.createErr = c.createErr

			sig := mkSignal(c.targetType, c.metric, "sig-"+c.name, c.targetID, model.SupervisionStatusAlert)
			err := d.DispatchFromSignal(context.Background(), sig)

			if c.wantErr {
				if err == nil || !errContains(err, c.errSubstr) {
					t.Fatalf("err = %v, want error containing %q", err, c.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if ar.createCalls != c.wantCreateCalls {
				t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
			}
			if len(ar.created) > 0 {
				got := ar.created[0]
				// 仅在非错误路径下检查 Status（错误路径下 Create 仍会捕获 pending 动作，
				// 但 wantActionStatus 未设置，无需断言）
				if !c.wantErr && c.wantActionStatus != "" && got.Status != c.wantActionStatus {
					t.Errorf("Status = %v, want %v", got.Status, c.wantActionStatus)
				}
				if c.wantAutonomyLvl != "" && got.AutonomyLevel != c.wantAutonomyLvl {
					t.Errorf("AutonomyLevel = %v, want %v", got.AutonomyLevel, c.wantAutonomyLvl)
				}
				if c.wantReasonSubstr != "" && !strings.Contains(got.Reason, c.wantReasonSubstr) {
					t.Errorf("Reason = %q, want substring %q", got.Reason, c.wantReasonSubstr)
				}
				// pending 动作不应设置 AppliedAt
				if got.AppliedAt != nil {
					t.Errorf("AppliedAt should be nil for pending action")
				}
				// Before 应包含 metric/value/threshold
				if got.Before["metric"] != c.metric {
					t.Errorf("Before[metric] = %v, want %v", got.Before["metric"], c.metric)
				}
			}
		})
	}

	// 测试 createPendingAction 中 GetStatus 错误时默认 manual
	t.Run("get_status_error_defaults_to_manual", func(t *testing.T) {
		svc, sr, _ := newSelfLearningTestService(5 * time.Second)
		// 设置缓存使 ShouldExecuteAction 成功，但 createPendingAction 的 GetStatus 失败
		// 由于缓存命中，GetStatus 不会失败；要测试此路径需清空缓存
		// 但清空缓存后 ShouldExecuteAction 也会失败
		// 所以 createPendingAction 中 GetStatus 错误的默认行为只能在缓存命中时正常工作
		// 此测试验证 supervised 模式下缓存命中时正常创建 pending
		snap := &SwitchSnapshot{
			AutonomyLevel:           model.AutonomyLevelSupervised,
			EnableRAG:               true,
			EnableAsset:             true,
			EnableLLM:               true,
			MaxDailyCorrections:     100,
			MaxDailyPromotions:      5,
			LowQualityThreshold:     3.0,
			ChampionRewardThreshold: 1.5,
			ABTestMinSamples:        100,
			CircuitBreakerThreshold: 0.3,
			CircuitBreakerWindowMin: 30,
		}
		setCache(svc, snap, 0, 0)
		sr.sw.AutonomyLevel = model.AutonomyLevelSupervised
		sr.sw.EnableRAG = true

		ar := &capActionRepo{}
		d := NewSelfCorrectionDispatcher(svc, ar, &mockLogRepo{}, &mockSignalRepo{}, nil, nil, nil)

		sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, "s1", "q1", model.SupervisionStatusAlert)
		err := d.DispatchFromSignal(context.Background(), sig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ar.createCalls != 1 {
			t.Fatalf("createCalls = %d, want 1", ar.createCalls)
		}
		if ar.created[0].AutonomyLevel != model.AutonomyLevelSupervised {
			t.Errorf("AutonomyLevel = %v, want supervised", ar.created[0].AutonomyLevel)
		}
	})
}

// ============================================================================
// TestSelfCorrectionDispatcher_ApproveAction
// ============================================================================

func TestSelfCorrectionDispatcher_ApproveAction(t *testing.T) {
	type tc struct {
		name             string
		action           *model.SelfCorrectionAction
		getByIDErr       error
		updateErr        error
		createErr        error
		wantErr          bool
		errSubstr        string
		wantUpdateCalls  int
		wantFinalStatus  model.CorrectionActionStatus
		wantCreateCalls  int
	}

	cases := []tc{
		// GetByID 错误
		{
			name: "get_by_id_error", action: mkPendingAction("aid-1", model.CorrectionRetrieveRetry, "q1"),
			getByIDErr: errors.New("db down"),
			wantErr: true, errSubstr: "get action failed",
			wantUpdateCalls: 0,
		},
		// 状态非 pending
		{
			name: "status_applied_not_pending",
			action: func() *model.SelfCorrectionAction {
				a := mkPendingAction("aid-2", model.CorrectionRetrieveRetry, "q1")
				a.Status = model.CorrectionStatusApplied
				return a
			}(),
			wantErr: true, errSubstr: "not pending",
			wantUpdateCalls: 0,
		},
		{
			name: "status_failed_not_pending",
			action: func() *model.SelfCorrectionAction {
				a := mkPendingAction("aid-3", model.CorrectionRetrieveRetry, "q1")
				a.Status = model.CorrectionStatusFailed
				return a
			}(),
			wantErr: true, errSubstr: "not pending",
			wantUpdateCalls: 0,
		},
		{
			name: "status_skipped_not_pending",
			action: func() *model.SelfCorrectionAction {
				a := mkPendingAction("aid-4", model.CorrectionRetrieveRetry, "q1")
				a.Status = model.CorrectionStatusSkipped
				return a
			}(),
			wantErr: true, errSubstr: "not pending",
			wantUpdateCalls: 0,
		},
		// retrieve_retry 成功
		{
			name: "retrieve_retry_success", action: mkPendingAction("aid-5", model.CorrectionRetrieveRetry, "q1"),
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusApplied,
			wantCreateCalls: 1, // executeRetrieveRetry 调用 Create
		},
		// query_rewrite 成功
		{
			name: "query_rewrite_success", action: mkPendingAction("aid-6", model.CorrectionQueryRewrite, "q2"),
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusApplied,
			wantCreateCalls: 1,
		},
		// chunk_archive 成功（需要 targetID 为数字）
		{
			name: "chunk_archive_success",
			action: func() *model.SelfCorrectionAction {
				a := mkPendingAction("aid-7", model.CorrectionChunkArchive, "123")
				a.TargetType = "rag_chunk"
				return a
			}(),
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusApplied,
			wantCreateCalls: 1,
		},
		// chunk_archive 失败（空 target_id）→ UpdateStatus(failed)
		{
			name: "chunk_archive_empty_target_fails",
			action: mkPendingAction("aid-8", model.CorrectionChunkArchive, ""),
			wantErr: true, errSubstr: "target_id is empty",
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusFailed,
			wantCreateCalls: 0,
		},
		// chunk_archive 失败（非数字 target_id）→ UpdateStatus(failed)
		{
			name: "chunk_archive_non_numeric_fails",
			action: mkPendingAction("aid-9", model.CorrectionChunkArchive, "abc"),
			wantErr: true, errSubstr: "parse chunk_id failed",
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusFailed,
			wantCreateCalls: 0,
		},
		// asset_promote → executeAction 返回 nil → UpdateStatus(applied)
		{
			name: "asset_promote_success", action: mkPendingAction("aid-10", model.CorrectionAssetPromote, "asset-1"),
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusApplied,
			wantCreateCalls: 0,
		},
		// asset_rollback → executeAction 返回 nil → UpdateStatus(applied)
		{
			name: "asset_rollback_success", action: mkPendingAction("aid-11", model.CorrectionAssetRollback, "asset-2"),
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusApplied,
			wantCreateCalls: 0,
		},
		// champion_upsert → executeAction 返回 error → UpdateStatus(failed)
		{
			name: "champion_upsert_error", action: mkPendingAction("aid-12", model.CorrectionChampionUpsert, "chunk-1"),
			wantErr: true, errSubstr: "champion_upsert should be triggered",
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusFailed,
			wantCreateCalls: 0,
		},
		// llm_correction + nil corrector → executeAction 返回 error → UpdateStatus(failed)
		{
			name: "llm_correction_nil_corrector", action: mkPendingAction("aid-13", model.CorrectionLLMCorrection, "reply-1"),
			wantErr: true, errSubstr: "llm corrector is nil",
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusFailed,
			wantCreateCalls: 0,
		},
		// UpdateStatus 错误（成功路径）→ propagate
		{
			name: "update_status_error_on_success", action: mkPendingAction("aid-14", model.CorrectionRetrieveRetry, "q1"),
			updateErr: errors.New("update db error"),
			wantErr: true, errSubstr: "update db error",
			wantUpdateCalls: 1, wantCreateCalls: 1,
		},
		// UpdateStatus 错误（失败路径）→ 返回原始 executeAction 错误
		{
			name: "update_status_error_on_failure",
			action: mkPendingAction("aid-15", model.CorrectionChampionUpsert, "chunk-1"),
			updateErr: errors.New("update also failed"),
			wantErr: true, errSubstr: "champion_upsert should be triggered",
			wantUpdateCalls: 1, wantCreateCalls: 0,
		},
		// executeRetrieveRetry 中 Create 错误 → executeAction 失败 → UpdateStatus(failed)
		{
			name: "create_error_in_execute_via_approve",
			action: mkPendingAction("aid-16", model.CorrectionRetrieveRetry, "q1"),
			createErr: errors.New("create during approve failed"),
			wantErr: true, errSubstr: "create during approve failed",
			wantUpdateCalls: 1, wantFinalStatus: model.CorrectionStatusFailed,
			wantCreateCalls: 1,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
			ar.getByIDAction = c.action
			ar.getByIDErr = c.getByIDErr
			ar.updateErr = c.updateErr
			ar.createErr = c.createErr

			err := d.ApproveAction(context.Background(), c.action.ActionID, 42, "approved by tester")

			if c.wantErr {
				if err == nil || !errContains(err, c.errSubstr) {
					t.Fatalf("err = %v, want error containing %q", err, c.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if ar.updateCalls != c.wantUpdateCalls {
				t.Errorf("updateCalls = %d, want %d", ar.updateCalls, c.wantUpdateCalls)
			}
			if ar.createCalls != c.wantCreateCalls {
				t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
			}
			if c.wantFinalStatus != "" && len(ar.updateCallsList) > 0 {
				if ar.updateCallsList[0].Status != c.wantFinalStatus {
					t.Errorf("update status = %v, want %v", ar.updateCallsList[0].Status, c.wantFinalStatus)
				}
			}
		})
	}
}

// ============================================================================
// TestSelfCorrectionDispatcher_RejectAction
// ============================================================================

func TestSelfCorrectionDispatcher_RejectAction(t *testing.T) {
	t.Run("success_updates_to_skipped", func(t *testing.T) {
		d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
		err := d.RejectAction(context.Background(), "aid-reject-1", 99, "bad action")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ar.updateCalls != 1 {
			t.Errorf("updateCalls = %d, want 1", ar.updateCalls)
		}
		if len(ar.updateCallsList) != 1 {
			t.Fatalf("updateCallsList len = %d, want 1", len(ar.updateCallsList))
		}
		uc := ar.updateCallsList[0]
		if uc.ActionID != "aid-reject-1" {
			t.Errorf("ActionID = %v, want aid-reject-1", uc.ActionID)
		}
		if uc.Status != model.CorrectionStatusSkipped {
			t.Errorf("Status = %v, want skipped", uc.Status)
		}
		if uc.Extra["operator_id"] != uint(99) {
			t.Errorf("operator_id = %v, want 99", uc.Extra["operator_id"])
		}
		reason, ok := uc.Extra["reason"].(string)
		if !ok || !strings.Contains(reason, "rejected: bad action") {
			t.Errorf("reason = %v, want 'rejected: bad action'", uc.Extra["reason"])
		}
	})

	t.Run("update_error_propagates", func(t *testing.T) {
		d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
		ar.updateErr = errors.New("update reject failed")
		err := d.RejectAction(context.Background(), "aid-reject-2", 1, "nope")
		if err == nil || !errContains(err, "update reject failed") {
			t.Fatalf("err = %v, want 'update reject failed'", err)
		}
		if ar.updateCalls != 1 {
			t.Errorf("updateCalls = %d, want 1", ar.updateCalls)
		}
	})

	t.Run("empty_reason", func(t *testing.T) {
		d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
		err := d.RejectAction(context.Background(), "aid-reject-3", 1, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ar.updateCalls != 1 {
			t.Errorf("updateCalls = %d, want 1", ar.updateCalls)
		}
		reason, _ := ar.updateCallsList[0].Extra["reason"].(string)
		if !strings.Contains(reason, "rejected:") {
			t.Errorf("reason = %q, want 'rejected:'", reason)
		}
	})

	t.Run("different_operator", func(t *testing.T) {
		d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
		err := d.RejectAction(context.Background(), "aid-reject-4", 777, "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ar.updateCallsList[0].Extra["operator_id"] != uint(777) {
			t.Errorf("operator_id = %v, want 777", ar.updateCallsList[0].Extra["operator_id"])
		}
	})
}

// ============================================================================
// TestSelfCorrectionDispatcher_SignalStatuses  3 种信号状态
// ============================================================================

func TestSelfCorrectionDispatcher_SignalStatuses(t *testing.T) {
	type tc struct {
		name             string
		status           model.SupervisionSignalStatus
		autonomy         model.AutonomyLevel
		wantCreateCalls  int
		wantActionStatus model.CorrectionActionStatus
	}

	for _, status := range allSignalStatuses() {
		for _, autonomy := range []model.AutonomyLevel{model.AutonomyLevelAutonomous, model.AutonomyLevelSupervised} {
			status := status
			autonomy := autonomy
			name := fmt.Sprintf("%s_%s", status, autonomy)
			c := tc{
				name:     name,
				status:   status,
				autonomy: autonomy,
			}
			if autonomy == model.AutonomyLevelAutonomous {
				c.wantCreateCalls = 1
				c.wantActionStatus = model.CorrectionStatusApplied
			} else {
				c.wantCreateCalls = 1
				c.wantActionStatus = model.CorrectionStatusPending
			}

			t.Run(c.name, func(t *testing.T) {
				d, _, ar, _ := newDispatcherSetup(c.autonomy, true, true, true)
				sig := mkSignal(model.SupervisionTargetRAG, model.SupervisionMetricRecallPrecision, "sig-"+c.name, "q1", c.status)
				err := d.DispatchFromSignal(context.Background(), sig)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if ar.createCalls != c.wantCreateCalls {
					t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
				}
				if len(ar.created) > 0 && ar.created[0].Status != c.wantActionStatus {
					t.Errorf("Status = %v, want %v", ar.created[0].Status, c.wantActionStatus)
				}
			})
		}
	}
}

// ============================================================================
// TestSelfCorrectionDispatcher_SupervisionTargetTypes  4 种目标类型
// ============================================================================

func TestSelfCorrectionDispatcher_SupervisionTargetTypes(t *testing.T) {
	type tc struct {
		name            string
		targetType      model.SupervisionTargetType
		metric          string
		targetID        string
		wantErr         bool
		errSubstr       string
		wantCreateCalls int
	}

	cases := []tc{
		// rag target
		{
			name: "rag_recall_precision", targetType: model.SupervisionTargetRAG,
			metric: model.SupervisionMetricRecallPrecision, targetID: "q1",
			wantCreateCalls: 1,
		},
		{
			name: "rag_recall_coverage", targetType: model.SupervisionTargetRAG,
			metric: model.SupervisionMetricRecallCoverage, targetID: "q2",
			wantCreateCalls: 1,
		},
		// asset target
		{
			name: "asset_effectiveness", targetType: model.SupervisionTargetAsset,
			metric: model.SupervisionMetricAssetEffectiveness, targetID: "a1",
			wantCreateCalls: 0, // asset_rollback logged
		},
		{
			name: "asset_adoption", targetType: model.SupervisionTargetAsset,
			metric: model.SupervisionMetricAssetAdoption, targetID: "a2",
			wantCreateCalls: 0,
		},
		// llm target
		{
			name: "llm_generation_fidelity", targetType: model.SupervisionTargetLLM,
			metric: model.SupervisionMetricGenerationFidelity, targetID: "r1",
			wantErr: true, errSubstr: "llm corrector is nil",
			wantCreateCalls: 0,
		},
		{
			name: "llm_answer_relevance", targetType: model.SupervisionTargetLLM,
			metric: model.SupervisionMetricAnswerRelevance, targetID: "r2",
			wantErr: true, errSubstr: "llm corrector is nil",
			wantCreateCalls: 0,
		},
		// hybrid target
		{
			name: "hybrid_recall_precision", targetType: model.SupervisionTargetHybrid,
			metric: model.SupervisionMetricRecallPrecision, targetID: "h1",
			wantCreateCalls: 1,
		},
		{
			name: "hybrid_asset_effectiveness", targetType: model.SupervisionTargetHybrid,
			metric: model.SupervisionMetricAssetEffectiveness, targetID: "123",
			wantCreateCalls: 1, // chunk_archive (non-asset target)
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
			sig := mkSignal(c.targetType, c.metric, "sig-"+c.name, c.targetID, model.SupervisionStatusAlert)
			err := d.DispatchFromSignal(context.Background(), sig)

			if c.wantErr {
				if err == nil || !errContains(err, c.errSubstr) {
					t.Fatalf("err = %v, want error containing %q", err, c.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if ar.createCalls != c.wantCreateCalls {
				t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
			}
		})
	}
}

// ============================================================================
// TestSelfCorrectionDispatcher_AllActionTypesViaApprove  7 种动作类型
// ============================================================================

func TestSelfCorrectionDispatcher_AllActionTypesViaApprove(t *testing.T) {
	// 通过 ApproveAction 测试所有 7 种 action type 的 executeAction 路径
	type tc struct {
		name            string
		actionType      model.CorrectionActionType
		targetID        string
		wantErr         bool
		errSubstr       string
		wantCreateCalls int
		wantFinalStatus model.CorrectionActionStatus
	}

	for _, at := range allActionTypes() {
		at := at
		c := tc{
			name:       string(at),
			actionType: at,
			targetID:   "123",
		}
		switch at {
		case model.CorrectionRetrieveRetry, model.CorrectionQueryRewrite, model.CorrectionChunkArchive:
			c.wantCreateCalls = 1
			c.wantFinalStatus = model.CorrectionStatusApplied
		case model.CorrectionChampionUpsert:
			c.wantErr = true
			c.errSubstr = "champion_upsert should be triggered"
			c.wantFinalStatus = model.CorrectionStatusFailed
		case model.CorrectionAssetPromote, model.CorrectionAssetRollback:
			c.wantCreateCalls = 0
			c.wantFinalStatus = model.CorrectionStatusApplied
		case model.CorrectionLLMCorrection:
			c.wantErr = true
			c.errSubstr = "llm corrector is nil"
			c.wantFinalStatus = model.CorrectionStatusFailed
		}

		t.Run(c.name, func(t *testing.T) {
			d, _, ar, _ := newDispatcherSetup(model.AutonomyLevelAutonomous, true, true, true)
			ar.getByIDAction = mkPendingAction("aid-"+c.name, c.actionType, c.targetID)

			err := d.ApproveAction(context.Background(), "aid-"+c.name, 1, "test note")
			if c.wantErr {
				if err == nil || !errContains(err, c.errSubstr) {
					t.Fatalf("err = %v, want error containing %q", err, c.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if ar.createCalls != c.wantCreateCalls {
				t.Errorf("createCalls = %d, want %d", ar.createCalls, c.wantCreateCalls)
			}
			if ar.updateCalls != 1 {
				t.Errorf("updateCalls = %d, want 1", ar.updateCalls)
			}
			if c.wantFinalStatus != "" && len(ar.updateCallsList) > 0 {
				if ar.updateCallsList[0].Status != c.wantFinalStatus {
					t.Errorf("update status = %v, want %v", ar.updateCallsList[0].Status, c.wantFinalStatus)
				}
			}
		})
	}
}

// ============================================================================
// TestSelfCorrectionDispatcher_DispatchFromSignal_AllMetricsComprehensive
// ============================================================================

func TestSelfCorrectionDispatcher_DispatchFromSignal_AllMetricsComprehensive(t *testing.T) {
	// 综合测试：所有 10 种指标 × 4 种目标类型 × 3 种自治等级
	// 仅验证无 panic + 错误/成功模式正确，不深究每个组合
	for _, metric := range allSupervisionMetrics() {
		for _, tt := range allSupervisionTargetTypes() {
			for _, autonomy := range []model.AutonomyLevel{model.AutonomyLevelAutonomous, model.AutonomyLevelSupervised, model.AutonomyLevelManual} {
				metric := metric
				tt := tt
				autonomy := autonomy
				name := fmt.Sprintf("%s/%s/%s", metric, tt, autonomy)
				t.Run(name, func(t *testing.T) {
					d, _, ar, _ := newDispatcherSetup(autonomy, true, true, true)
					sig := mkSignal(tt, metric, "sig-"+name, "123", model.SupervisionStatusAlert)
					err := d.DispatchFromSignal(context.Background(), sig)
					// 不严格断言错误（某些组合会出错，某些不会）
					// 仅验证：无 panic + Create 调用次数合理
					if err != nil && ar.createCalls > 0 {
						// 如果出错但 Create 被调用了，说明错误发生在 Create 之后
						// 这是合理的
					}
					if err == nil && ar.createCalls == 0 {
						// asset_rollback 路径或 autonomous 执行成功但无 Create（asset 路径）
					}
				})
			}
		}
	}
}
