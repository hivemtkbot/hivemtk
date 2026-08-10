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
	// RecordBatch 批量插入事件，单次 SQL 替代「for event → Record」（CC- N+1 优化）
	RecordBatch(ctx context.Context, events []*model.CustomerEvent) error
	GetByCustomerID(ctx context.Context, customerID string, limit int) ([]*model.CustomerEvent, error)
	GetByTimeRange(ctx context.Context, start, end time.Time) ([]*model.CustomerEvent, error)
	GetStats(ctx context.Context, start, end time.Time) (*EventStats, error)
	DeleteByCustomerID(ctx context.Context, customerID string) (int64, error)
}

// customerEventRepository implements CustomerEventRepository
type customerEventRepository struct{}

// NewCustomerEventRepository creates a new CustomerEventRepository instance
func NewCustomerEventRepository() CustomerEventRepository {
	return &customerEventRepository{}
}

// Record records a new customer event
func (r *customerEventRepository) Record(ctx context.Context, event *model.CustomerEvent) error {
	return _db.GetDB().WithContext(ctx).Create(event).Error
}

// RecordBatch 批量插入事件（CC- N+1 优化）
//
// 使用 gorm.Create 批量插入，单次 SQL 取代 N 次单条插入。
//   - events 为空时直接返回 nil（noop）
//   - events 中含 nil 元素时跳过
func (r *customerEventRepository) RecordBatch(ctx context.Context, events []*model.CustomerEvent) error {
	if len(events) == 0 {
		return nil
	}
	// 过滤 nil 元素，避免 gorm 报 invalid value
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
		return _db.GetDB().WithContext(ctx).Create(filtered[0]).Error
	}
	return _db.GetDB().WithContext(ctx).Create(filtered).Error
}

// GetByCustomerID retrieves events for a specific customer
func (r *customerEventRepository) GetByCustomerID(ctx context.Context, customerID string, limit int) ([]*model.CustomerEvent, error) {
	var events []*model.CustomerEvent

	query := _db.GetDB().WithContext(ctx).Where("customer_id = ?", customerID)
	if limit > 0 {
		query = query.Order("occurred_at DESC").Limit(limit)
	}
	if err := query.Find(&events).Error; err != nil {
		return nil, err
	}

	return events, nil
}

// GetByTimeRange retrieves events within a time range
func (r *customerEventRepository) GetByTimeRange(ctx context.Context, start, end time.Time) ([]*model.CustomerEvent, error) {
	var events []*model.CustomerEvent

	if err := _db.GetDB().WithContext(ctx).
		Where("occurred_at >= ? AND occurred_at <= ?", start, end).
		Order("occurred_at ASC").
		Find(&events).Error; err != nil {
		return nil, err
	}

	return events, nil
}

// DeleteByCustomerID deletes all events for a specific customer, returns deleted count
func (r *customerEventRepository) DeleteByCustomerID(ctx context.Context, customerID string) (int64, error) {
	result := _db.GetDB().WithContext(ctx).Where("customer_id = ?", customerID).Delete(&model.CustomerEvent{})
	return result.RowsAffected, result.Error
}

func (r *customerEventRepository) GetStats(ctx context.Context, start, end time.Time) (*EventStats, error) {
	stats := &EventStats{}

	// Get total events
	var total int64
	if err := _db.GetDB().WithContext(ctx).
		Model(&model.CustomerEvent{}).
		Where("occurred_at >= ? AND occurred_at <= ?", start, end).
		Count(&total).Error; err != nil {
		return nil, err
	}
	stats.TotalEvents = total

	// Get events by type
	var typeCounts []struct {
		EventType model.EventType `gorm:"column:event_type"`
		Count     int64           `gorm:"column:count"`
	}
	if err := _db.GetDB().WithContext(ctx).
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

	// Get events by source
	var sourceCounts []struct {
		EventSource model.EventSource `gorm:"column:event_source"`
		Count       int64             `gorm:"column:count"`
	}
	if err := _db.GetDB().WithContext(ctx).
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
