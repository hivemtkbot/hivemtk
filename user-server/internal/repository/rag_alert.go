package repository

// rag_alert_repository.go RAG 风控预警仓储（C 域 P1 缺口 #3）
//
// 五层架构归属: L3 Repository 层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6.3 风控预警
//
// 职责:
//   - rag_alerts 表的 CRUD / 列表查询 / 批量解决
//   - 同窗口同类型预警幂等检查
//   - 严重度统计（持续窗口数）
//   - knowledge_documents 向量化失败率聚合

import (
	"context"
	"time"

	"gorm.io/gorm"

	kbmodel "marketing/internal/aiagent/knowledge/model"
	"marketing/internal/model"
)

// ----------------------------------------------------------------------------
// 仓储接口
// ----------------------------------------------------------------------------

// RagAlertRepository RAG 风控预警仓储接口
type RagAlertRepository interface {
	// Create 创建预警记录
	Create(ctx context.Context, alert *model.RagAlert) error
	// FindByID 按 ID 查询预警（不存在返回 gorm.ErrRecordNotFound）
	FindByID(ctx context.Context, id int64) (*model.RagAlert, error)
	// Save 保存预警（全字段更新）
	Save(ctx context.Context, alert *model.RagAlert) error
	// ListActive 查询活跃预警（未解决）；alertType 为空时查所有类型；按 created_at DESC
	ListActive(ctx context.Context, alertType string, limit int) ([]model.RagAlert, error)
	// ListHistory 查询预警历史（含已解决）；alertType 为空时查所有类型；按 created_at DESC
	ListHistory(ctx context.Context, alertType string, limit int) ([]model.RagAlert, error)
	// ResolveActive 批量解决指定类型的活跃预警；fields 为待更新字段；返回受影响行数
	ResolveActive(ctx context.Context, alertType string, fields map[string]any) (int64, error)
	// CountActiveByWindow 检查同窗口同类型是否已有活跃预警
	CountActiveByWindow(ctx context.Context, alertType string, windowStart, windowEnd time.Time) (int64, error)
	// CountByTypeSince 查询指定类型在 [since, currentWindowStart) 范围内的预警数（含已解决）
	CountByTypeSince(ctx context.Context, alertType string, since, currentWindowStart time.Time) (int64, error)
	// AggregateEmbeddingStatus 聚合 knowledge_documents 的向量化状态，返回 (total, failed)
	AggregateEmbeddingStatus(ctx context.Context) (total int64, failed int64, err error)
}

// ----------------------------------------------------------------------------
// 实现
// ----------------------------------------------------------------------------

type ragAlertRepo struct {
	db *gorm.DB
}

// NewRagAlertRepository 创建 RAG 风控预警仓储
func NewRagAlertRepository(db *gorm.DB) RagAlertRepository {
	return &ragAlertRepo{db: db}
}

// Create 创建预警记录
func (r *ragAlertRepo) Create(ctx context.Context, alert *model.RagAlert) error {
	return r.db.WithContext(ctx).Create(alert).Error
}

// FindByID 按 ID 查询预警
func (r *ragAlertRepo) FindByID(ctx context.Context, id int64) (*model.RagAlert, error) {
	var alert model.RagAlert
	if err := r.db.WithContext(ctx).First(&alert, id).Error; err != nil {
		return nil, err
	}
	return &alert, nil
}

// Save 保存预警（全字段更新）
func (r *ragAlertRepo) Save(ctx context.Context, alert *model.RagAlert) error {
	return r.db.WithContext(ctx).Save(alert).Error
}

// ListActive 查询活跃预警（未解决）
func (r *ragAlertRepo) ListActive(ctx context.Context, alertType string, limit int) ([]model.RagAlert, error) {
	var alerts []model.RagAlert
	q := r.db.WithContext(ctx).
		Where("resolved = false").
		Order("created_at DESC").
		Limit(limit)
	if alertType != "" {
		q = q.Where("alert_type = ?", alertType)
	}
	if err := q.Find(&alerts).Error; err != nil {
		return nil, err
	}
	return alerts, nil
}

// ListHistory 查询预警历史（含已解决）
func (r *ragAlertRepo) ListHistory(ctx context.Context, alertType string, limit int) ([]model.RagAlert, error) {
	var alerts []model.RagAlert
	q := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit)
	if alertType != "" {
		q = q.Where("alert_type = ?", alertType)
	}
	if err := q.Find(&alerts).Error; err != nil {
		return nil, err
	}
	return alerts, nil
}

// ResolveActive 批量解决指定类型的活跃预警
func (r *ragAlertRepo) ResolveActive(ctx context.Context, alertType string, fields map[string]any) (int64, error) {
	q := r.db.WithContext(ctx).
		Model(&model.RagAlert{}).
		Where("resolved = false")
	if alertType != "" {
		q = q.Where("alert_type = ?", alertType)
	}
	res := q.Updates(fields)
	if err := res.Error; err != nil {
		return 0, err
	}
	return res.RowsAffected, nil
}

// CountActiveByWindow 检查同窗口同类型是否已有活跃预警
func (r *ragAlertRepo) CountActiveByWindow(ctx context.Context, alertType string, windowStart, windowEnd time.Time) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.RagAlert{}).
		Where("alert_type = ? AND window_start = ? AND window_end = ? AND resolved = false",
			alertType, windowStart, windowEnd).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountByTypeSince 查询指定类型在 [since, currentWindowStart) 范围内的预警数
func (r *ragAlertRepo) CountByTypeSince(ctx context.Context, alertType string, since, currentWindowStart time.Time) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.RagAlert{}).
		Where("alert_type = ? AND window_start >= ? AND window_start < ?",
			alertType, since, currentWindowStart).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// AggregateEmbeddingStatus 聚合 knowledge_documents 的向量化状态
//
// SQL 与原 service 实现一致：使用 COUNT(*) FILTER (WHERE embed_status = 'failed') 统计失败数
func (r *ragAlertRepo) AggregateEmbeddingStatus(ctx context.Context) (int64, int64, error) {
	var stats struct {
		Total  int64
		Failed int64
	}
	if err := r.db.WithContext(ctx).
		Model(&kbmodel.KnowledgeDocument{}).
		Select(`
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE embed_status = 'failed') AS failed
		`).
		Scan(&stats).Error; err != nil {
		return 0, 0, err
	}
	return stats.Total, stats.Failed, nil
}

// 编译期断言
var _ RagAlertRepository = (*ragAlertRepo)(nil)
