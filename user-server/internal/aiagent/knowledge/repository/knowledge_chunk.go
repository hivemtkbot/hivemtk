package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/aiagent/knowledge/model"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// KnowledgeChunkRepository 知识库分段仓储
type KnowledgeChunkRepository struct {
	db *gorm.DB
}

// NewKnowledgeChunkRepository 创建知识库分段仓储
func NewKnowledgeChunkRepository(db *gorm.DB) *KnowledgeChunkRepository {
	return &KnowledgeChunkRepository{db: db}
}

// Create 创建分段
func (r *KnowledgeChunkRepository) Create(ctx context.Context, chunk *model.KnowledgeChunk) error {
	if chunk.Metadata == "" {
		chunk.Metadata = "{}"
	}
	return r.db.WithContext(ctx).Create(chunk).Error
}

// BatchCreate 批量创建分段
func (r *KnowledgeChunkRepository) BatchCreate(ctx context.Context, chunks []model.KnowledgeChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	batchSize := 100
	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		if err := r.db.WithContext(ctx).Create(chunks[i:end]).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetByDocumentID 根据文档 ID 获取分段
func (r *KnowledgeChunkRepository) GetByDocumentID(ctx context.Context, documentID uint64) ([]model.KnowledgeChunk, error) {
	var chunks []model.KnowledgeChunk
	if err := r.db.WithContext(ctx).Where("document_id = ?", documentID).
		Order("chunk_index ASC").Find(&chunks).Error; err != nil {
		return nil, err
	}
	return chunks, nil
}

// PageByDocumentID 分页获取文档分段
func (r *KnowledgeChunkRepository) PageByDocumentID(ctx context.Context, documentID uint64, page, pageSize int) ([]model.KnowledgeChunk, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	var total int64
	q := r.db.WithContext(ctx).Model(&model.KnowledgeChunk{}).Where("document_id = ?", documentID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var chunks []model.KnowledgeChunk
	if err := q.Order("chunk_index ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&chunks).Error; err != nil {
		return nil, 0, err
	}
	return chunks, total, nil
}

// Update 更新分段
//
// embedding_id 总是参与更新；service 层决定是否清空，repository 层如实落盘。
// 这样 service.UpdateChunk 显式设置 chunk.EmbeddingID = "" 触发重新向量化时能正常清空旧向量。
//   - 唯一调用方为 KnowledgeMerchantService.UpdateChunk，行为一致，无副作用。
func (r *KnowledgeChunkRepository) Update(ctx context.Context, chunk *model.KnowledgeChunk) error {
	if chunk.ID == 0 {
		return errors.New("chunk id is required")
	}
	updates := map[string]any{
		"content":      chunk.Content,
		"char_count":   chunk.CharCount,
		"token_count":  chunk.TokenCount,
		"content_hash": chunk.ContentHash,
		"embedding_id": chunk.EmbeddingID,
	}
	return r.db.WithContext(ctx).Model(&model.KnowledgeChunk{}).
		Where("id = ?", chunk.ID).Updates(updates).Error
}

// Delete 根据 ID 删除分段
func (r *KnowledgeChunkRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.KnowledgeChunk{}).Error
}

// DeleteByDocumentID 删除文档的所有分段
func (r *KnowledgeChunkRepository) DeleteByDocumentID(ctx context.Context, documentID uint64) error {
	return r.db.WithContext(ctx).Where("document_id = ?", documentID).
		Delete(&model.KnowledgeChunk{}).Error
}

// DeleteByProductID 删除产品的所有分段
func (r *KnowledgeChunkRepository) DeleteByProductID(ctx context.Context, productID string) error {
	return r.db.WithContext(ctx).Where("product_id = ?", productID).
		Delete(&model.KnowledgeChunk{}).Error
}

// CountByProductID 统计产品分段数
func (r *KnowledgeChunkRepository) CountByProductID(ctx context.Context, productID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.KnowledgeChunk{}).
		Where("product_id = ?", productID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *KnowledgeChunkRepository) CountByMerchant(ctx context.Context) (int64, error) {
	var count int64
	q := r.db.WithContext(ctx).Model(&model.KnowledgeChunk{})
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// FindByContentHash 根据内容哈希查重
func (r *KnowledgeChunkRepository) FindByContentHash(ctx context.Context, productID string, hash string) (*model.KnowledgeChunk, error) {
	var chunk model.KnowledgeChunk
	q := r.db.WithContext(ctx).Where("content_hash = ?", hash)
	if productID != "" {
		q = q.Where("product_id = ?", productID)
	}
	if err := q.First(&chunk).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &chunk, nil
}

// UpdateScore 更新最近检索分数
func (r *KnowledgeChunkRepository) UpdateScore(ctx context.Context, id uint64, score float64) error {
	return r.db.WithContext(ctx).Model(&model.KnowledgeChunk{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"similarity_score": score,
			"hit_count":        gorm.Expr("hit_count + 1"),
		}).Error
}

// IncrementHitCount 批量累加命中分段 hit_count（tooluse 调用，替代直接 DB 访问）
//
// 五层架构合规：L4 能力层（tooluse）通过 L3 仓储层间接操作 DB
func (r *KnowledgeChunkRepository) IncrementHitCount(ctx context.Context, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.KnowledgeChunk{}).
		Where("id IN ?", ids).
		UpdateColumn("hit_count", gorm.Expr("hit_count + 1")).Error
}

// GetByID 根据 ID 获取分段
func (r *KnowledgeChunkRepository) GetByID(ctx context.Context, id uint64) (*model.KnowledgeChunk, error) {
	var chunk model.KnowledgeChunk
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&chunk).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("chunk not found")
		}
		return nil, err
	}
	return &chunk, nil
}

// BatchUpdateLastIndexed 批量更新最后索引时间
func (r *KnowledgeChunkRepository) BatchUpdateLastIndexed(ctx context.Context, documentID uint64) error {
	return r.db.WithContext(ctx).Model(&model.KnowledgeChunk{}).
		Where("document_id = ?", documentID).
		Update("created_at", time.Now()).Error
}

// UpdateEmbeddingsBatch 事务式批量更新分段 embedding
//
// 要求 chunks 与 embeddings 长度一致；使用参数化 SQL 防止注入；事务保证原子性。
func (r *KnowledgeChunkRepository) UpdateEmbeddingsBatch(ctx context.Context, chunks []model.KnowledgeChunk, embeddings [][]float32) error {
	return r.UpdateEmbeddingsBatchWithSource(ctx, chunks, embeddings, "tei")
}

// UpdateEmbeddingsBatchWithSource D16：更新 embedding 并显式标记来源（"tei"/"hash"）。
// 兜底向量（hash）必须与真实向量（tei）可区分——读路径按 source='tei' 过滤防混写。
func (r *KnowledgeChunkRepository) UpdateEmbeddingsBatchWithSource(ctx context.Context, chunks []model.KnowledgeChunk, embeddings [][]float32, source string) error {
	if len(chunks) != len(embeddings) {
		return errors.New("chunks 与 embeddings 长度不一致")
	}
	if len(chunks) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, c := range chunks {
			vec := vecToPGString(embeddings[i])
			if err := tx.Exec(
				"UPDATE knowledge_chunks SET embedding = $1::vector, embed_status = 'indexed', embedding_source = $3 WHERE id = $2",
				vec, c.ID, source,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// vecToPGString 把 []float32 序列化为 pgvector 字面量字符串
//
// pgvector 支持的格式: '[1.0,2.0,3.0,...]'
// 必须用科学计数或保留小数位，否则 PG 会报 dimension mismatch
func vecToPGString(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var b []byte
	b = append(b, '[')
	for i, f := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendFloat(b, float64(f), 'g', -1, 32)
	}
	b = append(b, ']')
	return string(b)
}

