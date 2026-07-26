package agent_runtime

import (
	"context"
	"time"
)

// ============================================================================
// Inference Stage 阶段接口
// ============================================================================
//
// 每个阶段（Perception / Alignment / Gatekeeper / Planner）实现本接口。
// 编排器 InferenceCycle.RunOnce 严格按"感知 → 对齐 → 门禁 → 规划"顺序调用。
//
// 设计要点：
//  1. 阶段通过 InferenceContext 共享数据，阶段方法不应修改 ctx 之外的全局状态
//  2. 阶段失败不阻塞编排器（错误写到 ctx.Stages，仍继续下一步）
//  3. 阶段可在早退时返回 StopResult（典型场景：危机门禁命中转人工）
//  4. 阶段可设置 ctx.Decision（最终决策），编排器尊重之
// ============================================================================

// InferenceStage 推理阶段接口
type InferenceStage interface {
	// Name 阶段名（用于日志/可观测）
	Name() string

	// Execute 执行阶段
	//
	// 入参：ctx 累积到本阶段为止的所有数据
	// 返回：StageResult
	//   - Continue=true → 编排器继续下一阶段
	//   - Continue=false, EarlyReturn=true → 编排器立即结束（用 decision）
	//   - 任何 Error 不视为立即结束（除非阶段选择 FailResult）
	Execute(ctx context.Context, ic *InferenceContext) StageResult
}

// ============================================================================
// Stage 1: 感知阶段接口
// ============================================================================

// PerceptionStage 感知阶段
//
// 职责：情绪分析 + 意图识别
// 实现：基于规则（关键词命中）+ 现有 intent_recognition 服务
type PerceptionStage interface {
	InferenceStage
}

// SentimentAnalyzer 情绪分析器
type SentimentAnalyzer interface {
	Analyze(ctx context.Context, text string) SentimentScore
}

// IntentRecognizer 意图识别器
type IntentRecognizer interface {
	Recognize(ctx context.Context, text string, hint map[string]string) IntentResult
}

// ============================================================================
// Stage 2: 6维拟人度打分接口
// ============================================================================

// AlignmentScorer 6维拟人度评分器
type AlignmentScorer interface {
	Score(ctx context.Context, ic *InferenceContext) AlignmentScore
}

// ============================================================================
// Stage 3: 危机感门禁接口
// ============================================================================

// CrisisDetector 危机感检测器
type CrisisDetector interface {
	// Detect 基于情绪 + 意图 + 关键词检测危机等级
	Detect(ctx context.Context, ic *InferenceContext) CrisisSignal
}

// ============================================================================
// Stage 4a: 转人工门禁接口
// ============================================================================

// EscalationHandler 转人工处理器
//
// 命中危机时调用：写 Redis 永久锁 + 通知坐席 + 准备降级话术
type EscalationHandler interface {
	// Escalate 触发转人工门禁
	// 返回：转人工后的降级回复 + 是否成功锁定
	Escalate(ctx context.Context, ic *InferenceContext) (*EscalationResult, error)
}

// EscalationResult 转人工结果
type EscalationResult struct {
	Locked        bool      `json:"locked"`                   // 是否成功加锁
	Notified      bool      `json:"notified"`                 // 是否通知坐席
	NotifiedStaff []string  `json:"notified_staff,omitempty"` // 通知到的坐席
	FallbackReply string    `json:"fallback_reply"`           // 转人工后的降级回复（"已为您转接人工客服"）
	Reason        string    `json:"reason"`                   // 转人工原因
	StartedAt     time.Time `json:"started_at"`
}

// ============================================================================
// Stage 4b: 任务规划器接口
// ============================================================================

// TaskPlanner 任务规划器
//
// 正常路径：基于意图/情绪/对齐分数，规划下一步工具调用
// 输出 ActionPlan，被 AgentRuntime 用于驱动工具链 + LLM
type TaskPlanner interface {
	Plan(ctx context.Context, ic *InferenceContext) (*ActionPlan, error)
}

// ============================================================================
// Stage 5: 动作执行器接口
// ============================================================================

// ActionExecutor 动作执行器
//
// 执行 ActionPlan：调 LLM / 调工具 / 写记忆
// 返回最终回复（用于回写到 SalesResponse）
type ActionExecutor interface {
	// Execute 执行计划，返回最终回复内容
	Execute(ctx context.Context, ic *InferenceContext, plan *ActionPlan) (*ActionResult, error)
}

// ActionResult 动作执行结果
type ActionResult struct {
	Reply       string         `json:"reply"`
	ReplyType   string         `json:"reply_type"` // text/card/handoff
	ToolsCalled []string       `json:"tools_called"`
	LLMModel    string         `json:"llm_model"`
	TokensUsed  int            `json:"tokens_used"`
	Confidence  float64        `json:"confidence"`
	Duration    time.Duration  `json:"duration_ns"`
	Extra       map[string]any `json:"extra,omitempty"`
}
