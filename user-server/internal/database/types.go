package database

// SearchResult 向量搜索结果
type SearchResult struct {
	ID     int64          `json:"id"`
	Score  float64        `json:"score"`
	Fields map[string]any `json:"fields"`
}

// Chunk 文档片段
// P2-5/6 修复：从 pgvector.go 迁移到 types.go（pgvector.go 整体删除）
// vector_store.go 的 VectorStore.Insert 接口依赖此类型
type Chunk struct {
	ID        string
	Content   string
	Embedding []float32
	Metadata  map[string]any
}
