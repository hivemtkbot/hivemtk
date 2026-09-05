package service

import (
	"fmt"
	"strconv"

	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/text"
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
	Weight     float64        `json:"weight"`
}

type chunkRow struct {
	ID         uint64
	DocumentID uint64
	Content    string
	Score      float64
}

func chunksToRAGChunks(chunks []ragretrieval.Chunk) []RAGChunk {
	if len(chunks) == 0 {
		return nil
	}
	result := make([]RAGChunk, 0, len(chunks))
	for _, c := range chunks {
		result = append(result, RAGChunk{
			Content: truncateText(c.Content, ChunkContentPreview()),
			Source:  "hybrid",
			Score:   c.Score,
			DocID:   c.DocumentID,
			ChunkID: c.ID,
			Weight:  c.Weight,
		})
	}
	return result
}

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
			Content:    truncateText(c.Content, ChunkContentPreview()),
			Score:      c.Score,
			Metadata:   meta,
			Weight:     c.Weight,
		})
	}
	return result
}

func (s *RagSearcher) toRAGChunks(pairs []scored) []RAGChunk {
	result := make([]RAGChunk, 0, len(pairs))
	for _, p := range pairs {
		result = append(result, RAGChunk{
			Content: truncateText(p.row.Content, ChunkContentPreview()),
			Score:   p.score,
			DocID:   strconv.FormatUint(p.row.DocumentID, 10),
			ChunkID: strconv.FormatUint(p.row.ID, 10),
		})
	}
	return result
}

func (s *RagSearcher) toMerchantChunks(pairs []scored) []MerchantRAGChunk {
	result := make([]MerchantRAGChunk, 0, len(pairs))
	for _, p := range pairs {
		result = append(result, MerchantRAGChunk{
			ID:         p.row.ID,
			DocumentID: p.row.DocumentID,
			Content:    truncateText(p.row.Content, ChunkContentPreview()),
			Score:      p.score,
		})
	}
	return result
}

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

func truncateText(s string, max int) string {
	return text.Truncate(s, max)
}
