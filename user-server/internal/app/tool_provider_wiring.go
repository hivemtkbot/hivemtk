package app

import (
	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/bridge"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
)



// ReachToolProvider 提供 20 个多渠道触达工具
type ReachToolProvider struct{}

func (p *ReachToolProvider) Name() string                   { return "reach" }
func (p *ReachToolProvider) Category() tooluse.ToolCategory { return tooluse.CategoryReach }
func (p *ReachToolProvider) Description() string {
	return "多渠道触达工具（短信/邮件/微信/抖音/小红书/Telegram/WhatsApp/飞书等 20 个）"
}

// Provide 构造触达工具集
//
// 内部完成 ReachAdapter 装配（从 DB 读取真实集成配置）。
//
// 2026-08-18 二次审核重构：BridgeReachAdapter 仅作 ReachAdapter 接口的合规包装（提供 SendDouyin/
// Kuaishou/XHS/TikTok/Xianyu 五种网页渠道方法名），其内部把"网页渠道"方法直接转给
// service.DeliverBridgeOutbound，不再走 WS callback 间接层。2026-08-18 修复前，
// 这五个方法把消息推入 in-memory httpReplyBuffer，buffer 无人读取 → 静默丢消息。
func (p *ReachToolProvider) Provide(ctx tooluse.ProviderContext) ([]tooluse.Tool, error) {
	if ctx.DB == nil {
		return nil, errProviderDBRequired
	}
	adapter := bridge.NewBridgeReachAdapter(NewIntegrationReachAdapterFromDB(ctx.DB), GetBridgeIngressSvc())
	deps := NewReachToolDepsWithAdapter(ctx.DB, adapter)
	return tooluse.BuildReachTools(deps), nil
}


// PrivateMessageToolProvider 提供 3 个私信工具
type PrivateMessageToolProvider struct{}

func (p *PrivateMessageToolProvider) Name() string { return "pm" }
func (p *PrivateMessageToolProvider) Category() tooluse.ToolCategory {
	return tooluse.CategoryPrivateMessage
}
func (p *PrivateMessageToolProvider) Description() string {
	return "私信工具（pm.session.open/read/message.send）"
}

func (p *PrivateMessageToolProvider) Provide(ctx tooluse.ProviderContext) ([]tooluse.Tool, error) {
	if ctx.DB == nil {
		return nil, errProviderDBRequired
	}
	sessionSvc := service.NewCustomerSessionServiceWithDB(ctx.DB)
	deps := tooluse.NewPrivateMessageToolDepsWithPort(service.NewSessionPortAdapter(sessionSvc))
	return tooluse.BuildPrivateMessageTools(deps), nil
}


// CustomerToolProvider 提供 8 个客户工具
type CustomerToolProvider struct{}

func (p *CustomerToolProvider) Name() string                   { return "customer" }
func (p *CustomerToolProvider) Category() tooluse.ToolCategory { return tooluse.CategoryCustomer }
func (p *CustomerToolProvider) Description() string {
	return "客户工具（search/get/create/update/merge/add_tag/remove_tag/segment）"
}

func (p *CustomerToolProvider) Provide(ctx tooluse.ProviderContext) ([]tooluse.Tool, error) {
	if ctx.DB == nil {
		return nil, errProviderDBRequired
	}
	customerPort := service.NewCustomerPortAdapter(service.NewCustomerService())
	deps := tooluse.NewCustomerToolDepsWithPort(customerPort, ctx.DB, NewCustomerDataStore())
	return tooluse.BuildCustomerTools(deps), nil
}


// KnowledgeToolProvider 提供 4 个知识工具
type KnowledgeToolProvider struct{}

func (p *KnowledgeToolProvider) Name() string                   { return "knowledge" }
func (p *KnowledgeToolProvider) Category() tooluse.ToolCategory { return tooluse.CategoryKnowledge }
func (p *KnowledgeToolProvider) Description() string {
	return "知识工具（rag.search/knowledge.feedback/add_doc/list_kb）"
}

func (p *KnowledgeToolProvider) Provide(ctx tooluse.ProviderContext) ([]tooluse.Tool, error) {
	if ctx.DB == nil {
		return nil, errProviderDBRequired
	}
	deps := tooluse.NewKnowledgeToolDepsWithDB(ctx.DB)
	return tooluse.BuildKnowledgeTools(deps), nil
}


// BusinessToolProvider 提供 5 个业务工具
type BusinessToolProvider struct{}

func (p *BusinessToolProvider) Name() string                   { return "business" }
func (p *BusinessToolProvider) Category() tooluse.ToolCategory { return tooluse.CategoryBusiness }
func (p *BusinessToolProvider) Description() string {
	return "业务工具（follow_task.create/update、order.lookup、aftersale.create/query）"
}

func (p *BusinessToolProvider) Provide(ctx tooluse.ProviderContext) ([]tooluse.Tool, error) {
	if ctx.DB == nil {
		return nil, errProviderDBRequired
	}
	orderPort := service.NewOrderPortAdapter(repository.NewExternalOrderRepository())
	afterSalePort := service.NewAfterSalePortAdapter(service.NewAfterSaleService())
	deps := tooluse.NewBusinessToolDepsWithPorts(
		service.NewFollowUpPortAdapter(service.NewFollowUpService(service.NewCustomerJourneyService())),
		orderPort,
		afterSalePort,
		ctx.DB,
	)
	return tooluse.BuildBusinessTools(deps), nil
}


// errProviderDBRequired Provider 依赖 DB 但未提供
var errProviderDBRequired = &providerError{"provider requires DB in ProviderContext"}

type providerError struct{ msg string }

func (e *providerError) Error() string { return e.msg }


// globalProviderRegistry 全局 ProviderRegistry 单例
//
// 在 registerAllAgentTools 中初始化，供 /api/agent/tools/providers 接口读取
var globalProviderRegistry *tooluse.ProviderRegistry

// GetGlobalProviderRegistry 读取全局 ProviderRegistry（未装配时为 nil）
func GetGlobalProviderRegistry() *tooluse.ProviderRegistry { return globalProviderRegistry }

// SetGlobalProviderRegistryForTest 仅用于测试：临时替换/清空全局 ProviderRegistry
func SetGlobalProviderRegistryForTest(r *tooluse.ProviderRegistry) { globalProviderRegistry = r }

// initBuiltinToolProviders 注册 5 个内置 Provider 到指定 registry
//
// 调用方：registerAllAgentTools
func initBuiltinToolProviders(registry *tooluse.ProviderRegistry) {
	providers := []tooluse.ToolProvider{
		&ReachToolProvider{},
		&PrivateMessageToolProvider{},
		&CustomerToolProvider{},
		&KnowledgeToolProvider{},
		&BusinessToolProvider{},
	}
	for _, p := range providers {
		if err := registry.RegisterProvider(p); err != nil {
			logger.Errorf("[agent] 注册内置 Provider %s 失败: %v", p.Name(), err)
		}
	}
}

// registerAllAgentToolsViaProviders 通过 Provider 模式装配所有工具
//
// 这是 registerAllAgentTools 的新实现（+ 优化），
// 替代原有硬编码 5 个 registerAgent*Tools 调用。
//
// 流程：
//  1. 创建 ProviderRegistry
//  2. 注册 5 个内置 Provider
//  3. 注册所有自注册的第三方 Provider
//  4. 调用 RegisterAll 批量装配到 ToolRegistry
//  5. 日志输出装配结果
//  6. 初始化 ToolRouter（依赖工具已注册）
func registerAllAgentToolsViaProviders(gormDB *gorm.DB) {
	providerRegistry := tooluse.NewProviderRegistry()
	globalProviderRegistry = providerRegistry

	initBuiltinToolProviders(providerRegistry)

	autoProviders := tooluse.GetAutoRegisteredProviders()
	for _, p := range autoProviders {
		if err := providerRegistry.RegisterProvider(p); err != nil {
			logger.Errorf("[agent] 注册第三方 Provider %s 失败: %v", p.Name(), err)
		} else {
			logger.Infof("[agent] ✅ 第三方 Provider %s 已注册", p.Name())
		}
	}

	ctx := tooluse.ProviderContext{
		DB:     gormDB,
		Config: tooluse.ProviderConfig{Enabled: true}, 
	}
	results, err := providerRegistry.RegisterAll(ctx, tooluse.GetGlobalRegistry())
	if err != nil {
		logger.Errorf("[agent] ⚠️ Provider 批量装配存在失败: %v", err)
	}

	tooluse.RegisterCardTools(tooluse.GetGlobalRegistry())
	logger.Info("[agent] ✅ 会话内卡片工具（card.show）已接入全局注册中心")

	totalTools := 0
	totalProviders := 0
	failedProviders := 0
	for _, r := range results {
		if r.Skipped {
			logger.Infof("[agent] ⏭️  Provider %s 跳过: %s", r.ProviderName, r.SkippedReason)
			continue
		}
		if r.Err != "" {
			failedProviders++
			logger.Errorf("[agent] ❌ Provider %s 失败: %s", r.ProviderName, r.Err)
			continue
		}
		totalProviders++
		totalTools += r.ToolCount
		logger.Infof("[agent] ✅ Provider %s 装配完成: %d 个工具 (耗时 %v)",
			r.ProviderName, r.ToolCount, r.Duration)
	}
	logger.Infof("[agent] 🎯 工具链装配总结: providers=%d tools=%d failed=%d",
		totalProviders, totalTools, failedProviders)

	InitGlobalToolRouter()
}

