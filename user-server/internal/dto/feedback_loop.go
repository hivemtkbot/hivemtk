package dto

// feedback_loop.go P0-5 反馈学习闭环数据传输对象
//
// 五层架构归属: L2 网关/L3 业务 之间的传输层
// 设计依据: docs/核心链路优化.md 第十七章 §17.2/§17.4
//
// 私域独立部署: 无 merchant_id 字段
//
// 本文件定义 service/feedback_loop 包对外暴露的 DTO：
//   1. CollectRequest        反馈采集请求（外部 → service）
//   2. ChampionAnalysisReport 销冠对话分析报告（service → 外部）
//   3. OptimizationReport    SOP 优化报告（service → 外部）
//   4. BanditSelectResult    Bandit 选择结果（service → 外部）

import (
	"errors"
	"time"
)

// ----------------------------------------------------------------------------
// 信号类型（与 model.FeedbackSignalKey 字符串值一致）
// ----------------------------------------------------------------------------

// FeedbackSignalKey 反馈信号类型
type FeedbackSignalKey string

const (
	FBSignalLike         FeedbackSignalKey = "like"          // 客户点赞（显式）
	FBSignalDislike      FeedbackSignalKey = "dislike"       // 客户点踩（显式）
	FBSignalRating       FeedbackSignalKey = "rating"        // 评分 1-5（显式）
	FBSignalComplaint    FeedbackSignalKey = "complaint"     // 投诉（显式）
	FBSignalConversion   FeedbackSignalKey = "conversion"    // 转化成功（隐式）
	FBSignalReplyRate    FeedbackSignalKey = "reply_rate"    // 客户回复率（隐式）
	FBSignalDuration     FeedbackSignalKey = "duration"      // 会话时长（隐式）
	FBSignalTransfer     FeedbackSignalKey = "transfer"      // 转人工（隐式）
	FBSignalChampionMark FeedbackSignalKey = "champion_mark" // 销冠标记（销冠）
	FBSignalScriptAdopt  FeedbackSignalKey = "script_adopt"  // 话术采用（销冠）
)

// FeedbackEventType 反馈事件类型
type FeedbackEventType string

const (
	FBEventTypeExplicit FeedbackEventType = "explicit" // 显式反馈
	FBEventTypeImplicit FeedbackEventType = "implicit" // 隐式反馈
	FBEventTypeChampion FeedbackEventType = "champion" // 销冠标记
)

// ----------------------------------------------------------------------------
// CollectRequest 反馈采集请求
// ----------------------------------------------------------------------------

// CollectRequest 反馈采集请求（外部 → FeedbackCollector）
//
// 用法：SalesEngine / PersonaEvaluator / API handler 构造此请求提交给 FeedbackCollector.Collect
type CollectRequest struct {
	SessionID         string            `json:"session_id"`          // 必填
	CustomerID        string            `json:"customer_id"`         // 必填
	SOPID             uint              `json:"sop_id"`              // 关联 SOP（0 表示无）
	ExecutionID       uint              `json:"execution_id"`        // SOP 执行 ID（0 表示无）
	Variant           string            `json:"variant"`             // A/B variant
	PromptCandidateID uint              `json:"prompt_candidate_id"` // Prompt 候选 ID
	EventType         FeedbackEventType `json:"event_type"`          // explicit/implicit/champion
	SignalKey         FeedbackSignalKey `json:"signal_key"`          // 信号类型
	SignalValue       any               `json:"signal_value"`        // 信号原始值（bool/float64/string）
	AIReply           string            `json:"ai_reply"`            // 触发反馈的 AI 回复快照
	CustomerMsg       string            `json:"customer_msg"`        // 客户消息快照
	Metadata          map[string]any    `json:"metadata"`            // 扩展元数据
	CreatedBy         uint              `json:"created_by"`          // 提交者（0 表示系统）
}

// Validate 校验采集请求合法性
func (r *CollectRequest) Validate() error {
	if r == nil {
		return ErrFeedbackRequestNil
	}
	if r.SessionID == "" {
		return ErrFeedbackSessionEmpty
	}
	if r.CustomerID == "" {
		return ErrFeedbackCustomerEmpty
	}
	if r.EventType == "" {
		return ErrFeedbackEventTypeEmpty
	}
	if r.SignalKey == "" {
		return ErrFeedbackSignalKeyEmpty
	}
	return nil
}

// ----------------------------------------------------------------------------
// ChampionAnalysisReport 销冠对话分析报告
// ----------------------------------------------------------------------------

// ChampionAnalysisReport 销冠对话分析报告（service → 外部）
//
// 由 ChampionDialogueAnalyzer.AnalyzePipeline 返回
type ChampionAnalysisReport struct {
	RunAt            time.Time            `json:"run_at"`            // 执行时间
	Since            time.Time            `json:"since"`             // 起始时间范围
	CandidateCount   int                  `json:"candidate_count"`   // 候选对话总数
	ClusterCount     int                  `json:"cluster_count"`     // 聚类簇数（含被过滤的小簇）
	PersistedCount   int                  `json:"persisted_count"`   // 持久化的销冠对话数
	ExtractedScripts []ExtractedScriptDTO `json:"extracted_scripts"` // LLM 提取的话术
	Errors           []string             `json:"errors"`            // 阶段性错误（不阻断整体流程）
}

// ExtractedScriptDTO LLM 提取的话术（service → script_templates 入库）
type ExtractedScriptDTO struct {
	Title              string   `json:"title"`               // 话术标题（≤20字）
	Content            string   `json:"content"`             // 话术正文（含变量占位符 {{product}}/{{customer_name}}）
	Scenario           string   `json:"scenario"`            // objection/closing/followup/nurture/repurchase
	TriggerKeywords    []string `json:"trigger_keywords"`    // 触发关键词
	JourneyStage       string   `json:"journey_stage"`       // lead/contact/consider/decide/retain
	EffectivenessScore float64  `json:"effectiveness_score"` // 0-1 有效性评分
	ClusterID          uint     `json:"cluster_id"`          // 来源聚类簇 ID
}

// ----------------------------------------------------------------------------
// OptimizationReport SOP 优化报告
// ----------------------------------------------------------------------------

// OptimizationReport SOP 优化报告（service → 外部）
//
// 由 SOPAutoOptimizer.ProcessPendingSuggestions 返回
type OptimizationReport struct {
	RunAt           time.Time `json:"run_at"`
	PendingCount    int       `json:"pending_count"`     // 待处理建议总数
	AppliedCount    int       `json:"applied_count"`     // 已自动应用数
	FailedCount     int       `json:"failed_count"`      // 应用失败数
	RolledBackCount int       `json:"rolled_back_count"` // 已回滚数
	PromotedCount   int       `json:"promoted_count"`    // 已自动选优数
	Errors          []string  `json:"errors"`            // 错误明细
}

// ----------------------------------------------------------------------------
// BanditSelectResult Bandit 选择结果
// ----------------------------------------------------------------------------

// BanditSelectResult Bandit 选择结果（service → 外部）
//
// 由 BanditAllocator.SelectArm / SelectPrompt 返回
type BanditSelectResult struct {
	ExperimentID      string `json:"experiment_id"`
	ArmKey            string `json:"arm_key"`             // 选中的 arm key
	PromptCandidateID uint   `json:"prompt_candidate_id"` // 若为 prompt 实验，关联的候选 ID
	SampleStrategy    string `json:"sample_strategy"`     // cold_start_uniform / thompson_sampling / forced_explore
	TotalSamples      int64  `json:"total_samples"`       // 当前实验总样本数
	CacheHit          bool   `json:"cache_hit"`           // 是否命中内存缓存
}

// BanditConvergenceResult Bandit 收敛检查结果
type BanditConvergenceResult struct {
	ExperimentID  string  `json:"experiment_id"`
	Converged     bool    `json:"converged"`
	WinnerArmKey  string  `json:"winner_arm_key"`
	PosteriorProb float64 `json:"posterior_prob"` // P(winner 最优) 后验概率
	TotalSamples  int64   `json:"total_samples"`
	MinSamplesMet bool    `json:"min_samples_met"` // 是否达到最小样本数
}

// ----------------------------------------------------------------------------
// 错误定义
// ----------------------------------------------------------------------------

var (
	ErrFeedbackRequestNil     = errors.New("feedback_loop: collect request is nil")
	ErrFeedbackSessionEmpty   = errors.New("feedback_loop: session_id is empty")
	ErrFeedbackCustomerEmpty  = errors.New("feedback_loop: customer_id is empty")
	ErrFeedbackEventTypeEmpty = errors.New("feedback_loop: event_type is empty")
	ErrFeedbackSignalKeyEmpty = errors.New("feedback_loop: signal_key is empty")
)
