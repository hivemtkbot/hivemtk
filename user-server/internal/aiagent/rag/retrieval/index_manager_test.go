package ragretrieval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestInMemoryIndexManager_NewInMemoryIndexManager tests the constructor
func TestInMemoryIndexManager_NewInMemoryIndexManager(t *testing.T) {
	manager := NewInMemoryIndexManager(128)
	assert.NotNil(t, manager)
	assert.Equal(t, 128, manager.dimension)

	managerDefault := NewInMemoryIndexManager(0)
	assert.NotNil(t, managerDefault)
	assert.Equal(t, 1024, managerDefault.dimension)

	managerNegative := NewInMemoryIndexManager(-100)
	assert.NotNil(t, managerNegative)
	assert.Equal(t, 1024, managerNegative.dimension)
}

// TestInMemoryIndexManager_BuildIndex tests building an index
func TestInMemoryIndexManager_BuildIndex(t *testing.T) {
	ctx := context.Background()
	manager := NewInMemoryIndexManager(3)

	tests := []struct {
		name        string
		kbID        string
		chunks      []Chunk
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty_kbID",
			kbID:        "",
			chunks:      []Chunk{{ID: "chunk1", Content: "test"}},
			expectError: true,
			errorMsg:    "kbID cannot be empty",
		},
		{
			name:        "empty_chunks",
			kbID:        "test_kb",
			chunks:      []Chunk{},
			expectError: true,
			errorMsg:    "chunks cannot be empty",
		},
		{
			name:        "invalid_embedding_dimension",
			kbID:        "test_kb",
			chunks:      []Chunk{{ID: "chunk1", Content: "test", Embedding: []float32{0.1, 0.2, 0.3, 0.4}}},
			expectError: true,
			errorMsg:    "chunk chunk1 has invalid embedding dimension",
		},
		{
			name:        "success",
			kbID:        "test_kb",
			chunks:      []Chunk{{ID: "chunk1", Content: "test content", Embedding: []float32{0.1, 0.2, 0.3}}},
			expectError: false,
		},
		{
			name:        "multiple_chunks",
			kbID:        "test_kb2",
			chunks:      []Chunk{{ID: "chunk1", Content: "test1", Embedding: []float32{0.1, 0.2, 0.3}}, {ID: "chunk2", Content: "test2", Embedding: []float32{0.4, 0.5, 0.6}}},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.BuildIndex(ctx, tt.kbID, tt.chunks)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				stats, err := manager.GetIndexStats(ctx, tt.kbID)
				assert.NoError(t, err)
				assert.Equal(t, len(tt.chunks), stats.VectorCount)
			}
		})
	}
}

// TestInMemoryIndexManager_AddToIndex tests adding chunks to an index
func TestInMemoryIndexManager_AddToIndex(t *testing.T) {
	ctx := context.Background()
	manager := NewInMemoryIndexManager(3)

	tests := []struct {
		name        string
		kbID        string
		chunk       Chunk
		setupKB     bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty_kbID",
			kbID:        "",
			chunk:       Chunk{ID: "chunk1", Content: "test", Embedding: []float32{0.1, 0.2, 0.3}},
			setupKB:     false,
			expectError: true,
			errorMsg:    "kbID cannot be empty",
		},
		{
			name:        "invalid_embedding_dimension",
			kbID:        "test_kb",
			chunk:       Chunk{ID: "chunk1", Content: "test", Embedding: []float32{0.1, 0.2, 0.3, 0.4}},
			setupKB:     false,
			expectError: true,
			errorMsg:    "chunk has invalid embedding dimension",
		},
		{
			name:        "add_to_nonexistent_kb",
			kbID:        "test_kb",
			chunk:       Chunk{ID: "chunk1", Content: "test", Embedding: []float32{0.1, 0.2, 0.3}},
			setupKB:     false,
			expectError: false,
		},
		{
			name:        "add_to_existing_kb",
			kbID:        "test_kb",
			chunk:       Chunk{ID: "chunk2", Content: "test2", Embedding: []float32{0.4, 0.5, 0.6}},
			setupKB:     true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupKB {
				err := manager.BuildIndex(ctx, tt.kbID, []Chunk{{ID: "chunk1", Content: "initial", Embedding: []float32{0.1, 0.2, 0.3}}})
				assert.NoError(t, err)
			}

			err := manager.AddToIndex(ctx, tt.kbID, tt.chunk)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				stats, err := manager.GetIndexStats(ctx, tt.kbID)
				assert.NoError(t, err)
				if tt.setupKB {
					assert.Equal(t, 2, stats.VectorCount)
				} else {
					assert.Equal(t, 1, stats.VectorCount)
				}
			}
		})
	}
}

// TestInMemoryIndexManager_RemoveFromIndex tests removing chunks from an index
func TestInMemoryIndexManager_RemoveFromIndex(t *testing.T) {
	ctx := context.Background()
	manager := NewInMemoryIndexManager(3)

	testChunks := []Chunk{
		{ID: "chunk1", Content: "test1", Embedding: []float32{0.1, 0.2, 0.3}},
		{ID: "chunk2", Content: "test2", Embedding: []float32{0.4, 0.5, 0.6}},
		{ID: "chunk3", Content: "test3", Embedding: []float32{0.7, 0.8, 0.9}},
	}
	err := manager.BuildIndex(ctx, "test_kb", testChunks)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		kbID        string
		chunkID     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty_kbID",
			kbID:        "",
			chunkID:     "chunk1",
			expectError: true,
			errorMsg:    "kbID cannot be empty",
		},
		{
			name:        "empty_chunkID",
			kbID:        "test_kb",
			chunkID:     "",
			expectError: true,
			errorMsg:    "chunkID cannot be empty",
		},
		{
			name:        "nonexistent_kb",
			kbID:        "nonexistent_kb",
			chunkID:     "chunk1",
			expectError: true,
			errorMsg:    "knowledge base nonexistent_kb does not exist",
		},
		{
			name:        "nonexistent_chunk",
			kbID:        "test_kb",
			chunkID:     "nonexistent_chunk",
			expectError: true,
			errorMsg:    "chunk nonexistent_chunk not found in knowledge base test_kb",
		},
		{
			name:        "success",
			kbID:        "test_kb",
			chunkID:     "chunk2",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.RemoveFromIndex(ctx, tt.kbID, tt.chunkID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				stats, err := manager.GetIndexStats(ctx, tt.kbID)
				assert.NoError(t, err)
				assert.Equal(t, 2, stats.VectorCount)
			}
		})
	}
}

// TestInMemoryIndexManager_SearchIndex tests searching an index
func TestInMemoryIndexManager_SearchIndex(t *testing.T) {
	ctx := context.Background()
	manager := NewInMemoryIndexManager(3)

	testChunks := []Chunk{
		{ID: "chunk1", Content: "test content 1", Embedding: []float32{0.1, 0.2, 0.3}},
		{ID: "chunk2", Content: "test content 2", Embedding: []float32{0.4, 0.5, 0.6}},
		{ID: "chunk3", Content: "test content 3", Embedding: []float32{0.7, 0.8, 0.9}},
	}
	err := manager.BuildIndex(ctx, "test_kb", testChunks)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		kbID        string
		queryVec    []float32
		topK        int
		expectError bool
		errorMsg    string
		expectedLen int
	}{
		{
			name:        "empty_kbID",
			kbID:        "",
			queryVec:    []float32{0.1, 0.2, 0.3},
			topK:        5,
			expectError: true,
			errorMsg:    "kbID cannot be empty",
		},
		{
			name:        "invalid_query_vector_dimension",
			kbID:        "test_kb",
			queryVec:    []float32{0.1, 0.2},
			topK:        5,
			expectError: true,
			errorMsg:    "query vector has invalid dimension",
		},
		{
			name:        "nonexistent_kb",
			kbID:        "nonexistent_kb",
			queryVec:    []float32{0.1, 0.2, 0.3},
			topK:        5,
			expectError: false,
			expectedLen: 0,
		},
		{
			name:        "search_with_topK",
			kbID:        "test_kb",
			queryVec:    []float32{0.1, 0.2, 0.3},
			topK:        2,
			expectError: false,
			expectedLen: 2,
		},
		{
			name:        "search_with_default_topK",
			kbID:        "test_kb",
			queryVec:    []float32{0.1, 0.2, 0.3},
			topK:        0,
			expectError: false,
			expectedLen: 3,
		},
		{
			name:        "search_with_negative_topK",
			kbID:        "test_kb",
			queryVec:    []float32{0.1, 0.2, 0.3},
			topK:        -1,
			expectError: false,
			expectedLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := manager.SearchIndex(ctx, tt.kbID, tt.queryVec, tt.topK)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.Len(t, results, tt.expectedLen)
				for i := 1; i < len(results); i++ {
					assert.GreaterOrEqual(t, results[i-1].Score, results[i].Score)
				}
			}
		})
	}
}

// TestInMemoryIndexManager_DropIndex tests dropping an index
func TestInMemoryIndexManager_DropIndex(t *testing.T) {
	ctx := context.Background()
	manager := NewInMemoryIndexManager(3)

	testChunks := []Chunk{{ID: "chunk1", Content: "test", Embedding: []float32{0.1, 0.2, 0.3}}}
	err := manager.BuildIndex(ctx, "test_kb", testChunks)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		kbID        string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty_kbID",
			kbID:        "",
			expectError: true,
			errorMsg:    "kbID cannot be empty",
		},
		{
			name:        "success",
			kbID:        "test_kb",
			expectError: false,
		},
		{
			name:        "drop_nonexistent_kb",
			kbID:        "nonexistent_kb",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.DropIndex(ctx, tt.kbID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				_, err := manager.GetIndexStats(ctx, tt.kbID)
				assert.Error(t, err)
			}
		})
	}
}

// TestInMemoryIndexManager_GetIndexStats tests getting index statistics
func TestInMemoryIndexManager_GetIndexStats(t *testing.T) {
	ctx := context.Background()
	manager := NewInMemoryIndexManager(3)

	testChunks := []Chunk{
		{ID: "chunk1", Content: "test content", Metadata: map[string]any{"key": "value"}, Embedding: []float32{0.1, 0.2, 0.3}},
		{ID: "chunk2", Content: "test content 2", Embedding: []float32{0.4, 0.5, 0.6}},
	}
	err := manager.BuildIndex(ctx, "test_kb", testChunks)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		kbID        string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty_kbID",
			kbID:        "",
			expectError: true,
			errorMsg:    "kbID cannot be empty",
		},
		{
			name:        "nonexistent_kb",
			kbID:        "nonexistent_kb",
			expectError: true,
			errorMsg:    "knowledge base nonexistent_kb does not exist",
		},
		{
			name:        "success",
			kbID:        "test_kb",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats, err := manager.GetIndexStats(ctx, tt.kbID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, stats)
				assert.Equal(t, tt.kbID, stats.KbID)
				assert.Equal(t, 2, stats.VectorCount)
				assert.Equal(t, 3, stats.Dimension)
				assert.Greater(t, stats.MemoryUsage, int64(0))
			}
		})
	}
}

// TestFAISSIndexManager tests the FAISS index manager (delegated to InMemoryIndexManager)
func TestFAISSIndexManager(t *testing.T) {
	ctx := context.Background()

	t.Run("NewFAISSIndexManager", func(t *testing.T) {
		manager := NewFAISSIndexManager(128)
		assert.NotNil(t, manager)
		assert.Equal(t, 128, manager.backend.dimension)

		managerDefault := NewFAISSIndexManager(0)
		assert.NotNil(t, managerDefault)
		assert.Equal(t, 1024, managerDefault.backend.dimension)
	})

	t.Run("BuildIndex", func(t *testing.T) {
		manager := NewFAISSIndexManager(3)
		err := manager.BuildIndex(ctx, "test_kb", []Chunk{
			{ID: "chunk1", Content: "test", Embedding: []float32{0.1, 0.2, 0.3}},
		})
		assert.NoError(t, err)
	})

	t.Run("AddToIndex", func(t *testing.T) {
		manager := NewFAISSIndexManager(3)
		err := manager.BuildIndex(ctx, "test_kb", []Chunk{
			{ID: "chunk1", Content: "test", Embedding: []float32{0.1, 0.2, 0.3}},
		})
		assert.NoError(t, err)
		err = manager.AddToIndex(ctx, "test_kb", Chunk{ID: "chunk2", Content: "test2", Embedding: []float32{0.4, 0.5, 0.6}})
		assert.NoError(t, err)
	})

	t.Run("RemoveFromIndex", func(t *testing.T) {
		manager := NewFAISSIndexManager(3)
		err := manager.BuildIndex(ctx, "test_kb", []Chunk{
			{ID: "chunk1", Content: "test", Embedding: []float32{0.1, 0.2, 0.3}},
		})
		assert.NoError(t, err)
		err = manager.RemoveFromIndex(ctx, "test_kb", "chunk1")
		assert.NoError(t, err)
	})

	t.Run("SearchIndex", func(t *testing.T) {
		manager := NewFAISSIndexManager(3)
		err := manager.BuildIndex(ctx, "test_kb", []Chunk{
			{ID: "chunk1", Content: "test", Embedding: []float32{0.1, 0.2, 0.3}},
		})
		assert.NoError(t, err)
		results, err := manager.SearchIndex(ctx, "test_kb", []float32{0.1, 0.2, 0.3}, 5)
		assert.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("DropIndex", func(t *testing.T) {
		manager := NewFAISSIndexManager(3)
		err := manager.DropIndex(ctx, "test_kb")
		assert.NoError(t, err)
	})

	t.Run("GetIndexStats", func(t *testing.T) {
		manager := NewFAISSIndexManager(3)
		err := manager.BuildIndex(ctx, "test_kb", []Chunk{
			{ID: "chunk1", Content: "test", Embedding: []float32{0.1, 0.2, 0.3}},
		})
		assert.NoError(t, err)
		stats, err := manager.GetIndexStats(ctx, "test_kb")
		assert.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Equal(t, "test_kb", stats.KbID)
		assert.Equal(t, 3, stats.Dimension)
		assert.Equal(t, 1, stats.VectorCount)
	})
}

// TestCalculateSimilarity tests the similarity calculation helper
func TestCalculateSimilarity(t *testing.T) {
	tests := []struct {
		name       string
		vecA       []float32
		vecB       []float32
		expectZero bool
	}{
		{
			name:       "empty_vectors",
			vecA:       []float32{},
			vecB:       []float32{},
			expectZero: true,
		},
		{
			name:       "different_lengths",
			vecA:       []float32{0.1, 0.2, 0.3},
			vecB:       []float32{0.1, 0.2},
			expectZero: true,
		},
		{
			name:       "identical_vectors",
			vecA:       []float32{0.1, 0.2, 0.3},
			vecB:       []float32{0.1, 0.2, 0.3},
			expectZero: false,
		},
		{
			name:       "orthogonal_vectors",
			vecA:       []float32{1, 0, 0},
			vecB:       []float32{0, 1, 0},
			expectZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := CalculateSimilarity(tt.vecA, tt.vecB)
			if tt.expectZero {
				assert.Equal(t, 0.0, similarity)
			} else {
				assert.NotEqual(t, 0.0, similarity)
			}
		})
	}
}

// TestNormalizeVector tests the vector normalization helper
func TestNormalizeVector(t *testing.T) {
	tests := []struct {
		name       string
		vec        []float32
		expectZero bool
	}{
		{
			name:       "empty_vector",
			vec:        []float32{},
			expectZero: true,
		},
		{
			name:       "zero_vector",
			vec:        []float32{0, 0, 0},
			expectZero: true,
		},
		{
			name:       "unit_vector",
			vec:        []float32{1, 0, 0},
			expectZero: false,
		},
		{
			name:       "needs_normalization",
			vec:        []float32{3, 4, 0},
			expectZero: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := NormalizeVector(tt.vec)

			if tt.expectZero || len(tt.vec) == 0 {
				assert.Equal(t, len(tt.vec), len(normalized))
			} else if len(tt.vec) > 0 && !isZeroVector(tt.vec) {

				var sum float64
				for _, v := range normalized {
					sum += float64(v) * float64(v)
				}
				assert.InDelta(t, 1.0, sum, 0.0001)
			}
		})
	}
}

func isZeroVector(vec []float32) bool {
	for _, v := range vec {
		if v != 0 {
			return false
		}
	}
	return true
}
