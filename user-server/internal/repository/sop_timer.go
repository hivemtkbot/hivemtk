package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SOPTimerRepository SOP 定时器仓储
//
// 负责 sop_timers 表的读写，服务于 OutboxDispatcher 与 WaitExecutor。
type SOPTimerRepository struct {
	db *gorm.DB
}

// NewSOPTimerRepository 创建 SOP 定时器仓储
func NewSOPTimerRepository(db *gorm.DB) *SOPTimerRepository {
	return &SOPTimerRepository{db: db}
}

// Create 创建 SOP 定时器
func (r *SOPTimerRepository) Create(ctx context.Context, timer *model.SOPTimer) error {
	if r == nil || r.db == nil {
		return errors.New("sop timer repository not initialized")
	}
	return r.db.WithContext(ctx).Create(timer).Error
}

// FindDueForUpdate 查询到期 timer（FOR UPDATE SKIP LOCKED，多实例并发安全）
//
// 查询条件：status='pending' AND wait_until <= now
// 排序：wait_until ASC
// 调用方应在事务外使用本方法配合 MarkFired 实现幂等抢占。
func (r *SOPTimerRepository) FindDueForUpdate(ctx context.Context, now time.Time, limit int) ([]model.SOPTimer, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sop timer repository not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	var timers []model.SOPTimer
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("status = ? AND wait_until <= ?", "pending", now).
		Order("wait_until ASC").
		Limit(limit).
		Find(&timers).Error
	if err != nil {
		return nil, err
	}
	return timers, nil
}

// MarkFired 原子标记 timer 为 fired（防多实例重复处理），返回受影响行数
//
// 条件：id 匹配且 status='pending'，更新 status='fired' 与 fired_at=now
// RowsAffected=0 表示已被其他实例处理，调用方应跳过。
func (r *SOPTimerRepository) MarkFired(ctx context.Context, id uint, now time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop timer repository not initialized")
	}
	res := r.db.WithContext(ctx).Model(&model.SOPTimer{}).
		Where("id = ? AND status = ?", id, "pending").
		Updates(map[string]any{
			"status":   "fired",
			"fired_at": &now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// CountPendingByExecutionID 统计指定执行 ID 的 pending timer 数
//
// 用于卡死检测：有 pending timer 表示 wait 节点正在等待，不算卡死。
func (r *SOPTimerRepository) CountPendingByExecutionID(ctx context.Context, executionID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop timer repository not initialized")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.SOPTimer{}).
		Where("execution_id = ? AND status = ?", executionID, "pending").
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
