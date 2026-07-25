package repository

// self_learning_switch_repo.go 自我学习三位一体统一开关 + 监督信号 + 矫正动作仓储
//
// 五层架构归属: L3 Repository 层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §7.2 §7.3 §7.4
//
// 本文件包含 3 个仓储接口：
//   1. SelfLearningSwitchRepository    - 统一开关 CRUD（单例 id=1）
//   2. SelfSupervisionSignalRepository - 5 维监督信号聚合（按小时分桶）
//   3. SelfCorrectionActionRepository  - 7 类矫正动作审计
//
// 设计要点：
//   - Switch 单例：通过 id=1 强制约束，GetOrCreate 懒初始化
//   - SupervisionSignal：使用 UPSERT 语义（同一 bucket 仅一条）
//   - CorrectionAction：状态机转换（pending→applied→rolled_back）

import (
	"context"
	"errors"
	"fmt"
	"time"

	"marketing/internal/model"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// ============================================================================
// 1. SelfLearningSwitchRepository 三位一体统一开关仓储
// ============================================================================

// SelfLearningSwitchRepository 自我学习统一开关仓储接口
type SelfLearningSwitchRepository interface {
	// Get 获取开关（单例，固定 id=1；不存在时返回 error）
	Get(ctx context.Context) (*model.SelfLearningSwitch, error)
	// GetOrCreate 获取或创建开关（首次部署时初始化为默认值）
	GetOrCreate(ctx context.Context) (*model.SelfLearningSwitch, error)
	// Update 更新开关配置
	Update(ctx context.Context, m *model.SelfLearningSwitch) error
	// IncrementTodayCorrections 累加今日矫正数（原子操作）
	IncrementTodayCorrections(ctx context.Context, delta int) error
	// IncrementTodayPromotions 累加今日晋升数（原子操作）
	IncrementTodayPromotions(ctx context.Context, delta int) error
	// ResetDailyCounters 每日 0 点重置计数器（由 cron 触发）
	ResetDailyCounters(ctx context.Context) error
	// MarkTriggered 标记最后触发时间
	MarkTriggered(ctx context.Context) error
	// SetCircuitOpen 设置熔断状态
	SetCircuitOpen(ctx context.Context, open bool) error
}

type selfLearningSwitchRepo struct {
	db *gorm.DB
}

// NewSelfLearningSwitchRepository 创建开关仓储
func NewSelfLearningSwitchRepository(db *gorm.DB) SelfLearningSwitchRepository {
	return &selfLearningSwitchRepo{db: db}
}

// Get 获取开关（单例）
func (r *selfLearningSwitchRepo) Get(ctx context.Context) (*model.SelfLearningSwitch, error) {
	var s model.SelfLearningSwitch
	err := r.db.WithContext(ctx).Where("id = ?", 1).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetOrCreate 获取或创建开关（首次部署懒初始化）
//
// 错误判定：使用 errors.Is(err, gorm.ErrRecordNotFound) 而非 err != gorm.ErrRecordNotFound，
// 因为 GORM 可能对错误进行包装（如事务回滚后的 wrapped error），直接 != 比较会漏判，
// 导致非 RecordNotFound 错误（如连接断开）被误当作"首次初始化"处理。
func (r *selfLearningSwitchRepo) GetOrCreate(ctx context.Context) (*model.SelfLearningSwitch, error) {
	s, err := r.Get(ctx)
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// 首次初始化：插入默认值
	newSwitch := &model.SelfLearningSwitch{
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
	if err := r.db.WithContext(ctx).Create(newSwitch).Error; err != nil {
		// 并发场景下可能已被其他协程创建，重试一次
		return r.Get(ctx)
	}
	return newSwitch, nil
}

// Update 更新开关配置
//
// 仅更新配置字段，不触碰运行时字段（today_corrections, today_promotions,
// today_reset_at, last_triggered_at, circuit_open），避免 Read-Modify-Write
// 期间的 lost-update：
//
// 场景：UpdateSwitch 读 sw（today_corrections=5）→ 期间另一协程
// IncrementTodayCorrections 原子加 1（DB 中=6）→ Update 用 Save(sw)
// 覆盖全部列，today_corrections 被回写为 5，IncrementTodayCorrections 丢失。
//
// 修复：改用 Updates(map) 只写配置字段，运行时字段由各自的原子方法维护。
// Save() 会写入所有列（包括零值），Updates(map) 只写指定列。
func (r *selfLearningSwitchRepo) Update(ctx context.Context, m *model.SelfLearningSwitch) error {
	if m == nil {
		return fmt.Errorf("switch is nil")
	}
	return r.db.WithContext(ctx).Model(&model.SelfLearningSwitch{}).
		Where("id = ?", 1).
		Updates(map[string]any{
			"autonomy_level":            m.AutonomyLevel,
			"enable_rag":                m.EnableRAG,
			"enable_asset":              m.EnableAsset,
			"enable_llm":                m.EnableLLM,
			"max_daily_corrections":     m.MaxDailyCorrections,
			"max_daily_promotions":      m.MaxDailyPromotions,
			"low_quality_threshold":     m.LowQualityThreshold,
			"champion_reward_threshold": m.ChampionRewardThreshold,
			"ab_test_min_samples":       m.ABTestMinSamples,
			"circuit_breaker_threshold": m.CircuitBreakerThreshold,
			"circuit_breaker_window_min": m.CircuitBreakerWindowMin,
			"updated_by":                m.UpdatedBy,
		}).Error
}

// IncrementTodayCorrections 原子累加今日矫正数
func (r *selfLearningSwitchRepo) IncrementTodayCorrections(ctx context.Context, delta int) error {
	return r.db.WithContext(ctx).Model(&model.SelfLearningSwitch{}).
		Where("id = ?", 1).
		UpdateColumn("today_corrections", gorm.Expr("today_corrections + ?", delta)).Error
}

// IncrementTodayPromotions 原子累加今日晋升数
func (r *selfLearningSwitchRepo) IncrementTodayPromotions(ctx context.Context, delta int) error {
	return r.db.WithContext(ctx).Model(&model.SelfLearningSwitch{}).
		Where("id = ?", 1).
		UpdateColumn("today_promotions", gorm.Expr("today_promotions + ?", delta)).Error
}

// ResetDailyCounters 每日 0 点重置计数器
func (r *selfLearningSwitchRepo) ResetDailyCounters(ctx context.Context) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.SelfLearningSwitch{}).
		Where("id = ?", 1).
		Updates(map[string]any{
			"today_corrections": 0,
			"today_promotions":  0,
			"today_reset_at":    now,
		}).Error
}

// MarkTriggered 标记最后触发时间
func (r *selfLearningSwitchRepo) MarkTriggered(ctx context.Context) error {
	return r.db.WithContext(ctx).Model(&model.SelfLearningSwitch{}).
		Where("id = ?", 1).
		UpdateColumn("last_triggered_at", time.Now()).Error
}

// SetCircuitOpen 设置熔断状态
func (r *selfLearningSwitchRepo) SetCircuitOpen(ctx context.Context, open bool) error {
	return r.db.WithContext(ctx).Model(&model.SelfLearningSwitch{}).
		Where("id = ?", 1).
		UpdateColumn("circuit_open", open).Error
}

// ============================================================================
// 2. SelfSupervisionSignalRepository 5 维监督信号仓储
// ============================================================================

// SelfSupervisionSignalRepository 监督信号仓储接口
type SelfSupervisionSignalRepository interface {
	// UpsertSignal 上报或更新监督信号（按 target+metric+bucket UPSERT）
	UpsertSignal(ctx context.Context, m *model.SelfSupervisionSignal) error
	// GetByID 按 signal_id 查询
	GetByID(ctx context.Context, signalID string) (*model.SelfSupervisionSignal, error)
	// ListByMetric 按 metric + 时间范围查询
	ListByMetric(ctx context.Context, targetType model.SupervisionTargetType, metricName string, from, to time.Time, limit int) ([]*model.SelfSupervisionSignal, error)
	// ListAlerts 查询当前告警中的信号（status != normal）
	ListAlerts(ctx context.Context, since time.Time, limit int) ([]*model.SelfSupervisionSignal, error)
	// ListByTarget 按目标 ID 查询（特定 asset_id 的效能历史）
	ListByTarget(ctx context.Context, targetType model.SupervisionTargetType, targetID string, from, to time.Time, limit int) ([]*model.SelfSupervisionSignal, error)
	// AggregateByRange 按时间范围聚合指标（用于看板）
	AggregateByRange(ctx context.Context, targetType model.SupervisionTargetType, metricName string, from, to time.Time) (avg float64, count int64, err error)
}

type selfSupervisionSignalRepo struct {
	db *gorm.DB
}

// NewSelfSupervisionSignalRepository 创建监督信号仓储
func NewSelfSupervisionSignalRepository(db *gorm.DB) SelfSupervisionSignalRepository {
	return &selfSupervisionSignalRepo{db: db}
}

// UpsertSignal 上报或更新监督信号
//
// UPSERT 语义：相同 (target_type, target_id, metric_name, bucket_hour) 仅一条
// 复用 unique index uk_self_supervision_signals_bucket
//
// 注意：调用方必须先通过 service.GenSignalID 生成 signal_id 并写入 m.SignalID
func (r *selfSupervisionSignalRepo) UpsertSignal(ctx context.Context, m *model.SelfSupervisionSignal) error {
	if m == nil {
		return fmt.Errorf("signal is nil")
	}
	if m.SignalID == "" {
		return fmt.Errorf("signal_id is empty (caller must generate it via service.GenSignalID before UpsertSignal)")
	}
	// 状态判定：value < threshold*0.8 normal, value < threshold warning, value >= threshold alert
	r.classifyStatus(m)
	// 使用 ON CONFLICT UPSERT
	sql := `
		INSERT INTO self_supervision_signals
			(signal_id, target_type, target_id, metric_name, bucket_hour,
			 value, baseline, threshold, sample_count, status, trace_ids, detail,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
		ON CONFLICT (target_type, target_id, metric_name, bucket_hour)
		DO UPDATE SET
			value        = EXCLUDED.value,
			baseline     = EXCLUDED.baseline,
			threshold    = EXCLUDED.threshold,
			sample_count = self_supervision_signals.sample_count + EXCLUDED.sample_count,
			status       = EXCLUDED.status,
			trace_ids    = self_supervision_signals.trace_ids || EXCLUDED.trace_ids,
			detail       = EXCLUDED.detail,
			updated_at   = NOW()
	`
	traceArr := pq.StringArray(m.TraceIDs)
	return r.db.WithContext(ctx).Exec(sql,
		m.SignalID, string(m.TargetType), m.TargetID, m.MetricName, m.BucketHour,
		m.Value, m.Baseline, m.Threshold, m.SampleCount, string(m.Status), traceArr, m.Detail,
	).Error
}

// classifyStatus 根据 value/threshold 自动分类告警状态
//
// 规则：
//   - threshold=0 或 sample_count=0 → normal（不告警）
//   - value < threshold*0.8 → normal
//   - value < threshold → warning
//   - value >= threshold → alert
//
// 对于"低越好"指标（如 recall_precision 越低越异常），调用方需自行反向赋值
func (r *selfSupervisionSignalRepo) classifyStatus(m *model.SelfSupervisionSignal) {
	if m.Threshold <= 0 || m.SampleCount == 0 {
		m.Status = model.SupervisionStatusNormal
		return
	}
	if m.Value >= m.Threshold {
		m.Status = model.SupervisionStatusAlert
	} else if m.Value >= m.Threshold*0.8 {
		m.Status = model.SupervisionStatusWarning
	} else {
		m.Status = model.SupervisionStatusNormal
	}
}

// GetByID 按 signal_id 查询
func (r *selfSupervisionSignalRepo) GetByID(ctx context.Context, signalID string) (*model.SelfSupervisionSignal, error) {
	var s model.SelfSupervisionSignal
	err := r.db.WithContext(ctx).Where("signal_id = ?", signalID).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListByMetric 按 metric + 时间范围查询
func (r *selfSupervisionSignalRepo) ListByMetric(ctx context.Context, targetType model.SupervisionTargetType, metricName string, from, to time.Time, limit int) ([]*model.SelfSupervisionSignal, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	var list []*model.SelfSupervisionSignal
	q := r.db.WithContext(ctx).
		Where("target_type = ? AND metric_name = ?", targetType, metricName)
	if !from.IsZero() {
		q = q.Where("bucket_hour >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("bucket_hour < ?", to)
	}
	err := q.Order("bucket_hour ASC").Limit(limit).Find(&list).Error
	return list, err
}

// ListAlerts 查询当前告警中的信号
func (r *selfSupervisionSignalRepo) ListAlerts(ctx context.Context, since time.Time, limit int) ([]*model.SelfSupervisionSignal, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var list []*model.SelfSupervisionSignal
	q := r.db.WithContext(ctx).
		Where("status != ?", model.SupervisionStatusNormal)
	if !since.IsZero() {
		q = q.Where("bucket_hour >= ?", since)
	}
	err := q.Order("bucket_hour DESC").Limit(limit).Find(&list).Error
	return list, err
}

// ListByTarget 按目标 ID 查询
func (r *selfSupervisionSignalRepo) ListByTarget(ctx context.Context, targetType model.SupervisionTargetType, targetID string, from, to time.Time, limit int) ([]*model.SelfSupervisionSignal, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	var list []*model.SelfSupervisionSignal
	q := r.db.WithContext(ctx).
		Where("target_type = ? AND target_id = ?", targetType, targetID)
	if !from.IsZero() {
		q = q.Where("bucket_hour >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("bucket_hour < ?", to)
	}
	err := q.Order("bucket_hour ASC").Limit(limit).Find(&list).Error
	return list, err
}

// AggregateByRange 按时间范围聚合指标
func (r *selfSupervisionSignalRepo) AggregateByRange(ctx context.Context, targetType model.SupervisionTargetType, metricName string, from, to time.Time) (avg float64, count int64, err error) {
	type result struct {
		Avg   float64
		Count int64
	}
	var res result
	q := r.db.WithContext(ctx).Model(&model.SelfSupervisionSignal{}).
		Where("target_type = ? AND metric_name = ?", targetType, metricName)
	if !from.IsZero() {
		q = q.Where("bucket_hour >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("bucket_hour < ?", to)
	}
	err = q.Select("COALESCE(AVG(value), 0) as avg, COALESCE(SUM(sample_count), 0) as count").Scan(&res).Error
	return res.Avg, res.Count, err
}

// ============================================================================
// 3. SelfCorrectionActionRepository 7 类矫正动作仓储
// ============================================================================

// SelfCorrectionActionRepository 矫正动作仓储接口
type SelfCorrectionActionRepository interface {
	// Create 创建矫正动作
	Create(ctx context.Context, m *model.SelfCorrectionAction) error
	// GetByID 按 action_id 查询
	GetByID(ctx context.Context, actionID string) (*model.SelfCorrectionAction, error)
	// UpdateStatus 更新状态
	UpdateStatus(ctx context.Context, actionID string, status model.CorrectionActionStatus, extraUpdates map[string]any) error
	// ListPending 列出待执行动作（supervised 模式下待人工确认）
	ListPending(ctx context.Context, limit int) ([]*model.SelfCorrectionAction, error)
	// ListByTarget 按目标查询历史矫正动作
	ListByTarget(ctx context.Context, targetType, targetID string, limit int) ([]*model.SelfCorrectionAction, error)
	// ListByTriggerLog 按触发日志查询
	ListByTriggerLog(ctx context.Context, triggerLogID string, limit int) ([]*model.SelfCorrectionAction, error)
	// ListByFilter 综合过滤查询（用于审计页面）
	ListByFilter(ctx context.Context, filter CorrectionActionFilter) ([]*model.SelfCorrectionAction, int64, error)
	// CountToday 今日矫正动作统计（按 action_type 分组）
	CountToday(ctx context.Context) (map[model.CorrectionActionType]int64, error)
}

// CorrectionActionFilter 矫正动作过滤条件
type CorrectionActionFilter struct {
	ActionType  model.CorrectionActionType
	TargetType  string
	TargetID    string
	Status      model.CorrectionActionStatus
	TriggerLogID string
	Since       time.Time
	Until       time.Time
	Page        int
	Size        int
}

type selfCorrectionActionRepo struct {
	db *gorm.DB
}

// NewSelfCorrectionActionRepository 创建矫正动作仓储
func NewSelfCorrectionActionRepository(db *gorm.DB) SelfCorrectionActionRepository {
	return &selfCorrectionActionRepo{db: db}
}

// GenActionID 已迁移至 service 层 (service/self_learning/id_generator.go)
//
// 历史问题：原实现包含 time.Now().UnixNano()，导致每次调用生成不同 ID，
// 破坏了幂等性——同一触发重复调用会创建多条相同语义的 action。
// 现已移除时间戳，确保幂等。如需为同一触发创建多个同类型 action，
// 调用方应传入不同的 targetID 区分。

// ErrDuplicateAction 矫正动作重复（action_id UNIQUE 冲突）
//
// 调用方应通过 errors.Is(err, ErrDuplicateAction) 判断是否为幂等冲突。
var ErrDuplicateAction = errors.New("self_correction_action: duplicate action_id")

// Create 创建矫正动作
//
// 幂等：action_id UNIQUE 冲突时返回 ErrDuplicateAction
// 调用方应通过 errors.Is 判断幂等冲突，其他错误原样上抛。
func (r *selfCorrectionActionRepo) Create(ctx context.Context, m *model.SelfCorrectionAction) error {
	if m == nil {
		return fmt.Errorf("action is nil")
	}
	if m.ActionID == "" {
		return fmt.Errorf("action_id is empty")
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if isDuplicateKeyErr(err) {
			return ErrDuplicateAction
		}
		return err
	}
	return nil
}

// GetByID 按 action_id 查询
func (r *selfCorrectionActionRepo) GetByID(ctx context.Context, actionID string) (*model.SelfCorrectionAction, error) {
	var a model.SelfCorrectionAction
	err := r.db.WithContext(ctx).Where("action_id = ?", actionID).First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ErrActionAlreadyFinalized 矫正动作已终结（rolled_back/failed/skipped 为终态）
//
// UpdateStatus 采用状态机守卫，仅允许从非终态（pending/applied）转换。
// 已终结的动作再调用 UpdateStatus 会返回本错误。
// 调用方应通过 errors.Is 识别并安全忽略（幂等语义）。
var ErrActionAlreadyFinalized = errors.New("self_correction_action: already finalized (rolled_back/failed/skipped)")

// UpdateStatus 更新状态（带状态机守卫）
//
// 守卫：WHERE status IN ('pending', 'applied')，仅允许从非终态转换。
// 终态（rolled_back/failed/skipped）不可再变更，防止状态回归。
// 若动作已终结，返回 ErrActionAlreadyFinalized。
//
// 合法转换路径：
//   - pending → applied（执行成功）
//   - pending → failed（执行失败）
//   - pending → skipped（护栏拦截）
//   - applied → rolled_back（回滚）
func (r *selfCorrectionActionRepo) UpdateStatus(ctx context.Context, actionID string, status model.CorrectionActionStatus, extraUpdates map[string]any) error {
	updates := map[string]any{
		"status":     status,
		"updated_at": time.Now(),
	}
	switch status {
	case model.CorrectionStatusApplied:
		now := time.Now()
		updates["applied_at"] = &now
	case model.CorrectionStatusRolledBack:
		now := time.Now()
		updates["rolled_back_at"] = &now
	}
	for k, v := range extraUpdates {
		updates[k] = v
	}
	result := r.db.WithContext(ctx).Model(&model.SelfCorrectionAction{}).
		Where("action_id = ? AND status IN ?", actionID,
			[]model.CorrectionActionStatus{model.CorrectionStatusPending, model.CorrectionStatusApplied}).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrActionAlreadyFinalized
	}
	return nil
}

// ListPending 列出待执行动作
func (r *selfCorrectionActionRepo) ListPending(ctx context.Context, limit int) ([]*model.SelfCorrectionAction, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var list []*model.SelfCorrectionAction
	err := r.db.WithContext(ctx).
		Where("status = ?", model.CorrectionStatusPending).
		Order("created_at ASC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// ListByTarget 按目标查询历史矫正动作
func (r *selfCorrectionActionRepo) ListByTarget(ctx context.Context, targetType, targetID string, limit int) ([]*model.SelfCorrectionAction, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var list []*model.SelfCorrectionAction
	err := r.db.WithContext(ctx).
		Where("target_type = ? AND target_id = ?", targetType, targetID).
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// ListByTriggerLog 按触发日志查询
func (r *selfCorrectionActionRepo) ListByTriggerLog(ctx context.Context, triggerLogID string, limit int) ([]*model.SelfCorrectionAction, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var list []*model.SelfCorrectionAction
	err := r.db.WithContext(ctx).
		Where("trigger_log_id = ?", triggerLogID).
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// ListByFilter 综合过滤查询
func (r *selfCorrectionActionRepo) ListByFilter(ctx context.Context, filter CorrectionActionFilter) ([]*model.SelfCorrectionAction, int64, error) {
	if filter.Size <= 0 || filter.Size > 500 {
		filter.Size = 50
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	var list []*model.SelfCorrectionAction
	var total int64
	q := r.db.WithContext(ctx).Model(&model.SelfCorrectionAction{})
	if filter.ActionType != "" {
		q = q.Where("action_type = ?", filter.ActionType)
	}
	if filter.TargetType != "" {
		q = q.Where("target_type = ?", filter.TargetType)
	}
	if filter.TargetID != "" {
		q = q.Where("target_id = ?", filter.TargetID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.TriggerLogID != "" {
		q = q.Where("trigger_log_id = ?", filter.TriggerLogID)
	}
	if !filter.Since.IsZero() {
		q = q.Where("created_at >= ?", filter.Since)
	}
	if !filter.Until.IsZero() {
		q = q.Where("created_at < ?", filter.Until)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC").
		Offset((filter.Page - 1) * filter.Size).
		Limit(filter.Size).
		Find(&list).Error
	return list, total, err
}

// CountToday 今日矫正动作统计
//
// 时区说明：参见 self_learning_repo.go 的 startOfTodayShanghai 注释
func (r *selfCorrectionActionRepo) CountToday(ctx context.Context) (map[model.CorrectionActionType]int64, error) {
	type result struct {
		ActionType model.CorrectionActionType
		Count      int64
	}
	var results []result
	start := startOfTodayShanghai()
	err := r.db.WithContext(ctx).Model(&model.SelfCorrectionAction{}).
		Select("action_type, COUNT(*) as count").
		Where("created_at >= ?", start).
		Group("action_type").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	out := make(map[model.CorrectionActionType]int64, len(results))
	for _, r := range results {
		out[r.ActionType] = r.Count
	}
	return out, nil
}
