package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
)

type IntentExampleRepository struct {
	db *gorm.DB
}

// NewIntentExampleRepository 创建意图示例句仓储实例
func NewIntentExampleRepository() *IntentExampleRepository {
	return &IntentExampleRepository{
		db: _db.GetDB(),
	}
}

// SetDB 注入 db（用于测试）
func (r *IntentExampleRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// ListAll 拉取全部示例向量行
func (r *IntentExampleRepository) ListAll(ctx context.Context) ([]model.IntentExample, error) {
	var rows []model.IntentExample
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// Count 统计总行数（索引导入幂等判断用）
func (r *IntentExampleRepository) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.IntentExample{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// UpsertBatch 按 text 幂等批量写入（冲突时更新 intent/vector）
func (r *IntentExampleRepository) UpsertBatch(ctx context.Context, rows []*model.IntentExample) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "text"}},
			DoUpdates: clause.AssignmentColumns([]string{"intent", "vector"}),
		}).
		Create(&rows).Error
}
