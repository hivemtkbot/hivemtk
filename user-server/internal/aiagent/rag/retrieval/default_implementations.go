package ragretrieval

import (
	"context"
	"fmt"
	"sync"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
)

// InMemoryStorage 内存存储实现
type InMemoryStorage struct {
	knowledgeBases map[string]*KnowledgeBaseInfo
	documents      map[string]map[string]*Document // kbID -> docID -> Document
	mutex          sync.RWMutex
}

// NewInMemoryStorage 创建新的内存存储
func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		knowledgeBases: make(map[string]*KnowledgeBaseInfo),
		documents:      make(map[string]map[string]*Document),
	}
}

// SaveDocument 保存文档
func (s *InMemoryStorage) SaveDocument(ctx context.Context, kbID string, doc Document) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.documents[kbID]; !exists {
		s.documents[kbID] = make(map[string]*Document)
	}

	s.documents[kbID][doc.ID] = &doc

	return nil
}

// GetDocument 获取文档
func (s *InMemoryStorage) GetDocument(ctx context.Context, kbID, docID string) (*Document, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	docMap, exists := s.documents[kbID]
	if !exists {
		return nil, fmt.Errorf("knowledge base %s not found", kbID)
	}

	doc, exists := docMap[docID]
	if !exists {
		return nil, fmt.Errorf("document %s not found in knowledge base %s", docID, kbID)
	}

	return doc, nil
}

// DeleteDocument 删除文档
func (s *InMemoryStorage) DeleteDocument(ctx context.Context, kbID, docID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	docMap, exists := s.documents[kbID]
	if !exists {
		return fmt.Errorf("knowledge base %s not found", kbID)
	}

	if _, exists := docMap[docID]; !exists {
		return fmt.Errorf("document %s not found in knowledge base %s", docID, kbID)
	}

	delete(docMap, docID)

	// 更新知识库统计信息
	if kbInfo, kbExists := s.knowledgeBases[kbID]; kbExists {
		kbInfo.DocCount--
		kbInfo.UpdatedAt = time.Now()
	}

	return nil
}

// ListDocuments 列出文档
func (s *InMemoryStorage) ListDocuments(ctx context.Context, kbID string) ([]Document, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	docMap, exists := s.documents[kbID]
	if !exists {
		return []Document{}, nil
	}

	var docs []Document
	for _, doc := range docMap {
		docs = append(docs, *doc)
	}

	return docs, nil
}

// SaveKnowledgeBase 保存知识库信息
func (s *InMemoryStorage) SaveKnowledgeBase(ctx context.Context, kbInfo KnowledgeBaseInfo) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.knowledgeBases[kbInfo.ID] = &kbInfo

	return nil
}

// GetKnowledgeBase 获取知识库信息
func (s *InMemoryStorage) GetKnowledgeBase(ctx context.Context, kbID string) (*KnowledgeBaseInfo, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	kbInfo, exists := s.knowledgeBases[kbID]
	if !exists {
		return nil, fmt.Errorf("knowledge base %s not found", kbID)
	}

	return kbInfo, nil
}

// DeleteKnowledgeBase 删除知识库
func (s *InMemoryStorage) DeleteKnowledgeBase(ctx context.Context, kbID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.knowledgeBases[kbID]; !exists {
		return fmt.Errorf("knowledge base %s not found", kbID)
	}

	delete(s.knowledgeBases, kbID)
	delete(s.documents, kbID)

	return nil
}

// ListKnowledgeBases 列出知识库
func (s *InMemoryStorage) ListKnowledgeBases(ctx context.Context, ownerID string, includePublic bool) ([]KnowledgeBaseInfo, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var kbs []KnowledgeBaseInfo
	for _, kbInfo := range s.knowledgeBases {
		// 根据ownerID和public标志过滤
		if kbInfo.OwnerID == ownerID || (includePublic && kbInfo.IsPublic) {
			kbs = append(kbs, *kbInfo)
		}
	}

	return kbs, nil
}

// InMemoryCache 内存缓存实现
type InMemoryCache struct {
	items map[string]*cacheItem
	mutex sync.RWMutex
}

type cacheItem struct {
	value      any
	expiration time.Time
}

// NewInMemoryCache 创建新的内存缓存
func NewInMemoryCache() *InMemoryCache {
	cache := &InMemoryCache{
		items: make(map[string]*cacheItem),
	}

	// 启动清理过期项的goroutine
	go cache.startCleanup()

	return cache
}

// Get 获取缓存项
func (c *InMemoryCache) Get(key string) (any, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(item.expiration) {
		return nil, false
	}

	return item.value, true
}

// Set 设置缓存项
func (c *InMemoryCache) Set(key string, value any, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.items[key] = &cacheItem{
		value:      value,
		expiration: time.Now().Add(ttl),
	}
}

// Delete 删除缓存项
func (c *InMemoryCache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.items, key)
}

// startCleanup 启动清理过期项的goroutine
func (c *InMemoryCache) startCleanup() {
	ticker := time.NewTicker(5 * time.Minute) // 每5分钟清理一次
	defer ticker.Stop()

	for range ticker.C {
		c.cleanupExpired()
	}
}

// cleanupExpired 清理过期项
func (c *InMemoryCache) cleanupExpired() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if now.After(item.expiration) {
			delete(c.items, key)
		}
	}
}

// NewDefaultRagRetrievalService 创建带有真实实现的 RAG 检索服务
// 真实路径:EmbeddingService 走 OpenAI 兼容 /v1/embeddings
// 离线降级:无 API Key 时由 EmbeddingService 内部降级到本地哈希实现并在日志中标记
func NewDefaultRagRetrievalService() *RagRetrievalServiceImpl {
	config := &RetrievalConfig{
		DefaultTopK:                5,
		DefaultSimilarityThreshold: 0.5,
		MaxTopK:                    10,
		MinSimilarityThreshold:     0.1,
		CacheTTL:                   30 * time.Minute,
		MaxChunkSize:               512,
		DefaultChunkOverlap:        50,
		MaxQueryLength:             1000,
		MaxDocLength:               10000,
	}

	embedding := llm.NewEmbeddingService()
	dim := embedding.DefaultConfig().Dimension
	vectorizer := NewVectorizer(dim, embedding)
	indexer := NewInMemoryIndexManager(dim)
	storage := NewInMemoryStorage()
	var retrievalCache CacheInterface = NewInMemoryCache()
	if cache.GlobalIsRedis() {
		retrievalCache = NewRedisBackedCache(cache.GetGlobalCache())
	}

	return NewRagRetrievalService(vectorizer, indexer, storage, retrievalCache, config)
}

// NewProductionRagRetrievalService 创建适用于生产环境的RAG检索服务
// 注意：这只是一个示例，实际生产环境需要使用持久化存储和分布式缓存
func NewProductionRagRetrievalService(
	vectorizer VectorizerInterface,
	indexer IndexManagerInterface,
	storage StorageInterface,
	cache CacheInterface,
	config *RetrievalConfig,
) *RagRetrievalServiceImpl {
	return NewRagRetrievalService(vectorizer, indexer, storage, cache, config)
}

// NewConfigurableRagRetrievalService 创建可配置的RAG检索服务
func NewConfigurableRagRetrievalService() *RagRetrievalServiceImpl {
	appConfig := config.GetAppConfig()

	configObj := &RetrievalConfig{
		DefaultTopK:                5,
		DefaultSimilarityThreshold: 0.5,
		MaxTopK:                    10,
		MinSimilarityThreshold:     0.1,
		CacheTTL:                   30 * time.Minute,
		MaxChunkSize:               512,
		DefaultChunkOverlap:        50,
		MaxQueryLength:             1000,
		MaxDocLength:               10000,
	}

	// 真实 Embedding 注入
	embedding := llm.NewEmbeddingService()
	dim := embedding.DefaultConfig().Dimension
	vectorizer := NewVectorizer(dim, embedding)

	indexer, err := NewIndexManagerWithDB(db.GetDB(), appConfig.VectorDatabase)
	if err != nil {
		logger.Warnf("Failed to create configured index manager: %v, falling back to in-memory", err)
		indexer = NewInMemoryIndexManager(dim)
	}

	storage := NewInMemoryStorage()
	var retrievalCache CacheInterface = NewInMemoryCache()
	if cache.GlobalIsRedis() {
		retrievalCache = NewRedisBackedCache(cache.GetGlobalCache())
	}

	retrieval := NewRagRetrievalService(vectorizer, indexer, storage, retrievalCache, configObj)

	// 重排：本地 TEI + bge-reranker-v2-m3（RERANK_ENABLED=false 时自动跳过）
	if rc := DefaultRerankConfig(); rc.Enabled {
		retrieval.SetReranker(NewLocalReranker())
	}
	return retrieval
}
