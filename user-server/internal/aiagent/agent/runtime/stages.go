package agent_runtime

import (
	"context"
	"time"
)


// InferenceStage 推理阶段接口
type InferenceStage interface {
	Name() string

	Execute(ctx context.Context, ic *InferenceContext) StageResult
}


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


// AlignmentScorer 6维拟人度评分器
type AlignmentScorer interface {
	Score(ctx context.Context, ic *InferenceContext) AlignmentScore
}


// CrisisDetector 危机感检测器
type CrisisDetector interface {
	Detect(ctx context.Context, ic *InferenceContext) CrisisSignal
}


// EscalationHandler 转人工处理器
//
// 命中危机时调用：写 Redis 永久锁 + 通知坐席 + 准备降级话术
type EscalationHandler interface {
	Escalate(ctx context.Context, ic *InferenceContext) (*EscalationResult, error)
}

// EscalationResult 转人工结果
type EscalationResult struct {
	Locked        bool      `json:"locked"`                   
	Notified      bool      `json:"notified"`                 
	NotifiedStaff []string  `json:"notified_staff,omitempty"` 
	FallbackReply string    `json:"fallback_reply"`           
	Reason        string    `json:"reason"`                   
	StartedAt     time.Time `json:"started_at"`
}


// TaskPlanner 任务规划器
//
// 正常路径：基于意图/情绪/对齐分数，规划下一步工具调用
// 输出 ActionPlan，被 AgentRuntime 用于驱动工具链 + LLM
type TaskPlanner interface {
	Plan(ctx context.Context, ic *InferenceContext) (*ActionPlan, error)
}


// ActionExecutor 动作执行器
//
// 执行 ActionPlan：调 LLM / 调工具 / 写记忆
// 返回最终回复（用于回写到 SalesResponse）
type ActionExecutor interface {
	Execute(ctx context.Context, ic *InferenceContext, plan *ActionPlan) (*ActionResult, error)
}

// ActionResult 动作执行结果
type ActionResult struct {
	Reply       string         `json:"reply"`
	ReplyType   string         `json:"reply_type"` 
	ToolsCalled []string       `json:"tools_called"`
	LLMModel    string         `json:"llm_model"`
	TokensUsed  int            `json:"tokens_used"`
	Confidence  float64        `json:"confidence"`
	Duration    time.Duration  `json:"duration_ns"`
	Extra       map[string]any `json:"extra,omitempty"`
}

