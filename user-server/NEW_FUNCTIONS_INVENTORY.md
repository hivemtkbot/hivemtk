# user-server 中 New* 标识符完整清单

> 生成时间: 2026-07-25
> 统计: 导出构造函数 `func NewXxx` = 950 · 非导出 `func newXxx` = 99 · 方法 = 0 · 类型/结构体 = 0 · 文件/目录 = 0 · **合计 1049**
> 说明: 所有 New* 标识符均为函数（Go 惯例的构造函数/工厂函数），无任何 New 开头的方法、类型或文件。

## cmd/embedding-server
  - `newTestServer`  (main_test.go:15) [test]

## internal/aiagent/agent/auto_reply
  - `NewAutoReplyIntegrationService`  (auto_reply_integration_service.go:23)
  - `NewDefaultAutoReplyIntegrationService`  (default_implementations.go:11)
  - `NewReplyProcessorImpl`  (reply_processor.go:19)
  - `NewRuleBasedMatcherImpl`  (rule_matcher.go:24)
  - `NewDefaultRuleBasedMatcher`  (rule_matcher.go:76)

## internal/aiagent/agent/bridge
  - `NewSalesEngineBridge`  (sales_bridge.go:33)

## internal/aiagent/agent/browser
  - `NewAssistant`  (assistant.go:26)
  - `NewAutoReplyBot`  (auto_reply.go:52)
  - `NewAutoReplyManager`  (auto_reply_manager.go:17)
  - `NewInMemoryDedup`  (message_dedup.go:21)
  - `NewRateLimiter`  (rate_limiter.go:54)
  - `NewSimpleRuleHandler`  (reply_handler.go:109)
  - `NewIntegrationReplyHandler`  (reply_handler.go:34)
  - `NewSliderSolver`  (slider_solver.go:39)
  - `NewXianyuWebSocket`  (xianyu_ws.go:60)

## internal/aiagent/agent/runtime
  - `NewDefaultAlignmentScorer`  (alignment_stage.go:32)
  - `NewDefaultAlignmentScorerWith`  (alignment_stage.go:37)
  - `NewAssetBundleLoader`  (asset_bundle_loader.go:52)
  - `NewChatHistoryRedisAdapter`  (chat_history_redis.go:43)
  - `NewCoreDataFlowOrchestrator`  (dataflow_orchestrator.go:99)
  - `NewEventSubscriber`  (event_subscriber.go:35)
  - `NewDefaultCrisisDetector`  (gatekeeper_stage.go:37)
  - `newLocalReplyGuard`  (idempotency.go:43)
  - `NewInferenceCycleWithConfig`  (inference_cycle.go:113)
  - `NewInferenceCycle`  (inference_cycle.go:87)
  - `NewInferenceCycleWithStages`  (inference_cycle.go:99)
  - `NewPGAgentContextLoader`  (loader.go:34)
  - `newTestAgent`  (loader_test.go:227) [test]
  - `NewLocalLLMClient`  (local_llm_client.go:55)
  - `NewKeywordIntentRecognizer`  (perception_stage.go:203)
  - `NewDefaultPerceptionStage`  (perception_stage.go:32)
  - `NewDefaultPerceptionStageWith`  (perception_stage.go:40)
  - `NewKeywordSentimentAnalyzer`  (perception_stage.go:96)
  - `NewDefaultTaskPlanner`  (planner_stage.go:35)
  - `NewDefaultTaskPlannerWithFAQ`  (planner_stage.go:40)
  - `NewAgentRuntime`  (types.go:236)

## internal/aiagent/agent/tooluse
  - `NewFollowTaskCreateTool`  (business_tools.go:249)
  - `NewFollowTaskUpdateTool`  (business_tools.go:359)
  - `NewOrderLookupTool`  (business_tools.go:455)
  - `NewBusinessToolDeps`  (business_tools.go:48)
  - `NewAfterSaleCreateTool`  (business_tools.go:532)
  - `NewBusinessToolDepsWithDB`  (business_tools.go:56)
  - `NewAfterSaleQueryTool`  (business_tools.go:603)
  - `NewBusinessToolDepsWithPorts`  (business_tools.go:68)
  - `NewCircuitBreakerRegistry`  (circuit_breaker.go:193)
  - `newToolCircuit`  (circuit_breaker.go:97)
  - `NewCustomerSearchTool`  (customer_tools.go:132)
  - `NewCustomerGetTool`  (customer_tools.go:226)
  - `NewCustomerCreateTool`  (customer_tools.go:297)
  - `NewCustomerUpdateTool`  (customer_tools.go:350)
  - `NewCustomerMergeTool`  (customer_tools.go:423)
  - `NewCustomerAddTagTool`  (customer_tools.go:474)
  - `NewCustomerRemoveTagTool`  (customer_tools.go:542)
  - `NewCustomerToolDeps`  (customer_tools.go:58)
  - `NewCustomerSegmentTool`  (customer_tools.go:610)
  - `NewCustomerToolDepsWithDB`  (customer_tools.go:67)
  - `NewCustomerToolDepsWithPort`  (customer_tools.go:83)
  - `NewAlertManager`  (db_audit_persister.go:302)
  - `NewCompositeAuditLogger`  (db_audit_persister.go:460)
  - `NewDBAuditLogger`  (db_audit_persister.go:76)
  - `NewDeadLetterReplayer`  (dead_letter.go:332)
  - `NewDeadLetterQueue`  (dead_letter.go:82)
  - `NewTokenBucketLimiter`  (decorator.go:710)
  - `NewExponentialBackoffPolicy`  (decorator.go:756)
  - `NewMemoryAuditLogger`  (decorator.go:816)
  - `NewMemoryCostTracker`  (decorator.go:876)
  - `NewDoubleInterceptOrchestrator`  (double_intercept.go:101)
  - `NewToolExecutor`  (executor.go:59)
  - `newMockTool`  (executor_test.go:28) [test]
  - `NewMemoryFeedbackSink`  (feedback_decorator.go:205)
  - `NewMonitoredFeedbackSink`  (feedback_decorator.go:350)
  - `NewRagSearchTool`  (knowledge_tools.go:112)
  - `NewKnowledgeFeedbackTool`  (knowledge_tools.go:285)
  - `NewKnowledgeAddDocTool`  (knowledge_tools.go:394)
  - `NewKnowledgeToolDeps`  (knowledge_tools.go:43)
  - `NewKnowledgeListKBTool`  (knowledge_tools.go:504)
  - `NewKnowledgeToolDepsWithDB`  (knowledge_tools.go:59)
  - `NewLoopGuard`  (loop_guard.go:97)
  - `NewWhitelistPermissionChecker`  (permission_whitelist.go:51)
  - `NewPrivateMessageToolDeps`  (private_message_tools.go:41)
  - `NewPrivateMessageToolDepsWithPort`  (private_message_tools.go:51)
  - `NewProviderRegistry`  (provider.go:131)
  - `NewWebReachAdapter`  (reach_integration_adapter.go:226)
  - `NewIntegrationReachAdapter`  (reach_integration_adapter.go:65)
  - `NewIntegrationReachAdapterFromDB`  (reach_integration_adapter.go:77)
  - `NewReachScheduleTool`  (reach_tools.go:1090)
  - `NewReachRecallTool`  (reach_tools.go:1171)
  - `NewReachHealthTool`  (reach_tools.go:1220)
  - `NewReachHistoryTool`  (reach_tools.go:1265)
  - `NewReachTemplateApplyTool`  (reach_tools.go:1333)
  - `NewReachAccountListTool`  (reach_tools.go:1412)
  - `NewReachTelegramSendTool`  (reach_tools.go:1536)
  - `NewReachWhatsAppSendTool`  (reach_tools.go:1613)
  - `NewReachFeishuSendTool`  (reach_tools.go:1700)
  - `NewReachToolDeps`  (reach_tools.go:171)
  - `NewReachWebSendTool`  (reach_tools.go:1786)
  - `NewReachToolDepsWithDB`  (reach_tools.go:178)
  - `NewReachToolDepsWithAdapter`  (reach_tools.go:202)
  - `NewReachSMSSendTool`  (reach_tools.go:348)
  - `NewReachEmailSendTool`  (reach_tools.go:424)
  - `NewReachWeComSendTool`  (reach_tools.go:493)
  - `NewReachWeixinSendTool`  (reach_tools.go:562)
  - `NewReachDouyinSendTool`  (reach_tools.go:627)
  - `NewReachKuaishouSendTool`  (reach_tools.go:696)
  - `NewReachXHSSendTool`  (reach_tools.go:765)
  - `NewReachDingTalkSendTool`  (reach_tools.go:834)
  - `NewReachCardSendTool`  (reach_tools.go:899)
  - `NewReachBatchTool`  (reach_tools.go:980)
  - `NewToolRegistry`  (registry.go:31)
  - `NewResultCache`  (result_cache.go:78)
  - `NewStreamStateMachine`  (stream_state_machine.go:146)
  - `NewToolRouter`  (tool_router.go:82)

## internal/aiagent/embedding
  - `NewProjector`  (embedding.go:51)
  - `NewLocalEmbedding`  (embedding.go:98)

## internal/aiagent/eval
  - `NewChrFEvaluator`  (chrf_eval.go:41)
  - `NewEvaluator`  (evaluator.go:48)
  - `NewDefaultLLMJudge`  (llm_judge.go:75)

## internal/aiagent/knowledge/controller
  - `NewKnowledgeBaseController`  (knowledge_base_controller.go:17)
  - `NewKnowledgeMerchantController`  (knowledge_merchant_controller.go:27)
  - `NewKnowledgeWorkspaceController`  (knowledge_workspace_controller.go:29)
  - `NewRagConfigController`  (rag_config_controller.go:21) **[web同名引用 ×1 · 相似度低(仅引用后端命名,无同名工具实现)]**

## internal/aiagent/knowledge/repository
  - `NewKBDocumentRepository`  (kb_document_repository.go:19)
  - `NewKnowledgeChunkRepository`  (knowledge_chunk_repository.go:19)
  - `NewKnowledgeDocumentRepository`  (knowledge_document_repository.go:18)
  - `NewKnowledgeImportLogRepository`  (knowledge_import_log_repository.go:17)
  - `NewExternalImportJobRepository`  (knowledge_merchant.go:152)
  - `NewKnowledgeFeedbackRepository`  (knowledge_merchant.go:18)
  - `NewKnowledgeAPITokenRepository`  (knowledge_merchant.go:79)
  - `NewKnowledgeOpenAPIRepository`  (knowledge_openapi_repository.go:18)
  - `NewKnowledgeSearchLogRepository`  (knowledge_search_log_repository.go:17)
  - `NewRagConfigRepository`  (rag_config_repository.go:18)

## internal/aiagent/knowledge/service
  - `NewKnowledgeBaseService`  (knowledge_base.go:30)
  - `NewKnowledgeBaseServiceWithDB`  (knowledge_base.go:40)
  - `NewKnowledgeMerchantService`  (knowledge_merchant.go:41)
  - `NewKnowledgeMerchantServiceWithDB`  (knowledge_merchant.go:58)
  - `NewKnowledgeService`  (knowledge_service.go:56)
  - `NewKnowledgeServiceWithDB`  (knowledge_service.go:61)
  - `newKnowledgeServiceWithDB`  (knowledge_service.go:65)
  - `NewKnowledgeStatisticsService`  (knowledge_statistics_service.go:33)
  - `NewKnowledgeStatisticsServiceWithDB`  (knowledge_statistics_service.go:38)
  - `newKnowledgeStatisticsServiceWithDB`  (knowledge_statistics_service.go:42)
  - `NewRagConfigService`  (rag_config_service.go:29)
  - `NewRAGStack`  (rag_factory.go:22)
  - `NewRagSearcher`  (rag_searcher.go:44)
  - `NewRagSearcherWithDB`  (rag_searcher.go:51)
  - `newRagSearcherWithDB`  (rag_searcher.go:55)

## internal/aiagent/llm
  - `newDispatcherBase`  (dispatcher.go:106)
  - `NewInMemoryAlertSink`  (dispatcher.go:1159)
  - `NewDispatcher`  (dispatcher.go:126)
  - `NewDispatcherFromConfig`  (dispatcher.go:141)
  - `NewLogEntry`  (dispatcher_observability.go:207)
  - `newTestDispatcher`  (dispatcher_test.go:9) [test]
  - `NewHashEmbeddingService`  (embedding_hash.go:21)
  - `NewEmbeddingService`  (embedding_service.go:101)
  - `NewEmbeddingServiceWithConfig`  (embedding_service.go:110)
  - `newEmbeddingSem`  (embedding_service.go:90)
  - `NewLLMService`  (llm_service.go:118)
  - `NewHTTPHealthChecker`  (provider_failover.go:130)
  - `NewProviderFailover`  (provider_failover.go:186)
  - `newFakeChecker`  (provider_failover_test.go:25) [test]
  - `newTestFailover`  (provider_failover_test.go:52) [test]
  - `NewReActAdapter`  (react_adapter.go:161)
  - `NewInMemoryTraceBus`  (trace_context.go:199)
  - `NewTraceContext`  (trace_context.go:68)

## internal/aiagent/rag/core
  - `NewMockEmbedder`  (rag_engine.go:117)
  - `NewRAGEngine`  (rag_engine.go:163)
  - `NewRAGEngineWithEmbedder`  (rag_engine.go:186)
  - `NewRemoteEmbedder`  (rag_engine.go:75)
  - `newTestRAGEngine`  (rag_engine_test.go:26) [test]

## internal/aiagent/rag/customer_service
  - `NewInMemoryDialogManager`  (dialog_manager.go:26)
  - `NewContextUnderstandingService`  (dialog_manager.go:336)
  - `NewLLMServiceAdapter`  (llm_adapter.go:16)
  - `NewPgDialogManager`  (pg_dialog_manager.go:21)
  - `NewQualityAssessorImpl`  (quality_assessor.go:25)
  - `NewRagCustomerService`  (rag_customer_service.go:43)
  - `NewDefaultRagCustomerService`  (rag_customer_service.go:446)
  - `NewSimpleFeedbackLearner`  (rag_customer_service.go:475)
  - `NewResponseGeneratorImpl`  (response_generator.go:122)
  - `NewRemoteLLMService`  (response_generator.go:496)

## internal/aiagent/rag/incremental
  - `NewIncrementalIndexer`  (incremental_indexer.go:76)

## internal/aiagent/rag/retrieval
  - `NewBGEM3Vectorizer`  (bge_m3_vectorizer.go:111)
  - `NewBM25Retriever`  (bm25_retriever.go:33)
  - `NewCachedEmbeddingClient`  (cached_embedding_client.go:63)
  - `NewContextualRetrievalEnhancer`  (contextual_retrieval.go:59)
  - `NewInMemoryCache`  (default_implementations.go:172)
  - `NewDefaultRagRetrievalService`  (default_implementations.go:246)
  - `NewInMemoryStorage`  (default_implementations.go:24)
  - `NewProductionRagRetrievalService`  (default_implementations.go:274)
  - `NewConfigurableRagRetrievalService`  (default_implementations.go:285)
  - `NewHybridSearcher`  (hybrid_searcher.go:116)
  - `NewHyDEGenerator`  (hyde_generator.go:51)
  - `NewInMemoryIndexManager`  (index_manager.go:22)
  - `NewFAISSIndexManager`  (index_manager.go:245)
  - `NewIndexManagerWithDB`  (index_manager_factory.go:21)
  - `NewIndexManager`  (index_manager_factory.go:34)
  - `NewDispatcherChatAdapter`  (llm_chat.go:48)
  - `NewLRUCache`  (lru_cache.go:26)
  - `NewMultiQueryGenerator`  (multi_query_generator.go:48)
  - `NewQueryRewriter`  (query_rewriter.go:80)
  - `newMockRedisClient`  (query_rewriter_test.go:32) [test]
  - `NewRagRetrievalService`  (rag_retrieval_service.go:73)
  - `NewMockStorage`  (rag_retrieval_service_test.go:183) [test]
  - `NewMockCache`  (rag_retrieval_service_test.go:268) [test]
  - `NewMockIndexManager`  (rag_retrieval_service_test.go:51) [test]
  - `NewRedisBackedCache`  (redis_cache.go:31)
  - `NewGoRedisAdapter`  (redis_client.go:38)
  - `NewLocalReranker`  (rerank.go:75)
  - `NewLocalRerankerWithConfig`  (rerank.go:81)
  - `newRerankScoreCache`  (rerank_advanced.go:103)
  - `NewCrossEncoderScorer`  (rerank_advanced.go:218)
  - `NewCrossEncoderReranker`  (rerank_advanced.go:284)
  - `NewRRFReranker`  (rerank_advanced.go:344)
  - `NewHybridReranker`  (rerank_advanced.go:446)
  - `NewDefaultRerankerWithConfig`  (rerank_advanced.go:547)
  - `NewRerankerToInterfaceAdapter`  (rerank_advanced.go:628)
  - `newMockReranker`  (rerank_advanced_test.go:40) [test]
  - `NewRRFFusion`  (rrf_fusion.go:31)
  - `NewRAGThreeTierService`  (three_tier.go:78)
  - `NewRAGThreeTierAdapter`  (three_tier_adapter.go:17)
  - `newThreeTierService`  (three_tier_test.go:86) [test]
  - `NewTranslationCache`  (translation_cache.go:60)
  - `NewVectorRetriever`  (vector_retriever.go:42)
  - `NewVectorizer`  (vectorizer.go:31)
  - `NewVectorizerFromConfig`  (vectorizer.go:56)

## internal/aiagent/rag/service
  - `NewRAGService`  (rag_service.go:52)

## internal/aiagent/vector
  - `NewRemoteVectorizer`  (vector_model.go:205)
  - `NewHashVectorizer`  (vector_model.go:246)
  - `NewVectorProcessor`  (vector_model.go:307)
  - `NewInMemoryVectorStore`  (vector_model.go:47)

## internal/cache
  - `NewCacheManager`  (manager.go:22)
  - `NewMemoryCache`  (memory.go:33)
  - `NewMemoryCacheWithLimit`  (memory.go:38)
  - `NewRedisCacheWithClient`  (redis.go:139)
  - `NewRedisCache`  (redis.go:28)

## internal/channelbot/core
  - `NewBaseClient`  (core.go:68)

## internal/channelbot/telegram
  - `NewClient`  (telegram.go:29) `[已修复→NewTelegramClient]`

## internal/channelbot/whatsapp
  - `NewCloudClient`  (whatsapp.go:33)
  - `newFakeClient`  (whatsapp_test.go:26) [test]

## internal/content/controller
  - `NewAIContentController`  (ai_content.go:22)
  - `newStringBuilderWriter`  (batch_operation.go:131)
  - `NewBatchExportController`  (batch_operation.go:145)
  - `NewBatchOperationController`  (batch_operation.go:200)
  - `NewBatchImportController`  (batch_operation.go:23)
  - `NewMarketingFlowController`  (marketing_flow.go:19)
  - `NewMaterialController`  (material.go:18)
  - `NewScriptTemplateController`  (script_template.go:19)
  - `NewTemplateMarketController`  (template_market.go:20)

## internal/content/repository
  - `NewPromptTemplateRepository`  (ai_content.go:105)
  - `NewAIGenerationRecordRepository`  (ai_content.go:26)
  - `NewBatchOperationRepository`  (batch_operation_repository.go:20)
  - `NewMarketingFlowRepository`  (marketing_flow.go:17)
  - `NewMarketingFlowRepositoryWithDB`  (marketing_flow.go:24)
  - `NewFlowExecutionRepository`  (marketing_flow.go:85)
  - `NewFlowExecutionRepositoryWithDB`  (marketing_flow.go:92)
  - `NewMaterialRepository`  (material.go:24)
  - `NewMaterialRepositoryWithDB`  (material.go:28)
  - `NewMaterialCategoryRepository`  (material_category.go:24)
  - `NewMaterialCategoryRepositoryWithDB`  (material_category.go:28)
  - `NewScriptCategoryRepository`  (script_template.go:106)
  - `NewScriptRecommendRepository`  (script_template.go:140)
  - `NewScriptTemplateRepository`  (script_template.go:17)

## internal/content/service
  - `NewAIContentService`  (ai_content.go:25)
  - `NewPromptTemplateService`  (ai_content.go:263)
  - `NewBatchOperationService`  (batch_operation.go:30)
  - `NewMarketingFlowServiceWithDB`  (marketing_flow.go:102)
  - `NewMarketingFlowService`  (marketing_flow.go:88)
  - `NewMaterialService`  (material.go:53)
  - `NewMaterialServiceWithDB`  (material.go:62)
  - `NewObsConfigServiceAdapter`  (material.go:80)
  - `NewScriptTemplateService`  (script_template.go:18)
  - `NewTemplateMarketService`  (template_market.go:22)

## internal/controller
  - `NewAccountController`  (account.go:18)
  - `NewAdminConfigController`  (admin_config.go:19)
  - `NewAgentStatusController`  (agent_status_controller.go:24) **[web同名引用 ×1 · 相似度低(仅引用后端命名,无同名工具实现)]**
  - `NewAIAgentController`  (ai_agent_controller.go:38)
  - `NewAIAgentControllerWithService`  (ai_agent_controller.go:46)
  - `NewChannelAgentBindingController`  (ai_agent_controller.go:473)
  - `NewChannelAgentBindingControllerWithService`  (ai_agent_controller.go:480)
  - `NewCustomerServiceAgentController`  (ai_agent_controller.go:619)
  - `NewCustomerServiceAgentControllerWithService`  (ai_agent_controller.go:626)
  - `NewAISuggestionController`  (ai_suggestion_controller.go:19) **[web同名引用 ×1 · 相似度低(仅引用后端命名,无同名工具实现)]**
  - `NewAnomalyLoginDetectorController`  (anomaly_login_detector_controller.go:34)
  - `NewAppConfigController`  (app_config.go:22)
  - `NewAssetBundleController`  (asset_bundle.go:32)
  - `NewAssetMarketController`  (asset_market.go:25)
  - `NewAuthController`  (auth.go:24)
  - `NewSystemUserController`  (auth.go:487)
  - `NewAutoReplyController`  (auto_reply.go:86)
  - `NewAutoReplyManagerController`  (auto_reply_manager_controller.go:20)
  - `NewRestoreController`  (backup.go:144)
  - `NewBackupController`  (backup.go:18)
  - `NewBaseCardController`  (base_card_controller.go:20)
  - `NewCardStatsFactoryController`  (card_stats_factory.go:27)
  - `NewChatChannelController`  (chat_channel_controller.go:26)
  - `NewChatPublicController`  (chat_public.go:43)
  - `NewClueController`  (clue.go:19)
  - `NewClueScoreController`  (clue_score.go:19)
  - `NewCommunityController`  (community.go:18)
  - `NewCustomerController`  (customer.go:19)
  - `NewCustomer360Controller`  (customer_360.go:29)
  - `NewCustomerEventController`  (customer_event.go:21)
  - `NewCustomerJourneyController`  (customer_journey_controller.go:21)
  - `NewCustomerOneIDController`  (customer_oneid_controller.go:26)
  - `NewCustomerRFMController`  (customer_rfm.go:19)
  - `NewCustomerSessionController`  (customer_session.go:35)
  - `NewDashboardSSEController`  (dashboard_sse.go:70)
  - `NewDialogueMemoryController`  (dialogue_memory_controller.go:22)
  - `NewDingTalkAppAccountController`  (dingtalk_app_account_controller.go:27)
  - `NewDomainPoolController`  (domain_pool.go:25)
  - `NewDouyinCardController`  (douyin_card.go:20)
  - `NewDouyinCardStatsController`  (douyin_card_stats.go:19)
  - `NewEmailDraftController`  (email_draft.go:20)
  - `NewEmailJobsController`  (email_jobs.go:20)
  - `NewEmailListController`  (email_list.go:24)
  - `NewEmailOpenTrackerController`  (email_open_tracker_controller.go:34)
  - `NewEmailSendController`  (email_send.go:16)
  - `NewEmailSmtpController`  (email_smtp.go:19)
  - `NewEmailTrackingController`  (email_tracking.go:31)
  - `NewEmailUnsubscribeController`  (email_unsubscribe.go:27)
  - `NewFeishuAccountController`  (feishu_account_controller.go:33)
  - `NewGlossaryController`  (glossary_controller.go:33)
  - `NewI18nStatsController`  (i18n_stats_controller.go:35)
  - `NewInboxController`  (inbox_controller.go:21)
  - `NewInboxIngressController`  (inbox_controller.go:325)
  - `NewIntegrationController`  (integration.go:20)
  - `NewIntentController`  (intent_controller.go:23)
  - `NewKuaishouCardController`  (kuaishou_card.go:20)
  - `NewKuaishouCardStatsController`  (kuaishou_card_stats.go:19)
  - `NewLiveCodeController`  (live_code_controller.go:22)
  - `NewLLMProviderController`  (llm_provider_controller.go:27)
  - `NewLLMRoutingController`  (llm_routing_controller.go:31)
  - `NewMessageHubController`  (message_hub_controller.go:22)
  - `NewMessageHubControllerWithCache`  (message_hub_controller.go:30)
  - `NewMigrationController`  (migration_controller.go:36)
  - `NewUpgradeController`  (migration_controller.go:46)
  - `NewNotificationController`  (notification.go:20)
  - `NewObjectionHandlerController`  (objection_handler_controller.go:20)
  - `NewObsConfigController`  (obs_config.go:23)
  - `NewOperationLogController`  (operation_log.go:26)
  - `NewPermissionController`  (permission.go:33)
  - `NewPlatformController`  (platform.go:20)
  - `NewQuickReplyController`  (quick_reply_controller.go:18) **[web同名引用 ×1 · 相似度低(仅引用后端命名,无同名工具实现)]**
  - `NewRagHealthController`  (rag_health_controller.go:28)
  - `NewRagRecallMonitorController`  (rag_recall_monitor_controller.go:32)
  - `NewRagSafetyGuardController`  (rag_safety_guard_controller.go:29)
  - `NewReachPipelineController`  (reach_pipeline_controller.go:21)
  - `NewRecoveryQueueController`  (recovery_queue.go:21)
  - `NewRedirectController`  (redirect_controller.go:33)
  - `NewRoleController`  (role.go:32)
  - `NewSalesPersonaController`  (sales_persona_controller.go:21)
  - `NewSecurityAuditController`  (security_audit_controller.go:21)
  - `NewSelfLearningController`  (self_learning_controller.go:51)
  - `NewSessionTagController`  (session_tag_controller.go:18) **[web同名引用 ×1 · 相似度低(仅引用后端命名,无同名工具实现)]**
  - `NewShortLinkController`  (short_link.go:20)
  - `NewShortLinkStatsController`  (short_link_stats.go:20)
  - `NewSmsController`  (sms.go:21)
  - `NewSmsDeliveryTrackerController`  (sms_delivery_tracker_controller.go:34)
  - `NewSmsUnsubscribeController`  (sms_unsubscribe.go:27)
  - `NewSOPController`  (sop_controller.go:21)
  - `NewSSEDashboardController`  (sse_dashboard.go:35)
  - `NewSystemConfigController`  (system_config.go:19)
  - `NewSystemInfoController`  (system_info.go:16)
  - `NewSystemInitController`  (system_init.go:30)
  - `NewSystemOpsController`  (system_ops.go:29)
  - `NewSystemUserAdminController`  (system_user.go:35)
  - `NewTelegramAccountController`  (telegram_account_controller.go:36)
  - `NewTikTokAutoReplyController`  (tiktok_auto_reply_controller.go:20)
  - `NewTikTokCardController`  (tiktok_card_controller.go:22)
  - `NewTraceController`  (trace_controller.go:15)
  - `NewTuningController`  (tuning_controller.go:36)
  - `NewUnifiedMessageController`  (unified_message.go:21)
  - `NewPlatformAccountController`  (unified_message.go:94)
  - `newUploadRouter`  (upload_test.go:41) [test]
  - `NewUserController`  (user.go:18)
  - `NewUserSegmentController`  (user_segment.go:19)
  - `NewWebhookController`  (webhook_controller.go:27)
  - `NewWeComController`  (wecom.go:24)
  - `NewWeComHealthController`  (wecom_health_controller.go:21)
  - `NewWhatsappController`  (whatsapp.go:24)
  - `NewWhatsAppCloudAccountController`  (whatsapp_cloud_account_controller.go:34)
  - `NewGroupMessagingController`  (whatsapp_group_messaging.go:33)
  - `NewXianyuAutoReplyController`  (xianyu_auto_reply.go:41)
  - `NewXianyuCardController`  (xianyu_card.go:22)
  - `NewXianyuCardStatsController`  (xianyu_card_stats.go:23)
  - `NewXiaohongshuAutoReplyController`  (xiaohongshu_auto_reply.go:28)
  - `NewXiaohongshuCardController`  (xiaohongshu_card.go:20)
  - `NewXiaohongshuCardStatsController`  (xiaohongshu_card_stats.go:18)
  - `NewMockXiaohongshuCardStatsService`  (xiaohongshu_card_stats_test.go:22) [test]

## internal/cron
  - `NewDomainHealthCheckJob`  (domain_health_job.go:20)
  - `NewLiveCodeRotator`  (live_code_rotator.go:17)

## internal/email/service
  - `NewEmailDraftService`  (email_draft.go:19)
  - `newTestEmailDraftRepository`  (email_draft_test.go:27) [test]
  - `NewEmailJobsService`  (email_jobs.go:20)
  - `newTestEmailJobsRepository`  (email_jobs_test.go:27) [test]
  - `NewEmailListService`  (email_list.go:25)
  - `NewEmailSendService`  (email_send.go:28)
  - `NewEmailSmtpService`  (email_smtp.go:15)

## internal/etl
  - `NewDocumentProcessor`  (document_processor.go:29)

## internal/event
  - `newTestError`  (bus_test.go:309) [test]
  - `NewOperationLogSubscriber`  (subscribers.go:26)
  - `newTestLogRepo`  (subscribers_test.go:179) [test]

## internal/migration/migrations
  - `NewADomainP1Migration`  (a_domain_p1_migration.go:28)
  - `NewAgentAssetBindingMigration`  (ai_agent_asset_binding_migration.go:19)
  - `NewAIAgentExtensionMigration`  (ai_agent_logic_migration.go:27)
  - `NewAIAgentSchemaMigration`  (ai_agent_schema_migration.go:32)
  - `NewAmountMoneyMigration`  (amount_money_migration.go:48)
  - `NewAssetBundleMigration`  (asset_bundle_migration.go:34)
  - `NewAuthSecurityMigration`  (auth_security_migration.go:34)
  - `NewComplianceDEMigration`  (compliance_d_e_migration.go:37)
  - `NewConfidenceMigration`  (confidence_migration.go:38)
  - `NewFeedbackLoopMigration`  (feedback_loop_migration.go:37)
  - `NewHP1Migration`  (h_p1_migration.go:30)
  - `NewHumanizeEvaluatorMigration`  (humanize_evaluator_migration.go:37)
  - `NewInitialSchemaMigration`  (initial_schema.go:17)
  - `NewMarketingFlowSchemaMigration`  (initial_schema.go:55)
  - `NewKnowledgeVectorMigration`  (knowledge_vector_migration.go:30)
  - `NewLP1Migration`  (l_p1_migration.go:28)
  - `NewLLMRoutingLogsExtendMigration`  (llm_routing_logs_extend_migration.go:37)
  - `NewLLMRoutingLogsMigration`  (llm_routing_logs_migration.go:27)
  - `NewLLMUsageRecordsMigration`  (llm_usage_records_migration.go:25)
  - `NewMP1Migration`  (m_p1_migration.go:38)
  - `NewMerchantIDNullableMigration`  (merchant_id_nullable_migration.go:21)
  - `NewMultilingualI18nMigration`  (multilingual_i18n_migration.go:37)
  - `NewMultilingualI18nP13Migration`  (multilingual_i18n_p13_migration.go:30)
  - `NewRagHybridMigration`  (rag_hybrid_migration.go:35)
  - `NewRagMonitoringMigration`  (rag_monitoring_migration.go:33)
  - `NewSelfLearningMigration`  (self_learning_migration.go:37)
  - `NewShortLinkColumnsMigration`  (shortlink_columns_migration.go:33)
  - `NewSOPExecutorMigration`  (sop_executor_migration.go:27)
  - `NewUnmultitenantSchemaMigration`  (unmultitenant_migration.go:17)
  - `NewUserBlacklistMigration`  (user_blacklist_migration.go:29)
  - `NewWecomWebhookFieldsMigration`  (wecom_webhook_fields_migration.go:17)

## internal/migration
  - `NewMigrationRegistry`  (registry.go:28)
  - `NewMigrationService`  (service.go:29)
  - `NewMigrationServiceDefault`  (service.go:42)

## internal/ops/controller
  - `NewABExperimentController`  (ab_experiment.go:19)
  - `NewAIProductivityController`  (ai_productivity_controller.go:20)
  - `NewChurnPredictionController`  (churn_prediction.go:20)
  - `NewConversionFunnelController`  (conversion_funnel_controller.go:20)
  - `NewCustomReportController`  (custom_report.go:20)
  - `NewDashboardScreenController`  (dashboard_screen.go:22)
  - `NewPerformanceTestController`  (performance_test_controller.go:20)

## internal/ops/repository
  - `NewABConversionEventRepository`  (ab_experiment.go:135)
  - `NewABExperimentRepository`  (ab_experiment.go:16)
  - `NewABExperimentResultRepository`  (ab_experiment.go:174)
  - `NewABVariantRepository`  (ab_experiment.go:77)
  - `NewAIProductivityRepository`  (ai_productivity_repository.go:25)
  - `NewChurnModelConfigRepository`  (churn_prediction.go:165)
  - `NewChurnPredictionRepository`  (churn_prediction.go:17)
  - `NewChurnStatisticsRepository`  (churn_prediction.go:202)
  - `NewChurnPredictionRepositoryWithDB`  (churn_prediction.go:232)
  - `NewChurnWarningRepositoryWithDB`  (churn_prediction.go:237)
  - `NewChurnModelConfigRepositoryWithDB`  (churn_prediction.go:242)
  - `NewChurnStatisticsRepositoryWithDB`  (churn_prediction.go:247)
  - `NewChurnWarningRepository`  (churn_prediction.go:94)
  - `NewConversionFunnelRepository`  (conversion_funnel_repository.go:25)
  - `NewCustomReportRepositoryWithDB`  (custom_report.go:112)
  - `NewCustomReportRepository`  (custom_report.go:15)
  - `NewMarketTemplateRepository`  (dashboard_screen.go:107)
  - `NewDashboardScreenRepository`  (dashboard_screen.go:17)
  - `NewMarketTemplateDownloadRepository`  (dashboard_screen.go:197)
  - `NewDashboardWidgetRepository`  (dashboard_screen.go:78)
  - `NewPerformanceTestRepository`  (performance_test_repository.go:18)
  - `NewStatsRepository`  (stats.go:48)

## internal/ops/service
  - `NewABExperimentService`  (ab_experiment.go:23)
  - `NewAIProductivityService`  (ai_productivity_service.go:16)
  - `NewChurnPredictionService`  (churn_prediction.go:23)
  - `NewChurnPredictionServiceWithDB`  (churn_prediction.go:33)
  - `NewConversionFunnelService`  (conversion_funnel_service.go:16)
  - `NewCustomReportService`  (custom_report.go:24)
  - `NewCustomReportServiceWithDB`  (custom_report.go:34)
  - `NewDashboardScreenService`  (dashboard_screen.go:23)
  - `newTestDashboardScreenRepository`  (dashboard_screen_test.go:27) [test]
  - `newTestDashboardWidgetRepository`  (dashboard_screen_test.go:32) [test]
  - `NewPerformanceTestService`  (performance_test_service.go:22)

## internal/pkg/metrics
  - `NewCounterVec`  (metrics.go:42)
  - `NewHistogramVec`  (metrics.go:93)

## internal/pkg/shared/service
  - `NewSystemStatsService`  (system_stats_service.go:17)

## internal/pkg/testutil
  - `NewTestDB`  (testdb.go:46)

## internal/pkg/trace
  - `NewContextWithTraceID`  (trace.go:220)
  - `NewTracer`  (trace.go:70)
  - `NewTracerFromContext`  (trace.go:85)

## internal/pkg/utils
  - `NewAppError`  (error_handler.go:40)

## internal/pkg/utils/httpclient
  - `NewWithTimeout`  (httpclient.go:34)

## internal/pkg/utils
  - `NewJWTUtils`  (jwt.go:85)
  - `NewLogger`  (logger.go:47)

## internal/pkg/utils/logger
  - `newRotatingWriter`  (logger.go:252)

## internal/pkg/utils/pagination
  - `newCtx`  (pagination_test.go:14) [test]

## internal/pkg/utils
  - `NewSensitiveEncryptor`  (sensitive_encryption.go:24)

## internal/platform
  - `NewBrowserAdapter`  (adapter.go:301)
  - `NewDouyinAdapter`  (adapter.go:496)
  - `NewKuaishouAdapter`  (adapter.go:497)
  - `NewXiaohongshuAdapter`  (adapter.go:498)
  - `NewXianyuAdapter`  (adapter.go:499)
  - `NewTiktokAdapter`  (adapter.go:500)
  - `NewPlatformAPIClient`  (asset_market_adapter.go:14)
  - `NewAssetMarketClient`  (asset_market_client.go:29)
  - `NewClient`  (client.go:30) `[已修复→NewPlatformClient]`
  - `NewContributorClient`  (contributor_client.go:42)
  - `newAdapterRegistry`  (registry.go:50)
  - `NewAdapterRegistry`  (registry.go:55)

## internal/reach/card/template
  - `NewTemplateService`  (template.go:55)

## internal/repository
  - `NewAccountRepository`  (account.go:28)
  - `NewAccountRepositoryWithDB`  (account.go:33)
  - `NewAfterSaleRepository`  (aftersale.go:18)
  - `NewAgentStatusRepository`  (agent_status_repository.go:18)
  - `NewAgentStatusRepositoryWithDB`  (agent_status_repository.go:25)
  - `NewChannelAgentBindingRepository`  (ai_agent_repository.go:138)
  - `NewCustomerServiceAgentRepository`  (ai_agent_repository.go:231)
  - `NewAIAgentRepository`  (ai_agent_repository.go:32)
  - `NewAISuggestionRepository`  (ai_suggestion_repository.go:18)
  - `NewAISuggestionRepositoryWithDB`  (ai_suggestion_repository.go:25)
  - `NewAssetBundleABTestRepository`  (asset_bundle_ab_test_repo.go:55)
  - `NewAssetBundleCandidateRepository`  (asset_bundle_candidate_repo.go:45)
  - `NewAssetBundleRepository`  (asset_bundle_repo.go:58)
  - `NewAssetBundleVersionLogRepository`  (asset_bundle_version_log_repo.go:27)
  - `NewAutoReplyRuleRepository`  (auto_reply.go:134)
  - `NewAutoReplyRuleRepositoryWithDB`  (auto_reply.go:139)
  - `NewAutoReplyAccountRepository`  (auto_reply.go:22)
  - `NewAutoReplyAccountRepositoryAuto`  (auto_reply.go:28)
  - `NewAutoReplyLogRepository`  (auto_reply.go:304)
  - `NewAutoReplyLogRepositoryAuto`  (auto_reply.go:310)
  - `NewBackupRepository`  (backup.go:18)
  - `NewBackupRepositoryWithDB`  (backup.go:25)
  - `NewRestoreRecordRepository`  (backup.go:92)
  - `NewRestoreRecordRepositoryWithDB`  (backup.go:99)
  - `NewBackupDataRepository`  (backup_data.go:39)
  - `NewBackupDataRepositoryWithDB`  (backup_data.go:44)
  - `NewDailyCardUVStatsRepository`  (card_access_repository.go:111)
  - `NewCardAccessRepository`  (card_access_repository.go:23)
  - `NewChatChannelRepository`  (chat_channel_repository.go:20)
  - `NewChatChannelRepositoryWithDB`  (chat_channel_repository.go:27)
  - `NewClueRepository`  (clue.go:38)
  - `NewClueRepositoryWithDB`  (clue.go:43)
  - `NewClueEngagementRepository`  (clue_score.go:115)
  - `NewClueEngagementRepositoryWithDB`  (clue_score.go:119)
  - `NewClueScoreRepository`  (clue_score.go:27)
  - `NewClueScoreRepositoryWithDB`  (clue_score.go:32)
  - `NewCommunityRepository`  (community.go:36)
  - `NewCommunityRepositoryDefault`  (community.go:40)
  - `NewHandoffDecisionRepository`  (confidence_repositories.go:140)
  - `NewReviewQueueRepository`  (confidence_repositories.go:219)
  - `NewThresholdPolicyRepository`  (confidence_repositories.go:305)
  - `NewSLAMonitorRepository`  (confidence_repositories.go:353)
  - `NewConfidenceSignalRepository`  (confidence_repositories.go:37)
  - `NewABTestRepository`  (confidence_repositories.go:401)
  - `NewConfidenceCalibrationRepository`  (confidence_repositories.go:87)
  - `NewCustomerEventRepository`  (customer_event_repository.go:44)
  - `NewCustomerRepository`  (customer_repository.go:60)
  - `NewRecoveryQueueRepository`  (customer_rfm.go:134)
  - `NewRecoveryQueueRepositoryWithDB`  (customer_rfm.go:138)
  - `NewCustomerRFMRepository`  (customer_rfm.go:27)
  - `NewCustomerRFMRepositoryWithDB`  (customer_rfm.go:31)
  - `NewCustomerSessionRepository`  (customer_session.go:24)
  - `NewCustomerSessionRepositoryWithDB`  (customer_session.go:31)
  - `NewCustomerTagRepository`  (customer_tag_repository.go:22)
  - `NewDashboardStatsRepository`  (dashboard_stats_repository.go:65)
  - `NewDialogueMemoryRepository`  (dialogue_memory_repository.go:39)
  - `NewDialogueMemoryRepositoryWithDB`  (dialogue_memory_repository.go:44)
  - `NewDingTalkAppRepository`  (dingtalk_app_repository.go:27)
  - `NewDomainHealthLogRepository`  (domain_pool.go:207)
  - `NewDomainBlacklistRepository`  (domain_pool.go:240)
  - `NewDomainPoolRepository`  (domain_pool.go:40)
  - `NewDomainPoolRepositoryWithDB`  (domain_pool.go:45)
  - `NewDouyinCardRepository`  (douyin_card.go:39)
  - `NewDouyinCardStatsRepository`  (douyin_card_stats.go:41)
  - `NewEmailDraftRepository`  (email_draft.go:26)
  - `NewEmailJobsRepository`  (email_jobs.go:26)
  - `NewEmailListRepository`  (email_list.go:31)
  - `NewEmailSendRepository`  (email_send.go:26)
  - `NewEmailSmtpRepository`  (email_smtp.go:24)
  - `NewEmailTrackingRepository`  (email_tracking.go:34)
  - `NewEmailUnsubscribeRepository`  (email_unsubscribe.go:29)
  - `NewFeedbackLearningRepository`  (feedback_learning_repository.go:30)
  - `NewFeedbackLoopRepositoryWithDB`  (feedback_loop_repository.go:145)
  - `NewFeedbackLoopRepository`  (feedback_loop_repository.go:26)
  - `NewFeishuMessageRepository`  (feishu.go:134)
  - `NewTelegramAccountRepository`  (feishu.go:161)
  - `NewFeishuAccountRepository`  (feishu.go:17)
  - `NewWhatsAppCloudAccountRepository`  (feishu.go:212)
  - `NewFeishuCustomerRepository`  (feishu.go:95)
  - `NewBaseRepository`  (generic.go:37)
  - `NewGlossaryRepository`  (glossary_repo.go:38)
  - `NewGlossaryRepositoryWithDB`  (glossary_repo.go:44)
  - `NewHumanizeLowQualitySampleCollector`  (humanize_low_quality_collector.go:28)
  - `NewChampionBaselineRepository`  (humanize_repositories.go:118)
  - `NewABTestStatRepository`  (humanize_repositories.go:223)
  - `NewHumanizeScoreRepository`  (humanize_repositories.go:35)
  - `NewI18nStatsRepository`  (i18n_stats_repo.go:89)
  - `NewI18nStatsRepositoryWithDB`  (i18n_stats_repo.go:95)
  - `NewExternalCustomerRepository`  (integration.go:155)
  - `NewIntegrationAccountRepository`  (integration.go:18)
  - `NewExternalOrderRepository`  (integration.go:229)
  - `NewExternalProductRepository`  (integration.go:321)
  - `NewWebhookEventRepository`  (integration.go:378)
  - `NewSyncLogRepository`  (integration.go:98)
  - `NewIntegrationTemplateRepository`  (integration_template.go:28)
  - `NewIntegrationTemplateRepositoryWithDB`  (integration_template.go:32)
  - `NewIntentLogRepository`  (intent_recognition.go:123)
  - `NewIntentRecordRepository`  (intent_recognition.go:32)
  - `NewKnowledgeRepository`  (knowledge.go:22)
  - `NewKnowledgeRepositoryWithDB`  (knowledge.go:27)
  - `NewKnowledgeChunkExtRepository`  (knowledge_chunk_quality_repo.go:54)
  - `NewKuaishouCardRepository`  (kuaishou_card.go:30)
  - `NewKuaishouCardStatsRepository`  (kuaishou_card_stats.go:53)
  - `NewLiveCodeRepository`  (live_code.go:28)
  - `NewLiveCodeClickLogRepository`  (live_code_click_log.go:35)
  - `NewLiveCodeQRRepository`  (live_code_qr.go:28)
  - `NewLocalAssetDataRepository`  (local_asset_repo.go:294)
  - `NewLocalAssetSyncLogRepository`  (local_asset_repo.go:326)
  - `NewLocalAssetRepository`  (local_asset_repo.go:61)
  - `NewLoginRiskRepository`  (login_risk.go:41)
  - `NewLoginRiskRepositoryWithDB`  (login_risk.go:46)
  - `NewLowQualitySampleRepository`  (low_quality_sample_repository.go:20)
  - `NewMemoryRepository`  (memory_repository.go:83)
  - `NewMemoryRepositoryWithDB`  (memory_repository.go:88)
  - `NewMessageRepository`  (message.go:22)
  - `NewMessageHubRepository`  (message_hub_inbox.go:23)
  - `NewMessageHubRepositoryWithDB`  (message_hub_inbox.go:29)
  - `NewInboxConversationRepository`  (message_hub_inbox.go:375)
  - `NewInboxConversationRepositoryWithDB`  (message_hub_inbox.go:381)
  - `NewInboxAssignmentRepository`  (message_hub_inbox.go:871)
  - `NewInboxAssignmentRepositoryWithDB`  (message_hub_inbox.go:877)
  - `NewMessageQueueRepository`  (message_queue.go:23)
  - `NewMessageQueueRepositoryWithDB`  (message_queue.go:28)
  - `NewNotificationRepository`  (notification.go:19)
  - `NewNotificationRepositoryWithDB`  (notification.go:24)
  - `NewObsConfigRepository`  (obs_config.go:33)
  - `NewObsConfigRepositoryWithDB`  (obs_config.go:37)
  - `NewOperationLogRepository`  (operation_log.go:37)
  - `NewOperationLogRepositoryWithDB`  (operation_log.go:42)
  - `NewOrderRepository`  (order.go:36)
  - `NewOrderRepositoryWithDB`  (order.go:41)
  - `NewPasswordHistoryRepository`  (password_history_repository.go:39)
  - `NewPersonaLowQualitySampleRepository`  (persona_repository.go:38)
  - `NewPersonaLowQualitySampleRepositoryWithDB`  (persona_repository.go:43)
  - `NewQuickReplyRepository`  (quick_reply_repository.go:17)
  - `NewRagAlertRepository`  (rag_alert_repository.go:59)
  - `NewRagHealthRepository`  (rag_health_repository.go:31)
  - `NewRagMetricsRepository`  (rag_metrics_repository.go:76)
  - `NewRagRecallMonitorRepository`  (rag_recall_monitor_repository.go:71)
  - `NewReachPipelineRepository`  (reach_pipeline.go:46)
  - `NewRFMRuleRepository`  (rfm_rule.go:19)
  - `NewUserRFMRepositoryWithDB`  (rfm_rule.go:208)
  - `NewUserRFMRepository`  (rfm_rule.go:73)
  - `NewSalesPersonaRepository`  (sales_persona.go:19)
  - `NewScriptLibraryRepository`  (script_library_repository.go:25)
  - `NewSecurityAuditRepository`  (security_audit.go:27)
  - `NewSelfLearningLogRepository`  (self_learning_repo.go:114)
  - `NewSelfSupervisionSignalRepository`  (self_learning_switch_repo.go:207)
  - `NewSelfCorrectionActionRepository`  (self_learning_switch_repo.go:396)
  - `NewSelfLearningSwitchRepository`  (self_learning_switch_repo.go:59)
  - `NewSessionMessageRepository`  (session_message_repository.go:19)
  - `NewSessionMessageRepositoryWithDB`  (session_message_repository.go:26)
  - `NewSessionTagRepository`  (session_tag_repository.go:17)
  - `NewShortLinkRepository`  (short_link.go:29)
  - `NewShortLinkAccessRepository`  (short_link_access.go:29)
  - `NewSmlistRepository`  (sm_list.go:25)
  - `NewSmsRepository`  (sms.go:58)
  - `NewSmsDeliveryRepository`  (sms_delivery_repository.go:51)
  - `NewSmsTrackingRepository`  (sms_tracking.go:47)
  - `NewSmsUnsubscribeRepository`  (sms_unsubscribe.go:29)
  - `NewSopAgentRepository`  (sop.go:18)
  - `NewSopExecutionRepository`  (sop.go:71)
  - `NewSOPExecEventRepository`  (sop_exec_event.go:21)
  - `NewSOPTimerRepository`  (sop_timer.go:21)
  - `NewSystemConfigRepository`  (system_config.go:28)
  - `NewSystemConfigKVRepository`  (system_config_kv_repository.go:43)
  - `NewSystemStatsRepository`  (system_stats_repository.go:42)
  - `NewSystemUserRepository`  (system_user.go:73)
  - `NewTikTokCardRepository`  (tiktok_card.go:35)
  - `NewUnifiedReplyRepository`  (unified_message.go:111)
  - `NewUnifiedReplyRepositoryWithDB`  (unified_message.go:116)
  - `NewPlatformAccountRepository`  (unified_message.go:177)
  - `NewUnifiedMessageRepository`  (unified_message.go:28)
  - `NewUnifiedMessageRepositoryWithDB`  (unified_message.go:33)
  - `NewMigrationCheckpointRepository`  (upgrade.go:127)
  - `NewUpgradeTaskRepository`  (upgrade.go:18)
  - `NewMigrationRecordRepository`  (upgrade.go:84)
  - `NewUserRepository`  (user.go:29)
  - `NewUserBlacklistRepository`  (user_blacklist.go:19)
  - `NewUserBlacklistRepositoryWithDB`  (user_blacklist.go:24)
  - `NewUserMFARepository`  (user_mfa.go:33)
  - `NewUserMFARepositoryWithDB`  (user_mfa.go:38)
  - `NewUserTagRepository`  (user_tag.go:29)
  - `NewUserTagRepositoryWithDB`  (user_tag.go:34)
  - `NewWeComGroupRepository`  (wecom.go:152)
  - `NewWeComAccountRepository`  (wecom.go:19)
  - `NewWeComGroupMemberRepository`  (wecom.go:212)
  - `NewWeComMessageRepository`  (wecom.go:254)
  - `NewWeComTagRepository`  (wecom.go:306)
  - `NewWeComAccountHealthRepository`  (wecom.go:379)
  - `NewWeComCustomerRepository`  (wecom.go:82)
  - `NewWhatsappRepository`  (whatsapp.go:48)
  - `NewWhatsappTemplateRepository`  (whatsapp_template.go:17)
  - `NewWhatsappTemplateRepositoryWithDB`  (whatsapp_template.go:22)
  - `NewXianyuCardRepository`  (xianyu_card.go:27)
  - `NewXianyuCardStatsRepository`  (xianyu_card_stats.go:61)
  - `NewXiaohongshuCardRepository`  (xiaohongshu_card.go:27)
  - `NewXiaohongshuCardStatsRepository`  (xiaohongshu_card_stats.go:41)

## internal/router
  - `NewFeedbackCollectorAdapter`  (feedback_sink_adapter.go:48)
  - `newTestExecutor`  (tool_debug_routes_test.go:104) [test]
  - `newMockEchoTool`  (tool_debug_routes_test.go:63) [test]
  - `newMockFailingTool`  (tool_debug_routes_test.go:79) [test]
  - `NewToolExecutorAdapter`  (tool_executor_adapter.go:29)
  - `newMockProviderWithTools`  (tool_provider_test.go:55) [test]

## internal/service
  - `NewABTestLoader`  (abtest_loader.go:32)
  - `NewAccountService`  (account.go:14)
  - `NewAccountServiceWithRepo`  (account_domain_service.go:18)
  - `NewAfterSaleService`  (aftersale.go:22)
  - `NewAgentLoader`  (agent_loader.go:41)
  - `NewAgentStatusService`  (agent_status.go:25)
  - `NewChannelAgentBindingService`  (ai_agent_service.go:281)
  - `NewChannelAgentBindingServiceWithDB`  (ai_agent_service.go:286)
  - `NewCustomerServiceAgentService`  (ai_agent_service.go:428)
  - `NewCustomerServiceAgentServiceWithDB`  (ai_agent_service.go:433)
  - `NewCustomerServiceAgentServiceViaPort`  (ai_agent_service.go:454)
  - `NewAIAgentService`  (ai_agent_service.go:60)
  - `NewAIAgentServiceWithDB`  (ai_agent_service.go:65)
  - `NewAISuggestionService`  (ai_suggestion.go:27)
  - `NewOrderIntentExtractor`  (ai_tagger.go:218)
  - `NewAITagger`  (ai_tagger.go:37)
  - `NewAnomalyLoginDetector`  (anomaly_login_detector.go:100)
  - `NewAnomalyLoginDetectorWithConfig`  (anomaly_login_detector.go:108)
  - `NewAssetBundleService`  (asset_bundle.go:354)
  - `newHotPlugCache`  (asset_bundle.go:373)
  - `NewKnowledgeSearchPortAdapter`  (asset_bundle_port_adapter.go:113)
  - `NewAssetBundleWeavePortAdapter`  (asset_bundle_port_adapter.go:33)
  - `newMockAssetBundleRepo`  (asset_bundle_test.go:555) [test]
  - `NewAssetMarketService`  (asset_market_service.go:15)
  - `NewAuthService`  (auth.go:65)
  - `NewAutoReplyService`  (auto_reply.go:30)
  - `NewAutoReplyServiceAuto`  (auto_reply.go:43)
  - `NewAutoTagger`  (auto_tagger.go:19)
  - `newTestAutoReplyService`  (autoreply_test.go:25) [test]
  - `NewBackupService`  (backup.go:25)
  - `NewRestoreService`  (backup.go:310)
  - `NewScheduleBackupService`  (backup.go:522)
  - `newTestBackupRepository`  (backup_test.go:33) [test]
  - `newTestRestoreRecordRepository`  (backup_test.go:38) [test]
  - `NewCardAccessService`  (card_access_service.go:42)
  - `NewPlatformXiaohongshuCardStatsAdapter`  (card_stats_adapters.go:115)
  - `NewPlatformKuaishouCardStatsAdapter`  (card_stats_adapters.go:186)
  - `NewPlatformXianyuCardStatsAdapter`  (card_stats_adapters.go:266)
  - `NewPlatformTiktokCardStatsAdapter`  (card_stats_adapters.go:350)
  - `NewPlatformDouyinCardStatsAdapter`  (card_stats_adapters.go:46)
  - `NewChatChannelService`  (chat_channel_service.go:40)
  - `NewVisitorChatService`  (chat_visitor_service.go:49)
  - `newInboxConversationRepo`  (chat_visitor_service.go:69)
  - `NewClueService`  (clue.go:13)
  - `NewClueScoreService`  (clue_score.go:27)
  - `NewClueScoreServiceWithRepos`  (clue_score.go:36)
  - `NewCommunityService`  (community.go:19)

## internal/service/confidence
  - `NewABTestService`  (ab_test.go:37) [test]
  - `NewConfidenceAggregator`  (aggregator.go:44)
  - `NewCalibrator`  (calibrator.go:38)
  - `NewDynamicThresholdCalculator`  (dynamic_threshold.go:31)
  - `NewGoldenSectionSearcher`  (golden_section.go:26)
  - `NewHandoffDecisionService`  (handoff_decision.go:41)
  - `NewReviewQueueService`  (review_queue.go:45)
  - `NewSignalCollector`  (signal_collector.go:42)
  - `NewSLAMonitorService`  (sla_monitor.go:37)
  - `NewTemperatureScaler`  (temperature_scaler.go:25)
  - `NewThresholdPolicyEngine`  (threshold_policy.go:30)
  - `NewVetoChain`  (veto_rule.go:176)
  - `NewVetoChainWithRules`  (veto_rule.go:190)
  - `NewWeightedAggregator`  (weighted_aggregator.go:48)
  - `NewDefaultWeightedAggregator`  (weighted_aggregator.go:54)

## internal/service
  - `NewContentAuditor`  (content_auditor.go:40)
  - `NewCustomer360ServiceWithDB`  (customer_360.go:30)
  - `NewCustomer360Service`  (customer_360.go:42)
  - `NewUserProfileService`  (customer_domain_service.go:118)
  - `NewTagRuleService`  (customer_domain_service.go:194)
  - `NewCustomerQueryService`  (customer_domain_service.go:368)
  - `NewUserTagService`  (customer_domain_service.go:65)
  - `NewCustomerIdentityService`  (customer_identity_service.go:17)
  - `NewCustomerJourneyService`  (customer_journey.go:209)
  - `NewCustomerJourneyServiceWithCache`  (customer_journey.go:217)
  - `newLongTermMemorySystem`  (customer_long_term_memory_test.go:28) [test]
  - `NewCustomerOrchestrator`  (customer_orchestrator.go:57)
  - `NewCustomerOrchestratorWithDeps`  (customer_orchestrator.go:66)
  - `NewCustomerRFMService`  (customer_rfm.go:23)
  - `NewCustomerRFMServiceWithRepos`  (customer_rfm.go:34)
  - `NewCustomerService`  (customer_service.go:17)
  - `NewCustomerSessionService`  (customer_session.go:45)
  - `NewCustomerSessionServiceWithDB`  (customer_session.go:59)
  - `NewDashboardStatsService`  (dashboard_sse_stats.go:99)
  - `NewDialogueMemoryService`  (dialogue_memory.go:35)
  - `newMemoryService`  (dialogue_memory_test.go:25) [test]
  - `NewDingTalkAppService`  (dingtalk_app_service.go:33)
  - `NewDingTalkService`  (dingtalk_service.go:41)
  - `NewDomainHealthService`  (domain_health.go:84)
  - `NewDomainHealthServiceWithDeps`  (domain_health.go:98)
  - `NewDomainPoolService`  (domain_pool.go:38)
  - `newTestDomainPoolRepository`  (domain_pool_test.go:26) [test]
  - `NewDouyinCardService`  (douyin_card.go:41)
  - `NewDouyinCardStatsService`  (douyin_card_stats.go:28)
  - `NewEmailOpenTrackerService`  (email_open_tracker.go:75)
  - `newOpenTracker`  (email_open_tracker_test.go:23) [test]
  - `NewEmailTrackingService`  (email_tracking.go:48)
  - `newEmailTrackingService`  (email_tracking_test.go:27) [test]
  - `NewEmailUnsubscribeService`  (email_unsubscribe.go:52)
  - `newEmailUnsubscribeService`  (email_unsubscribe_test.go:27) [test]
  - `NewEventTracker`  (event_tracker.go:22)
  - `NewFeedbackLearner`  (feedback_learner.go:57)
  - `NewFeedbackLearningService`  (feedback_learning_service.go:43)

## internal/service/feedback_loop
  - `NewBanditAllocator`  (bandit_allocator.go:60)
  - `NewChampionDialogueAnalyzer`  (champion_dialogue_analyzer.go:50)
  - `NewFeedbackCollector`  (feedback_collector.go:57)
  - `newStubEmbedder`  (helpers_test.go:106) [test]
  - `newStubBanditAllocator`  (helpers_test.go:174) [test]
  - `newStubLLMDispatcher`  (helpers_test.go:57) [test] `[已修复→newFeedbackLoopStubLLMDispatcher]`
  - `NewPromptIterator`  (prompt_iterator.go:45)
  - `NewSOPAutoOptimizer`  (sop_auto_optimizer.go:45)

## internal/service
  - `NewFeedbackLoopCron`  (feedback_loop_cron.go:46)
  - `NewFeishuIntegrationService`  (feishu_service.go:126)
  - `NewTelegramService`  (feishu_service.go:339)
  - `NewTelegramIntegrationService`  (feishu_service.go:406)
  - `NewFeishuService`  (feishu_service.go:48)
  - `NewWhatsAppCloudService`  (feishu_service.go:528)
  - `NewWhatsAppCloudIntegrationService`  (feishu_service.go:610)
  - `newFeishuTestAccount`  (feishu_service_test.go:66) [test]
  - `NewFollowUpService`  (followup_reminder.go:73)
  - `NewCacheNotifier`  (human_escalation.go:267)
  - `NewHumanEscalationManager`  (human_escalation.go:89)

## internal/service/humanize
  - `NewABTestStatsService`  (abtest_stats.go:39)
  - `newStubBaselineRepo`  (helpers_test.go:124) [test]
  - `newStubSampleCollector`  (helpers_test.go:178) [test]
  - `newStubEvaluator`  (helpers_test.go:223) [test]
  - `newStubLLMDispatcher`  (helpers_test.go:54) [test] `[已修复→newHumanizeStubLLMDispatcher]`
  - `newStubScoreRepo`  (helpers_test.go:92) [test]
  - `NewLLMScorer`  (llm_scorer.go:43)
  - `NewRuleScorer`  (rule_scorer.go:123)
  - `NewHumanizeEvalService`  (service.go:59)
  - `newServiceWithStubs`  (service_test.go:39) [test]
  - `NewTFIDFPhraseExtractor`  (tfidf_phrase.go:36)

## internal/service
  - `NewHumanizePolisher`  (humanize_polisher.go:49)

## internal/service/i18n
  - `NewEvalService`  (eval_service.go:57)
  - `NewDeepLTranslator`  (fallback_bridge.go:118)
  - `NewFallbackBridge`  (fallback_bridge.go:298)
  - `NewFewShotService`  (fewshot_service.go:78)
  - `NewGlossaryService`  (glossary_service.go:84)
  - `NewLangConfigResolver`  (lang_config_resolver.go:68)
  - `NewPostValidator`  (post_validator.go:76)
  - `NewPretranslateService`  (pretranslate_service.go:57)
  - `NewI18nStatsService`  (stats_service.go:77)

## internal/service
  - `NewInboxIngressService`  (inbox_ingress.go:63)
  - `NewInboxIngressServiceWithDB`  (inbox_ingress.go:71)
  - `NewInboxService`  (inbox_service.go:77)
  - `NewInboxServiceWithDB`  (inbox_service.go:86)
  - `newInboxService`  (inbox_service_test.go:24) [test]
  - `NewXiaoshouyiClient`  (integration.go:141)
  - `NewFenxiangxiaoClient`  (integration.go:318)
  - `NewIntegrationService`  (integration.go:40)
  - `NewTaobaoClient`  (integration.go:494)
  - `NewJDClient`  (integration.go:663)
  - `NewIntegrationTemplateService`  (integration_template.go:19)
  - `NewIntegrationTemplateServiceWithRepo`  (integration_template.go:24)
  - `NewIntentRecognizer`  (intent_recognition.go:59)
  - `newIntentRecognizer`  (intent_recognition_test.go:22) [test]
  - `NewKuaishouCardService`  (kuaishou_card.go:42)
  - `NewKuaishouCardStatsService`  (kuaishou_card_stats.go:21)
  - `NewLiveCodeService`  (live_code.go:48)
  - `newTestLiveCodeService`  (live_code_test.go:51) [test]
  - `NewLLMRoutingService`  (llm_routing_service.go:129)
  - `NewLocalAssetService`  (local_asset_service.go:39)
  - `NewLoginRiskService`  (login_risk.go:70)
  - `NewLoginRiskServiceWithRepo`  (login_risk.go:75)
  - `NewMessageService`  (message.go:13)
  - `NewMessageHubService`  (message_hub_service.go:135)
  - `NewMessageHubServiceWithDB`  (message_hub_service.go:144)
  - `newTestService`  (message_hub_service_test.go:25) [test] `[已修复→newMessageHubTestService]`
  - `newReq`  (message_hub_service_test.go:33) [test]
  - `NewMessageQueueService`  (message_queue_service.go:28)
  - `NewMFAService`  (mfa_service.go:69)
  - `NewNotificationService`  (notification.go:24)
  - `NewObjectionHandlerService`  (objection_handler_service.go:18)
  - `NewObsConfigService`  (obs_config.go:37)
  - `NewOpenAPIService`  (openapi_service.go:36)
  - `NewOpenAPIServiceWithDB`  (openapi_service.go:49)
  - `NewOperationLogService`  (operation_log_domain_service.go:26)
  - `NewOrderService`  (order.go:17)
  - `NewOrderServiceWithDB`  (order.go:22)
  - `NewOrderDraftService`  (order_draft.go:144)
  - `NewPasswordPolicyService`  (password_policy.go:96)
  - `NewAuthorizationService`  (permission.go:36)
  - `NewAuthorizationServiceWithRepo`  (permission.go:44)
  - `NewPermissionService`  (permission_check.go:164)
  - `NewLLMPersonaEvaluator`  (persona_evaluator.go:136)
  - `NewRuleBasedPersonaEvaluator`  (persona_evaluator.go:284)
  - `NewDBLowQualitySampleCollector`  (persona_evaluator.go:637)
  - `NewPersonaEvaluationService`  (persona_evaluator.go:705)
  - `NewPlatformAccountService`  (platform_account.go:16)
  - `newPlatformAccountRepoForTest`  (platform_account_test.go:217) [test]
  - `NewPlatformAccountServiceWithRepo`  (platform_account_test.go:26) [test]
  - `NewQuickReplyService`  (quick_reply.go:23)
  - `NewRagAlertCron`  (rag_alert.go:403)
  - `NewRagAlertService`  (rag_alert.go:87)
  - `NewRagHealthService`  (rag_health.go:80)
  - `NewRagMetricsCron`  (rag_metrics.go:510)
  - `NewRagMetricsService`  (rag_metrics.go:71)
  - `NewRagRecallMonitorService`  (rag_recall_monitor.go:91)
  - `newRecallMonitor`  (rag_recall_monitor_test.go:19) [test]
  - `NewRagSafetyGuardService`  (rag_safety_guard.go:156)
  - `newSafetyGuard`  (rag_safety_guard_test.go:24) [test]
  - `NewReachPipelineService`  (reach_pipeline.go:220)
  - `newReachTestService`  (reach_pipeline_test.go:26) [test]
  - `newReachPipelineReq`  (reach_pipeline_test.go:32) [test]
  - `newReachJobReq`  (reach_pipeline_test.go:46) [test]
  - `NewCountedSendPipeline`  (reach_send_pipeline.go:1077)
  - `NewFuncChannelAdapter`  (reach_send_pipeline.go:1125)
  - `NewAlwaysFailAdapter`  (reach_send_pipeline.go:1150)
  - `NewFlakyAdapter`  (reach_send_pipeline.go:1176)
  - `NewMemorySendRateLimiter`  (reach_send_pipeline.go:242)
  - `NewDefaultContentAuditor`  (reach_send_pipeline.go:311)
  - `newACAutomaton`  (reach_send_pipeline.go:397)
  - `NewMemorySendAuditLogger`  (reach_send_pipeline.go:489)
  - `NewMemorySendCostTracker`  (reach_send_pipeline.go:544)
  - `NewSendPipeline`  (reach_send_pipeline.go:660)
  - `newTestPipeline`  (reach_send_pipeline_test.go:39) [test]
  - `newSuccessAdapter`  (reach_send_pipeline_test.go:44) [test]
  - `NewRecoveryQueueService`  (recovery_queue.go:19)
  - `NewRecoveryQueueServiceWithRepo`  (recovery_queue.go:27)
  - `newMockRecoveryRepo`  (recovery_queue_test.go:107) [test]
  - `NewRepurchaseEngine`  (repurchase_engine.go:79)
  - `NewRFMCalculatorService`  (rfm_calculator.go:23)
  - `NewRoleService`  (role.go:38)
  - `NewRoleServiceWithRepo`  (role.go:45)
  - `NewRowLevelSecurityService`  (row_level_security.go:184)
  - `NewSalesActionTrigger`  (sales_action_trigger.go:73)
  - `NewSalesDashboard`  (sales_dashboard.go:104)
  - `NewSalesEngine`  (sales_engine.go:215)
  - `NewCustomerLookupAdapter`  (sales_engine_adapters.go:135)
  - `NewScriptLookupAdapter`  (sales_engine_adapters.go:31)
  - `NewSalesPersonaService`  (sales_persona_service.go:18)
  - `NewSalesPersonaServiceWithDB`  (sales_persona_service.go:28)
  - `NewPlaybookService`  (sales_playbook.go:82)
  - `NewSalesWorkbenchService`  (sales_workbench.go:111)
  - `NewScriptLoader`  (script_loader.go:27)
  - `NewSecurityAuditService`  (security_audit_service.go:23)
  - `NewSegmentService`  (segment_service.go:28)
  - `NewSegmentServiceWithDeps`  (segment_service.go:36)

## internal/service/self_learning
  - `NewAssetBundleLearner`  (asset_bundle_learner.go:87)
  - `NewAssetBundleSelfSupervisor`  (asset_bundle_self_supervisor.go:108)
  - `NewDialogueEventPublisher`  (dialogue_publisher.go:66)
  - `NewLLMSelfCorrector`  (llm_self_corrector.go:56)
  - `NewOrchestrator`  (orchestrator.go:77)
  - `newTestOrchestrator`  (orchestrator_test.go:198) [test]
  - `newTestRAGCorrector`  (orchestrator_test.go:212) [test]
  - `newTestAssetLearner`  (orchestrator_test.go:219) [test]
  - `newTestAssetSupervisor`  (orchestrator_test.go:224) [test]
  - `newOrchSwitchSvc`  (orchestrator_test.go:247) [test]
  - `NewRAGSelfCorrector`  (rag_self_corrector.go:63)
  - `NewRAGSelfSupervisor`  (rag_self_supervisor.go:56)
  - `newSupervisorWithMocks`  (rag_self_supervisor_test.go:246) [test]
  - `newSwitchSvcWithCache`  (rag_self_supervisor_test.go:260) [test]
  - `newDispatcherWithMocks`  (rag_self_supervisor_test.go:285) [test]
  - `NewSelfCorrectionDispatcher`  (self_correction_dispatcher.go:55)
  - `newDispatcherSetup`  (self_correction_dispatcher_test.go:132) [test]
  - `newDispatcherWithLLM`  (self_correction_dispatcher_test.go:163) [test]
  - `NewSwitchService`  (switch_service.go:86)
  - `newTestService`  (switch_service_test.go:279) [test] `[已修复→newSelfLearningTestService]`

## internal/service
  - `NewSelfLearningCron`  (self_learning_cron.go:40)
  - `NewSelfLearningService`  (self_learning_service.go:47)
  - `newTestSwitchRepo`  (self_learning_service_test.go:503) [test]
  - `newTestSelfLearningService`  (self_learning_service_test.go:508) [test]
  - `NewSensitiveFieldEncryption`  (sensitive_encryption.go:32)
  - `NewSessionAssignmentService`  (session_assignment.go:28)
  - `NewSessionTagService`  (session_tag.go:23)
  - `NewShortLinkService`  (short_link.go:55)
  - `newTestShortLinkService`  (short_link_test.go:27) [test]
  - `newTestShortLinkRepository`  (short_link_test.go:32) [test]
  - `newTestShortLinkAccessRepository`  (short_link_test.go:36) [test]
  - `NewSmlistService`  (sm_list.go:13)
  - `newTestSmlistRepository`  (sm_list_test.go:27) [test]
  - `NewSmartCSOrchestrator`  (smart_cs_orchestrator.go:71)
  - `NewSmsService`  (sms.go:64)
  - `NewSmsDeliveryTrackerService`  (sms_delivery_tracker.go:73)
  - `newDeliveryTracker`  (sms_delivery_tracker_test.go:25) [test]
  - `newTestSmsRepository`  (sms_test.go:34) [test]
  - `NewSmsTrackingService`  (sms_tracking.go:57)
  - `newSmsTrackingService`  (sms_tracking_test.go:28) [test]
  - `NewSmsUnsubscribeService`  (sms_unsubscribe.go:48)
  - `newSmsUnsubscribeService`  (sms_unsubscribe_test.go:26) [test]
  - `NewSOPExecutionDispatcher`  (sop_dispatcher.go:139)
  - `NewSOPLoader`  (sop_loader.go:30)
  - `NewNodeExecutorRegistry`  (sop_node_executor.go:115)
  - `NewMessageNodeExecutor`  (sop_node_executors.go:375)
  - `NewLLMNodeExecutor`  (sop_node_executors.go:583)
  - `NewWaitExecutor`  (sop_node_executors.go:741)
  - `NewSOPStuckDetector`  (sop_outbox_dispatcher.go:222)
  - `NewSOPOutboxDispatcher`  (sop_outbox_dispatcher.go:60)
  - `NewSOPScheduler`  (sop_scheduler.go:60)
  - `NewSOPService`  (sop_service.go:108)
  - `NewWelcomeSOP`  (sop_service.go:800)
  - `NewObjectionSOP`  (sop_service.go:838)
  - `NewSSEHub`  (sse_hub.go:154)
  - `NewSSEClient`  (sse_hub.go:73)
  - `NewSystemConfigService`  (system_config.go:17)
  - `newTestSystemConfigRepository`  (system_config_test.go:25) [test]
  - `NewSystemInitService`  (system_init.go:22)
  - `NewSystemMonitorServiceWithRepo`  (system_monitor.go:105)
  - `NewSystemMonitorService`  (system_monitor.go:98)
  - `NewSystemUserService`  (system_user.go:52)
  - `NewSystemUserServiceWithRepo`  (system_user.go:57)
  - `NewTikTokAutoReplyService`  (tiktok_auto_reply.go:30)
  - `NewTikTokCardService`  (tiktok_card.go:37)
  - `NewTikTokCardServiceWithDB`  (tiktok_card.go:45)
  - `NewFollowUpPortAdapter`  (tool_ports_adapter.go:137)
  - `NewOrderPortAdapter`  (tool_ports_adapter.go:194)
  - `NewAfterSalePortAdapter`  (tool_ports_adapter.go:256)
  - `NewCustomerPortAdapter`  (tool_ports_adapter.go:28)
  - `NewSessionPortAdapter`  (tool_ports_adapter.go:87)
  - `NewTuningService`  (tuning.go:84)
  - `NewUnifiedInboxService`  (unified_inbox.go:155)
  - `NewUnifiedMessageService`  (unified_message.go:246)
  - `NewReplyDecisionEngine`  (unified_message.go:24)
  - `NewUserService`  (user.go:32)
  - `newFakeLLMDispatcher`  (web_chat_e2e_test.go:128) [test]
  - `newFakeRAG`  (web_chat_e2e_test.go:67) [test]
  - `NewWebhookService`  (webhook_service.go:195)
  - `newWebhookEventRepoWithDB`  (webhook_service.go:331)
  - `newIntegrationAccountRepoWithDB`  (webhook_service.go:341)
  - `NewWeComService`  (wecom.go:31) **[web同名引用 ×3 · 相似度低(仅引用后端命名,无同名工具实现)]**
  - `NewWeComServiceWithDB`  (wecom.go:36)
  - `NewWeComAccountHealthService`  (wecom_account_health.go:58)
  - `newWeComHealthService`  (wecom_account_health_test.go:26) [test]
  - `NewWeComIntegrationService`  (wecom_integration.go:27)
  - `NewWhatsappService`  (whatsapp.go:29)
  - `NewWhatsAppTemplateService`  (whatsapp_template_service.go:20)
  - `NewWorkflowLoader`  (workflow_loader.go:27)
  - `NewWSAgentExecutor`  (ws_agent_executor.go:36)
  - `NewXianyuAutoReplyService`  (xianyu_auto_reply.go:26)
  - `NewXianyuCardService`  (xianyu_card.go:40)
  - `NewXianyuCardStatsService`  (xianyu_card_stats.go:33)
  - `NewXiaohongshuAutoReplyService`  (xiaohongshu_auto_reply.go:24)
  - `NewXiaohongshuCardService`  (xiaohongshu_card.go:38)
  - `NewXiaohongshuCardStatsService`  (xiaohongshu_card_stats.go:28)

## internal/system/install
  - `newInstallID`  (install.go:225)

## internal/websocket
  - `NewPendingAck`  (ack_tracker.go:31)
  - `NewEnvelope`  (envelope.go:18) **[web同名结构 ×3 · 相似度中(前端WS消息信封{seq,ts,type,payload},概念对应)]**
  - `NewWSHandler`  (handler.go:32)
  - `NewClient`  (hub.go:227) `[已修复→NewWSClient]`
  - `NewAgentClient`  (hub.go:232)
  - `NewVisitorClient`  (hub.go:243)
  - `NewHub`  (hub.go:54)
  - `NewVisitorWSHandler`  (visitor_handler.go:54)

## tests/perf/perflib
  - `NewLoadRunner`  (runner.go:56)

## user-web 同名（去 New 前缀）比对结果

- **比对方法**：将 1049 个 `New*` 函数名去掉 `New/new` 前缀，得到 1035 个基础名；在 user-web 源码（384 个 `.ts/.js/.vue/.tsx/.jsx/.mjs`，已排除 `node_modules/dist/.git`）中按"独立标识符 token"精确匹配。跨语言（Go↔TS/Vue）无法做代码体 diff，相似度采用启发式判定：是否作为**定义**（`function/class/export/const/...`）出现 vs 仅**引用**（注释/字符串/API 路径/import 说明）。
- **总体结论**：**user-web 中不存在与这 1049 个 `New*` 对应的同名（去 New）工具 / 函数 / 类定义**。
- **命中统计**：1035 个基础名中仅 **7 个**作为 token 出现，且**全部为"引用型(REF)"**，**0 个**为定义型。

| 基础名(去 New) | user-web 位置 | 命中数 | 性质 | 相似度 |
|---|---|---|---|---|
| AISuggestionController | `src/api/customerService.js:8` | 1 | 注释引用后端 Controller 名 | 低 |
| AgentStatusController | `src/api/customerService.js:5` | 1 | 注释引用后端 Controller 名 | 低 |
| QuickReplyController | `src/api/customerService.js:6` | 1 | 注释引用后端 Controller 名 | 低 |
| SessionTagController | `src/api/customerService.js:7` | 1 | 注释引用后端 Controller 名 | 低 |
| RagConfigController | `src/api/rag-product-config.js:3` | 1 | 注释引用后端 Controller 名 | 低 |
| WeComService | `src/api/wecomAccount.js:159/165/170` | 3 | 注释引用后端 Service 名 | 低 |
| Envelope | `src/utils/agentSocket.js:7,149` · `src/utils/chatSocket.js:6` | 3 | 前端 WS 消息信封结构 `{seq,ts,type,payload}`，概念对应 | 中 |

**说明**
- Controller / Service 系列：前端 API 模块注释明确写明"对应后端 controller/... 的 Controller / Service"，仅作路由与命名引用，**前端未实现同名工具**。
- Envelope：前端 WebSocket 客户端约定的下行消息包装格式（`{seq, ts, type, payload}`），是前端自己定义的协议结构；与后端 `NewEnvelope`（通常用于加密/邮件信封）语义大概率不同，属于**同名概念对应**，非代码体可比，故评"中"。

> 注：前端普遍采用 camelCase（`emailService` 等），而 Go 基础名为 PascalCase（`EmailService`），按"严格同名"匹配时 camelCase 命名不会命中；若需做"语义级"对应（如 `NewWeComService` ↔ `wecomAccount` 模块），需额外的命名映射与人工研判，本清单未自动纳入。

## user-server 内部「去 New 前缀」同名复用（收敛范围之外）

- **比对方法**：将 1049 个 `New*` 构造函数名去 `New/new` 前缀得到 1035 个基础名；在 user-server 全部 1475 个 `.go` 文件中按"独立标识符 token"匹配，并定位每个基础名在哪些**不同包**里被独立定义为类型/函数/变量/常量（`func/type/var/const/struct/interface`）。
- **总体结论**：1035 个基础名中，**91 个**在 **≥2 个不同包**里各自有同名定义（真正的跨包同名复用 / 潜在命名热点）；其余 944 个仅在单一包内出现（多为"构造函数 `NewXxx` ↔ 返回类型 `Xxx`"的同包正常绑定，不计入复用）。
- **分类**：
  - 同构分层复用（架构正常）：`*Repository`+`*Service` 同义 **36** 个；`*Controller`+`*Service`+`*Router` 同义 **6** 个。
  - 高散度（同名出现在 ≥4 个包）：`Client`(11)、`LangConfigResolver`(5)、`Dispatcher`(4)。
  - 潜在同名类型冲突（建议抽取共享包）：`Logger`、`TestError`、`TraceContext`、`JWTUtils`、`Client`、`Hub`、`SSEHub` 等。

### 91 个跨包同名定义清单（基础名 · 定义包数 · 包列表）

| 基础名(去 New) | 包数 | 定义所在包 |
|---|---|---|
| Client | 11 | cmd/api; internal/websocket; internal/cache; internal/platform; internal/controller; internal/channelbot/core; internal/channelbot/telegram; internal/aiagent/agent/runtime; internal/aiagent/knowledge/service; internal/service; internal/pkg/utils/tgbot |
| LangConfigResolver | 5 | internal/middleware; internal/websocket; internal/controller; internal/service/i18n; internal/router |
| Dispatcher | 4 | internal/aiagent/llm; internal/aiagent/rag/retrieval; internal/service; internal/router |
| BanditAllocator | 3 | internal/service/self_learning; internal/service; internal/service/feedback_loop |
| DingTalkAppService | 3 | internal/controller; internal/service; internal/router |
| ProviderFailover | 3 | internal/controller; internal/aiagent/llm; internal/router |
| RateLimiter | 3 | internal/middleware; internal/aiagent/agent/tooluse; internal/aiagent/agent/browser |
| SalesEngine | 3 | internal/controller; internal/service; internal/router |
| SmartCSOrchestrator | 3 | internal/controller; internal/service; internal/router |
| WebhookService | 3 | internal/controller; internal/service; internal/router |
| WhatsAppCloudService | 3 | internal/controller; internal/service; internal/router |
| AIAgentRepository | 2 | internal/repository; internal/service |
| AIAgentService | 2 | internal/service; internal/router |
| AssetBundleRepository | 2 | internal/repository; internal/service/self_learning |
| AutoReplyBot | 2 | internal/platform; internal/aiagent/agent/browser |
| AutoReplyManager | 2 | internal/controller; internal/aiagent/agent/browser |
| BackupRepository | 2 | internal/repository; internal/service |
| ChannelAgentBindingService | 2 | internal/controller; internal/service |
| ChatChannelService | 2 | internal/middleware; internal/service |
| ConfidenceAggregator | 2 | internal/service/confidence; internal/service |
| CoreDataFlowOrchestrator | 2 | internal/aiagent/agent/runtime; internal/router |
| CustomerRepository | 2 | internal/repository; internal/service |
| CustomerSessionService | 2 | internal/aiagent/agent/tooluse; internal/service |
| DashboardScreenRepository | 2 | internal/ops/repository; internal/ops/service |
| DashboardWidgetRepository | 2 | internal/ops/repository; internal/ops/service |
| DialogueMemoryRepository | 2 | internal/repository; internal/service |
| DomainPoolRepository | 2 | internal/repository; internal/service |
| DouyinCardService | 2 | internal/controller; internal/service |
| EmailDraftRepository | 2 | internal/repository; internal/email/service |
| EmailJobsRepository | 2 | internal/repository; internal/email/service |
| EmbeddingService | 2 | internal/aiagent/llm; internal/aiagent/knowledge/service |
| FallbackBridge | 2 | internal/aiagent/rag/customer_service; internal/service/i18n |
| FeedbackCollector | 2 | internal/service; internal/service/feedback_loop |
| FeedbackLoopRepository | 2 | internal/repository; internal/service/feedback_loop |
| FeishuMessageRepository | 2 | internal/repository; internal/service |
| Hub | 2 | internal/websocket; internal/service |
| HumanizeEvalService | 2 | internal/service; internal/service/humanize |
| HumanizeScoreRepository | 2 | internal/repository; internal/service/humanize |
| HybridSearcher | 2 | internal/aiagent/rag/retrieval; internal/aiagent/knowledge/service |
| InboxAssignmentRepository | 2 | internal/repository; internal/service |
| InboxConversationRepository | 2 | internal/repository; internal/service |
| IncrementalIndexer | 2 | internal/aiagent/agent/runtime; internal/aiagent/rag/incremental |
| IntegrationAccountRepository | 2 | internal/repository; internal/service |
| IntentLogRepository | 2 | internal/repository; internal/service |
| IntentRecognizer | 2 | internal/aiagent/agent/runtime; internal/service |
| IntentRecordRepository | 2 | internal/repository; internal/service |
| JWTUtils | 2 | internal/service; internal/pkg/utils |
| LLMService | 2 | internal/aiagent/llm; internal/aiagent/rag/customer_service |
| LiveCodeController | 2 | internal/controller; internal/router |
| LocalEmbedding | 2 | internal/aiagent/embedding; internal/service |
| Logger | 2 | internal/pkg/utils/logger; internal/pkg/utils |
| MarketingFlowService | 2 | internal/content/service; internal/service |
| MemoryAuditLogger | 2 | internal/aiagent/agent/tooluse; internal/router |
| MemoryCostTracker | 2 | internal/aiagent/agent/tooluse; internal/router |
| MessageHubRepository | 2 | internal/repository; internal/service |
| MessageQueueRepository | 2 | internal/repository; internal/service |
| MigrationRegistry | 2 | internal/migration/migrations; internal/migration |
| PersonaLowQualitySampleRepository | 2 | internal/repository; internal/service |
| PlatformAccountRepository | 2 | internal/repository; internal/service |
| PlatformController | 2 | internal/controller; internal/router |
| ProviderRegistry | 2 | internal/aiagent/agent/tooluse; internal/router |
| RAGEngine | 2 | internal/aiagent/rag/core; internal/service/self_learning |
| RAGStack | 2 | internal/controller; internal/aiagent/knowledge/service |
| RagAlertRepository | 2 | internal/repository; internal/service |
| RagCustomerService | 2 | internal/aiagent/agent/auto_reply; internal/aiagent/rag/customer_service |
| RagRetrievalService | 2 | internal/aiagent/agent/auto_reply; internal/aiagent/rag/retrieval |
| RestoreRecordRepository | 2 | internal/repository; internal/service |
| SSEHub | 2 | internal/controller; internal/service |
| SalesPersonaRepository | 2 | internal/repository; internal/service |
| SelfCorrectionActionRepository | 2 | internal/repository; internal/service/self_learning |
| SendPipeline | 2 | internal/aiagent/agent/tooluse; internal/service |
| SessionMessageRepository | 2 | internal/repository; internal/service |
| ShortLinkAccessRepository | 2 | internal/repository; internal/service |
| ShortLinkRepository | 2 | internal/repository; internal/service |
| SmlistRepository | 2 | internal/repository; internal/service |
| SmsRepository | 2 | internal/repository; internal/service |
| SopAgentRepository | 2 | internal/repository; internal/service |
| SopExecutionRepository | 2 | internal/repository; internal/service |
| SystemConfigRepository | 2 | internal/repository; internal/service |
| TestError | 2 | internal/pkg/utils/logger; internal/pkg/utils/response |
| ToolExecutor | 2 | internal/aiagent/agent/tooluse; internal/router |
| ToolRegistry | 2 | internal/aiagent/agent/tooluse; internal/router |
| ToolRouter | 2 | internal/aiagent/agent/tooluse; internal/router |
| TraceContext | 2 | internal/middleware; internal/aiagent/llm |
| TranslationCache | 2 | internal/aiagent/rag/customer_service; internal/aiagent/rag/retrieval |
| UnifiedMessageRepository | 2 | internal/repository; internal/service |
| Vectorizer | 2 | internal/aiagent/rag/retrieval; internal/aiagent/vector |
| WeComIntegrationService | 2 | internal/aiagent/agent/tooluse; internal/service |
| WebhookEventRepository | 2 | internal/repository; internal/service |
| WhatsappService | 2 | internal/controller; internal/service |
| WhitelistPermissionChecker | 2 | internal/aiagent/agent/tooluse; internal/router |

> 说明：本表仅列"在 ≥2 个不同包各自有同名定义"的基础名。由于 Go 以包为命名空间，这些同名类型/函数**不会编译冲突**，但属于"同一概念在多包重复定义"，其中 `Client`/`Logger`/`TraceContext`/`JWTUtils`/`TestError` 等通用基础设施类建议抽取为共享包以减少重复与漂移风险。
## 编码规范违例检查：「Hub 与 NewHub 不可并存」

> 检查规则（来自编码规范）：对同一概念名 `X`，应当只有一种命名形态。
> - 正确写法：`type X` + `func NewX()`（包内 `NewX` 返回 `X`）。
> - 违例形态 A（同包）：包内同时存在 `func NewX` 与裸 `func/var/const X`。
> - 违例形态 B（跨包重复构造器）：同一构造函数名 `NewX` 在 ≥2 个不同包各自定义（即「另一个 NewHub」）。
> - 违例形态 C（跨包同名混用）：同一 `X` 在某包作裸符号、在另一包作 `NewX`（建议复核，Go 包隔离不报错）。
>
> 扫描范围：user-server 全部 1475 个 `.go` 文件，18612 条顶层声明，1033 个构造函数基类名。

### 结论汇总

| 检查项 | 结果 | 说明 |
|--------|------|------|
| A. 同包「裸符号 + 构造器」 | **0 违例** | 单包内命名 100% 合规，`NewX` 始终配对 `type X`（共 795 对合法 `type X`+`NewX`） |
| B. 跨包重复构造器 `NewX` | **3 个基类名 / 7 处** | 字面意义的「另一个 NewHub」，确属不规范 |
| C. 跨包同名混用 `X`↔`NewX` | 15 个候选 | 多数为不同包的「同名不同类型」（Go 包命名空间隔离，编译无误），仅作复核建议 |

### B. 跨包重复构造器（✅ 已全部修复，见内联 `[已修复→…]` 标记）

1. **`NewClient`** — 原 3 个包各自定义构造函数，已分别重命名（2026-07-25 修复）：
   - `internal/websocket/hub.go` `func NewClient` → **`NewWSClient`**
   - `internal/platform/client.go` `func NewClient` → **`NewPlatformClient`**
   - `internal/channelbot/telegram/telegram.go` `func NewClient` → **`NewTelegramClient`**
   - 同步更新全部调用点（含 `controller/platform.go`、`feishu_service.go`、`tgbot.go`、`websocket/handler.go` 及对应测试）。

2. **`newStubLLMDispatcher`** — 2 个测试包重复，因实现不同接口（`ChatSend` vs `Dispatch`）未合并，已各包改名：
   - `internal/service/feedback_loop/helpers_test.go` → **`newFeedbackLoopStubLLMDispatcher`**
   - `internal/service/humanize/helpers_test.go` → **`newHumanizeStubLLMDispatcher`**

3. **`newTestService`** — 2 个测试包重复且签名不同，已各包改名：
   - `internal/service/self_learning/switch_service_test.go`（及 orchestrator/rag_self_supervisor/self_correction_dispatcher 测试）→ **`newSelfLearningTestService`**
   - `internal/service/message_hub_service_test.go` → **`newMessageHubTestService`**

### C. 跨包同名混用候选（建议复核，非编译错误）

以下基类名在某包作裸 `type/func/var/const X`、在另一包作 `func NewX`。因 Go 按包隔离命名空间，均**不引起编译错误**；但若指向同一业务概念，建议统一命名。

- `RateLimiter`：CTOR `browser` / BARE `type` `middleware`、`tooluse`
- `TestError`：CTOR `event`(test) / BARE `func` `logger`(test)、`response`(test)
- `AssetBundleRepository`：CTOR `repository` / BARE `type` `selflearning`
- `BanditAllocator`：CTOR `feedbackloop` / BARE `type` `selflearning`
- `ChampionBaselineRepository`：CTOR `repository` / BARE `type` `humanize`
- `Client`：CTOR `websocket`/`platform`/`telegram` / BARE `var` `httpclient`
- `Ctx`：CTOR `pagination`(test) / BARE `func` `logger`
- `FallbackBridge`：CTOR `i18n` / BARE `type` `ragcustomerservice`
- `HumanizeScoreRepository`：CTOR `repository` / BARE `type` `humanize`
- `IntentRecognizer`：CTOR `service` / BARE `type` `agent_runtime`
- `PlatformAPIClient`：CTOR `platform` / BARE `type` `repository`
- `RAGEngine`：CTOR `rag_core` / BARE `type` `selflearning`
- `SalesEngineBridge`：CTOR `agent_bridge` / BARE `type` `agent_runtime`
- `Vectorizer`：CTOR `ragretrieval` / BARE `type` `vector`
- `WithTimeout`：CTOR `httpclient` / BARE `func` `channelbot/core`

### 标记说明与交付（2026-07-25）
- 内联清单中 7 处违例行已改为 `[已修复→新名]`。
- `B` 类 3 项（7 处）已全部修复并通过编译 + 测试验证（`go build ./...` 通过；websocket/telegram/humanize/feedback_loop/self_learning/MessageHub 相关测试全绿）。
- `C` 类 15 项经论证**不修复**：均为不同包的「同名不同类型」，Go 按包隔离命名空间，编译无误；强行改名会把恰好同名的不同类型无意义地改掉，违背规范本意。
- `internal/service` 整包仅剩 1 例失败 `TestWebhookService_Receive_TelegramJoinEvent`，与本次改名无关（包已编译通过，失败源于 LLM 端点 8207 未启动的 Telegram webhook 测试，属环境依赖）。
