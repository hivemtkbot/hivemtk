package router

import (
	"context"

	"marketing/internal/aiagent/agent/tooluse"
	knowledgesvc "marketing/internal/aiagent/knowledge/service"
	"marketing/internal/aiagent/llm"
	contentservice "marketing/internal/content/service"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"marketing/internal/service"

	"gorm.io/gorm"
)

// ============================================================================
// SalesEngine 工厂
// ----------------------------------------------------------------------------
// 集中构建智能体销冠引擎及其 9 步链路依赖，避免 router.go 出现大量零散构造代码。
// 五层架构：本文件位于 Router 层，依赖 Service 层，不直接访问 DB（通过 db.GetDB()）。
//
// 9 步链路依赖对应：
//   ① resolveCustomer → CustomerLookup 适配器（→ CustomerRepository）
//   ② recallMemory    → DialogueMemoryService
//   ③ 意图识别         → IntentRecognizer
//   ④ SOP 匹配         → SOPService
//   ⑤ RAG 召回         → RAGSearcher 适配器（→ RagSearcher）
//   ⑥ LLM 生成         → llm.Dispatcher
//   ⑦ 拟人润色         → HumanizePolisher（SalesEngine 内部创建）
//   ⑧ 发送前审核       → ContentAuditor（SalesEngine 内部创建）
//   ⑨ 反馈学习         → FeedbackLearner（通过 SetFeedbackLearner 注入，智能体自我进化闭环）
// ============================================================================

// buildSalesEngine 构建智能体销冠引擎（真实依赖注入）
// 调用方：router.Setup()
func buildSalesEngine(gormDB *gorm.DB) *service.SalesEngine {
	dispatcher := llm.GetGlobalDispatcher()

	// ② 对话记忆服务
	memorySvc := service.NewDialogueMemoryService(gormDB, dispatcher)

	// ③ 意图识别服务（cache 传 nil，使用进程内规则匹配 + LLM 兜底）
	intentSvc := service.NewIntentRecognizer(gormDB, dispatcher, nil)

	// ④ SOP 智能体服务
	sopSvc := service.NewSOPService(gormDB, dispatcher)

	// ⑤ RAG 召回适配器
	ragSearcher := knowledgesvc.NewRagSearcher()

	// 话术库 + 客户查询适配器
	scriptLookup := service.NewScriptLookupAdapter(contentservice.NewScriptTemplateService())
	customerLookup := service.NewCustomerLookupAdapter(repository.NewCustomerRepository())

	// 构建 SalesEngine（db 通过 db.GetDB() 保证全局一致）
	engine := service.NewSalesEngine(
		db.GetDB(),
		dispatcher,
		intentSvc,
		memorySvc,
		sopSvc,
		ragSearcher,
		scriptLookup,
		customerLookup,
	)

	// ⑨ 反馈学习器注入（AI 自我进化闭环）
	// 每次 Handle 结束都记录决策快照（intent/confidence/SOP/回复/是否转人工），
	// 后续客户下一条消息或人工接管时由 SmartCSOrchestrator 更新 CustomerAccept
	engine.SetFeedbackLearner(context.Background(), service.NewFeedbackLearner(gormDB))

	// P0-3 置信度聚合器注入（5 维信号 + 动态阈值驱动转人工）
	// 注入后 shouldTransferToHuman 不再使用静态规则（IntentChurn/IntentComplaint/MessageCount>30），
	// 而是由聚合置信度 + 动态阈值决策
	confidenceAgg := service.GetConfidenceAggregator()
	if confidenceAgg == nil {
		// 自动初始化（依赖 nil embedder 走 CtxRelev=0.5 降级路径）
		confidenceAgg = service.InitConfidenceAggregator(gormDB, dispatcher, nil)
	}
	engine.SetConfidenceAggregator(context.Background(), confidenceAgg)

	// P0-4 拟人度评估器注入（RuleScorer 全量 + LLMScorer 边界采样 + 重生成）
	// 注入后 Step 7.5 评估回复自然度，<0.85 触发重生成（最多 3 次）
	humanizeSvc := service.GetHumanizeEvalService()
	if humanizeSvc == nil {
		humanizeSvc = service.InitHumanizeEvalService(gormDB, dispatcher)
	}
	engine.SetHumanizeEvaluator(context.Background(), humanizeSvc)
	// 注入重生成 dispatcher：让评估器在拟人不达标时调用 LLM 重新生成
	service.SetHumanizeRegenerateDispatcher(dispatcher)

	// P0-3 真正的智能体 Agent Loop 注入（LLM ↔ 工具 循环）
	// 通过 ToolExecutorAdapter 适配 *tooluse.ToolExecutor → service.AgentToolExecutor 接口
	// 避免直接依赖 tooluse 包导致循环依赖（tooluse 反向依赖 service 持有 IntegrationService）
	toolExec := tooluse.GetGlobalExecutor()
	if toolExec != nil {
		engine.SetToolExecutor(context.Background(), NewToolExecutorAdapter(toolExec))
		logger.Info("[agent] ✅ SalesEngine 已注入 ToolExecutor（Agent Loop 已启用）")
	} else {
		logger.Warn("[agent] SalesEngine 未注入 ToolExecutor（globalExecutor 未初始化，走原 9 步流水线）")
	}

	return engine
}

// buildSmartOrchestrator 构建智能体统一编排器
// 调用方：router.Setup()
// 设计：以 SalesEngine 为核心，套一层 SmartCSOrchestrator 编排壳，实现
//
//	"LLM 能力 + 智能体 协作体"：高置信度自动回复，低置信度转人工 + 推送建议
//	座席可随时接管智能体会话（人机协同）
//
// 配置：使用 DefaultOrchestratorConfig（置信度 0.7 / 自动回复开 / 自动连续上限 5）
func buildSmartOrchestrator(engine *service.SalesEngine) *service.SmartCSOrchestrator {
	return service.NewSmartCSOrchestrator(engine, service.DefaultOrchestratorConfig())
}

// registerAgentReachTools 生产接线：把智能体全部工具（含 reach.web.send）
// 真实注册到全局工具注册中心，并注入【真实】IntegrationReachAdapter。
//
// 关键：使用 NewReachToolDepsWithAdapter（而非 WithDB 的 NoOp 桥接），
// 让 SendPipeline 底层桥接真实 adapter，确保 reach.web.send 在生产真正落库 +
// 实时推访客 WebSocket，而非空壳 NoOp。
//
// 调用方：router.Setup()
func registerAgentReachTools(gormDB *gorm.DB) {
	// 真实触达适配器（含 web 网页客服渠道）
	adapter := tooluse.NewIntegrationReachAdapterFromDB(gormDB)
	deps := tooluse.NewReachToolDepsWithAdapter(gormDB, adapter)

	// 注册全部 20 个触达工具到全局注册中心
	if err := tooluse.RegisterReachTools(tooluse.GetGlobalRegistry(), deps); err != nil {
		logger.Errorf("[agent] 注册触达工具失败（reach.web.send 等将不可用）：%v", err)
		return
	}
	logger.Info("[agent] ✅ 触达工具（含 reach.web.send 网页客服）已真实接入全局注册中心")
}

// registerAgentPrivateMessageTools 将「私信工具」注册到全局注册中心。
// 私信模块（CustomerSessionService）是智能体对话域载体：被动模式读取/回复会话，
// 主动模式由智能体开启私信会话与用户链接。详见 ADR-013（双模式）。
//
// 调用方：router.Setup()
func registerAgentPrivateMessageTools(gormDB *gorm.DB) {
	sessionSvc := service.NewCustomerSessionServiceWithDB(gormDB)
	deps := tooluse.NewPrivateMessageToolDeps(sessionSvc)
	if err := tooluse.RegisterPrivateMessageTools(tooluse.GetGlobalRegistry(), deps); err != nil {
		logger.Errorf("[agent] 注册私信工具失败（pm.* 将不可用）：%v", err)
		return
	}
	logger.Info("[agent] ✅ 私信工具（pm.session.open/read/message.send）已接入全局注册中心")
}
