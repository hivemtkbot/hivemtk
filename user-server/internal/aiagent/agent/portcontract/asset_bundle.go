package portcontract

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
)

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
	AssetID      string
	UserQuery    string
	RAGDocs      []RAGDocumentPort
	ChatHistory  []model.AssetBundleMessage
	MerchantVars map[string]string
	Options      WeaveOptionsPort
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
	RAGPosition         string
	MaxHistoryMessages  int
	StripFewShotJSON    bool
	IncludeMerchantVars bool
}

// AssetBundleWeavePort 资产包 Weave 织布端口
//
// 实现方：service.AssetBundleWeavePortAdapter（见 service/asset_bundle_port_adapter.go）
// 消费方：agent_runtime/asset_bundle_loader.go（实现 AssetLoader 接口）
type AssetBundleWeavePort interface {
	WeaveForRequest(ctx context.Context, in WeaveRequestPort) ([]model.AssetBundleMessage, error)

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
	FetchHistory(ctx context.Context, sessionID string, limit int) ([]model.AssetBundleMessage, error)

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
	Chat(ctx context.Context, messages []model.AssetBundleMessage, model string, temperature float64) (string, error)
}
