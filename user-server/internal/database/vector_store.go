package database

import "context"

// VectorStore 向量存储接口
type VectorStore interface {
	// CreateCollection 创建集合
	CreateCollection(collectionName string, dimension int) error

	// Insert 插入数据
	Insert(ctx context.Context, productID string, chunks []Chunk) error

	// Search 搜索向量
	Search(ctx context.Context, productID string, embedding []float32, topK int) ([]SearchResult, error)

	// Delete 删除数据
	Delete(ctx context.Context, productID string, chunkIDs []string) error

	// HasCollection 检查集合是否存在
	HasCollection(collectionName string) (bool, error)

	// DropCollection 删除集合
	DropCollection(collectionName string) error

	// GetCollectionStatistics 获取集合统计信息
	GetCollectionStatistics(collectionName string) (map[string]any, error)

	// CreateIndex 创建索引
	CreateIndex(collectionName string) error
}
