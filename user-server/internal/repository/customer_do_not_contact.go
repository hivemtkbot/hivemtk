package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CustomerDoNotContactRepository 客户全局退订标志位仓库接口
type CustomerDoNotContactRepository interface {
	// Create 写入标志位；幂等：唯一索引(one_id, channel)冲突时跳过，
	// 返回 created=false 表示已存在（不视为错误）
	Create(ctx context.Context, record *model.CustomerDoNotContact) (created bool, err error)
	// ExistsByOneIDAndChannels 判断 one_id 在给定渠道集合中是否存在标志位
	// （调用方传入 [精确渠道, ""] 实现"先查渠道精确再查全局"语义）
	ExistsByOneIDAndChannels(ctx context.Context, oneID string, channels []string) (bool, error)
	// DeleteByOneIDAndChannel 删除标志位（重新订阅）；不存在时幂等返回 nil
	DeleteByOneIDAndChannel(ctx context.Context, oneID, channel string) error
	// ListByOneID 查询 one_id 的全部标志位行
	ListByOneID(ctx context.Context, oneID string) ([]*model.CustomerDoNotContact, error)
}

type customerDoNotContactRepo struct {
	db *gorm.DB
}

// NewCustomerDoNotContactRepository 创建全局退订标志位仓库实例
func NewCustomerDoNotContactRepository(db *gorm.DB) CustomerDoNotContactRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &customerDoNotContactRepo{db: db}
}

// Create 写入标志位（ON CONFLICT DO NOTHING 保证幂等）
func (r *customerDoNotContactRepo) Create(ctx context.Context, record *model.CustomerDoNotContact) (bool, error) {
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(record)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ExistsByOneIDAndChannels one_id 在给定渠道集合中任一命中即返回 true
func (r *customerDoNotContactRepo) ExistsByOneIDAndChannels(ctx context.Context, oneID string, channels []string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.CustomerDoNotContact{}).
		Where("one_id = ? AND channel IN ?", oneID, channels).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteByOneIDAndChannel 删除标志位（channel 为空串即删除全渠道行）
func (r *customerDoNotContactRepo) DeleteByOneIDAndChannel(ctx context.Context, oneID, channel string) error {
	result := r.db.WithContext(ctx).
		Where("one_id = ? AND channel = ?", oneID, channel).
		Delete(&model.CustomerDoNotContact{})
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		return result.Error
	}
	return nil
}

// ListByOneID 查询 one_id 的全部标志位行
func (r *customerDoNotContactRepo) ListByOneID(ctx context.Context, oneID string) ([]*model.CustomerDoNotContact, error) {
	var records []*model.CustomerDoNotContact
	if err := r.db.WithContext(ctx).
		Where("one_id = ?", oneID).
		Order("created_at DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
