package event

import "time"

// 主题定义
// 命名规范：{模块}.{动作}，如 operation.log、health.report
const (
	// TopicOperationLog 操作日志主题
	// 发布者：TeamUserService.logOperation
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
	WorkspaceID uint   `json:"workspace_id"` // 知识库工作区 ID
	DocumentID  uint   `json:"document_id"`  // 文档 ID
	ChangeType  string `json:"change_type"`  // create / update / delete
	ContentHash string `json:"content_hash"` // 用于增量检测
	OperatorID  uint   `json:"operator_id"`  // 操作人 ID
	TraceID     string `json:"trace_id"`
}
