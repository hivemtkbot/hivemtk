package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	"time"

	"gorm.io/gorm"
)

// SOPExecEventRepository SOP 执行事件仓储
//
// 负责 sop_exec_events 表的读写，服务于 SOPExecutionDispatcher（写事件）
// 与 SOPStuckDetector（查询最近事件以判断是否卡死）。
type SOPExecEventRepository struct {
	db *gorm.DB
}

// NewSOPExecEventRepository 创建 SOP 执行事件仓储
func NewSOPExecEventRepository(db *gorm.DB) *SOPExecEventRepository {
	return &SOPExecEventRepository{db: db}
}

// Create 创建 SOP 执行事件
//
// 事件日志的幂等性由唯一约束 (execution_id, node_id, attempt) 保证，
// 调用方应对重复写入错误降级处理（仅记录日志，不影响主流程）。
func (r *SOPExecEventRepository) Create(ctx context.Context, event *model.SOPExecEvent) error {
	if r == nil || r.db == nil {
		return errors.New("sop exec event repository not initialized")
	}
	return r.db.WithContext(ctx).Create(event).Error
}

// CountRecentByExecutionID 统计指定执行 ID 在 since 之后的执行事件数
//
// 用于卡死检测：最近有事件表示节点仍在推进，不算卡死。
func (r *SOPExecEventRepository) CountRecentByExecutionID(ctx context.Context, executionID uint, since time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop exec event repository not initialized")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPExecEvent{}).
		Where("execution_id = ? AND created_at > ?", executionID, since).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

