// Package service 知识库子域 —— Contextual Retrieval 接线（M6 R-3）
//
// 核实结论（MASTER_COMPETITIVE_DECISIONS.md R-3）：
//   - contextual_retrieval 实现完整：ragretrieval.ContextualRetrievalEnhancer
//     在索引期对每个 chunk 调用 LLM 生成 50-100 token 上下文摘要，拼在 chunk
//     内容之前写入 knowledge_chunks.contextual_context，并重新 embedding；
//     BM25 召回优先走 contextual_tsv（含上下文+正文的 tsvector）。
//   - EnableHyDE=false 与本决策无关：HyDE 是查询时假设文档改写，
//     R-3 要求的是入库期 chunk 上下文前缀（非 HyDE），保持 EnableHyDE=false 不动。
//   - 缺口是接线：EnhanceDocument 此前无任何调用方（死代码）。
//     本文件把 enhancer 挂入 processDocumentAsync 入库管线。
//
// 开关：默认启用；RAG_CONTEXTUAL_ENHANCE=0/false/off 显式关闭
// （LLM 成本敏感的批量导入场景可临时关闭）。
package service

import (
	"context"
	"os"
	"strings"

	"hivemtk-user/internal/aiagent/llm"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
	"hivemtk-user/internal/pkg/utils/logger"
)

// contextualEnhanceEnabled R-3 上下文增强开关（默认启用）
func contextualEnhanceEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("RAG_CONTEXTUAL_ENHANCE"))) {
	case "0", "false", "off":
		return false
	default:
		return true
	}
}

// enhanceDocumentContextual 对文档全部 chunk 做 Contextual Retrieval 增强
//
// 失败仅告警不阻断入库主流程（文档已向量化可用，上下文前缀属增量优化）。
// embedding 使用全局默认配置（TEI bge-m3）；per-product 自定义 embedding 的
// 文档如维度与默认不一致，enhancer 内部 re-embed 会失败并被日志记录，不影响原向量。
func (s *KnowledgeService) enhanceDocumentContextual(ctx context.Context, documentID uint64) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[R-3] contextual enhance panic doc=%d: %v", documentID, r)
		}
	}()
	if s.db == nil || s.llmSvc == nil || s.embeddingSvc == nil {
		return
	}
	chatClient := ragretrieval.NewDispatcherChatAdapter(llm.NewDispatcher(s.llmSvc)).
		WithScenario(llm.ScenarioLongSummary) // llm_chat.go 建议：Contextual Retrieval 用长上下文场景
	enhancer := ragretrieval.NewContextualRetrievalEnhancer(s.db, chatClient, s.embeddingSvc, nil)
	if err := enhancer.EnhanceDocument(ctx, documentID); err != nil {
		logger.Warnf("[R-3] contextual enhance failed doc=%d (non-blocking): %v", documentID, err)
		return
	}
	logger.Infof("[R-3] contextual enhance done doc=%d", documentID)
}
