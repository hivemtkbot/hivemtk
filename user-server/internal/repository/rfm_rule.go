package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

// RFMRuleRepository RFM 规则仓库
type RFMRuleRepository struct {
	db *gorm.DB
}

// NewRFMRuleRepository 创建 RFM 规则仓库实例
func NewRFMRuleRepository() *RFMRuleRepository {
	return &RFMRuleRepository{
		db: _db.GetDB(),
	}
}

// Create 创建规则
func (r *RFMRuleRepository) Create(ctx context.Context, rule *model.RFMRule) error {
	return r.db.WithContext(ctx).Create(rule).Error
}

// Update 更新规则
func (r *RFMRuleRepository) Update(ctx context.Context, rule *model.RFMRule) error {
	return r.db.WithContext(ctx).Save(rule).Error
}

// Delete 删除规则
func (r *RFMRuleRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.RFMRule{}, id).Error
}

// GetByID 根据 ID 获取规则
func (r *RFMRuleRepository) GetByID(ctx context.Context, id uint) (*model.RFMRule, error) {
	var rule model.RFMRule
	err := r.db.WithContext(ctx).First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetAll 获取所有规则
func (r *RFMRuleRepository) GetAll(ctx context.Context) ([]*model.RFMRule, error) {
	var rules []*model.RFMRule
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&rules).Error
	return rules, err
}

// GetActiveRule 获取活跃规则
func (r *RFMRuleRepository) GetActiveRule(ctx context.Context) (*model.RFMRule, error) {
	var rule model.RFMRule
	err := r.db.WithContext(ctx).Where("is_active = ?", true).First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// UserRFMRepository 用户 RFM 仓库
type UserRFMRepository struct {
	db *gorm.DB
}

// NewUserRFMRepository 创建用户 RFM 仓库实例
func NewUserRFMRepository() *UserRFMRepository {
	return &UserRFMRepository{
		db: _db.GetDB(),
	}
}

// Create 创建用户 RFM 记录
func (r *UserRFMRepository) Create(ctx context.Context, rfm *model.UserRFM) error {
	return r.db.WithContext(ctx).Create(rfm).Error
}

// Update 更新用户 RFM 记录
func (r *UserRFMRepository) Update(ctx context.Context, rfm *model.UserRFM) error {
	return r.db.WithContext(ctx).Save(rfm).Error
}

// GetByUserID 根据用户 ID 获取 RFM
func (r *UserRFMRepository) GetByUserID(ctx context.Context, userID uint) (*model.UserRFM, error) {
	var rfm model.UserRFM
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&rfm).Error
	if err != nil {
		return nil, err
	}
	return &rfm, nil
}

// GetAll 获取所有用户 RFM 列表
func (r *UserRFMRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.UserRFM, int64, error) {
	var rfms []*model.UserRFM
	var total int64

	db := r.db.WithContext(ctx)
	err := db.Model(&model.UserRFM{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = db.
		Order("total_score DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&rfms).Error

	return rfms, total, err
}

// GetByLayer 根据分层获取用户列表
func (r *UserRFMRepository) GetByLayer(ctx context.Context, layer string, page, pageSize int) ([]*model.UserRFM, int64, error) {
	var rfms []*model.UserRFM
	var total int64

	db := r.db.WithContext(ctx)
	err := db.Model(&model.UserRFM{}).Where("layer = ?", layer).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = db.Where("layer = ?", layer).
		Order("total_score DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&rfms).Error

	return rfms, total, err
}

// GetLayerCount 获取各分层用户数量
func (r *UserRFMRepository) GetLayerCount(ctx context.Context) (map[string]int64, error) {
	type LayerCount struct {
		Layer string
		Count int64
	}

	var results []LayerCount
	err := r.db.WithContext(ctx).Model(&model.UserRFM{}).
		Select("layer, COUNT(*) as count").
		Group("layer").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	countMap := make(map[string]int64)
	for _, result := range results {
		countMap[result.Layer] = result.Count
	}

	return countMap, nil
}

// DeleteByUserID 删除用户 RFM 记录
func (r *UserRFMRepository) DeleteByUserID(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.UserRFM{}).Error
}

// BatchUpsert 批量 upsert 用户 RFM 记录
func (r *UserRFMRepository) BatchUpsert(ctx context.Context, rfms []*model.UserRFM) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, rfm := range rfms {
			var existing model.UserRFM
			err := tx.Where("user_id = ?", rfm.UserID).First(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(rfm).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				rfm.ID = existing.ID
				if err := tx.Save(rfm).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// GetNeedUpdateUsers 获取需要更新 RFM 的用户
func (r *UserRFMRepository) GetNeedUpdateUsers(ctx context.Context, days int) ([]*model.UserRFM, error) {
	var rfms []*model.UserRFM
	threshold := time.Now().AddDate(0, 0, -days)

	err := r.db.WithContext(ctx).Where("(updated_at < ? OR updated_at IS NULL)", threshold).
		Find(&rfms).Error

	return rfms, err
}

// NewUserRFMRepositoryWithDB 创建指定数据库连接的用户 RFM 仓库实例（用于测试）
func NewUserRFMRepositoryWithDB(db *gorm.DB) *UserRFMRepository {
	return &UserRFMRepository{db: db}
}

