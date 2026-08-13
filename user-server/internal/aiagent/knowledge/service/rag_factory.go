package service

import (
	"hivemtk-user/internal/aiagent/llm"
	rag_core "hivemtk-user/internal/aiagent/rag/core"
	ragcustomerservice "hivemtk-user/internal/aiagent/rag/customer_service"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/pkg/utils/logger"
	"time"

	"gorm.io/gorm"
)

type RAGStack struct {
	Retrieval ragretrieval.RagRetrievalService
	Customer  ragcustomerservice.RagCustomerService
}

func NewRAGStack(db *gorm.DB, glossary ragcustomerservice.GlossaryRenderer, calibrator ragcustomerservice.OutputCalibrator) *RAGStack {
	embeddingSvc := llm.NewEmbeddingService()
	// Embedding 走本地 TEI（真实 bge-m3，EmbeddingDim 维）
	embedder := rag_core.NewRemoteEmbedder(EmbeddingDim)
	_ = embeddingSvc
	_ = embedder

	vectorizer := &ragretrieval.Vectorizer{}
	indexManager := ragretrieval.NewInMemoryIndexManager(EmbeddingDim)
	storage := ragretrieval.NewInMemoryStorage()
	// RAG 检索缓存：REDIS_HOST 配置时复用全局 Redis 缓存（跨实例共享检索结果，
	// 减少重复向量化/检索）；未配置则回退进程内内存缓存（向后兼容单实例）。
	var ragCache ragretrieval.CacheInterface = ragretrieval.NewInMemoryCache()
	if cache.GlobalIsRedis() {
		ragCache = ragretrieval.NewRedisBackedCache(cache.GetGlobalCache())
	}

	retrievalCfg := &ragretrieval.RetrievalConfig{
		DefaultTopK:                DefaultTopK,
		DefaultSimilarityThreshold: DefaultSimilarityThreshold,
		MaxTopK:                    10,
		CacheTTL:                   30 * time.Minute,
		MaxChunkSize:               MaxSearchListSize,
		DefaultChunkOverlap:        200,
	}

	retrieval := ragretrieval.NewRagRetrievalService(vectorizer, indexManager, storage, ragCache, retrievalCfg)

	// 重排：本地 TEI + bge-reranker-v2-m3（RERANK_ENABLED=false 时自动跳过）
	if rc := ragretrieval.DefaultRerankConfig(); rc.Enabled {
		retrieval.SetReranker(ragretrieval.NewLocalReranker())
	}

	dialogCfg := &ragcustomerservice.DialogManagerConfig{
		DefaultMaxHistoryLength: 10,
		DefaultSessionTimeout:   30 * time.Minute,
		SessionCleanupInterval:  5 * time.Minute,
	}

	var dialogManager ragcustomerservice.DialogManagerInterface
	if db != nil {
		dialogManager = ragcustomerservice.NewPgDialogManager(db, dialogCfg)
		logger.Info("[RAGFactory] 使用 PostgreSQL 持久化会话")
	} else {
		dialogManager = ragcustomerservice.NewInMemoryDialogManager(dialogCfg)
		logger.Info("[RAGFactory] 使用内存会话（无DB）")
	}

	contextUnderstanding := &ragcustomerservice.ContextUnderstandingServiceImpl{}

	var llmSvc ragcustomerservice.LLMServiceInterface = nil
	respGenCfg := &ragcustomerservice.ResponseGenerationConfig{
		DefaultTemperature: DefaultTemperature,
		DefaultMaxTokens:   DefaultMaxTokens,
	}
	responseGenerator := ragcustomerservice.NewResponseGeneratorImpl(llmSvc, respGenCfg)
	if glossary != nil {
		responseGenerator = responseGenerator.WithGlossaryRenderer(glossary)
	}
	if calibrator != nil {
		responseGenerator = responseGenerator.WithOutputCalibrator(calibrator)
	}

	qaCfg := &ragcustomerservice.QualityAssessmentConfig{
		RelevanceWeight:   0.3,
		AccuracyWeight:    0.3,
		CoherenceWeight:   0.2,
		HelpfulnessWeight: 0.2,
		MinimumScore:      0.5,
	}
	qualityAssessor := ragcustomerservice.NewQualityAssessorImpl(qaCfg)
	feedbackLearner := &ragcustomerservice.SimpleFeedbackLearner{}

	csConfig := &ragcustomerservice.CustomerServiceConfig{
		DefaultMaxHistoryLength: 10,
		DefaultTimeout:          30 * 60,
		DefaultTemperature:      DefaultTemperature,
		DefaultMaxTokens:        DefaultMaxTokens,
		RetrievalTopK:           DefaultTopK,
		RetrievalThreshold:      DefaultSimilarityThreshold,
		CacheTTL:                30 * time.Minute,
		MaxConcurrentSessions:   100,
		SessionCleanupInterval:  5 * time.Minute,
		EnableFallback:          true,
		FallbackResponse:        "感谢您的消息，我们会尽快回复您！",
	}

	customerService := ragcustomerservice.NewRagCustomerService(
		dialogManager, contextUnderstanding, responseGenerator,
		qualityAssessor, feedbackLearner, retrieval, csConfig,
	)

	logger.Info("[RAGFactory] RAG 栈初始化完成")

	return &RAGStack{
		Retrieval: retrieval,
		Customer:  customerService,
	}
}
