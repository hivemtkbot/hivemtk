package ragretrieval

import (
	"context"
	"time"
)

// Document 表示知识库中的文档
type Document struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata"`
	Embedding []float32      `json:"-"` // 嵌入向量，不序列化到JSON
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Chunk 表示文档分片
type Chunk struct {
	ID         string         `json:"id"`
	DocumentID string         `json:"document_id"`
	Content    string         `json:"content"`
	Title      string         `json:"title"`
	Metadata   map[string]any `json:"metadata"`
	Embedding  []float32      `json:"-"`     // 嵌入向量
	Score      float64        `json:"score"` // 相似度分数
	Weight     float64        `json:"weight"` // 自学习权重（默认 0，由 RagSearcher 回填）
	TokenCount int            `json:"token_count"`
	ChunkIndex int            `json:"chunk_index"` // 在原文档中的分片索引
}

// SearchResult 检索结果结构
type SearchResult struct {
	DocumentID string         `json:"document_id"`
	Content    string         `json:"content"`
	Title      string         `json:"title"`
	Score      float64        `json:"score"` // 相似度分数
	Metadata   map[string]any `json:"metadata"`
	Confidence float64        `json:"confidence"`  // 置信度
	ChunkIndex int            `json:"chunk_index"` // 在原文档中的分片索引
}

// SearchParams 检索参数
type SearchParams struct {
	TopK                int            `json:"top_k"`                // 返回结果数量
	SimilarityThreshold float64        `json:"similarity_threshold"` // 相似度阈值
	Filters             map[string]any `json:"filters"`              // 过滤条件
	RelevanceBoost      float64        `json:"relevance_boost"`      // 相关性增强因子
}

// KnowledgeBaseInfo 知识库信息
type KnowledgeBaseInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	IsPublic    bool      `json:"is_public"`
	DocCount    int       `json:"doc_count"`
	TotalChunks int       `json:"total_chunks"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RagRetrievalService RAG检索服务接口
type RagRetrievalService interface {
	// IndexDocuments 向知识库中添加文档
	IndexDocuments(ctx context.Context, kbID string, documents []Document) error

	// Search 在知识库中检索相关信息
	Search(ctx context.Context, kbID string, query string, params SearchParams) ([]SearchResult, error)

	// DeleteKnowledgeBase 删除整个知识库
	DeleteKnowledgeBase(ctx context.Context, kbID string) error

	// DeleteDocumentFromKB 从知识库中删除特定文档
	DeleteDocumentFromKB(ctx context.Context, kbID, docID string) error

	// UpdateDocumentInKB 更新知识库中的文档
	UpdateDocumentInKB(ctx context.Context, kbID, docID string, document Document) error

	// GetKnowledgeBaseInfo 获取知识库信息
	GetKnowledgeBaseInfo(ctx context.Context, kbID string) (*KnowledgeBaseInfo, error)

	// ListKnowledgeBases 列出所有知识库
	ListKnowledgeBases(ctx context.Context, ownerID string, includePublic bool) ([]KnowledgeBaseInfo, error)

	// CreateKnowledgeBase 创建知识库
	CreateKnowledgeBase(ctx context.Context, kbInfo KnowledgeBaseInfo) error
}

// VectorizerInterface 向量化引擎接口
type VectorizerInterface interface {
	// EmbedText 将文本转换为向量
	EmbedText(text string) ([]float32, error)

	// EmbedBatch 批量向量化
	EmbedBatch(texts []string) ([][]float32, error)

	// GetDimension 获取向量维度
	GetDimension() int

	// ValidateEmbedding 验证向量有效性
	ValidateEmbedding(embedding []float32) bool
}

// IndexManagerInterface 索引管理器接口
type IndexManagerInterface interface {
	// BuildIndex 构建索引
	BuildIndex(ctx context.Context, kbID string, chunks []Chunk) error

	// AddToIndex 向索引中添加向量
	AddToIndex(ctx context.Context, kbID string, chunk Chunk) error

	// RemoveFromIndex 从索引中删除向量
	RemoveFromIndex(ctx context.Context, kbID, chunkID string) error

	// SearchIndex 在索引中检索
	SearchIndex(ctx context.Context, kbID string, queryVec []float32, topK int) ([]Chunk, error)

	// DropIndex 删除索引
	DropIndex(ctx context.Context, kbID string) error

	// GetIndexStats 获取索引统计信息
	GetIndexStats(ctx context.Context, kbID string) (*IndexStats, error)
}

// IndexStats 索引统计信息
type IndexStats struct {
	KbID        string    `json:"kb_id"`
	VectorCount int       `json:"vector_count"`
	Dimension   int       `json:"dimension"`
	MemoryUsage int64     `json:"memory_usage"`
	LastUpdated time.Time `json:"last_updated"`
}
