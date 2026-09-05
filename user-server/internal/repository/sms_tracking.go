package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// SmsTrackingRepository 短信追踪仓库接口
type SmsTrackingRepository interface {
	CreateStatus(ctx context.Context, status *model.SmsDeliveryStatus) error
	UpdateStatus(ctx context.Context, status *model.SmsDeliveryStatus) error
	GetByMessageID(ctx context.Context, messageID string) (*model.SmsDeliveryStatus, error)
	MessageIDExists(ctx context.Context, messageID string) (bool, error)

	ListStatusesByJob(ctx context.Context, jobID string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error)
	ListStatusesByPhone(ctx context.Context, phone string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error)
	ListRetryableStatuses(ctx context.Context, limit int) ([]*model.SmsDeliveryStatus, error)
	ListStatusesByRange(ctx context.Context, start, end time.Time) ([]*model.SmsDeliveryStatus, error)

	CountByJob(ctx context.Context, jobID, status string) (int64, error)
	CountByRange(ctx context.Context, start, end time.Time, status string) (int64, error)

	GetJobMetric(ctx context.Context, jobID string) (*model.SmsJobMetric, error)
	UpsertJobMetric(ctx context.Context, metric *model.SmsJobMetric) error
	ListJobMetricsByRange(ctx context.Context, start, end time.Time) ([]*model.SmsJobMetric, error)
}

type smsTrackingRepo struct {
	db *gorm.DB
}

// NewSmsTrackingRepository 创建短信追踪仓库
//
// db 为 nil 时尝试从全局获取（_db.GetDB()）；若全局也未初始化（如测试场景未 SetTestDB），
// 则返回 db=nil 的 repo，所有方法走 nil 兜底分支返回错误或空结果，避免 panic。
func NewSmsTrackingRepository(db *gorm.DB) SmsTrackingRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &smsTrackingRepo{db: db}
}

var errNilDB = fmt.Errorf("sms tracking repo: db is nil")

func (r *smsTrackingRepo) CreateStatus(ctx context.Context, status *model.SmsDeliveryStatus) error {
	if r == nil || r.db == nil {
		return errNilDB
	}
	return r.db.WithContext(ctx).Create(status).Error
}

func (r *smsTrackingRepo) UpdateStatus(ctx context.Context, status *model.SmsDeliveryStatus) error {
	if r == nil || r.db == nil {
		return errNilDB
	}
	return r.db.WithContext(ctx).Save(status).Error
}

func (r *smsTrackingRepo) GetByMessageID(ctx context.Context, messageID string) (*model.SmsDeliveryStatus, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var status model.SmsDeliveryStatus
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).First(&status).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &status, nil
}

func (r *smsTrackingRepo) MessageIDExists(ctx context.Context, messageID string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errNilDB
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.SmsDeliveryStatus{}).Where("message_id = ?", messageID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *smsTrackingRepo) ListStatusesByJob(ctx context.Context, jobID string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, nil
	}
	var statuses []*model.SmsDeliveryStatus
	var total int64

	query := r.db.WithContext(ctx).Model(&model.SmsDeliveryStatus{}).Where("job_id = ?", jobID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("received_at DESC").Offset(offset).Limit(limit).Find(&statuses).Error; err != nil {
		return nil, 0, err
	}
	return statuses, total, nil
}

func (r *smsTrackingRepo) ListStatusesByPhone(ctx context.Context, phone string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, nil
	}
	var statuses []*model.SmsDeliveryStatus
	var total int64

	query := r.db.WithContext(ctx).Model(&model.SmsDeliveryStatus{}).Where("phone = ?", phone)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("received_at DESC").Offset(offset).Limit(limit).Find(&statuses).Error; err != nil {
		return nil, 0, err
	}
	return statuses, total, nil
}

func (r *smsTrackingRepo) ListRetryableStatuses(ctx context.Context, limit int) ([]*model.SmsDeliveryStatus, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var statuses []*model.SmsDeliveryStatus
	err := r.db.WithContext(ctx).Where("is_retryable = ? AND status = ?",
		true, model.SmsStatusRetryable).
		Order("received_at ASC").
		Limit(limit).
		Find(&statuses).Error
	return statuses, err
}

func (r *smsTrackingRepo) ListStatusesByRange(ctx context.Context, start, end time.Time) ([]*model.SmsDeliveryStatus, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var statuses []*model.SmsDeliveryStatus
	err := r.db.WithContext(ctx).Where("received_at BETWEEN ? AND ?", start, end).Find(&statuses).Error
	return statuses, err
}

func (r *smsTrackingRepo) CountByJob(ctx context.Context, jobID, status string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	query := r.db.WithContext(ctx).Model(&model.SmsDeliveryStatus{}).Where("job_id = ?", jobID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Count(&count).Error
	return count, err
}

func (r *smsTrackingRepo) CountByRange(ctx context.Context, start, end time.Time, status string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	query := r.db.WithContext(ctx).Model(&model.SmsDeliveryStatus{}).Where("received_at BETWEEN ? AND ?", start, end)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Count(&count).Error
	return count, err
}

func (r *smsTrackingRepo) GetJobMetric(ctx context.Context, jobID string) (*model.SmsJobMetric, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var metric model.SmsJobMetric
	err := r.db.WithContext(ctx).Where("job_id = ?", jobID).First(&metric).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &metric, nil
}

func (r *smsTrackingRepo) UpsertJobMetric(ctx context.Context, metric *model.SmsJobMetric) error {
	if r == nil || r.db == nil {
		return errNilDB
	}
	existing, err := r.GetJobMetric(ctx, metric.JobID)
	if err != nil {
		return err
	}
	if existing != nil {
		metric.ID = existing.ID
		return r.db.WithContext(ctx).Save(metric).Error
	}
	return r.db.WithContext(ctx).Create(metric).Error
}

func (r *smsTrackingRepo) ListJobMetricsByRange(ctx context.Context, start, end time.Time) ([]*model.SmsJobMetric, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var metrics []*model.SmsJobMetric
	err := r.db.WithContext(ctx).Where("updated_at BETWEEN ? AND ?", start, end).Find(&metrics).Error
	return metrics, err
}
