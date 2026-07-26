// Package service 知识库子域 —— 文档导入与异步处理
//
// 文档导入（保存文件 + 创建记录）+ 异步处理流水线（分片/向量化/入索引）。
// 单一职责：所有"把外部文件变成可检索内容"的代码集中在此文件。
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"marketing/internal/aiagent/knowledge/model"
	rag_core "marketing/internal/aiagent/rag/core"
	ragretrieval "marketing/internal/aiagent/rag/retrieval"
	"marketing/internal/pkg/utils/async"
	"marketing/internal/pkg/utils/logger"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ImportDocumentResult 单文档导入结果
type ImportDocumentResult struct {
	DocumentID	uint	`json:"document_id"`
	Title		string	`json:"title"`
	FilePath	string	`json:"file_path"`
	Status		string	`json:"status"`
}

// ImportDocument 导入文档:保存文件 + 创建记录(status=pending) + 异步处理
func (s *KnowledgeBaseService) ImportDocument(ctx context.Context, title string, file multipart.File, header *multipart.FileHeader) (*ImportDocumentResult, error) {
	if header == nil {
		return nil, errors.New("文件不能为空")
	}

	// 校验文件类型
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".pdf": true, ".docx": true, ".doc": true, ".txt": true, ".md": true}
	if !allowed[ext] {
		return nil, fmt.Errorf("不支持的文件类型: %s", ext)
	}

	// 创建保存目录
	uploadDir := filepath.Join("uploads", "knowledge-base", time.Now().Format("20060102"))
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("创建上传目录失败: %w", err)
	}
	filename := uuid.New().String() + ext
	filePath := filepath.Join(uploadDir, filename)

	// 写入文件
	dst, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("创建文件失败: %w", err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}

	// 文件实际大小
	size, _ := getFileSize(filePath)

	if title == "" {
		title = strings.TrimSuffix(header.Filename, ext)
	}

	doc := &model.KBDocument{
		Title:		title,
		FilePath:	filePath,
		FileSize:	size,
		FileType:	ext,
		Status:		model.KBDocumentStatusPending,
	}
	if err := s.db.WithContext(ctx).Create(doc).Error; err != nil {
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("保存文档记录失败: %w", err)
	}

	// 异步处理文档(分片 + 向量化 + 入索引)
	// 使用 async.RunWithTimeout 保留原 ctx 的 trace ID 等 Value，
	// 但用 Background() 作为 parent 并施加 AsyncProcessingTimeout 超时，防止文档永久卡在 processing。
	async.RunWithTimeout(ctx, AsyncProcessingTimeout, func(procCtx context.Context) {
		s.processDocumentAsync(procCtx, doc.ID, filePath)
	})

	return &ImportDocumentResult{
		DocumentID:	doc.ID,
		Title:		doc.Title,
		FilePath:	filePath,
		Status:		string(doc.Status),
	}, nil
}

// processDocumentAsync 异步处理文档
func (s *KnowledgeBaseService) processDocumentAsync(ctx context.Context, documentID uint, filePath string) {
	bgCtx := ctx
	// panic 兜底：异步 goroutine 内异常会导致文档永久 pending，必须 recover 并标记失败
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[knowledge-base] processDocumentAsync panic doc=%d: %v", documentID, r)
			s.markDocumentFailed(bgCtx, documentID, fmt.Sprintf("处理异常: %v", r))
		}
	}()

	// 标记为 processing
	_ = s.db.WithContext(bgCtx).Model(&model.KBDocument{}).
		Where("id = ?", documentID).
		Updates(map[string]any{"status": model.KBDocumentStatusProcessing, "error_msg": ""}).Error

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		s.markDocumentFailed(bgCtx, documentID, fmt.Sprintf("读取文件失败: %v", err))
		return
	}
	content := string(contentBytes)

	ragDoc := rag_core.Document{
		ID:		fmt.Sprintf("kbdoc_%d", documentID),
		Content:	content,
		Metadata: map[string]any{
			"document_id":	documentID,
			"file_path":	filePath,
			"title":	filepath.Base(filePath),
		},
		CreatedAt:	time.Now(),
	}

	chunks, err := s.processor.ProcessDocument(bgCtx, ragDoc)
	if err != nil {
		s.markDocumentFailed(bgCtx, documentID, fmt.Sprintf("分片失败: %v", err))
		return
	}

	if len(chunks) == 0 {
		s.markDocumentFailed(bgCtx, documentID, "文档分片结果为空")
		return
	}

	// 真实向量化(无 API key 时降级到 hash embedding)
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embeddings, err := s.vector.EmbedBatch(texts)
	if err != nil {
		s.markDocumentFailed(bgCtx, documentID, fmt.Sprintf("向量化失败: %v", err))
		return
	}

	// 转换为 ragretrieval.Chunk
	idxChunks := make([]ragretrieval.Chunk, len(chunks))
	for i, c := range chunks {
		idxChunks[i] = ragretrieval.Chunk{
			ID:		c.ID,
			DocumentID:	c.DocumentID,
			Content:	c.Content,
			Metadata:	c.Metadata,
			Embedding:	embeddings[i],
			Score:		c.Score,
			TokenCount:	c.TokenCount,
		}
	}

	// 写入索引
	kbIndexID := fmt.Sprintf("kb_%d", documentID)
	if err := s.indexer.BuildIndex(bgCtx, kbIndexID, idxChunks); err != nil {
		s.markDocumentFailed(bgCtx, documentID, fmt.Sprintf("构建索引失败: %v", err))
		return
	}

	// 更新状态
	_ = s.db.WithContext(bgCtx).Model(&model.KBDocument{}).Where("id = ?", documentID).Updates(map[string]any{
		"status":	model.KBDocumentStatusIndexed,
		"chunk_count":	len(chunks),
		"content":	content,
		"error_msg":	"",
	}).Error
}

func (s *KnowledgeBaseService) markDocumentFailed(ctx context.Context, documentID uint, errMsg string) {
	_ = s.db.WithContext(ctx).Model(&model.KBDocument{}).Where("id = ?", documentID).Updates(map[string]any{
		"status":	model.KBDocumentStatusFailed,
		"error_msg":	errMsg,
	}).Error
}
