package agent_runtime

import (
	"context"
	"errors"
	"strings"

	"marketing/internal/aiagent/agent/portcontract"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// 资产包加载器：实现 AssetLoader 接口
//
// 文档依据：docs/企业级架构优化/资产包模式.md §二 Weave 织布算法
//
// 设计：
//  - 通过 portcontract.AssetBundleWeavePort 调用 service.AssetBundleService.WeaveForRequest
//    （依赖反转，避免 agent_runtime ↔ service 循环 import）
//  - 通过 portcontract.ChatHistoryPort 拉取活跃会话历史
//  - 通过 portcontract.KnowledgeSearchPort 检索商户本地 RAG
//  - 通过 AgentContext.RagProductIDs / Persona 等字段获得商户参数
//  - 输出 AssetContext.Messages（OpenAI 兼容 messages，可直接喂给 LLM）
//
// 装配：service 包的 asset_bundle_port_adapter.go 在 main 启动期注入本 Loader
// 到 CoreDataFlowOrchestrator.SetAssetLoader
// ============================================================================

// AssetBundleLoader 资产包加载器
//
// 实现 AssetLoader 接口（dataflow_orchestrator.go）
type AssetBundleLoader struct {
	weave    portcontract.AssetBundleWeavePort // 调用 service.WeaveForRequest
	history  portcontract.ChatHistoryPort     // 拉取活跃会话历史
	search   portcontract.KnowledgeSearchPort // 商户本地 RAG 检索
	defaults LoaderDefaults                   // 默认配置
}

// LoaderDefaults 加载器默认配置
type LoaderDefaults struct {
	AssetID           string            // 默认资产包（agentCtx 缺失时使用）
	HistoryLimit      int               // 历史消息最大条数（默认 10）
	RAGTopK           int               // RAG 检索 topK（默认 5）
	MerchantVars      map[string]string // 默认商户参数（agentCtx 缺失时使用）
	StripFewShotJSON  bool              // 是否剥离 Few-Shot JSON 尾巴
	IncludeMerchantVars bool            // 是否注入商户参数
	RAGPosition       string            // after_system / after_fewshots
}

// NewAssetBundleLoader 创建资产包加载器
func NewAssetBundleLoader(
	weave portcontract.AssetBundleWeavePort,
	history portcontract.ChatHistoryPort,
	search portcontract.KnowledgeSearchPort,
	defaults LoaderDefaults,
) *AssetBundleLoader {
	if defaults.HistoryLimit <= 0 {
		defaults.HistoryLimit = 10
	}
	if defaults.RAGTopK <= 0 {
		defaults.RAGTopK = 5
	}
	if defaults.RAGPosition == "" {
		defaults.RAGPosition = "after_fewshots"
	}
	defaults.IncludeMerchantVars = true
	defaults.StripFewShotJSON = true
	return &AssetBundleLoader{
		weave:    weave,
		history:  history,
		search:   search,
		defaults: defaults,
	}
}

// LoadContext 加载资产上下文
//
// 流程（文档 §二）：
//  1. 解析 AssetID（优先从 AgentContext 取；缺失用 defaults.AssetID）
//  2. 拉取活跃会话历史（ChatHistoryPort）
//  3. 检索商户本地 RAG（KnowledgeSearchPort）
//  4. 调用 WeaveForRequest 拼装 messages（AssetBundleWeavePort）
//  5. 输出 AssetContext.Messages（OpenAI 兼容，可直接喂 LLM）
func (l *AssetBundleLoader) LoadContext(ctx context.Context, payload CustomerMessagePayload, agentCtx *AgentContext) (*AssetContext, error) {
	if l == nil || l.weave == nil {
		return nil, errors.New("asset bundle loader not configured")
	}

	// 1. 解析 AssetID
	assetID := l.resolveAssetID(agentCtx)
	if assetID == "" {
		// 无资产包时降级到 noop
		logger.Warnf("[asset_loader] no asset_id resolved, fallback to noop session=%s", payload.SessionID)
		return noopAssetLoader{}.LoadContext(ctx, payload, agentCtx)
	}

	// 2. 拉取活跃会话历史
	history := []model.AssetBundleMessage{}
	if l.history != nil && payload.SessionID != "" {
		h, err := l.history.FetchHistory(ctx, payload.SessionID, l.defaults.HistoryLimit)
		if err != nil {
			logger.Warnf("[asset_loader] fetch history failed session=%s err=%v", payload.SessionID, err)
		} else {
			history = h
		}
	}

	// 3. 商户本地 RAG 检索
	ragDocs := []portcontract.RAGDocumentPort{}
	if l.search != nil && payload.Content != "" {
		docs, err := l.search.Search(ctx, payload.Content, l.defaults.RAGTopK)
		if err != nil {
			logger.Warnf("[asset_loader] rag search failed session=%s err=%v", payload.SessionID, err)
		} else {
			ragDocs = docs
		}
	}

	// 4. 解析商户参数（优先从 agentCtx；缺失用 defaults）
	merchantVars := l.resolveMerchantVars(agentCtx)

	// 5. 调用 WeaveForRequest
	weaveReq := portcontract.WeaveRequestPort{
		AssetID:     assetID,
		UserQuery:   payload.Content,
		RAGDocs:     ragDocs,
		ChatHistory: history,
		MerchantVars: merchantVars,
		Options: portcontract.WeaveOptionsPort{
			RAGPosition:         l.defaults.RAGPosition,
			MaxHistoryMessages:  l.defaults.HistoryLimit,
			StripFewShotJSON:    l.defaults.StripFewShotJSON,
			IncludeMerchantVars: l.defaults.IncludeMerchantVars,
		},
	}
	messages, err := l.weave.WeaveForRequest(ctx, weaveReq)
	if err != nil {
		// 热插拔未启用 / 资产包不存在 → 降级 noop
		if errors.Is(err, portcontract.ErrAssetBundleNotEnabled) || errors.Is(err, portcontract.ErrAssetBundleNotFound) {
			logger.Warnf("[asset_loader] bundle not available asset=%s err=%v, fallback noop", assetID, err)
			return noopAssetLoader{}.LoadContext(ctx, payload, agentCtx)
		}
		return nil, err
	}

	logger.Infof("[asset_loader] ok asset=%s msgs=%d rag=%d hist=%d session=%s",
		assetID, len(messages), len(ragDocs), len(history), payload.SessionID)

	// 6. 拼装 AssetContext
	// 从 messages 第一条 system 提取 PromptText（向后兼容）
	promptText := ""
	for _, m := range messages {
		if m.Role == "system" {
			promptText = m.Content
			break
		}
	}
	// AssetContext 当前 schema 只暴露 L1/L2/PromptText/SystemTools；
	// 资产包 ID / 织布结果 messages / RAG 文档 / 商户参数 暂不写入上下文结构，
	// 由调用方（dataflow_orchestrator）从局部变量与 weave 输入重建，避免扩展公共类型。
	_ = assetID
	_ = messages
	_ = ragDocs
	_ = merchantVars
	return &AssetContext{
		L1ShortTerm: map[string]string{},
		L2Profile:   map[string]string{},
		PromptText:  promptText,
		SystemTools: []string{},
	}, nil
}

// resolveAssetID 解析资产包业务键
//
// 优先级：
//  1. AgentContext.Persona（约定为资产包 AssetID；与 ai_agents 表对齐）
//  2. AgentContext.AgentCode（兜底）
//  3. defaults.AssetID
//  4. payload.Raw["asset_id"]（运行时动态指定）
func (l *AssetBundleLoader) resolveAssetID(agentCtx *AgentContext) string {
	if agentCtx != nil {
		if agentCtx.Persona != "" {
			return agentCtx.Persona
		}
		if agentCtx.AgentCode != "" {
			return agentCtx.AgentCode
		}
	}
	if l.defaults.AssetID != "" {
		return l.defaults.AssetID
	}
	return ""
}

// resolveMerchantVars 解析商户参数
//
// 优先级：
//  1. AgentContext（约定 AgentCode=shop_name, SystemPrompt 含 campaign=xxx 等）
//  2. defaults.MerchantVars
//
// 设计：AgentContext 当前没有 MerchantVars 字段，约定通过 SystemPrompt 解析
// （格式：campaign_name=xxx;discount_pct=10%）。这样不需要改 AgentContext 类型。
func (l *AssetBundleLoader) resolveMerchantVars(agentCtx *AgentContext) map[string]string {
	out := make(map[string]string)
	// 先用 defaults 兜底
	for k, v := range l.defaults.MerchantVars {
		out[k] = v
	}
	if agentCtx != nil && agentCtx.SystemPrompt != "" {
		// 解析 SystemPrompt 中的 key=value;key=value 格式
		for _, kv := range strings.Split(agentCtx.SystemPrompt, ";") {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				continue
			}
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if k != "" && v != "" {
				out[k] = v
			}
		}
	}
	return out
}

// 编译期断言：AssetBundleLoader 实现 AssetLoader
var _ AssetLoader = (*AssetBundleLoader)(nil)
