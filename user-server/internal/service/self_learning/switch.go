package selflearning

// switch_service.go 三位一体统一开关服务
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §7.4
//
// 职责：
//   1. 统一开关 CRUD（单例 id=1）
//   2. 三级自治等级（manual / supervised / autonomous）
//   3. 6 道安全护栏（v1.1 §7.4.5）：
//        - 每日矫正动作数上限
//        - 每日资产包晋升数上限
//        - 低质语料阈值
//        - 销冠补录阈值
//        - A/B 最小样本数
//        - 熔断阈值（失败率）
//   4. 熔断器（滑动窗口失败率统计）
//   5. 实时快照（供其他组件并发读取决策）
//
// 用户开启即全自动执行（v1.1 §7.4）：
//   - 用户调用 UpdateSwitch(autonomous, enable_rag=true, enable_asset=true)
//   - 后续所有触发事件（dialogue.started/ended）由 Orchestrator 自动派发
//   - 矫正动作根据 autonomy_level 自动执行或转入待审

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
)

const (
	// defaultCacheTTL 默认缓存有效期
	defaultCacheTTL = 5 * time.Second
	// breakerMinSamples 触发熔断所需的最小样本数
	breakerMinSamples = 10
	// breakerCleanupThreshold 触发懒清理的条目数阈值
	breakerCleanupThreshold = 200
	// defaultBreakerWindowMin 默认熔断器窗口（分钟）
	defaultBreakerWindowMin = 30
	// defaultBreakerThreshold 默认熔断失败率阈值
	defaultBreakerThreshold = 0.3
)

// SwitchService 三位一体统一开关服务
//
// 同时管控 RAG / 资产包 / LLM 三个子系统的自我学习开关
// 通过 SwitchSnapshot 暴露只读视图，供 RAGSelfCorrector/AssetBundleLearner
// /SelfCorrectionDispatcher 等组件读取决策。
type SwitchService struct {
	switchRepo repository.SelfLearningSwitchRepository
	logRepo    repository.SelfLearningLogRepository

	// 内存缓存（atomic 读，避免高频 DB 查询）
	// 写时通过 mutex 串行化
	cacheMu   sync.RWMutex
	cached    *SwitchSnapshot
	cachedAt  time.Time // 缓存填充时间，用于 TTL 判断（独立于 snap.UpdatedAt）
	cacheExp  time.Duration

	// 熔断器滑动窗口（最近 N 分钟的失败计数）
	breakerMu      sync.Mutex
	breakerWindow  []breakerEntry
	breakerEntries int // 累计条目数（防止内存无限增长）

	// 配额本地缓存（由 DB 原子操作保证准确性，本地缓存仅用于快速预检）
	quotaTodayCorrections atomic.Int64
	quotaTodayPromotions  atomic.Int64
	quotaLastReset        atomic.Int64 // unix timestamp of last reset
}

// breakerEntry 熔断器滑动窗口条目
type breakerEntry struct {
	ts      time.Time
	success bool
}

// NewSwitchService 创建开关服务
//
// cacheExp: 内存缓存有效期（建议 5s）；超过则从 DB 重新加载
func NewSwitchService(
	switchRepo repository.SelfLearningSwitchRepository,
	logRepo repository.SelfLearningLogRepository,
	cacheExp time.Duration,
) *SwitchService {
	if cacheExp <= 0 {
		cacheExp = defaultCacheTTL
	}
	return &SwitchService{
		switchRepo: switchRepo,
		logRepo:    logRepo,
		cacheExp:   cacheExp,
	}
}

// ============================================================================
// 开关 CRUD
// ============================================================================

// GetStatus 获取开关状态（带内存缓存）
func (s *SwitchService) GetStatus(ctx context.Context) (*SwitchSnapshot, error) {
	// 尝试读缓存
	s.cacheMu.RLock()
	if s.cached != nil && time.Since(s.cachedAt) < s.cacheExp {
		snap := *s.cached
		s.cacheMu.RUnlock()
		// 合并本地配额计数（避免 DB 读放大）
		snap.TodayCorrections = int(s.quotaTodayCorrections.Load())
		snap.TodayPromotions = int(s.quotaTodayPromotions.Load())
		return &snap, nil
	}
	s.cacheMu.RUnlock()

	// 缓存未命中或已过期，从 DB 重新加载
	sw, err := s.switchRepo.GetOrCreate(ctx)
	if err != nil {
		return nil, err
	}
	snap := s.toSnapshot(sw)

	s.cacheMu.Lock()
	s.cached = snap
	s.cachedAt = time.Now()
	// 同步本地配额
	s.quotaTodayCorrections.Store(int64(sw.TodayCorrections))
	s.quotaTodayPromotions.Store(int64(sw.TodayPromotions))
	if sw.TodayResetAt != nil {
		s.quotaLastReset.Store(sw.TodayResetAt.Unix())
	}
	s.cacheMu.Unlock()

	// 返回拷贝，避免调用方修改缓存
	out := *snap
	out.TodayCorrections = int(s.quotaTodayCorrections.Load())
	out.TodayPromotions = int(s.quotaTodayPromotions.Load())
	return &out, nil
}

// UpdateSwitch 更新开关配置
//
// 用户开启全自动：UpdateSwitch(autonomous, true, true, true)
// 关闭：UpdateSwitch(manual, false, false, false)
//
// 校验说明：原 dto.SwitchConfigRequest.Validate() 已下沉至本包
// ValidateSwitchConfig 函数（五层架构规范：DTO 层禁止含业务逻辑）。
// L4 层保留防御性校验，确保即使 L3 未校验也能拦截非法参数。
func (s *SwitchService) UpdateSwitch(ctx context.Context, req *dto.SwitchConfigRequest, operatorID uint) (*SwitchSnapshot, error) {
	if err := ValidateSwitchConfig(req); err != nil {
		return nil, err
	}
	sw, err := s.switchRepo.GetOrCreate(ctx)
	if err != nil {
		return nil, err
	}
	// 更新字段
	sw.AutonomyLevel = req.AutonomyLevel
	sw.EnableRAG = req.EnableRAG
	sw.EnableAsset = req.EnableAsset
	sw.EnableLLM = req.EnableLLM
	sw.MaxDailyCorrections = req.MaxDailyCorrections
	sw.MaxDailyPromotions = req.MaxDailyPromotions
	sw.LowQualityThreshold = req.LowQualityThreshold
	sw.ChampionRewardThreshold = req.ChampionRewardThreshold
	sw.ABTestMinSamples = req.ABTestMinSamples
	sw.CircuitBreakerThreshold = req.CircuitBreakerThreshold
	sw.CircuitBreakerWindowMin = req.CircuitBreakerWindowMin
	sw.UpdatedBy = operatorID

	if err := s.switchRepo.Update(ctx, sw); err != nil {
		return nil, err
	}

	// 刷新缓存
	snap := s.toSnapshot(sw)
	s.cacheMu.Lock()
	s.cached = snap
	s.cachedAt = time.Now()
	s.cacheMu.Unlock()
	return snap, nil
}

// ============================================================================
// 护栏检查（v1.1 §7.4.5）
// ============================================================================

// CheckGuardrail 护栏检查（其他组件执行矫正动作前必须调用）
//
// 检查项：
//   1. 开关是否启用对应子系统（enable_rag/enable_asset/enable_llm）
//   2. 熔断器是否开启
//   3. 今日配额是否耗尽
//   4. autonomy_level 是否允许自动执行
//
// 调用方根据返回的 GuardrailCheckResult.Passed 决定是否继续执行
func (s *SwitchService) CheckGuardrail(ctx context.Context, actionType model.CorrectionActionType) (*GuardrailCheckResult, error) {
	snap, err := s.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	result := &GuardrailCheckResult{
		AutonomyLevel:   snap.AutonomyLevel,
		CircuitOpen:     snap.CircuitOpen,
		DailyQuotaLimit: snap.MaxDailyCorrections,
		DailyQuotaUsed:  int(s.quotaTodayCorrections.Load()),
	}
	// 1. 开关启用检查（按 actionType 路由到对应子系统）
	switch actionType {
	case model.CorrectionRetrieveRetry, model.CorrectionQueryRewrite,
		model.CorrectionChunkArchive, model.CorrectionChampionUpsert:
		if !snap.EnableRAG {
			result.BlockedReasons = append(result.BlockedReasons, "rag self-learning disabled")
		}
	case model.CorrectionAssetPromote, model.CorrectionAssetRollback:
		if !snap.EnableAsset {
			result.BlockedReasons = append(result.BlockedReasons, "asset self-learning disabled")
		}
	case model.CorrectionLLMCorrection:
		if !snap.EnableLLM {
			result.BlockedReasons = append(result.BlockedReasons, "llm self-correction disabled")
		}
	}
	// 2. 熔断器检查
	if snap.CircuitOpen {
		result.BlockedReasons = append(result.BlockedReasons, "circuit breaker open")
	}
	// 3. 每日配额检查
	if snap.MaxDailyCorrections > 0 && result.DailyQuotaUsed >= snap.MaxDailyCorrections {
		result.BlockedReasons = append(result.BlockedReasons, "daily correction quota exceeded")
	}
	// 4. 资产包晋升单独配额
	if actionType == model.CorrectionAssetPromote {
		promotionsUsed := int(s.quotaTodayPromotions.Load())
		if snap.MaxDailyPromotions > 0 && promotionsUsed >= snap.MaxDailyPromotions {
			result.BlockedReasons = append(result.BlockedReasons, "daily promotion quota exceeded")
		}
	}
	// 5. autonomy_level 检查（manual 模式下所有自动矫正动作均被拦截）
	if snap.AutonomyLevel == model.AutonomyLevelManual {
		result.BlockedReasons = append(result.BlockedReasons, "manual mode forbids auto action")
	}

	result.Passed = len(result.BlockedReasons) == 0
	return result, nil
}

// RecordCorrectionAction 记录矫正动作执行结果（用于配额扣减 & 熔断器统计）
//
// actionType: 矫正动作类型
// success:    是否成功
// isPromotion: 是否为资产包晋升动作（同时扣减晋升配额）
func (s *SwitchService) RecordCorrectionAction(ctx context.Context, actionType model.CorrectionActionType, success bool, isPromotion bool) error {
	// 1. 累加今日配额（DB 原子操作）
	if err := s.switchRepo.IncrementTodayCorrections(ctx, 1); err != nil {
		return err
	}
	s.quotaTodayCorrections.Add(1)
	if isPromotion {
		if err := s.switchRepo.IncrementTodayPromotions(ctx, 1); err != nil {
			return err
		}
		s.quotaTodayPromotions.Add(1)
	}
	// 2. 标记触发时间
	if err := s.switchRepo.MarkTriggered(ctx); err != nil {
		return err
	}
	// 3. 写入熔断器滑动窗口
	s.recordBreakerEntry(success)
	return nil
}

// ============================================================================
// 熔断器（滑动窗口失败率）
// ============================================================================

// recordBreakerEntry 记录熔断器条目
func (s *SwitchService) recordBreakerEntry(success bool) {
	s.breakerMu.Lock()
	defer s.breakerMu.Unlock()
	now := time.Now()
	s.breakerWindow = append(s.breakerWindow, breakerEntry{ts: now, success: success})
	s.breakerEntries++
	// 清理过期条目（超过窗口大小的 2 倍）
	// 懒清理：每次写入时检查最近 breakerCleanupThreshold 条，避免 O(N) 扫描
	if s.breakerEntries >= breakerCleanupThreshold {
		s.cleanupBreakerLocked()
	}
}

// cleanupBreakerLocked 清理过期的熔断器条目（调用方持锁）
func (s *SwitchService) cleanupBreakerLocked() {
	// 从快照读取窗口大小
	s.cacheMu.RLock()
	windowMin := defaultBreakerWindowMin
	if s.cached != nil && s.cached.CircuitBreakerWindowMin > 0 {
		windowMin = s.cached.CircuitBreakerWindowMin
	}
	s.cacheMu.RUnlock()
	cutoff := time.Now().Add(-time.Duration(windowMin) * time.Minute)
	// 双指针原地清理
	idx := 0
	for _, e := range s.breakerWindow {
		if e.ts.After(cutoff) {
			s.breakerWindow[idx] = e
			idx++
		}
	}
	s.breakerWindow = s.breakerWindow[:idx]
	s.breakerEntries = len(s.breakerWindow)
}

// checkCircuit 检查是否应触发熔断
//
// 返回值：open true 表示应熔断
func (s *SwitchService) checkCircuit() (open bool, failureRate float64, total int) {
	s.breakerMu.Lock()
	defer s.breakerMu.Unlock()
	s.cleanupBreakerLocked()
	total = len(s.breakerWindow)
	if total < breakerMinSamples {
		// 样本数不足，不触发熔断
		return false, 0, total
	}
	failed := 0
	for _, e := range s.breakerWindow {
		if !e.success {
			failed++
		}
	}
	failureRate = float64(failed) / float64(total)
	// 读取阈值
	s.cacheMu.RLock()
	threshold := defaultBreakerThreshold
	if s.cached != nil && s.cached.CircuitBreakerThreshold > 0 {
		threshold = s.cached.CircuitBreakerThreshold
	}
	s.cacheMu.RUnlock()
	return failureRate >= threshold, failureRate, total
}

// EvaluateCircuit 评估熔断状态（由后台协程周期性调用）
//
// 若失败率超过阈值，则设置 circuit_open=true
// 持续 5 分钟无新失败后自动 half-open 恢复
func (s *SwitchService) EvaluateCircuit(ctx context.Context) error {
	open, rate, total := s.checkCircuit()
	snap, err := s.GetStatus(ctx)
	if err != nil {
		return err
	}
	// 状态变化时更新 DB
	if open && !snap.CircuitOpen {
		if err := s.switchRepo.SetCircuitOpen(ctx, true); err != nil {
			return err
		}
		s.cacheMu.Lock()
		if s.cached != nil {
			s.cached.CircuitOpen = true
		}
		s.cacheMu.Unlock()
	} else if !open && snap.CircuitOpen && total > 0 {
		// 半开恢复：仅当最近窗口无失败时才解除熔断
		if rate == 0 {
			if err := s.switchRepo.SetCircuitOpen(ctx, false); err != nil {
				return err
			}
			s.cacheMu.Lock()
			if s.cached != nil {
				s.cached.CircuitOpen = false
			}
			s.cacheMu.Unlock()
		}
	}
	return nil
}

// ============================================================================
// 每日配额重置（由 cron 0 点触发）
// ============================================================================

// ResetDailyCounters 重置每日计数器
func (s *SwitchService) ResetDailyCounters(ctx context.Context) error {
	if err := s.switchRepo.ResetDailyCounters(ctx); err != nil {
		return err
	}
	s.quotaTodayCorrections.Store(0)
	s.quotaTodayPromotions.Store(0)
	s.quotaLastReset.Store(time.Now().Unix())
	// 同时清理熔断器窗口
	s.breakerMu.Lock()
	s.breakerWindow = s.breakerWindow[:0]
	s.breakerEntries = 0
	s.breakerMu.Unlock()
	return nil
}

// CleanStaleLogs 清理超期 failed/running 日志（孤儿数据治理）
//
// 将 started_at 早于 cutoff 的 failed 和 running 日志批量标记为 skipped。
// 由 cron.daily 触发，cutoff 通常为 7 天前。
//
// 覆盖两类孤儿数据：
//   - failed  → 长期未重试的失败日志，占据看板"最近失败"列表
//   - running → 协程崩溃/进程 OOM 等导致日志卡在 running 状态
//
// 设计理由：
//   - 系统是事件驱动的，新会话会自然重试（不同 session_id 不冲突）
//   - 7 天窗口足够运维介入排查，超期则自动清理（保留记录，仅状态降级）
func (s *SwitchService) CleanStaleLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	return s.logRepo.MarkStaleLogsAsSkipped(ctx, cutoff)
}

// ============================================================================
// 工具方法
// ============================================================================

// toSnapshot 实体转快照
func (s *SwitchService) toSnapshot(sw *model.SelfLearningSwitch) *SwitchSnapshot {
	return &SwitchSnapshot{
		AutonomyLevel:           sw.AutonomyLevel,
		EnableRAG:               sw.EnableRAG,
		EnableAsset:             sw.EnableAsset,
		EnableLLM:               sw.EnableLLM,
		CircuitOpen:             sw.CircuitOpen,
		MaxDailyCorrections:     sw.MaxDailyCorrections,
		MaxDailyPromotions:      sw.MaxDailyPromotions,
		TodayCorrections:        sw.TodayCorrections,
		TodayPromotions:         sw.TodayPromotions,
		LowQualityThreshold:     sw.LowQualityThreshold,
		ChampionRewardThreshold: sw.ChampionRewardThreshold,
		ABTestMinSamples:        sw.ABTestMinSamples,
		CircuitBreakerThreshold: sw.CircuitBreakerThreshold,
		CircuitBreakerWindowMin: sw.CircuitBreakerWindowMin,
		LastTriggeredAt:         sw.LastTriggeredAt,
		UpdatedAt:               sw.UpdatedAt,
	}
}

// ============================================================================
// 业务判定方法（供其他组件便捷调用）
// ============================================================================

// ShouldExecuteAction 判定是否应执行指定动作（结合护栏 + 自治等级）
//
// 返回值：
//   - allow: 是否允许执行
//   - reason: 拒绝原因（allow=false 时）
//   - err: 系统错误
func (s *SwitchService) ShouldExecuteAction(ctx context.Context, actionType model.CorrectionActionType) (allow bool, reason string, err error) {
	g, err := s.CheckGuardrail(ctx, actionType)
	if err != nil {
		return false, "", err
	}
	if !g.Passed {
		return false, joinReasons(g.BlockedReasons), nil
	}
	return true, "", nil
}

// joinReasons 合并拒绝原因
func joinReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	out := ""
	for i, r := range reasons {
		if i > 0 {
			out += "; "
		}
		out += r
	}
	return out
}

// IsAutonomous 是否为全自动模式
func (s *SwitchService) IsAutonomous(ctx context.Context) (bool, error) {
	snap, err := s.GetStatus(ctx)
	if err != nil {
		return false, err
	}
	return snap.AutonomyLevel == model.AutonomyLevelAutonomous, nil
}

// IsSupervised 是否为半自动模式
func (s *SwitchService) IsSupervised(ctx context.Context) (bool, error) {
	snap, err := s.GetStatus(ctx)
	if err != nil {
		return false, err
	}
	return snap.AutonomyLevel == model.AutonomyLevelSupervised, nil
}

// IsManual 是否为手动模式
func (s *SwitchService) IsManual(ctx context.Context) (bool, error) {
	snap, err := s.GetStatus(ctx)
	if err != nil {
		return false, err
	}
	return snap.AutonomyLevel == model.AutonomyLevelManual, nil
}
