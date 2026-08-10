package repository

// feedback_learning_repository.go 反馈学习域仓储
//
// 五层架构归属: L4 数据访问层
// 设计依据: docs/核心链路优化.md 第十七章 §17.4
//
// 覆盖：SessionMessage(反馈学习专用查询)、SOPNodeTransition、
// OptimizationSuggestion、SalesChampionProfileSnapshot 的 CRUD。
// SOPAgent / SOPExecution 的查询复用 sop.go 中的 SopAgentRepository / SopExecutionRepository。
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// FeedbackLearningRepository 反馈学习域仓储
type FeedbackLearningRepository struct {
	db *gorm.DB
}

// NewFeedbackLearningRepository 创建反馈学习域仓储
func NewFeedbackLearningRepository(db *gorm.DB) *FeedbackLearningRepository {
	return &FeedbackLearningRepository{db: db}
}

// ----------------------------------------------------------------------------
// SessionMessage 查询（反馈学习专用）
// ----------------------------------------------------------------------------

// ListAIMessagesByPeriod 查询指定时间段内的 AI 回复消息
//
// 过滤：sender_type=ai AND created_at BETWEEN start AND end
// 排序：created_at ASC
// 限制：limit 条（limit<=0 时不限制）
func (r *FeedbackLearningRepository) ListAIMessagesByPeriod(ctx context.Context, start, end time.Time, limit int) ([]model.SessionMessage, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feedback learning repository not initialized")
	}
	q := r.db.WithContext(ctx).Model(&model.SessionMessage{}).
		Where("sender_type = ? AND created_at BETWEEN ? AND ?", "ai", start, end).
		Order("created_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var messages []model.SessionMessage
	if err := q.Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}

// ListCustomerMessagesBySessions 查询指定 sessionIDs 内的客户消息
//
// 过滤：session_id IN ? AND sender_type=user AND created_at BETWEEN start AND end
// 排序：created_at ASC
// sessionIDs 为空时返回 nil
func (r *FeedbackLearningRepository) ListCustomerMessagesBySessions(ctx context.Context, sessionIDs []string, start, end time.Time) ([]model.SessionMessage, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feedback learning repository not initialized")
	}
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	var msgs []model.SessionMessage
	err := r.db.WithContext(ctx).Model(&model.SessionMessage{}).
		Where("session_id IN ? AND sender_type = ? AND created_at BETWEEN ? AND ?",
			sessionIDs, "user", start, end).
		Order("created_at ASC").
		Find(&msgs).Error
	if err != nil {
		return nil, err
	}
	return msgs, nil
}

// ----------------------------------------------------------------------------
// SOPNodeTransition 节点流转记录
// ----------------------------------------------------------------------------

// CreateNodeTransition 创建节点流转记录
// t 为 nil 时直接返回 nil（与原 service 实现一致）
func (r *FeedbackLearningRepository) CreateNodeTransition(ctx context.Context, t *model.SOPNodeTransition) error {
	if r == nil || r.db == nil {
		return errors.New("feedback learning repository not initialized")
	}
	if t == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(t).Error
}

// ListNodeTransitionsBySOPAndVariant 查询节点流转记录
//
// 过滤：sop_id=? AND (variant=? 若 variant 非空)
// 排序：created_at ASC
func (r *FeedbackLearningRepository) ListNodeTransitionsBySOPAndVariant(ctx context.Context, sopID uint, variant string) ([]model.SOPNodeTransition, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feedback learning repository not initialized")
	}
	q := r.db.WithContext(ctx).Model(&model.SOPNodeTransition{}).
		Where("sop_id = ?", sopID)
	if variant != "" {
		q = q.Where("variant = ?", variant)
	}
	var transitions []model.SOPNodeTransition
	if err := q.Order("created_at ASC").Find(&transitions).Error; err != nil {
		return nil, err
	}
	return transitions, nil
}

// ----------------------------------------------------------------------------
// OptimizationSuggestion 优化建议
// ----------------------------------------------------------------------------

// CreateSuggestionsInBatches 批量创建优化建议
//
// batchSize 为每批写入条数（与原 CreateInBatches 一致）
// suggestions 为空时直接返回 nil
func (r *FeedbackLearningRepository) CreateSuggestionsInBatches(ctx context.Context, suggestions []model.OptimizationSuggestion, batchSize int) error {
	if r == nil || r.db == nil {
		return errors.New("feedback learning repository not initialized")
	}
	if len(suggestions) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(suggestions, batchSize).Error
}

// ListPendingSuggestions 列出待审核建议
//
// 过滤：status=pending AND (sop_id=? 若 sopID>0)
// 排序：priority DESC, generated_at DESC
// 限制：limit 条（limit<=0 时默认 50）
func (r *FeedbackLearningRepository) ListPendingSuggestions(ctx context.Context, sopID uint, limit int) ([]model.OptimizationSuggestion, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feedback learning repository not initialized")
	}
	if limit <= 0 {
		limit = 50
	}
	q := r.db.WithContext(ctx).Model(&model.OptimizationSuggestion{}).
		Where("status = ?", model.SuggestionStatusPending).
		Order("priority DESC, generated_at DESC").
		Limit(limit)
	if sopID > 0 {
		q = q.Where("sop_id = ?", sopID)
	}
	var list []model.OptimizationSuggestion
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateSuggestionFields 按 ID 更新建议指定字段
func (r *FeedbackLearningRepository) UpdateSuggestionFields(ctx context.Context, suggestionID uint, fields map[string]any) error {
	if r == nil || r.db == nil {
		return errors.New("feedback learning repository not initialized")
	}
	return r.db.WithContext(ctx).Model(&model.OptimizationSuggestion{}).
		Where("id = ?", suggestionID).
		Updates(fields).Error
}

// ----------------------------------------------------------------------------
// SalesChampionProfileSnapshot 销冠画像快照
// ----------------------------------------------------------------------------

// CreateProfileSnapshot 创建画像快照
func (r *FeedbackLearningRepository) CreateProfileSnapshot(ctx context.Context, snapshot *model.SalesChampionProfileSnapshot) error {
	if r == nil || r.db == nil {
		return errors.New("feedback learning repository not initialized")
	}
	return r.db.WithContext(ctx).Create(snapshot).Error
}

// ListLatestProfileSnapshots 列出最新画像快照
//
// 过滤：(staff_id=? 若 staffID>0) AND (scenario=? 若 scenario 非空)
// 排序：generated_at DESC
// 限制：limit 条
func (r *FeedbackLearningRepository) ListLatestProfileSnapshots(ctx context.Context, staffID uint, scenario string, limit int) ([]model.SalesChampionProfileSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feedback learning repository not initialized")
	}
	q := r.db.WithContext(ctx).Model(&model.SalesChampionProfileSnapshot{}).
		Order("generated_at DESC").
		Limit(limit)
	if staffID > 0 {
		q = q.Where("staff_id = ?", staffID)
	}
	if scenario != "" {
		q = q.Where("scenario = ?", scenario)
	}
	var list []model.SalesChampionProfileSnapshot
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
