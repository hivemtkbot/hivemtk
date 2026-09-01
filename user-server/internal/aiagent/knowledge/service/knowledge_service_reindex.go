package service

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"hivemtk-user/internal/aiagent/knowledge/model"
	"hivemtk-user/internal/aiagent/knowledge/repository"
	"hivemtk-user/internal/pkg/utils/async"
	"hivemtk-user/internal/pkg/utils/logger"
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
	logger.Infof("[knowledge][Reindex] called doc=%d chunks=%d", doc.ID, len(chunks))
	if len(chunks) == 0 {
		// R43 修复：0 分片说明上次分片阶段失败（如短文本被 MinChunkSize 全量过滤，
		// 或 embedding 不可达中断），绝非"无可索引内容"。此前直接伪造 indexed/100
		// 状态（假成功），文档永远无法被检索。改为：读回源内容重走完整分片+向量化管线。
		content := ""
		logger.Infof("[knowledge][Reindex] zero-chunk re-pipeline doc=%d file=%s", doc.ID, doc.FilePath)
		if doc.FilePath != "" {
			if b, rerr := os.ReadFile(doc.FilePath); rerr == nil {
				content = string(b)
			}
		}
		if strings.TrimSpace(content) == "" {
			_ = s.docRepo.UpdateStatus(ctx, doc.ID, model.EmbedStatusFailed, 0,
				"重建失败：0 分片且源内容不可读（文件缺失或为空）")
			return nil
		}
		meta := map[string]any{}
		if doc.Metadata != "" {
			_ = json.Unmarshal([]byte(doc.Metadata), &meta)
		}
		async.RunWithTimeout(ctx, AsyncProcessingTimeout, func(procCtx context.Context) {
			s.processDocumentAsync(procCtx, doc.ID, doc.ProductID, doc.FilePath,
				doc.FileName, content, doc.MimeType, doc.Title, doc.SourceType, meta)
		})
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

