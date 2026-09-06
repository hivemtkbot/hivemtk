package ragretrieval

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestInMemoryStorage_DocumentCRUD 文档存取删
func TestInMemoryStorage_DocumentCRUD(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	err := s.SaveDocument(ctx, "kb1", Document{ID: "d1", Content: "内容1"})
	assert.NoError(t, err)

	doc, err := s.GetDocument(ctx, "kb1", "d1")
	assert.NoError(t, err)
	assert.Equal(t, "内容1", doc.Content)

	_, err = s.GetDocument(ctx, "nope", "d1")
	assert.Error(t, err)
	_, err = s.GetDocument(ctx, "kb1", "nope")
	assert.Error(t, err)

	docs, err := s.ListDocuments(ctx, "kb1")
	assert.NoError(t, err)
	assert.Len(t, docs, 1)

	assert.NoError(t, s.DeleteDocument(ctx, "kb1", "d1"))
	assert.Len(t, s.documents["kb1"], 0)

	assert.Error(t, s.DeleteDocument(ctx, "nope", "d1"))
}

// TestInMemoryStorage_KnowledgeBaseCRUD 知识库存取删
func TestInMemoryStorage_KnowledgeBaseCRUD(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	assert.NoError(t, s.SaveKnowledgeBase(ctx, KnowledgeBaseInfo{ID: "kb1", Name: "KB1"}))
	info, err := s.GetKnowledgeBase(ctx, "kb1")
	assert.NoError(t, err)
	assert.Equal(t, "KB1", info.Name)

	_, err = s.GetKnowledgeBase(ctx, "nope")
	assert.Error(t, err)

	assert.NoError(t, s.DeleteKnowledgeBase(ctx, "kb1"))
	assert.Error(t, s.DeleteKnowledgeBase(ctx, "nope"))
}

// TestInMemoryStorage_ListKnowledgeBases 按 owner/public 过滤
func TestInMemoryStorage_ListKnowledgeBases(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	assert.NoError(t, s.SaveKnowledgeBase(ctx, KnowledgeBaseInfo{ID: "k1", OwnerID: "u1", IsPublic: false}))
	assert.NoError(t, s.SaveKnowledgeBase(ctx, KnowledgeBaseInfo{ID: "k2", OwnerID: "u1", IsPublic: true}))
	assert.NoError(t, s.SaveKnowledgeBase(ctx, KnowledgeBaseInfo{ID: "k3", OwnerID: "u2", IsPublic: true}))

	list, err := s.ListKnowledgeBases(ctx, "u1", false)
	assert.NoError(t, err)
	assert.Len(t, list, 2)

	list, err = s.ListKnowledgeBases(ctx, "u1", true)
	assert.NoError(t, err)
	assert.Len(t, list, 3)
}

// TestInMemoryCache_Basic 基本存取与过期
func TestInMemoryCache_Basic(t *testing.T) {
	ctx := context.Background()
	c := NewInMemoryCache()

	c.Set("k", "v", time.Minute)
	val, ok := c.Get("k")
	assert.True(t, ok)
	assert.Equal(t, "v", val)

	_, ok = c.Get("missing")
	assert.False(t, ok)

	c.Delete("k")
	_, ok = c.Get("k")
	assert.False(t, ok)

	c.Set("e", "x", 1*time.Nanosecond)
	time.Sleep(2 * time.Millisecond)
	_, ok = c.Get("e")
	assert.False(t, ok)
	_ = ctx
}

// TestNewDefaultRagRetrievalService 默认构造（真实 EmbeddingService + 内存索引/存储/缓存）
func TestNewDefaultRagRetrievalService(t *testing.T) {
	svc := NewDefaultRagRetrievalService()
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.config)
	assert.Equal(t, 5, svc.config.DefaultTopK)
	assert.Equal(t, 0.5, svc.config.DefaultSimilarityThreshold)
	assert.NotNil(t, svc.vectorizer)
	assert.NotNil(t, svc.indexer)
	assert.NotNil(t, svc.storage)
	assert.NotNil(t, svc.cache)
}

// TestNewProductionRagRetrievalService 生产构造包装
func TestNewProductionRagRetrievalService(t *testing.T) {
	svc := NewProductionRagRetrievalService(
		&constantVectorizer{dimension: 128},
		NewMockIndexManager(),
		NewMockStorage(),
		NewMockCache(),
		&RetrievalConfig{DefaultTopK: 5},
	)
	assert.NotNil(t, svc)
	assert.Equal(t, 5, svc.config.DefaultTopK)
}
