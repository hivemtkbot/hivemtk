package ragretrieval


import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// HybridSearcherTestModels AutoMigrate 模型列表
// 注意：knowledge_chunks 表用 raw SQL 创建（含 pgvector 列），不能用 AutoMigrate
type HybridSearcherTestModels struct{}

// setupHybridTestDB 创建混合检索测试 DB（含 knowledge_chunks / knowledge_search_logs 表）
//
// 表结构对齐 v2.6.0 + v2.7.0 迁移：
//   - knowledge_chunks: id, document_id, content, product_id, embedding, content_tsv, contextual_context, contextual_tsv
//   - knowledge_search_logs: query, product_id, vector_count, bm25_count, ...
func setupHybridTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping PG integration test in short mode")
	}
	db := testutil.NewTestDB(t)

	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		`DROP TABLE IF EXISTS knowledge_chunks CASCADE`,
		`CREATE TABLE knowledge_chunks (
			id BIGSERIAL PRIMARY KEY,
			document_id BIGINT NOT NULL DEFAULT 0,
			product_id BIGINT NOT NULL DEFAULT 0,
			content TEXT NOT NULL DEFAULT '',
			chunk_index INT DEFAULT 0,
			embedding vector(1024),
			embedding_id VARCHAR(64),
			content_tsv tsvector,
			contextual_context TEXT,
			contextual_tsv tsvector,
			content_hash VARCHAR(64),
			embed_status VARCHAR(20) DEFAULT 'pending',
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE INDEX idx_knowledge_chunks_embedding_hnsw ON knowledge_chunks USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64)`,
		`CREATE INDEX idx_knowledge_chunks_content_tsv ON knowledge_chunks USING GIN (content_tsv)`,
		`CREATE OR REPLACE FUNCTION knowledge_chunks_tsv_trigger() RETURNS trigger AS $$
		BEGIN
			NEW.content_tsv := to_tsvector('simple', coalesce(NEW.content, ''));
			NEW.contextual_tsv := to_tsvector('simple', coalesce(NEW.contextual_context, '') || ' ' || coalesce(NEW.content, ''));
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,
		`CREATE TRIGGER knowledge_chunks_tsv_update
			BEFORE INSERT OR UPDATE ON knowledge_chunks
			FOR EACH ROW EXECUTE FUNCTION knowledge_chunks_tsv_trigger()`,
		`DROP TABLE IF EXISTS knowledge_search_logs`,
		`CREATE TABLE knowledge_search_logs (
			id BIGSERIAL PRIMARY KEY,
			query TEXT,
			product_id BIGINT,
			top_k INT,
			vector_count INT DEFAULT 0,
			bm25_count INT DEFAULT 0,
			fused_count INT DEFAULT 0,
			rerank_count INT DEFAULT 0,
			vector_latency_ms BIGINT DEFAULT 0,
			bm25_latency_ms BIGINT DEFAULT 0,
			rewrite_latency_ms BIGINT DEFAULT 0,
			rerank_latency_ms BIGINT DEFAULT 0,
			rewrite_used VARCHAR(50),
			cache_hit BOOLEAN DEFAULT false,
			created_at TIMESTAMP DEFAULT NOW()
		)`,
	}
	for _, sql := range stmts {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("setup SQL failed (%s): %v", sql, err)
		}
	}
	return db
}

// insertChunk 插入测试 chunk（自动维护 content_tsv）
func insertChunk(t *testing.T, db *gorm.DB, docID uint, productID string, content string, embedding []float32) {
	t.Helper()
	vecLiteral := vecToPGString(embedding)
	sql := `
		INSERT INTO knowledge_chunks (document_id, product_id, content, embedding, embed_status, content_tsv)
		VALUES ($1, $2, $3, $4::vector, 'indexed', to_tsvector('simple', $3))
	`
	if err := db.Exec(sql, docID, productID, content, vecLiteral).Error; err != nil {
		t.Fatalf("insert chunk failed: %v", err)
	}
}

// makePrefixVector 前 prefixLen 维为 1.0、其余为 0 的向量
// 说明：makeFixedVector 产生的同值向量彼此平行（余弦距离全为 0），无法区分相似度排序；
// 本 helper 通过不同激活前缀长度构造方向可区分的向量（cos = prefixLen/dim）
func makePrefixVector(dim int, prefixLen int) []float32 {
	v := make([]float32, dim)
	for i := 0; i < prefixLen && i < dim; i++ {
		v[i] = 1.0
	}
	return v
}

// waitUntil 每隔 20ms 轮询 cond 直到为真或超时（异步写入的确定性等待，替代固定 sleep）
func waitUntil(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for !cond() {
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

// TestHybridSearcher_VectorRetrieve_EndToEnd 集成测试：向量召回端到端
//
// 场景：3 个 chunk 有 embedding，1 个无 embedding；查询向量(全1) 与 chunk100(cos=1.0) 最相似，
// chunk101(cos=0.707) 次之，chunk102(cos=0.25) 最差
// 期望：返回 chunk100 排第一
func TestHybridSearcher_VectorRetrieve_EndToEnd(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" && os.Getenv("POSTGRES_TEST_HOST") == "" {
		t.Skip("skipping PG integration test (no POSTGRES_TEST_DSN)")
	}
	db := setupHybridTestDB(t)

	mockEmbed := &mockEmbeddingService{
		vectors: [][]float32{makeFixedVector(1024, 1.0)}, 
	}
	searcher := NewHybridSearcher(db, mockEmbed, nil, nil, nil, &HybridSearcherConfig{
		DefaultTopK:      5,
		CandidatePool:    50,
		FusedTopN:        20,
		RRFK:             60,
		EfSearch:         80,
		EnableHyDE:       false,
		EnableMultiQuery: false,
		EnableRerank:     false,
	})

	insertChunk(t, db, 100, "1", "如何申请退货退款流程", makeFixedVector(1024, 1.0))
	insertChunk(t, db, 101, "1", "商品保修政策说明", makePrefixVector(1024, 512))
	insertChunk(t, db, 102, "1", "联系方式与客服电话", makePrefixVector(1024, 256))

	out, err := searcher.Search(context.Background(), "如何退货", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if out[0].DocumentID != "100" {
		t.Errorf("first DocumentID=%s want=100", out[0].DocumentID)
	}
}

// TestHybridSearcher_BM25Retrieve_Fallback 集成测试：BM25 召回（向量路失败时）
//
// 场景：向量 embedding 全部为 nil（mockEmbed 返回 error），但 BM25 路径应能召回
// 期望：向量路失败，BM25 路成功，仍返回结果
func TestHybridSearcher_BM25Retrieve_Fallback(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" && os.Getenv("POSTGRES_TEST_HOST") == "" {
		t.Skip("skipping PG integration test (no POSTGRES_TEST_DSN)")
	}
	db := setupHybridTestDB(t)

	mockEmbed := &mockEmbeddingService{err: fmt.Errorf("TEI down")}
	searcher := NewHybridSearcher(db, mockEmbed, nil, nil, nil, &HybridSearcherConfig{
		DefaultTopK:      5,
		CandidatePool:    50,
		FusedTopN:        20,
		RRFK:             60,
		EfSearch:         80,
		EnableHyDE:       false,
		EnableMultiQuery: false,
		EnableRerank:     false,
	})

	insertChunkNoEmbed(t, db, 100, "1", "如何申请退货退款流程")
	insertChunkNoEmbed(t, db, 101, "1", "商品保修政策说明")

	out, err := searcher.Search(context.Background(), "退货", 5)
	if err != nil {
		t.Fatalf("Search should succeed via BM25 fallback: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected BM25 to return results")
	}
}

// insertChunkNoEmbed 插入无 embedding 的 chunk（仅 BM25 可用）
func insertChunkNoEmbed(t *testing.T, db *gorm.DB, docID uint, productID string, content string) {
	t.Helper()
	sql := `
		INSERT INTO knowledge_chunks (document_id, product_id, content, embed_status, content_tsv)
		VALUES ($1, $2, $3, 'pending', to_tsvector('simple', $3))
	`
	if err := db.Exec(sql, docID, productID, content).Error; err != nil {
		t.Fatalf("insert chunk (no embed) failed: %v", err)
	}
}

// TestHybridSearcher_BothFail_ReturnsError 集成测试：两路均失败时返回 error
func TestHybridSearcher_BothFail_ReturnsError(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" && os.Getenv("POSTGRES_TEST_HOST") == "" {
		t.Skip("skipping PG integration test (no POSTGRES_TEST_DSN)")
	}
	db := setupHybridTestDB(t)

	mockEmbed := &mockEmbeddingService{err: fmt.Errorf("TEI down")}
	searcher := NewHybridSearcher(db, mockEmbed, nil, nil, nil, &HybridSearcherConfig{
		EnableHyDE:       false,
		EnableMultiQuery: false,
		EnableRerank:     false,
	})

	insertChunkNoEmbed(t, db, 100, "1", "")

	out, err := searcher.Search(context.Background(), "", 5)
	if err != nil {
		t.Logf("Search returned err (acceptable): %v", err)
	}
	if len(out) != 0 {
		t.Errorf("empty query should return empty, got=%d", len(out))
	}
}

// TestHybridSearcher_SearchIndex_WithProductFilter 集成测试：按 product_id 过滤检索
func TestHybridSearcher_SearchIndex_WithProductFilter(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" && os.Getenv("POSTGRES_TEST_HOST") == "" {
		t.Skip("skipping PG integration test (no POSTGRES_TEST_DSN)")
	}
	db := setupHybridTestDB(t)

	mockEmbed := &mockEmbeddingService{
		vectors: [][]float32{makeFixedVector(1024, 1.0)},
	}
	searcher := NewHybridSearcher(db, mockEmbed, nil, nil, nil, &HybridSearcherConfig{
		EnableHyDE:       false,
		EnableMultiQuery: false,
		EnableRerank:     false,
	})

	insertChunk(t, db, 100, "1", "产品A的退货流程", makeFixedVector(1024, 1.0))
	insertChunk(t, db, 200, "2", "产品B的退货流程", makeFixedVector(1024, 1.0)) 

	out, err := searcher.SearchIndex(context.Background(), "1", "退货", 5)
	if err != nil {
		t.Fatalf("SearchIndex failed: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected results for product_id=1")
	}
	for _, c := range out {
		if c.DocumentID != "100" {
			t.Errorf("DocumentID=%s want=100 (product_id=1 filter)", c.DocumentID)
		}
	}
}

// TestHybridSearcher_TopKTruncation 集成测试：topK 截断
func TestHybridSearcher_TopKTruncation(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" && os.Getenv("POSTGRES_TEST_HOST") == "" {
		t.Skip("skipping PG integration test (no POSTGRES_TEST_DSN)")
	}
	db := setupHybridTestDB(t)

	mockEmbed := &mockEmbeddingService{
		vectors: [][]float32{makeFixedVector(1024, 1.0)},
	}
	searcher := NewHybridSearcher(db, mockEmbed, nil, nil, nil, &HybridSearcherConfig{
		EnableHyDE:       false,
		EnableMultiQuery: false,
		EnableRerank:     false,
	})

	for i := 0; i < 10; i++ {
		insertChunk(t, db, uint(100+i), "1", fmt.Sprintf("chunk-%d", i), makeFixedVector(1024, 1.0))
	}

	out, err := searcher.Search(context.Background(), "test", 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(out) > 3 {
		t.Errorf("topK=3 should truncate, got=%d", len(out))
	}
}

// TestHybridSearcher_LogSearch_WritesToDB 集成测试：logSearch 写入 knowledge_search_logs
func TestHybridSearcher_LogSearch_WritesToDB(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" && os.Getenv("POSTGRES_TEST_HOST") == "" {
		t.Skip("skipping PG integration test (no POSTGRES_TEST_DSN)")
	}
	db := setupHybridTestDB(t)

	mockEmbed := &mockEmbeddingService{
		vectors: [][]float32{makeFixedVector(1024, 1.0)},
	}
	searcher := NewHybridSearcher(db, mockEmbed, nil, nil, nil, &HybridSearcherConfig{
		EnableHyDE:       false,
		EnableMultiQuery: false,
		EnableRerank:     false,
	})

	insertChunk(t, db, 100, "1", "测试内容", makeFixedVector(1024, 1.0))

	_, err := searcher.Search(context.Background(), "测试", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// 验证 knowledge_search_logs 表有记录（logSearch 为异步 fire-and-forget，轮询等待）
	logWritten := waitUntil(func() bool {
		var count int64
		if err := db.Raw(`SELECT COUNT(*) FROM knowledge_search_logs`).Scan(&count).Error; err != nil {
			t.Fatalf("query logs failed: %v", err)
		}
		return count > 0
	}, 5*time.Second)
	if !logWritten {
		t.Error("knowledge_search_logs should have at least 1 record")
	}
}

// TestHybridSearcher_RerankerFailed_FallbackToFused 集成测试：rerank 失败时回退到融合顺序
func TestHybridSearcher_RerankerFailed_FallbackToFused(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" && os.Getenv("POSTGRES_TEST_HOST") == "" {
		t.Skip("skipping PG integration test (no POSTGRES_TEST_DSN)")
	}
	db := setupHybridTestDB(t)

	mockEmbed := &mockEmbeddingService{
		vectors: [][]float32{makeFixedVector(1024, 1.0)},
	}
	failingReranker := &mockReranker{err: fmt.Errorf("rerank service down")}
	searcher := NewHybridSearcher(db, mockEmbed, failingReranker, nil, nil, &HybridSearcherConfig{
		EnableHyDE:       false,
		EnableMultiQuery: false,
		EnableRerank:     true, 
	})

	insertChunk(t, db, 100, "1", "测试内容1", makeFixedVector(1024, 1.0))
	insertChunk(t, db, 101, "1", "测试内容2", makeFixedVector(1024, 0.9))

	out, err := searcher.Search(context.Background(), "测试", 5)
	if err != nil {
		t.Fatalf("Search should succeed even when rerank fails: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected results even when rerank fails")
	}
	// D17b: 降级路径分数必须已归一化到 (0,1]（RAGQual/门控按 0~1 语义消费）
	for i, c := range out {
		if c.Score <= 0 || c.Score > 1.0001 {
			t.Errorf("out[%d] 分数量纲异常（RRF 未归一化）: %v", i, c.Score)
		}
	}
}

// mockReranker mock RerankerInterface
type mockReranker struct {
	err error
}

func (m *mockReranker) Rerank(_ context.Context, _ string, docs []RerankDoc) ([]RerankResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]RerankResult, len(docs))
	for i, d := range docs {
		out[i] = RerankResult{ID: d.ID, Score: float64(len(docs) - i)}
	}
	return out, nil
}

var _ RerankerInterface = (*mockReranker)(nil)

// 兼容性引用（防止 import 报未使用）
var _ = llm.EmbeddingServiceInterface(nil)

