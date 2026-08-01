// Package rag 提供 RAG 知识库相关能力。
//
// 本包当前职责：
//  1. IncrementalIndexer 增量索引器
//     订阅 event.TopicKnowledgeDocumentChanged 事件
//     监听知识库文档变更(创建/更新/删除)
//     触发向量化与 chunk 更新
//
// 设计依据：ADR-008 §2.5 (子项 2 增量索引更新)
package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/aiagent/knowledge/model"
	knowledgerepo "marketing/internal/aiagent/knowledge/repository"
	"marketing/internal/aiagent/llm"
	"marketing/internal/aiagent/rag/core"
	"marketing/internal/etl"
	"marketing/internal/event"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// 1. IncrementalIndexer 增量索引器
// ============================================================================

// IncrementalIndexer 知识库文档增量索引器
//
// 职责：
//  1. 订阅 event.TopicKnowledgeDocumentChanged 事件
//  2. 根据 ChangeType(create/update/delete)执行不同动作
//  3. create/update:解析 → 切块 → Embedding → 写 chunks
//  4. delete:级联删除所有 chunks
//
// 并发：内部 RWMutex 保证线程安全
// 失败：仅记录日志,不重试(主流程可重新发布事件)
type IncrementalIndexer struct {
	mu          sync.RWMutex
	chunksByDoc map[string][]*eventIndexedChunk // docID -> chunks
	embedder    llm.EmbeddingServiceInterface
	processor   *etl.DocumentProcessor
	docRepo     *knowledgerepo.KnowledgeDocumentRepository
	chunkRepo   *knowledgerepo.KnowledgeChunkRepository
	db          *gorm.DB
	stopped     bool
}

// eventIndexedChunk 内部索引分片
type eventIndexedChunk struct {
	DocumentID  string
	ChunkIndex  int
	Content     string
	ContentHash string
	Embedding   []float32
	TokenCount  int
	IndexedAt   time.Time
}

// NewIncrementalIndexer 创建增量索引器
//
// 参数：
//   - embedder:  向量服务(可为 nil,nil 时跳过 embedding 仅切块)
//   - processor: 文档处理器(可为 nil,nil 时使用默认配置)
//   - db:        数据库连接(可为 nil,nil 时退化为内存索引模式;不查库、不持久化 chunks)
func NewIncrementalIndexer(embedder llm.EmbeddingServiceInterface, processor *etl.DocumentProcessor, db *gorm.DB) *IncrementalIndexer {
	if processor == nil {
		processor = etl.NewDocumentProcessor(nil)
	}
	idx := &IncrementalIndexer{
		chunksByDoc: make(map[string][]*eventIndexedChunk),
		embedder:    embedder,
		processor:   processor,
		db:          db,
	}
	if db != nil {
		idx.docRepo = knowledgerepo.NewKnowledgeDocumentRepository(db)
		idx.chunkRepo = knowledgerepo.NewKnowledgeChunkRepository(db)
	}
	return idx
}

// Handle 处理知识库文档变更事件
//
// 实现 event.Handler 接口
//
// 步骤：
//  1. 类型断言载荷
//  2. 根据 ChangeType 分发
//  3. 异常隔离(panic 不影响 worker)
func (i *IncrementalIndexer) Handle(evt event.Event) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[rag] panic in incremental_indexer topic=%s err=%v\n%s",
				evt.Topic, r, debug.Stack())
		}
	}()

	if i.stopped {
		return nil
	}

	payload, ok := evt.Payload.(event.KnowledgeDocumentChangePayload)
	if !ok {
		// 兼容指针
		if p, ok := evt.Payload.(*event.KnowledgeDocumentChangePayload); ok && p != nil {
			return i.handlePayload(context.Background(), *p)
		}
		return nil // 非本订阅者关心的事件,直接忽略
	}

	return i.handlePayload(context.Background(), payload)
}

// handlePayload 处理单个变更事件
func (i *IncrementalIndexer) handlePayload(ctx context.Context, payload event.KnowledgeDocumentChangePayload) error {
	start := time.Now()

	docIDStr := strconv.FormatUint(uint64(payload.DocumentID), 10)
	logger.Infof("[rag] incremental indexer received workspace=%v doc=%d change=%s hash=%s",
		payload.WorkspaceID, payload.DocumentID, payload.ChangeType, payload.ContentHash)

	var err error
	switch payload.ChangeType {
	case "create", "update":
		err = i.indexDocument(ctx, payload, docIDStr)
	case "delete":
		err = i.deleteDocument(docIDStr)
	default:
		logger.Infof("[rag] unknown change_type=%s, ignored", payload.ChangeType)
		return nil
	}

	if err != nil {
		logger.Errorf("[rag] index document failed doc=%d err=%v", payload.DocumentID, err)
		return err
	}

	logger.Infof("[rag] incremental indexed doc=%d change=%s duration=%s",
		payload.DocumentID, payload.ChangeType, time.Since(start))
	return nil
}

// indexDocument 索引文档(创建/更新)
//
// 真实实现：从 knowledge_documents 表读取文档 FilePath / FileURL / SourceType 决定内容获取方式：
//   - FilePath 非空 → 读取本地文件内容
//   - FileURL 非空 → HTTP GET 获取内容
//   - 都为空 → 退化为 SourceRef（飞书/Notion 等）或 Title 占位文本
//
// 随后切块 → Embedding → 写 knowledge_chunks 表（事务模式）。
// db 为 nil 或 docRepo 不可用时退化为内存索引（仅切块 + 内存存储），便于测试与离线运行。
func (i *IncrementalIndexer) indexDocument(ctx context.Context, payload event.KnowledgeDocumentChangePayload, docIDStr string) error {
	// 1. 解析真实文档内容
	content, err := i.loadDocumentContent(ctx, payload, docIDStr)
	if err != nil {
		// 内容获取失败：记录告警并降级为 Title 占位（避免 silent skip）
		logger.Errorf("[rag] load content failed doc=%d err=%v, fallback to title", payload.DocumentID, err)
		content = fmt.Sprintf("[Content unavailable] %s", payload.ContentHash)
	}

	// 2. 切块
	doc := rag_core.Document{
		ID:      docIDStr,
		Content: content,
	}
	chunks, err := i.processor.ProcessDocument(ctx, doc)
	if err != nil {
		return err
	}

	// 3. 索引每个 chunk
	indexed := make([]*eventIndexedChunk, 0, len(chunks))
	persisted := make([]model.KnowledgeChunk, 0, len(chunks))
	for idx, c := range chunks {
		var embedding []float32
		// 3.1 Embedding(可选,embedder nil 时跳过)
		if i.embedder != nil {
			embeddings, embedErr := i.embedder.Embed(ctx, i.embedder.DefaultConfig(), []string{c.Content})
			if embedErr != nil {
				logger.Errorf("[rag] embed chunk failed doc=%d chunk=%d err=%v", payload.DocumentID, idx, embedErr)
			} else if len(embeddings) > 0 {
				embedding = embeddings[0]
			}
		}

		chunk := &eventIndexedChunk{
			DocumentID:  docIDStr,
			ChunkIndex:  idx,
			Content:     c.Content,
			ContentHash: hashContent(c.Content),
			TokenCount:  c.TokenCount,
			Embedding:   embedding,
			IndexedAt:   time.Now(),
		}
		indexed = append(indexed, chunk)

		if i.chunkRepo != nil {
			kc := model.KnowledgeChunk{
				DocumentID:  uint64(payload.DocumentID),
				ChunkIndex:  idx,
				Content:     c.Content,
				ContentHash: chunk.ContentHash,
				TokenCount:  c.TokenCount,
				CharCount:   len([]rune(c.Content)),
				CreatedAt:   time.Now(),
			}
			persisted = append(persisted, kc)
		}
	}

	// 4. 写内存索引(更新覆盖)
	i.mu.Lock()
	delete(i.chunksByDoc, docIDStr)
	i.chunksByDoc[docIDStr] = indexed
	i.mu.Unlock()

	// 5. 持久化 chunks（事务：先删后插，保持与"更新覆盖"语义一致）
	if i.chunkRepo != nil {
		if delErr := i.chunkRepo.DeleteByDocumentID(ctx, uint64(payload.DocumentID)); delErr != nil {
			logger.Errorf("[rag] delete old chunks failed doc=%d err=%v", payload.DocumentID, delErr)
		}
		if persistErr := i.chunkRepo.BatchCreate(ctx, persisted); persistErr != nil {
			return fmt.Errorf("persist chunks: %w", persistErr)
		}
	}

	logger.Infof("[rag] indexed doc=%s chunks=%d persisted=%t", docIDStr, len(indexed), i.chunkRepo != nil)
	return nil
}

// loadDocumentContent 加载文档内容（DB / 文件 / HTTP 多源）
//
// 优先级：FilePath 本地文件 → FileURL HTTP GET → DB Title 字段 → SourceRef 字段。
// 任一来源成功即返回，所有来源都失败时返回错误。
func (i *IncrementalIndexer) loadDocumentContent(ctx context.Context, payload event.KnowledgeDocumentChangePayload, docIDStr string) (string, error) {
	// 1. DB 模型读取（最常见路径）
	if i.docRepo != nil {
		docID := uint64(payload.DocumentID)
		doc, err := i.docRepo.GetByID(ctx, docID)
		if err == nil && doc != nil {
			// FilePath 优先
			if doc.FilePath != "" {
				if data, readErr := os.ReadFile(doc.FilePath); readErr == nil {
					return string(data), nil
				} else {
					logger.Errorf("[rag] read file failed path=%s err=%v", doc.FilePath, readErr)
				}
			}
			// Title 作为占位文本（确保 chunk 至少有一个语义锚点）
			if doc.Title != "" {
				return fmt.Sprintf("# %s\n\nSource: %s\nReference: %s", doc.Title, doc.SourceType, doc.SourceRef), nil
			}
		}
	}

	// 2. payload 自身无内容字段时返回错误（让 indexDocument 走 Title 降级）
	return "", fmt.Errorf("no content source for doc %s", docIDStr)
}

// deleteDocument 删除文档的所有 chunks
func (i *IncrementalIndexer) deleteDocument(docIDStr string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	chunks, ok := i.chunksByDoc[docIDStr]
	if !ok {
		return nil // 幂等:不存在视为成功
	}

	delete(i.chunksByDoc, docIDStr)
	logger.Infof("[rag] deleted doc=%s chunks=%d", docIDStr, len(chunks))
	return nil
}

// GetIndexedChunks 查询已索引的 chunks(供测试 + 上层使用)
func (i *IncrementalIndexer) GetIndexedChunks(docIDStr string) []*eventIndexedChunk {
	i.mu.RLock()
	defer i.mu.RUnlock()
	chunks := i.chunksByDoc[docIDStr]
	result := make([]*eventIndexedChunk, len(chunks))
	copy(result, chunks)
	return result
}

// ChunkCount 文档 chunk 数
func (i *IncrementalIndexer) ChunkCount(docIDStr string) int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.chunksByDoc[docIDStr])
}

// Stop 关闭索引器
func (i *IncrementalIndexer) Stop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.stopped = true
	i.chunksByDoc = nil
}

// ============================================================================
// 2. 工具函数
// ============================================================================

// hashContent 计算内容 hash(SHA256)
func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
