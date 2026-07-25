package repository

// rag_health_repository.go RAG 健康度仓储（C 域 P1 缺口 #4）
//
// 五层架构归属: L3 Repository 层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6.4 RAG 健康度
//
// 职责:
//   - 统计知识库 chunk 总数（用于健康度评分的"知识库覆盖"维度）
//   - 其它健康度相关查询由 RagMetrics / RagAlert 子服务直接走各自仓储

import (
	"context"

	"gorm.io/gorm"

	kbmodel "marketing/internal/aiagent/knowledge/model"
)

// RagHealthRepository RAG 健康度仓储接口
type RagHealthRepository interface {
	// CountKnowledgeChunks 统计知识库 chunk 总数
	CountKnowledgeChunks(ctx context.Context) (int64, error)
}

type ragHealthRepo struct {
	db *gorm.DB
}

// NewRagHealthRepository 创建 RAG 健康度仓储
func NewRagHealthRepository(db *gorm.DB) RagHealthRepository {
	return &ragHealthRepo{db: db}
}

// CountKnowledgeChunks 统计知识库 chunk 总数
func (r *ragHealthRepo) CountKnowledgeChunks(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&kbmodel.KnowledgeChunk{}).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// 编译期断言
var _ RagHealthRepository = (*ragHealthRepo)(nil)
