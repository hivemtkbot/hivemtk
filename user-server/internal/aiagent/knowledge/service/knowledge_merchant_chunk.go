package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"marketing/internal/aiagent/knowledge/model"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// 3) 分段编辑
// ============================================================================

// GetDocumentChunks 列出文档分段（支持分页）
func (s *KnowledgeMerchantService) GetDocumentChunks(ctx context.Context, documentID uint64, page, pageSize int) ([]model.KnowledgeChunk, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	return s.chunkRepo.PageByDocumentID(ctx, documentID, page, pageSize)
}

// UpdateChunkRequest 更新分段内容
type UpdateChunkRequest struct {
	ChunkID uint64 `json:"chunk_id"`
	Content string `json:"content"`
}

// UpdateChunk 更新分段
func (s *KnowledgeMerchantService) UpdateChunk(ctx context.Context, req *UpdateChunkRequest) error {
	if req.ChunkID == 0 {
		return errors.New("chunk_id 不能为空")
	}
	if strings.TrimSpace(req.Content) == "" {
		return errors.New("内容不能为空")
	}
	chunk, err := s.chunkRepo.GetByID(ctx, req.ChunkID)
	if err != nil {
		return fmt.Errorf("分段不存在: %w", err)
	}
	chunk.Content = req.Content
	chunk.CharCount = len(req.Content)
	chunk.TokenCount = len(strings.Fields(req.Content))
	h := sha256.Sum256([]byte(req.Content))
	chunk.ContentHash = hex.EncodeToString(h[:])
	chunk.EmbeddingID = "" // 内容变化后清空旧向量，触发重新向量化
	if err := s.chunkRepo.Update(ctx, chunk); err != nil {
		return err
	}
	// 重新向量化（per-product 配置优先），保持 knowledge_chunks.embedding 与内容一致
	if err := s.kbService.EmbedAndPersistChunks(ctx, chunk.ProductID, []model.KnowledgeChunk{*chunk}); err != nil {
		logger.Errorf("[knowledge] 分段更新后重新向量化失败: %v", err)
	}
	return nil
}

// DeleteChunk 删除分段
func (s *KnowledgeMerchantService) DeleteChunk(ctx context.Context, chunkID uint64) error {
	if chunkID == 0 {
		return errors.New("chunk_id 不能为空")
	}
	return s.chunkRepo.Delete(ctx, chunkID)
}

// SplitChunkRequest 拆分分段
type SplitChunkRequest struct {
	ChunkID uint64   `json:"chunk_id"`
	Parts   []string `json:"parts"`
}

// SplitChunk 拆分分段为多段
func (s *KnowledgeMerchantService) SplitChunk(ctx context.Context, req *SplitChunkRequest) error {
	if req.ChunkID == 0 {
		return errors.New("chunk_id 不能为空")
	}
	if len(req.Parts) < 2 {
		return errors.New("至少需要 2 段")
	}
	chunk, err := s.chunkRepo.GetByID(ctx, req.ChunkID)
	if err != nil {
		return err
	}
	// 删除原分段，插入新分段
	if err := s.chunkRepo.Delete(ctx, chunk.ID); err != nil {
		return err
	}
	docID := chunk.DocumentID
	productID := chunk.ProductID
	newChunks := make([]model.KnowledgeChunk, 0, len(req.Parts))
	baseIndex := chunk.ChunkIndex
	for i, part := range req.Parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		h := sha256.Sum256([]byte(part))
		newChunks = append(newChunks, model.KnowledgeChunk{
			DocumentID:  docID,
			ProductID:   productID,
			ChunkIndex:  baseIndex + i,
			Content:     part,
			ContentHash: hex.EncodeToString(h[:]),
			CharCount:   len(part),
			TokenCount:  len(strings.Fields(part)),
		})
	}
	if err := s.chunkRepo.BatchCreate(ctx, newChunks); err != nil {
		return err
	}
	// 重新向量化（per-product 配置优先），保持 knowledge_chunks.embedding 与新分段一致
	if err := s.kbService.EmbedAndPersistChunks(ctx, productID, newChunks); err != nil {
		logger.Errorf("[knowledge] 分段重切后重新向量化失败: %v", err)
	}
	return nil
}
