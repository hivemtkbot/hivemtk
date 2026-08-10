package service

import (
	"context"
	"log"
	"sync"

	"hivemtk-user/internal/dto"
)

// sales_engine_agent_asset_bundle.go 资产包解析器（渠道→智能体→资产包 三层接线）
//
// 五层架构归属: L2 业务编排层
// 设计依据: docs/ASSET_BUNDLE_CLOSED_LOOP.md / docs/企业级架构优化/对话驱动自我学习机制.md
//
// 职责：
//   - 定义 AssetBundleResolver 接口：将 asset_bundle_id 解析为 system prompt
//   - 提供 resolveAssetBundlePersona 工具函数：SalesEngine 执行前调用，解析失败时安全降级
//   - 提供全局解析器注入点 SetAssetBundleResolver（由 main.go 装配具体实现）
//
// 解析失败的安全降级策略：
//   1. resolver == nil → 返回空字符串（沿用 agentCtx.Persona 原人设）
//   2. assetBundleID == "" → 返回空字符串（未绑定资产包，沿用原人设）
//   3. resolver.ResolveSystemPrompt 返回 error → 记录日志，返回空字符串（降级沿用原人设）
//
// 这样保证资产包解析链路的任何故障都不会阻断主对话流程。

// AssetBundleResolver 资产包解析器接口
//
// 实现方：由 service 层提供具体实现（如 AssetBundleService.ResolveSystemPrompt）
// 注入点：main.go 装配时调用 SetAssetBundleResolver
type AssetBundleResolver interface {
	// ResolveSystemPrompt 根据 assetBundleID 解析资产包的 system prompt
	// 返回 error 时调用方应安全降级（沿用原 Persona）
	ResolveSystemPrompt(ctx context.Context, assetBundleID string) (string, error)
}

// 全局解析器单例（由 main.go 装配时注入）
var (
	assetBundleResolverForSalesEngine AssetBundleResolver
	assetBundleResolverMu             sync.RWMutex
)

// SetAssetBundleResolver 注入全局资产包解析器
// 调用方：main.go 在装配 AssetBundleService 后调用
// 线程安全：使用 RWMutex 保护，支持运行期热更新
func SetAssetBundleResolver(r AssetBundleResolver) {
	assetBundleResolverMu.Lock()
	defer assetBundleResolverMu.Unlock()
	assetBundleResolverForSalesEngine = r
}

// GetAssetBundleResolver 获取全局资产包解析器（未注入返回 nil）
func GetAssetBundleResolver() AssetBundleResolver {
	assetBundleResolverMu.RLock()
	defer assetBundleResolverMu.RUnlock()
	return assetBundleResolverForSalesEngine
}

// resolveAssetBundlePersona 解析资产包人设话术
//
// 调用时机：SalesEngine.HandleWithAgent 在覆盖 req.Config 之前
// 行为：
//   - agentCtx == nil → 返回空
//   - assetBundleID == "" → 返回空（未绑定资产包）
//   - resolver == nil → 返回空（未注入解析器，安全降级）
//   - resolver.ResolveSystemPrompt 失败 → 记录日志，返回空（降级沿用原人设）
//   - 成功 → 返回资产包 system prompt
//
// 调用方应根据返回值决定是否覆盖 agentCtx.Persona / SystemPrompt
func resolveAssetBundlePersona(ctx context.Context, agentCtx *dto.AgentContext, resolver AssetBundleResolver) string {
	if agentCtx == nil || agentCtx.AssetBundleID == "" {
		return ""
	}
	if resolver == nil {
		return ""
	}
	prompt, err := resolver.ResolveSystemPrompt(ctx, agentCtx.AssetBundleID)
	if err != nil {
		log.Printf("[sales_engine] resolve asset bundle persona failed: asset=%s err=%v (fallback to original persona)",
			agentCtx.AssetBundleID, err)
		return ""
	}
	return prompt
}
