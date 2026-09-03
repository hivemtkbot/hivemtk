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

// UpdateCustomerAcceptBySession 客户消息抵达时，按 session_id 标记最近一条 feedback_record 的 customer_accept=true
//
// 设计意图：FeedbackRecorder.recordFeedback 创建时 CustomerAccept=false（生成时尚未知客户是否接受），
// 当客户下一条消息进来时（inbound 落库成功后），意味着客户收到了 AI 回复并继续对话 = 接受了。
// 此方法在 inbox persist 钩子中调用，标记后自学习闭环才能拿到有效信号。
//
// 幂等：UPDATE customer_accept=true 是幂等操作，重复调用不产生副作用。
func (r *FeedbackRecordRepository) UpdateCustomerAcceptBySession(ctx context.Context, sessionID string) (int64, error) {
	if r == nil || r.db == nil || sessionID == "" {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Model(&model.FeedbackRecordORM{}).
		Where("session_id = ?", sessionID).
		Where("customer_accept = ?", false).
		Order("id DESC").
		Limit(1).
		Update("customer_accept", true)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// UpdateTransferredBySession 转人工时标记 feedback_record 的 transferred=true + transfer_reason
func (r *FeedbackRecordRepository) UpdateTransferredBySession(ctx context.Context, sessionID, reason string) (int64, error) {
	if r == nil || r.db == nil || sessionID == "" {
		return 0, nil
	}
	updates := map[string]any{
		"transferred":      true,
		"transfer_reason":  reason,
	}
	res := r.db.WithContext(ctx).
		Model(&model.FeedbackRecordORM{}).
		Where("session_id = ?", sessionID).
		Where("transferred = ?", false).
		Order("id DESC").
		Limit(1).
		Updates(updates)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}
