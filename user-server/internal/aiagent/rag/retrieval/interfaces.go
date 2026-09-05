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
	Embedding []float32      `json:"-"`
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
	Embedding  []float32      `json:"-"`
	Score      float64        `json:"score"`
	Weight     float64        `json:"weight"`
	TokenCount int            `json:"token_count"`
	ChunkIndex int            `json:"chunk_index"`
}

// SearchResult 检索结果结构
type SearchResult struct {
	DocumentID string         `json:"document_id"`
	Content    string         `json:"content"`
	Title      string         `json:"title"`
	Score      float64        `json:"score"`
	Metadata   map[string]any `json:"metadata"`
	Confidence float64        `json:"confidence"`
	ChunkIndex int            `json:"chunk_index"`
}

// SearchParams 检索参数
type SearchParams struct {
	TopK                int            `json:"top_k"`
	SimilarityThreshold float64        `json:"similarity_threshold"`
	Filters             map[string]any `json:"filters"`
	RelevanceBoost      float64        `json:"relevance_boost"`
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
	IndexDocuments(ctx context.Context, kbID string, documents []Document) error

	Search(ctx context.Context, kbID string, query string, params SearchParams) ([]SearchResult, error)

	DeleteKnowledgeBase(ctx context.Context, kbID string) error

	DeleteDocumentFromKB(ctx context.Context, kbID, docID string) error

	UpdateDocumentInKB(ctx context.Context, kbID, docID string, document Document) error

	GetKnowledgeBaseInfo(ctx context.Context, kbID string) (*KnowledgeBaseInfo, error)

	ListKnowledgeBases(ctx context.Context, ownerID string, includePublic bool) ([]KnowledgeBaseInfo, error)

	CreateKnowledgeBase(ctx context.Context, kbInfo KnowledgeBaseInfo) error
}

// VectorizerInterface 向量化引擎接口
type VectorizerInterface interface {
	EmbedText(text string) ([]float32, error)

	EmbedBatch(texts []string) ([][]float32, error)

	GetDimension() int

	ValidateEmbedding(embedding []float32) bool
}

// IndexManagerInterface 索引管理器接口
type IndexManagerInterface interface {
	BuildIndex(ctx context.Context, kbID string, chunks []Chunk) error

	AddToIndex(ctx context.Context, kbID string, chunk Chunk) error

	RemoveFromIndex(ctx context.Context, kbID, chunkID string) error

	SearchIndex(ctx context.Context, kbID string, queryVec []float32, topK int) ([]Chunk, error)

	DropIndex(ctx context.Context, kbID string) error

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
