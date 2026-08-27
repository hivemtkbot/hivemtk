package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// FeedbackRecordRepository 反馈记录仓储
//
// 负责 feedback_records 表的写入与查询。FeedbackLearner 在 RecordFeedback
// 时通过本仓储落库，失败仅记 warn 不影响内存缓存与业务链路。
type FeedbackRecordRepository struct {
	db *gorm.DB
}

// NewFeedbackRecordRepository 创建反馈记录仓储
func NewFeedbackRecordRepository(db *gorm.DB) *FeedbackRecordRepository {
	return &FeedbackRecordRepository{db: db}
}

// NewFeedbackRecordRepositoryWithTx 在事务中复用仓储
func NewFeedbackRecordRepositoryWithTx(tx *gorm.DB) *FeedbackRecordRepository {
	return &FeedbackRecordRepository{db: tx}
}

// Create 写入一条反馈记录
func (r *FeedbackRecordRepository) Create(ctx context.Context, orm *model.FeedbackRecordORM) error {
	if r == nil || r.db == nil {
		return errors.New("feedback record repository not initialized")
	}
	if orm == nil {
		return errors.New("feedback record orm is nil")
	}
	return r.db.WithContext(ctx).Create(orm).Error
}
