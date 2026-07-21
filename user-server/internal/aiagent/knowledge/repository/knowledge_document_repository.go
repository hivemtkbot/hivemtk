package repository

import (
	"context"
	"errors"
	"marketing/internal/aiagent/knowledge/model"
	"time"

	"gorm.io/gorm"
)

// KnowledgeDocumentRepository 知识库文档仓储(产品维度)
type KnowledgeDocumentRepository struct {
	db *gorm.DB
}

// NewKnowledgeDocumentRepository 创建知识库文档仓储
func NewKnowledgeDocumentRepository(db *gorm.DB) *KnowledgeDocumentRepository {
	return &KnowledgeDocumentRepository{db: db}
}

// Create 创建文档
func (r *KnowledgeDocumentRepository) Create(ctx context.Context, doc *model.KnowledgeDocument) error {
	if doc.SourceType == "" {
		doc.SourceType = model.SourceTypeUpload
	}
	if doc.EmbedStatus == "" {
		doc.EmbedStatus = model.EmbedStatusPending
	}
	if doc.Status == 0 {
		doc.Status = 1
	}
	if doc.Tags == "" {
		doc.Tags = "[]"
	}
	if doc.Metadata == "" {
		doc.Metadata = "{}"
	}
	return r.db.WithContext(ctx).Create(doc).Error
}

// GetByID 根据 ID 获取文档
func (r *KnowledgeDocumentRepository) GetByID(ctx context.Context, id uint64) (*model.KnowledgeDocument, error) {
	var doc model.KnowledgeDocument
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&doc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("knowledge document not found")
		}
		return nil, err
	}
	return &doc, nil
}

// GetByProductAndID 根据产品 ID + 文档 ID 获取(独立部署)
//
// 2026-07-18 修复：KnowledgeDocument.ProductID 是 int64（迁移 schema 为 INTEGER），
// 但前端传入的 RagProduct.ID 是 string UUID。需要通过 HashStringToInt64 把 UUID
// 映射回 int64 才能命中记录。productID=0 表示不按 product 过滤。
func (r *KnowledgeDocumentRepository) GetByProductAndID(ctx context.Context, productID, id int64) (*model.KnowledgeDocument, error) {
	var doc model.KnowledgeDocument
	q := r.db.WithContext(ctx).Where("id = ?", id)
	if productID > 0 {
		q = q.Where("product_id = ?", productID)
	}
	if err := q.First(&doc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("knowledge document not found")
		}
		return nil, err
	}
	return &doc, nil
}

// ListFilter 文档列表筛选
//
// 2026-07-18 修复：KnowledgeDocument.ProductID 是 int64（迁移 schema 为 INTEGER），
// 但前端传入的 RagProduct.ID 是 string UUID。调用方用 HashStringToInt64 把 UUID
// 映射回 int64 后再传入。
type ListFilter struct {
	ProductID   int64
	EmbedStatus string
	SourceType  string
	Category    string
	Keyword     string
	Page        int
	PageSize    int
}

// List 列出文档
func (r *KnowledgeDocumentRepository) List(ctx context.Context, filter ListFilter) ([]*model.KnowledgeDocument, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.KnowledgeDocument{})
	if filter.ProductID > 0 {
		q = q.Where("product_id = ?", filter.ProductID)
	}
	if filter.EmbedStatus != "" {
		q = q.Where("embed_status = ?", filter.EmbedStatus)
	}
	if filter.SourceType != "" {
		q = q.Where("source_type = ?", filter.SourceType)
	}
	if filter.Category != "" {
		q = q.Where("category = ?", filter.Category)
	}
	if filter.Keyword != "" {
		q = q.Where("title LIKE ?", "%"+filter.Keyword+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var docs []*model.KnowledgeDocument
	offset := (filter.Page - 1) * filter.PageSize
	if err := q.Order("created_at DESC").Offset(offset).Limit(filter.PageSize).Find(&docs).Error; err != nil {
		return nil, 0, err
	}
	return docs, total, nil
}

// Update 更新文档
func (r *KnowledgeDocumentRepository) Update(ctx context.Context, doc *model.KnowledgeDocument) error {
	doc.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(doc).Error
}

// UpdateStatus 更新文档状态
func (r *KnowledgeDocumentRepository) UpdateStatus(ctx context.Context, id uint64, status model.EmbedStatus, progress int, errMsg string) error {
	updates := map[string]any{
		"embed_status":   status,
		"embed_progress": progress,
		"error_msg":      errMsg,
		"updated_at":     time.Now(),
	}
	if status == model.EmbedStatusIndexed {
		now := time.Now()
		updates["last_index_at"] = now
	}
	return r.db.WithContext(ctx).Model(&model.KnowledgeDocument{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateChunkStats 更新分段统计
func (r *KnowledgeDocumentRepository) UpdateChunkStats(ctx context.Context, id uint64, chunkCount, totalTokens int) error {
	return r.db.WithContext(ctx).Model(&model.KnowledgeDocument{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"chunk_count":  chunkCount,
			"total_tokens": totalTokens,
			"updated_at":   time.Now(),
		}).Error
}

// Delete 删除文档
func (r *KnowledgeDocumentRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.KnowledgeDocument{}).Error
}

// DeleteByProduct 根据产品删除所有文档
func (r *KnowledgeDocumentRepository) DeleteByProduct(ctx context.Context, productID int64) error {
	return r.db.WithContext(ctx).Model(&model.KnowledgeDocument{}).Where("product_id = ?", productID).
		Delete(&model.KnowledgeDocument{}).Error
}

// CountByProduct 统计产品文档数
func (r *KnowledgeDocumentRepository) CountByProduct(ctx context.Context, productID int64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.KnowledgeDocument{}).Where("product_id = ?", productID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *KnowledgeDocumentRepository) CountByMerchant(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.KnowledgeDocument{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountTodayImports 今日导入数（独立部署下统计全量）
func (r *KnowledgeDocumentRepository) CountTodayImports(ctx context.Context) (int64, error) {
	var count int64
	todayStart := time.Now().Format("2006-01-02")
	if err := r.db.WithContext(ctx).Model(&model.KnowledgeDocument{}).
		Where("created_at >= ?", todayStart).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
