// Package repository 客户画像洞察数据访问（P-4 画像字段实现）
package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// CustomerProfileInsightRepository 客户画像补充数据访问接口（轻量聚合，供 Customer360 buildUserProfile 使用）
type CustomerProfileInsightRepository interface {
	// MessageHourHistogram 按小时(0-23)聚合指定会话在 since 之后的消息条数。
	MessageHourHistogram(ctx context.Context, sessionIDs []string, since time.Time) (map[int]int64, error)
}

type customerProfileInsightRepository struct {
	db *gorm.DB
}

// NewCustomerProfileInsightRepository 创建（全局 DB）
func NewCustomerProfileInsightRepository() CustomerProfileInsightRepository {
	return &customerProfileInsightRepository{db: _db.GetDB()}
}

// NewCustomerProfileInsightRepositoryWithDB 创建（注入 DB，测试用）
func NewCustomerProfileInsightRepositoryWithDB(db *gorm.DB) CustomerProfileInsightRepository {
	return &customerProfileInsightRepository{db: db}
}

// MessageHourHistogram 轻量 SQL：单条 GROUP BY 按小时聚合最近消息（P-4 PreferredTime 来源）。
func (r *customerProfileInsightRepository) MessageHourHistogram(ctx context.Context, sessionIDs []string, since time.Time) (map[int]int64, error) {
	out := make(map[int]int64)
	if r.db == nil || len(sessionIDs) == 0 {
		return out, nil
	}

	type row struct {
		Hour  int   `gorm:"column:hour"`
		Count int64 `gorm:"column:cnt"`
	}
	rows := make([]row, 0, 24)
	err := r.db.WithContext(ctx).
		Model(&model.SessionMessage{}).
		Select("EXTRACT(HOUR FROM created_at)::int AS hour, COUNT(*) AS cnt").
		Where("session_id IN ?", sessionIDs).
		Where("created_at >= ?", since).
		Group("hour").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, rw := range rows {
		if rw.Hour >= 0 && rw.Hour <= 23 {
			out[rw.Hour] = rw.Count
		}
	}
	return out, nil
}
