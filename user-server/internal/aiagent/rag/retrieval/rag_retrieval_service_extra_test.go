package ragretrieval

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRagRetrievalServiceImpl_Search_QueryTooLong 查询超过最大长度
func TestRagRetrievalServiceImpl_Search_QueryTooLong(t *testing.T) {
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), NewMockStorage(), NewMockCache(), &RetrievalConfig{MaxQueryLength: 10})
	ctx := context.Background()

	_, err := service.Search(ctx, "kb", "12345678901", SearchParams{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum allowed length")
}

// TestRagRetrievalServiceImpl_Search_KBNotFound 知识库不存在
func TestRagRetrievalServiceImpl_Search_KBNotFound(t *testing.T) {
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), NewMockStorage(), NewMockCache(), nil)
	ctx := context.Background()

	// 未预存 KB，GetKnowledgeBase 返回错误
	_, err := service.Search(ctx, "missing_kb", "查询", SearchParams{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "knowledge base not found")
}

// TestRagRetrievalServiceImpl_Search_CacheHit 搜索结果进入缓存
func TestRagRetrievalServiceImpl_Search_CacheHit(t *testing.T) {
	storage := NewMockStorage()
	cache := NewMockCache()
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), storage, cache, &RetrievalConfig{
		MaxQueryLength: 1000, DefaultTopK: 5, DefaultSimilarityThreshold: 0.3, MinSimilarityThreshold: 0.1,
		MaxChunkSize: 1000, DefaultChunkOverlap: 100, MaxDocLength: 10000,
	})
	ctx := context.Background()

	storage.SaveKnowledgeBase(ctx, KnowledgeBaseInfo{ID: "cache_kb", OwnerID: "u1"})
	assert.NoError(t, service.IndexDocuments(ctx, "cache_kb", []Document{{ID: "d1", Content: "缓存测试文档"}}))

	params := SearchParams{TopK: 5, SimilarityThreshold: 0.3}
	results, err := service.Search(ctx, "cache_kb", "缓存", params)
	assert.NoError(t, err)
	assert.NotEmpty(t, results)

	// 验证结果已写入缓存（cacheKey 格式与 Search 内部一致）
	key := fmt.Sprintf("search:%s:%s:%v", "cache_kb", "缓存", params)
	cached, ok := cache.Get(key)
	assert.True(t, ok, "search result should be cached")
	cachedResults, ok := cached.([]SearchResult)
	assert.True(t, ok)
	assert.Len(t, cachedResults, len(results))
}

// TestRagRetrievalServiceImpl_CreateKnowledgeBase_EmptyID 空 ID 校验
func TestRagRetrievalServiceImpl_CreateKnowledgeBase_EmptyID(t *testing.T) {
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), NewMockStorage(), NewMockCache(), nil)
	ctx := context.Background()
	err := service.CreateKnowledgeBase(ctx, KnowledgeBaseInfo{ID: ""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

// TestRagRetrievalServiceImpl_GetKnowledgeBaseInfo_EmptyID 空 ID 校验
func TestRagRetrievalServiceImpl_GetKnowledgeBaseInfo_EmptyID(t *testing.T) {
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), NewMockStorage(), NewMockCache(), nil)
	ctx := context.Background()
	_, err := service.GetKnowledgeBaseInfo(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

// TestRagRetrievalServiceImpl_DeleteDocumentFromKB_NotFound 删除不存在文档
func TestRagRetrievalServiceImpl_DeleteDocumentFromKB_NotFound(t *testing.T) {
	storage := NewMockStorage()
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), storage, NewMockCache(), nil)
	ctx := context.Background()
	storage.SaveKnowledgeBase(ctx, KnowledgeBaseInfo{ID: "dd_kb"})
	err := service.DeleteDocumentFromKB(ctx, "dd_kb", "ghost")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "document not found")
}

// TestRagRetrievalServiceImpl_UpdateDocumentInKB_NotFound 更新不存在文档
func TestRagRetrievalServiceImpl_UpdateDocumentInKB_NotFound(t *testing.T) {
	storage := NewMockStorage()
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), storage, NewMockCache(), nil)
	ctx := context.Background()
	storage.SaveKnowledgeBase(ctx, KnowledgeBaseInfo{ID: "ud_kb"})
	err := service.UpdateDocumentInKB(ctx, "ud_kb", "ghost", Document{Content: "x"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "document not found")
}

// TestRagRetrievalServiceImpl_IndexDocuments_AutoCreateKB 知识库不存在时自动创建
func TestRagRetrievalServiceImpl_IndexDocuments_AutoCreateKB(t *testing.T) {
	storage := NewMockStorage()
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), storage, NewMockCache(), &RetrievalConfig{MaxDocLength: 10000, MaxChunkSize: 1000, DefaultChunkOverlap: 100})
	ctx := context.Background()

	// 不预存 KB
	err := service.IndexDocuments(ctx, "auto_kb", []Document{{ID: "d1", Content: "自动创建知识库文档"}})
	assert.NoError(t, err)

	info, err := storage.GetKnowledgeBase(ctx, "auto_kb")
	assert.NoError(t, err)
	assert.Equal(t, 1, info.DocCount)
	assert.Equal(t, 1, info.TotalChunks)
}

// TestRagRetrievalServiceImpl_preprocessDocuments 预处理校验
func TestRagRetrievalServiceImpl_preprocessDocuments(t *testing.T) {
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), NewMockStorage(), NewMockCache(), &RetrievalConfig{MaxDocLength: 20})

	// 空文档 ID
	_, err := service.preprocessDocuments([]Document{{ID: "", Content: "内容"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty ID")

	// 空内容
	_, err = service.preprocessDocuments([]Document{{ID: "d1", Content: ""}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty content")

	// 内容超长
	_, err = service.preprocessDocuments([]Document{{ID: "d1", Content: strings.Repeat("长", 100)}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum allowed length")

	// 正常
	out, err := service.preprocessDocuments([]Document{{ID: "d1", Content: "正常内容"}})
	assert.NoError(t, err)
	assert.Len(t, out, 1)
	assert.False(t, out[0].CreatedAt.IsZero())
}

// TestRagRetrievalServiceImpl_createChunks 分片行为
func TestRagRetrievalServiceImpl_createChunks(t *testing.T) {
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), NewMockStorage(), NewMockCache(),
		&RetrievalConfig{MaxChunkSize: 10, DefaultChunkOverlap: 2, MaxDocLength: 10000})

	meta := map[string]any{"src": "test"}
	docs := []Document{{ID: "doc1", Title: "标题", Content: "这是一段用于测试语义分片切分行为的较长文本内容需要被切分成多个片段", Metadata: meta}}
	chunks := service.createChunks(docs)

	assert.NotEmpty(t, chunks)
	for i, c := range chunks {
		assert.Equal(t, i, c.ChunkIndex, "chunk index should be sequential")
		assert.Equal(t, "doc1", c.DocumentID)
		assert.Equal(t, "标题", c.Title)
		assert.Equal(t, meta, c.Metadata, "metadata should be copied to chunk")
		assert.True(t, strings.HasPrefix(c.ID, "doc1_chunk_"), "chunk id should derive from doc id")
	}
}

// TestSemanticChunkStrategy_CreateChunks_Single 短文档单分片
func TestSemanticChunkStrategy_CreateChunks_Single(t *testing.T) {
	s := &SemanticChunkStrategy{}
	chunks := s.CreateChunks(Document{ID: "d", Content: "短文档"}, ChunkConfig{ChunkSize: 1000, ChunkOverlap: 0})
	assert.Len(t, chunks, 1)
	assert.Equal(t, "短文档", chunks[0].Content)
}

// TestSemanticChunkStrategy_findSemanticBoundary 语义边界查找
func TestSemanticChunkStrategy_findSemanticBoundary(t *testing.T) {
	s := &SemanticChunkStrategy{}
	content := "abc. def! ghi?"

	// 在句子边界 '.'(index 3) 处截断，suggestedEnd=5 应回退到 4
	assert.Equal(t, 4, s.findSemanticBoundary(content, 0, 5))
	// suggestedEnd 超出长度，返回整段长度
	assert.Equal(t, len(content), s.findSemanticBoundary(content, 0, 100))
	// 无边界时返回 suggestedEnd
	assert.Equal(t, 4, s.findSemanticBoundary("abcdef", 0, 4))
}

// TestRagRetrievalServiceImpl_filterResults 过滤与阈值
func TestRagRetrievalServiceImpl_filterResults(t *testing.T) {
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), NewMockStorage(), NewMockCache(), nil)
	chunks := []Chunk{
		{Score: 0.9, Metadata: map[string]any{"type": "faq"}},
		{Score: 0.2, Metadata: map[string]any{"type": "faq"}},
		{Score: 0.8, Metadata: map[string]any{"type": "other"}},
	}

	// 仅阈值
	assert.Len(t, service.filterResults(chunks, nil, 0.5), 2)
	// 仅过滤器
	assert.Len(t, service.filterResults(chunks, map[string]any{"type": "faq"}, 0), 2)
	// 阈值 + 过滤器
	assert.Len(t, service.filterResults(chunks, map[string]any{"type": "other"}, 0.5), 1)
	// 两者都为空 → 全部
	assert.Len(t, service.filterResults(chunks, nil, 0), 3)
}

// TestRagRetrievalServiceImpl_matchFilters 过滤器匹配
func TestRagRetrievalServiceImpl_matchFilters(t *testing.T) {
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), NewMockStorage(), NewMockCache(), nil)
	chunk := Chunk{Metadata: map[string]any{"type": "faq", "num": 3, "rate": 0.9}}

	assert.True(t, service.matchFilters(chunk, map[string]any{"type": "faq"}))
	assert.False(t, service.matchFilters(chunk, map[string]any{"type": "other"}))
	assert.True(t, service.matchFilters(chunk, map[string]any{"num": 3}))
	assert.True(t, service.matchFilters(chunk, map[string]any{"rate": 0.9}))
	assert.False(t, service.matchFilters(chunk, map[string]any{"rate": 0.1}))
	assert.True(t, service.matchFilters(chunk, map[string]any{})) // 空过滤器始终匹配
}

// TestRagRetrievalServiceImpl_rankResults 按分数降序
func TestRagRetrievalServiceImpl_rankResults(t *testing.T) {
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), NewMockStorage(), NewMockCache(), nil)
	chunks := []Chunk{{Score: 0.2}, {Score: 0.9}, {Score: 0.5}}
	ranked := service.rankResults(chunks, "q")
	assert.Equal(t, []float64{0.9, 0.5, 0.2}, []float64{ranked[0].Score, ranked[1].Score, ranked[2].Score})
}

// TestRagRetrievalServiceImpl_calculateConfidence 置信度边界
func TestRagRetrievalServiceImpl_calculateConfidence(t *testing.T) {
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), NewMockStorage(), NewMockCache(), nil)
	assert.Equal(t, 1.0, service.calculateConfidence(1.0))
	assert.Equal(t, 0.0, service.calculateConfidence(0.0))
	assert.InDelta(t, 0.5, service.calculateConfidence(0.5), 0.0001)
	mid := service.calculateConfidence(0.8)
	assert.Greater(t, mid, 0.5)
	assert.Less(t, mid, 1.0)
}
