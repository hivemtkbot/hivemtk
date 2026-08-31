package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
)

// PersonaLowQualitySampleRepository 拟人度低质样本仓储接口
//
// 仅负责持久化，业务逻辑（序列化 DimensionScores / CandidateReplies、判定 sampleType）
// 由 service 层的 DBLowQualitySampleCollector 完成。
type PersonaLowQualitySampleRepository interface {
	Create(ctx context.Context, sample *model.LowQualitySample) error
}

// personaLowQualitySampleRepository 实现 PersonaLowQualitySampleRepository
type personaLowQualitySampleRepository struct {
	db *gorm.DB
}

// NewPersonaLowQualitySampleRepository 构造（无参，内部取库句柄）
func NewPersonaLowQualitySampleRepository() PersonaLowQualitySampleRepository {
	return &personaLowQualitySampleRepository{db: _db.GetDB()}
}

// NewPersonaLowQualitySampleRepositoryWithDB 创建指定数据库连接的 PersonaLowQualitySampleRepository 实例（用于测试）
func NewPersonaLowQualitySampleRepositoryWithDB(db *gorm.DB) PersonaLowQualitySampleRepository {
	return &personaLowQualitySampleRepository{db: db}
}

// Create 持久化低质样本到 low_quality_samples 表
// 与原 service.DBLowQualitySampleCollector.Collect 行为一致：
//   - sample 为 nil 时直接返回 nil（noop）
//   - 错误信息包裹 "save low quality sample: %w"
func (r *personaLowQualitySampleRepository) Create(ctx context.Context, sample *model.LowQualitySample) error {
	if sample == nil {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(sample).Error; err != nil {
		return fmt.Errorf("save low quality sample: %w", err)
	}
	return nil
}
