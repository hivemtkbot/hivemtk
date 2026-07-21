package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
)

// FeedbackLoopRepository 反馈闭环域仓储（P0-3 反馈学习相关管理面读模型）
//
// 覆盖：反馈事件(FeedbackEvent)、销冠对话(ChampionDialogue)、
// Prompt 候选(PromptCandidate)、Bandit 臂(BanditArm) 的查询。
// 命名按业务域（feedback_loop），不按管理角色或优先级。
type FeedbackLoopRepository struct {
	db *gorm.DB
}

// NewFeedbackLoopRepository 构造（无参，内部取库句柄，遵循本包其他仓储约定）
func NewFeedbackLoopRepository() *FeedbackLoopRepository {
	return &FeedbackLoopRepository{db: db.GetDB()}
}

// ListFeedbackEvents 反馈事件分页列表
func (r *FeedbackLoopRepository) ListFeedbackEvents(ctx context.Context, sessionID, signalKey string, page, pageSize int) ([]model.FeedbackEvent, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.FeedbackEvent{})
	if sessionID != "" {
		q = q.Where("session_id = ?", sessionID)
	}
	if signalKey != "" {
		q = q.Where("signal_key = ?", signalKey)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.FeedbackEvent
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// FeedbackEventStat 反馈事件聚合结果
type FeedbackEventStat struct {
	SignalKey   string  `json:"signal_key"`
	Count       int64   `json:"count"`
	TotalReward float64 `json:"total_reward"`
}

// StatsFeedbackEvents 按信号键聚合
func (r *FeedbackLoopRepository) StatsFeedbackEvents(ctx context.Context, since time.Time) ([]FeedbackEventStat, error) {
	var rows []FeedbackEventStat
	if err := r.db.WithContext(ctx).
		Model(&model.FeedbackEvent{}).
		Select("signal_key, COUNT(*) as count, SUM(reward) as total_reward").
		Where("created_at > ?", since).
		Group("signal_key").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListChampionDialogues 销冠对话分页列表
//
// 注意：ChampionDialogue 模型无 intent/industry 列，原 controller 的这两个过滤为潜在 bug，
// 此处保留原始 Where 以不改变运行行为（仅在对应参数传入时触发）。
func (r *FeedbackLoopRepository) ListChampionDialogues(ctx context.Context, intent, industry string, page, pageSize int) ([]model.ChampionDialogue, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.ChampionDialogue{})
	if intent != "" {
		q = q.Where("intent = ?", intent)
	}
	if industry != "" {
		q = q.Where("industry = ?", industry)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.ChampionDialogue
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListPromptCandidates Prompt 候选分页列表
func (r *FeedbackLoopRepository) ListPromptCandidates(ctx context.Context, status string, page, pageSize int) ([]model.PromptCandidate, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.PromptCandidate{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.PromptCandidate
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// UpdatePromptCandidateStatus 更新 Prompt 候选状态
func (r *FeedbackLoopRepository) UpdatePromptCandidateStatus(ctx context.Context, id, status string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.PromptCandidate{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status, "updated_at": now}).Error
}

// ListBanditArms Bandit 臂分页列表
func (r *FeedbackLoopRepository) ListBanditArms(ctx context.Context, experimentID, sopID string, page, pageSize int) ([]model.BanditArm, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.BanditArm{})
	if experimentID != "" {
		q = q.Where("experiment_id = ?", experimentID)
	}
	if sopID != "" {
		q = q.Where("sop_id = ?", sopID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.BanditArm
	if err := q.Order("experiment_id DESC, arm_key ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
