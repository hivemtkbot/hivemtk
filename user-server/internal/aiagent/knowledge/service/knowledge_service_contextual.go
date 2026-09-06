package service

import (
	"context"
	"os"
	"strings"

	"hivemtk-user/internal/aiagent/llm"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
	"hivemtk-user/internal/pkg/utils/logger"
)

func contextualEnhanceEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("RAG_CONTEXTUAL_ENHANCE"))) {
	case "0", "false", "off":
		return false
	default:
		return true
	}
}

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
		WithScenario(llm.ScenarioLongSummary)
	enhancer := ragretrieval.NewContextualRetrievalEnhancer(s.db, chatClient, s.embeddingSvc, nil)
	if err := enhancer.EnhanceDocument(ctx, documentID); err != nil {
		logger.Warnf("[R-3] contextual enhance failed doc=%d (non-blocking): %v", documentID, err)
		return
	}
	logger.Infof("[R-3] contextual enhance done doc=%d", documentID)
}
