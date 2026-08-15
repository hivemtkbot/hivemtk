package rag_core

import (
	"context"
	"errors"
	"fmt"
	"hivemtk-user/internal/aiagent/llm"
	"math"
	"strings"
	"time"
)

// Document 表示知识库中的文档
type Document struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata"`
	Embedding []float32      `json:"-"` 
	CreatedAt time.Time      `json:"created_at"`
}

// Chunk 表示文档分片
type Chunk struct {
	ID         string         `json:"id"`
	DocumentID string         `json:"document_id"`
	Content    string         `json:"content"`
	Metadata   map[string]any `json:"metadata"`
	Embedding  []float32      `json:"-"`     
	Score      float64        `json:"score"` 
	TokenCount int            `json:"token_count"`
}

// RAGConfig RAG引擎配置
type RAGConfig struct {
	ChunkSize           int     `json:"chunk_size"`             
	ChunkOverlap        int     `json:"chunk_overlap"`          
	MaxChunksToRetrieve int     `json:"max_chunks_to_retrieve"` 
	SimilarityThreshold float64 `json:"similarity_threshold"`   
	VectorDimension     int     `json:"vector_dimension"`       
}

// RAGEngineInterface RAG引擎接口
type RAGEngineInterface interface {
	AddDocuments(ctx context.Context, docs []Document) error
	DeleteDocument(ctx context.Context, docID string) error
	Search(ctx context.Context, query string, topK int) ([]Chunk, error)
	GetDocument(ctx context.Context, docID string) (*Document, error)
	UpdateConfig(config *RAGConfig) error
	GetConfig() *RAGConfig
}

// RAGEngine RAG引擎实现
type RAGEngine struct {
	config    *RAGConfig
	documents map[string]*Document
	chunks    []*Chunk
	embedder  EmbedderInterface
}

// EmbedderInterface 嵌入器接口
type EmbedderInterface interface {
	EmbedText(text string) ([]float32, error)
	EmbedQuery(query string) ([]float32, error)
	GetDimension() int
}

// RemoteEmbedder 真实 Embedding 适配器(对接 OpenAI 兼容 /v1/embeddings)
type RemoteEmbedder struct {
	dimension int
	svc       llm.EmbeddingServiceInterface
	cfg       *llm.EmbeddingConfig
}

// NewRemoteEmbedder 构造真实 Embedder
func NewRemoteEmbedder(dimension int) *RemoteEmbedder {
	svc := llm.NewEmbeddingService()
	cfg := svc.DefaultConfig()
	if dimension <= 0 {
		dimension = cfg.Dimension
	}
	cfg.Dimension = dimension
	return &RemoteEmbedder{
		dimension: dimension,
		svc:       svc,
		cfg:       cfg,
	}
}

// EmbedText 真实 Embedding
func (r *RemoteEmbedder) EmbedText(text string) ([]float32, error) {
	if text == "" {
		return make([]float32, r.dimension), nil
	}
	vec, err := r.svc.EmbedOne(context.Background(), r.cfg, text)
	if err != nil {
		return nil, err
	}
	return vec, nil
}

// EmbedQuery 真实 Embedding(查询)
func (r *RemoteEmbedder) EmbedQuery(query string) ([]float32, error) {
	return r.EmbedText(query)
}

// GetDimension 获取维度
func (r *RemoteEmbedder) GetDimension() int { return r.dimension }

// MockEmbedder 纯内存测试 Embedder：基于 FNV-1a 哈希生成确定性伪向量。
// 不发起任何网络请求，用于单元测试隔离 embedding 服务依赖。
// 同一输入文本 → 同一向量，保证 AddDocuments / Search 的可比性。
type MockEmbedder struct {
	dimension int
}

// NewMockEmbedder 构造测试 Embedder
func NewMockEmbedder(dimension int) *MockEmbedder {
	if dimension <= 0 {
		dimension = 768
	}
	return &MockEmbedder{dimension: dimension}
}

// hashToFloat32Vector FNV-1a 派生 dim 维伪向量（与 vector 包 hashToVector 算法一致）
func hashToFloat32Vector(text string, dim int) []float32 {
	vec := make([]float32, dim)
	const (
		offset uint64 = 14695981039346656037
		prime  uint64 = 1099511628211
	)
	h := offset
	for i := 0; i < len(text); i++ {
		h ^= uint64(text[i])
		h *= prime
	}
	state := h
	for i := 0; i < dim; i++ {
		state = state*6364136223846793005 + 1442695040888963407
		vec[i] = float32((state>>11)&0xFFFF)/32768.0 - 1.0
	}
	return vec
}

// EmbedText 测试用 Embedding
func (m *MockEmbedder) EmbedText(text string) ([]float32, error) {
	if text == "" {
		return make([]float32, m.dimension), nil
	}
	return hashToFloat32Vector(text, m.dimension), nil
}

// EmbedQuery 测试用 Embedding
func (m *MockEmbedder) EmbedQuery(query string) ([]float32, error) {
	return m.EmbedText(query)
}

// GetDimension 获取维度
func (m *MockEmbedder) GetDimension() int { return m.dimension }

// NewRAGEngine 创建新的RAG引擎
// NewRAGEngine 创建默认 RAG 引擎（dimension 等元信息会由调用方在
// NewRAGEngineWithConfig / NewRAGEngineWithEmbedder 中按真实 embedding 服务注入）
func NewRAGEngine(config *RAGConfig) *RAGEngine {
	if config == nil {
		config = &RAGConfig{
			ChunkSize:           512,
			ChunkOverlap:        50,
			MaxChunksToRetrieve: 5,
			SimilarityThreshold: 0.5,
			VectorDimension: 1024,
		}
	}

	return &RAGEngine{
		config:    config,
		documents: make(map[string]*Document),
		chunks:    make([]*Chunk, 0),
		embedder:  NewRemoteEmbedder(config.VectorDimension),
	}
}

// NewRAGEngineWithEmbedder 创建使用指定 Embedder 的 RAG 引擎（用于测试注入 MockEmbedder）
func NewRAGEngineWithEmbedder(config *RAGConfig, embedder EmbedderInterface) *RAGEngine {
	if config == nil {
		config = &RAGConfig{
			ChunkSize:           512,
			ChunkOverlap:        50,
			MaxChunksToRetrieve: 5,
			SimilarityThreshold: 0.5,
			VectorDimension:     1024,
		}
	}
	if embedder == nil {
		embedder = NewRemoteEmbedder(config.VectorDimension)
	}
	return &RAGEngine{
		config:    config,
		documents: make(map[string]*Document),
		chunks:    make([]*Chunk, 0),
		embedder:  embedder,
	}
}

// AddDocuments 添加文档到知识库
func (r *RAGEngine) AddDocuments(ctx context.Context, docs []Document) error {
	for _, doc := range docs {
		chunks := r.splitDocument(doc)

		for _, chunk := range chunks {
			embedding, err := r.embedder.EmbedText(chunk.Content)
			if err != nil {
				return fmt.Errorf("failed to embed chunk: %w", err)
			}
			chunk.Embedding = embedding
		}

		r.documents[doc.ID] = &doc
		r.chunks = append(r.chunks, chunks...)
	}

	return nil
}

// splitDocument 文档分片逻辑
func (r *RAGEngine) splitDocument(doc Document) []*Chunk {
	content := doc.Content

	var chunks []*Chunk
	start := 0

	for start < len(content) {
		end := start + r.config.ChunkSize

		if end > len(content) {
			end = len(content)
		}

		actualEnd := r.findSentenceBoundary(content, start, end)

		chunk := &Chunk{
			ID:         fmt.Sprintf("%s_chunk_%d", doc.ID, len(chunks)),
			DocumentID: doc.ID,
			Content:    content[start:actualEnd],
			Metadata:   doc.Metadata,
			TokenCount: len(strings.Fields(content[start:actualEnd])), 
		}

		chunks = append(chunks, chunk)

		start = actualEnd - r.config.ChunkOverlap
		if start < actualEnd { 
			start = actualEnd
		}
	}

	return chunks
}

// findSentenceBoundary 查找句子边界以避免在单词中间分割
func (r *RAGEngine) findSentenceBoundary(content string, start, suggestedEnd int) int {
	if suggestedEnd >= len(content) {
		return len(content)
	}

	for i := suggestedEnd; i > start; i-- {
		char := content[i-1]
		if char == '.' || char == '!' || char == '?' || char == ' ' || char == '\n' || char == '\t' {
			return i
		}
	}

	return suggestedEnd
}

// DeleteDocument 删除文档
func (r *RAGEngine) DeleteDocument(ctx context.Context, docID string) error {
	delete(r.documents, docID)

	newChunks := make([]*Chunk, 0, len(r.chunks))
	for _, chunk := range r.chunks {
		if chunk.DocumentID != docID {
			newChunks = append(newChunks, chunk)
		}
	}
	r.chunks = newChunks

	return nil
}

// Search 搜索相关分片
func (r *RAGEngine) Search(ctx context.Context, query string, topK int) ([]Chunk, error) {
	if topK <= 0 {
		topK = r.config.MaxChunksToRetrieve
	}

	queryEmbedding, err := r.embedder.EmbedQuery(query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	scores := make([]struct {
		chunk *Chunk
		score float64
	}, len(r.chunks))

	for i, chunk := range r.chunks {
		score := cosineSimilarity(queryEmbedding, chunk.Embedding)
		scores[i] = struct {
			chunk *Chunk
			score float64
		}{chunk: chunk, score: score}
	}

	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[i].score < scores[j].score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// 返回topK结果，过滤掉低于阈值的
	var results []Chunk
	for _, scoredChunk := range scores {
		if len(results) >= topK {
			break
		}
		if scoredChunk.score >= r.config.SimilarityThreshold {
			sc := *scoredChunk.chunk
			sc.Score = scoredChunk.score
			results = append(results, sc)
		}
	}

	return results, nil
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// GetDocument 获取文档
func (r *RAGEngine) GetDocument(ctx context.Context, docID string) (*Document, error) {
	doc, exists := r.documents[docID]
	if !exists {
		return nil, errors.New("document not found")
	}
	return doc, nil
}

// UpdateConfig 更新配置
func (r *RAGEngine) UpdateConfig(config *RAGConfig) error {
	if config.ChunkSize <= 0 {
		return errors.New("chunk size must be positive")
	}
	if config.ChunkOverlap >= config.ChunkSize {
		return errors.New("chunk overlap must be less than chunk size")
	}
	if config.MaxChunksToRetrieve <= 0 {
		return errors.New("max chunks to retrieve must be positive")
	}
	if config.SimilarityThreshold < 0 || config.SimilarityThreshold > 1 {
		return errors.New("similarity threshold must be between 0 and 1")
	}

	r.config = config
	return nil
}

// GetConfig 获取当前配置
func (r *RAGEngine) GetConfig() *RAGConfig {
	return r.config
}

