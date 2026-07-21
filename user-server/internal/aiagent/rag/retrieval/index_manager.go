package ragretrieval

import (
	"context"
	"errors"
	"fmt"
	"marketing/internal/pkg/utils/logger"
	"math"
	"sync"
	"time"
)

// InMemoryIndexManager 内存索引管理器实现
// 注意：在实际生产环境中，这里应该使用专门的向量数据库（如FAISS、Milvus、Pinecone等）
type InMemoryIndexManager struct {
	indices   map[string][]Chunk // 按知识库ID存储索引
	mutex     sync.RWMutex       // 读写锁，保证并发安全
	dimension int                // 向量维度
}

// NewInMemoryIndexManager 创建新的内存索引管理器
func NewInMemoryIndexManager(dimension int) *InMemoryIndexManager {
	if dimension <= 0 {
		dimension = 1024 // 2026-07-18 私域基线：本地 TEI + bge-m3（1024 维）
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

	// 验证所有向量维度是否正确
	for _, chunk := range chunks {
		if len(chunk.Embedding) != im.dimension {
			return fmt.Errorf("chunk %s has invalid embedding dimension: expected %d, got %d",
				chunk.ID, im.dimension, len(chunk.Embedding))
		}
	}

	// 存储索引
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

	// 检查知识库是否存在，不存在则创建
	if _, exists := im.indices[kbID]; !exists {
		im.indices[kbID] = make([]Chunk, 0)
	}

	// 添加到索引
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

	// 找到要删除的chunk并移除
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
		topK = 5 // 默认返回5个结果
	}

	im.mutex.RLock()
	indexSlice, exists := im.indices[kbID]
	im.mutex.RUnlock()

	if !exists {
		return []Chunk{}, nil // 知识库不存在，返回空结果
	}

	// 计算查询向量与所有索引向量的相似度
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

	// 按相似度排序（降序）
	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[i].score < scores[j].score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// 返回topK结果
	resultCount := len(scores)
	if topK < resultCount {
		resultCount = topK
	}

	result := make([]Chunk, resultCount)
	for i := 0; i < resultCount; i++ {
		result[i] = scores[i].chunk
		// 设置相似度分数
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

	// 计算内存使用量（粗略估算）
	var memoryUsage int64
	for _, chunk := range indexSlice {
		memoryUsage += int64(len(chunk.Content))        // 内容大小
		memoryUsage += int64(len(chunk.Embedding) * 4)  // 向量大小（float32为4字节）
		memoryUsage += int64(len(chunk.Metadata) * 100) // 元数据估算
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
	backend *InMemoryIndexManager // 委托给内存索引管理器
}

// NewFAISSIndexManager 创建FAISS索引管理器
func NewFAISSIndexManager(dimension int) *FAISSIndexManager {
	if dimension <= 0 {
		dimension = 1024 // 2026-07-18 私域基线
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
		return vec // 零向量无法归一化，返回原向量
	}

	normalized := make([]float32, len(vec))
	for i, v := range vec {
		normalized[i] = float32(float64(v) / magnitude)
	}

	return normalized
}
