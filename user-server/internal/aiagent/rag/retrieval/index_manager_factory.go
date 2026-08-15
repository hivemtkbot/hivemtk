package ragretrieval

import (
	"fmt"
	"hivemtk-user/internal/config"

	"gorm.io/gorm"
)

// IndexManagerFactory 索引管理器工厂
type IndexManagerFactory struct{}


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
func NewIndexManager(cfg config.VectorDatabaseConfig) (IndexManagerInterface, error) {
	if cfg.Type != config.VectorDBTypePGVector {
		return nil, fmt.Errorf("unsupported vector database type: %s", cfg.Type)
	}
	return NewInMemoryIndexManager(512), nil
}

