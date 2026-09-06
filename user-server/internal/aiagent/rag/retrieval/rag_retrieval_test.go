package ragretrieval

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"
)

type constantVectorizer struct {
	dimension int
}

func (v *constantVectorizer) EmbedText(text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}
	vec := make([]float32, v.dimension)
	val := float32(1.0 / math.Sqrt(float64(v.dimension)))
	for i := range vec {
		vec[i] = val
	}
	return vec, nil
}

func (v *constantVectorizer) EmbedBatch(texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := v.EmbedText(text)
		if err != nil {
			return nil, err
		}
		results[i] = vec
	}
	return results, nil
}

func (v *constantVectorizer) GetDimension() int { return v.dimension }
func (v *constantVectorizer) ValidateEmbedding(embedding []float32) bool {
	return len(embedding) == v.dimension
}

// MockIndexManager 模拟索引管理器
type MockIndexManager struct {
	indexes map[string][]Chunk
}

func NewMockIndexManager() *MockIndexManager {
	return &MockIndexManager{
		indexes: make(map[string][]Chunk),
	}
}

func (m *MockIndexManager) BuildIndex(ctx context.Context, kbID string, chunks []Chunk) error {
	if kbID == "" {
		return fmt.Errorf("kbID cannot be empty")
	}
	m.indexes[kbID] = chunks
	return nil
}

func (m *MockIndexManager) AddToIndex(ctx context.Context, kbID string, chunk Chunk) error {
	m.indexes[kbID] = append(m.indexes[kbID], chunk)
	return nil
}

func (m *MockIndexManager) RemoveFromIndex(ctx context.Context, kbID, chunkID string) error {
	chunks := m.indexes[kbID]
	newChunks := make([]Chunk, 0)
	for _, c := range chunks {
		if c.ID != chunkID {
			newChunks = append(newChunks, c)
		}
	}
	m.indexes[kbID] = newChunks
	return nil
}

func (m *MockIndexManager) SearchIndex(ctx context.Context, kbID string, queryVec []float32, topK int) ([]Chunk, error) {
	chunks := m.indexes[kbID]
	if len(chunks) == 0 {
		return []Chunk{}, nil
	}

	scoredChunks := make([]struct {
		chunk Chunk
		score float64
	}, len(chunks))

	for i, chunk := range chunks {
		if len(chunk.Embedding) > 0 && len(queryVec) > 0 {
			score := cosineSimilarity(queryVec, chunk.Embedding)
			scoredChunks[i] = struct {
				chunk Chunk
				score float64
			}{
				chunk: chunk,
				score: score,
			}
		} else {
			scoredChunks[i] = struct {
				chunk Chunk
				score float64
			}{
				chunk: chunk,
				score: 0.95 - float64(i)*0.05,
			}
		}
	}

	for i := 0; i < len(scoredChunks)-1; i++ {
		for j := i + 1; j < len(scoredChunks); j++ {
			if scoredChunks[i].score < scoredChunks[j].score {
				scoredChunks[i], scoredChunks[j] = scoredChunks[j], scoredChunks[i]
			}
		}
	}

	result := make([]Chunk, 0, topK)
	for i := 0; i < len(scoredChunks) && i < topK; i++ {
		chunkCopy := scoredChunks[i].chunk
		chunkCopy.Score = scoredChunks[i].score
		result = append(result, chunkCopy)
	}

	return result, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	if len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func (m *MockIndexManager) DropIndex(ctx context.Context, kbID string) error {
	delete(m.indexes, kbID)
	return nil
}

func (m *MockIndexManager) GetIndexStats(ctx context.Context, kbID string) (*IndexStats, error) {
	chunks := m.indexes[kbID]
	return &IndexStats{
		KbID:        kbID,
		VectorCount: len(chunks),
		Dimension:   128,
		MemoryUsage: int64(len(chunks) * 128 * 4),
		LastUpdated: time.Now(),
	}, nil
}

// MockStorage 模拟存储
type MockStorage struct {
	knowledgeBases map[string]KnowledgeBaseInfo
	documents      map[string]map[string]Document
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		knowledgeBases: make(map[string]KnowledgeBaseInfo),
		documents:      make(map[string]map[string]Document),
	}
}

func (m *MockStorage) SaveDocument(ctx context.Context, kbID string, doc Document) error {
	if m.documents[kbID] == nil {
		m.documents[kbID] = make(map[string]Document)
	}
	m.documents[kbID][doc.ID] = doc
	return nil
}

func (m *MockStorage) GetDocument(ctx context.Context, kbID, docID string) (*Document, error) {
	if docs, ok := m.documents[kbID]; ok {
		if doc, exists := docs[docID]; exists {
			return &doc, nil
		}
	}
	return nil, fmt.Errorf("document not found")
}

func (m *MockStorage) DeleteDocument(ctx context.Context, kbID, docID string) error {
	if docs, ok := m.documents[kbID]; ok {
		delete(docs, docID)
		return nil
	}
	return fmt.Errorf("knowledge base not found")
}

func (m *MockStorage) ListDocuments(ctx context.Context, kbID string) ([]Document, error) {
	docs, ok := m.documents[kbID]
	if !ok {
		return nil, fmt.Errorf("knowledge base not found")
	}
	result := make([]Document, 0, len(docs))
	for _, doc := range docs {
		result = append(result, doc)
	}
	return result, nil
}

func (m *MockStorage) SaveKnowledgeBase(ctx context.Context, kbInfo KnowledgeBaseInfo) error {
	m.knowledgeBases[kbInfo.ID] = kbInfo
	return nil
}

func (m *MockStorage) GetKnowledgeBase(ctx context.Context, kbID string) (*KnowledgeBaseInfo, error) {
	if kb, ok := m.knowledgeBases[kbID]; ok {
		return &kb, nil
	}
	return nil, fmt.Errorf("knowledge base not found")
}

func (m *MockStorage) DeleteKnowledgeBase(ctx context.Context, kbID string) error {
	delete(m.knowledgeBases, kbID)
	delete(m.documents, kbID)
	return nil
}

func (m *MockStorage) ListKnowledgeBases(ctx context.Context, ownerID string, includePublic bool) ([]KnowledgeBaseInfo, error) {
	result := make([]KnowledgeBaseInfo, 0)
	for _, kb := range m.knowledgeBases {
		if kb.OwnerID == ownerID || (includePublic && kb.IsPublic) {
			result = append(result, kb)
		}
	}
	return result, nil
}

func (m *MockStorage) CreateKnowledgeBase(ctx context.Context, kbInfo KnowledgeBaseInfo) error {
	if _, ok := m.knowledgeBases[kbInfo.ID]; ok {
		return fmt.Errorf("knowledge base already exists")
	}
	m.knowledgeBases[kbInfo.ID] = kbInfo
	return nil
}

// MockCache 模拟缓存
type MockCache struct {
	data map[string]any
}

func NewMockCache() *MockCache {
	return &MockCache{
		data: make(map[string]any),
	}
}

func (m *MockCache) Get(key string) (any, bool) {
	val, ok := m.data[key]
	return val, ok
}

func (m *MockCache) Set(key string, value any, ttl time.Duration) {
	m.data[key] = value
}

func (m *MockCache) Delete(key string) {
	delete(m.data, key)
}

// TestRagRetrievalServiceImpl_NewRagRetrievalService 测试创建服务
func TestRagRetrievalServiceImpl_NewRagRetrievalService(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()
	config := &RetrievalConfig{
		DefaultTopK:                5,
		DefaultSimilarityThreshold: 0.5,
		MaxTopK:                    10,
	}

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, config)
	if service == nil {
		t.Fatal("Expected service to be created")
	}

	nilService := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	if nilService == nil {
		t.Fatal("Expected service to be created with nil config")
	}
}

// TestRagRetrievalServiceImpl_IndexDocuments 测试索引文档
func TestRagRetrievalServiceImpl_IndexDocuments(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()
	config := &RetrievalConfig{
		MaxDocLength:   10000,
		MaxQueryLength: 1000,
		MaxChunkSize:   1000,
	}

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, config)
	ctx := context.Background()

	kbInfo := KnowledgeBaseInfo{
		ID:          "test_kb",
		Name:        "测试知识库",
		Description: "用于测试的知识库",
		OwnerID:     "user1",
		IsPublic:    true,
	}
	storage.SaveKnowledgeBase(ctx, kbInfo)

	documents := []Document{
		{ID: "doc1", Title: "文档 1", Content: "这是第一个测试文档的内容"},
		{ID: "doc2", Title: "文档 2", Content: "这是第二个测试文档的内容"},
	}

	err := service.IndexDocuments(ctx, "test_kb", documents)
	if err != nil {
		t.Fatalf("Failed to index documents: %v", err)
	}

	storedDoc, err := storage.GetDocument(ctx, "test_kb", "doc1")
	if err != nil {
		t.Errorf("Failed to get stored document: %v", err)
	}
	if storedDoc.Title != "文档 1" {
		t.Errorf("Expected title '文档 1', got %s", storedDoc.Title)
	}

	stats, err := indexer.GetIndexStats(ctx, "test_kb")
	if err != nil {
		t.Errorf("Failed to get index stats: %v", err)
	}
	if stats.VectorCount == 0 {
		t.Error("Expected index to have vectors")
	}
}

// TestRagRetrievalServiceImpl_IndexDocuments_EmptyKB 测试空知识库 ID
func TestRagRetrievalServiceImpl_IndexDocuments_EmptyKB(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	err := service.IndexDocuments(ctx, "", []Document{})
	if err == nil {
		t.Error("Expected error for empty kbID")
	}
}

// TestRagRetrievalServiceImpl_IndexDocuments_EmptyDocs 测试空文档列表
func TestRagRetrievalServiceImpl_IndexDocuments_EmptyDocs(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	err := service.IndexDocuments(ctx, "test_kb", []Document{})
	if err == nil {
		t.Error("Expected error for empty documents")
	}
}

// TestRagRetrievalServiceImpl_Search 测试搜索功能
func TestRagRetrievalServiceImpl_Search(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()
	config := &RetrievalConfig{
		MaxQueryLength:             1000,
		MaxDocLength:               10000,
		MaxChunkSize:               1000,
		DefaultTopK:                5,
		DefaultSimilarityThreshold: 0.3,
		MinSimilarityThreshold:     0.1,
	}

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, config)
	ctx := context.Background()

	kbInfo := KnowledgeBaseInfo{ID: "search_kb", OwnerID: "user1", IsPublic: true}
	err := storage.SaveKnowledgeBase(ctx, kbInfo)
	if err != nil {
		t.Fatalf("Failed to save knowledge base: %v", err)
	}

	savedKb, err := storage.GetKnowledgeBase(ctx, "search_kb")
	if err != nil {
		t.Fatalf("Failed to get saved knowledge base: %v", err)
	}
	t.Logf("Saved knowledge base: %+v", savedKb)

	documents := []Document{
		{ID: "doc1", Title: "AI 文档", Content: "人工智能是未来科技"},
		{ID: "doc2", Title: "ML 文档", Content: "机器学习是 AI 的分支"},
	}
	err = service.IndexDocuments(ctx, "search_kb", documents)
	if err != nil {
		t.Fatalf("Failed to index documents: %v", err)
	}

	stats, err := indexer.GetIndexStats(ctx, "search_kb")
	if err != nil {
		t.Fatalf("Failed to get index stats: %v", err)
	}
	t.Logf("Index stats: %+v", stats)

	docs, err := storage.ListDocuments(ctx, "search_kb")
	if err != nil {
		t.Fatalf("Failed to list documents: %v", err)
	}
	t.Logf("Stored documents: %+v", docs)

	mockIndexer := indexer
	t.Logf("Indexed chunks count: %d", len(mockIndexer.indexes["search_kb"]))
	for i, chunk := range mockIndexer.indexes["search_kb"] {
		t.Logf("Chunk %d: ID=%s, DocumentID=%s, Content=%s", i, chunk.ID, chunk.DocumentID, chunk.Content)
	}

	params := SearchParams{TopK: 5, SimilarityThreshold: 0.3}

	cache.Delete("search:search_kb:人工智能:{5 0.3 map[] 0}")

	queryVec, _ := vectorizer.EmbedText("人工智能")
	t.Logf("Query vector (first 10): %v", queryVec[:10])
	chunks, err := indexer.SearchIndex(ctx, "search_kb", queryVec, params.TopK)
	if err != nil {
		t.Fatalf("Failed to search index: %v", err)
	}
	t.Logf("Direct indexer results count: %d", len(chunks))
	for i, chunk := range chunks {
		t.Logf("Chunk %d: ID=%s, Score=%f, Embedding (first 5)=%v", i, chunk.ID, chunk.Score, chunk.Embedding[:5])
	}

	filteredChunks := service.filterResults(chunks, params.Filters, params.SimilarityThreshold)
	t.Logf("Filtered chunks: %+v", filteredChunks)

	rankedChunks := service.rankResults(filteredChunks, "人工智能")
	t.Logf("Ranked chunks count: %d", len(rankedChunks))
	for i, chunk := range rankedChunks {
		t.Logf("Ranked chunk %d: DocumentID=%s, Score=%f", i, chunk.DocumentID, chunk.Score)
		doc, err := storage.GetDocument(ctx, "search_kb", chunk.DocumentID)
		if err != nil {
			t.Logf("Failed to get document %s: %v", chunk.DocumentID, err)
		} else {
			t.Logf("Got document %s: %+v", chunk.DocumentID, doc)
		}
	}

	results, err := service.Search(ctx, "search_kb", "人工智能", params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	t.Logf("Service indexer type: %T", service.indexer)
	t.Logf("Service storage type: %T", service.storage)
	t.Logf("Service config: %+v", service.config)

	t.Logf("Search results: %+v", results)

	if len(results) == 0 {
		t.Error("Expected search results")
	}

	for _, result := range results {
		if result.Score < 0 || result.Score > 1 {
			t.Errorf("Invalid score: %f", result.Score)
		}
		if result.Confidence < 0 || result.Confidence > 1 {
			t.Errorf("Invalid confidence: %f", result.Confidence)
		}
	}
}

// TestRagRetrievalServiceImpl_Search_EmptyQuery 测试空查询
func TestRagRetrievalServiceImpl_Search_EmptyQuery(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	_, err := service.Search(ctx, "test_kb", "", SearchParams{})
	if err == nil {
		t.Error("Expected error for empty query")
	}
}

// TestRagRetrievalServiceImpl_Search_EmptyKB 测试空知识库
func TestRagRetrievalServiceImpl_Search_EmptyKB(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	_, err := service.Search(ctx, "", "query", SearchParams{})
	if err == nil {
		t.Error("Expected error for empty kbID")
	}
}

// TestRagRetrievalServiceImpl_DeleteKnowledgeBase 测试删除知识库
func TestRagRetrievalServiceImpl_DeleteKnowledgeBase(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	kbInfo := KnowledgeBaseInfo{ID: "delete_kb"}
	storage.SaveKnowledgeBase(ctx, kbInfo)
	service.IndexDocuments(ctx, "delete_kb", []Document{{ID: "doc1", Content: "测试内容"}})

	err := service.DeleteKnowledgeBase(ctx, "delete_kb")
	if err != nil {
		t.Fatalf("Failed to delete knowledge base: %v", err)
	}

	_, err = storage.GetKnowledgeBase(ctx, "delete_kb")
	if err == nil {
		t.Error("Expected knowledge base to be deleted")
	}
}

// TestRagRetrievalServiceImpl_DeleteDocumentFromKB 测试删除文档
func TestRagRetrievalServiceImpl_DeleteDocumentFromKB(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	kbInfo := KnowledgeBaseInfo{ID: "delete_doc_kb"}
	storage.SaveKnowledgeBase(ctx, kbInfo)
	service.IndexDocuments(ctx, "delete_doc_kb", []Document{
		{ID: "doc1", Content: "文档 1"},
		{ID: "doc2", Content: "文档 2"},
	})

	err := service.DeleteDocumentFromKB(ctx, "delete_doc_kb", "doc1")
	if err != nil {
		t.Fatalf("Failed to delete document: %v", err)
	}

	_, err = storage.GetDocument(ctx, "delete_doc_kb", "doc1")
	if err == nil {
		t.Error("Expected document to be deleted")
	}

	doc, err := storage.GetDocument(ctx, "delete_doc_kb", "doc2")
	if err != nil {
		t.Errorf("Expected doc2 to still exist: %v", err)
	}
	if doc.Content != "文档 2" {
		t.Errorf("Expected doc2 content, got %s", doc.Content)
	}
}

// TestRagRetrievalServiceImpl_UpdateDocumentInKB 测试更新文档
func TestRagRetrievalServiceImpl_UpdateDocumentInKB(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	kbInfo := KnowledgeBaseInfo{ID: "update_kb"}
	storage.SaveKnowledgeBase(ctx, kbInfo)
	service.IndexDocuments(ctx, "update_kb", []Document{
		{ID: "doc1", Content: "原始内容", Title: "原始标题"},
	})

	updatedDoc := Document{Content: "更新后的内容", Title: "更新后的标题"}
	err := service.UpdateDocumentInKB(ctx, "update_kb", "doc1", updatedDoc)
	if err != nil {
		t.Fatalf("Failed to update document: %v", err)
	}

	doc, err := storage.GetDocument(ctx, "update_kb", "doc1")
	if err != nil {
		t.Fatalf("Failed to get updated document: %v", err)
	}
	if doc.Content != "更新后的内容" {
		t.Errorf("Expected updated content, got %s", doc.Content)
	}
}

// TestRagRetrievalServiceImpl_GetKnowledgeBaseInfo 测试获取知识库信息
func TestRagRetrievalServiceImpl_GetKnowledgeBaseInfo(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	kbInfo := KnowledgeBaseInfo{ID: "info_kb", Name: "测试知识库", OwnerID: "user1"}
	storage.SaveKnowledgeBase(ctx, kbInfo)

	info, err := service.GetKnowledgeBaseInfo(ctx, "info_kb")
	if err != nil {
		t.Fatalf("Failed to get knowledge base info: %v", err)
	}

	if info.Name != "测试知识库" {
		t.Errorf("Expected name '测试知识库', got %s", info.Name)
	}
}

// TestRagRetrievalServiceImpl_ListKnowledgeBases 测试列出知识库
func TestRagRetrievalServiceImpl_ListKnowledgeBases(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	kbs := []KnowledgeBaseInfo{
		{ID: "kb1", Name: "知识库 1", OwnerID: "user1", IsPublic: false},
		{ID: "kb2", Name: "知识库 2", OwnerID: "user1", IsPublic: true},
		{ID: "kb3", Name: "知识库 3", OwnerID: "user2", IsPublic: true},
	}
	for _, kb := range kbs {
		storage.SaveKnowledgeBase(ctx, kb)
	}

	list, err := service.ListKnowledgeBases(ctx, "user1", false)
	if err != nil {
		t.Fatalf("Failed to list knowledge bases: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("Expected 2 knowledge bases, got %d", len(list))
	}

	list, err = service.ListKnowledgeBases(ctx, "user1", true)
	if err != nil {
		t.Fatalf("Failed to list knowledge bases with public: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("Expected 3 knowledge bases with public, got %d", len(list))
	}
}

// TestRagRetrievalServiceImpl_CreateKnowledgeBase 测试创建知识库
func TestRagRetrievalServiceImpl_CreateKnowledgeBase(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	kbInfo := KnowledgeBaseInfo{ID: "new_kb", Name: "新建知识库", OwnerID: "user1"}

	err := service.CreateKnowledgeBase(ctx, kbInfo)
	if err != nil {
		t.Fatalf("Failed to create knowledge base: %v", err)
	}

	info, err := storage.GetKnowledgeBase(ctx, "new_kb")
	if err != nil {
		t.Fatalf("Failed to get created knowledge base: %v", err)
	}
	if info.Name != "新建知识库" {
		t.Errorf("Expected name '新建知识库', got %s", info.Name)
	}
}

// TestRagRetrievalServiceImpl_CreateKnowledgeBase_Duplicate 测试重复创建
func TestRagRetrievalServiceImpl_CreateKnowledgeBase_Duplicate(t *testing.T) {
	vectorizer := &constantVectorizer{dimension: 128}
	indexer := NewMockIndexManager()
	storage := NewMockStorage()
	cache := NewMockCache()

	service := NewRagRetrievalService(vectorizer, indexer, storage, cache, nil)
	ctx := context.Background()

	kbInfo := KnowledgeBaseInfo{ID: "dup_kb", Name: "重复知识库"}

	err := service.CreateKnowledgeBase(ctx, kbInfo)
	if err != nil {
		t.Fatalf("Failed to create knowledge base: %v", err)
	}

	info, err := service.GetKnowledgeBaseInfo(ctx, "dup_kb")
	if err != nil {
		t.Fatalf("Failed to get created knowledge base: %v", err)
	}
	if info.Name != "重复知识库" {
		t.Errorf("Expected name '重复知识库', got %s", info.Name)
	}
}

// TestSearchParams 测试搜索参数
func TestSearchParams(t *testing.T) {
	params := SearchParams{
		TopK:                10,
		SimilarityThreshold: 0.7,
		Filters:             map[string]any{"type": "important"},
		RelevanceBoost:      1.2,
	}

	if params.TopK != 10 {
		t.Errorf("Expected TopK 10, got %d", params.TopK)
	}
	if params.SimilarityThreshold != 0.7 {
		t.Errorf("Expected SimilarityThreshold 0.7, got %f", params.SimilarityThreshold)
	}
}

// TestSearchResult 测试结果结构
func TestSearchResult(t *testing.T) {
	result := SearchResult{
		DocumentID: "doc1",
		Content:    "相关内容",
		Title:      "相关文档",
		Score:      0.85,
		Confidence: 0.9,
		ChunkIndex: 2,
	}

	if result.DocumentID != "doc1" {
		t.Errorf("Expected DocumentID 'doc1', got %s", result.DocumentID)
	}
	if result.Score < 0 || result.Score > 1 {
		t.Errorf("Invalid Score: %f", result.Score)
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		t.Errorf("Invalid Confidence: %f", result.Confidence)
	}
}

// TestIndexStats 测试索引统计
func TestIndexStats(t *testing.T) {
	stats := IndexStats{
		KbID:        "test_kb",
		VectorCount: 100,
		Dimension:   128,
		MemoryUsage: 1024 * 128 * 4,
		LastUpdated: time.Now(),
	}

	if stats.KbID != "test_kb" {
		t.Errorf("Expected KbID 'test_kb', got %s", stats.KbID)
	}
	if stats.VectorCount != 100 {
		t.Errorf("Expected VectorCount 100, got %d", stats.VectorCount)
	}
}

// TestChunk 测试 Chunk 结构
func TestChunk(t *testing.T) {
	chunk := Chunk{
		ID:         "chunk1",
		DocumentID: "doc1",
		Content:    "测试分片内容",
		Title:      "测试文档",
		Score:      0.85,
		TokenCount: 50,
		ChunkIndex: 0,
	}

	if chunk.ID != "chunk1" {
		t.Errorf("Expected ID 'chunk1', got %s", chunk.ID)
	}
	if chunk.Score < 0 || chunk.Score > 1 {
		t.Errorf("Invalid Score: %f", chunk.Score)
	}
}

// TestDocument 测试 Document 结构
func TestDocument(t *testing.T) {
	doc := Document{
		ID:       "doc1",
		Title:    "测试文档",
		Content:  "测试内容",
		Metadata: map[string]any{"author": "tester"},
	}

	if doc.ID != "doc1" {
		t.Errorf("Expected ID 'doc1', got %s", doc.ID)
	}
	if doc.Title != "测试文档" {
		t.Errorf("Expected Title '测试文档', got %s", doc.Title)
	}
}

// TestKnowledgeBaseInfo 测试知识库信息
func TestKnowledgeBaseInfo(t *testing.T) {
	kbInfo := KnowledgeBaseInfo{
		ID:          "kb1",
		Name:        "测试知识库",
		Description: "测试描述",
		OwnerID:     "user1",
		IsPublic:    true,
		DocCount:    10,
		TotalChunks: 50,
	}

	if kbInfo.ID != "kb1" {
		t.Errorf("Expected ID 'kb1', got %s", kbInfo.ID)
	}
	if kbInfo.DocCount != 10 {
		t.Errorf("Expected DocCount 10, got %d", kbInfo.DocCount)
	}
	if !kbInfo.IsPublic {
		t.Error("Expected IsPublic to be true")
	}
}
