package migrations

import (
	"context"
	"fmt"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// KnowledgeVectorMigration RAG 向量存储迁移
//
// 背景:
//   - pgvector 扩展已安装(v0.8.5)
//   - knowledge_chunks 表存在但只有 embedding_id(string)和 similarity_score(numeric),
//     没有真正的 vector 列 → RAG 检索只能走 BM25-lite 文本匹配,无法发挥向量召回效果
//   - rag_products.vector_table 字段虽然声明了"每产品一张向量表",但实际从未创建过这些表
//
// 本迁移:
//  1. 给 knowledge_chunks 增加 embedding vector(1024) 列(与 TEI bge-m3 维度一致)
//  2. 给该列加 HNSW 索引(vector_cosine_ops),支持高效 ANN 检索
//  3. 幂等:列/索引已存在时不报错,可重入
//  4. 不删除 rag_products.vector_table 字段(架构文档约束),实际召回使用 knowledge_chunks.embedding
type KnowledgeVectorMigration struct {
	db *gorm.DB
}

// NewKnowledgeVectorMigration 创建 RAG 向量迁移
func NewKnowledgeVectorMigration(db *gorm.DB) *KnowledgeVectorMigration {
	return &KnowledgeVectorMigration{db: db}
}

// Version 返回版本号
func (m *KnowledgeVectorMigration) Version() string {
	return "v2.6.0"
}

// Name 返回迁移名称
func (m *KnowledgeVectorMigration) Name() string {
	return "RAG 向量存储(pgvector)"
}

// Description 返回迁移描述
func (m *KnowledgeVectorMigration) Description() string {
	return "为 knowledge_chunks 表增加 embedding vector(1024) 列与 HNSW 索引,使 RAG 检索走真正的余弦相似度而非 BM25-lite"
}

// Up 执行升级
func (m *KnowledgeVectorMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 1) 确认 pgvector 扩展可用
	var extCount int64
	if err := m.db.Raw(`SELECT COUNT(*) FROM pg_extension WHERE extname = 'vector'`).Scan(&extCount).Error; err != nil {
		return fmt.Errorf("查询 pgvector 扩展失败: %w", err)
	}
	if extCount == 0 {
		// 尝试创建
		if err := m.db.Exec(`CREATE EXTENSION IF NOT EXISTS vector`).Error; err != nil {
			return fmt.Errorf("pgvector 扩展未安装且创建失败: %w(请先在数据库中安装 pgvector)", err)
		}
	}

	// 2) 给 knowledge_chunks 增加 embedding 向量列(幂等)
	// 1024 = TEI bge-m3 实际输出维度
	addColumnSQL := `ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS embedding vector(1024)`
	if err := m.db.Exec(addColumnSQL).Error; err != nil {
		return fmt.Errorf("添加 embedding 列失败: %w", err)
	}

	// 3) 加 HNSW 索引(vector_cosine_ops 余弦距离)
	//    HNSW 相比 IVFFlat:无训练阶段,数据量小(<100k)时召回更准;私域部署场景最合适
	createIndexSQL := `CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_embedding
		ON knowledge_chunks
		USING hnsw (embedding vector_cosine_ops)`
	if err := m.db.Exec(createIndexSQL).Error; err != nil {
		return fmt.Errorf("创建 HNSW 索引失败: %w", err)
	}

	// 4) 同时给 product_id 加索引(已经在 migration 中有 idx_knowledge_chunks_product_id,这里仅做一次存在性确认)
	//    跳过重复创建,避免 IF NOT EXISTS 二次报错

	return nil
}

// Down 执行降级
func (m *KnowledgeVectorMigration) Down(ctx context.Context) error {
	// 删除索引
	if err := m.db.Exec(`DROP INDEX IF EXISTS idx_knowledge_chunks_embedding`).Error; err != nil {
		return err
	}
	// 删除列
	if err := m.db.Exec(`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS embedding`).Error; err != nil {
		return err
	}
	return nil
}

var _ migration.Migration = (*KnowledgeVectorMigration)(nil)
