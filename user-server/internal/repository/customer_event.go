package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"
)

// EventTypeCount holds event count by event type
type EventTypeCount struct {
	EventType model.EventType `json:"event_type"`
	Count     int64           `json:"count"`
}

// EventSourceCount holds event count by event source
type EventSourceCount struct {
	EventSource model.EventSource `json:"event_source"`
	Count       int64             `json:"count"`
}

// EventStats holds statistics for events
type EventStats struct {
	TotalEvents   int64              `json:"total_events"`
	ByEventType   []EventTypeCount   `json:"by_event_type"`
	ByEventSource []EventSourceCount `json:"by_event_source"`
}

// CustomerEventRepository defines the interface for customer event data access
type CustomerEventRepository interface {
	Record(ctx context.Context, event *model.CustomerEvent) error
	RecordBatch(ctx context.Context, events []*model.CustomerEvent) error
	ReassignCustomerID(ctx context.Context, fromCustomerID, toCustomerID string) (int64, error)
	GetByCustomerID(ctx context.Context, customerID string, limit int) ([]*model.CustomerEvent, error)
	GetByTimeRange(ctx context.Context, start, end time.Time) ([]*model.CustomerEvent, error)
	GetStats(ctx context.Context, start, end time.Time) (*EventStats, error)
	DeleteByCustomerID(ctx context.Context, customerID string) (int64, error)
	ListGlobal(ctx context.Context, eventType string, limit, offset int) ([]*model.CustomerEvent, int64, error)
}

type customerEventRepository struct{}

// NewCustomerEventRepository creates a new CustomerEventRepository instance
func NewCustomerEventRepository() CustomerEventRepository {
	return &customerEventRepository{}
}

func (r *customerEventRepository) Record(ctx context.Context, event *model.CustomerEvent) error {
	return dbFromCtx(ctx).Create(event).Error
}

func (r *customerEventRepository) RecordBatch(ctx context.Context, events []*model.CustomerEvent) error {
	if len(events) == 0 {
		return nil
	}
	filtered := make([]*model.CustomerEvent, 0, len(events))
	for _, e := range events {
		if e != nil {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return dbFromCtx(ctx).Create(filtered[0]).Error
	}
	return dbFromCtx(ctx).Create(filtered).Error
}

func (r *customerEventRepository) ReassignCustomerID(ctx context.Context, fromCustomerID, toCustomerID string) (int64, error) {
	if fromCustomerID == "" || toCustomerID == "" || fromCustomerID == toCustomerID {
		return 0, nil
	}
	res := dbFromCtx(ctx).Model(&model.CustomerEvent{}).
		Where("customer_id = ?", fromCustomerID).
		Update("customer_id", toCustomerID)
	return res.RowsAffected, res.Error
}

func (r *customerEventRepository) GetByCustomerID(ctx context.Context, customerID string, limit int) ([]*model.CustomerEvent, error) {
	var events []*model.CustomerEvent

	query := dbFromCtx(ctx).Where("customer_id = ?", customerID).Order("occurred_at DESC")
	query = applyListLimit(query, limit)
	if err := query.Find(&events).Error; err != nil {
		return nil, err
	}

	return events, nil
}

func (r *customerEventRepository) GetByTimeRange(ctx context.Context, start, end time.Time) ([]*model.CustomerEvent, error) {
	var events []*model.CustomerEvent

	if err := dbFromCtx(ctx).
		Where("occurred_at >= ? AND occurred_at <= ?", start, end).
		Order("occurred_at ASC").
		Find(&events).Error; err != nil {
		return nil, err
	}

	return events, nil
}

func (r *customerEventRepository) DeleteByCustomerID(ctx context.Context, customerID string) (int64, error) {
	result := dbFromCtx(ctx).Where("customer_id = ?", customerID).Delete(&model.CustomerEvent{})
	return result.RowsAffected, result.Error
}

func (r *customerEventRepository) GetStats(ctx context.Context, start, end time.Time) (*EventStats, error) {
	stats := &EventStats{}

	var total int64
	if err := dbFromCtx(ctx).
		Model(&model.CustomerEvent{}).
		Where("occurred_at >= ? AND occurred_at <= ?", start, end).
		Count(&total).Error; err != nil {
		return nil, err
	}
	stats.TotalEvents = total

	var typeCounts []struct {
		EventType model.EventType `gorm:"column:event_type"`
		Count     int64           `gorm:"column:count"`
	}
	if err := dbFromCtx(ctx).
		Model(&model.CustomerEvent{}).
		Select("event_type, COUNT(*) as count").
		Where("occurred_at >= ? AND occurred_at <= ?", start, end).
		Group("event_type").
		Scan(&typeCounts).Error; err != nil {
		return nil, err
	}

	for _, tc := range typeCounts {
		stats.ByEventType = append(stats.ByEventType, EventTypeCount{
			EventType: tc.EventType,
			Count:     tc.Count,
		})
	}

	var sourceCounts []struct {
		EventSource model.EventSource `gorm:"column:event_source"`
		Count       int64             `gorm:"column:count"`
	}
	if err := dbFromCtx(ctx).
		Model(&model.CustomerEvent{}).
		Select("event_source, COUNT(*) as count").
		Where("occurred_at >= ? AND occurred_at <= ?", start, end).
		Group("event_source").
		Scan(&sourceCounts).Error; err != nil {
		return nil, err
	}

	for _, sc := range sourceCounts {
		stats.ByEventSource = append(stats.ByEventSource, EventSourceCount{
			EventSource: sc.EventSource,
			Count:       sc.Count,
		})
	}

	return stats, nil
}

func (r *customerEventRepository) ListGlobal(ctx context.Context, eventType string, limit, offset int) ([]*model.CustomerEvent, int64, error) {
	q := _db.GetDB().WithContext(ctx).Model(&model.CustomerEvent{})
	if eventType != "" {
		q = q.Where("event_type = ?", eventType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var list []*model.CustomerEvent
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, total, err
}
