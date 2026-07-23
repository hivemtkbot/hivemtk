package portcontract

import (
	"context"
	"errors"

	"marketing/internal/model"
)

// ----------------------------------------------------------------------------
// 资产包域：Weave 织布 / 知识库检索 / 会话历史
//
// 设计动机（参考 customer.go）：
//   - agent_runtime 包需要调用 service.AssetBundleService.WeaveForRequest，
//     但 service.webhook_service.go 已经反向引用了 agent_runtime，
//     直接 import 会导致 service ↔ agent_runtime 循环。
//   - 通过本包抽出 Port 接口，agent_runtime 仅依赖 portcontract；
//     service 包的 adapter（asset_bundle_port_adapter.go）实现接口并通过 setter 注入。
// ----------------------------------------------------------------------------
//
// 文档依据：docs/企业级架构优化/资产包模式.md §二 Weave 织布算法 / §三 运行态 Request Payload

// ErrAssetBundleNotEnabled 资产包未热启用（sentinel）
//
// 工具层（agent_runtime）依赖此 sentinel 区分"未启用"与其他错误，
// 不再 import service 包。
var ErrAssetBundleNotEnabled = errors.New("asset bundle is not hot-enabled")

// ErrAssetBundleNotFound 资产包不存在（sentinel）
var ErrAssetBundleNotFound = errors.New("asset bundle not found")

// WeaveRequestPort Weave 织布请求入参（agent_runtime → service 投影）
//
// 字段含义参考 service.WeaveInput，但去掉了 Asset *model.AssetBundle 字段
// （由 service 层根据 AssetID 自行加载，避免 agent_runtime 触发 DB 查询）
type WeaveRequestPort struct {
	AssetID      string                          // 资产包业务键
	UserQuery    string                          // 当前用户最新消息
	RAGDocs      []RAGDocumentPort               // 商户本地 RAG 检索结果
	ChatHistory  []model.AssetBundleMessage      // 活跃会话历史
	MerchantVars map[string]string               // 商户参数（shop_name / campaign_name / discount_pct / support_contact）
	Options      WeaveOptionsPort                // 织布策略
}

// RAGDocumentPort RAG 检索结果投影
type RAGDocumentPort struct {
	ID      string
	Title   string
	Content string
	Score   float64
	Source  string
}

// WeaveOptionsPort 织布策略投影
type WeaveOptionsPort struct {
	RAGPosition         string // after_system / after_fewshots
	MaxHistoryMessages  int
	StripFewShotJSON    bool
	IncludeMerchantVars bool
}

// AssetBundleWeavePort 资产包 Weave 织布端口
//
// 实现方：service.AssetBundleWeavePortAdapter（见 service/asset_bundle_port_adapter.go）
// 消费方：agent_runtime/asset_bundle_loader.go（实现 AssetLoader 接口）
type AssetBundleWeavePort interface {
	// WeaveForRequest 业务化 Weave：加载资产包 + 织布
	//
	// 入参：WeaveRequestPort
	// 出参：拼装好的 OpenAI 兼容 messages 数组
	WeaveForRequest(ctx context.Context, in WeaveRequestPort) ([]model.AssetBundleMessage, error)

	// IsBundleEnabled 判断某资产包是否已热启用（运行期缓存查询）
	IsBundleEnabled(assetID string) bool
}

// ChatHistoryPort 活跃会话历史端口
//
// 实现方：service.ChatHistoryPortAdapter（见 service/chat_history_port_adapter.go）
// 消费方：agent_runtime/asset_bundle_loader.go（实现 AssetLoader 接口）
//
// 数据源：Redis LPush 列表（key 格式：chat_history:{session_id}）
// 设计：每次客户/坐席发送消息时由 publisher 写入；loader 读取时按时间正序返回
type ChatHistoryPort interface {
	// FetchHistory 拉取指定会话最近 N 条历史消息（按时间正序返回）
	//
	// 参数：
	//   - sessionID: 会话唯一 ID
	//   - limit: 最大返回条数（0 表示使用实现默认值，通常 10）
	FetchHistory(ctx context.Context, sessionID string, limit int) ([]model.AssetBundleMessage, error)

	// AppendHistory 追加一条历史消息（客户或坐席发送后调用）
	AppendHistory(ctx context.Context, sessionID string, msg model.AssetBundleMessage) error
}

// KnowledgeSearchPort 知识库检索端口
//
// 实现方：service.KnowledgeBaseService（直接实现，方法签名匹配）
// 消费方：agent_runtime/asset_bundle_loader.go
//
// 设计：把 KnowledgeBaseService.Search 的签名抽象成端口，
// 便于 agent_runtime 不直接 import service 包
type KnowledgeSearchPort interface {
	// Search 在已索引的知识库中检索与 query 最相关的 topK 个文档片段
	Search(ctx context.Context, query string, topK int) ([]RAGDocumentPort, error)
}

// LLMChatPort 本地 LLM 聊天端口（OpenAI /v1/chat/completions 兼容）
//
// 实现方：agent_runtime.LocalLLMClient（同包实现，不走 service 适配器）
// 消费方：agent_runtime.InferenceCycle / PlannerStage
//
// 设计：放在 portcontract 包是为了让 service.PlaygroundService 也能复用同接口，
// 但实际实现由 agent_runtime 包提供
type LLMChatPort interface {
	// Chat 发送一次 chat completions 请求
	//
	// 参数：
	//   - messages: OpenAI 兼容 messages 数组
	//   - model: 模型名（如 qwen2.5:7b / llama3.1:8b）
	//   - temperature: 温度（0.0-1.0）
	Chat(ctx context.Context, messages []model.AssetBundleMessage, model string, temperature float64) (string, error)
}
