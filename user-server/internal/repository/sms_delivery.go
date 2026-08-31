package repository

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// SmsDeliveryAggregateRow 到达率基础聚合行
type SmsDeliveryAggregateRow struct {
	Total     int64
	Delivered int64
	Failed    int64
	Retryable int64
}

// SmsDeliveryCarrierStatRow 按运营商维度统计行
type SmsDeliveryCarrierStatRow struct {
	Provider  string
	Total     int64
	Delivered int64
	Failed    int64
}

// SmsDeliveryRepository 短信到达率追踪仓库接口
//
// 配套 service.SmsDeliveryTrackerService，封装 sms_number_portability_logs 与
// sms_delivery_statuses 两张表的所有 DB 操作。service 不再直接持有 *gorm.DB。
type SmsDeliveryRepository interface {
	CreatePortability(ctx context.Context, rec *model.SmsNumberPortabilityRecord) error
	LoadLatestPortability(ctx context.Context, limit int) ([]model.SmsNumberPortabilityRecord, error)
	ListPortability(ctx context.Context, phone string, page, limit int) ([]model.SmsNumberPortabilityRecord, int64, error)

	GetDeliveryAggregate(ctx context.Context, start, end time.Time) (*SmsDeliveryAggregateRow, error)
	CountBlacklisted(ctx context.Context, start, end time.Time) (int64, error)
	CountPortabilityFailure(ctx context.Context, start, end time.Time) (int64, error)
	GetCarrierStats(ctx context.Context, start, end time.Time) ([]SmsDeliveryCarrierStatRow, error)
}

type smsDeliveryRepo struct {
	db *gorm.DB
}

// NewSmsDeliveryRepository 创建短信到达率追踪仓库
func NewSmsDeliveryRepository(db *gorm.DB) SmsDeliveryRepository {
	return &smsDeliveryRepo{db: db}
}

// CreatePortability 创建携号转网记录
//
// nil 兜底：db 未注入时静默返回 nil（测试场景），避免 nil pointer panic。
// 生产环境 db 必须非空（由 NewSmsDeliveryRepository 构造时保证）。
func (r *smsDeliveryRepo) CreatePortability(ctx context.Context, rec *model.SmsNumberPortabilityRecord) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(rec).Error
}

// LoadLatestPortability 加载最近 N 条携号转网记录（按 detected_at 倒序）
//
// 与原实现一致：不显式 WithContext（原 service 调用 s.db.Order(...).Find(...) 也未带 ctx）。
// nil 兜底：db 未注入时返回空切片与 nil error，避免 nil pointer panic（测试场景）。
func (r *smsDeliveryRepo) LoadLatestPortability(ctx context.Context, limit int) ([]model.SmsNumberPortabilityRecord, error) {
	if r == nil || r.db == nil {
		return []model.SmsNumberPortabilityRecord{}, nil
	}
	rows := []model.SmsNumberPortabilityRecord{}
	if err := r.db.Order("detected_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListPortability 分页查询携号转网记录（按 detected_at 倒序），可选 phone 过滤
//
// nil 兜底：db 未注入时返回空结果（测试场景）。
func (r *smsDeliveryRepo) ListPortability(ctx context.Context, phone string, page, limit int) ([]model.SmsNumberPortabilityRecord, int64, error) {
	if r == nil || r.db == nil {
		return []model.SmsNumberPortabilityRecord{}, 0, nil
	}
	q := r.db.WithContext(ctx).Model(&model.SmsNumberPortabilityRecord{})
	if phone != "" {
		q = q.Where("phone = ?", phone)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count portability: %w", err)
	}

	var rows []model.SmsNumberPortabilityRecord
	if err := q.Order("detected_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list portability: %w", err)
	}
	return rows, total, nil
}

// GetDeliveryAggregate 聚合时间窗口内的到达率基础指标
//
// Raw SQL 与原 service 实现完全一致（含 PostgreSQL FILTER 子句）。
// nil 兜底：db 未注入时返回错误（service 层 GetDeliveryRateMetrics 期望 error）。
func (r *smsDeliveryRepo) GetDeliveryAggregate(ctx context.Context, start, end time.Time) (*SmsDeliveryAggregateRow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("sms delivery repo: db is nil")
	}
	var row SmsDeliveryAggregateRow
	if err := r.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ?", start, end).
		Select(`
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed,
			COUNT(*) FILTER (WHERE status = 'retryable') AS retryable
		`).
		Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("query delivery aggregate: %w", err)
	}
	return &row, nil
}

// CountBlacklisted 统计黑名单触达失败数（error_code LIKE 'ERR_4002%'）
func (r *smsDeliveryRepo) CountBlacklisted(ctx context.Context, start, end time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var n int64
	err := r.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ? AND error_code LIKE ?", start, end, "ERR_4002%").
		Count(&n).Error
	return n, err
}

// CountPortabilityFailure 统计携号转网触达失败数
//
// 与原实现一致：error_code = 'ERR_4005' OR error_msg LIKE '%携号转网%'
func (r *smsDeliveryRepo) CountPortabilityFailure(ctx context.Context, start, end time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var n int64
	err := r.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ? AND (error_code = 'ERR_4005' OR error_msg LIKE '%携号转网%')", start, end).
		Count(&n).Error
	return n, err
}

// GetCarrierStats 按运营商维度统计（group by provider）
//
// Raw SQL 与原 service 实现完全一致（含 PostgreSQL FILTER 子句 + COALESCE）。
func (r *smsDeliveryRepo) GetCarrierStats(ctx context.Context, start, end time.Time) ([]SmsDeliveryCarrierStatRow, error) {
	if r == nil || r.db == nil {
		return []SmsDeliveryCarrierStatRow{}, nil
	}
	var rows []SmsDeliveryCarrierStatRow
	if err := r.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ?", start, end).
		Select(`
			COALESCE(provider, 'unknown') AS provider,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed
		`).
		Group("provider").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
