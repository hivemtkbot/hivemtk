// Package agent_runtime 的子包：单次推理闭环 (Inference Cycle)
//
// 设计依据：docs/企业级架构优化/认知决策大脑层.md
//
// 核心目标：把 AgentRuntime 的一次"客户消息 → AI 回复"完整推理过程
// 拆成 6 个有序阶段 (Stage)，每个阶段独立可测、可替换、可观测。
//
// 阶段流（严格按文档）：
//
//	[1] Perception (感知)         - 情绪 + 意图识别
//	[2] Alignment  (6维拟人度打分)  - 同理心/热情/专业度等
//	[3] Gatekeeper (危机感门禁)    - 判定是否触发转人工
//	[4a] Escalation (转人工门禁)   - 当危机 → 锁会话 + 通知坐席
//	[4b] Planner    (任务规划器)   - 当正常 → 决定调什么工具/写什么回复
//	[5] Action     (执行)         - LLM 调用 + 工具执行
//
// 阶段之间通过 InferenceContext (不可变快照) 传递数据，
// 编排器 (InferenceCycle) 负责串联、超时控制、错误隔离、可观测日志。
//
// 五层架构归位：本文件属于 aiagent/agent/runtime 子包，遵循 ADR-008
// 的运行时隔离原则，不直接访问 db.GetDB()，通过 Repository / Bridge 访问。
package agent_runtime

import (
	"time"
)

// ============================================================================
// 1. Sentiment 情绪标签
// ============================================================================

// Sentiment 情绪标签
//
// 来源：方向6 情绪感知模块；本文件先定义枚举，便于测试
type Sentiment string

const (
	SentimentCalm     Sentiment = "calm"     // 平静
	SentimentAnxious  Sentiment = "anxious"  // 焦虑
	SentimentAngry    Sentiment = "angry"    // 愤怒
	SentimentAppreci Sentiment = "appreci" // 赞赏
	SentimentConfused Sentiment = "confused" // 困惑
	SentimentUnknown  Sentiment = "unknown"
)

// SentimentScore 情绪打分（0-1）
type SentimentScore struct {
	Label  Sentiment `json:"label"`
	Score  float64   `json:"score"`  // 主情绪强度
	Detail map[Sentiment]float64 `json:"detail,omitempty"` // 多情绪细分
}

// ============================================================================
// 2. Intent 意图标签
// ============================================================================

// Intent 意图分类
type Intent string

const (
	IntentChitchat       Intent = "chitchat"        // 闲聊
	IntentInquiry        Intent = "inquiry"         // 询价
	IntentOrderStatus    Intent = "order_status"    // 查单
	IntentComplaint      Intent = "complaint"       // 投诉
	IntentRefund         Intent = "refund"          // 退款
	IntentAfterSales     Intent = "after_sales"     // 售后
	IntentSalesLead      Intent = "sales_lead"      // 销售线索
	IntentGreeting       Intent = "greeting"        // 寒暄
	IntentFarewell       Intent = "farewell"        // 告别
	IntentHandoffToHuman Intent = "handoff_human"   // 强转人工（方向6 文档示例4）
	IntentFAQ            Intent = "faq"             // FAQ 问答（方向6 文档示例1）
	IntentUnknown        Intent = "unknown"
)

// IntentResult 意图识别结果
type IntentResult struct {
	Primary   Intent             `json:"primary"`
	Secondary []Intent           `json:"secondary,omitempty"`
	Score     float64            `json:"score"`     // 主意图置信度
	Tags      map[string]string  `json:"tags,omitempty"` // 实体槽位（如"产品=手机"）
}

// ============================================================================
// 3. AlignmentScore 6维拟人度打分
// ============================================================================

// AlignmentDimension 拟人度维度
type AlignmentDimension string

const (
	DimEmpathy   AlignmentDimension = "empathy"   // 同理心
	DimEnthusiasm AlignmentDimension = "enthusiasm" // 热情度
	DimExpertise AlignmentDimension = "expertise" // 专业度
	DimPatience  AlignmentDimension = "patience"  // 耐心
	DimClarity   AlignmentDimension = "clarity"   // 清晰度
	DimPoliteness AlignmentDimension = "politeness" // 礼貌度
)

// AlignmentScore 6维拟人度评分（每个维度 1-5 分）
type AlignmentScore struct {
	Empathy    int `json:"empathy"`    // 同理心：针对用户焦虑/愤怒指数
	Enthusiasm int `json:"enthusiasm"` // 热情度：针对询价/赞赏指数
	Expertise  int `json:"expertise"`  // 专业度：知识库检索置信度
	Patience   int `json:"patience"`   // 耐心：会话轮次
	Clarity    int `json:"clarity"`    // 清晰度：表述简洁
	Politeness int `json:"politeness"` // 礼貌度：敬语使用
}

// Total 综合分（加权平均）
func (a AlignmentScore) Total() float64 {
	sum := a.Empathy + a.Enthusiasm + a.Expertise + a.Patience + a.Clarity + a.Politeness
	return float64(sum) / 6.0
}

// MaxDimension 返回最需要提升的维度（分最低）
func (a AlignmentScore) MaxDimension() AlignmentDimension {
	minDim := DimEmpathy
	minScore := a.Empathy
	if a.Enthusiasm < minScore {
		minDim = DimEnthusiasm
		minScore = a.Enthusiasm
	}
	if a.Expertise < minScore {
		minDim = DimExpertise
		minScore = a.Expertise
	}
	if a.Patience < minScore {
		minDim = DimPatience
		minScore = a.Patience
	}
	if a.Clarity < minScore {
		minDim = DimClarity
		minScore = a.Clarity
	}
	if a.Politeness < minScore {
		minDim = DimPoliteness
	}
	return minDim
}

// ============================================================================
// 4. CrisisLevel 危机感等级
// ============================================================================

// CrisisLevel 危机感等级
type CrisisLevel int

const (
	// CrisisNone 无危机 (0)
	CrisisNone CrisisLevel = 0
	// CrisisLow 低 (1-2)：保持观察
	CrisisLow CrisisLevel = 1
	// CrisisMedium 中 (3)：人工关注
	CrisisMedium CrisisLevel = 2
	// CrisisHigh 高 (4-5)：强制转人工
	CrisisHigh CrisisLevel = 3
)

// CrisisSignal 危机信号
type CrisisSignal struct {
	Level     CrisisLevel `json:"level"`
	Triggers  []string    `json:"triggers"`  // 触发词列表（"退款/骗子/起诉/曝光"）
	Reason    string      `json:"reason"`
	DetectedAt time.Time  `json:"detected_at"`
}

// NeedsEscalation 是否需要转人工
func (c CrisisSignal) NeedsEscalation() bool {
	return c.Level >= CrisisHigh
}

// ============================================================================
// 5. ActionPlan 行动方案
// ============================================================================

// ActionPlan 任务规划器输出
type ActionPlan struct {
	// PlanType 方案类型
	PlanType string `json:"plan_type"`
	// ToolCalls 计划调用的工具
	ToolCalls []PlannedToolCall `json:"tool_calls,omitempty"`
	// ReplyHint 回复方向（用于 LLM prompt 引导）
	ReplyHint string `json:"reply_hint,omitempty"`
	// SkipLLM 是否跳过 LLM 直接回复（如标准化 FAQ）
	SkipLLM bool `json:"skip_llm"`
	// SkipReason 跳过 LLM 的原因
	SkipReason string `json:"skip_reason,omitempty"`
	// Confidence 规划置信度
	Confidence float64 `json:"confidence"`
}

// PlannedToolCall 计划工具调用
type PlannedToolCall struct {
	ToolName string         `json:"tool_name"`
	Args     map[string]any `json:"args"`
	Priority int            `json:"priority"`
}

// ============================================================================
// 6. StageDecision 阶段决策记录
// ============================================================================

// StageDecision 阶段决策记录（用于可观测与可调试）
type StageDecision struct {
	Stage     string        `json:"stage"`
	Action    string        `json:"action"`
	Reason    string        `json:"reason,omitempty"`
	Duration  time.Duration `json:"duration_ns"`
	Success   bool          `json:"success"`
	Error     string        `json:"error,omitempty"`
}

// ============================================================================
// 7. InferenceContext 推理上下文（在阶段间传递的不可变快照）
// ============================================================================

// InferenceContext 推理上下文
//
// 编排器在每阶段调用时传递的"数据袋"，所有阶段都从这里读输入 + 写输出。
// 严格不可变（写时复制），便于并发快照与可观测回放。
type InferenceContext struct {
	// 入参
	Payload   CustomerMessagePayload `json:"payload"`
	AgentCtx  *AgentContext          `json:"agent_ctx"`

	// Stage 1 输出
	Sentiment SentimentScore `json:"sentiment,omitempty"`
	Intent    IntentResult   `json:"intent,omitempty"`

	// Stage 2 输出
	Alignment AlignmentScore `json:"alignment,omitempty"`

	// Stage 3 输出
	Crisis CrisisSignal `json:"crisis,omitempty"`

	// Stage 4b 输出
	Plan *ActionPlan `json:"plan,omitempty"`

	// 控制
	StartTime time.Time         `json:"start_time"`
	Stages    []StageDecision   `json:"stages"`

	// 决策（最终）
	Decision InferenceDecision `json:"decision"`
}

// InferenceDecision 最终决策
type InferenceDecision struct {
	// HandoffToHuman 是否转人工
	HandoffToHuman bool `json:"handoff_to_human"`
	// HandoffReason 转人工原因
	HandoffReason string `json:"handoff_reason,omitempty"`
	// Plan 行动方案（决定后保留）
	Plan *ActionPlan `json:"plan,omitempty"`
	// Reply 草稿回复（仅最终回复，不含中间过程）
	Reply string `json:"reply,omitempty"`
	// ReplyType 回复类型
	ReplyType string `json:"reply_type,omitempty"`
	// Confidence 综合置信度
	Confidence float64 `json:"confidence"`
	// StopReason 推理终止原因
	StopReason string `json:"stop_reason,omitempty"`
	// TotalDuration 总耗时
	TotalDuration time.Duration `json:"total_duration_ns"`
	// Crisis 危机感信号快照（方向6：决策后保留便于审计/观测）
	Crisis CrisisSignal `json:"crisis,omitempty"`
	// Sentiment 情绪快照（方向6）
	Sentiment SentimentScore `json:"sentiment,omitempty"`
	// Intent 意图快照（方向6）
	Intent IntentResult `json:"intent,omitempty"`
	// Alignment 6维拟人度快照（方向6）
	Alignment AlignmentScore `json:"alignment,omitempty"`
}

// ============================================================================
// 8. Stage 接口返回值
// ============================================================================

// StageResult 阶段执行结果
type StageResult struct {
	// Continue 是否继续下一阶段（false 时编排器立即终止）
	Continue bool
	// EarlyReturn 是否立即返回（跳过剩余阶段）
	EarlyReturn bool
	// Decision 若 EarlyReturn=true，编排器直接采用此决策
	Decision *InferenceDecision
	// Error 阶段错误（不阻塞继续，但记录到 stages）
	Error error
}

// ContinueResult 简单的"继续"结果
func ContinueResult() StageResult {
	return StageResult{Continue: true}
}

// StopResult 立即终止（成功）
func StopResult(decision *InferenceDecision) StageResult {
	return StageResult{
		Continue:    false,
		EarlyReturn: true,
		Decision:    decision,
	}
}

// FailResult 立即终止（失败）
func FailResult(err error) StageResult {
	return StageResult{
		Continue:    false,
		EarlyReturn: true,
		Error:       err,
	}
}
