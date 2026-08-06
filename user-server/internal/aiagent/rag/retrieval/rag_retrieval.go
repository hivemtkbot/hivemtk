package ragretrieval

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"marketing/internal/pkg/utils/logger"
)

// RagRetrievalServiceImpl RAG检索服务实现
type RagRetrievalServiceImpl struct {
	vectorizer VectorizerInterface
	indexer    IndexManagerInterface
	storage    StorageInterface // 文档存储接口
	cache      CacheInterface   // 缓存接口
	config     *RetrievalConfig
	reranker   RerankerInterface // 可选重排器（本地 TEI + bge-reranker-v2-m3），nil 时跳过重排
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
	DefaultTopK                int           `json:"default_top_k"`                // 默认返回结果数量
	DefaultSimilarityThreshold float64       `json:"default_similarity_threshold"` // 默认相似度阈值
	MaxTopK                    int           `json:"max_top_k"`                    // 最大返回结果数量
	MinSimilarityThreshold     float64       `json:"min_similarity_threshold"`     // 最小相似度阈值
	CacheTTL                   time.Duration `json:"cache_ttl"`                    // 缓存TTL
	MaxChunkSize               int           `json:"max_chunk_size"`               // 最大分片大小
	DefaultChunkOverlap        int           `json:"default_chunk_overlap"`        // 默认分片重叠大小
	MaxQueryLength             int           `json:"max_query_length"`             // 最大查询长度
	MaxDocLength               int           `json:"max_doc_length"`               // 最大文档长度
}

// ChunkStrategy 分片策略接口
type ChunkStrategy interface {
	CreateChunks(doc Document, config ChunkConfig) []Chunk
}

// SemanticChunkStrategy 语义分片策略
type SemanticChunkStrategy struct{}

// ChunkConfig 分片配置
type ChunkConfig struct {
	ChunkSize    int    `json:"chunk_size"`     // 分片大小
	ChunkOverlap int    `json:"chunk_overlap"`  // 分片重叠大小
	MinChunkSize int    `json:"min_chunk_size"` // 最小分片大小
	Strategy     string `json:"strategy"`       // 分片策略
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

	// 1. 预处理文档
	processedDocs, err := r.preprocessDocuments(documents)
	if err != nil {
		return fmt.Errorf("failed to preprocess documents: %w", err)
	}

	// 2. 分片处理
	allChunks := r.createChunks(processedDocs)

	// 3. 向量化
	for i := range allChunks {
		embedding, err := r.vectorizer.EmbedText(allChunks[i].Content)
		if err != nil {
			return fmt.Errorf("failed to embed chunk: %w", err)
		}
		allChunks[i].Embedding = embedding
	}

	// 4. 存储原始文档
	for _, doc := range processedDocs {
		err := r.storage.SaveDocument(ctx, kbID, doc)
		if err != nil {
			return fmt.Errorf("failed to save document: %w", err)
		}
	}

	// 5. 构建索引
	err = r.indexer.BuildIndex(ctx, kbID, allChunks)
	if err != nil {
		return fmt.Errorf("failed to build index: %w", err)
	}

	// 6. 更新知识库统计信息
	kbInfo, err := r.storage.GetKnowledgeBase(ctx, kbID)
	if err != nil {
		// 如果知识库不存在，创建新的
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

	// 7. 清除相关缓存
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

	// 1. 检查缓存
	cacheKey := fmt.Sprintf("search:%s:%s:%v", kbID, query, params)
	if cached, found := r.cache.Get(cacheKey); found {
		if results, ok := cached.([]SearchResult); ok {
			return results, nil
		}
	}

	// 2. 验证知识库是否存在
	_, err := r.storage.GetKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("knowledge base not found: %w", err)
	}

	// 3. 设置默认参数
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

	// 4. 向量化查询
	queryVec, err := r.vectorizer.EmbedText(query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// 5. 执行检索（启用重排时拉取更大的候选池）
	candidateK := params.TopK
	if r.reranker != nil {
		candidateK = maxInt(params.TopK*3, 20)
	}
	chunks, err := r.indexer.SearchIndex(ctx, kbID, queryVec, candidateK)
	if err != nil {
		return nil, fmt.Errorf("failed to search index: %w", err)
	}

	// 5.5 重排（若已装配本地 reranker；失败则安全回退到向量排序）
	if r.reranker != nil {
		reranked, rerr := r.reranker.Rerank(ctx, query, toRerankDocs(chunks))
		if rerr != nil {
			logger.Errorf("[RAG] rerank 失败，回退向量排序: %v", rerr)
		} else {
			chunks = applyRerank(chunks, reranked)
		}
	}

	// 6. 过滤和排序
	filteredChunks := r.filterResults(chunks, params.Filters, params.SimilarityThreshold)
	rankedChunks := r.rankResults(filteredChunks, query)

	// 6.5 重排后候选可能多于 TopK，截断
	if len(rankedChunks) > params.TopK {
		rankedChunks = rankedChunks[:params.TopK]
	}

	// 7. 转换为SearchResult
	var results []SearchResult
	for _, chunk := range rankedChunks {
		doc, err := r.storage.GetDocument(ctx, kbID, chunk.DocumentID)
		if err != nil {
			// 如果无法获取文档，跳过此结果
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

	// 8. 空召回告警：知识库已索引但本次检索 0 命中，便于定位向量化/阈值问题
	if len(results) == 0 {
		logger.Warnf("[RAG] 空召回 kbID=%s query=%q topK=%d threshold=%.2f 候选池=%d",
			kbID, query, params.TopK, params.SimilarityThreshold, len(chunks))
	}

	// 9. 更新缓存
	r.cache.Set(cacheKey, results, r.config.CacheTTL)

	return results, nil
}

// DeleteKnowledgeBase 删除整个知识库
func (r *RagRetrievalServiceImpl) DeleteKnowledgeBase(ctx context.Context, kbID string) error {
	if kbID == "" {
		return errors.New("kbID cannot be empty")
	}

	// 1. 删除索引
	err := r.indexer.DropIndex(ctx, kbID)
	if err != nil {
		return fmt.Errorf("failed to drop index: %w", err)
	}

	// 2. 删除存储中的知识库和相关文档
	err = r.storage.DeleteKnowledgeBase(ctx, kbID)
	if err != nil {
		return fmt.Errorf("failed to delete knowledge base from storage: %w", err)
	}

	// 3. 清除缓存
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

	// 1. 检查文档是否存在
	_, err := r.storage.GetDocument(ctx, kbID, docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// 2. 从索引中删除相关分片
	// 由于我们没有直接的接口来删除特定文档的所有分片，我们需要重建索引
	// 更好的做法是在索引管理器中实现按文档ID删除分片的方法
	// 但现在我们暂时重建整个知识库的索引

	allDocs, err := r.storage.ListDocuments(ctx, kbID)
	if err != nil {
		return fmt.Errorf("failed to list documents: %w", err)
	}

	// 过滤掉要删除的文档
	remainingDocs := make([]Document, 0)
	for _, doc := range allDocs {
		if doc.ID != docID {
			remainingDocs = append(remainingDocs, doc)
		}
	}

	// 重新索引剩余文档
	allChunks := r.createChunks(remainingDocs)

	// 向量化
	for i := range allChunks {
		embedding, err := r.vectorizer.EmbedText(allChunks[i].Content)
		if err != nil {
			return fmt.Errorf("failed to embed chunk: %w", err)
		}
		allChunks[i].Embedding = embedding
	}

	// 重建索引
	err = r.indexer.BuildIndex(ctx, kbID, allChunks)
	if err != nil {
		return fmt.Errorf("failed to rebuild index: %w", err)
	}

	// 从存储中删除文档
	err = r.storage.DeleteDocument(ctx, kbID, docID)
	if err != nil {
		return fmt.Errorf("failed to delete document from storage: %w", err)
	}

	// 更新知识库统计信息
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

	// 清除缓存
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

	// 1. 检查文档是否存在
	existingDoc, err := r.storage.GetDocument(ctx, kbID, docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// 2. 删除旧文档的索引
	err = r.DeleteDocumentFromKB(ctx, kbID, docID)
	if err != nil {
		return fmt.Errorf("failed to delete old document: %w", err)
	}

	// 3. 添加更新后的文档
	updatedDoc := document
	updatedDoc.ID = docID                        // 保持相同的ID
	updatedDoc.CreatedAt = existingDoc.CreatedAt // 保持创建时间
	updatedDoc.UpdatedAt = time.Now()            // 更新时间

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
	// 调用存储层获取知识库列表
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

	// 设置默认值
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

// preprocessDocuments 预处理文档
func (r *RagRetrievalServiceImpl) preprocessDocuments(docs []Document) ([]Document, error) {
	processed := make([]Document, len(docs))

	for i, doc := range docs {
		// 验证文档ID
		if doc.ID == "" {
			return nil, fmt.Errorf("document at index %d has empty ID", i)
		}

		// 验证文档内容
		if doc.Content == "" {
			return nil, fmt.Errorf("document %s has empty content", doc.ID)
		}

		// 检查文档长度
		if len(doc.Content) > r.config.MaxDocLength {
			return nil, fmt.Errorf("document %s content exceeds maximum allowed length of %d", doc.ID, r.config.MaxDocLength)
		}

		// 设置默认值
		if doc.CreatedAt.IsZero() {
			doc.CreatedAt = time.Now()
		}
		doc.UpdatedAt = time.Now()

		processed[i] = doc
	}

	return processed, nil
}

// createChunks 创建文档分片
func (r *RagRetrievalServiceImpl) createChunks(docs []Document) []Chunk {
	var allChunks []Chunk

	strategy := &SemanticChunkStrategy{}
	chunkSize := r.config.MaxChunkSize
	if chunkSize <= 0 {
		chunkSize = 1000 // 兜底：避免 0 导致生成空分片
	}
	overlap := r.config.DefaultChunkOverlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 10 // 重叠必须小于分片大小，否则分片无进展
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
		// 确定分片结束位置
		end := start + config.ChunkSize

		// 确保不超过内容长度
		if end > len(content) {
			end = len(content)
		}

		// 尝试在语义边界处分割
		actualEnd := s.findSemanticBoundary(content, start, end)

		// 创建分片
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

		// 更新起始位置，考虑重叠
		start = actualEnd - config.ChunkOverlap
		if start < actualEnd { // 确保进度
			start = actualEnd
		}

		// 防止无限循环
		if start == actualEnd {
			start++
		}
	}

	return chunks
}

// findSemanticBoundary 寻找语义边界
func (s *SemanticChunkStrategy) findSemanticBoundary(content string, start, suggestedEnd int) int {
	if suggestedEnd >= len(content) {
		return len(content)
	}

	// 优先在句子边界处分割
	for i := suggestedEnd; i > start; i-- {
		char := content[i-1]
		if isSentenceBoundary(char) {
			return i
		}
	}

	// 然后在段落边界处分割
	for i := suggestedEnd; i > start; i-- {
		if content[i-1] == '\n' && i > start+1 && content[i-2] == '\n' {
			return i
		}
	}

	// 最后在词语边界处分割
	for i := suggestedEnd; i > start; i-- {
		if content[i-1] == ' ' || content[i-1] == '\t' {
			return i
		}
	}

	// 如果找不到合适的边界，则使用建议的位置
	return suggestedEnd
}

// isSentenceBoundary 检查是否为句子边界
func isSentenceBoundary(char byte) bool {
	return char == '.' || char == '!' || char == '?' || char == ';' || char == ':'
}

// estimateTokenCount 估算token数量
func estimateTokenCount(text string) int {
	// 简单估算：按空格分割单词数
	words := strings.Fields(text)
	return len(words)
}

// filterResults 过滤结果
func (r *RagRetrievalServiceImpl) filterResults(chunks []Chunk, filters map[string]any, threshold float64) []Chunk {
	if len(filters) == 0 && threshold <= 0 {
		return chunks
	}

	var filtered []Chunk
	for _, chunk := range chunks {
		// 检查相似度阈值
		if threshold > 0 && chunk.Score < threshold {
			continue
		}

		// 检查过滤器
		if !r.matchFilters(chunk, filters) {
			continue
		}

		filtered = append(filtered, chunk)
	}

	return filtered
}

// matchFilters 检查是否匹配过滤器
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

// checkFilterMatch 检查单个过滤器匹配
func (r *RagRetrievalServiceImpl) checkFilterMatch(chunk Chunk, key string, value any) bool {
	// 检查元数据中是否包含指定的键值对
	if metadataValue, exists := chunk.Metadata[key]; exists {
		switch v := value.(type) {
		case string:
			if mv, ok := metadataValue.(string); ok {
				return mv == v
			}
		case int, int32, int64:
			// 处理数值比较
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

// rankResults 排序结果
func (r *RagRetrievalServiceImpl) rankResults(chunks []Chunk, query string) []Chunk {
	// 当前使用相似度分数排序，未来可以加入其他排序因素
	// 如：新鲜度、权威性、相关性等

	sorted := make([]Chunk, len(chunks))
	copy(sorted, chunks)

	// 按相似度分数降序排列
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Score < sorted[j].Score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// calculateConfidence 计算置信度
func (r *RagRetrievalServiceImpl) calculateConfidence(similarityScore float64) float64 {
	// 使用sigmoid函数将相似度分数转换为置信度
	// 将分数归一化到0-1范围
	if similarityScore >= 1.0 {
		return 1.0
	}
	if similarityScore <= 0.0 {
		return 0.0
	}

	// 简单的线性缩放，也可以使用更复杂的函数
	expVal := math.Exp(similarityScore - 0.5)
	confidence := expVal / (1 + expVal)

	// 确保在0-1范围内
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return confidence
}

// clearCacheForKB 清除知识库相关缓存
func (r *RagRetrievalServiceImpl) clearCacheForKB(kbID string) {
	// 在实际实现中，这里应该清除与特定知识库相关的所有缓存项
	// 由于我们没有遍历缓存的接口，这里只是注释说明
	// 实际实现可能需要一个带模式匹配的缓存清除方法
}
