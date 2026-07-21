package repository

import (
	"context"
	"errors"
	"marketing/internal/model"
	"time"

	"gorm.io/gorm"
)

// SopAgentRepository SOP 智能体仓储
type SopAgentRepository struct {
	db *gorm.DB
}

// NewSopAgentRepository 创建 SOP 智能体仓储
func NewSopAgentRepository(db *gorm.DB) *SopAgentRepository {
	return &SopAgentRepository{db: db}
}

// ListActiveByTriggerType 列出指定触发类型的活跃 SOP
func (r *SopAgentRepository) ListActiveByTriggerType(ctx context.Context, triggerType string) ([]model.SOPAgent, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var list []model.SOPAgent
	if err := r.db.WithContext(ctx).
		Where("is_active = ? AND trigger_type = ?", true, triggerType).
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateTriggerConfig 按 ID 更新 trigger_config 字段
func (r *SopAgentRepository) UpdateTriggerConfig(ctx context.Context, id uint, triggerConfig string) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.SOPAgent{}).
		Where("id = ?", id).
		Update("trigger_config", triggerConfig).Error
}

// ============================================================================
// SopExecutionRepository
// ============================================================================

// SopExecutionRepository SOP 执行记录仓储
type SopExecutionRepository struct {
	db *gorm.DB
}

// NewSopExecutionRepository 创建 SOP 执行记录仓储
func NewSopExecutionRepository(db *gorm.DB) *SopExecutionRepository {
	return &SopExecutionRepository{db: db}
}

// CleanupStuck 清理卡死的执行（started_at 早于 threshold 且 status=runningStatus 的记录批量标记为 failedStatus）
// 返回受影响行数
func (r *SopExecutionRepository) CleanupStuck(ctx context.Context, threshold time.Time, runningStatus, failedStatus string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("status = ? AND started_at < ?", runningStatus, threshold).
		Updates(map[string]any{
			"status":        failedStatus,
			"error_message": "执行超时，自动标记失败",
			"completed_at":  time.Now(),
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// CountBySOPIDAndStatus 统计某 SOP 指定状态的执行数
func (r *SopExecutionRepository) CountBySOPIDAndStatus(ctx context.Context, sopID uint, status string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop execution repository not initialized")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPExecution{}).
		Where("sop_id = ? AND status = ?", sopID, status).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
