package agent_runtime

import (
	"os"
	"path/filepath"
	"testing"

	"hivemtk-user/internal/aiagent/knowledge/model"
	rag "hivemtk-user/internal/aiagent/rag/incremental"
	"hivemtk-user/internal/pkg/testutil"
)


// e2eDocContent E2E 测试用的文档内容,长度足以触发切块（默认 chunkSize 约 500 字符）
const e2eDocContent = "营销自动化系统支持多渠道统一管理。" +
	"系统通过统一的接入网关,将微信公众号、企业微信、小程序、APP、网页等渠道汇总," +
	"并使用统一的用户身份识别机制。\n\n" +
	"自动化营销引擎基于事件总线和策略引擎,可以根据用户行为(如:访问、点击、留言、购买)" +
	"触发预设的 SOP(标准操作流程)。SOP 可以包括:发送优惠券、推送消息、分配人工客服、记录客户画像等动作。\n\n" +
	"AI 智能体(AI Agent)模块基于大语言模型和 RAG 知识库,可以为客户提供 7x24 小时的" +
	"智能问答、个性化推荐、订单查询、售后处理等服务。\n\n" +
	"RAG 知识库通过向量化检索技术,将企业的产品手册、常见问题、营销话术等内容" +
	"索引为可语义检索的知识片段。AI 智能体回答问题时,先从 RAG 知识库中检索相关内容," +
	"再结合大语言模型生成回答,确保回答的准确性和时效性。\n"

// setupE2EKnowledgeTestEnv 创建单文档 E2E 测试环境
func setupE2EKnowledgeTestEnv(t *testing.T, content string) (*rag.IncrementalIndexer, uint64) {
	t.Helper()
	database := testutil.NewTestDB(t, &model.KnowledgeDocument{}, &model.KnowledgeChunk{})

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "doc.txt")
	if err := os.WriteFile(filePath, []byte(e2eDocContent), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	doc := &model.KnowledgeDocument{
		Title:       "E2E Test Doc",
		ProductID:   "1",
		SourceType:  model.SourceTypeText,
		FilePath:    filePath,
		EmbedStatus: model.EmbedStatusPending,
		Status:      1,
	}
	if err := database.Create(doc).Error; err != nil {
		t.Fatalf("create test doc: %v", err)
	}
	return rag.NewIncrementalIndexer(nil, nil, database), doc.ID
}

// setupE2EKnowledgeTestEnvMulti 创建多文档 E2E 测试环境
func setupE2EKnowledgeTestEnvMulti(t *testing.T, n int) (*rag.IncrementalIndexer, []uint64) {
	t.Helper()
	database := testutil.NewTestDB(t, &model.KnowledgeDocument{}, &model.KnowledgeChunk{})

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "doc.txt")
	if err := os.WriteFile(filePath, []byte(e2eDocContent), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	docIDs := make([]uint64, 0, n)
	for i := 0; i < n; i++ {
		doc := &model.KnowledgeDocument{
			Title:       "E2E Multi Doc",
			ProductID:   "1",
			SourceType:  model.SourceTypeText,
			FilePath:    filePath,
			EmbedStatus: model.EmbedStatusPending,
			Status:      1,
		}
		if err := database.Create(doc).Error; err != nil {
			t.Fatalf("create test doc %d: %v", i, err)
		}
		docIDs = append(docIDs, doc.ID)
	}
	return rag.NewIncrementalIndexer(nil, nil, database), docIDs
}

