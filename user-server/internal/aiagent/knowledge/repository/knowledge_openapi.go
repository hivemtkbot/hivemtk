package repository

import (
	"context"
	"errors"
	"marketing/internal/aiagent/knowledge/model"
	"time"

	"gorm.io/gorm"
)

// KnowledgeOpenAPIRepository OpenAPI 数据源仓储
type KnowledgeOpenAPIRepository struct {
	db *gorm.DB
}

// NewKnowledgeOpenAPIRepository 创建 OpenAPI 仓储
func NewKnowledgeOpenAPIRepository(db *gorm.DB) *KnowledgeOpenAPIRepository {
	return &KnowledgeOpenAPIRepository{db: db}
}

// Create 创建数据源
func (r *KnowledgeOpenAPIRepository) Create(ctx context.Context, src *model.KnowledgeOpenAPISource) error {
	if src.LastStatus == "" {
		src.LastStatus = "never"
	}
	if src.AuthConfig == "" {
		src.AuthConfig = "{}"
	}
	if src.FieldMapping == "" {
		src.FieldMapping = "{}"
	}
	if src.Method == "" {
		src.Method = "GET"
	}
	if src.Type == "" {
		src.Type = "rest"
	}
	if src.AuthType == "" {
		src.AuthType = "none"
	}
	if src.Enabled == 0 {
		src.Enabled = 1
	}
	return r.db.WithContext(ctx).Create(src).Error
}

// GetByID 根据 ID 获取
func (r *KnowledgeOpenAPIRepository) GetByID(ctx context.Context, id uint64) (*model.KnowledgeOpenAPISource, error) {
	var src model.KnowledgeOpenAPISource
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&src).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("openapi source not found")
		}
		return nil, err
	}
	return &src, nil
}

// GetByProductAndID 根据产品和 ID 获取
func (r *KnowledgeOpenAPIRepository) GetByProductAndID(ctx context.Context, productID string, id int64) (*model.KnowledgeOpenAPISource, error) {
	var src model.KnowledgeOpenAPISource
	q := r.db.WithContext(ctx).Where("id = ?", id)
	if productID != "" {
		q = q.Where("product_id = ?", productID)
	}
	if err := q.First(&src).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("openapi source not found")
		}
		return nil, err
	}
	return &src, nil
}

// List 列出数据源
func (r *KnowledgeOpenAPIRepository) List(ctx context.Context, productID string) ([]model.KnowledgeOpenAPISource, error) {
	var sources []model.KnowledgeOpenAPISource
	q := r.db.WithContext(ctx).Model(&model.KnowledgeOpenAPISource{})
	if productID != "" {
		q = q.Where("product_id = ?", productID)
	}
	if err := q.Order("created_at DESC").Find(&sources).Error; err != nil {
		return nil, err
	}
	return sources, nil
}

// Update 更新
func (r *KnowledgeOpenAPIRepository) Update(ctx context.Context, src *model.KnowledgeOpenAPISource) error {
	// 兼容 jsonb 字段:空串需转为 "{}"
	if src.AuthConfig == "" {
		src.AuthConfig = "{}"
	}
	if src.FieldMapping == "" {
		src.FieldMapping = "{}"
	}
	src.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(src).Error
}

// UpdateSyncStatus 更新同步状态
func (r *KnowledgeOpenAPIRepository) UpdateSyncStatus(ctx context.Context, id uint64, status, errMsg string, totalSynced int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.KnowledgeOpenAPISource{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_sync_at": now,
			"last_status":  status,
			"last_error":   errMsg,
			"total_synced": gorm.Expr("total_synced + ?", totalSynced),
			"updated_at":   now,
		}).Error
}

// Delete 删除
func (r *KnowledgeOpenAPIRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.KnowledgeOpenAPISource{}).Error
}

// ListEnabled 列出启用的数据源
func (r *KnowledgeOpenAPIRepository) ListEnabled(ctx context.Context) ([]model.KnowledgeOpenAPISource, error) {
	var sources []model.KnowledgeOpenAPISource
	if err := r.db.WithContext(ctx).Where("enabled = ?", 1).Find(&sources).Error; err != nil {
		return nil, err
	}
	return sources, nil
}
