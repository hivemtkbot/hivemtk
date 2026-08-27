package service

import (
	"context"
	"time"

	"hivemtk-user/internal/aiagent/knowledge/model"
	"hivemtk-user/internal/aiagent/knowledge/repository"
)

// Reindex 重建指定文档的向量索引。复用已落库的分片内容重新向量化，不重新分块。
func (s *KnowledgeService) Reindex(ctx context.Context, productID string, docID uint64) error {
	doc, err := s.docRepo.GetByProductAndID(ctx, productID, int64(docID))
	if err != nil {
		return err
	}
	chunks, err := s.chunkRepo.GetByDocumentID(ctx, doc.ID)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		_ = s.docRepo.UpdateStatus(ctx, doc.ID, model.EmbedStatusIndexed, 100, "")
		return nil
	}
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embService, embCfg := s.resolveEmbeddingConfig(ctx, productID)
	embeddings, err := embService.Embed(ctx, embCfg, texts)
	if err != nil {
		_ = s.docRepo.UpdateStatus(ctx, doc.ID, model.EmbedStatusFailed, 0, err.Error())
		return err
	}
	// N-4 维度守卫：preset 不符条目剔除（log+跳过），不中断批量写入
	chunks, embeddings = filterValidEmbeddings(embService, embCfg, chunks, embeddings)
	if err := s.chunkRepo.UpdateEmbeddingsBatch(ctx, chunks, embeddings); err != nil {
		return err
	}
	now := time.Now()
	doc.EmbedStatus = model.EmbedStatusIndexed
	doc.LastIndexAt = &now
	_ = s.docRepo.Update(ctx, doc)
	return nil
}

// RebuildIndex 重建某产品下全部文档的向量索引。
func (s *KnowledgeService) RebuildIndex(ctx context.Context, productID string) error {
	docs, _, err := s.docRepo.List(ctx, repository.ListFilter{ProductID: productID, PageSize: 1000})
	if err != nil {
		return err
	}
	var firstErr error
	for _, d := range docs {
		if err := s.Reindex(ctx, productID, d.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

