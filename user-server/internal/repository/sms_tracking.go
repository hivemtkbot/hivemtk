package repository

import (
	"errors"
	"time"

	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// SmsTrackingRepository 短信追踪仓库接口
type SmsTrackingRepository interface {
	// 单条状态
	CreateStatus(status *model.SmsDeliveryStatus) error
	UpdateStatus(status *model.SmsDeliveryStatus) error
	GetByMessageID(messageID string) (*model.SmsDeliveryStatus, error)
	MessageIDExists(messageID string) (bool, error)

	// 批量查询
	ListStatusesByJob(jobID string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error)
	ListStatusesByPhone(phone string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error)
	ListRetryableStatuses(limit int) ([]*model.SmsDeliveryStatus, error)
	ListStatusesByRange(start, end time.Time) ([]*model.SmsDeliveryStatus, error)

	// 统计
	CountByJob(jobID, status string) (int64, error)
	CountByRange(start, end time.Time, status string) (int64, error)

	// 任务指标
	GetJobMetric(jobID string) (*model.SmsJobMetric, error)
	UpsertJobMetric(metric *model.SmsJobMetric) error
	ListJobMetricsByRange(start, end time.Time) ([]*model.SmsJobMetric, error)
}

type smsTrackingRepo struct {
	db *gorm.DB
}

// NewSmsTrackingRepository 创建短信追踪仓库
func NewSmsTrackingRepository(db *gorm.DB) SmsTrackingRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &smsTrackingRepo{db: db}
}

// CreateStatus 创建送达状态记录
func (r *smsTrackingRepo) CreateStatus(status *model.SmsDeliveryStatus) error {
	return r.db.Create(status).Error
}

// UpdateStatus 更新送达状态记录
func (r *smsTrackingRepo) UpdateStatus(status *model.SmsDeliveryStatus) error {
	return r.db.Save(status).Error
}

// GetByMessageID 根据消息 ID 查询状态
func (r *smsTrackingRepo) GetByMessageID(messageID string) (*model.SmsDeliveryStatus, error) {
	var status model.SmsDeliveryStatus
	err := r.db.Where("message_id = ?", messageID).First(&status).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &status, nil
}

// MessageIDExists 判断 message_id 是否已存在（webhook 重放幂等）
func (r *smsTrackingRepo) MessageIDExists(messageID string) (bool, error) {
	var count int64
	err := r.db.Model(&model.SmsDeliveryStatus{}).Where("message_id = ?", messageID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListStatusesByJob 分页查询任务的送达状态
func (r *smsTrackingRepo) ListStatusesByJob(jobID string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error) {
	var statuses []*model.SmsDeliveryStatus
	var total int64

	query := r.db.Model(&model.SmsDeliveryStatus{}).Where("job_id = ?", jobID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("received_at DESC").Offset(offset).Limit(limit).Find(&statuses).Error; err != nil {
		return nil, 0, err
	}
	return statuses, total, nil
}

// ListStatusesByPhone 分页查询手机号的送达状态
func (r *smsTrackingRepo) ListStatusesByPhone(phone string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error) {
	var statuses []*model.SmsDeliveryStatus
	var total int64

	query := r.db.Model(&model.SmsDeliveryStatus{}).Where("phone = ?", phone)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("received_at DESC").Offset(offset).Limit(limit).Find(&statuses).Error; err != nil {
		return nil, 0, err
	}
	return statuses, total, nil
}

// ListRetryableStatuses 查询可重试的失败状态（用于定时重试任务）
// 同时返回已达最大重试次数但状态仍为 retryable 的记录，
// 由 service 层判断是否需要将其标记为 failed 并停止重试
func (r *smsTrackingRepo) ListRetryableStatuses(limit int) ([]*model.SmsDeliveryStatus, error) {
	var statuses []*model.SmsDeliveryStatus
	err := r.db.Where("is_retryable = ? AND status = ?",
		true, model.SmsStatusRetryable).
		Order("received_at ASC").
		Limit(limit).
		Find(&statuses).Error
	return statuses, err
}

// ListStatusesByRange 查询时间区间内的状态记录
func (r *smsTrackingRepo) ListStatusesByRange(start, end time.Time) ([]*model.SmsDeliveryStatus, error) {
	var statuses []*model.SmsDeliveryStatus
	err := r.db.Where("received_at BETWEEN ? AND ?", start, end).Find(&statuses).Error
	return statuses, err
}

// CountByJob 统计任务某状态的总数
func (r *smsTrackingRepo) CountByJob(jobID, status string) (int64, error) {
	var count int64
	query := r.db.Model(&model.SmsDeliveryStatus{}).Where("job_id = ?", jobID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Count(&count).Error
	return count, err
}

// CountByRange 统计时间区间内某状态的总数
func (r *smsTrackingRepo) CountByRange(start, end time.Time, status string) (int64, error) {
	var count int64
	query := r.db.Model(&model.SmsDeliveryStatus{}).Where("received_at BETWEEN ? AND ?", start, end)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Count(&count).Error
	return count, err
}

// GetJobMetric 获取任务指标
func (r *smsTrackingRepo) GetJobMetric(jobID string) (*model.SmsJobMetric, error) {
	var metric model.SmsJobMetric
	err := r.db.Where("job_id = ?", jobID).First(&metric).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &metric, nil
}

// UpsertJobMetric 创建或更新任务指标
func (r *smsTrackingRepo) UpsertJobMetric(metric *model.SmsJobMetric) error {
	existing, err := r.GetJobMetric(metric.JobID)
	if err != nil {
		return err
	}
	if existing != nil {
		metric.ID = existing.ID
		return r.db.Save(metric).Error
	}
	return r.db.Create(metric).Error
}

// ListJobMetricsByRange 查询时间区间内的任务指标
func (r *smsTrackingRepo) ListJobMetricsByRange(start, end time.Time) ([]*model.SmsJobMetric, error) {
	var metrics []*model.SmsJobMetric
	err := r.db.Where("updated_at BETWEEN ? AND ?", start, end).Find(&metrics).Error
	return metrics, err
}
