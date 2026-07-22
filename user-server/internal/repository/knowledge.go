package repository

import (
	"context"
	"strings"

	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// KnowledgeRepository 知识库仓库
//
// 从 service/unified_message.go 迁出,五层架构合规:
// service 层不持有 *gorm.DB 也不应内嵌 repository 实现,统一在 repository 包管理。
type KnowledgeRepository struct {
	db *gorm.DB
}

// NewKnowledgeRepository 创建知识库仓库实例(无参,内部用 _db.GetDB())
func NewKnowledgeRepository() *KnowledgeRepository {
	return &KnowledgeRepository{db: _db.GetDB()}
}

// NewKnowledgeRepositoryWithDB 创建知识库仓库实例(显式注入 db)
func NewKnowledgeRepositoryWithDB(db *gorm.DB) *KnowledgeRepository {
	return &KnowledgeRepository{db: db}
}

// Search 全文搜索知识库文章
// 简化版:基于 LIKE 的全文搜索,生产环境应替换为向量检索(FAISS/Milvus 等)
func (r *KnowledgeRepository) Search(ctx context.Context, query string, topK int) ([]*model.KnowledgeHit, error) {
	var hits []*model.KnowledgeHit

	// 私域独立部署:无 merchant_id 字段
	err := r.db.Table("knowledge_articles").
		Select("id, title, content, 1.0 as score, 'article' as source, category_id").
		Where("title LIKE ? OR content LIKE ?", "%"+query+"%", "%"+query+"%").
		Order("score DESC").
		Limit(topK).
		Scan(&hits).Error

	if err != nil {
		// 如果表不存在,返回空结果而不是错误
		if strings.Contains(err.Error(), "doesn't exist") {
			return []*model.KnowledgeHit{}, nil
		}
		return nil, err
	}

	return hits, nil
}
