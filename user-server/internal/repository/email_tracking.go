package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// EmailTrackingRepository 邮件追踪仓库接口
type EmailTrackingRepository interface {
	CreateEvent(ctx context.Context, event *model.EmailTrackingEvent) error
	EventExists(ctx context.Context, eventID string) (bool, error)
	ListEventsByJob(ctx context.Context, jobID string, page, limit int) ([]*model.EmailTrackingEvent, int64, error)
	CountEventsByJob(ctx context.Context, jobID, eventType string) (int64, error)
	CountUniqueEmailsByJob(ctx context.Context, jobID, eventType string) (int64, error)
	ListEventsByRange(ctx context.Context, start, end time.Time) ([]*model.EmailTrackingEvent, error)
	CountEventsByRange(ctx context.Context, start, end time.Time, eventType string) (int64, error)

	GetJobMetric(ctx context.Context, jobID string) (*model.EmailJobMetric, error)
	UpsertJobMetric(ctx context.Context, metric *model.EmailJobMetric) error
	ListJobMetricsByRange(ctx context.Context, start, end time.Time) ([]*model.EmailJobMetric, error)
}

type emailTrackingRepo struct {
	db *gorm.DB
}

// NewEmailTrackingRepository 创建邮件追踪仓库
func NewEmailTrackingRepository(db *gorm.DB) EmailTrackingRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &emailTrackingRepo{db: db}
}

// CreateEvent 创建追踪事件（event_id 唯一）
func (r *emailTrackingRepo) CreateEvent(ctx context.Context, event *model.EmailTrackingEvent) error {
	if r == nil || r.db == nil {
		return errors.New("email tracking repository 未初始化（db is nil）")
	}
	return r.db.Create(event).Error
}

// EventExists 判断 event_id 是否已存在（webhook 重放幂等）
func (r *emailTrackingRepo) EventExists(ctx context.Context, eventID string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var count int64
	err := r.db.Model(&model.EmailTrackingEvent{}).Where("event_id = ?", eventID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListEventsByJob 分页查询任务的追踪事件
func (r *emailTrackingRepo) ListEventsByJob(ctx context.Context, jobID string, page, limit int) ([]*model.EmailTrackingEvent, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var events []*model.EmailTrackingEvent
	var total int64

	query := r.db.Model(&model.EmailTrackingEvent{}).Where("job_id = ?", jobID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("timestamp DESC").Offset(offset).Limit(limit).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// CountEventsByJob 统计任务某类事件总数
func (r *emailTrackingRepo) CountEventsByJob(ctx context.Context, jobID, eventType string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var count int64
	query := r.db.Model(&model.EmailTrackingEvent{}).Where("job_id = ?", jobID)
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	err := query.Count(&count).Error
	return count, err
}

// CountUniqueEmailsByJob 统计任务某类事件去重邮箱数（open_rate / click_rate 使用）
func (r *emailTrackingRepo) CountUniqueEmailsByJob(ctx context.Context, jobID, eventType string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var count int64
	err := r.db.Model(&model.EmailTrackingEvent{}).
		Where("job_id = ? AND event_type = ?", jobID, eventType).
		Distinct("email").Count(&count).Error
	return count, err
}

// ListEventsByRange 查询时间区间内的事件（聚合区间指标使用）
func (r *emailTrackingRepo) ListEventsByRange(ctx context.Context, start, end time.Time) ([]*model.EmailTrackingEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var events []*model.EmailTrackingEvent
	err := r.db.Where("timestamp BETWEEN ? AND ?", start, end).Find(&events).Error
	return events, err
}

// CountEventsByRange 统计时间区间内某类事件总数
func (r *emailTrackingRepo) CountEventsByRange(ctx context.Context, start, end time.Time, eventType string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var count int64
	query := r.db.Model(&model.EmailTrackingEvent{}).Where("timestamp BETWEEN ? AND ?", start, end)
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	err := query.Count(&count).Error
	return count, err
}

// GetJobMetric 获取任务指标
func (r *emailTrackingRepo) GetJobMetric(ctx context.Context, jobID string) (*model.EmailJobMetric, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var metric model.EmailJobMetric
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
func (r *emailTrackingRepo) UpsertJobMetric(ctx context.Context, metric *model.EmailJobMetric) error {
	if r == nil || r.db == nil {
		return errors.New("email tracking repository 未初始化（db is nil）")
	}
	existing, err := r.GetJobMetric(ctx, metric.JobID)
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
func (r *emailTrackingRepo) ListJobMetricsByRange(ctx context.Context, start, end time.Time) ([]*model.EmailJobMetric, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var metrics []*model.EmailJobMetric
	err := r.db.Where("updated_at BETWEEN ? AND ?", start, end).Find(&metrics).Error
	return metrics, err
}

