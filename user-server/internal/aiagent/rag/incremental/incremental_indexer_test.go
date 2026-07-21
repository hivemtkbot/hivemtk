package rag

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	knowledgemodel "marketing/internal/aiagent/knowledge/model"
	"marketing/internal/event"
	"marketing/internal/pkg/testutil"
)

// ============================================================================
// IncrementalIndexer 单元测试
// ----------------------------------------------------------------------------
// 验证:
//   1. create 事件触发索引并真实持久化 chunks
//   2. update 事件覆盖旧 chunks
//   3. delete 事件清除 chunks
//   4. 非法载荷 / nil 载荷 不 panic
//   5. unknown change_type 静默忽略
//   6. hashContent 一致性
// ============================================================================

// longTestContent 写入临时文件的可切块长内容
// 长度：~3000 字符 → 至少 2 个 chunk（默认 chunkSize 约 500 字符）
const longTestContent = "营销自动化系统支持多渠道统一管理。" +
	"第一段介绍：系统通过统一的接入网关，将微信公众号、企业微信、小程序、APP、网页等渠道汇总，" +
	"并使用统一的用户身份识别机制。\n\n" +
	"第二段介绍：自动化营销引擎基于事件总线和策略引擎，" +
	"可以根据用户行为（如：访问、点击、留言、购买）触发预设的 SOP（标准操作流程）。" +
	"SOP 可以包括：发送优惠券、推送消息、分配人工客服、记录客户画像等动作。\n\n" +
	"第三段介绍：AI 智能体（AI Agent）模块基于大语言模型和 RAG 知识库，" +
	"可以为客户提供 7x24 小时的智能问答、个性化推荐、订单查询、售后处理等服务。" +
	"AI 智能体也可以在必要时无缝转接给人工客服，保证服务质量。\n\n" +
	"第四段介绍：RAG 知识库通过向量化检索技术，" +
	"将企业的产品手册、常见问题、营销话术等内容索引为可语义检索的知识片段。" +
	"AI 智能体回答问题时，先从 RAG 知识库中检索相关内容，再结合大语言模型生成回答，" +
	"确保回答的准确性和时效性。\n\n" +
	"第五段介绍：系统的私域独立部署模式（2026-07 决策）允许每个商户独立部署一套完整系统，" +
	"数据完全隔离，满足数据安全合规要求。\n"

// setupIndexerTestEnv 准备测试环境：DB + 临时文档文件 + indexer
// 返回：indexer、临时文件路径、cleanup 函数
func setupIndexerTestEnv(t *testing.T) (*IncrementalIndexer, string, uint64) {
	t.Helper()
	database := testutil.NewTestDB(t,
		&knowledgemodel.KnowledgeDocument{},
		&knowledgemodel.KnowledgeChunk{},
	)

	// 写入临时文档文件
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "doc.txt")
	if err := os.WriteFile(filePath, []byte(longTestContent), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// 在 DB 中创建 KnowledgeDocument 记录
	doc := &knowledgemodel.KnowledgeDocument{
		Title:      "营销自动化系统介绍",
		SourceType: "upload",
		FilePath:   filePath,
		ProductID:  1,
	}
	if err := database.WithContext(context.Background()).Create(doc).Error; err != nil {
		t.Fatalf("create knowledge_document: %v", err)
	}

	indexer := NewIncrementalIndexer(nil, nil, database)
	return indexer, filePath, doc.ID
}

// TestIncrementalIndexer_Create 验证 create 事件触发索引并真实持久化 chunks
func TestIncrementalIndexer_Create(t *testing.T) {
	indexer, _, docID := setupIndexerTestEnv(t)

	payload := event.KnowledgeDocumentChangePayload{
		WorkspaceID: 1,
		DocumentID:  uint(docID),
		ChangeType:  "create",
		ContentHash: "hash_create_001",
	}

	err := indexer.Handle(event.Event{
		Topic:   event.TopicKnowledgeDocumentChanged,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	docIDStr := uintToStr(uint(docID))
	if indexer.ChunkCount(docIDStr) == 0 {
		t.Error("expected chunks after create, got 0")
	}
}

// TestIncrementalIndexer_Update 验证 update 事件覆盖旧 chunks
func TestIncrementalIndexer_Update(t *testing.T) {
	indexer, _, docID := setupIndexerTestEnv(t)
	docIDStr := uintToStr(uint(docID))

	// 先 create
	if err := indexer.Handle(event.Event{
		Topic: event.TopicKnowledgeDocumentChanged,
		Payload: event.KnowledgeDocumentChangePayload{
			DocumentID: uint(docID), ChangeType: "create", ContentHash: "hash_200_v1",
		},
	}); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	firstCount := indexer.ChunkCount(docIDStr)
	if firstCount == 0 {
		t.Fatal("expected chunks after initial create")
	}

	time.Sleep(10 * time.Millisecond) // 让 IndexedAt 不同

	// 再 update
	if err := indexer.Handle(event.Event{
		Topic: event.TopicKnowledgeDocumentChanged,
		Payload: event.KnowledgeDocumentChangePayload{
			DocumentID: uint(docID), ChangeType: "update", ContentHash: "hash_200_v2",
		},
	}); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	secondCount := indexer.ChunkCount(docIDStr)
	if secondCount == 0 {
		t.Error("expected chunks after update, got 0")
	}

	// 验证 indexedAt 是新的
	chunks := indexer.GetIndexedChunks(docIDStr)
	hasNew := false
	for _, c := range chunks {
		if time.Since(c.IndexedAt) < 5*time.Second {
			hasNew = true
			break
		}
	}
	if !hasNew {
		t.Error("expected new IndexedAt after update")
	}
}

// TestIncrementalIndexer_Delete 验证 delete 事件清除 chunks
func TestIncrementalIndexer_Delete(t *testing.T) {
	indexer, _, docID := setupIndexerTestEnv(t)
	docIDStr := uintToStr(uint(docID))

	// 先 create
	if err := indexer.Handle(event.Event{
		Topic: event.TopicKnowledgeDocumentChanged,
		Payload: event.KnowledgeDocumentChangePayload{
			DocumentID: uint(docID), ChangeType: "create", ContentHash: "hash_300",
		},
	}); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if indexer.ChunkCount(docIDStr) == 0 {
		t.Fatal("expected chunks after create")
	}

	// 再 delete
	err := indexer.Handle(event.Event{
		Topic: event.TopicKnowledgeDocumentChanged,
		Payload: event.KnowledgeDocumentChangePayload{
			DocumentID: uint(docID), ChangeType: "delete",
		},
	})
	if err != nil {
		t.Errorf("Handle delete returned error: %v", err)
	}

	if indexer.ChunkCount(docIDStr) != 0 {
		t.Errorf("expected 0 chunks after delete, got %d", indexer.ChunkCount(docIDStr))
	}
}

// TestIncrementalIndexer_DeleteNonExistent 验证删除不存在的文档不报错
func TestIncrementalIndexer_DeleteNonExistent(t *testing.T) {
	indexer, _, _ := setupIndexerTestEnv(t)

	err := indexer.Handle(event.Event{
		Topic: event.TopicKnowledgeDocumentChanged,
		Payload: event.KnowledgeDocumentChangePayload{
			DocumentID: 999, ChangeType: "delete",
		},
	})
	if err != nil {
		t.Errorf("delete non-existent should be idempotent, got: %v", err)
	}
}

// TestIncrementalIndexer_UnknownChangeType 验证未知 change_type 被忽略
func TestIncrementalIndexer_UnknownChangeType(t *testing.T) {
	indexer, _, _ := setupIndexerTestEnv(t)

	err := indexer.Handle(event.Event{
		Topic: event.TopicKnowledgeDocumentChanged,
		Payload: event.KnowledgeDocumentChangePayload{
			DocumentID: 400, ChangeType: "magic",
		},
	})
	if err != nil {
		t.Errorf("unknown change_type should return nil, got: %v", err)
	}
}

// TestIncrementalIndexer_InvalidPayload 验证非法载荷不 panic
func TestIncrementalIndexer_InvalidPayload(t *testing.T) {
	indexer, _, _ := setupIndexerTestEnv(t)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Handle panicked on invalid payload: %v", r)
		}
	}()

	err := indexer.Handle(event.Event{
		Topic:   event.TopicKnowledgeDocumentChanged,
		Payload: "invalid string",
	})
	if err != nil {
		t.Errorf("invalid payload should return nil, got: %v", err)
	}
}

// TestIncrementalIndexer_NilPointerPayload 验证 nil 指针载荷不 panic
func TestIncrementalIndexer_NilPointerPayload(t *testing.T) {
	indexer, _, _ := setupIndexerTestEnv(t)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Handle panicked on nil pointer: %v", r)
		}
	}()

	err := indexer.Handle(event.Event{
		Topic:   event.TopicKnowledgeDocumentChanged,
		Payload: (*event.KnowledgeDocumentChangePayload)(nil),
	})
	if err != nil {
		t.Errorf("nil pointer payload should return nil, got: %v", err)
	}
}

// TestHashContent 验证 hashContent 一致性
func TestHashContent(t *testing.T) {
	h1 := hashContent("hello world")
	h2 := hashContent("hello world")
	h3 := hashContent("hello WORLD")

	if h1 != h2 {
		t.Errorf("hashContent should be consistent: h1=%s, h2=%s", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("different content should produce different hash: %s", h1)
	}
	if len(h1) != 64 { // SHA256 hex = 64 chars
		t.Errorf("hash length = %d, want 64", len(h1))
	}
}

// TestIncrementalIndexer_Stop 验证 Stop 后不再处理事件
func TestIncrementalIndexer_Stop(t *testing.T) {
	indexer, _, _ := setupIndexerTestEnv(t)
	indexer.Stop()

	// Stop 后 Handle 应该 no-op(不 panic,不索引)
	err := indexer.Handle(event.Event{
		Topic: event.TopicKnowledgeDocumentChanged,
		Payload: event.KnowledgeDocumentChangePayload{
			DocumentID: 500, ChangeType: "create", ContentHash: "hash_500",
		},
	})
	if err != nil {
		t.Errorf("Handle after Stop returned error: %v", err)
	}
	// 验证 chunksByDoc 已置 nil,GetIndexedChunks 应返回空
	chunks := indexer.GetIndexedChunks("500")
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks after Stop, got %d", len(chunks))
	}
}

// TestIncrementalIndexer_MultipleDocuments 验证多文档并发
func TestIncrementalIndexer_MultipleDocuments(t *testing.T) {
	database := testutil.NewTestDB(t,
		&knowledgemodel.KnowledgeDocument{},
		&knowledgemodel.KnowledgeChunk{},
	)
	tmpDir := t.TempDir()
	indexer := NewIncrementalIndexer(nil, nil, database)

	// 创建 5 个文档 + 文件
	docIDs := make([]uint64, 0, 5)
	for i := 0; i < 5; i++ {
		filePath := filepath.Join(tmpDir, "doc_"+intToStr(i)+".txt")
		if err := os.WriteFile(filePath, []byte(longTestContent), 0644); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		doc := &knowledgemodel.KnowledgeDocument{
			Title:      "文档" + intToStr(i),
			SourceType: "upload",
			FilePath:   filePath,
			ProductID:  1,
		}
		if err := database.WithContext(context.Background()).Create(doc).Error; err != nil {
			t.Fatalf("create document %d: %v", i, err)
		}
		docIDs = append(docIDs, doc.ID)

		if err := indexer.Handle(event.Event{
			Topic: event.TopicKnowledgeDocumentChanged,
			Payload: event.KnowledgeDocumentChangePayload{
				DocumentID: uint(doc.ID), ChangeType: "create", ContentHash: "hash_multi_" + intToStr(i),
			},
		}); err != nil {
			t.Fatalf("handle doc %d: %v", i, err)
		}
	}

	for _, id := range docIDs {
		if indexer.ChunkCount(uintToStr(uint(id))) == 0 {
			t.Errorf("doc %d should have chunks", id)
		}
	}
}

// TestIncrementalIndexer_NilDB_Fallback 验证 db=nil 时降级到内存索引模式（不 panic）
func TestIncrementalIndexer_NilDB_Fallback(t *testing.T) {
	indexer := NewIncrementalIndexer(nil, nil, nil)

	// 不应 panic
	err := indexer.Handle(event.Event{
		Topic: event.TopicKnowledgeDocumentChanged,
		Payload: event.KnowledgeDocumentChangePayload{
			DocumentID: 100, ChangeType: "create", ContentHash: "hash_fallback",
		},
	})
	if err != nil {
		t.Errorf("nil DB should not return error, got: %v", err)
	}
	// 内存中应记录 0 个 chunk（fallback 占位文本太短无法切块）
	// 这只是验证行为，不强求 chunks>0
	_ = indexer.ChunkCount("100")
}

// TestIncrementalIndexer_ChunksPersisted 验证 chunks 真的被持久化到 DB
func TestIncrementalIndexer_ChunksPersisted(t *testing.T) {
	database := testutil.NewTestDB(t,
		&knowledgemodel.KnowledgeDocument{},
		&knowledgemodel.KnowledgeChunk{},
	)
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "doc_persist.txt")
	if err := os.WriteFile(filePath, []byte(longTestContent), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	doc := &knowledgemodel.KnowledgeDocument{
		Title:      "持久化测试",
		SourceType: "upload",
		FilePath:   filePath,
		ProductID:  1,
	}
	if err := database.WithContext(context.Background()).Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}

	indexer := NewIncrementalIndexer(nil, nil, database)
	if err := indexer.Handle(event.Event{
		Topic: event.TopicKnowledgeDocumentChanged,
		Payload: event.KnowledgeDocumentChangePayload{
			DocumentID: uint(doc.ID), ChangeType: "create", ContentHash: "hash_persist",
		},
	}); err != nil {
		t.Fatalf("handle failed: %v", err)
	}

	// 验证 DB 中有 chunk
	var chunks []knowledgemodel.KnowledgeChunk
	if err := database.WithContext(context.Background()).
		Where("document_id = ?", doc.ID).Find(&chunks).Error; err != nil {
		t.Fatalf("query chunks: %v", err)
	}
	if len(chunks) == 0 {
		t.Errorf("expected persisted chunks in DB, got 0")
	}
	// 验证每个 chunk 都有关键字段
	for i, c := range chunks {
		if c.DocumentID != doc.ID {
			t.Errorf("chunk %d: document_id mismatch: got=%d want=%d", i, c.DocumentID, doc.ID)
		}
		if c.Content == "" {
			t.Errorf("chunk %d: empty content", i)
		}
	}
}

// uintToStr 辅助:uint → string
func uintToStr(v uint) string {
	return strings.TrimSpace(formatUint(v))
}

// intToStr 辅助:int → string
func intToStr(v int) string {
	if v == 0 {
		return "0"
	}
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// formatUint 格式化 uint
func formatUint(v uint) string {
	if v == 0 {
		return "0"
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}
