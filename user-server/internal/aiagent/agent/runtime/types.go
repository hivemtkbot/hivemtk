// Package agent_runtime 提供 AI 智能体运行时隔离层。
//
// 设计依据：docs/architecture/adr/ADR-008-agent-runtime-isolation.md
//
// 核心职责：
//  1. 加载智能体上下文（从 ai_agents 表 + channel_agent_bindings 表）
//  2. 桥接 SalesEngine / SmartCSOrchestrator（只通过 interface）
//  3. 订阅 internal/event/bus.go 的 customer.message.received 主题
//
// 依赖方向（铁律）：
//
//	agent_runtime ──→ tools (interface)        // 业务能力
//	agent_runtime ──→ event/bus                // 事件基础设施
//	agent_runtime ──→ sales_engine (interface) // AI 引擎接口
//	agent_runtime ──→ smart_cs (interface)     // 智能体接口
//
// 禁止：
//   - 直接 import internal/service 下的具体 Service 实现
//   - 直接访问 *gorm.DB
//   - 在业务 Service 内部被同步调用（只通过 Event Bus 触发）
package agent_runtime

import (
	"context"
	"sync"
	"time"
)

// ============================================================================
// 1. AgentContext 智能体上下文
// ============================================================================

// AgentContext 智能体运行时上下文
//
// 从 ai_agents 表加载的完整智能体配置 + 渠道绑定关系 + 缓存
// 这是 agent_runtime 包的"领域模型"，与 model.AIAgent(数据库模型)解耦
type AgentContext struct {
	// 基础信息
	AgentID   uint   // 主键
	AgentCode string // 业务唯一码
	Name      string
	AgentType string // sales / customer_service / hybrid

	// 人设
	Persona      string
	SystemPrompt string
	Greeting     string

	// 知识库挂载
	RagProductIDs []string

	// SOP / 话术库 / 决策策略 / A/B 实验挂载（ADR-008 §2.3）
	SOPIDs              []string
	ScriptLibraryIDs    []string
	DecisionStrategyIDs []string
	ABExperimentIDs     []string

	// LLM 配置
	LLMModel         string
	Temperature      float64
	MaxTokens        int
	TopP             float64
	FrequencyPenalty float64
	PresencePenalty  float64

	// 引擎开关
	EnableRAG            bool
	EnableScriptMatch    bool
	EnableHumanizePolish bool
	EnableContentAudit   bool
	EnablePlaybook       bool
	RAGTopK              int

	// 转人工策略
	ConfidenceThreshold float64
	MaxAIConsecutive    int

	// 元信息
	Version   int
	LoadedAt  time.Time
	Channel   string // 触发加载的渠道
	AccountID string // 触发加载的账号
}

// ============================================================================
// 2. AgentRuntime 智能体运行时接口
// ============================================================================

// AgentRuntime 智能体运行时统一入口
//
// 所有 AI 触发都通过本接口进入，外部不直接持有 SalesEngine / SmartCSOrchestrator
// 这是 agent_runtime 包对外暴露的唯一入口
type AgentRuntime interface {
	// HandleCustomerMessage 处理客户消息事件（来自 Event Bus）
	//
	// 调用方：EventSubscriber.Handle
	// 流程：
	//  1. LoadAgentContext 从 ai_agents + channel_agent_bindings 加载配置
	//  2. 根据 AgentType 路由到 SalesEngine 或 SmartCSOrchestrator
	//  3. 调 Reach Pipeline 发送回复
	HandleCustomerMessage(ctx context.Context, payload CustomerMessagePayload) (*SalesResponse, error)

	// LoadAgentContext 加载智能体上下文
	//
	// 缓存策略：同一 (channel, account) 5 分钟内复用
	// 配置变更时通过 RefreshCache 失效
	LoadAgentContext(ctx context.Context, channelType, accountID string) (*AgentContext, error)

	// RefreshCache 刷新智能体配置缓存
	//
	// 触发时机：ai_agents 表 UPDATE / channel_agent_bindings 变更时
	RefreshCache(ctx context.Context, agentID uint) error

	// Stop 优雅关闭运行时
	Stop(ctx context.Context) error
}

// ============================================================================
// 3. CustomerMessagePayload 客户消息事件载荷
// ============================================================================

// CustomerMessagePayload 客户消息事件载荷
//
// 由 Channel Adapter 在 webhook 处理完成后 publish 到 event bus
// agent_runtime.EventSubscriber.Handle 消费
//
// 关联主题：event.TopicCustomerMessageReceived = "customer.message.received"
type CustomerMessagePayload struct {
	ChannelType string    // telegram / wecom / feishu / douyin / ...
	AccountID   string    // 渠道账号主键
	CustomerID  string    // 客户 OneID（已归一化）
	SessionID   string    // 会话唯一 ID（方向8 核心数据流必备；缺省由 channel:customer 构造）
	Content     string    // 消息内容
	MessageType string    // text / image / voice / event
	Timestamp   time.Time // 消息时间戳
	TraceID     string    // 全链路追踪 ID

	// 渠道原始字段（透传，由 SalesEngine 按需使用）
	Raw map[string]any `json:"raw,omitempty"`
}

// ============================================================================
// 4. SalesResponse 销售响应
// ============================================================================

// SalesResponse 销售智能体响应
type SalesResponse struct {
	// 回复内容
	ReplyContent string
	ReplyType    string // text / card / image / handoff
	Confidence   float64

	// 触发信息
	AgentID    uint
	AgentCode  string
	Channel    string
	CustomerID string
	TraceID    string

	// 工具调用链
	ToolsCalled []string
	LLMModel    string
	TokensUsed  int

	// 状态
	HandoffToHuman bool
	StopReason     string

	// 时延
	Duration time.Duration
}

// ============================================================================
// 5. EventSubscriber 事件订阅者接口
// ============================================================================

// EventSubscriber 事件订阅者
//
// 实现 internal/event.Handler 接口
// 订阅 customer.message.received 主题
type EventSubscriber interface {
	// Handle 处理事件总线消息
	Handle(evt Event) error
}

// Event 内部事件类型（来自 internal/event）
//
// 这里重新声明类型签名，避免循环依赖
// 实际实现时由 internal/event 包的 Event 类型转换而来
type Event struct {
	Topic     string
	Payload   any
	Timestamp time.Time
	Source    string
}

// ============================================================================
// 6. cacheKey 缓存键
// ============================================================================

// cacheKey 缓存键
type cacheKey struct {
	Channel   string
	AccountID string
}

// String 序列化
func (k cacheKey) String() string {
	return k.Channel + ":" + k.AccountID
}

// ============================================================================
// 7. defaultAgentRuntime 默认实现（骨架阶段）
// ============================================================================

// defaultAgentRuntime 默认运行时实现
//
// 骨架阶段：仅实现 AgentContext 缓存 + 接口签名
// 完整实现在后续任务中补充
type defaultAgentRuntime struct {
	mu         sync.RWMutex
	cache      map[cacheKey]*cachedContext
	cacheTTL   time.Duration
	loader     AgentContextLoader
	salesSales SalesEngineBridge
	csBridge   SmartCSBridge
	stopped    bool
}

type cachedContext struct {
	ctx      *AgentContext
	cachedAt time.Time
}

// NewAgentRuntime 创建智能体运行时实例
func NewAgentRuntime(loader AgentContextLoader, sales SalesEngineBridge, cs SmartCSBridge) AgentRuntime {
	rt := &defaultAgentRuntime{
		cache:      make(map[cacheKey]*cachedContext),
		cacheTTL:   5 * time.Minute,
		loader:     loader,
		salesSales: sales,
		csBridge:   cs,
	}
	return rt
}

// ============================================================================
// 8. AgentContextLoader 上下文加载器接口
// ============================================================================

// AgentContextLoader 智能体上下文加载器
//
// 负责从 ai_agents + channel_agent_bindings 表加载配置
// agent_runtime 通过本接口与持久层解耦
type AgentContextLoader interface {
	// LoadByChannelAccount 按渠道+账号加载智能体上下文
	LoadByChannelAccount(ctx context.Context, channelType, accountID string) (*AgentContext, error)

	// Invalidate 失效指定智能体缓存
	Invalidate(ctx context.Context, agentID uint) error
}

// ============================================================================
// 9. SalesEngineBridge 销售引擎桥接器
// ============================================================================

// SalesEngineBridge 销售引擎桥接器
//
// agent_runtime 通过本接口调用 SalesEngine，不直接持有具体类型
// 这是解耦的关键：agent_runtime 不 import internal/service/sales_engine.go
type SalesEngineBridge interface {
	// HandleWithAgent 调销售引擎
	HandleWithAgent(ctx context.Context, agentCtx *AgentContext, req *SalesRequest) (*SalesResponse, error)
}

// SalesRequest 销售请求（agent_runtime → SalesEngine 桥接）
type SalesRequest struct {
	Channel    string         // 渠道
	AccountID  string         // 账号
	CustomerID string         // 客户 OneID
	Content    string         // 用户消息
	TraceID    string         // 追踪 ID
	Raw        map[string]any // 原始字段
}

// ============================================================================
// 11. 错误定义（桥接器使用）
// ============================================================================

// ErrBridgeNotInitialized 桥接器未初始化
var ErrBridgeNotInitialized = &RuntimeError{Code: "bridge_nil", Message: "engine bridge not initialized"}

// ============================================================================
// 10. SmartCSBridge 智能体桥接器
// ============================================================================

// SmartCSBridge 智能体桥接器
type SmartCSBridge interface {
	// HandleIncomingWithAgent 调智能体
	HandleIncomingWithAgent(ctx context.Context, agentCtx *AgentContext, req *SalesRequest) (*SalesResponse, error)
}
