// Package service 知识库子域 —— 异步处理流水线
//
// 文档异步处理全流程（status update → 文本提取 → 分片 → 写库 → 向量化 → 入索引）。
// 单一职责：异步 goroutine 内执行的所有步骤集中在此文件，便于追踪"知识库入库
// 失败"的所有失败点。
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"marketing/internal/aiagent/knowledge/model"
	rag_core "marketing/internal/aiagent/rag/core"
	ragretrieval "marketing/internal/aiagent/rag/retrieval"
	"marketing/internal/etl"
	"marketing/internal/pkg/utils/logger"
)

// processDocumentAsync 异步处理文档
//
// 流程：mark processing → 文本提取 → 分片 → 写分段表 → 向量化 → 持久化向量
//
//	→ 入内存索引（生产 s.indexer==nil 跳过）→ mark indexed。
//
// 任何步骤失败通过 markFailed 写入 document.error_msg 并终止，
// panic 通过 defer recover 兜底确保文档不会永久 pending。
func (s *KnowledgeService) processDocumentAsync(bgCtx context.Context, documentID uint64, productID string, filePath, fileName, content, mimeType, title string, source model.SourceType, docMeta map[string]any) {
	// panic 兜底：异步 goroutine 内异常会导致文档永久 pending 且无法重试，必须 recover 并标记失败
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[knowledge] processDocumentAsync panic doc=%d: %v", documentID, r)
			s.markFailed(bgCtx, documentID, fmt.Sprintf("处理异常: %v", r))
		}
	}()

	// 1. 标记为处理中
	if err := s.docRepo.UpdateStatus(bgCtx, documentID, model.EmbedStatusProcessing, 10, ""); err != nil {
		logger.Errorf("[knowledge] 标记处理中失败: %v", err)
		return
	}

	// 2. 读取并提取文本
	//    上传的 PDF/DOCX 是二进制，不能直接 string(bytes) 切片（会产出乱码）。
	//    先按文件名扩展名提取纯文本，再交给分片器。
	if content == "" && filePath != "" {
		bytes, err := os.ReadFile(filePath)
		if err != nil {
			s.markFailed(bgCtx, documentID, fmt.Sprintf("读取文件失败: %v", err))
			return
		}
		extracted, extErr := etl.ExtractText(fileName, bytes)
		if extErr != nil {
			// 解析失败(如扫描件PDF)退化为原始字节，避免整篇丢失
			logger.Warnf("[knowledge] 文本提取失败 doc=%d: %v，退化为原始内容", documentID, extErr)
			extracted = string(bytes)
		}
		content = extracted
	}
	if strings.TrimSpace(content) == "" {
		s.markFailed(bgCtx, documentID, "文档内容为空（可能为扫描件或空文件）")
		return
	}

	// 3. 分片
	ragDoc := rag_core.Document{
		ID:      fmt.Sprintf("kb_doc_%d", documentID),
		Content: content,
		Metadata: map[string]any{
			"document_id": documentID,
			"product_id":  productID,
			"title":       title,
			"source":      string(source),
		},
		CreatedAt: time.Now(),
	}
	chunks, err := s.processor.ProcessDocument(bgCtx, ragDoc)
	if err != nil {
		s.markFailed(bgCtx, documentID, fmt.Sprintf("分片失败: %v", err))
		return
	}
	if len(chunks) == 0 {
		s.markFailed(bgCtx, documentID, "分片结果为空")
		return
	}
	if err := s.docRepo.UpdateStatus(bgCtx, documentID, model.EmbedStatusProcessing, 30, ""); err != nil {
		logger.Errorf("[knowledge] 更新进度失败: %v", err)
	}

	// 4. 写入分段表（分片携带文档级基础信息 + 业务附加字段）
	baseMeta := map[string]any{
		"document_id": float64(documentID),
		"product_id":  productID,
		"title":       title,
		"source":      string(source),
	}
	for k, v := range docMeta {
		baseMeta[k] = v
	}
	chunkModels := make([]model.KnowledgeChunk, 0, len(chunks))
	totalTokens := 0
	for idx, c := range chunks {
		hash := sha256.Sum256([]byte(c.Content))
		hashStr := hex.EncodeToString(hash[:])
		chunkMeta := map[string]any{"chunk_index": float64(idx)}
		for k, v := range baseMeta {
			chunkMeta[k] = v
		}
		metaBytes, _ := json.Marshal(chunkMeta)
		chunkModels = append(chunkModels, model.KnowledgeChunk{
			DocumentID:  documentID,
			ProductID:   productID,
			ChunkIndex:  idx,
			Content:     c.Content,
			ContentHash: hashStr,
			TokenCount:  c.TokenCount,
			CharCount:   len(c.Content),
			Metadata:    string(metaBytes),
		})
		totalTokens += c.TokenCount
	}
	if err := s.chunkRepo.BatchCreate(bgCtx, chunkModels); err != nil {
		s.markFailed(bgCtx, documentID, fmt.Sprintf("写入分段失败: %v", err))
		return
	}
	if err := s.docRepo.UpdateStatus(bgCtx, documentID, model.EmbedStatusProcessing, 60, ""); err != nil {
		logger.Errorf("[knowledge] 更新进度失败: %v", err)
	}

	// 5. 向量化（per 知识库 embedding 配置优先，否则全局默认）
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embService, embCfg := s.resolveEmbeddingConfig(bgCtx, productID)
	embeddings, err := embService.Embed(bgCtx, embCfg, texts)
	if err != nil {
		s.markFailed(bgCtx, documentID, fmt.Sprintf("向量化失败: %v", err))
		return
	}
	if err := s.docRepo.UpdateStatus(bgCtx, documentID, model.EmbedStatusProcessing, 80, ""); err != nil {
		logger.Errorf("[knowledge] 更新进度失败: %v", err)
	}

	// 5.5 持久化向量到 knowledge_chunks.embedding（pgvector）
	// embeddings 用于内存索引与持久化；生产 s.indexer==nil 时通过持久化路径承接，
	// 否则 knowledge_chunks.embedding 为空会导致检索侧 vectorSearch 读不到任何向量 → RAG 召回失效。
	if err := s.persistChunkEmbeddings(bgCtx, chunkModels, embeddings); err != nil {
		s.markFailed(bgCtx, documentID, fmt.Sprintf("持久化向量失败: %v", err))
		return
	}

	// 6. 入内存索引（生产 s.indexer==nil 跳过；真实检索走 pgvector，见 rag_searcher.go）
	if s.indexer != nil {
		idxChunks := make([]ragretrieval.Chunk, len(chunks))
		for i, c := range chunks {
			idxChunks[i] = ragretrieval.Chunk{
				ID:         c.ID,
				DocumentID: c.DocumentID,
				Content:    c.Content,
				Metadata:   c.Metadata,
				Embedding:  embeddings[i],
				Score:      c.Score,
				TokenCount: c.TokenCount,
				ChunkIndex: i,
			}
		}
		_ = s.indexer.BuildIndex(bgCtx, fmt.Sprintf("product_%s", productID), idxChunks)
	}

	// 7. 更新文档状态
	if err := s.docRepo.UpdateChunkStats(bgCtx, documentID, len(chunks), totalTokens); err != nil {
		logger.Errorf("[knowledge] 更新分段统计失败: %v", err)
	}
	if err := s.docRepo.UpdateStatus(bgCtx, documentID, model.EmbedStatusIndexed, 100, ""); err != nil {
		logger.Errorf("[knowledge] 更新已索引状态失败: %v", err)
	}

	// 8. 更新产品冗余字段
	docCount, _ := s.docRepo.CountByProduct(bgCtx, productID)
	chunkCount, _ := s.chunkRepo.CountByProductID(bgCtx, productID)
	now := time.Now()
	// 反查 UUID
	var productUUID string
	if s.ragRepo != nil {
		if prod, _ := s.ragRepo.FindRagProductByIDOnly(bgCtx, productID); prod != nil {
			productUUID = prod.ID
		}
	}
	if productUUID != "" {
		_ = s.ragRepo.UpdateRagProductStats(bgCtx, productUUID, int(docCount), chunkCount, &now)
		// 注:UpdateRagProductStats 方法名保留以兼容旧逻辑(独立部署模式下不真正使用)
	}
}

// markFailed 标记文档为失败状态
func (s *KnowledgeService) markFailed(ctx context.Context, documentID uint64, errMsg string) {
	if err := s.docRepo.UpdateStatus(ctx, documentID, model.EmbedStatusFailed, 0, errMsg); err != nil {
		logger.Errorf("[knowledge] 标记失败状态错误: %v", err)
	}
}

