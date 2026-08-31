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

// FindClaimExhaustedPendingTimers S1-5：扫描 claim_count ≥ maxClaims 的 pending timer，
// 用于死信迁移兜底。兼容旧数据：payload->>'claim_count' 回退整数解析。
func (r *SOPTimerRepository) FindClaimExhaustedPendingTimers(ctx context.Context, maxClaims, limit int) ([]model.SOPTimer, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sop timer repository not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	const claimExpr = `(CASE WHEN payload->>'claim_count' ~ '^[0-9]+$' THEN (payload->>'claim_count')::int ELSE 0 END)`
	var list []model.SOPTimer
	err := r.db.WithContext(ctx).
		Where("status = ? AND (claim_count >= ? OR "+claimExpr+" >= ?)",
			"pending", maxClaims, maxClaims).
		Limit(limit).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// FindMaxWaitOverduePendingTimers S1-2：扫描 max_wait_at 已过期的 pending timer。
// 兼容旧数据：payload->>'max_wait_at' 回退 RFC3339 timestamptz 解析。
func (r *SOPTimerRepository) FindMaxWaitOverduePendingTimers(ctx context.Context, now time.Time, limit int) ([]model.SOPTimer, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sop timer repository not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	const maxWaitExpr = `(CASE WHEN payload->>'max_wait_at' ~ '^\d{4}-\d{2}-\d{2}T' THEN (payload->>'max_wait_at')::timestamptz ELSE NULL END)`
	var list []model.SOPTimer
	err := r.db.WithContext(ctx).
		Where("status = ? AND ((max_wait_at IS NOT NULL AND max_wait_at <= ?) OR (max_wait_at IS NULL AND "+maxWaitExpr+" IS NOT NULL AND "+maxWaitExpr+" <= ?))",
			"pending", now, now).
		Limit(limit).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// TransitionPendingStatus 原子把指定 timer 从 pending 转为新状态，返回受影响行数。
//
// 用于死信 / 跳过：WHERE id = ? AND status = 'pending' 限定避免抢占失败时误改。
func (r *SOPTimerRepository) TransitionPendingStatus(ctx context.Context, id uint, newStatus string, firedAt time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop timer repository not initialized")
	}
	res := r.db.WithContext(ctx).Model(&model.SOPTimer{}).
		Where("id = ? AND status = ?", id, "pending").
		Updates(map[string]any{
			"status":   newStatus,
			"fired_at": firedAt,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// BumpClaimCount 累计 claim_count（payload claim_count 同步更新），返回受影响行数
//
// 用于 S1-5：认领失败累计，达到阈值再调用 TransitionPendingStatus 转 dead_letter。
func (r *SOPTimerRepository) BumpClaimCount(ctx context.Context, id uint, claims int, payload model.JSONMap) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop timer repository not initialized")
	}
	updates := map[string]any{
		"claim_count": claims,
		"payload":     payload,
	}
	res := r.db.WithContext(ctx).Model(&model.SOPTimer{}).
		Where("id = ? AND status = ?", id, "pending").
		Updates(updates)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// BumpClaimCountAndDeadLetter 累计 claim_count 并原子转 dead_letter，返回受影响行数
func (r *SOPTimerRepository) BumpClaimCountAndDeadLetter(ctx context.Context, id uint, claims int, payload model.JSONMap, firedAt time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("sop timer repository not initialized")
	}
	updates := map[string]any{
		"status":      "dead_letter",
		"claim_count": claims,
		"payload":     payload,
		"fired_at":    firedAt,
	}
	res := r.db.WithContext(ctx).Model(&model.SOPTimer{}).
		Where("id = ? AND status = ?", id, "pending").
		Updates(updates)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// GetExecutionSummary 按 ID 获取执行记录的 id+sop_id 摘要，用于写 skipped 事件前置校验
func (r *SOPTimerRepository) GetExecutionSummary(ctx context.Context, executionID uint) (*model.SOPExecution, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sop timer repository not initialized")
	}
	var row model.SOPExecution
	if err := r.db.WithContext(ctx).Select("id, sop_id").
		Where("id = ?", executionID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
