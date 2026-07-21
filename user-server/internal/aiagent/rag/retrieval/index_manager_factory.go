package ragretrieval

import (
	"fmt"
	"marketing/internal/pkg/utils/config"

	"gorm.io/gorm"
)

// IndexManagerFactory 索引管理器工厂
type IndexManagerFactory struct{}

// P2-5/6 修复：删除 PGVectorIndexManager 死代码后，工厂统一返回 InMemoryIndexManager
// 历史背景：pgvector 向量存储从未真正落地（无 AutoMigrate、knowledge_chunks.embedding_id 全空），
// RAG 实际使用 BM25-lite 文本匹配（详见 rag_searcher.go 的 fallback 分支）。
// 保留 InMemoryIndexManager 作为接口实现，避免 nil panic；后续若真正落地 pgvector，
// 可在此处重新接入新的 IndexManager 实现。

// NewIndexManagerWithDB 根据配置创建索引管理器（带数据库实例）
// 当前统一返回 InMemoryIndexManager 作为兜底（DB 参数保留供未来扩展使用）
func NewIndexManagerWithDB(db *gorm.DB, cfg config.VectorDatabaseConfig) (IndexManagerInterface, error) {
	if cfg.Type != config.VectorDBTypePGVector {
		return nil, fmt.Errorf("unsupported vector database type: %s", cfg.Type)
	}
	dim := cfg.PGVector.Dimension
	if dim <= 0 {
		dim = 512
	}
	return NewInMemoryIndexManager(dim), nil
}

// NewIndexManager 从配置创建索引管理器（无 DB 注入，使用 InMemory 后备）
// 修复：原 default_implementations.go 调用了不存在的 NewIndexManager
func NewIndexManager(cfg config.VectorDatabaseConfig) (IndexManagerInterface, error) {
	if cfg.Type != config.VectorDBTypePGVector {
		return nil, fmt.Errorf("unsupported vector database type: %s", cfg.Type)
	}
	// InMemory 实现作为兜底（避免无 DB 时的 nil panic）
	return NewInMemoryIndexManager(512), nil
}
