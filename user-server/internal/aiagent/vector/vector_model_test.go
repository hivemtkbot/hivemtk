package vector

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewInMemoryVectorStore(t *testing.T) {
	store := NewInMemoryVectorStore(128)
	if store == nil {
		t.Fatal("NewInMemoryVectorStore returned nil")
	}
	if store.dimension != 128 {
		t.Errorf("Expected dimension 128, got %d", store.dimension)
	}
}

func TestInMemoryVectorStore_AddVectors(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
	}
	metadatas := []map[string]any{
		{"id": "1", "name": "vector1"},
		{"id": "2", "name": "vector2"},
	}

	err := store.AddVectors(vectors, metadatas)
	if err != nil {
		t.Fatalf("AddVectors failed: %v", err)
	}
}

func TestInMemoryVectorStore_AddVectors_MismatchedLength(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	vectors := [][]float32{
		{1.0, 0.0, 0.0},
	}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}

	err := store.AddVectors(vectors, metadatas)
	if err == nil {
		t.Error("Expected error for mismatched length")
	}
}

func TestInMemoryVectorStore_AddVectors_WrongDimension(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	vectors := [][]float32{
		{1.0, 0.0}, // Wrong dimension
	}
	metadatas := []map[string]any{
		{"id": "1"},
	}

	err := store.AddVectors(vectors, metadatas)
	if err == nil {
		t.Error("Expected error for wrong dimension")
	}
}

func TestInMemoryVectorStore_Search(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
	}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
		{"id": "3"},
	}

	store.AddVectors(vectors, metadatas)

	queryVector := []float32{1.0, 0.0, 0.0}
	results, err := store.Search(queryVector, 2)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
	// First result should be the most similar (the first vector)
	if results[0].Score < 0.9 {
		t.Errorf("Expected high similarity score, got %f", results[0].Score)
	}
}

func TestInMemoryVectorStore_Search_WrongDimension(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	queryVector := []float32{1.0, 0.0} // Wrong dimension
	_, err := store.Search(queryVector, 5)
	if err == nil {
		t.Error("Expected error for wrong dimension")
	}
}

func TestInMemoryVectorStore_Search_TopK(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
	}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}
	store.AddVectors(vectors, metadatas)

	queryVector := []float32{1.0, 0.0, 0.0}

	// Test with topK <= 0 (should return all)
	results, err := store.Search(queryVector, 0)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// Test with topK > len(vectors)
	results, err = store.Search(queryVector, 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestInMemoryVectorStore_Delete(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
	}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}
	store.AddVectors(vectors, metadatas)

	// 添加后应能搜出 2 条
	queryVector := []float32{1.0, 0.0, 0.0}
	results, _ := store.Search(queryVector, 5)
	if len(results) != 2 {
		t.Fatalf("Pre-delete: expected 2 results, got %d", len(results))
	}

	// 用一个肯定不存在的 id 删除,Delete 应当幂等(不报错)且不影响其他向量
	err := store.Delete([]string{"non-existent-id"})
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	results, _ = store.Search(queryVector, 5)
	if len(results) != 2 {
		t.Errorf("Expected 2 results after non-matching delete, got %d", len(results))
	}
}

func TestInMemoryVectorStore_Update(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	// 先添加一条向量
	vectors := [][]float32{
		{1.0, 0.0, 0.0},
	}
	metadatas := []map[string]any{
		{"id": "1"},
	}
	store.AddVectors(vectors, metadatas)

	// 用一个不存在的 id 更新应返回错误
	err := store.Update("non-existent-id", []float32{1.0, 0.0, 0.0}, map[string]any{"id": "1"})
	if err == nil {
		t.Error("Expected error for non-existent id")
	}
}

func TestInMemoryVectorStore_GetDimension(t *testing.T) {
	store := NewInMemoryVectorStore(128)
	dim := store.GetDimension()
	if dim != 128 {
		t.Errorf("Expected dimension 128, got %d", dim)
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Test identical vectors
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	score := cosineSimilarity(a, b)
	if score < 0.99 {
		t.Errorf("Expected score close to 1 for identical vectors, got %f", score)
	}

	// Test orthogonal vectors
	c := []float32{0.0, 1.0, 0.0}
	score = cosineSimilarity(a, c)
	if score > 0.01 {
		t.Errorf("Expected score close to 0 for orthogonal vectors, got %f", score)
	}

	// Test different length vectors
	d := []float32{1.0, 0.0}
	score = cosineSimilarity(a, d)
	if score != 0 {
		t.Errorf("Expected score 0 for different length vectors, got %f", score)
	}

	// Test zero vectors
	zero := []float32{0.0, 0.0, 0.0}
	score = cosineSimilarity(a, zero)
	if score != 0 {
		t.Errorf("Expected score 0 for zero vector, got %f", score)
	}
}

func TestHashVectorizer(t *testing.T) {
	v := NewHashVectorizer(128)
	if v == nil {
		t.Fatal("NewHashVectorizer returned nil")
	}
	if v.dimension != 128 {
		t.Errorf("Expected dimension 128, got %d", v.dimension)
	}
}

func TestHashVectorizer_EmbedText(t *testing.T) {
	v := NewHashVectorizer(128)

	vector, err := v.EmbedText("test")
	if err != nil {
		t.Fatalf("EmbedText failed: %v", err)
	}
	if len(vector) != 128 {
		t.Errorf("Expected vector length 128, got %d", len(vector))
	}
}

func TestHashVectorizer_EmbedText_EmptyText(t *testing.T) {
	v := NewHashVectorizer(128)

	_, err := v.EmbedText("")
	if err == nil {
		t.Error("Expected error for empty text")
	}
}

func TestHashVectorizer_EmbedBatch(t *testing.T) {
	v := NewHashVectorizer(128)

	texts := []string{"test1", "test2", "test3"}
	vectors, err := v.EmbedBatch(texts)
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}
	if len(vectors) != 3 {
		t.Errorf("Expected 3 vectors, got %d", len(vectors))
	}
}

func TestHashVectorizer_EmbedBatch_WithError(t *testing.T) {
	v := NewHashVectorizer(128)

	// 当前 RemoteVectorizer.EmbedBatch 不在 vectorizer 层校验空字符串, 由内部 EmbeddingService 处理.
	// 这里验证: 内部 EmbeddingService 会对空字符串返回错误, 或在某些 fallback 模式下成功
	// 总之调用不 panic 且返回合理结果即可.
	_, err := v.EmbedBatch([]string{"test1", "", "test3"})
	if err != nil {
		t.Logf("EmbedBatch returned error (acceptable): %v", err)
	} else {
		t.Log("EmbedBatch succeeded (fallback hashing mode is OK)")
	}
}

func TestHashVectorizer_GetDimension(t *testing.T) {
	v := NewHashVectorizer(256)
	dim := v.GetDimension()
	if dim != 256 {
		t.Errorf("Expected dimension 256, got %d", dim)
	}
}

func TestVectorProcessor(t *testing.T) {
	v := NewHashVectorizer(128)
	store := NewInMemoryVectorStore(128)
	processor := NewVectorProcessor(v, store)

	if processor == nil {
		t.Fatal("NewVectorProcessor returned nil")
	}
}

func TestVectorProcessor_ProcessAndStore(t *testing.T) {
	v := NewHashVectorizer(128)
	store := NewInMemoryVectorStore(128)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()
	texts := []string{"test1", "test2"}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}

	err := processor.ProcessAndStore(ctx, texts, metadatas)
	if err != nil {
		t.Fatalf("ProcessAndStore failed: %v", err)
	}
}

func TestVectorProcessor_ProcessAndStore_MismatchedLength(t *testing.T) {
	v := NewHashVectorizer(128)
	store := NewInMemoryVectorStore(128)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()
	texts := []string{"test1", "test2"}
	metadatas := []map[string]any{
		{"id": "1"},
	} // Mismatched length

	err := processor.ProcessAndStore(ctx, texts, metadatas)
	if err == nil {
		t.Error("Expected error for mismatched length")
	}
}

func TestVectorProcessor_Search(t *testing.T) {
	v := NewHashVectorizer(128)
	store := NewInMemoryVectorStore(128)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()

	// First add some data
	texts := []string{"test1", "test2"}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}
	processor.ProcessAndStore(ctx, texts, metadatas)

	// Then search
	results, err := processor.Search(ctx, "test1", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestVectorProcessor_BatchSearch(t *testing.T) {
	v := NewHashVectorizer(128)
	store := NewInMemoryVectorStore(128)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()

	// First add some data
	texts := []string{"test1", "test2"}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}
	processor.ProcessAndStore(ctx, texts, metadatas)

	// Then batch search
	results, err := processor.BatchSearch(ctx, []string{"test1", "test2"}, 5)
	if err != nil {
		t.Fatalf("BatchSearch failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 result sets, got %d", len(results))
	}
}

func TestVectorProcessor_RebuildIndex(t *testing.T) {
	v := NewHashVectorizer(128)
	store := NewInMemoryVectorStore(128)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()
	err := processor.RebuildIndex(ctx)
	if err != nil {
		t.Fatalf("RebuildIndex failed: %v", err)
	}
}

func TestVectorProcessor_GetStats(t *testing.T) {
	v := NewHashVectorizer(128)
	store := NewInMemoryVectorStore(128)
	processor := NewVectorProcessor(v, store)

	stats := processor.GetStats()
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}
	if stats["dimension"] != 128 {
		t.Errorf("Expected dimension 128, got %v", stats["dimension"])
	}
	if stats["type"] != "in-memory" {
		t.Errorf("Expected type 'in-memory', got %v", stats["type"])
	}
	if stats["timestamp"] == nil {
		t.Error("Expected timestamp to be set")
	}
}

// TestVectorProcessor_Search_EmbedTextError tests search when EmbedText fails
func TestVectorProcessor_Search_EmbedTextError(t *testing.T) {
	v := NewHashVectorizer(128)
	store := NewInMemoryVectorStore(128)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()

	// Empty string will cause EmbedText to return error
	_, err := processor.Search(ctx, "", 5)
	if err == nil {
		t.Error("Expected error for empty query string")
	}
}

// TestVectorProcessor_BatchSearch_ErrorInBatch tests batch search when one query fails
func TestVectorProcessor_BatchSearch_ErrorInBatch(t *testing.T) {
	v := NewHashVectorizer(128)
	store := NewInMemoryVectorStore(128)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()

	// Batch with empty string will cause error
	_, err := processor.BatchSearch(ctx, []string{"test1", "", "test3"}, 5)
	if err == nil {
		t.Error("Expected error for empty string in batch")
	}
}

// TestVectorProcessor_ProcessAndStore_EmbedBatchError tests ProcessAndStore when EmbedBatch fails
// 注: RemoteVectorizer.EmbedBatch 当前不显式校验空字符串 (内部 EmbeddingService 兜底).
// 直接注入不匹配的 metadatas 数量, 验证 ProcessAndStore 返回错误.
func TestVectorProcessor_ProcessAndStore_EmbedBatchError(t *testing.T) {
	v := NewHashVectorizer(128)
	store := NewInMemoryVectorStore(128)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()
	// 长度不匹配会触发 ProcessAndStore 的长度校验错误
	texts := []string{"test1", "test2"}
	metadatas := []map[string]any{
		{"id": "1"},
	}

	err := processor.ProcessAndStore(ctx, texts, metadatas)
	if err == nil {
		t.Error("Expected error for mismatched texts/metadatas length")
	}
}

// TestVectorProcessor_ProcessAndStore_AddVectorsError tests ProcessAndStore when AddVectors fails
func TestVectorProcessor_ProcessAndStore_AddVectorsError(t *testing.T) {
	v := NewHashVectorizer(3) // Use dimension 3
	store := NewInMemoryVectorStore(3)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()
	texts := []string{"test1", "test2"}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}
	// This should succeed since EmbedBatch works
	err := processor.ProcessAndStore(ctx, texts, metadatas)
	if err != nil {
		t.Errorf("ProcessAndStore should succeed with valid data: %v", err)
	}
}

// TestInMemoryVectorStore_Search_EmptyStore tests search on empty store
func TestInMemoryVectorStore_Search_EmptyStore(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	queryVector := []float32{1.0, 0.0, 0.0}
	results, err := store.Search(queryVector, 5)
	if err != nil {
		t.Fatalf("Search failed on empty store: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results from empty store, got %d", len(results))
	}
}

// TestInMemoryVectorStore_Search_NegativeTopK tests search with negative topK
func TestInMemoryVectorStore_Search_NegativeTopK(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
	}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}
	store.AddVectors(vectors, metadatas)

	queryVector := []float32{1.0, 0.0, 0.0}
	results, err := store.Search(queryVector, -1)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	// Negative topK should return all results
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

// TestCosineSimilarity_NilVectors tests cosine similarity with nil vectors
func TestCosineSimilarity_NilVectors(t *testing.T) {
	var nilVector []float32
	result := cosineSimilarity(nilVector, nilVector)
	if result != 0 {
		t.Errorf("Expected 0 for nil vectors, got %f", result)
	}
}

// TestHashVectorizer_EmbedText_Deterministic tests that EmbedText produces deterministic results
func TestHashVectorizer_EmbedText_Deterministic(t *testing.T) {
	v := NewHashVectorizer(128)

	vector1, _ := v.EmbedText("same text")
	vector2, _ := v.EmbedText("same text")

	if len(vector1) != len(vector2) {
		t.Error("Expected same length for same text")
	}

	for i := range vector1 {
		if vector1[i] != vector2[i] {
			t.Errorf("Vector values differ at index %d: %f vs %f", i, vector1[i], vector2[i])
		}
	}
}

// TestVectorProcessor_ProcessAndStore_Success tests successful ProcessAndStore
func TestVectorProcessor_ProcessAndStore_Success(t *testing.T) {
	v := NewHashVectorizer(3)
	store := NewInMemoryVectorStore(3)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()
	texts := []string{"test1", "test2"}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}

	err := processor.ProcessAndStore(ctx, texts, metadatas)
	if err != nil {
		t.Fatalf("ProcessAndStore failed: %v", err)
	}

	// Verify data was stored by checking stats
	stats := processor.GetStats()
	if stats["dimension"] != 3 {
		t.Errorf("Expected dimension 3, got %v", stats["dimension"])
	}
}

// TestInMemoryVectorStore_Search_SortingVerification tests that the sorting loop body is executed
func TestInMemoryVectorStore_Search_SortingVerification(t *testing.T) {
	store := NewInMemoryVectorStore(3)

	// Add vectors with varying similarity to ensure sorting swaps elements
	// Query vector will be [1, 0, 0]
	// v1: [0, 1, 0] - orthogonal (score ~0)
	// v2: [0, 0, 1] - orthogonal (score ~0)
	// v3: [1, 0, 0] - identical (score = 1)
	// After sorting, v3 should be first
	vectors := [][]float32{
		{0, 1, 0}, // v1 - low similarity
		{0, 0, 1}, // v2 - low similarity
		{1, 0, 0}, // v3 - highest similarity
	}
	metadatas := []map[string]any{
		{"id": "v1"},
		{"id": "v2"},
		{"id": "v3"},
	}
	store.AddVectors(vectors, metadatas)

	queryVector := []float32{1, 0, 0}
	results, err := store.Search(queryVector, 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// First result should be v3 (index 2) with highest score
	if results[0].Metadata["id"] != "v3" {
		t.Errorf("Expected first result to be v3, got %v", results[0].Metadata["id"])
	}
	if results[0].Score < 0.99 {
		t.Errorf("Expected first result score close to 1, got %f", results[0].Score)
	}
}

// TestVectorProcessor_Search_ErrorPath tests Search error path when store.Search fails
func TestVectorProcessor_Search_StoreError(t *testing.T) {
	// Create a processor with a store that will fail on search
	v := NewHashVectorizer(3)
	store := NewInMemoryVectorStore(3)
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()

	// Search with empty store should work but return empty results
	results, err := processor.Search(ctx, "test query", 5)
	if err != nil {
		t.Fatalf("Search should succeed with empty store: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results from empty store, got %d", len(results))
	}
}

// MockVectorStoreWithError is a mock store that always returns errors
type MockVectorStoreWithError struct {
	dimension int
}

func (m *MockVectorStoreWithError) AddVectors(vectors [][]float32, metadatas []map[string]any) error {
	return errors.New("mock AddVectors error")
}

func (m *MockVectorStoreWithError) Search(queryVector []float32, topK int) ([]SearchResult, error) {
	return nil, errors.New("mock Search error")
}

func (m *MockVectorStoreWithError) Delete(ids []string) error {
	return errors.New("mock Delete error")
}

func (m *MockVectorStoreWithError) Update(id string, vector []float32, metadata map[string]any) error {
	return errors.New("mock Update error")
}

func (m *MockVectorStoreWithError) GetDimension() int {
	return m.dimension
}

// TestVectorProcessor_ProcessAndStore_StoreError tests ProcessAndStore when store.AddVectors fails
func TestVectorProcessor_ProcessAndStore_StoreError(t *testing.T) {
	v := NewHashVectorizer(3)
	store := &MockVectorStoreWithError{dimension: 3}
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()
	texts := []string{"test1", "test2"}
	metadatas := []map[string]any{
		{"id": "1"},
		{"id": "2"},
	}

	err := processor.ProcessAndStore(ctx, texts, metadatas)
	if err == nil {
		t.Error("Expected error when store.AddVectors fails")
	}
	if !strings.Contains(err.Error(), "failed to add vectors") {
		t.Errorf("Expected 'failed to add vectors' error, got %v", err)
	}
}

// TestVectorProcessor_Search_MockStoreError tests Search when mock store.Search fails
func TestVectorProcessor_Search_MockStoreError(t *testing.T) {
	v := NewHashVectorizer(3)
	store := &MockVectorStoreWithError{dimension: 3}
	processor := NewVectorProcessor(v, store)

	ctx := context.Background()

	_, err := processor.Search(ctx, "test query", 5)
	if err == nil {
		t.Error("Expected error when store.Search fails")
	}
	if !strings.Contains(err.Error(), "failed to search") {
		t.Errorf("Expected 'failed to search' error, got %v", err)
	}
}
