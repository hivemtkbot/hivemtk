package ragretrieval

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

// RagRetrievalServiceImpl RAG检索服务实现
type RagRetrievalServiceImpl struct {
	vectorizer VectorizerInterface
	indexer    IndexManagerInterface
	storage    StorageInterface
	cache      CacheInterface
	config     *RetrievalConfig
	reranker   RerankerInterface
}

// StorageInterface 存储接口
type StorageInterface interface {
	SaveDocument(ctx context.Context, kbID string, doc Document) error
	GetDocument(ctx context.Context, kbID, docID string) (*Document, error)
	DeleteDocument(ctx context.Context, kbID, docID string) error
	ListDocuments(ctx context.Context, kbID string) ([]Document, error)
	SaveKnowledgeBase(ctx context.Context, kbInfo KnowledgeBaseInfo) error
	GetKnowledgeBase(ctx context.Context, kbID string) (*KnowledgeBaseInfo, error)
	DeleteKnowledgeBase(ctx context.Context, kbID string) error
	ListKnowledgeBases(ctx context.Context, ownerID string, includePublic bool) ([]KnowledgeBaseInfo, error)
}

// CacheInterface 缓存接口
type CacheInterface interface {
	Get(key string) (any, bool)
	Set(key string, value any, ttl time.Duration)
	Delete(key string)
}

// RetrievalConfig 检索配置
type RetrievalConfig struct {
	DefaultTopK                int           `json:"default_top_k"`
	DefaultSimilarityThreshold float64       `json:"default_similarity_threshold"`
	MaxTopK                    int           `json:"max_top_k"`
	MinSimilarityThreshold     float64       `json:"min_similarity_threshold"`
	CacheTTL                   time.Duration `json:"cache_ttl"`
	MaxChunkSize               int           `json:"max_chunk_size"`
	DefaultChunkOverlap        int           `json:"default_chunk_overlap"`
	MaxQueryLength             int           `json:"max_query_length"`
	MaxDocLength               int           `json:"max_doc_length"`
}

// ChunkStrategy 分片策略接口
type ChunkStrategy interface {
	CreateChunks(doc Document, config ChunkConfig) []Chunk
}

// SemanticChunkStrategy 语义分片策略
type SemanticChunkStrategy struct{}

// ChunkConfig 分片配置
type ChunkConfig struct {
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
	MinChunkSize int    `json:"min_chunk_size"`
	Strategy     string `json:"strategy"`
}

// NewRagRetrievalService 创建新的RAG检索服务
func NewRagRetrievalService(
	vectorizer VectorizerInterface,
	indexer IndexManagerInterface,
	storage StorageInterface,
	cache CacheInterface,
	config *RetrievalConfig,
) *RagRetrievalServiceImpl {
	if config == nil {
		config = &RetrievalConfig{
			DefaultTopK:                5,
			DefaultSimilarityThreshold: 0.5,
			MaxTopK:                    10,
			MinSimilarityThreshold:     0.1,
			CacheTTL:                   30 * time.Minute,
			MaxChunkSize:               1000,
			DefaultChunkOverlap:        100,
			MaxQueryLength:             1000,
			MaxDocLength:               10000,
		}
	}

	return &RagRetrievalServiceImpl{
		vectorizer: vectorizer,
		indexer:    indexer,
		storage:    storage,
		cache:      cache,
		config:     config,
	}
}

// IndexDocuments 向知识库中添加文档
func (r *RagRetrievalServiceImpl) IndexDocuments(ctx context.Context, kbID string, documents []Document) error {
	if kbID == "" {
		return errors.New("kbID cannot be empty")
	}

	if len(documents) == 0 {
		return errors.New("documents cannot be empty")
	}

	processedDocs, err := r.preprocessDocuments(documents)
	if err != nil {
		return fmt.Errorf("failed to preprocess documents: %w", err)
	}

	allChunks := r.createChunks(processedDocs)

	for i := range allChunks {
		embedding, err := r.vectorizer.EmbedText(allChunks[i].Content)
		if err != nil {
			return fmt.Errorf("failed to embed chunk: %w", err)
		}
		allChunks[i].Embedding = embedding
	}

	for _, doc := range processedDocs {
		err := r.storage.SaveDocument(ctx, kbID, doc)
		if err != nil {
			return fmt.Errorf("failed to save document: %w", err)
		}
	}

	err = r.indexer.BuildIndex(ctx, kbID, allChunks)
	if err != nil {
		return fmt.Errorf("failed to build index: %w", err)
	}

	kbInfo, err := r.storage.GetKnowledgeBase(ctx, kbID)
	if err != nil {
		kbInfo = &KnowledgeBaseInfo{
			ID:        kbID,
			CreatedAt: time.Now(),
		}
	}

	kbInfo.DocCount = len(processedDocs)
	kbInfo.TotalChunks = len(allChunks)
	kbInfo.UpdatedAt = time.Now()

	err = r.storage.SaveKnowledgeBase(ctx, *kbInfo)
	if err != nil {
		return fmt.Errorf("failed to update knowledge base info: %w", err)
	}

	r.clearCacheForKB(kbID)

	return nil
}

// Search 在知识库中检索相关信息
func (r *RagRetrievalServiceImpl) Search(ctx context.Context, kbID string, query string, params SearchParams) ([]SearchResult, error) {
	if kbID == "" {
		return nil, errors.New("kbID cannot be empty")
	}

	if query == "" {
		return nil, errors.New("query cannot be empty")
	}

	if len(query) > r.config.MaxQueryLength {
		return nil, fmt.Errorf("query length exceeds maximum allowed length of %d", r.config.MaxQueryLength)
	}

	cacheKey := fmt.Sprintf("search:%s:%s:%v", kbID, query, params)
	if cached, found := r.cache.Get(cacheKey); found {
		if results, ok := cached.([]SearchResult); ok {
			return results, nil
		}
	}

	_, err := r.storage.GetKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("knowledge base not found: %w", err)
	}

	if params.TopK <= 0 {
		params.TopK = r.config.DefaultTopK
	}
	if r.config.MaxTopK > 0 && params.TopK > r.config.MaxTopK {
		params.TopK = r.config.MaxTopK
	}
	if params.SimilarityThreshold == 0 {
		params.SimilarityThreshold = r.config.DefaultSimilarityThreshold
	}
	if params.SimilarityThreshold < r.config.MinSimilarityThreshold {
		params.SimilarityThreshold = r.config.MinSimilarityThreshold
	}

	queryVec, err := r.vectorizer.EmbedText(query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	candidateK := params.TopK
	if r.reranker != nil {
		candidateK = maxInt(params.TopK*3, 20)
	}
	chunks, err := r.indexer.SearchIndex(ctx, kbID, queryVec, candidateK)
	if err != nil {
		return nil, fmt.Errorf("failed to search index: %w", err)
	}

	if r.reranker != nil {
		reranked, rerr := r.reranker.Rerank(ctx, query, toRerankDocs(chunks))
		if rerr != nil {
			logger.Errorf("[RAG] rerank 失败，回退向量排序: %v", rerr)
		} else {
			chunks = applyRerank(chunks, reranked)
		}
	}

	filteredChunks := r.filterResults(chunks, params.Filters, params.SimilarityThreshold)
	rankedChunks := r.rankResults(filteredChunks, query)

	if len(rankedChunks) > params.TopK {
		rankedChunks = rankedChunks[:params.TopK]
	}

	var results []SearchResult
	for _, chunk := range rankedChunks {
		doc, err := r.storage.GetDocument(ctx, kbID, chunk.DocumentID)
		if err != nil {
			continue
		}

		confidence := r.calculateConfidence(chunk.Score)
		result := SearchResult{
			DocumentID: chunk.DocumentID,
			Content:    chunk.Content,
			Title:      doc.Title,
			Score:      chunk.Score,
			Metadata:   chunk.Metadata,
			Confidence: confidence,
			ChunkIndex: chunk.ChunkIndex,
		}
		results = append(results, result)
	}

	if len(results) == 0 {
		logger.Warnf("[RAG] 空召回 kbID=%s query=%q topK=%d threshold=%.2f 候选池=%d",
			kbID, query, params.TopK, params.SimilarityThreshold, len(chunks))
	}

	r.cache.Set(cacheKey, results, r.config.CacheTTL)

	return results, nil
}

// DeleteKnowledgeBase 删除整个知识库
func (r *RagRetrievalServiceImpl) DeleteKnowledgeBase(ctx context.Context, kbID string) error {
	if kbID == "" {
		return errors.New("kbID cannot be empty")
	}

	err := r.indexer.DropIndex(ctx, kbID)
	if err != nil {
		return fmt.Errorf("failed to drop index: %w", err)
	}

	err = r.storage.DeleteKnowledgeBase(ctx, kbID)
	if err != nil {
		return fmt.Errorf("failed to delete knowledge base from storage: %w", err)
	}

	r.clearCacheForKB(kbID)

	return nil
}

// DeleteDocumentFromKB 从知识库中删除特定文档
func (r *RagRetrievalServiceImpl) DeleteDocumentFromKB(ctx context.Context, kbID, docID string) error {
	if kbID == "" {
		return errors.New("kbID cannot be empty")
	}

	if docID == "" {
		return errors.New("docID cannot be empty")
	}

	_, err := r.storage.GetDocument(ctx, kbID, docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	allDocs, err := r.storage.ListDocuments(ctx, kbID)
	if err != nil {
		return fmt.Errorf("failed to list documents: %w", err)
	}

	remainingDocs := make([]Document, 0)
	for _, doc := range allDocs {
		if doc.ID != docID {
			remainingDocs = append(remainingDocs, doc)
		}
	}

	allChunks := r.createChunks(remainingDocs)

	for i := range allChunks {
		embedding, err := r.vectorizer.EmbedText(allChunks[i].Content)
		if err != nil {
			return fmt.Errorf("failed to embed chunk: %w", err)
		}
		allChunks[i].Embedding = embedding
	}

	err = r.indexer.BuildIndex(ctx, kbID, allChunks)
	if err != nil {
		return fmt.Errorf("failed to rebuild index: %w", err)
	}

	err = r.storage.DeleteDocument(ctx, kbID, docID)
	if err != nil {
		return fmt.Errorf("failed to delete document from storage: %w", err)
	}

	kbInfo, err := r.storage.GetKnowledgeBase(ctx, kbID)
	if err != nil {
		return fmt.Errorf("failed to get knowledge base info: %w", err)
	}

	kbInfo.DocCount = len(remainingDocs)
	kbInfo.TotalChunks = len(allChunks)
	kbInfo.UpdatedAt = time.Now()

	err = r.storage.SaveKnowledgeBase(ctx, *kbInfo)
	if err != nil {
		return fmt.Errorf("failed to update knowledge base info: %w", err)
	}

	r.clearCacheForKB(kbID)

	return nil
}

// UpdateDocumentInKB 更新知识库中的文档
func (r *RagRetrievalServiceImpl) UpdateDocumentInKB(ctx context.Context, kbID, docID string, document Document) error {
	if kbID == "" {
		return errors.New("kbID cannot be empty")
	}

	if docID == "" {
		return errors.New("docID cannot be empty")
	}

	existingDoc, err := r.storage.GetDocument(ctx, kbID, docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	err = r.DeleteDocumentFromKB(ctx, kbID, docID)
	if err != nil {
		return fmt.Errorf("failed to delete old document: %w", err)
	}

	updatedDoc := document
	updatedDoc.ID = docID
	updatedDoc.CreatedAt = existingDoc.CreatedAt
	updatedDoc.UpdatedAt = time.Now()

	err = r.IndexDocuments(ctx, kbID, []Document{updatedDoc})
	if err != nil {
		return fmt.Errorf("failed to index updated document: %w", err)
	}

	return nil
}

// GetKnowledgeBaseInfo 获取知识库信息
func (r *RagRetrievalServiceImpl) GetKnowledgeBaseInfo(ctx context.Context, kbID string) (*KnowledgeBaseInfo, error) {
	if kbID == "" {
		return nil, errors.New("kbID cannot be empty")
	}

	info, err := r.storage.GetKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("failed to get knowledge base info: %w", err)
	}

	return info, nil
}

// ListKnowledgeBases 列出所有知识库
func (r *RagRetrievalServiceImpl) ListKnowledgeBases(ctx context.Context, ownerID string, includePublic bool) ([]KnowledgeBaseInfo, error) {
	kbs, err := r.storage.ListKnowledgeBases(ctx, ownerID, includePublic)
	if err != nil {
		return nil, fmt.Errorf("failed to list knowledge bases: %w", err)
	}

	return kbs, nil
}

// CreateKnowledgeBase 创建知识库
func (r *RagRetrievalServiceImpl) CreateKnowledgeBase(ctx context.Context, kbInfo KnowledgeBaseInfo) error {
	if kbInfo.ID == "" {
		return errors.New("knowledge base ID cannot be empty")
	}

	if kbInfo.CreatedAt.IsZero() {
		kbInfo.CreatedAt = time.Now()
	}
	if kbInfo.UpdatedAt.IsZero() {
		kbInfo.UpdatedAt = time.Now()
	}

	err := r.storage.SaveKnowledgeBase(ctx, kbInfo)
	if err != nil {
		return fmt.Errorf("failed to create knowledge base: %w", err)
	}

	return nil
}

func (r *RagRetrievalServiceImpl) preprocessDocuments(docs []Document) ([]Document, error) {
	processed := make([]Document, len(docs))

	for i, doc := range docs {
		if doc.ID == "" {
			return nil, fmt.Errorf("document at index %d has empty ID", i)
		}

		if doc.Content == "" {
			return nil, fmt.Errorf("document %s has empty content", doc.ID)
		}

		if len(doc.Content) > r.config.MaxDocLength {
			return nil, fmt.Errorf("document %s content exceeds maximum allowed length of %d", doc.ID, r.config.MaxDocLength)
		}

		if doc.CreatedAt.IsZero() {
			doc.CreatedAt = time.Now()
		}
		doc.UpdatedAt = time.Now()

		processed[i] = doc
	}

	return processed, nil
}

func (r *RagRetrievalServiceImpl) createChunks(docs []Document) []Chunk {
	var allChunks []Chunk

	strategy := &SemanticChunkStrategy{}
	chunkSize := r.config.MaxChunkSize
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	overlap := r.config.DefaultChunkOverlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 10
	}
	config := ChunkConfig{
		ChunkSize:    chunkSize,
		ChunkOverlap: overlap,
	}

	for _, doc := range docs {
		chunks := strategy.CreateChunks(doc, config)
		allChunks = append(allChunks, chunks...)
	}

	return allChunks
}

// CreateChunks 实现分片策略
func (s *SemanticChunkStrategy) CreateChunks(doc Document, config ChunkConfig) []Chunk {
	var chunks []Chunk
	content := doc.Content
	start := 0

	for start < len(content) {
		end := start + config.ChunkSize

		if end > len(content) {
			end = len(content)
		}

		actualEnd := s.findSemanticBoundary(content, start, end)

		chunk := Chunk{
			ID:         fmt.Sprintf("%s_chunk_%d", doc.ID, len(chunks)),
			DocumentID: doc.ID,
			Content:    content[start:actualEnd],
			Title:      doc.Title,
			Metadata:   doc.Metadata,
			TokenCount: estimateTokenCount(content[start:actualEnd]),
			ChunkIndex: len(chunks),
		}

		chunks = append(chunks, chunk)

		start = actualEnd - config.ChunkOverlap
		if start < actualEnd {
			start = actualEnd
		}

		if start == actualEnd {
			start++
		}
	}

	return chunks
}

func (s *SemanticChunkStrategy) findSemanticBoundary(content string, start, suggestedEnd int) int {
	if suggestedEnd >= len(content) {
		return len(content)
	}

	for i := suggestedEnd; i > start; i-- {
		char := content[i-1]
		if isSentenceBoundary(char) {
			return i
		}
	}

	for i := suggestedEnd; i > start; i-- {
		if content[i-1] == '\n' && i > start+1 && content[i-2] == '\n' {
			return i
		}
	}

	for i := suggestedEnd; i > start; i-- {
		if content[i-1] == ' ' || content[i-1] == '\t' {
			return i
		}
	}

	return suggestedEnd
}

func isSentenceBoundary(char byte) bool {
	return char == '.' || char == '!' || char == '?' || char == ';' || char == ':'
}

func estimateTokenCount(text string) int {
	words := strings.Fields(text)
	return len(words)
}

func (r *RagRetrievalServiceImpl) filterResults(chunks []Chunk, filters map[string]any, threshold float64) []Chunk {
	if len(filters) == 0 && threshold <= 0 {
		return chunks
	}

	var filtered []Chunk
	for _, chunk := range chunks {
		if threshold > 0 && chunk.Score < threshold {
			continue
		}

		if !r.matchFilters(chunk, filters) {
			continue
		}

		filtered = append(filtered, chunk)
	}

	return filtered
}

func (r *RagRetrievalServiceImpl) matchFilters(chunk Chunk, filters map[string]any) bool {
	if len(filters) == 0 {
		return true
	}

	for key, value := range filters {
		if !r.checkFilterMatch(chunk, key, value) {
			return false
		}
	}

	return true
}

func (r *RagRetrievalServiceImpl) checkFilterMatch(chunk Chunk, key string, value any) bool {
	if metadataValue, exists := chunk.Metadata[key]; exists {
		switch v := value.(type) {
		case string:
			if mv, ok := metadataValue.(string); ok {
				return mv == v
			}
		case int, int32, int64:
			return fmt.Sprintf("%v", metadataValue) == fmt.Sprintf("%v", value)
		case float64:
			if mv, ok := metadataValue.(float64); ok {
				return mv == v
			}
		default:
			return fmt.Sprintf("%v", metadataValue) == fmt.Sprintf("%v", value)
		}
	}

	return false
}

func (r *RagRetrievalServiceImpl) rankResults(chunks []Chunk, query string) []Chunk {

	sorted := make([]Chunk, len(chunks))
	copy(sorted, chunks)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Score < sorted[j].Score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

func (r *RagRetrievalServiceImpl) calculateConfidence(similarityScore float64) float64 {
	if similarityScore >= 1.0 {
		return 1.0
	}
	if similarityScore <= 0.0 {
		return 0.0
	}

	expVal := math.Exp(similarityScore - 0.5)
	confidence := expVal / (1 + expVal)

	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return confidence
}

func (r *RagRetrievalServiceImpl) clearCacheForKB(kbID string) {
}
