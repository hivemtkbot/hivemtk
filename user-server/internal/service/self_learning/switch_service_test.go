package selflearning

// switch_service_test.go SwitchService 单元测试
//
// 测试策略：
//   - 使用标准库 testing，table-driven + t.Run 子测试
//   - 自实现 mockSwitchRepo / mockLogRepo（不依赖 DB）
//   - 覆盖：GetStatus 缓存命中/未命中、UpdateSwitch 各自治等级转换、
//     CheckGuardrail 7×3×2×2×2 组合、RecordCorrectionAction 配额扣减、
//     EvaluateCircuit 熔断触发/半开恢复、ResetDailyCounters、
//     ShouldExecuteAction、IsAutonomous/IsSupervised/IsManual

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
)

// ============================================================================
// Mock Repositories
// ============================================================================

// mockSwitchRepo SelfLearningSwitchRepository 的内存 mock 实现
type mockSwitchRepo struct {
	mu sync.Mutex
	sw *model.SelfLearningSwitch

	// 可配置错误
	getErr                  error
	getOrCreateErr          error
	updateErr               error
	incrementCorrectionsErr error
	incrementPromotionsErr  error
	resetDailyErr           error
	markTriggeredErr        error
	setCircuitOpenErr       error

	// 调用计数与记录值
	getCalls                  int
	getOrCreateCalls          int
	updateCalls               int
	incrementCorrectionsCalls int
	incrementCorrectionsSum   int
	incrementPromotionsCalls  int
	incrementPromotionsSum    int
	resetDailyCalls           int
	markTriggeredCalls        int
	setCircuitOpenCalls       int
	setCircuitOpenValues      []bool
}

func (m *mockSwitchRepo) Get(ctx context.Context) (*model.SelfLearningSwitch, error) {
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

func (m *mockSwitchRepo) GetOrCreate(ctx context.Context) (*model.SelfLearningSwitch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreateCalls++
	if m.getOrCreateErr != nil {
		return nil, m.getOrCreateErr
	}
	if m.sw == nil {
		m.sw = defaultSwitch()
	}
	cp := *m.sw
	return &cp, nil
}

func (m *mockSwitchRepo) Update(ctx context.Context, s *model.SelfLearningSwitch) error {
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

func (m *mockSwitchRepo) IncrementTodayCorrections(ctx context.Context, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incrementCorrectionsCalls++
	m.incrementCorrectionsSum += delta
	if m.incrementCorrectionsErr != nil {
		return m.incrementCorrectionsErr
	}
	if m.sw != nil {
		m.sw.TodayCorrections += delta
	}
	return nil
}

func (m *mockSwitchRepo) IncrementTodayPromotions(ctx context.Context, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incrementPromotionsCalls++
	m.incrementPromotionsSum += delta
	if m.incrementPromotionsErr != nil {
		return m.incrementPromotionsErr
	}
	if m.sw != nil {
		m.sw.TodayPromotions += delta
	}
	return nil
}

func (m *mockSwitchRepo) ResetDailyCounters(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetDailyCalls++
	if m.resetDailyErr != nil {
		return m.resetDailyErr
	}
	if m.sw != nil {
		now := time.Now()
		m.sw.TodayCorrections = 0
		m.sw.TodayPromotions = 0
		m.sw.TodayResetAt = &now
	}
	return nil
}

func (m *mockSwitchRepo) MarkTriggered(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markTriggeredCalls++
	if m.markTriggeredErr != nil {
		return m.markTriggeredErr
	}
	if m.sw != nil {
		now := time.Now()
		m.sw.LastTriggeredAt = &now
	}
	return nil
}

func (m *mockSwitchRepo) SetCircuitOpen(ctx context.Context, open bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCircuitOpenCalls++
	m.setCircuitOpenValues = append(m.setCircuitOpenValues, open)
	if m.setCircuitOpenErr != nil {
		return m.setCircuitOpenErr
	}
	if m.sw != nil {
		m.sw.CircuitOpen = open
	}
	return nil
}

// mockLogRepo SelfLearningLogRepository 的内存 mock 实现
//
// SwitchService 当前未直接调用 logRepo 方法，但构造函数需要此依赖；
// 完整实现接口以便未来扩展且满足接口约束。
// 所有方法均加锁，支持并发调用（Orchestrator 会并发 spawn 多个协程共享同一 mock）。
type mockLogRepo struct {
	mu sync.Mutex

	createErr          error
	existsErr          error
	updateStatusErr    error
	getByLogIDErr      error
	listByScenarioErr  error
	listByStatusErr    error
	countTodayErr      error
	markStaleErr       error

	createCalls       int
	existsCalls       int
	existsResult      bool
	getByLogIDLog     *model.SelfLearningLog
	markStaleCalls    int
	markStaleResult   int64
}

func (m *mockLogRepo) Create(ctx context.Context, log *model.SelfLearningLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	return m.createErr
}

func (m *mockLogRepo) ExistsBySessionAndScenario(ctx context.Context, sessionID string, scenario model.SelfLearningScenario) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.existsCalls++
	return m.existsResult, m.existsErr
}

func (m *mockLogRepo) UpdateStatus(ctx context.Context, logID string, status model.SelfLearningStatus, errMsg string, outputSummary model.JSONMap, durationMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateStatusErr
}

func (m *mockLogRepo) GetByLogID(ctx context.Context, logID string) (*model.SelfLearningLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getByLogIDErr != nil {
		return nil, m.getByLogIDErr
	}
	return m.getByLogIDLog, nil
}

func (m *mockLogRepo) ListByScenario(ctx context.Context, scenario model.SelfLearningScenario, since time.Time, limit int) ([]*model.SelfLearningLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil, m.listByScenarioErr
}

func (m *mockLogRepo) ListByStatus(ctx context.Context, status model.SelfLearningStatus, limit int) ([]*model.SelfLearningLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil, m.listByStatusErr
}

func (m *mockLogRepo) CountToday(ctx context.Context) (map[model.SelfLearningStatus]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil, m.countTodayErr
}

func (m *mockLogRepo) MarkStaleLogsAsSkipped(ctx context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markStaleCalls++
	return m.markStaleResult, m.markStaleErr
}

// 编译期接口断言
var (
	_ = func() interface{ Get(context.Context) (*model.SelfLearningSwitch, error) } {
		return &mockSwitchRepo{}
	}
)

// ============================================================================
// 辅助函数
// ============================================================================

func defaultSwitch() *model.SelfLearningSwitch {
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

func newTestService(cacheExp time.Duration) (*SwitchService, *mockSwitchRepo, *mockLogRepo) {
	sr := &mockSwitchRepo{sw: defaultSwitch()}
	lr := &mockLogRepo{}
	svc := NewSwitchService(sr, lr, cacheExp)
	return svc, sr, lr
}

// setCache 直接写入缓存（同包访问），并同步配额计数
func setCache(svc *SwitchService, snap *SwitchSnapshot, quotaCorr, quotaPromo int64) {
	svc.cacheMu.Lock()
	snap.UpdatedAt = time.Now()
	svc.cached = snap
	svc.cachedAt = time.Now()
	svc.cacheMu.Unlock()
	svc.quotaTodayCorrections.Store(quotaCorr)
	svc.quotaTodayPromotions.Store(quotaPromo)
}

// allActionTypes 7 种矫正动作类型
func allActionTypes() []model.CorrectionActionType {
	return []model.CorrectionActionType{
		model.CorrectionRetrieveRetry,
		model.CorrectionQueryRewrite,
		model.CorrectionChunkArchive,
		model.CorrectionChampionUpsert,
		model.CorrectionAssetPromote,
		model.CorrectionAssetRollback,
		model.CorrectionLLMCorrection,
	}
}

// expectedGuardrailReasons 复现 CheckGuardrail 的拦截原因计算逻辑（用于断言）
func expectedGuardrailReasons(action model.CorrectionActionType, snap *SwitchSnapshot, quotaCorr, quotaPromo int) []string {
	var reasons []string
	switch action {
	case model.CorrectionRetrieveRetry, model.CorrectionQueryRewrite,
		model.CorrectionChunkArchive, model.CorrectionChampionUpsert:
		if !snap.EnableRAG {
			reasons = append(reasons, "rag self-learning disabled")
		}
	case model.CorrectionAssetPromote, model.CorrectionAssetRollback:
		if !snap.EnableAsset {
			reasons = append(reasons, "asset self-learning disabled")
		}
	case model.CorrectionLLMCorrection:
		if !snap.EnableLLM {
			reasons = append(reasons, "llm self-correction disabled")
		}
	}
	if snap.CircuitOpen {
		reasons = append(reasons, "circuit breaker open")
	}
	if snap.MaxDailyCorrections > 0 && quotaCorr >= snap.MaxDailyCorrections {
		reasons = append(reasons, "daily correction quota exceeded")
	}
	if action == model.CorrectionAssetPromote && snap.MaxDailyPromotions > 0 && quotaPromo >= snap.MaxDailyPromotions {
		reasons = append(reasons, "daily promotion quota exceeded")
	}
	if snap.AutonomyLevel == model.AutonomyLevelManual {
		reasons = append(reasons, "manual mode forbids auto action")
	}
	return reasons
}

// ============================================================================
// TestSwitchService_GetStatus  缓存命中/未命中（200 组）
// ============================================================================

func TestSwitchService_GetStatus(t *testing.T) {
	type tc struct {
		name      string
		cacheHit  bool
		autonomy  model.AutonomyLevel
		enableRAG bool
		enableAst bool
		enableLLM bool
		open      bool
		maxCorr   int
		maxPromo  int
		todayCorr int
		todayPromo int
	}
	var cases []tc
	autonomies := []model.AutonomyLevel{
		model.AutonomyLevelManual, model.AutonomyLevelSupervised, model.AutonomyLevelAutonomous,
	}
	maxCorrOpts := []int{0, 50, 100, 200, 500}
	maxPromoOpts := []int{0, 5, 10, 20}
	todayCorrOpts := []int{0, 30, 99, 100, 150}
	todayPromoOpts := []int{0, 4, 5, 8}

	// 生成 200 组用例：100 缓存命中 + 100 缓存未命中
	for i := 0; i < 200; i++ {
		c := tc{
			name:       fmt.Sprintf("case_%03d", i),
			cacheHit:   i < 100,
			autonomy:   autonomies[i%3],
			enableRAG:  i%2 == 0,
			enableAst:  i%3 != 0,
			enableLLM:  i%5 < 3,
			open:       i%7 == 0,
			maxCorr:    maxCorrOpts[i%len(maxCorrOpts)],
			maxPromo:   maxPromoOpts[i%len(maxPromoOpts)],
			todayCorr:  todayCorrOpts[i%len(todayCorrOpts)],
			todayPromo: todayPromoOpts[i%len(todayPromoOpts)],
		}
		cases = append(cases, c)
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			svc, sr, _ := newTestService(5 * time.Second)
			// 准备 DB 状态
			sr.sw.AutonomyLevel = c.autonomy
			sr.sw.EnableRAG = c.enableRAG
			sr.sw.EnableAsset = c.enableAst
			sr.sw.EnableLLM = c.enableLLM
			sr.sw.CircuitOpen = c.open
			sr.sw.MaxDailyCorrections = c.maxCorr
			sr.sw.MaxDailyPromotions = c.maxPromo
			sr.sw.TodayCorrections = c.todayCorr
			sr.sw.TodayPromotions = c.todayPromo

			if c.cacheHit {
				// 预填缓存，命中路径
				cachedSnap := svc.toSnapshot(sr.sw)
				setCache(svc, cachedSnap, int64(c.todayCorr), int64(c.todayPromo))
			} else {
				// 不填缓存，强制走 DB；同时让缓存过期
				svc.cacheMu.Lock()
				svc.cached = nil
				svc.cacheMu.Unlock()
			}

			snap, err := svc.GetStatus(context.Background())
			if err != nil {
				t.Fatalf("GetStatus returned error: %v", err)
			}
			if snap.AutonomyLevel != c.autonomy {
				t.Errorf("AutonomyLevel = %v, want %v", snap.AutonomyLevel, c.autonomy)
			}
			if snap.EnableRAG != c.enableRAG {
				t.Errorf("EnableRAG = %v, want %v", snap.EnableRAG, c.enableRAG)
			}
			if snap.EnableAsset != c.enableAst {
				t.Errorf("EnableAsset = %v, want %v", snap.EnableAsset, c.enableAst)
			}
			if snap.EnableLLM != c.enableLLM {
				t.Errorf("EnableLLM = %v, want %v", snap.EnableLLM, c.enableLLM)
			}
			if snap.CircuitOpen != c.open {
				t.Errorf("CircuitOpen = %v, want %v", snap.CircuitOpen, c.open)
			}
			if snap.MaxDailyCorrections != c.maxCorr {
				t.Errorf("MaxDailyCorrections = %v, want %v", snap.MaxDailyCorrections, c.maxCorr)
			}
			if snap.MaxDailyPromotions != c.maxPromo {
				t.Errorf("MaxDailyPromotions = %v, want %v", snap.MaxDailyPromotions, c.maxPromo)
			}
			// TodayCorrections/Promotions 来自原子计数
			if snap.TodayCorrections != c.todayCorr {
				t.Errorf("TodayCorrections = %v, want %v", snap.TodayCorrections, c.todayCorr)
			}
			if snap.TodayPromotions != c.todayPromo {
				t.Errorf("TodayPromotions = %v, want %v", snap.TodayPromotions, c.todayPromo)
			}

			if c.cacheHit {
				if sr.getOrCreateCalls != 0 {
					t.Errorf("cache hit should not call repo; getOrCreateCalls=%d", sr.getOrCreateCalls)
				}
			} else {
				if sr.getOrCreateCalls != 1 {
					t.Errorf("cache miss should call repo once; getOrCreateCalls=%d", sr.getOrCreateCalls)
				}
				// 缓存未命中后应回填缓存
				svc.cacheMu.RLock()
				hasCache := svc.cached != nil
				svc.cacheMu.RUnlock()
				if !hasCache {
					t.Errorf("cache should be populated after DB load")
				}
				// 原子计数应同步
				if int(svc.quotaTodayCorrections.Load()) != c.todayCorr {
					t.Errorf("quotaTodayCorrections = %d, want %d", svc.quotaTodayCorrections.Load(), c.todayCorr)
				}
			}
		})
	}
}

// ============================================================================
// TestSwitchService_UpdateSwitch  各 autonomy_level 转换 + 参数组合
// ============================================================================

func TestSwitchService_UpdateSwitch(t *testing.T) {
	autonomies := []model.AutonomyLevel{
		model.AutonomyLevelManual, model.AutonomyLevelSupervised, model.AutonomyLevelAutonomous,
	}
	enableCombos := [][3]bool{
		{false, false, false},
		{true, false, false},
		{false, true, false},
		{false, false, true},
		{true, true, false},
		{true, false, true},
		{false, true, true},
		{true, true, true},
	}

	type tc struct {
		name    string
		req     *dto.SwitchConfigRequest
		wantErr error
	}

	var cases []tc
	// 3 autonomy × 8 enable 组合 = 24 成功用例
	for _, a := range autonomies {
		for i, ec := range enableCombos {
			cases = append(cases, tc{
				name: fmt.Sprintf("ok_%s_en%d", a, i),
				req: &dto.SwitchConfigRequest{
					AutonomyLevel:           a,
					EnableRAG:               ec[0],
					EnableAsset:             ec[1],
					EnableLLM:               ec[2],
					MaxDailyCorrections:     100,
					MaxDailyPromotions:      5,
					LowQualityThreshold:     3.0,
					ChampionRewardThreshold: 1.5,
					ABTestMinSamples:        100,
					CircuitBreakerThreshold: 0.3,
					CircuitBreakerWindowMin: 30,
				},
			})
		}
	}
	// 不同参数组合
	paramSets := []struct {
		maxCorr, maxPromo, abSamples, windowMin int
		lowQual, champion, cbThreshold          float64
	}{
		{0, 0, 0, 10, 0, 0, 0},
		{500, 50, 1000, 60, 5.0, 2.5, 0.5},
		{1, 1, 1, 1, 0.1, 0.1, 0.1},
		{200, 20, 200, 45, 4.0, 1.8, 0.99},
	}
	for _, a := range autonomies {
		for i, p := range paramSets {
			cases = append(cases, tc{
				name: fmt.Sprintf("ok_%s_params%d", a, i),
				req: &dto.SwitchConfigRequest{
					AutonomyLevel:           a,
					EnableRAG:               true,
					EnableAsset:             true,
					EnableLLM:               true,
					MaxDailyCorrections:     p.maxCorr,
					MaxDailyPromotions:      p.maxPromo,
					LowQualityThreshold:     p.lowQual,
					ChampionRewardThreshold: p.champion,
					ABTestMinSamples:        p.abSamples,
					CircuitBreakerThreshold: p.cbThreshold,
					CircuitBreakerWindowMin: p.windowMin,
				},
			})
		}
	}
	// Validate 失败用例
	cases = append(cases,
		tc{name: "invalid_autonomy", req: &dto.SwitchConfigRequest{AutonomyLevel: "invalid"}, wantErr: dto.ErrInvalidAutonomyLevel},
		tc{name: "negative_max_corr", req: &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, MaxDailyCorrections: -1}, wantErr: dto.ErrInvalidMaxDailyCorrections},
		tc{name: "negative_max_promo", req: &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, MaxDailyPromotions: -1}, wantErr: dto.ErrInvalidMaxDailyPromotions},
		tc{name: "negative_low_qual", req: &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, LowQualityThreshold: -0.1}, wantErr: dto.ErrInvalidLowQualityThreshold},
		tc{name: "negative_champion", req: &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, ChampionRewardThreshold: -1}, wantErr: dto.ErrInvalidChampionRewardThreshold},
		tc{name: "negative_abtest", req: &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, ABTestMinSamples: -1}, wantErr: dto.ErrInvalidABTestMinSamples},
		tc{name: "cb_threshold_gt1", req: &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, CircuitBreakerThreshold: 1.5}, wantErr: dto.ErrInvalidCircuitBreakerThreshold},
		tc{name: "cb_threshold_negative", req: &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual, CircuitBreakerThreshold: -0.1}, wantErr: dto.ErrInvalidCircuitBreakerThreshold},
	)
	// Validate 对 CircuitBreakerWindowMin<=0 会自动设为 30（不报错），单独验证
	cases = append(cases, tc{
		name: "cb_window_zero_defaults_to_30",
		req: &dto.SwitchConfigRequest{
			AutonomyLevel:           model.AutonomyLevelAutonomous,
			EnableRAG:               true,
			CircuitBreakerWindowMin: 0,
		},
	})

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			svc, sr, _ := newTestService(5 * time.Second)
			snap, err := svc.UpdateSwitch(context.Background(), c.req, 42)

			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
				if sr.updateCalls != 0 {
					t.Errorf("update should not be called on validate failure; calls=%d", sr.updateCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateSwitch returned error: %v", err)
			}
			if sr.updateCalls != 1 {
				t.Errorf("updateCalls = %d, want 1", sr.updateCalls)
			}
			// 验证字段写入
			if snap.AutonomyLevel != c.req.AutonomyLevel {
				t.Errorf("AutonomyLevel = %v, want %v", snap.AutonomyLevel, c.req.AutonomyLevel)
			}
			if snap.EnableRAG != c.req.EnableRAG {
				t.Errorf("EnableRAG = %v, want %v", snap.EnableRAG, c.req.EnableRAG)
			}
			if snap.EnableAsset != c.req.EnableAsset {
				t.Errorf("EnableAsset = %v, want %v", snap.EnableAsset, c.req.EnableAsset)
			}
			if snap.EnableLLM != c.req.EnableLLM {
				t.Errorf("EnableLLM = %v, want %v", snap.EnableLLM, c.req.EnableLLM)
			}
			if snap.MaxDailyCorrections != c.req.MaxDailyCorrections {
				t.Errorf("MaxDailyCorrections = %v, want %v", snap.MaxDailyCorrections, c.req.MaxDailyCorrections)
			}
			if snap.MaxDailyPromotions != c.req.MaxDailyPromotions {
				t.Errorf("MaxDailyPromotions = %v, want %v", snap.MaxDailyPromotions, c.req.MaxDailyPromotions)
			}
			if snap.LowQualityThreshold != c.req.LowQualityThreshold {
				t.Errorf("LowQualityThreshold = %v, want %v", snap.LowQualityThreshold, c.req.LowQualityThreshold)
			}
			if snap.ChampionRewardThreshold != c.req.ChampionRewardThreshold {
				t.Errorf("ChampionRewardThreshold = %v, want %v", snap.ChampionRewardThreshold, c.req.ChampionRewardThreshold)
			}
			if snap.ABTestMinSamples != c.req.ABTestMinSamples {
				t.Errorf("ABTestMinSamples = %v, want %v", snap.ABTestMinSamples, c.req.ABTestMinSamples)
			}
			if snap.CircuitBreakerThreshold != c.req.CircuitBreakerThreshold {
				t.Errorf("CircuitBreakerThreshold = %v, want %v", snap.CircuitBreakerThreshold, c.req.CircuitBreakerThreshold)
			}
			// window<=0 时 Validate 自动设为 30
			wantWindow := c.req.CircuitBreakerWindowMin
			if wantWindow <= 0 {
				wantWindow = 30
			}
			if snap.CircuitBreakerWindowMin != wantWindow {
				t.Errorf("CircuitBreakerWindowMin = %v, want %v", snap.CircuitBreakerWindowMin, wantWindow)
			}
			// 缓存应被刷新
			svc.cacheMu.RLock()
			cached := svc.cached
			svc.cacheMu.RUnlock()
			if cached == nil {
				t.Fatalf("cache should be refreshed after UpdateSwitch")
			}
			if cached.AutonomyLevel != c.req.AutonomyLevel {
				t.Errorf("cached.AutonomyLevel = %v, want %v", cached.AutonomyLevel, c.req.AutonomyLevel)
			}
			// UpdatedBy 应记录操作人
			if sr.sw == nil || sr.sw.UpdatedBy != 42 {
				t.Errorf("UpdatedBy not persisted; sw.UpdatedBy=%v", sr.sw)
			}
		})
	}

	// 单独验证 GetOrCreate / Update 错误传播
	t.Run("get_or_create_error", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		sr.getOrCreateErr = errors.New("db down")
		_, err := svc.UpdateSwitch(context.Background(), &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual}, 1)
		if err == nil || err.Error() != "db down" {
			t.Fatalf("err = %v, want db down", err)
		}
	})
	t.Run("update_error", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		sr.updateErr = errors.New("update failed")
		_, err := svc.UpdateSwitch(context.Background(), &dto.SwitchConfigRequest{AutonomyLevel: model.AutonomyLevelManual}, 1)
		if err == nil || err.Error() != "update failed" {
			t.Fatalf("err = %v, want update failed", err)
		}
	})
}

// ============================================================================
// TestSwitchService_CheckGuardrail  7×3×2×2×2 = 168 组组合
// ============================================================================

func TestSwitchService_CheckGuardrail(t *testing.T) {
	autonomies := []model.AutonomyLevel{
		model.AutonomyLevelManual, model.AutonomyLevelSupervised, model.AutonomyLevelAutonomous,
	}
	enableOpts := []bool{true, false}
	// quotaOpts: {quotaCorr, quotaPromo}, maxCorr=100, maxPromo=5
	quotaOpts := []struct{ corr, promo int }{
		{0, 0},   // 未耗尽
		{100, 5}, // 耗尽
	}
	circuitOpts := []bool{false, true}

	type tc struct {
		name     string
		action   model.CorrectionActionType
		autonomy model.AutonomyLevel
		enable   bool
		quota    struct{ corr, promo int }
		open     bool
	}

	var cases []tc
	for _, action := range allActionTypes() {
		for _, a := range autonomies {
			for _, en := range enableOpts {
				for _, q := range quotaOpts {
					for _, op := range circuitOpts {
						c := tc{
							action: action, autonomy: a, enable: en,
							quota: q, open: op,
						}
						c.name = fmt.Sprintf("%s/%s/enable=%v/quotaCorr=%d/quotaPromo=%d/open=%v",
							action, a, en, q.corr, q.promo, op)
						cases = append(cases, c)
					}
				}
			}
		}
	}

	if len(cases) < 100 {
		t.Fatalf("expected >=100 guardrail cases, got %d", len(cases))
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			svc, _, _ := newTestService(5 * time.Second)
			snap := &SwitchSnapshot{
				AutonomyLevel:           c.autonomy,
				EnableRAG:               c.enable,
				EnableAsset:             c.enable,
				EnableLLM:               c.enable,
				CircuitOpen:             c.open,
				MaxDailyCorrections:     100,
				MaxDailyPromotions:      5,
				LowQualityThreshold:     3.0,
				ChampionRewardThreshold: 1.5,
				ABTestMinSamples:        100,
				CircuitBreakerThreshold: 0.3,
				CircuitBreakerWindowMin: 30,
			}
			setCache(svc, snap, int64(c.quota.corr), int64(c.quota.promo))

			g, err := svc.CheckGuardrail(context.Background(), c.action)
			if err != nil {
				t.Fatalf("CheckGuardrail error: %v", err)
			}
			wantReasons := expectedGuardrailReasons(c.action, snap, c.quota.corr, c.quota.promo)
			wantPassed := len(wantReasons) == 0

			if g.Passed != wantPassed {
				t.Errorf("Passed = %v, want %v (reasons=%v, got=%v)",
					g.Passed, wantPassed, wantReasons, g.BlockedReasons)
			}
			if len(g.BlockedReasons) != len(wantReasons) {
				t.Errorf("len(BlockedReasons) = %d, want %d; got=%v want=%v",
					len(g.BlockedReasons), len(wantReasons), g.BlockedReasons, wantReasons)
			} else {
				for i, r := range wantReasons {
					if g.BlockedReasons[i] != r {
						t.Errorf("BlockedReasons[%d] = %q, want %q (full got=%v want=%v)",
							i, g.BlockedReasons[i], r, g.BlockedReasons, wantReasons)
						break
					}
				}
			}
			if g.AutonomyLevel != c.autonomy {
				t.Errorf("AutonomyLevel = %v, want %v", g.AutonomyLevel, c.autonomy)
			}
			if g.CircuitOpen != c.open {
				t.Errorf("CircuitOpen = %v, want %v", g.CircuitOpen, c.open)
			}
			if g.DailyQuotaLimit != 100 {
				t.Errorf("DailyQuotaLimit = %v, want 100", g.DailyQuotaLimit)
			}
			if g.DailyQuotaUsed != c.quota.corr {
				t.Errorf("DailyQuotaUsed = %v, want %v", g.DailyQuotaUsed, c.quota.corr)
			}
		})
	}

	// GetStatus 错误传播
	t.Run("get_status_error", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("db unavailable")
		_, err := svc.CheckGuardrail(context.Background(), model.CorrectionRetrieveRetry)
		if err == nil || err.Error() != "db unavailable" {
			t.Fatalf("err = %v, want db unavailable", err)
		}
	})
}

// ============================================================================
// TestSwitchService_RecordCorrectionAction  配额扣减 + 晋升配额扣减
// ============================================================================

func TestSwitchService_RecordCorrectionAction(t *testing.T) {
	type tc struct {
		name               string
		action             model.CorrectionActionType
		success            bool
		isPromotion        bool
		incrCorrErr        error
		incrPromoErr       error
		markTriggeredErr   error
		wantErr            error
		wantCorrDelta      int   // 期望 IncrementTodayCorrections 调用次数
		wantPromoDelta     int   // 期望 IncrementTodayPromotions 调用次数
		wantMarkTriggered  int   // 期望 MarkTriggered 调用次数
		wantQuotaCorrIncr  int64 // 期望原子计数增量
		wantQuotaPromoIncr int64
		wantBreakerEntries int // 期望 breakerWindow 条目数
	}

	cases := []tc{
		{
			name: "success_not_promotion", action: model.CorrectionRetrieveRetry,
			success: true, isPromotion: false,
			wantCorrDelta: 1, wantMarkTriggered: 1,
			wantQuotaCorrIncr: 1, wantBreakerEntries: 1,
		},
		{
			name: "failure_not_promotion", action: model.CorrectionQueryRewrite,
			success: false, isPromotion: false,
			wantCorrDelta: 1, wantMarkTriggered: 1,
			wantQuotaCorrIncr: 1, wantBreakerEntries: 1,
		},
		{
			name: "success_promotion", action: model.CorrectionAssetPromote,
			success: true, isPromotion: true,
			wantCorrDelta: 1, wantPromoDelta: 1, wantMarkTriggered: 1,
			wantQuotaCorrIncr: 1, wantQuotaPromoIncr: 1, wantBreakerEntries: 1,
		},
		{
			name: "failure_promotion", action: model.CorrectionAssetPromote,
			success: false, isPromotion: true,
			wantCorrDelta: 1, wantPromoDelta: 1, wantMarkTriggered: 1,
			wantQuotaCorrIncr: 1, wantQuotaPromoIncr: 1, wantBreakerEntries: 1,
		},
		{
			name: "llm_correction_success", action: model.CorrectionLLMCorrection,
			success: true, isPromotion: false,
			wantCorrDelta: 1, wantMarkTriggered: 1,
			wantQuotaCorrIncr: 1, wantBreakerEntries: 1,
		},
		{
			name: "chunk_archive_failure", action: model.CorrectionChunkArchive,
			success: false, isPromotion: false,
			wantCorrDelta: 1, wantMarkTriggered: 1,
			wantQuotaCorrIncr: 1, wantBreakerEntries: 1,
		},
		{
			name: "champion_upsert_success", action: model.CorrectionChampionUpsert,
			success: true, isPromotion: false,
			wantCorrDelta: 1, wantMarkTriggered: 1,
			wantQuotaCorrIncr: 1, wantBreakerEntries: 1,
		},
		{
			name: "asset_rollback", action: model.CorrectionAssetRollback,
			success: true, isPromotion: false,
			wantCorrDelta: 1, wantMarkTriggered: 1,
			wantQuotaCorrIncr: 1, wantBreakerEntries: 1,
		},
		{
			name: "incr_corr_error", action: model.CorrectionRetrieveRetry,
			success: true, isPromotion: false, incrCorrErr: errors.New("corr db error"),
			wantErr:           errors.New("corr db error"),
			wantCorrDelta:     1, wantMarkTriggered: 0,
			wantQuotaCorrIncr: 0, wantBreakerEntries: 0,
		},
		{
			name: "incr_promo_error", action: model.CorrectionAssetPromote,
			success: true, isPromotion: true, incrPromoErr: errors.New("promo db error"),
			wantErr:            errors.New("promo db error"),
			wantCorrDelta:      1, wantPromoDelta: 1, wantMarkTriggered: 0,
			wantQuotaCorrIncr:  1, wantQuotaPromoIncr: 0, wantBreakerEntries: 0,
		},
		{
			name: "mark_triggered_error", action: model.CorrectionRetrieveRetry,
			success: true, isPromotion: false, markTriggeredErr: errors.New("mark error"),
			wantErr:           errors.New("mark error"),
			wantCorrDelta:     1, wantMarkTriggered: 1,
			wantQuotaCorrIncr: 1, wantBreakerEntries: 0,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			svc, sr, _ := newTestService(5 * time.Second)
			sr.incrementCorrectionsErr = c.incrCorrErr
			sr.incrementPromotionsErr = c.incrPromoErr
			sr.markTriggeredErr = c.markTriggeredErr
			beforeCorr := svc.quotaTodayCorrections.Load()
			beforePromo := svc.quotaTodayPromotions.Load()

			err := svc.RecordCorrectionAction(context.Background(), c.action, c.success, c.isPromotion)

			if c.wantErr != nil {
				if err == nil || err.Error() != c.wantErr.Error() {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if sr.incrementCorrectionsCalls != c.wantCorrDelta {
				t.Errorf("incrementCorrectionsCalls = %d, want %d", sr.incrementCorrectionsCalls, c.wantCorrDelta)
			}
			if sr.incrementPromotionsCalls != c.wantPromoDelta {
				t.Errorf("incrementPromotionsCalls = %d, want %d", sr.incrementPromotionsCalls, c.wantPromoDelta)
			}
			if sr.markTriggeredCalls != c.wantMarkTriggered {
				t.Errorf("markTriggeredCalls = %d, want %d", sr.markTriggeredCalls, c.wantMarkTriggered)
			}
			gotCorrIncr := svc.quotaTodayCorrections.Load() - beforeCorr
			if gotCorrIncr != c.wantQuotaCorrIncr {
				t.Errorf("quotaCorrIncr = %d, want %d", gotCorrIncr, c.wantQuotaCorrIncr)
			}
			gotPromoIncr := svc.quotaTodayPromotions.Load() - beforePromo
			if gotPromoIncr != c.wantQuotaPromoIncr {
				t.Errorf("quotaPromoIncr = %d, want %d", gotPromoIncr, c.wantQuotaPromoIncr)
			}
			svc.breakerMu.Lock()
			gotEntries := len(svc.breakerWindow)
			svc.breakerMu.Unlock()
			if gotEntries != c.wantBreakerEntries {
				t.Errorf("breakerEntries = %d, want %d", gotEntries, c.wantBreakerEntries)
			}
			// 验证 breaker 条目的 success 字段
			if c.wantBreakerEntries > 0 {
				svc.breakerMu.Lock()
				entry := svc.breakerWindow[0]
				svc.breakerMu.Unlock()
				if entry.success != c.success {
					t.Errorf("breaker entry success = %v, want %v", entry.success, c.success)
				}
			}
		})
	}
}

// ============================================================================
// TestSwitchService_EvaluateCircuit  熔断触发/半开恢复
// ============================================================================

func TestSwitchService_EvaluateCircuit(t *testing.T) {
	// addBreakerEntries 直接向滑动窗口写入 N 条（成功/失败）条目
	addBreakerEntries := func(svc *SwitchService, successCount, failCount int) {
		svc.breakerMu.Lock()
		defer svc.breakerMu.Unlock()
		now := time.Now()
		for i := 0; i < successCount; i++ {
			svc.breakerWindow = append(svc.breakerWindow, breakerEntry{ts: now, success: true})
		}
		for i := 0; i < failCount; i++ {
			svc.breakerWindow = append(svc.breakerWindow, breakerEntry{ts: now, success: false})
		}
		svc.breakerEntries = len(svc.breakerWindow)
	}

	type tc struct {
		name             string
		successCount     int
		failCount        int
		initialOpen      bool // 初始缓存中的 circuit_open
		threshold        float64
		setCircuitOpenErr error
		getOrCreateErr   error
		wantErr          error
		wantSetCalls     int   // 期望 SetCircuitOpen 调用次数
		wantSetOpen      *bool // 期望最终设置的 open 值（nil 表示不关心/未调用）
		wantCachedOpen   bool  // 调用后缓存中的 circuit_open
	}

	cases := []tc{
		{
			name: "empty_window_closed_stays_closed",
			successCount: 0, failCount: 0, initialOpen: false, threshold: 0.3,
			wantSetCalls: 0, wantCachedOpen: false,
		},
		{
			name: "empty_window_open_stays_open_no_recovery",
			successCount: 0, failCount: 0, initialOpen: true, threshold: 0.3,
			wantSetCalls: 0, wantCachedOpen: true, // total=0 不满足 total>0，不恢复
		},
		{
			name: "few_success_entries_open_recovers",
			successCount: 5, failCount: 0, initialOpen: true, threshold: 0.3,
			wantSetCalls: 1, wantSetOpen: boolPtr(false), wantCachedOpen: false,
		},
		{
			name: "10_failures_closed_triggers_open",
			successCount: 0, failCount: 10, initialOpen: false, threshold: 0.3,
			wantSetCalls: 1, wantSetOpen: boolPtr(true), wantCachedOpen: true,
		},
		{
			name: "10_success_closed_stays_closed",
			successCount: 10, failCount: 0, initialOpen: false, threshold: 0.3,
			wantSetCalls: 0, wantCachedOpen: false,
		},
		{
			name: "10_success_open_recovers",
			successCount: 10, failCount: 0, initialOpen: true, threshold: 0.3,
			wantSetCalls: 1, wantSetOpen: boolPtr(false), wantCachedOpen: false,
		},
		{
			name: "10_3fail_rate0.3_triggers_open",
			successCount: 7, failCount: 3, initialOpen: false, threshold: 0.3,
			wantSetCalls: 1, wantSetOpen: boolPtr(true), wantCachedOpen: true,
		},
		{
			name: "10_2fail_rate0.2_stays_closed",
			successCount: 8, failCount: 2, initialOpen: false, threshold: 0.3,
			wantSetCalls: 0, wantCachedOpen: false,
		},
		{
			name: "10_5fail_open_stays_open_no_recovery",
			successCount: 5, failCount: 5, initialOpen: true, threshold: 0.3,
			wantSetCalls: 0, wantCachedOpen: true, // open=true 但 snap.CircuitOpen=true，不触发 SetCircuitOpen
		},
		{
			name: "threshold0.5_4fail_no_open",
			successCount: 6, failCount: 4, initialOpen: false, threshold: 0.5,
			wantSetCalls: 0, wantCachedOpen: false,
		},
		{
			name: "threshold0.5_5fail_triggers_open",
			successCount: 5, failCount: 5, initialOpen: false, threshold: 0.5,
			wantSetCalls: 1, wantSetOpen: boolPtr(true), wantCachedOpen: true,
		},
		{
			name: "set_circuit_open_error_on_open",
			successCount: 0, failCount: 10, initialOpen: false, threshold: 0.3,
			setCircuitOpenErr: errors.New("set open failed"),
			wantErr: errors.New("set open failed"), wantSetCalls: 1, wantCachedOpen: false,
		},
		{
			name: "set_circuit_open_error_on_recover",
			successCount: 5, failCount: 0, initialOpen: true, threshold: 0.3,
			setCircuitOpenErr: errors.New("set recover failed"),
			wantErr: errors.New("set recover failed"), wantSetCalls: 1, wantCachedOpen: true,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			svc, sr, _ := newTestService(5 * time.Second)
			// 预填缓存，避免 GetStatus 走 DB
			snap := &SwitchSnapshot{
				AutonomyLevel:           model.AutonomyLevelAutonomous,
				CircuitOpen:             c.initialOpen,
				CircuitBreakerThreshold: c.threshold,
				CircuitBreakerWindowMin: 30,
				MaxDailyCorrections:     100,
			}
			setCache(svc, snap, 0, 0)
			// 同步 mock 状态，保证 GetStatus 一致
			sr.sw.CircuitOpen = c.initialOpen
			sr.sw.CircuitBreakerThreshold = c.threshold
			sr.setCircuitOpenErr = c.setCircuitOpenErr
			sr.getOrCreateErr = c.getOrCreateErr

			addBreakerEntries(svc, c.successCount, c.failCount)

			err := svc.EvaluateCircuit(context.Background())

			if c.wantErr != nil {
				if err == nil || err.Error() != c.wantErr.Error() {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if sr.setCircuitOpenCalls != c.wantSetCalls {
				t.Errorf("setCircuitOpenCalls = %d, want %d", sr.setCircuitOpenCalls, c.wantSetCalls)
			}
			if c.wantSetOpen != nil && len(sr.setCircuitOpenValues) > 0 {
				if sr.setCircuitOpenValues[len(sr.setCircuitOpenValues)-1] != *c.wantSetOpen {
					t.Errorf("last SetCircuitOpen value = %v, want %v",
						sr.setCircuitOpenValues[len(sr.setCircuitOpenValues)-1], *c.wantSetOpen)
				}
			}
			svc.cacheMu.RLock()
			gotCachedOpen := svc.cached.CircuitOpen
			svc.cacheMu.RUnlock()
			if gotCachedOpen != c.wantCachedOpen {
				t.Errorf("cached.CircuitOpen = %v, want %v", gotCachedOpen, c.wantCachedOpen)
			}
		})
	}

	// GetStatus 错误传播（缓存为空，走 DB 失败）
	t.Run("get_status_error", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("db down")
		err := svc.EvaluateCircuit(context.Background())
		if err == nil || err.Error() != "db down" {
			t.Fatalf("err = %v, want db down", err)
		}
	})
}

// ============================================================================
// TestSwitchService_ResetDailyCounters
// ============================================================================

func TestSwitchService_ResetDailyCounters(t *testing.T) {
	t.Run("success_resets_all_counters_and_breaker", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		// 预设计数器非零
		svc.quotaTodayCorrections.Store(50)
		svc.quotaTodayPromotions.Store(3)
		svc.quotaLastReset.Store(1000)
		// 预设 breaker 窗口非空
		svc.breakerMu.Lock()
		svc.breakerWindow = []breakerEntry{{ts: time.Now(), success: false}}
		svc.breakerEntries = 1
		svc.breakerMu.Unlock()

		err := svc.ResetDailyCounters(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sr.resetDailyCalls != 1 {
			t.Errorf("resetDailyCalls = %d, want 1", sr.resetDailyCalls)
		}
		if svc.quotaTodayCorrections.Load() != 0 {
			t.Errorf("quotaTodayCorrections = %d, want 0", svc.quotaTodayCorrections.Load())
		}
		if svc.quotaTodayPromotions.Load() != 0 {
			t.Errorf("quotaTodayPromotions = %d, want 0", svc.quotaTodayPromotions.Load())
		}
		if svc.quotaLastReset.Load() == 1000 {
			t.Errorf("quotaLastReset not updated")
		}
		svc.breakerMu.Lock()
		windowLen := len(svc.breakerWindow)
		entries := svc.breakerEntries
		svc.breakerMu.Unlock()
		if windowLen != 0 {
			t.Errorf("breakerWindow len = %d, want 0", windowLen)
		}
		if entries != 0 {
			t.Errorf("breakerEntries = %d, want 0", entries)
		}
	})

	t.Run("repo_error_propagates", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		sr.resetDailyErr = errors.New("reset db error")
		svc.quotaTodayCorrections.Store(50)

		err := svc.ResetDailyCounters(context.Background())
		if err == nil || err.Error() != "reset db error" {
			t.Fatalf("err = %v, want reset db error", err)
		}
		// 失败时计数器不应被重置
		if svc.quotaTodayCorrections.Load() != 50 {
			t.Errorf("quotaTodayCorrections = %d, want 50 (not reset on error)", svc.quotaTodayCorrections.Load())
		}
	})
}

// ============================================================================
// TestSwitchService_ShouldExecuteAction
// ============================================================================

func TestSwitchService_ShouldExecuteAction(t *testing.T) {
	type tc struct {
		name       string
		action     model.CorrectionActionType
		autonomy   model.AutonomyLevel
		enableRAG  bool
		enableAst  bool
		enableLLM  bool
		open       bool
		quotaCorr  int
		maxCorr    int
		wantAllow  bool
		wantReason string // wantAllow=false 时的预期原因（部分匹配即可，此处精确）
	}

	cases := []tc{
		{
			name: "autonomous_all_enabled_pass", action: model.CorrectionRetrieveRetry,
			autonomy: model.AutonomyLevelAutonomous, enableRAG: true, maxCorr: 100, quotaCorr: 0,
			wantAllow: true,
		},
		{
			name: "supervised_rag_disabled_blocked", action: model.CorrectionQueryRewrite,
			autonomy: model.AutonomyLevelSupervised, enableRAG: false, maxCorr: 100, quotaCorr: 0,
			wantAllow: false, wantReason: "rag self-learning disabled",
		},
		{
			name: "autonomous_asset_promote_quota_exceeded", action: model.CorrectionAssetPromote,
			autonomy: model.AutonomyLevelAutonomous, enableAst: true, maxCorr: 100, quotaCorr: 100,
			wantAllow: false, wantReason: "daily correction quota exceeded",
		},
		{
			name: "manual_mode_blocked", action: model.CorrectionLLMCorrection,
			autonomy: model.AutonomyLevelManual, enableLLM: true, maxCorr: 100, quotaCorr: 0,
			wantAllow: false, wantReason: "manual mode forbids auto action",
		},
		{
			name: "circuit_open_blocked", action: model.CorrectionChunkArchive,
			autonomy: model.AutonomyLevelAutonomous, enableRAG: true, open: true, maxCorr: 100, quotaCorr: 0,
			wantAllow: false, wantReason: "circuit breaker open",
		},
		{
			name: "llm_disabled_blocked", action: model.CorrectionLLMCorrection,
			autonomy: model.AutonomyLevelAutonomous, enableLLM: false, maxCorr: 100, quotaCorr: 0,
			wantAllow: false, wantReason: "llm self-correction disabled",
		},
		{
			name: "asset_disabled_blocked", action: model.CorrectionAssetRollback,
			autonomy: model.AutonomyLevelAutonomous, enableAst: false, maxCorr: 100, quotaCorr: 0,
			wantAllow: false, wantReason: "asset self-learning disabled",
		},
		{
			name: "multiple_reasons_combined", action: model.CorrectionAssetPromote,
			autonomy: model.AutonomyLevelManual, enableAst: false, open: true, maxCorr: 100, quotaCorr: 100,
			wantAllow: false,
			wantReason: "asset self-learning disabled; circuit breaker open; daily correction quota exceeded; manual mode forbids auto action",
		},
		{
			name: "champion_upsert_pass", action: model.CorrectionChampionUpsert,
			autonomy: model.AutonomyLevelSupervised, enableRAG: true, maxCorr: 100, quotaCorr: 50,
			wantAllow: true,
		},
		{
			name: "quota_zero_means_no_limit_pass", action: model.CorrectionRetrieveRetry,
			autonomy: model.AutonomyLevelAutonomous, enableRAG: true, maxCorr: 0, quotaCorr: 9999,
			wantAllow: true,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			svc, _, _ := newTestService(5 * time.Second)
			snap := &SwitchSnapshot{
				AutonomyLevel:       c.autonomy,
				EnableRAG:           c.enableRAG,
				EnableAsset:         c.enableAst,
				EnableLLM:           c.enableLLM,
				CircuitOpen:         c.open,
				MaxDailyCorrections: c.maxCorr,
				MaxDailyPromotions:  5,
			}
			setCache(svc, snap, int64(c.quotaCorr), 0)

			allow, reason, err := svc.ShouldExecuteAction(context.Background(), c.action)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if allow != c.wantAllow {
				t.Errorf("allow = %v, want %v (reason=%q)", allow, c.wantAllow, reason)
			}
			if !c.wantAllow {
				if c.wantReason != "" && reason != c.wantReason {
					t.Errorf("reason = %q, want %q", reason, c.wantReason)
				}
				if allow {
					t.Errorf("should be blocked but allow=true")
				}
			} else {
				if reason != "" {
					t.Errorf("reason should be empty when allowed, got %q", reason)
				}
			}
		})
	}

	t.Run("get_status_error_propagates", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("db error")
		_, _, err := svc.ShouldExecuteAction(context.Background(), model.CorrectionRetrieveRetry)
		if err == nil || err.Error() != "db error" {
			t.Fatalf("err = %v, want db error", err)
		}
	})
}

// ============================================================================
// TestSwitchService_IsAutonomous / IsSupervised / IsManual
// ============================================================================

func TestSwitchService_IsAutonomous(t *testing.T) {
	for _, a := range []model.AutonomyLevel{model.AutonomyLevelManual, model.AutonomyLevelSupervised, model.AutonomyLevelAutonomous} {
		a := a
		t.Run(string(a), func(t *testing.T) {
			svc, _, _ := newTestService(5 * time.Second)
			snap := &SwitchSnapshot{AutonomyLevel: a}
			setCache(svc, snap, 0, 0)
			got, err := svc.IsAutonomous(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := a == model.AutonomyLevelAutonomous
			if got != want {
				t.Errorf("IsAutonomous = %v, want %v", got, want)
			}
		})
	}
	t.Run("error_propagates", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("db error")
		got, err := svc.IsAutonomous(context.Background())
		if err == nil || err.Error() != "db error" {
			t.Fatalf("err = %v, want db error", err)
		}
		if got {
			t.Errorf("got = true, want false on error")
		}
	})
}

func TestSwitchService_IsSupervised(t *testing.T) {
	for _, a := range []model.AutonomyLevel{model.AutonomyLevelManual, model.AutonomyLevelSupervised, model.AutonomyLevelAutonomous} {
		a := a
		t.Run(string(a), func(t *testing.T) {
			svc, _, _ := newTestService(5 * time.Second)
			snap := &SwitchSnapshot{AutonomyLevel: a}
			setCache(svc, snap, 0, 0)
			got, err := svc.IsSupervised(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := a == model.AutonomyLevelSupervised
			if got != want {
				t.Errorf("IsSupervised = %v, want %v", got, want)
			}
		})
	}
	t.Run("error_propagates", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("db error")
		_, err := svc.IsSupervised(context.Background())
		if err == nil || err.Error() != "db error" {
			t.Fatalf("err = %v, want db error", err)
		}
	})
}

func TestSwitchService_IsManual(t *testing.T) {
	for _, a := range []model.AutonomyLevel{model.AutonomyLevelManual, model.AutonomyLevelSupervised, model.AutonomyLevelAutonomous} {
		a := a
		t.Run(string(a), func(t *testing.T) {
			svc, _, _ := newTestService(5 * time.Second)
			snap := &SwitchSnapshot{AutonomyLevel: a}
			setCache(svc, snap, 0, 0)
			got, err := svc.IsManual(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := a == model.AutonomyLevelManual
			if got != want {
				t.Errorf("IsManual = %v, want %v", got, want)
			}
		})
	}
	t.Run("error_propagates", func(t *testing.T) {
		svc, sr, _ := newTestService(5 * time.Second)
		svc.cacheMu.Lock()
		svc.cached = nil
		svc.cacheMu.Unlock()
		sr.getOrCreateErr = errors.New("db error")
		_, err := svc.IsManual(context.Background())
		if err == nil || err.Error() != "db error" {
			t.Fatalf("err = %v, want db error", err)
		}
	})
}

// ============================================================================
// TestSwitchService_CleanStaleLogs  孤儿数据治理（failed + running）
// ============================================================================

func TestSwitchService_CleanStaleLogs(t *testing.T) {
	t.Run("delegates_to_repo_with_cutoff", func(t *testing.T) {
		svc, _, lr := newTestService(5 * time.Second)
		lr.markStaleResult = 42
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		got, err := svc.CleanStaleLogs(context.Background(), cutoff)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 42 {
			t.Errorf("CleanStaleLogs = %d, want 42", got)
		}
		if lr.markStaleCalls != 1 {
			t.Errorf("markStaleCalls = %d, want 1", lr.markStaleCalls)
		}
	})

	t.Run("propagates_repo_error", func(t *testing.T) {
		svc, _, lr := newTestService(5 * time.Second)
		lr.markStaleErr = errors.New("db connection lost")
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		_, err := svc.CleanStaleLogs(context.Background(), cutoff)
		if err == nil || err.Error() != "db connection lost" {
			t.Fatalf("err = %v, want 'db connection lost'", err)
		}
		if lr.markStaleCalls != 1 {
			t.Errorf("markStaleCalls = %d, want 1", lr.markStaleCalls)
		}
	})

	t.Run("zero_rows_no_error", func(t *testing.T) {
		svc, _, lr := newTestService(5 * time.Second)
		lr.markStaleResult = 0
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		got, err := svc.CleanStaleLogs(context.Background(), cutoff)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 0 {
			t.Errorf("CleanStaleLogs = %d, want 0 (no stale logs)", got)
		}
	})
}

// boolPtr 辅助函数
func boolPtr(b bool) *bool { return &b }
