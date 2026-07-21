package repository

import (
	"context"

	"gorm.io/gorm"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
)

// LowQualitySampleRepository 低质样本仓储（拟人度域 P0-4 管理面读模型）
//
// 写入由 HumanizeLowQualitySampleCollector 负责，本仓储仅提供管理面列表查询，
// 命名按业务域（low_quality_sample），不按管理角色或优先级。
type LowQualitySampleRepository struct {
	db *gorm.DB
}

// NewLowQualitySampleRepository 构造（无参，内部取库句柄）
func NewLowQualitySampleRepository() *LowQualitySampleRepository {
	return &LowQualitySampleRepository{db: db.GetDB()}
}

// List 低质样本分页列表
func (r *LowQualitySampleRepository) List(ctx context.Context, sampleType string, page, pageSize int) ([]model.LowQualitySample, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.LowQualitySample{})
	if sampleType != "" {
		q = q.Where("sample_type = ?", sampleType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.LowQualitySample
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
