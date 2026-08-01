package event

import "time"

// 主题定义
// 命名规范：{模块}.{动作}，如 operation.log、health.report
const (
	// TopicOperationLog 操作日志主题
	// 发布者：业务 Service（如 SystemUserService、ContentService 等）
	// 订阅者：OperationLogSubscriber（写入 operation_logs 表）
	TopicOperationLog = "operation.log"

	// TopicHealthReport 账号健康度上报主题（预留，待后续落地）
	TopicHealthReport = "health.report"

	// TopicCustomerMerged 客户合并完成通知主题（预留，待 CustomerMerger 落地）
	TopicCustomerMerged = "customer.merged"

	// TopicCustomerMessageReceived 客户消息接收主题 — 2026-07-17 新增(ADR-008 §2.2)
	// 发布者:Channel Adapter(webhook 处理完成后)
	// 订阅者:agent_runtime.EventSubscriber
	// 用途:解耦渠道层与 AI 引擎,新增渠道无需修改 AI 代码
	TopicCustomerMessageReceived = "customer.message.received"

	// TopicKnowledgeDocumentChanged 知识库文档变更主题 — 2026-07-17 新增(ADR-008 §2.5 子项 2)
	// 发布者:KnowledgeDocumentService CRUD 后
	// 订阅者:rag.IncrementalIndexer
	// 用途:文档增/删/改时触发增量索引更新
	TopicKnowledgeDocumentChanged = "knowledge.document.changed"

	// ========================================================================
	// 自我学习机制事件主题（v1.1 §2.4）
	// 发布者:DialogueEventPublisher / RAGSelfCorrector / AssetBundleLearner
	// 订阅者:SelfLearningOrchestrator
	// ========================================================================

	// TopicDialogueStarted 对话开始事件主题
	// 发布者:DialogueEventPublisher.PublishStarted（OpenSession / 首条访客消息）
	// 订阅者:Orchestrator.onDialogueStarted → RAGSelfCorrector.Warmup
	TopicDialogueStarted = "dialogue.started"

	// TopicDialogueEnded 对话结束事件主题
	// 发布者:DialogueEventPublisher.PublishEnded（UpdateSessionStatus=closed/resolved）
	// 订阅者:Orchestrator.onDialogueEnded → RAG 反思 / 资产包候选生成 / 5 维监督指标采集
	TopicDialogueEnded = "dialogue.ended"

	// TopicAssetDegraded 资产包降级事件主题
	// 发布者:AssetBundleLearner.DegradeInactiveAssets（连续 30 天 use_count=0）
	// 订阅者:Orchestrator.onAssetDegraded（记录日志，候选生成由 cron 触发）
	TopicAssetDegraded = "asset.degraded"

	// TopicAssetDegradeWarning 资产包降级预警事件主题（autonomous 模式下 24h 前预警）
	// 发布者:AssetBundleLearner（降级前 24h 调用 PublishAssetDegradeWarning）
	// 订阅者:看板告警订阅
	TopicAssetDegradeWarning = "asset.degrade.warning"

	// TopicRagCorpusUpdated RAG 语料变更事件主题
	// 发布者:RAGSelfCorrector（销冠补录 / 低质归档 / 降权）
	// 订阅者:RAG 缓存失效订阅
	TopicRagCorpusUpdated = "rag.corpus.updated"
)

// OperationLogPayload 操作日志事件载荷
//
// 对应 model.OperationLog 字段，由 OperationLogSubscriber 转换并写入 DB
type OperationLogPayload struct {
	UserID     uint   `json:"user_id"`
	Username   string `json:"username"`
	Action     string `json:"action"`   // create, update, delete, login, change_password, reset_password
	Module     string `json:"module"`   // user, role, card, shortlink 等
	Resource   string `json:"resource"` // 资源类型（默认等于 Module）
	ResourceID string `json:"resource_id"`
	OldValue   any    `json:"old_value"` // 旧值（可序列化为 JSON）
	NewValue   any    `json:"new_value"` // 新值（可序列化为 JSON）
	IP         string `json:"ip"`
}

// CustomerMessagePayload 客户消息事件载荷 — 2026-07-17 新增(ADR-008 §2.2)
//
// 由 Channel Adapter 在 webhook 处理完成后 publish
// agent_runtime.EventSubscriber.Handle 消费
//
// 关联主题:TopicCustomerMessageReceived
type CustomerMessagePayload struct {
	ChannelType string         `json:"channel_type"` // telegram / wecom / feishu / douyin / ...
	AccountID   string         `json:"account_id"`   // 渠道账号主键
	CustomerID  string         `json:"customer_id"`  // 客户 OneID（已归一化）
	SessionID   string         `json:"session_id"`   // 会话唯一 ID（方向8 核心数据流必备；缺省由 channel:customer 构造）
	Content     string         `json:"content"`      // 消息内容
	MessageType string         `json:"message_type"` // text / image / voice / event
	Timestamp   time.Time      `json:"timestamp"`    // 消息时间戳
	TraceID     string         `json:"trace_id"`     // 全链路追踪 ID
	Raw         map[string]any `json:"raw,omitempty"`
}

// KnowledgeDocumentChangePayload 知识库文档变更事件载荷 — 2026-07-17 新增(ADR-008 §2.5 子项 2)
//
// 关联主题:TopicKnowledgeDocumentChanged
// 触发时机:KnowledgeDocumentService.Create/Update/Delete 后 publish
type KnowledgeDocumentChangePayload struct {
	WorkspaceID string   `json:"workspace_id"` // 知识库工作区 ID
	DocumentID  uint   `json:"document_id"`  // 文档 ID
	ChangeType  string `json:"change_type"`  // create / update / delete
	ContentHash string `json:"content_hash"` // 用于增量检测
	OperatorID  uint   `json:"operator_id"`  // 操作人 ID
	TraceID     string `json:"trace_id"`
}

// ============================================================================
// 自我学习机制事件载荷（v1.1 §2.4）
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md
// ============================================================================

// DialogueStartedPayload 对话开始事件载荷
//
// 关联主题:TopicDialogueStarted
// 触发时机:
//   - VisitorChatService.OpenSession 成功后（WS 连接建立）
//   - WebhookService 首条访客消息到达时
//
// 订阅者:SelfLearningOrchestrator.onDialogueStarted → RAGSelfCorrector.Warmup
type DialogueStartedPayload struct {
	SessionID    string    `json:"session_id"`
	VisitorID    string    `json:"visitor_id"`
	CustomerID   string    `json:"customer_id"`
	ChannelType  string    `json:"channel_type"`
	Scenario     string    `json:"scenario"`
	FirstMessage string    `json:"first_message"`
	StartedAt    time.Time `json:"started_at"`
	TraceID      string    `json:"trace_id"`
}

// DialogueEndedPayload 对话结束事件载荷
//
// 关联主题:TopicDialogueEnded
// 触发时机:CustomerSessionService.UpdateSessionStatus(status=closed/resolved) 成功后
//
// 订阅者:
//   - Orchestrator.onDialogueEnded
//   - → RAGSelfCorrector.Reflect（基于 reward 矫正语料质量）
//   - → AssetBundleLearner.GenerateCandidate（基于销冠对话生成资产包候选）
//   - → RAGSelfSupervisor.CollectMetrics / AssetBundleSelfSupervisor.CollectMetrics
type DialogueEndedPayload struct {
	SessionID        string         `json:"session_id"`
	VisitorID        string         `json:"visitor_id"`
	CustomerID       string         `json:"customer_id"`
	DurationSec      int64          `json:"duration_sec"`
	Outcome          string         `json:"outcome"` // converted / resolved / abandoned / ...
	AggregatedReward float64        `json:"aggregated_reward"`
	SignalBreakdown  map[string]any `json:"signal_breakdown,omitempty"`
	UsedCorpusIDs    []string       `json:"used_corpus_ids,omitempty"`
	UsedAssetIDs     []string       `json:"used_asset_ids,omitempty"`
	LastCustomerMsg  string         `json:"last_customer_msg,omitempty"`
	LastAIReply      string         `json:"last_ai_reply,omitempty"`
	TraceID          string         `json:"trace_id"`
	EndedAt          time.Time      `json:"ended_at"`
}

// AssetDegradedPayload 资产包降级事件载荷
//
// 关联主题:TopicAssetDegraded
// 触发时机:AssetBundleLearner.DegradeInactiveAssets 检测到资产包连续 30 天 use_count=0
// 订阅者:Orchestrator.onAssetDegraded（记录日志，候选生成由 cron 触发）
type AssetDegradedPayload struct {
	AssetID      string    `json:"asset_id"`
	AssetTitle   string    `json:"asset_title"`
	Reason       string    `json:"reason"` // stale_or_low_rating / manual / ...
	LastUseCount int64     `json:"last_use_count"`
	LastRating   float64   `json:"last_rating"`
	Scenario     string    `json:"scenario"`
	TraceID      string    `json:"trace_id"`
	DegradedAt   time.Time `json:"degraded_at"`
}

// AssetDegradeWarningPayload 资产包降级预警事件载荷
//
// 关联主题:TopicAssetDegradeWarning
// 触发时机:autonomous 模式下，资产包即将降级前 24h 预警
// 订阅者:看板告警订阅
type AssetDegradeWarningPayload struct {
	AssetID      string    `json:"asset_id"`
	AssetTitle   string    `json:"asset_title"`
	Reason       string    `json:"reason"`
	LastUseCount int64     `json:"last_use_count"`
	LastRating   float64   `json:"last_rating"`
	Scenario     string    `json:"scenario"`
	TraceID      string    `json:"trace_id"`
	WarnedAt     time.Time `json:"warned_at"`
}

// RagCorpusUpdatedPayload RAG 语料变更事件载荷
//
// 关联主题:TopicRagCorpusUpdated
// 触发时机:RAGSelfCorrector 销冠补录 / 低质归档 / 降权
// 订阅者:RAG 缓存失效订阅
type RagCorpusUpdatedPayload struct {
	CorpusID        string    `json:"corpus_id"` // chunk_id 字符串
	Action          string    `json:"action"`    // champion_upsert / low_quality_mark / archive / ...
	SourceSessionID string    `json:"source_session_id,omitempty"`
	NewQualityLabel string    `json:"new_quality_label,omitempty"`
	TraceID         string    `json:"trace_id"`
	UpdatedAt       time.Time `json:"updated_at"`
}
