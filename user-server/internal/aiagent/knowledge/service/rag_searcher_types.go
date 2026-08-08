package service

import (
	"fmt"
	"strconv"

	"marketing/internal/aiagent/rag/retrieval"
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/text"
)

// RAGChunk 别名(从 dto 包引用)
type RAGChunk = dto.RAGChunk

// MerchantRAGChunk 召回分段
type MerchantRAGChunk struct {
	ID         uint64         `json:"id"`
	DocumentID uint64         `json:"document_id"`
	Content    string         `json:"content"`
	Score      float64        `json:"score"`
	Metadata   map[string]any `json:"metadata"`
	// Weight 自学习权重（默认 1.0），由 RagSearcher 在检索时回填，作为排名第二依据。
	Weight float64 `json:"weight"`
}

// chunkRow 数据库扫描行（向量检索结果）
type chunkRow struct {
	ID         uint64
	DocumentID uint64
	Content    string
	Score      float64
}

// chunksToRAGChunks ragretrieval.Chunk → dto.RAGChunk
//
// 字段映射：
//   - Content: Chunk.Content → RAGChunk.Content（截断到 500 字）
//   - Score:   Chunk.Score → RAGChunk.Score
//   - DocID:   Chunk.DocumentID → RAGChunk.DocID（string 透传）
//   - ChunkID: Chunk.ID → RAGChunk.ChunkID（string 透传）
//   - Source:  固定 "hybrid"（区分 legacy 路径返回的 chunk）
func chunksToRAGChunks(chunks []ragretrieval.Chunk) []RAGChunk {
	if len(chunks) == 0 {
		return nil
	}
	result := make([]RAGChunk, 0, len(chunks))
	for _, c := range chunks {
		result = append(result, RAGChunk{
			Content: truncateText(c.Content, ChunkContentPreview),
			Source:  "hybrid",
			Score:   c.Score,
			DocID:   c.DocumentID,
			ChunkID: c.ID,
			Weight:  c.Weight,
		})
	}
	return result
}

// chunksToMerchantChunks ragretrieval.Chunk → MerchantRAGChunk
//
// 字段映射：
//   - ID / DocumentID: string → uint64（解析失败回退 0）
//   - Content / Score: 透传（Content 截断 500）
//   - Metadata: 携带 ChunkIndex / Title 用于上游展示
func chunksToMerchantChunks(chunks []ragretrieval.Chunk) []MerchantRAGChunk {
	if len(chunks) == 0 {
		return nil
	}
	result := make([]MerchantRAGChunk, 0, len(chunks))
	for _, c := range chunks {
		id, _ := strconv.ParseUint(c.ID, 10, 64)
		docID, _ := strconv.ParseUint(c.DocumentID, 10, 64)
		meta := c.Metadata
		if meta == nil {
			meta = make(map[string]any)
		}
		if c.Title != "" {
			meta["title"] = c.Title
		}
		meta["chunk_index"] = c.ChunkIndex
		result = append(result, MerchantRAGChunk{
			ID:         id,
			DocumentID: docID,
			Content:    truncateText(c.Content, ChunkContentPreview),
			Score:      c.Score,
			Metadata:   meta,
			Weight:     c.Weight,
		})
	}
	return result
}

// toRAGChunks scored → dto.RAGChunk
func (s *RagSearcher) toRAGChunks(pairs []scored) []RAGChunk {
	result := make([]RAGChunk, 0, len(pairs))
	for _, p := range pairs {
		result = append(result, RAGChunk{
			Content: truncateText(p.row.Content, ChunkContentPreview),
			Score:   p.score,
			DocID:   strconv.FormatUint(p.row.DocumentID, 10),
			ChunkID: strconv.FormatUint(p.row.ID, 10),
		})
	}
	return result
}

// toMerchantChunks []scored → []MerchantRAGChunk
func (s *RagSearcher) toMerchantChunks(pairs []scored) []MerchantRAGChunk {
	result := make([]MerchantRAGChunk, 0, len(pairs))
	for _, p := range pairs {
		result = append(result, MerchantRAGChunk{
			ID:         p.row.ID,
			DocumentID: p.row.DocumentID,
			Content:    truncateText(p.row.Content, ChunkContentPreview),
			Score:      p.score,
		})
	}
	return result
}

// filterMerchantChunksByMetadata 按附加字段过滤分片。
// 当且仅当分片的 Metadata 包含 filters 中所有键值对（字符串相等）时才保留。
// 用于把检索收敛到特定业务上下文，例如 {"customer_id":"123","order_id":"A01"}。
func filterMerchantChunksByMetadata(chunks []MerchantRAGChunk, filters map[string]string) []MerchantRAGChunk {
	if len(filters) == 0 {
		return chunks
	}
	out := make([]MerchantRAGChunk, 0, len(chunks))
	for _, c := range chunks {
		if chunkMatchesMetadata(c.Metadata, filters) {
			out = append(out, c)
		}
	}
	return out
}

// chunkMatchesMetadata 判断分片元信息是否满足全部过滤条件。
func chunkMatchesMetadata(meta map[string]any, filters map[string]string) bool {
	for k, want := range filters {
		got, ok := meta[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", got) != want {
			return false
		}
	}
	return true
}

// truncateText 截断(已迁移到 text.Truncate,本函数保留为薄包装以维持外部兼容)
func truncateText(s string, max int) string {
	return text.Truncate(s, max)
}
