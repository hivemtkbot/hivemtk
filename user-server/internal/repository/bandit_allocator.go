package repository


import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// GetRunningPromptABTestBySOPNode 查询指定 SOP 节点 running 状态的 Prompt A/B 测试
//
// 未找到时返回 gorm.ErrRecordNotFound（调用方按需转换为业务错误或默认值）
func (r *FeedbackLoopRepository) GetRunningPromptABTestBySOPNode(ctx context.Context, sopID uint, nodeID string) (*model.PromptABTest, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var test model.PromptABTest
	err := r.db.WithContext(ctx).
		Where("sop_id = ? AND sop_node_id = ? AND status = ?", sopID, nodeID, model.PromptABTestStatusRunning).
		First(&test).Error
	if err != nil {
		return nil, err
	}
	return &test, nil
}

// GetBanditArmByExperimentAndKey 查询指定实验 + arm_key 的 bandit arm
//
// 未找到时返回 gorm.ErrRecordNotFound
func (r *FeedbackLoopRepository) GetBanditArmByExperimentAndKey(ctx context.Context, experimentID, armKey string) (*model.BanditArm, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var arm model.BanditArm
	err := r.db.WithContext(ctx).
		Where("experiment_id = ? AND arm_key = ?", experimentID, armKey).
		First(&arm).Error
	if err != nil {
		return nil, err
	}
	return &arm, nil
}

// UpdateBanditArmReward 更新臂的奖励（成功/失败）
//
// 成功：alpha += 1, success_trials += 1
// 失败：beta += 1
// 通用：total_trials += 1, sum_reward += reward, avg_reward 重算, updated_at 刷新
func (r *FeedbackLoopRepository) UpdateBanditArmReward(ctx context.Context, experimentID, armKey string, success bool, reward float64) error {
	if r == nil || r.db == nil {
		return nil
	}
	updates := map[string]any{
		"total_trials": gorm.Expr("total_trials + 1"),
		"sum_reward":   gorm.Expr("sum_reward + ?", reward),
		"avg_reward":   gorm.Expr("CASE WHEN total_trials + 1 > 0 THEN (sum_reward + ?) / (total_trials + 1) ELSE 0 END", reward),
		"updated_at":   time.Now(),
	}
	if success {
		updates["alpha"] = gorm.Expr("alpha + 1")
		updates["success_trials"] = gorm.Expr("success_trials + 1")
	} else {
		updates["beta"] = gorm.Expr("beta + 1")
	}
	return r.db.WithContext(ctx).Model(&model.BanditArm{}).
		Where("experiment_id = ? AND arm_key = ?", experimentID, armKey).
		Updates(updates).Error
}

// PromoteBanditArmWinner 事务：提升胜出臂 + 淘汰其他臂
//
//  1. winner → status=promoted, promoted_at=NOW()
//  2. 其他 → status=retired, retired_at=NOW()
func (r *FeedbackLoopRepository) PromoteBanditArmWinner(ctx context.Context, experimentID, winnerKey string) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&model.BanditArm{}).
			Where("experiment_id = ? AND arm_key = ?", experimentID, winnerKey).
			Updates(map[string]any{
				"status":      model.BanditArmStatusPromoted,
				"promoted_at": now,
				"updated_at":  now,
			}).Error; err != nil {
			return fmt.Errorf("promote winner: %w", err)
		}
		if err := tx.Model(&model.BanditArm{}).
			Where("experiment_id = ? AND arm_key != ?", experimentID, winnerKey).
			Updates(map[string]any{
				"status":     model.BanditArmStatusRetired,
				"retired_at": now,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("retire losers: %w", err)
		}
		return nil
	})
}

// ListActiveBanditArms 查询指定实验中处于 exploring/exploiting 状态的臂
//
// 用于 Thompson Sampling 采样前的臂列表加载
func (r *FeedbackLoopRepository) ListActiveBanditArms(ctx context.Context, experimentID string) ([]*model.BanditArm, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var arms []*model.BanditArm
	err := r.db.WithContext(ctx).
		Where("experiment_id = ? AND status IN ?", experimentID, []string{model.BanditArmStatusExploring, model.BanditArmStatusExploiting}).
		Find(&arms).Error
	if err != nil {
		return nil, err
	}
	return arms, nil
}

// UpdateBanditArmLastSampled 更新臂的 last_sampled_at（异步标记采样时间）
//
// 用于 markSampledAsync：不阻塞主链路，单字段更新
func (r *FeedbackLoopRepository) UpdateBanditArmLastSampled(ctx context.Context, experimentID, armKey string, sampledAt time.Time) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.BanditArm{}).
		Where("experiment_id = ? AND arm_key = ?", experimentID, armKey).
		Update("last_sampled_at", sampledAt).Error
}

