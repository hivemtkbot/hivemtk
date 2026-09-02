package app

import (
	"context"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	knowledgesvc "hivemtk-user/internal/aiagent/knowledge/service"
	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/bridge"
	"hivemtk-user/internal/cache"
	contentservice "hivemtk-user/internal/content/service"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
	"hivemtk-user/internal/service/translation"

	"gorm.io/gorm"
)


// buildSalesEngine 构建智能体销冠引擎（真实依赖注入）
// 调用方：router.Setup()
//
// 意图识别实例统一复用全局单例（main.go 中 InitIntentRecognizer 初始化），
// 与 /api/intent/* 直连路由共享：
//   - 同一份 dispatcher / db / cache
//   - 同一份 SOP service 联动
//   - 同一份 IntentEnabled 开关
//
// 避免双实例导致 SOP 联动只在直连路由生效、销售引擎调用不生效的分裂问题。
func BuildSalesEngine(gormDB *gorm.DB) *service.SalesEngine {
	dispatcher := llm.GetGlobalDispatcher()

	memorySvc := service.NewDialogueMemoryService(gormDB, dispatcher)

	intentSvc := service.GetIntentRecognizer()
	if intentSvc == nil {
		intentSvc = service.NewIntentRecognizer(gormDB, dispatcher, nil)
	}

	sopSvc := service.NewSOPService(gormDB, dispatcher)

	ragSearcher := knowledgesvc.NewRagSearcher()

	scriptLookup := service.NewScriptLookupAdapter(contentservice.NewScriptTemplateService())
	customerLookup := service.NewCustomerLookupAdapter(repository.NewCustomerRepository())

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

	engine.SetFeedbackLearner(context.Background(), service.NewFeedbackLearner(gormDB))

	confidenceAgg := service.GetConfidenceAggregator()
	if confidenceAgg == nil {
		confidenceAgg = service.InitConfidenceAggregator(gormDB, dispatcher, nil)
	}
	_ = confidenceAgg 

	humanizeSvc := service.GetHumanizeEvalService()
	if humanizeSvc == nil {
		humanizeSvc = service.InitHumanizeEvalService(gormDB, dispatcher)
	}
	engine.SetHumanizeEvaluator(context.Background(), humanizeSvc)
	service.SetHumanizeRegenerateDispatcher(dispatcher)

	toolExec := tooluse.GetGlobalExecutor()
	if toolExec != nil {
		engine.SetToolExecutor(context.Background(), NewToolExecutorAdapter(toolExec))
		logger.Info("[agent] ✅ SalesEngine 已注入 ToolExecutor（Agent Loop 已启用）")
	} else {
		logger.Warn("[agent] SalesEngine 未注入 ToolExecutor（globalExecutor 未初始化，走原 9 步流水线）")
	}

	if pc := GetGlobalPermissionChecker(); pc != nil {
		engine.SetPermissionChecker(pc)
		logger.Info("[agent] ✅ SalesEngine 已注入工具权限检查器（按 Agent 白名单执行期放行）")
	}

	glossarySvc := translation.NewGlossaryService(repository.NewGlossaryRepositoryWithDB(db.GetDB()), cache.NewMemoryCache())
	engine.SetGlossaryRenderer(glossarySvc)
	engine.SetOutputCalibrator(glossarySvc)
	logger.Info("[agent] ✅ SalesEngine 已注入回复语言链路（术语表 + 后置校准）")

	return engine
}

// buildSmartOrchestrator 构建智能体统一编排器
// 调用方：router.Setup()
// 设计：以 SalesEngine 为核心，套一层 SmartCSOrchestrator 编排壳，实现
//
//	"LLM 能力 + 智能体 协作体"：高置信度自动回复，低置信度转人工 + 推送建议
//	座席可随时接管智能体会话（人机协同）
//
// 配置：使用 DefaultOrchestratorConfig（置信度 0.7 / 自动回复开 / 自动连续上限 10）
//
// 调优记录：9B 4-bit 在 RAG 短问答上 confidence 评估均值 ~0.55-0.65，
// 默认 0.7 阈值导致 80% 业务问答被判定为"低置信度"转人工。降到 0.5 让 AI 接管更多场景。
func BuildSmartOrchestrator(engine *service.SalesEngine, kbRepo *repository.KnowledgeBaseRepository) *service.SmartCSOrchestrator {
	cfg := service.DefaultOrchestratorConfig()
	cfg.ConfidenceThreshold = 0.5
	o := service.NewSmartCSOrchestrator(engine, cfg, kbRepo)
	o.SetIdentityService(service.NewCustomerIdentityService())
	return o
}

// registerAgentReachTools 生产接线：把智能体全部工具（含 reach.web.send）
// 真实注册到全局工具注册中心，并注入【真实】IntegrationReachAdapter。
//
// 关键：使用 NewReachToolDepsWithAdapter（而非 WithDB 的 NoOp 桥接），
// 让 SendPipeline 底层桥接真实 adapter，确保 reach.web.send 在生产真正落库 +
// 实时推访客 WebSocket，而非空壳 NoOp。
//
// 2026-08-18 二次审核重构：保留 BridgeReachAdapter 包装层（满足 tooluse.ReachAdapter 接口需
// SendDouyin/SendKuaishou/XHS/TikTok/Xianyu 五种方法名），但其内部把"网页渠道"方法直接转给
// service.DeliverBridgeOutbound。修复前这五个方法走 httpReplyBuffer 死通道，消息静默丢失。
func RegisterAgentReachTools(gormDB *gorm.DB) {
	adapter := bridge.NewBridgeReachAdapter(NewIntegrationReachAdapterFromDB(gormDB), GetBridgeIngressSvc())
	deps := NewReachToolDepsWithAdapter(gormDB, adapter)

	if err := tooluse.RegisterReachTools(tooluse.GetGlobalRegistry(), deps); err != nil {
		logger.Errorf("[agent] 注册触达工具失败（reach.web.send 等将不可用）：%v", err)
		return
	}
	logger.Info("[agent] ✅ 触达工具（含 reach.web.send 网页客服）已真实接入全局注册中心")
}

// registerAgentPrivateMessageTools 将「私信工具」注册到全局注册中心。
// 私信模块（CustomerSessionService）是智能体对话域载体：被动模式读取/回复会话，
// 主动模式由智能体开启私信会话与用户链接。详见 （双模式）。
//
// 调用方：router.Setup()
func RegisterAgentPrivateMessageTools(gormDB *gorm.DB) {
	sessionSvc := service.NewCustomerSessionServiceWithDB(gormDB)
	deps := tooluse.NewPrivateMessageToolDepsWithPort(service.NewSessionPortAdapter(sessionSvc))
	if err := tooluse.RegisterPrivateMessageTools(tooluse.GetGlobalRegistry(), deps); err != nil {
		logger.Errorf("[agent] 注册私信工具失败（pm.* 将不可用）：%v", err)
		return
	}
	logger.Info("[agent] ✅ 私信工具（pm.session.open/read/message.send）已接入全局注册中心")
}

