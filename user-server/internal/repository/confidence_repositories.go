package repository

// confidence_repositories.go 置信度驱动转人工 7 个 Repository
//
// 五层架构归属: L4 数据访问层
// 设计依据: docs/核心链路优化.md 第十五章 §15.3 表结构设计
//
// 7 个 Repository：
//  1. ConfidenceSignalRepository      - 5 维信号快照读写
//  2. ConfidenceCalibrationRepository - 校准参数历史 + active 切换
//  3. HandoffDecisionRepository       - 转人工决策记录
//  4. ThresholdPolicyRepository       - 动态阈值策略 CRUD
//  5. ABTestRepository                - A/B 测试配置 + 指标样本
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
)

// ============================================================================
// 1. ConfidenceSignalRepository
// ============================================================================

// ConfidenceSignalRepository 5 维信号快照读写
type ConfidenceSignalRepository struct{}

// NewConfidenceSignalRepository 构造
func NewConfidenceSignalRepository() *ConfidenceSignalRepository {
	return &ConfidenceSignalRepository{}
}

// Create 创建信号快照
func (r *ConfidenceSignalRepository) Create(ctx context.Context, s *model.ConfidenceSignal) error {
	return db.GetDB().WithContext(ctx).Create(s).Error
}

// GetBySignalID 按 SignalID 查询
func (r *ConfidenceSignalRepository) GetBySignalID(ctx context.Context, signalID string) (*model.ConfidenceSignal, error) {
	var s model.ConfidenceSignal
	err := db.GetDB().WithContext(ctx).Where("signal_id = ?", signalID).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListByTimeRange 按时间范围查询
func (r *ConfidenceSignalRepository) ListByTimeRange(ctx context.Context, start, end time.Time) ([]model.ConfidenceSignal, error) {
	var list []model.ConfidenceSignal
	err := db.GetDB().WithContext(ctx).
		Where("created_at >= ? AND created_at < ?", start, end).
		Order("created_at ASC").
		Find(&list).Error
	return list, err
}

// ListBySession 按 session 查询
func (r *ConfidenceSignalRepository) ListBySession(ctx context.Context, sessionID string, limit int) ([]model.ConfidenceSignal, error) {
	var list []model.ConfidenceSignal
	q := db.GetDB().WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&list).Error
	return list, err
}

// ============================================================================
// 2. ConfidenceCalibrationRepository
// ============================================================================

// ConfidenceCalibrationRepository 校准参数历史
type ConfidenceCalibrationRepository struct{}

// NewConfidenceCalibrationRepository 构造
func NewConfidenceCalibrationRepository() *ConfidenceCalibrationRepository {
	return &ConfidenceCalibrationRepository{}
}

// SaveActive 保存校准结果并标记为 active（同 signal_type 其他记录置 inactive）
func (r *ConfidenceCalibrationRepository) SaveActive(ctx context.Context, c *model.ConfidenceCalibration) error {
	return db.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 同 signal_type 其他记录置 inactive
		if err := tx.Model(&model.ConfidenceCalibration{}).
			Where("signal_type = ? AND is_active = true", c.SignalType).
			Update("is_active", false).Error; err != nil {
			return err
		}
		// 2) 插入新记录（active=true）
		c.IsActive = true
		return tx.Create(c).Error
	})
}

// GetActive 获取指定 signal_type 的当前 active 记录
func (r *ConfidenceCalibrationRepository) GetActive(ctx context.Context, signalType string) (*model.ConfidenceCalibration, error) {
	var c model.ConfidenceCalibration
	err := db.GetDB().WithContext(ctx).
		Where("signal_type = ? AND is_active = true", signalType).
		Order("created_at DESC").
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListBySignalType 列出指定 signal_type 的历史记录
func (r *ConfidenceCalibrationRepository) ListBySignalType(ctx context.Context, signalType string, limit int) ([]model.ConfidenceCalibration, error) {
	var list []model.ConfidenceCalibration
	q := db.GetDB().WithContext(ctx).
		Where("signal_type = ?", signalType).
		Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&list).Error
	return list, err
}

// ============================================================================
// 3. HandoffDecisionRepository
// ============================================================================

// HandoffDecisionRepository 转人工决策记录
type HandoffDecisionRepository struct{}

// NewHandoffDecisionRepository 构造
func NewHandoffDecisionRepository() *HandoffDecisionRepository {
	return &HandoffDecisionRepository{}
}

// Create 创建决策记录
func (r *HandoffDecisionRepository) Create(ctx context.Context, h *model.HandoffDecisionRecord) error {
	return db.GetDB().WithContext(ctx).Create(h).Error
}

// Update 更新决策记录
func (r *HandoffDecisionRepository) Update(ctx context.Context, h *model.HandoffDecisionRecord) error {
	return db.GetDB().WithContext(ctx).Save(h).Error
}

// GetByDecisionID 按 DecisionID 查询
func (r *HandoffDecisionRepository) GetByDecisionID(ctx context.Context, decisionID string) (*model.HandoffDecisionRecord, error) {
	var h model.HandoffDecisionRecord
	err := db.GetDB().WithContext(ctx).Where("decision_id = ?", decisionID).First(&h).Error
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// ListByTimeRange 按时间范围查询
func (r *HandoffDecisionRepository) ListByTimeRange(ctx context.Context, start, end time.Time) ([]model.HandoffDecisionRecord, error) {
	var list []model.HandoffDecisionRecord
	err := db.GetDB().WithContext(ctx).
		Where("created_at >= ? AND created_at < ?", start, end).
		Order("created_at ASC").
		Find(&list).Error
	return list, err
}

// ListBySession 按 session 查询
func (r *HandoffDecisionRepository) ListBySession(ctx context.Context, sessionID string, limit int) ([]model.HandoffDecisionRecord, error) {
	var list []model.HandoffDecisionRecord
	q := db.GetDB().WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&list).Error
	return list, err
}

// MarkAccepted 标记座席已接受
func (r *HandoffDecisionRepository) MarkAccepted(ctx context.Context, decisionID string, agentID int64) error {
	now := time.Now()
	return db.GetDB().WithContext(ctx).
		Model(&model.HandoffDecisionRecord{}).
		Where("decision_id = ?", decisionID).
		Updates(map[string]any{
			"assigned_agent_id": agentID,
			"accepted_at":       &now,
		}).Error
}

// MarkResolved 标记已解决
func (r *HandoffDecisionRepository) MarkResolved(ctx context.Context, decisionID string, customerAccepted bool) error {
	now := time.Now()
	return db.GetDB().WithContext(ctx).
		Model(&model.HandoffDecisionRecord{}).
		Where("decision_id = ?", decisionID).
		Updates(map[string]any{
			"resolved_at":       &now,
			"customer_accepted": customerAccepted,
		}).Error
}

// ============================================================================
// 4. ThresholdPolicyRepository
// ============================================================================

// ThresholdPolicyRepository 动态阈值策略
type ThresholdPolicyRepository struct{}

// NewThresholdPolicyRepository 构造
func NewThresholdPolicyRepository() *ThresholdPolicyRepository {
	return &ThresholdPolicyRepository{}
}

// Save 保存或更新策略
func (r *ThresholdPolicyRepository) Save(ctx context.Context, p *model.ThresholdPolicy) error {
	return db.GetDB().WithContext(ctx).Save(p).Error
}

// GetByIntent 获取指定意图的 active 策略
func (r *ThresholdPolicyRepository) GetByIntent(ctx context.Context, intentType string) (*model.ThresholdPolicy, error) {
	var p model.ThresholdPolicy
	err := db.GetDB().WithContext(ctx).
		Where("intent_type = ? AND is_active = true", intentType).
		Order("version DESC").
		First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListActive 列出所有 active 策略
func (r *ThresholdPolicyRepository) ListActive(ctx context.Context) ([]model.ThresholdPolicy, error) {
	var list []model.ThresholdPolicy
	err := db.GetDB().WithContext(ctx).
		Where("is_active = true").
		Order("intent_type ASC, version DESC").
		Find(&list).Error
	return list, err
}

// Deactivate 关闭指定意图的所有 active 策略
func (r *ThresholdPolicyRepository) Deactivate(ctx context.Context, intentType string) error {
	return db.GetDB().WithContext(ctx).
		Model(&model.ThresholdPolicy{}).
		Where("intent_type = ? AND is_active = true", intentType).
		Update("is_active", false).Error
}

// ============================================================================
// 7. ABTestRepository
// ============================================================================

// ABTestRepository A/B 测试配置与统计
type ABTestRepository struct{}

// NewABTestRepository 构造
func NewABTestRepository() *ABTestRepository {
	return &ABTestRepository{}
}

// Create 创建测试
func (r *ABTestRepository) Create(ctx context.Context, t *model.ABTest) error {
	return db.GetDB().WithContext(ctx).Create(t).Error
}

// Update 更新测试
func (r *ABTestRepository) Update(ctx context.Context, t *model.ABTest) error {
	return db.GetDB().WithContext(ctx).Save(t).Error
}

// GetByTestID 按 TestID 查询
func (r *ABTestRepository) GetByTestID(ctx context.Context, testID string) (*model.ABTest, error) {
	var t model.ABTest
	err := db.GetDB().WithContext(ctx).Where("test_id = ?", testID).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListByStatus 按 status 列出
func (r *ABTestRepository) ListByStatus(ctx context.Context, status string, limit int) ([]model.ABTest, error) {
	var list []model.ABTest
	q := db.GetDB().WithContext(ctx)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	q = q.Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&list).Error
	return list, err
}

// RecordMetric 记录单条指标样本
func (r *ABTestRepository) RecordMetric(ctx context.Context, m *model.ABTestMetric) error {
	return db.GetDB().WithContext(ctx).Create(m).Error
}

// ListMetricSamples 列出指定测试/分组/指标名的样本
func (r *ABTestRepository) ListMetricSamples(ctx context.Context, testID, group, metricName string) ([]float64, error) {
	var values []float64
	err := db.GetDB().WithContext(ctx).
		Model(&model.ABTestMetric{}).
		Where("test_id = ? AND group_name = ? AND metric_name = ?", testID, group, metricName).
		Order("created_at ASC").
		Pluck("value", &values).Error
	return values, err
}

// CountMetricSamples 统计样本数
func (r *ABTestRepository) CountMetricSamples(ctx context.Context, testID, group, metricName string) (int, error) {
	var count int64
	err := db.GetDB().WithContext(ctx).
		Model(&model.ABTestMetric{}).
		Where("test_id = ? AND group_name = ? AND metric_name = ?", testID, group, metricName).
		Count(&count).Error
	return int(count), err
}

// ============================================================================
// 管理面扩展方法（TuningController 使用，按业务域下沉，不新建上帝 service）
// ============================================================================

// ConfidenceBandStat 置信度决策带聚合结果
type ConfidenceBandStat struct {
	DecisionBand string `json:"decision_band"`
	Count        int64  `json:"count"`
}

// List 分页查询置信度信号（管理面）
// 注意：模型无 calculated_at 列，原 controller 使用 calculated_at 为潜在 bug，此处修正为 created_at。
func (r *ConfidenceSignalRepository) List(ctx context.Context, sessionID string, page, pageSize int) ([]model.ConfidenceSignal, int64, error) {
	q := db.GetDB().WithContext(ctx).Model(&model.ConfidenceSignal{})
	if sessionID != "" {
		q = q.Where("session_id = ?", sessionID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.ConfidenceSignal
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetByID 按主键查询置信度信号（管理面详情）
func (r *ConfidenceSignalRepository) GetByID(ctx context.Context, id string) (*model.ConfidenceSignal, error) {
	var s model.ConfidenceSignal
	if err := db.GetDB().WithContext(ctx).First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// StatsByBand 按决策带聚合统计（管理面）
func (r *ConfidenceSignalRepository) StatsByBand(ctx context.Context, since time.Time) ([]ConfidenceBandStat, error) {
	var rows []ConfidenceBandStat
	if err := db.GetDB().WithContext(ctx).
		Model(&model.ConfidenceSignal{}).
		Select("decision_band, COUNT(*) as count").
		Where("created_at > ?", since).
		Group("decision_band").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// List 分页查询置信度校准记录（管理面）
func (r *ConfidenceCalibrationRepository) List(ctx context.Context, signalType string, page, pageSize int) ([]model.ConfidenceCalibration, int64, error) {
	q := db.GetDB().WithContext(ctx).Model(&model.ConfidenceCalibration{})
	if signalType != "" {
		q = q.Where("signal_type = ?", signalType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.ConfidenceCalibration
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ListActivePolicies 返回启用中的阈值策略（管理面展示）
func (r *ThresholdPolicyRepository) ListActivePolicies(ctx context.Context) ([]model.ThresholdPolicy, error) {
	var list []model.ThresholdPolicy
	if err := db.GetDB().WithContext(ctx).
		Where("is_active = ?", true).
		Order("intent_type ASC").
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
