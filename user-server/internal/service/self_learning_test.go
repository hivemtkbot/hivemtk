package service

// self_learning_service_test.go SelfLearningService (L3 门面) 单元测试
//
// 测试策略：
//   - 使用标准库 testing，table-driven + t.Run 子测试
//   - 自实现 6 个 mock 仓储（不依赖 DB），实现 repository 层接口
//   - 对 L4 具体组件（SwitchService / RAGSelfSupervisor / AssetBundleSelfSupervisor /
//     SelfCorrectionDispatcher / Orchestrator）使用真实实例 + mock 仓储装配
//   - 覆盖：开关 API、看板 API、日志/候选/AB/矫正列表、PromoteABTest、
//     Approve/RejectCorrection、各组件 nil 降级、各仓储错误传播
//
// 测试函数命名：TestSelfLearningService_XXX

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
	selflearning "marketing/internal/service/self_learning"
)

// ============================================================================
// Mock 仓储实现
// ============================================================================

// --- mockSLSwitchRepo: SelfLearningSwitchRepository ---

type mockSLSwitchRepo struct {
	mu sync.Mutex
	sw *model.SelfLearningSwitch

	getErr                  error
	getOrCreateErr          error
	updateErr               error
	incrementCorrectionsErr error
	incrementPromotionsErr  error
	resetDailyErr           error
	markTriggeredErr        error
	setCircuitOpenErr       error

	getCalls                  int
	getOrCreateCalls          int
	updateCalls               int
	incrementCorrectionsCalls int
	incrementPromotionsCalls  int
	resetDailyCalls           int
	markTriggeredCalls        int
	setCircuitOpenCalls       int
}

func (m *mockSLSwitchRepo) Get(ctx context.Context) (*model.SelfLearningSwitch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalls++
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.sw == nil {
		return nil, errors.New("record not found")
	}
	cp := *m.sw
	return &cp, nil
}

func (m *mockSLSwitchRepo) GetOrCreate(ctx context.Context) (*model.SelfLearningSwitch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreateCalls++
	if m.getOrCreateErr != nil {
		return nil, m.getOrCreateErr
	}
	if m.sw == nil {
		m.sw = defaultSLSwitch()
	}
	cp := *m.sw
	return &cp, nil
}

func (m *mockSLSwitchRepo) Update(ctx context.Context, s *model.SelfLearningSwitch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	if m.updateErr != nil {
		return m.updateErr
	}
	if s != nil {
		cp := *s
		m.sw = &cp
	}
	return nil
}

func (m *mockSLSwitchRepo) IncrementTodayCorrections(ctx context.Context, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incrementCorrectionsCalls++
	if m.incrementCorrectionsErr != nil {
		return m.incrementCorrectionsErr
	}
	if m.sw != nil {
		m.sw.TodayCorrections += delta
	}
	return nil
}

func (m *mockSLSwitchRepo) IncrementTodayPromotions(ctx context.Context, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incrementPromotionsCalls++
	if m.incrementPromotionsErr != nil {
		return m.incrementPromotionsErr
	}
	if m.sw != nil {
		m.sw.TodayPromotions += delta
	}
	return nil
}

func (m *mockSLSwitchRepo) ResetDailyCounters(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetDailyCalls++
	return m.resetDailyErr
}

func (m *mockSLSwitchRepo) MarkTriggered(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markTriggeredCalls++
	return m.markTriggeredErr
}

func (m *mockSLSwitchRepo) SetCircuitOpen(ctx context.Context, open bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCircuitOpenCalls++
	return m.setCircuitOpenErr
}

// --- mockSLLogRepo: SelfLearningLogRepository ---

type mockSLLogRepo struct {
	mu sync.Mutex

	listByScenarioRes []*model.SelfLearningLog
	listByScenarioErr error
	listByStatusRes   []*model.SelfLearningLog
	listByStatusErr   error
	countTodayRes     map[model.SelfLearningStatus]int64
	countTodayErr     error

	createErr       error
	existsRes       bool
	existsErr       error
	updateStatusErr error
	getByLogIDRes   *model.SelfLearningLog
	getByLogIDErr   error
	markStaleRes    int64
	markStaleErr    error

	listByScenarioCalls int
	listByStatusCalls   int
	countTodayCalls     int
	createCalls         int
}

func (m *mockSLLogRepo) Create(ctx context.Context, lg *model.SelfLearningLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	return m.createErr
}

func (m *mockSLLogRepo) ExistsBySessionAndScenario(ctx context.Context, sessionID string, scenario model.SelfLearningScenario) (bool, error) {
	return m.existsRes, m.existsErr
}

func (m *mockSLLogRepo) UpdateStatus(ctx context.Context, logID string, status model.SelfLearningStatus, errMsg string, outputSummary model.JSONMap, durationMs int64) error {
	return m.updateStatusErr
}

func (m *mockSLLogRepo) GetByLogID(ctx context.Context, logID string) (*model.SelfLearningLog, error) {
	if m.getByLogIDErr != nil {
		return nil, m.getByLogIDErr
	}
	if m.getByLogIDRes != nil {
		cp := *m.getByLogIDRes
		return &cp, nil
	}
	return nil, errors.New("not found")
}

func (m *mockSLLogRepo) ListByScenario(ctx context.Context, scenario model.SelfLearningScenario, since time.Time, limit int) ([]*model.SelfLearningLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listByScenarioCalls++
	return m.listByScenarioRes, m.listByScenarioErr
}

func (m *mockSLLogRepo) ListByStatus(ctx context.Context, status model.SelfLearningStatus, limit int) ([]*model.SelfLearningLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listByStatusCalls++
	return m.listByStatusRes, m.listByStatusErr
}

func (m *mockSLLogRepo) CountToday(ctx context.Context) (map[model.SelfLearningStatus]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.countTodayCalls++
	if m.countTodayErr != nil {
		return nil, m.countTodayErr
	}
	if m.countTodayRes == nil {
		return make(map[model.SelfLearningStatus]int64), nil
	}
	cp := make(map[model.SelfLearningStatus]int64, len(m.countTodayRes))
	for k, v := range m.countTodayRes {
		cp[k] = v
	}
	return cp, nil
}

func (m *mockSLLogRepo) MarkStaleLogsAsSkipped(ctx context.Context, before time.Time) (int64, error) {
	return m.markStaleRes, m.markStaleErr
}

// --- mockSLSignalRepo: SelfSupervisionSignalRepository ---

type mockSLSignalRepo struct {
	mu sync.Mutex

	aggAvg   float64
	aggCount int64
	aggErr   error

	alertsList    []*model.SelfSupervisionSignal
	listAlertsErr error

	aggCalls        int
	listAlertsCalls int
}

func (m *mockSLSignalRepo) UpsertSignal(ctx context.Context, sig *model.SelfSupervisionSignal) error {
	return nil
}

func (m *mockSLSignalRepo) GetByID(ctx context.Context, signalID string) (*model.SelfSupervisionSignal, error) {
	return nil, errors.New("not found")
}

func (m *mockSLSignalRepo) ListByMetric(ctx context.Context, targetType model.SupervisionTargetType, metricName string, from, to time.Time, limit int) ([]*model.SelfSupervisionSignal, error) {
	return nil, nil
}

func (m *mockSLSignalRepo) ListAlerts(ctx context.Context, since time.Time, limit int) ([]*model.SelfSupervisionSignal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listAlertsCalls++
	if m.listAlertsErr != nil {
		return nil, m.listAlertsErr
	}
	return m.alertsList, nil
}

func (m *mockSLSignalRepo) ListByTarget(ctx context.Context, targetType model.SupervisionTargetType, targetID string, from, to time.Time, limit int) ([]*model.SelfSupervisionSignal, error) {
	return nil, nil
}

func (m *mockSLSignalRepo) AggregateByRange(ctx context.Context, targetType model.SupervisionTargetType, metricName string, from, to time.Time) (float64, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aggCalls++
	return m.aggAvg, m.aggCount, m.aggErr
}

// --- mockSLActionRepo: SelfCorrectionActionRepository ---

type mockSLActionRepo struct {
	mu sync.Mutex

	getByIDAction *model.SelfCorrectionAction
	getByIDErr    error
	getByIDCalls  int

	updateErr   error
	updateCalls int

	listPendingRes   []*model.SelfCorrectionAction
	listPendingErr   error
	listPendingCalls int

	listByFilterRes   []*model.SelfCorrectionAction
	listByFilterTotal int64
	listByFilterErr   error
	listByFilterCalls int

	createErr   error
	createCalls int
}

func (m *mockSLActionRepo) Create(ctx context.Context, a *model.SelfCorrectionAction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	return m.createErr
}

func (m *mockSLActionRepo) GetByID(ctx context.Context, actionID string) (*model.SelfCorrectionAction, error) {
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

func (m *mockSLActionRepo) UpdateStatus(ctx context.Context, actionID string, status model.CorrectionActionStatus, extraUpdates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	return m.updateErr
}

func (m *mockSLActionRepo) ListPending(ctx context.Context, limit int) ([]*model.SelfCorrectionAction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listPendingCalls++
	return m.listPendingRes, m.listPendingErr
}

func (m *mockSLActionRepo) ListByTarget(ctx context.Context, targetType, targetID string, limit int) ([]*model.SelfCorrectionAction, error) {
	return nil, nil
}

func (m *mockSLActionRepo) ListByTriggerLog(ctx context.Context, triggerLogID string, limit int) ([]*model.SelfCorrectionAction, error) {
	return nil, nil
}

func (m *mockSLActionRepo) ListByFilter(ctx context.Context, filter repository.CorrectionActionFilter) ([]*model.SelfCorrectionAction, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listByFilterCalls++
	return m.listByFilterRes, m.listByFilterTotal, m.listByFilterErr
}

func (m *mockSLActionRepo) CountToday(ctx context.Context) (map[model.CorrectionActionType]int64, error) {
	return make(map[model.CorrectionActionType]int64), nil
}

// --- mockSLCandidateRepo: AssetBundleCandidateRepository ---

type mockSLCandidateRepo struct {
	mu sync.Mutex

	listByStatusRes   []*model.AssetBundleCandidate
	listByStatusErr   error
	listByStatusCalls int

	createErr   error
	createCalls int
}

func (m *mockSLCandidateRepo) Create(ctx context.Context, c *model.AssetBundleCandidate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	return m.createErr
}

func (m *mockSLCandidateRepo) GetByCandidateID(ctx context.Context, candidateID string) (*model.AssetBundleCandidate, error) {
	return nil, errors.New("not found")
}

func (m *mockSLCandidateRepo) ListPendingByScenario(ctx context.Context, scenario string, since time.Time, limit int) ([]*model.AssetBundleCandidate, error) {
	return nil, nil
}

func (m *mockSLCandidateRepo) ListByStatus(ctx context.Context, status model.AssetBundleCandidateStatus, limit int) ([]*model.AssetBundleCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listByStatusCalls++
	return m.listByStatusRes, m.listByStatusErr
}

func (m *mockSLCandidateRepo) UpdateStatus(ctx context.Context, candidateID string, status model.AssetBundleCandidateStatus, extraUpdates map[string]any) error {
	return nil
}

func (m *mockSLCandidateRepo) CountToday(ctx context.Context) (map[model.AssetBundleCandidateStatus]int64, error) {
	return make(map[model.AssetBundleCandidateStatus]int64), nil
}

func (m *mockSLCandidateRepo) CountByStatus(ctx context.Context) (map[model.AssetBundleCandidateStatus]int64, error) {
	return make(map[model.AssetBundleCandidateStatus]int64), nil
}

// --- mockSLABTestRepo: AssetBundleABTestRepository ---

type mockSLABTestRepo struct {
	mu sync.Mutex

	listByStatusRes   []*model.AssetBundleABTest
	listByStatusErr   error
	listByStatusCalls int

	countByStatusRes   map[model.AssetBundleABTestStatus]int64
	countByStatusErr   error
	countByStatusCalls int

	createErr   error
	createCalls int
}

func (m *mockSLABTestRepo) Create(ctx context.Context, t *model.AssetBundleABTest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	return m.createErr
}

func (m *mockSLABTestRepo) GetByExperimentID(ctx context.Context, experimentID string) (*model.AssetBundleABTest, error) {
	return nil, errors.New("not found")
}

func (m *mockSLABTestRepo) FindRunningByBaseline(ctx context.Context, baselineAssetID string) (*model.AssetBundleABTest, error) {
	return nil, errors.New("not found")
}

func (m *mockSLABTestRepo) ListByStatus(ctx context.Context, status model.AssetBundleABTestStatus, limit int) ([]*model.AssetBundleABTest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listByStatusCalls++
	return m.listByStatusRes, m.listByStatusErr
}

func (m *mockSLABTestRepo) ListRunningByScenario(ctx context.Context, scenario string, limit int) ([]*model.AssetBundleABTest, error) {
	return nil, nil
}

func (m *mockSLABTestRepo) IncrementSamples(ctx context.Context, experimentID string, arm string, deltaSamples int, deltaReward float64) error {
	return nil
}

func (m *mockSLABTestRepo) UpdateStatus(ctx context.Context, experimentID string, status model.AssetBundleABTestStatus, extraUpdates map[string]any) error {
	return nil
}

func (m *mockSLABTestRepo) CountByStatus(ctx context.Context) (map[model.AssetBundleABTestStatus]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.countByStatusCalls++
	if m.countByStatusErr != nil {
		return nil, m.countByStatusErr
	}
	if m.countByStatusRes == nil {
		return make(map[model.AssetBundleABTestStatus]int64), nil
	}
	cp := make(map[model.AssetBundleABTestStatus]int64, len(m.countByStatusRes))
	for k, v := range m.countByStatusRes {
		cp[k] = v
	}
	return cp, nil
}

func (m *mockSLABTestRepo) ListConvergedPendingAction(ctx context.Context, limit int) ([]*model.AssetBundleABTest, error) {
	return nil, nil
}

// ============================================================================
// 辅助构造函数
// ============================================================================

func defaultSLSwitch() *model.SelfLearningSwitch {
	return &model.SelfLearningSwitch{
		ID:                      1,
		AutonomyLevel:           model.AutonomyLevelManual,
		EnableRAG:               false,
		EnableAsset:             false,
		EnableLLM:               false,
		MaxDailyCorrections:     100,
		MaxDailyPromotions:      5,
		LowQualityThreshold:     3.0,
		ChampionRewardThreshold: 1.5,
		ABTestMinSamples:        100,
		CircuitBreakerThreshold: 0.3,
		CircuitBreakerWindowMin: 30,
	}
}

// newTestSwitchSvc 构造真实 *selflearning.SwitchService + mock 仓储
func newTestSwitchRepo() *mockSLSwitchRepo {
	return &mockSLSwitchRepo{sw: defaultSLSwitch()}
}

// newTestSelfLearningService 构造 L3 服务（components 可为 nil）
func newTestSelfLearningService(comps *SelfLearningComponents) *SelfLearningService {
	if comps == nil {
		comps = &SelfLearningComponents{}
	}
	return &SelfLearningService{components: comps}
}

// makeLog 构造 SelfLearningLog
func makeLog(logID string, status model.SelfLearningStatus) *model.SelfLearningLog {
	return &model.SelfLearningLog{
		LogID:     logID,
		SessionID: "sess-" + logID,
		TraceID:   "trace-" + logID,
		Scenario:  model.ScenarioRagWarmup,
		Status:    status,
		StartedAt: time.Now().Add(-1 * time.Hour),
		CreatedAt: time.Now(),
	}
}

// makeCandidate 构造 AssetBundleCandidate
func makeCandidate(cid string, status model.AssetBundleCandidateStatus) *model.AssetBundleCandidate {
	return &model.AssetBundleCandidate{
		CandidateID: cid,
		Scenario:    "sales",
		Industry:    "tech",
		Language:    "zh",
		Status:      status,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// makeABTest 构造 AssetBundleABTest
func makeABTest(eid string, status model.AssetBundleABTestStatus) *model.AssetBundleABTest {
	return &model.AssetBundleABTest{
		ExperimentID:    eid,
		BaselineAssetID: "asset-baseline",
		CandidateID:     "cand-1",
		Scenario:        "sales",
		Status:          status,
		StartedAt:       time.Now().Add(-1 * time.Hour),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

// makeAction 构造 SelfCorrectionAction
func makeAction(aid string, atype model.CorrectionActionType, status model.CorrectionActionStatus) *model.SelfCorrectionAction {
	return &model.SelfCorrectionAction{
		ActionID:   aid,
		ActionType: atype,
		Scenario:   "rag",
		TargetType: "rag_chunk",
		TargetID:   "chunk-1",
		Status:     status,
		Operator:   "auto",
		Reason:     "test reason",
		Before:     map[string]any{"metric": "recall_precision", "value": 0.5, "threshold": 0.8},
		CreatedAt:  time.Now(),
	}
}

// ============================================================================
// 测试函数
// ============================================================================

// TestSelfLearningService_NewSelfLearningService 构造函数
func TestSelfLearningService_NewSelfLearningService(t *testing.T) {
	t.Run("nil_components_returns_nil", func(t *testing.T) {
		svc := NewSelfLearningService(nil, nil)
		if svc != nil {
			t.Fatalf("expected nil, got %v", svc)
		}
	})

	t.Run("nonnil_components_nil_db_repos_nil", func(t *testing.T) {
		comps := &SelfLearningComponents{}
		svc := NewSelfLearningService(comps, nil)
		if svc == nil {
			t.Fatal("expected non-nil service")
		}
		if svc.components != comps {
			t.Fatal("components mismatch")
		}
		if svc.logRepo != nil {
			t.Fatal("expected nil logRepo")
		}
		if svc.candidateRepo != nil {
			t.Fatal("expected nil candidateRepo")
		}
		if svc.abTestRepo != nil {
			t.Fatal("expected nil abTestRepo")
		}
		if svc.actionRepo != nil {
			t.Fatal("expected nil actionRepo")
		}
	})

	t.Run("nonnil_components_nonnil_db_repos_set", func(t *testing.T) {
		comps := &SelfLearningComponents{}
		// db 非空但无实际连接：NewXxxRepository 仅包装 db，不连接
		// 使用 nil db 调用会 panic，所以这里仅验证 nil db 路径已在上一用例覆盖
		svc := NewSelfLearningService(comps, nil)
		if svc == nil {
			t.Fatal("expected non-nil service")
		}
	})
}

// TestSelfLearningService_GetSwitchStatus 开关状态查询
func TestSelfLearningService_GetSwitchStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_service", func(t *testing.T) {
		var svc *SelfLearningService
		resp, err := svc.GetSwitchStatus(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("nil_components", func(t *testing.T) {
		svc := &SelfLearningService{components: nil}
		resp, err := svc.GetSwitchStatus(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("nil_switch_svc", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		resp, err := svc.GetSwitchStatus(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("switch_svc_error", func(t *testing.T) {
		sr := &mockSLSwitchRepo{getOrCreateErr: errors.New("db down")}
		ss := selflearning.NewSwitchService(sr, &mockSLLogRepo{}, 5*time.Second)
		comps := &SelfLearningComponents{SwitchSvc: ss}
		svc := newTestSelfLearningService(comps)
		resp, err := svc.GetSwitchStatus(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("success", func(t *testing.T) {
		sr := newTestSwitchRepo()
		ss := selflearning.NewSwitchService(sr, &mockSLLogRepo{}, 5*time.Second)
		comps := &SelfLearningComponents{SwitchSvc: ss}
		svc := newTestSelfLearningService(comps)
		resp, err := svc.GetSwitchStatus(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
		if resp.AutonomyLevel != model.AutonomyLevelManual {
			t.Fatalf("unexpected autonomy: %s", resp.AutonomyLevel)
		}
	})

	t.Run("success_circuit_open", func(t *testing.T) {
		sr := newTestSwitchRepo()
		sr.sw.CircuitOpen = true
		ss := selflearning.NewSwitchService(sr, &mockSLLogRepo{}, 5*time.Second)
		comps := &SelfLearningComponents{SwitchSvc: ss}
		svc := newTestSelfLearningService(comps)
		resp, err := svc.GetSwitchStatus(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !resp.CircuitOpen {
			t.Fatal("expected circuit open")
		}
	})
}

// TestSelfLearningService_UpdateSwitch 更新开关配置
func TestSelfLearningService_UpdateSwitch(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name    string
		svc     func() *SelfLearningService
		req     *dto.SwitchConfigRequest
		wantErr bool
	}{
		{
			name:    "nil_service",
			svc:     func() *SelfLearningService { var s *SelfLearningService; return s },
			req:     &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelAutonomous},
			wantErr: true,
		},
		{
			name:    "nil_components",
			svc:     func() *SelfLearningService { return &SelfLearningService{components: nil} },
			req:     &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelAutonomous},
			wantErr: true,
		},
		{
			name:    "nil_switch_svc",
			svc:     func() *SelfLearningService { return newTestSelfLearningService(&SelfLearningComponents{}) },
			req:     &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelAutonomous},
			wantErr: true,
		},
		{
			name: "validate_fail_invalid_autonomy",
			svc: func() *SelfLearningService {
				return newTestSelfLearningService(&SelfLearningComponents{SwitchSvc: selflearning.NewSwitchService(newTestSwitchRepo(), &mockSLLogRepo{}, 5*time.Second)})
			},
			req:     &dto.SwitchConfigRequest{AutonomyLevel: "invalid"},
			wantErr: true,
		},
		{
			name: "validate_fail_negative_max_corrections",
			svc: func() *SelfLearningService {
				return newTestSelfLearningService(&SelfLearningComponents{SwitchSvc: selflearning.NewSwitchService(newTestSwitchRepo(), &mockSLLogRepo{}, 5*time.Second)})
			},
			req:     &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, MaxDailyCorrections: -1},
			wantErr: true,
		},
		{
			name: "validate_fail_negative_max_promotions",
			svc: func() *SelfLearningService {
				return newTestSelfLearningService(&SelfLearningComponents{SwitchSvc: selflearning.NewSwitchService(newTestSwitchRepo(), &mockSLLogRepo{}, 5*time.Second)})
			},
			req:     &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, MaxDailyPromotions: -1},
			wantErr: true,
		},
		{
			name: "validate_fail_cb_threshold_over_1",
			svc: func() *SelfLearningService {
				return newTestSelfLearningService(&SelfLearningComponents{SwitchSvc: selflearning.NewSwitchService(newTestSwitchRepo(), &mockSLLogRepo{}, 5*time.Second)})
			},
			req:     &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, CircuitBreakerThreshold: 1.5},
			wantErr: true,
		},
		{
			name: "validate_fail_negative_low_quality_threshold",
			svc: func() *SelfLearningService {
				return newTestSelfLearningService(&SelfLearningComponents{SwitchSvc: selflearning.NewSwitchService(newTestSwitchRepo(), &mockSLLogRepo{}, 5*time.Second)})
			},
			req:     &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, LowQualityThreshold: -0.1},
			wantErr: true,
		},
		{
			name: "switch_svc_update_error",
			svc: func() *SelfLearningService {
				sr := &mockSLSwitchRepo{sw: defaultSLSwitch(), updateErr: errors.New("update failed")}
				return newTestSelfLearningService(&SelfLearningComponents{SwitchSvc: selflearning.NewSwitchService(sr, &mockSLLogRepo{}, 5*time.Second)})
			},
			req:     &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelSupervised, EnableRAG: true},
			wantErr: true,
		},
		{
			name: "success_autonomous",
			svc: func() *SelfLearningService {
				return newTestSelfLearningService(&SelfLearningComponents{SwitchSvc: selflearning.NewSwitchService(newTestSwitchRepo(), &mockSLLogRepo{}, 5*time.Second)})
			},
			req: &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelAutonomous, EnableRAG: true, EnableAsset: true, EnableLLM: true},
		},
		{
			name: "success_supervised",
			svc: func() *SelfLearningService {
				return newTestSelfLearningService(&SelfLearningComponents{SwitchSvc: selflearning.NewSwitchService(newTestSwitchRepo(), &mockSLLogRepo{}, 5*time.Second)})
			},
			req: &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelSupervised, EnableRAG: true},
		},
		{
			name: "success_manual_with_cb_window_zero_defaults_30",
			svc: func() *SelfLearningService {
				return newTestSelfLearningService(&SelfLearningComponents{SwitchSvc: selflearning.NewSwitchService(newTestSwitchRepo(), &mockSLLogRepo{}, 5*time.Second)})
			},
			req: &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, CircuitBreakerWindowMin: 0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := tc.svc()
			resp, err := svc.UpdateSwitch(ctx, tc.req, 1)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if resp != nil {
					t.Fatal("expected nil resp on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if resp == nil {
				t.Fatal("expected non-nil resp")
			}
		})
	}
}

// TestSelfLearningService_GetDashboard 看板
func TestSelfLearningService_GetDashboard(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_components", func(t *testing.T) {
		svc := &SelfLearningService{components: nil}
		resp, err := svc.GetDashboard(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("all_nilexcept_switch_success", func(t *testing.T) {
		sr := newTestSwitchRepo()
		ss := selflearning.NewSwitchService(sr, &mockSLLogRepo{}, 5*time.Second)
		comps := &SelfLearningComponents{SwitchSvc: ss}
		svc := &SelfLearningService{components: comps}
		resp, err := svc.GetDashboard(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
		if resp.Switch == nil {
			t.Fatal("expected non-nil switch")
		}
		if resp.TodayTotal != 0 {
			t.Fatalf("expected 0 total, got %d", resp.TodayTotal)
		}
	})

	t.Run("log_repo_count_today_success", func(t *testing.T) {
		lr := &mockSLLogRepo{
			countTodayRes: map[model.SelfLearningStatus]int64{
				model.SelfLearningStatusSuccess: 5,
				model.SelfLearningStatusFailed:  2,
				model.SelfLearningStatusSkipped: 1,
			},
		}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.GetDashboard(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.TodayTotal != 8 {
			t.Fatalf("expected 8 total, got %d", resp.TodayTotal)
		}
		if resp.TodaySuccess != 5 {
			t.Fatalf("expected 5 success, got %d", resp.TodaySuccess)
		}
		if resp.TodayFailed != 2 {
			t.Fatalf("expected 2 failed, got %d", resp.TodayFailed)
		}
		if resp.SuccessRate != 5.0/8.0 {
			t.Fatalf("expected success rate 5/8, got %v", resp.SuccessRate)
		}
		if resp.FailedRate != 2.0/8.0 {
			t.Fatalf("expected failed rate 2/8, got %v", resp.FailedRate)
		}
	})

	t.Run("log_repo_count_today_error_succeeds_with_zero", func(t *testing.T) {
		lr := &mockSLLogRepo{countTodayErr: errors.New("db error")}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.GetDashboard(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.TodayTotal != 0 {
			t.Fatalf("expected 0 total on error, got %d", resp.TodayTotal)
		}
	})

	t.Run("log_repo_list_by_status_error_empty_failed_logs", func(t *testing.T) {
		lr := &mockSLLogRepo{
			listByStatusErr: errors.New("db error"),
		}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.GetDashboard(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.RecentFailedLogs) != 0 {
			t.Fatalf("expected 0 failed logs, got %d", len(resp.RecentFailedLogs))
		}
	})

	t.Run("log_repo_failed_logs_populated", func(t *testing.T) {
		lr := &mockSLLogRepo{
			listByStatusRes: []*model.SelfLearningLog{
				makeLog("log-1", model.SelfLearningStatusFailed),
				makeLog("log-2", model.SelfLearningStatusFailed),
			},
		}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.GetDashboard(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.RecentFailedLogs) != 2 {
			t.Fatalf("expected 2 failed logs, got %d", len(resp.RecentFailedLogs))
		}
	})

	t.Run("action_repo_pending_populated", func(t *testing.T) {
		ar := &mockSLActionRepo{
			listPendingRes: []*model.SelfCorrectionAction{
				makeAction("a-1", model.CorrectionRetrieveRetry, model.CorrectionStatusPending),
			},
		}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, actionRepo: ar}
		resp, err := svc.GetDashboard(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.RecentChampionOps) != 1 {
			t.Fatalf("expected 1 champion op, got %d", len(resp.RecentChampionOps))
		}
	})

	t.Run("action_repo_pending_error_empty", func(t *testing.T) {
		ar := &mockSLActionRepo{listPendingErr: errors.New("db error")}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, actionRepo: ar}
		resp, err := svc.GetDashboard(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.RecentChampionOps) != 0 {
			t.Fatalf("expected 0 champion ops on error, got %d", len(resp.RecentChampionOps))
		}
	})

	t.Run("switch_svc_error_switch_nil", func(t *testing.T) {
		sr := &mockSLSwitchRepo{getOrCreateErr: errors.New("db error")}
		ss := selflearning.NewSwitchService(sr, &mockSLLogRepo{}, 5*time.Second)
		lr := &mockSLLogRepo{}
		svc := &SelfLearningService{components: &SelfLearningComponents{SwitchSvc: ss}, logRepo: lr}
		resp, err := svc.GetDashboard(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Switch != nil {
			t.Fatal("expected nil switch on error")
		}
	})

	t.Run("all_repos_success", func(t *testing.T) {
		sr := newTestSwitchRepo()
		ss := selflearning.NewSwitchService(sr, &mockSLLogRepo{}, 5*time.Second)
		lr := &mockSLLogRepo{
			countTodayRes: map[model.SelfLearningStatus]int64{
				model.SelfLearningStatusSuccess: 10,
				model.SelfLearningStatusFailed:  3,
			},
			listByStatusRes: []*model.SelfLearningLog{makeLog("f-1", model.SelfLearningStatusFailed)},
		}
		ar := &mockSLActionRepo{
			listPendingRes: []*model.SelfCorrectionAction{
				makeAction("a-1", model.CorrectionAssetPromote, model.CorrectionStatusPending),
			},
		}
		svc := &SelfLearningService{
			components: &SelfLearningComponents{SwitchSvc: ss},
			logRepo:    lr,
			actionRepo: ar,
		}
		resp, err := svc.GetDashboard(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.TodayTotal != 13 {
			t.Fatalf("expected 13 total, got %d", resp.TodayTotal)
		}
		if resp.Switch == nil {
			t.Fatal("expected non-nil switch")
		}
		if len(resp.RecentFailedLogs) != 1 {
			t.Fatalf("expected 1 failed log, got %d", len(resp.RecentFailedLogs))
		}
		if len(resp.RecentChampionOps) != 1 {
			t.Fatalf("expected 1 champion op, got %d", len(resp.RecentChampionOps))
		}
	})

	t.Run("zero_total_rates_zero", func(t *testing.T) {
		lr := &mockSLLogRepo{
			countTodayRes: map[model.SelfLearningStatus]int64{},
		}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.GetDashboard(ctx)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.SuccessRate != 0 || resp.FailedRate != 0 {
			t.Fatalf("expected 0 rates, got success=%v failed=%v", resp.SuccessRate, resp.FailedRate)
		}
	})
}

// TestSelfLearningService_GetRAGSupervisionDashboard RAG 监督看板
func TestSelfLearningService_GetRAGSupervisionDashboard(t *testing.T) {
	ctx := context.Background()

	buildSvc := func() (*SelfLearningService, *mockSLSignalRepo) {
		sr := &mockSLSignalRepo{aggAvg: 0.85, aggCount: 100, alertsList: []*model.SelfSupervisionSignal{
			{SignalID: "sig-1", MetricName: model.SupervisionMetricRecallPrecision, Status: model.SupervisionStatusAlert, BucketHour: time.Now()},
		}}
		sup := selflearning.NewRAGSelfSupervisor(nil, sr, nil, nil, nil)
		comps := &SelfLearningComponents{RAGSupervisor: sup}
		return newTestSelfLearningService(comps), sr
	}

	t.Run("nil_service", func(t *testing.T) {
		var svc *SelfLearningService
		resp, err := svc.GetRAGSupervisionDashboard(ctx, "24h")
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("nil_components", func(t *testing.T) {
		svc := &SelfLearningService{components: nil}
		resp, err := svc.GetRAGSupervisionDashboard(ctx, "24h")
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("nil_rag_supervisor", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		resp, err := svc.GetRAGSupervisionDashboard(ctx, "24h")
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("success_24h", func(t *testing.T) {
		svc, _ := buildSvc()
		resp, err := svc.GetRAGSupervisionDashboard(ctx, "24h")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
		if resp.Range != "24h" {
			t.Fatalf("expected range 24h, got %s", resp.Range)
		}
		if len(resp.RAGMetrics) != 4 {
			t.Fatalf("expected 4 RAG metrics, got %d", len(resp.RAGMetrics))
		}
	})

	t.Run("success_7d", func(t *testing.T) {
		svc, _ := buildSvc()
		resp, err := svc.GetRAGSupervisionDashboard(ctx, "7d")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Range != "7d" {
			t.Fatalf("expected range 7d, got %s", resp.Range)
		}
	})

	t.Run("success_30d", func(t *testing.T) {
		svc, _ := buildSvc()
		resp, err := svc.GetRAGSupervisionDashboard(ctx, "30d")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Range != "30d" {
			t.Fatalf("expected range 30d, got %s", resp.Range)
		}
	})

	t.Run("unknown_range_defaults_24h", func(t *testing.T) {
		svc, _ := buildSvc()
		resp, err := svc.GetRAGSupervisionDashboard(ctx, "unknown")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Range != "24h" {
			t.Fatalf("expected range 24h, got %s", resp.Range)
		}
	})
}

// TestSelfLearningService_GetAssetSupervisionDashboard 资产包监督看板
func TestSelfLearningService_GetAssetSupervisionDashboard(t *testing.T) {
	ctx := context.Background()

	buildSvc := func() *SelfLearningService {
		sr := &mockSLSignalRepo{aggAvg: 0.7, aggCount: 50}
		abRepo := &mockSLABTestRepo{
			countByStatusRes: map[model.AssetBundleABTestStatus]int64{
				model.ABTestStatusRunning:   3,
				model.ABTestStatusConverged: 2,
				model.ABTestStatusCompleted: 1,
			},
		}
		sup := selflearning.NewAssetBundleSelfSupervisor(nil, sr, nil, nil, abRepo, nil, nil)
		comps := &SelfLearningComponents{AssetSupervisor: sup}
		return newTestSelfLearningService(comps)
	}

	t.Run("nil_service", func(t *testing.T) {
		var svc *SelfLearningService
		resp, err := svc.GetAssetSupervisionDashboard(ctx, "24h")
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("nil_components", func(t *testing.T) {
		svc := &SelfLearningService{components: nil}
		resp, err := svc.GetAssetSupervisionDashboard(ctx, "24h")
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("nil_asset_supervisor", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		resp, err := svc.GetAssetSupervisionDashboard(ctx, "24h")
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("success_24h", func(t *testing.T) {
		svc := buildSvc()
		resp, err := svc.GetAssetSupervisionDashboard(ctx, "24h")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Range != "24h" {
			t.Fatalf("expected 24h, got %s", resp.Range)
		}
		if len(resp.AssetMetrics) != 5 {
			t.Fatalf("expected 5 asset metrics, got %d", len(resp.AssetMetrics))
		}
		if resp.ABTestSummary == nil {
			t.Fatal("expected non-nil ab test summary")
		}
		if resp.ABTestSummary.TotalCount != 6 {
			t.Fatalf("expected 6 total, got %d", resp.ABTestSummary.TotalCount)
		}
	})

	t.Run("success_7d", func(t *testing.T) {
		svc := buildSvc()
		resp, err := svc.GetAssetSupervisionDashboard(ctx, "7d")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Range != "7d" {
			t.Fatalf("expected 7d, got %s", resp.Range)
		}
	})

	t.Run("success_30d", func(t *testing.T) {
		svc := buildSvc()
		resp, err := svc.GetAssetSupervisionDashboard(ctx, "30d")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Range != "30d" {
			t.Fatalf("expected 30d, got %s", resp.Range)
		}
	})

	t.Run("unknown_range_defaults_24h", func(t *testing.T) {
		svc := buildSvc()
		resp, err := svc.GetAssetSupervisionDashboard(ctx, "")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Range != "24h" {
			t.Fatalf("expected 24h, got %s", resp.Range)
		}
	})
}

// TestSelfLearningService_GetOrchestratorStats Orchestrator 统计
func TestSelfLearningService_GetOrchestratorStats(t *testing.T) {
	t.Run("nil_service", func(t *testing.T) {
		var svc *SelfLearningService
		stats := svc.GetOrchestratorStats()
		if stats.Running {
			t.Fatal("expected not running")
		}
	})

	t.Run("nil_components", func(t *testing.T) {
		svc := &SelfLearningService{components: nil}
		stats := svc.GetOrchestratorStats()
		if stats.Running {
			t.Fatal("expected not running")
		}
	})

	t.Run("nil_orchestrator", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		stats := svc.GetOrchestratorStats()
		if stats.Running {
			t.Fatal("expected not running")
		}
		if stats.MaxConcurrent != 0 {
			t.Fatalf("expected 0 max concurrent, got %d", stats.MaxConcurrent)
		}
	})

	t.Run("success", func(t *testing.T) {
		orch := selflearning.NewOrchestrator(nil, nil, nil, nil, nil, 50, nil, nil)
		comps := &SelfLearningComponents{Orchestrator: orch}
		svc := newTestSelfLearningService(comps)
		stats := svc.GetOrchestratorStats()
		if stats.MaxConcurrent != 50 {
			t.Fatalf("expected 50 max concurrent, got %d", stats.MaxConcurrent)
		}
		if stats.Running {
			t.Fatal("expected not running before Start")
		}
	})
}

// TestSelfLearningService_ListLogs 日志列表查询
func TestSelfLearningService_ListLogs(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_log_repo_returns_empty", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		resp, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
		if len(resp.List) != 0 {
			t.Fatalf("expected 0 items, got %d", len(resp.List))
		}
	})

	t.Run("nil_service_returns_empty", func(t *testing.T) {
		var svc *SelfLearningService
		resp, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
		if len(resp.List) != 0 {
			t.Fatalf("expected 0 items, got %d", len(resp.List))
		}
	})

	t.Run("validate_size_zero_defaults_50", func(t *testing.T) {
		lr := &mockSLLogRepo{listByStatusRes: []*model.SelfLearningLog{makeLog("l-1", model.SelfLearningStatusSuccess)}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{Status: model.SelfLearningStatusSuccess, Size: 0})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Size != 50 {
			t.Fatalf("expected size 50, got %d", resp.Size)
		}
	})

	t.Run("validate_size_over_500_defaults_50", func(t *testing.T) {
		lr := &mockSLLogRepo{listByStatusRes: []*model.SelfLearningLog{makeLog("l-1", model.SelfLearningStatusSuccess)}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{Status: model.SelfLearningStatusSuccess, Size: 999})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Size != 50 {
			t.Fatalf("expected size 50, got %d", resp.Size)
		}
	})

	t.Run("validate_page_zero_defaults_1", func(t *testing.T) {
		lr := &mockSLLogRepo{listByStatusRes: []*model.SelfLearningLog{}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{Status: model.SelfLearningStatusSuccess, Page: 0})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Page != 1 {
			t.Fatalf("expected page 1, got %d", resp.Page)
		}
	})

	t.Run("status_filter_success", func(t *testing.T) {
		lr := &mockSLLogRepo{listByStatusRes: []*model.SelfLearningLog{
			makeLog("l-1", model.SelfLearningStatusFailed),
			makeLog("l-2", model.SelfLearningStatusFailed),
		}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{Status: model.SelfLearningStatusFailed, Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.List) != 2 {
			t.Fatalf("expected 2 items, got %d", len(resp.List))
		}
		if lr.listByStatusCalls != 1 {
			t.Fatalf("expected 1 ListByStatus call, got %d", lr.listByStatusCalls)
		}
	})

	t.Run("status_filter_error", func(t *testing.T) {
		lr := &mockSLLogRepo{listByStatusErr: errors.New("db error")}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{Status: model.SelfLearningStatusFailed, Page: 1, Size: 10})
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("scenario_filter_with_since", func(t *testing.T) {
		lr := &mockSLLogRepo{listByScenarioRes: []*model.SelfLearningLog{makeLog("l-1", model.SelfLearningStatusSuccess)}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{
			Scenario: model.ScenarioRagWarmup,
			Since:    time.Now().Add(-24 * time.Hour),
			Page:     1, Size: 10,
		})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.List) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.List))
		}
		if lr.listByScenarioCalls != 1 {
			t.Fatalf("expected 1 ListByScenario call, got %d", lr.listByScenarioCalls)
		}
	})

	t.Run("scenario_filter_without_since_defaults_7d", func(t *testing.T) {
		lr := &mockSLLogRepo{listByScenarioRes: []*model.SelfLearningLog{}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		_, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{
			Scenario: model.ScenarioRagReflect,
			Page:     1, Size: 10,
		})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if lr.listByScenarioCalls != 1 {
			t.Fatalf("expected 1 ListByScenario call, got %d", lr.listByScenarioCalls)
		}
	})

	t.Run("default_query_no_filters", func(t *testing.T) {
		lr := &mockSLLogRepo{listByScenarioRes: []*model.SelfLearningLog{makeLog("l-1", model.SelfLearningStatusSuccess)}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.List) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.List))
		}
		if lr.listByScenarioCalls != 1 {
			t.Fatalf("expected 1 ListByScenario call (default), got %d", lr.listByScenarioCalls)
		}
	})

	t.Run("scenario_filter_error", func(t *testing.T) {
		lr := &mockSLLogRepo{listByScenarioErr: errors.New("db error")}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, logRepo: lr}
		resp, err := svc.ListLogs(ctx, &dto.SelfLearningLogListRequest{Scenario: model.ScenarioRagWarmup, Page: 1, Size: 10})
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})
}

// TestSelfLearningService_ListCandidates 候选列表查询
func TestSelfLearningService_ListCandidates(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_candidate_repo_returns_empty", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		resp, err := svc.ListCandidates(ctx, &dto.CandidateListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
		if len(resp.List) != 0 {
			t.Fatalf("expected 0 items, got %d", len(resp.List))
		}
	})

	t.Run("nil_service_returns_empty", func(t *testing.T) {
		var svc *SelfLearningService
		resp, err := svc.ListCandidates(ctx, &dto.CandidateListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
	})

	t.Run("validate_size_zero_defaults_50", func(t *testing.T) {
		cr := &mockSLCandidateRepo{listByStatusRes: []*model.AssetBundleCandidate{}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, candidateRepo: cr}
		resp, err := svc.ListCandidates(ctx, &dto.CandidateListRequest{Size: 0})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Size != 50 {
			t.Fatalf("expected size 50, got %d", resp.Size)
		}
	})

	t.Run("status_filter_success", func(t *testing.T) {
		cr := &mockSLCandidateRepo{listByStatusRes: []*model.AssetBundleCandidate{
			makeCandidate("c-1", model.CandidateStatusPromoted),
		}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, candidateRepo: cr}
		resp, err := svc.ListCandidates(ctx, &dto.CandidateListRequest{Status: "promoted", Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.List) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.List))
		}
	})

	t.Run("status_filter_error", func(t *testing.T) {
		cr := &mockSLCandidateRepo{listByStatusErr: errors.New("db error")}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, candidateRepo: cr}
		resp, err := svc.ListCandidates(ctx, &dto.CandidateListRequest{Status: "promoted", Page: 1, Size: 10})
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("default_query_uses_candidate_status", func(t *testing.T) {
		cr := &mockSLCandidateRepo{listByStatusRes: []*model.AssetBundleCandidate{
			makeCandidate("c-1", model.CandidateStatusCandidate),
			makeCandidate("c-2", model.CandidateStatusCandidate),
		}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, candidateRepo: cr}
		resp, err := svc.ListCandidates(ctx, &dto.CandidateListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.List) != 2 {
			t.Fatalf("expected 2 items, got %d", len(resp.List))
		}
	})

	t.Run("default_query_error", func(t *testing.T) {
		cr := &mockSLCandidateRepo{listByStatusErr: errors.New("db error")}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, candidateRepo: cr}
		_, err := svc.ListCandidates(ctx, &dto.CandidateListRequest{Page: 1, Size: 10})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("validate_page_zero_defaults_1", func(t *testing.T) {
		cr := &mockSLCandidateRepo{listByStatusRes: []*model.AssetBundleCandidate{}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, candidateRepo: cr}
		resp, err := svc.ListCandidates(ctx, &dto.CandidateListRequest{Page: 0, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Page != 1 {
			t.Fatalf("expected page 1, got %d", resp.Page)
		}
	})
}

// TestSelfLearningService_ListABTests AB 实验列表查询
func TestSelfLearningService_ListABTests(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_abtest_repo_returns_empty", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		resp, err := svc.ListABTests(ctx, &dto.ABTestListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
		if len(resp.List) != 0 {
			t.Fatalf("expected 0 items, got %d", len(resp.List))
		}
	})

	t.Run("nil_service_returns_empty", func(t *testing.T) {
		var svc *SelfLearningService
		resp, err := svc.ListABTests(ctx, &dto.ABTestListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
	})

	t.Run("validate_size_zero_defaults_50", func(t *testing.T) {
		ar := &mockSLABTestRepo{listByStatusRes: []*model.AssetBundleABTest{}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, abTestRepo: ar}
		resp, err := svc.ListABTests(ctx, &dto.ABTestListRequest{Size: 0})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Size != 50 {
			t.Fatalf("expected size 50, got %d", resp.Size)
		}
	})

	t.Run("status_filter_success", func(t *testing.T) {
		ar := &mockSLABTestRepo{listByStatusRes: []*model.AssetBundleABTest{
			makeABTest("e-1", model.ABTestStatusConverged),
		}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, abTestRepo: ar}
		resp, err := svc.ListABTests(ctx, &dto.ABTestListRequest{Status: "converged", Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.List) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.List))
		}
	})

	t.Run("status_filter_error", func(t *testing.T) {
		ar := &mockSLABTestRepo{listByStatusErr: errors.New("db error")}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, abTestRepo: ar}
		resp, err := svc.ListABTests(ctx, &dto.ABTestListRequest{Status: "converged", Page: 1, Size: 10})
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("default_query_uses_running_status", func(t *testing.T) {
		ar := &mockSLABTestRepo{listByStatusRes: []*model.AssetBundleABTest{
			makeABTest("e-1", model.ABTestStatusRunning),
		}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, abTestRepo: ar}
		resp, err := svc.ListABTests(ctx, &dto.ABTestListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.List) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.List))
		}
	})

	t.Run("default_query_error", func(t *testing.T) {
		ar := &mockSLABTestRepo{listByStatusErr: errors.New("db error")}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, abTestRepo: ar}
		_, err := svc.ListABTests(ctx, &dto.ABTestListRequest{Page: 1, Size: 10})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("validate_page_zero_defaults_1", func(t *testing.T) {
		ar := &mockSLABTestRepo{listByStatusRes: []*model.AssetBundleABTest{}}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, abTestRepo: ar}
		resp, err := svc.ListABTests(ctx, &dto.ABTestListRequest{Page: 0, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Page != 1 {
			t.Fatalf("expected page 1, got %d", resp.Page)
		}
	})
}

// TestSelfLearningService_PromoteABTest 晋升 A/B 实验
func TestSelfLearningService_PromoteABTest(t *testing.T) {
	ctx := context.Background()

	buildSvcWithDispatcher := func(ar *mockSLActionRepo) *SelfLearningService {
		d := selflearning.NewSelfCorrectionDispatcher(nil, ar, nil, nil, nil, nil, nil)
		comps := &SelfLearningComponents{Dispatcher: d}
		return newTestSelfLearningService(comps)
	}

	t.Run("nil_service", func(t *testing.T) {
		var svc *SelfLearningService
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "candidate"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil_components", func(t *testing.T) {
		svc := &SelfLearningService{components: nil}
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "candidate"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil_dispatcher", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "candidate"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("validate_fail_empty_experiment_id", func(t *testing.T) {
		svc := buildSvcWithDispatcher(&mockSLActionRepo{})
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "", WinnerArm: "candidate"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("validate_fail_invalid_winner_arm", func(t *testing.T) {
		svc := buildSvcWithDispatcher(&mockSLActionRepo{})
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "invalid"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("baseline_winner_reject_success", func(t *testing.T) {
		ar := &mockSLActionRepo{}
		svc := buildSvcWithDispatcher(ar)
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "baseline", OperatorID: 1, Note: "rollback"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if ar.updateCalls != 1 {
			t.Fatalf("expected 1 update call, got %d", ar.updateCalls)
		}
	})

	t.Run("baseline_winner_reject_error", func(t *testing.T) {
		ar := &mockSLActionRepo{updateErr: errors.New("db error")}
		svc := buildSvcWithDispatcher(ar)
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "baseline"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("candidate_winner_approve_success_asset_promote", func(t *testing.T) {
		ar := &mockSLActionRepo{
			getByIDAction: makeAction("e-1", model.CorrectionAssetPromote, model.CorrectionStatusPending),
		}
		svc := buildSvcWithDispatcher(ar)
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "candidate", OperatorID: 1, Note: "promote"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if ar.getByIDCalls != 1 {
			t.Fatalf("expected 1 GetByID call, got %d", ar.getByIDCalls)
		}
		if ar.updateCalls != 1 {
			t.Fatalf("expected 1 update call, got %d", ar.updateCalls)
		}
	})

	t.Run("candidate_winner_approve_fail_not_pending", func(t *testing.T) {
		ar := &mockSLActionRepo{
			getByIDAction: makeAction("e-1", model.CorrectionAssetPromote, model.CorrectionStatusApplied),
		}
		svc := buildSvcWithDispatcher(ar)
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "candidate"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("candidate_winner_approve_fail_get_by_id_error", func(t *testing.T) {
		ar := &mockSLActionRepo{getByIDErr: errors.New("db error")}
		svc := buildSvcWithDispatcher(ar)
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "candidate"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("candidate_winner_approve_fail_execute_llm_correction_nil_corrector", func(t *testing.T) {
		// llm_correction action with nil llmCorrector → executeAction fails
		ar := &mockSLActionRepo{
			getByIDAction: makeAction("e-1", model.CorrectionLLMCorrection, model.CorrectionStatusPending),
		}
		svc := buildSvcWithDispatcher(ar)
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "candidate"})
		if err == nil {
			t.Fatal("expected error (llm corrector is nil)")
		}
		// ApproveAction should call UpdateStatus(failed) on execute failure
		if ar.updateCalls != 1 {
			t.Fatalf("expected 1 update call (failed), got %d", ar.updateCalls)
		}
	})

	t.Run("candidate_winner_approve_success_asset_rollback", func(t *testing.T) {
		ar := &mockSLActionRepo{
			getByIDAction: makeAction("e-1", model.CorrectionAssetRollback, model.CorrectionStatusPending),
		}
		svc := buildSvcWithDispatcher(ar)
		err := svc.PromoteABTest(ctx, &dto.ABTestPromoteRequest{ExperimentID: "e-1", WinnerArm: "candidate"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})
}

// TestSelfLearningService_ListCorrections 矫正动作列表查询
func TestSelfLearningService_ListCorrections(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_action_repo_returns_empty", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		resp, err := svc.ListCorrections(ctx, &dto.CorrectionListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
		if len(resp.List) != 0 {
			t.Fatalf("expected 0 items, got %d", len(resp.List))
		}
	})

	t.Run("nil_service_returns_empty", func(t *testing.T) {
		var svc *SelfLearningService
		resp, err := svc.ListCorrections(ctx, &dto.CorrectionListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil resp")
		}
	})

	t.Run("validate_size_zero_defaults_50", func(t *testing.T) {
		ar := &mockSLActionRepo{}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, actionRepo: ar}
		resp, err := svc.ListCorrections(ctx, &dto.CorrectionListRequest{Size: 0})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Size != 50 {
			t.Fatalf("expected size 50, got %d", resp.Size)
		}
	})

	t.Run("validate_page_zero_defaults_1", func(t *testing.T) {
		ar := &mockSLActionRepo{}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, actionRepo: ar}
		resp, err := svc.ListCorrections(ctx, &dto.CorrectionListRequest{Page: 0, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Page != 1 {
			t.Fatalf("expected page 1, got %d", resp.Page)
		}
	})

	t.Run("filter_empty_success", func(t *testing.T) {
		ar := &mockSLActionRepo{
			listByFilterRes:   []*model.SelfCorrectionAction{},
			listByFilterTotal: 0,
		}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, actionRepo: ar}
		resp, err := svc.ListCorrections(ctx, &dto.CorrectionListRequest{Page: 1, Size: 10})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.Total != 0 {
			t.Fatalf("expected 0 total, got %d", resp.Total)
		}
	})

	t.Run("filter_with_all_fields", func(t *testing.T) {
		ar := &mockSLActionRepo{
			listByFilterRes: []*model.SelfCorrectionAction{
				makeAction("a-1", model.CorrectionRetrieveRetry, model.CorrectionStatusApplied),
			},
			listByFilterTotal: 1,
		}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, actionRepo: ar}
		resp, err := svc.ListCorrections(ctx, &dto.CorrectionListRequest{
			ActionType: "retrieve_retry",
			TargetType: "rag_chunk",
			Status:     "applied",
			Since:      time.Now().Add(-24 * time.Hour),
			Until:      time.Now(),
			Page:       1, Size: 10,
		})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.List) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.List))
		}
		if resp.Total != 1 {
			t.Fatalf("expected 1 total, got %d", resp.Total)
		}
		if ar.listByFilterCalls != 1 {
			t.Fatalf("expected 1 ListByFilter call, got %d", ar.listByFilterCalls)
		}
	})

	t.Run("filter_error", func(t *testing.T) {
		ar := &mockSLActionRepo{listByFilterErr: errors.New("db error")}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, actionRepo: ar}
		resp, err := svc.ListCorrections(ctx, &dto.CorrectionListRequest{Page: 1, Size: 10})
		if err == nil {
			t.Fatal("expected error")
		}
		if resp != nil {
			t.Fatal("expected nil resp")
		}
	})

	t.Run("multiple_actions_total_correct", func(t *testing.T) {
		ar := &mockSLActionRepo{
			listByFilterRes: []*model.SelfCorrectionAction{
				makeAction("a-1", model.CorrectionRetrieveRetry, model.CorrectionStatusApplied),
				makeAction("a-2", model.CorrectionQueryRewrite, model.CorrectionStatusApplied),
				makeAction("a-3", model.CorrectionChunkArchive, model.CorrectionStatusApplied),
			},
			listByFilterTotal: 100,
		}
		svc := &SelfLearningService{components: &SelfLearningComponents{}, actionRepo: ar}
		resp, err := svc.ListCorrections(ctx, &dto.CorrectionListRequest{Page: 2, Size: 3})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(resp.List) != 3 {
			t.Fatalf("expected 3 items, got %d", len(resp.List))
		}
		if resp.Total != 100 {
			t.Fatalf("expected total 100, got %d", resp.Total)
		}
		if resp.Page != 2 {
			t.Fatalf("expected page 2, got %d", resp.Page)
		}
	})
}

// TestSelfLearningService_ApproveCorrection 批准矫正动作
func TestSelfLearningService_ApproveCorrection(t *testing.T) {
	ctx := context.Background()

	buildSvc := func(ar *mockSLActionRepo) *SelfLearningService {
		d := selflearning.NewSelfCorrectionDispatcher(nil, ar, nil, nil, nil, nil, nil)
		return newTestSelfLearningService(&SelfLearningComponents{Dispatcher: d})
	}

	t.Run("nil_service", func(t *testing.T) {
		var svc *SelfLearningService
		err := svc.ApproveCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil_components", func(t *testing.T) {
		svc := &SelfLearningService{components: nil}
		err := svc.ApproveCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil_dispatcher", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		err := svc.ApproveCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("validate_fail_empty_action_id", func(t *testing.T) {
		svc := buildSvc(&mockSLActionRepo{})
		err := svc.ApproveCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: ""})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("success_asset_promote", func(t *testing.T) {
		ar := &mockSLActionRepo{
			getByIDAction: makeAction("a-1", model.CorrectionAssetPromote, model.CorrectionStatusPending),
		}
		svc := buildSvc(ar)
		err := svc.ApproveCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1", OperatorID: 1, Reason: "approved"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if ar.getByIDCalls != 1 {
			t.Fatalf("expected 1 GetByID call, got %d", ar.getByIDCalls)
		}
		if ar.updateCalls != 1 {
			t.Fatalf("expected 1 update call, got %d", ar.updateCalls)
		}
	})

	t.Run("fail_not_pending", func(t *testing.T) {
		ar := &mockSLActionRepo{
			getByIDAction: makeAction("a-1", model.CorrectionAssetPromote, model.CorrectionStatusApplied),
		}
		svc := buildSvc(ar)
		err := svc.ApproveCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("fail_get_by_id_error", func(t *testing.T) {
		ar := &mockSLActionRepo{getByIDErr: errors.New("db error")}
		svc := buildSvc(ar)
		err := svc.ApproveCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("fail_execute_update_status_failed_error", func(t *testing.T) {
		// llm_correction with nil corrector → execute fails → UpdateStatus(failed) also fails
		ar := &mockSLActionRepo{
			getByIDAction: makeAction("a-1", model.CorrectionLLMCorrection, model.CorrectionStatusPending),
			updateErr:     errors.New("db error"),
		}
		svc := buildSvc(ar)
		err := svc.ApproveCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestSelfLearningService_RejectCorrection 拒绝矫正动作
func TestSelfLearningService_RejectCorrection(t *testing.T) {
	ctx := context.Background()

	buildSvc := func(ar *mockSLActionRepo) *SelfLearningService {
		d := selflearning.NewSelfCorrectionDispatcher(nil, ar, nil, nil, nil, nil, nil)
		return newTestSelfLearningService(&SelfLearningComponents{Dispatcher: d})
	}

	t.Run("nil_service", func(t *testing.T) {
		var svc *SelfLearningService
		err := svc.RejectCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil_components", func(t *testing.T) {
		svc := &SelfLearningService{components: nil}
		err := svc.RejectCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil_dispatcher", func(t *testing.T) {
		svc := newTestSelfLearningService(&SelfLearningComponents{})
		err := svc.RejectCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("validate_fail_empty_action_id", func(t *testing.T) {
		svc := buildSvc(&mockSLActionRepo{})
		err := svc.RejectCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: ""})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("success", func(t *testing.T) {
		ar := &mockSLActionRepo{}
		svc := buildSvc(ar)
		err := svc.RejectCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1", OperatorID: 1, Reason: "rejected"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if ar.updateCalls != 1 {
			t.Fatalf("expected 1 update call, got %d", ar.updateCalls)
		}
	})

	t.Run("fail_update_status_error", func(t *testing.T) {
		ar := &mockSLActionRepo{updateErr: errors.New("db error")}
		svc := buildSvc(ar)
		err := svc.RejectCorrection(ctx, &dto.CorrectionRollbackRequest{ActionID: "a-1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestSelfLearningService_DTOConversions DTO 转换辅助函数
func TestSelfLearningService_DTOConversions(t *testing.T) {
	t.Run("toLogResponse_nil", func(t *testing.T) {
		if r := toLogResponse(nil); r != nil {
			t.Fatal("expected nil")
		}
	})

	t.Run("toLogResponse_success", func(t *testing.T) {
		lg := makeLog("l-1", model.SelfLearningStatusSuccess)
		lg.DurationMs = 150
		lg.InputSummary = model.JSONMap{"key": "val"}
		r := toLogResponse(lg)
		if r == nil {
			t.Fatal("expected non-nil")
		}
		if r.LogID != "l-1" {
			t.Fatalf("expected l-1, got %s", r.LogID)
		}
		if r.DurationMs != 150 {
			t.Fatalf("expected 150, got %d", r.DurationMs)
		}
	})

	t.Run("toCandidateResponse_nil", func(t *testing.T) {
		if r := toCandidateResponse(nil); r != nil {
			t.Fatal("expected nil")
		}
	})

	t.Run("toCandidateResponse_success", func(t *testing.T) {
		c := makeCandidate("c-1", model.CandidateStatusCandidate)
		c.SourceSessionIDs = []string{"s-1", "s-2"}
		c.ExtractedScripts = model.JSONMap{"title": "test"}
		c.ProposedMessages = model.AssetBundleMessages{}
		r := toCandidateResponse(c)
		if r == nil {
			t.Fatal("expected non-nil")
		}
		if r.CandidateID != "c-1" {
			t.Fatalf("expected c-1, got %s", r.CandidateID)
		}
		if len(r.SourceSessionIDs) != 2 {
			t.Fatalf("expected 2 source session ids, got %d", len(r.SourceSessionIDs))
		}
	})

	t.Run("toCandidateResponse_nil_scripts", func(t *testing.T) {
		c := makeCandidate("c-1", model.CandidateStatusCandidate)
		c.ExtractedScripts = nil
		r := toCandidateResponse(c)
		if r == nil {
			t.Fatal("expected non-nil")
		}
		if r.ExtractedScripts != nil {
			t.Fatalf("expected nil scripts, got %v", r.ExtractedScripts)
		}
	})

	t.Run("toABTestResponse_nil", func(t *testing.T) {
		if r := toABTestResponse(nil); r != nil {
			t.Fatal("expected nil")
		}
	})

	t.Run("toABTestResponse_success", func(t *testing.T) {
		at := makeABTest("e-1", model.ABTestStatusRunning)
		at.BaselineSamples = 50
		at.CandidateSamples = 45
		r := toABTestResponse(at)
		if r == nil {
			t.Fatal("expected non-nil")
		}
		if r.ExperimentID != "e-1" {
			t.Fatalf("expected e-1, got %s", r.ExperimentID)
		}
		if r.BaselineSamples != 50 {
			t.Fatalf("expected 50, got %d", r.BaselineSamples)
		}
	})

	t.Run("toCorrectionItem_nil", func(t *testing.T) {
		if r := toCorrectionItem(nil); r != nil {
			t.Fatal("expected nil")
		}
	})

	t.Run("toCorrectionItem_success", func(t *testing.T) {
		a := makeAction("a-1", model.CorrectionRetrieveRetry, model.CorrectionStatusApplied)
		a.Operator = "user1"
		a.AutonomyLevel = model.AutonomyLevelSupervised
		r := toCorrectionItem(a)
		if r == nil {
			t.Fatal("expected non-nil")
		}
		if r.ActionID != "a-1" {
			t.Fatalf("expected a-1, got %s", r.ActionID)
		}
		if r.ActionType != "retrieve_retry" {
			t.Fatalf("expected retrieve_retry, got %s", r.ActionType)
		}
		if r.Status != "applied" {
			t.Fatalf("expected applied, got %s", r.Status)
		}
		if r.Operator != "user1" {
			t.Fatalf("expected user1, got %s", r.Operator)
		}
	})
}
