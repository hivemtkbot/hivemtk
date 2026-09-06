package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	contentmodel "hivemtk-user/internal/content/model"
	_db "hivemtk-user/internal/pkg/db"
)

// BatchOperationRepository 批量操作历史仓储
type BatchOperationRepository struct {
	db *gorm.DB
}

// NewBatchOperationRepository 创建批量操作历史仓储实例
func NewBatchOperationRepository() *BatchOperationRepository {
	return &BatchOperationRepository{db: _db.GetDB()}
}

// ListHistories 分页查询批量操作历史（按 created_at DESC）
func (r *BatchOperationRepository) ListHistories(ctx context.Context, page, pageSize int) ([]*contentmodel.BatchOperationHistory, int64, error) {
	var list []*contentmodel.BatchOperationHistory
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&contentmodel.BatchOperationHistory{}).
		Where("1 = 1").
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	err := r.db.WithContext(ctx).
		Model(&contentmodel.BatchOperationHistory{}).
		Where("1 = 1").
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&list).Error
	return list, total, err
}

// GetHistoryByID 根据 ID 获取批量操作历史
func (r *BatchOperationRepository) GetHistoryByID(ctx context.Context, id uint) (*contentmodel.BatchOperationHistory, error) {
	var history contentmodel.BatchOperationHistory
	err := r.db.WithContext(ctx).First(&history, id).Error
	return &history, err
}

// ErrBatchOperationNotCancellable 操作记录不存在或状态不可取消
var ErrBatchOperationNotCancellable = errors.New("操作记录不存在或状态不可取消")

// CancelHistory 取消批量操作（仅 pending / running 状态可取消）
//
// 返回 (nil) 表示成功；返回 ErrBatchOperationNotCancellable 表示记录不存在或状态不可取消。
func (r *BatchOperationRepository) CancelHistory(ctx context.Context, id uint) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&contentmodel.BatchOperationHistory{}).
		Where("id = ? AND status IN ?", id, []string{"pending", "running"}).
		Updates(map[string]any{
			"status":      "cancelled",
			"finished_at": now,
			"updated_at":  now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrBatchOperationNotCancellable
	}
	return nil
}
