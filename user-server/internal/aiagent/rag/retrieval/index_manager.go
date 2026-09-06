package ragretrieval

import (
	"context"
	"errors"
	"fmt"
	"hivemtk-user/internal/pkg/utils/logger"
	"math"
	"sort"
	"sync"
	"time"
)

// InMemoryIndexManager 内存索引管理器实现
// 注意：在实际生产环境中，这里应该使用专门的向量数据库（如FAISS、Milvus、Pinecone等）
type InMemoryIndexManager struct {
	indices   map[string][]Chunk
	mutex     sync.RWMutex
	dimension int
}

// NewInMemoryIndexManager 创建新的内存索引管理器
func NewInMemoryIndexManager(dimension int) *InMemoryIndexManager {
	if dimension <= 0 {
		dimension = 1024
	}

	return &InMemoryIndexManager{
		indices:   make(map[string][]Chunk),
		dimension: dimension,
	}
}

// BuildIndex 构建索引
func (im *InMemoryIndexManager) BuildIndex(ctx context.Context, kbID string, chunks []Chunk) error {
	if kbID == "" {
		return errors.New("kbID cannot be empty")
	}

	if len(chunks) == 0 {
		return errors.New("chunks cannot be empty")
	}

	im.mutex.Lock()
	defer im.mutex.Unlock()

	for _, chunk := range chunks {
		if len(chunk.Embedding) != im.dimension {
			return fmt.Errorf("chunk %s has invalid embedding dimension: expected %d, got %d",
				chunk.ID, im.dimension, len(chunk.Embedding))
		}
	}

	im.indices[kbID] = make([]Chunk, len(chunks))
	copy(im.indices[kbID], chunks)

	return nil
}

// AddToIndex 向索引中添加向量
func (im *InMemoryIndexManager) AddToIndex(ctx context.Context, kbID string, chunk Chunk) error {
	if kbID == "" {
		return errors.New("kbID cannot be empty")
	}

	if len(chunk.Embedding) != im.dimension {
		return fmt.Errorf("chunk has invalid embedding dimension: expected %d, got %d",
			im.dimension, len(chunk.Embedding))
	}

	im.mutex.Lock()
	defer im.mutex.Unlock()

	if _, exists := im.indices[kbID]; !exists {
		im.indices[kbID] = make([]Chunk, 0)
	}

	im.indices[kbID] = append(im.indices[kbID], chunk)

	return nil
}

// RemoveFromIndex 从索引中删除向量
func (im *InMemoryIndexManager) RemoveFromIndex(ctx context.Context, kbID, chunkID string) error {
	if kbID == "" {
		return errors.New("kbID cannot be empty")
	}

	if chunkID == "" {
		return errors.New("chunkID cannot be empty")
	}

	im.mutex.Lock()
	defer im.mutex.Unlock()

	indexSlice, exists := im.indices[kbID]
	if !exists {
		return fmt.Errorf("knowledge base %s does not exist", kbID)
	}

	newIndexSlice := make([]Chunk, 0)
	found := false

	for _, chunk := range indexSlice {
		if chunk.ID != chunkID {
			newIndexSlice = append(newIndexSlice, chunk)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("chunk %s not found in knowledge base %s", chunkID, kbID)
	}

	im.indices[kbID] = newIndexSlice

	return nil
}

// SearchIndex 在索引中检索
func (im *InMemoryIndexManager) SearchIndex(ctx context.Context, kbID string, queryVec []float32, topK int) ([]Chunk, error) {
	if kbID == "" {
		return nil, errors.New("kbID cannot be empty")
	}

	if len(queryVec) != im.dimension {
		return nil, fmt.Errorf("query vector has invalid dimension: expected %d, got %d",
			im.dimension, len(queryVec))
	}

	if topK <= 0 {
		topK = 5
	}

	im.mutex.RLock()
	indexSlice, exists := im.indices[kbID]
	im.mutex.RUnlock()

	if !exists {
		return []Chunk{}, nil
	}

	scores := make([]struct {
		chunk Chunk
		score float64
	}, len(indexSlice))

	for i, chunk := range indexSlice {
		score := CosineSimilarity(queryVec, chunk.Embedding)
		scores[i] = struct {
			chunk Chunk
			score float64
		}{
			chunk: chunk,
			score: score,
		}
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	resultCount := len(scores)
	if topK < resultCount {
		resultCount = topK
	}

	result := make([]Chunk, resultCount)
	for i := 0; i < resultCount; i++ {
		result[i] = scores[i].chunk
		result[i].Score = scores[i].score
	}

	return result, nil
}

// DropIndex 删除索引
func (im *InMemoryIndexManager) DropIndex(ctx context.Context, kbID string) error {
	if kbID == "" {
		return errors.New("kbID cannot be empty")
	}

	im.mutex.Lock()
	defer im.mutex.Unlock()

	delete(im.indices, kbID)

	return nil
}

// GetIndexStats 获取索引统计信息
func (im *InMemoryIndexManager) GetIndexStats(ctx context.Context, kbID string) (*IndexStats, error) {
	if kbID == "" {
		return nil, errors.New("kbID cannot be empty")
	}

	im.mutex.RLock()
	indexSlice, exists := im.indices[kbID]
	im.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("knowledge base %s does not exist", kbID)
	}

	var memoryUsage int64
	for _, chunk := range indexSlice {
		memoryUsage += int64(len(chunk.Content))
		memoryUsage += int64(len(chunk.Embedding) * 4)
		memoryUsage += int64(len(chunk.Metadata) * 100)
	}

	stats := &IndexStats{
		KbID:        kbID,
		VectorCount: len(indexSlice),
		Dimension:   im.dimension,
		MemoryUsage: memoryUsage,
		LastUpdated: time.Now(),
	}

	return stats, nil
}

// FAISSIndexManager FAISS索引管理器
// 使用 InMemoryIndexManager 作为后端，提供完整的向量索引功能
// 当未来需要接入FAISS C++库时，可替换后端实现
type FAISSIndexManager struct {
	backend *InMemoryIndexManager
}

// NewFAISSIndexManager 创建FAISS索引管理器
func NewFAISSIndexManager(dimension int) *FAISSIndexManager {
	if dimension <= 0 {
		dimension = 1024
	}
	return &FAISSIndexManager{
		backend: NewInMemoryIndexManager(dimension),
	}
}

// BuildIndex 构建索引
func (fm *FAISSIndexManager) BuildIndex(ctx context.Context, kbID string, chunks []Chunk) error {
	logger.Info(fmt.Sprintf("[FAISSIndexManager] Building index for KB: %s with %d chunks", kbID, len(chunks)))
	return fm.backend.BuildIndex(ctx, kbID, chunks)
}

// AddToIndex 向索引中添加向量
func (fm *FAISSIndexManager) AddToIndex(ctx context.Context, kbID string, chunk Chunk) error {
	logger.Info(fmt.Sprintf("[FAISSIndexManager] Adding chunk %s to KB: %s", chunk.ID, kbID))
	return fm.backend.AddToIndex(ctx, kbID, chunk)
}

// RemoveFromIndex 从索引中删除向量
func (fm *FAISSIndexManager) RemoveFromIndex(ctx context.Context, kbID, chunkID string) error {
	logger.Info(fmt.Sprintf("[FAISSIndexManager] Removing chunk %s from KB: %s", chunkID, kbID))
	return fm.backend.RemoveFromIndex(ctx, kbID, chunkID)
}

// SearchIndex 在索引中检索
func (fm *FAISSIndexManager) SearchIndex(ctx context.Context, kbID string, queryVec []float32, topK int) ([]Chunk, error) {
	logger.Info(fmt.Sprintf("[FAISSIndexManager] Searching in KB: %s with topK=%d", kbID, topK))
	results, err := fm.backend.SearchIndex(ctx, kbID, queryVec, topK)
	if err != nil {
		return nil, err
	}
	logger.Info(fmt.Sprintf("[FAISSIndexManager] Found %d results for KB: %s", len(results), kbID))
	return results, nil
}

// DropIndex 删除索引
func (fm *FAISSIndexManager) DropIndex(ctx context.Context, kbID string) error {
	logger.Info(fmt.Sprintf("[FAISSIndexManager] Dropping index for KB: %s", kbID))
	return fm.backend.DropIndex(ctx, kbID)
}

// GetIndexStats 获取索引统计信息
func (fm *FAISSIndexManager) GetIndexStats(ctx context.Context, kbID string) (*IndexStats, error) {
	logger.Info(fmt.Sprintf("[FAISSIndexManager] Getting stats for KB: %s", kbID))
	stats, err := fm.backend.GetIndexStats(ctx, kbID)
	if err != nil {
		return nil, err
	}
	logger.Info(fmt.Sprintf("[FAISSIndexManager] KB %s: %d vectors, %d bytes memory",
		kbID, stats.VectorCount, stats.MemoryUsage))
	return stats, nil
}

// CalculateSimilarity 计算相似度的辅助函数
func CalculateSimilarity(queryVec []float32, targetVec []float32) float64 {
	return CosineSimilarity(queryVec, targetVec)
}

// NormalizeVector 归一化向量的辅助函数
func NormalizeVector(vec []float32) []float32 {
	if len(vec) == 0 {
		return vec
	}

	var sumSquares float64
	for _, v := range vec {
		sumSquares += float64(v) * float64(v)
	}

	magnitude := math.Sqrt(sumSquares)
	if magnitude == 0 {
		return vec
	}

	normalized := make([]float32, len(vec))
	for i, v := range vec {
		normalized[i] = float32(float64(v) / magnitude)
	}

	return normalized
}
