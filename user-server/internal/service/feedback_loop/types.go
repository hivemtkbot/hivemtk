package feedbackloop

// types.go P0-5 反馈学习闭环类型与接口定义
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十七章 §17.4
//
// 私域独立部署: 无 merchant_id 字段
//
// 本包包含 5 个组件：
//   1. FeedbackCollector          反馈信号采集器（异步队列 + 落库 + 聚合）
//   2. ChampionDialogueAnalyzer   销冠对话分析器（pgvector 聚类 + LLM 话术提取）
//   3. PromptIterator             Prompt 迭代器（基于负反馈生成候选）
//   4. SOPAutoOptimizer           SOP 自动优化器（应用建议 + A/B + 回滚）
//   5. BanditAllocator            Multi-Armed Bandit 流量分配器（Thompson Sampling）
//
// 依赖抽象：
//   - DB: *gorm.DB（通过构造函数注入）
//   - LLM: LLMDispatcher 接口（抽象 llm.Dispatcher，便于测试）
//   - Embedding: Embedder 接口（抽象 embedding.LocalEmbedding，便于测试）

import (
	"context"
	"errors"
	"time"

	"marketing/internal/dto"
)

// ----------------------------------------------------------------------------
// 抽象接口（依赖倒置，便于单元测试）
// ----------------------------------------------------------------------------

// LLMDispatcher LLM 调度器接口
//
// 抽象 llm.Dispatcher，使 feedback_loop 包不直接依赖 llm 包
// 测试时可注入 stub 实现
type LLMDispatcher interface {
	// Dispatch 发送调度请求，返回 content 与 model
	Dispatch(ctx context.Context, scenario string, prompt, systemPrompt string, jsonMode bool, maxTokens int) (content, model string, err error)
}

// Embedder 文本向量化接口
//
// 抽象 embedding.LocalEmbedding，使 feedback_loop 包不直接依赖 embedding 包
type Embedder interface {
	// Embed 单条文本向量化，返回 float32 切片
	Embed(text string) []float32
	// Dimension 返回向量维度
	Dimension() int
}

// BanditAllocatorInterface Bandit 分配器接口（供 SOPAutoOptimizer 依赖）
//
// 实现方为 BanditAllocator，抽象接口便于测试 SOPAutoOptimizer 时注入 mock
type BanditAllocatorInterface interface {
	CheckConvergence(ctx context.Context, experimentID string) (string, bool)
	PromoteArm(ctx context.Context, experimentID, winnerKey string) error
}

// ----------------------------------------------------------------------------
// 信号权重配置
// ----------------------------------------------------------------------------

// SignalWeightMap 信号权重映射（可由配置覆盖）
//
// 权重设计依据 docs/核心链路优化.md §17.2.1 信号权重设计表：
//
//	客户点赞 +1.0  客户点踩 -1.5  评分 ≥4 +0.8  评分 ≤2 -1.0  投诉 -2.0
//	转化成功 +2.0   回复率>0.7 +0.5  会话>5min +0.3  转人工 -0.5
//	销冠标记 +1.5   话术采用 +0.6
type SignalWeightMap map[dto.FeedbackSignalKey]float64

// DefaultSignalWeights 默认信号权重
var DefaultSignalWeights = SignalWeightMap{
	dto.FBSignalLike:         1.0,
	dto.FBSignalDislike:      -1.5,
	dto.FBSignalRating:       0.8, // 评分需归一化（v/5）
	dto.FBSignalComplaint:    -2.0,
	dto.FBSignalConversion:   2.0,
	dto.FBSignalReplyRate:    0.5, // 已归一化（0-1）
	dto.FBSignalDuration:     0.3, // 需归一化（v/300）
	dto.FBSignalTransfer:     -0.5,
	dto.FBSignalChampionMark: 1.5,
	dto.FBSignalScriptAdopt:  0.6,
}

// ----------------------------------------------------------------------------
// BanditAllocator 配置
// ----------------------------------------------------------------------------

// BanditConfig Bandit 分配器配置
type BanditConfig struct {
	MinSamplesForExploit int     // 利用期最小样本数（默认 30，每臂）
	ExplorationFloor     float64 // 探索期最低流量比例（默认 0.10）
	TrafficCeiling       float64 // 单臂流量上限（默认 0.60）
	ConvergenceThreshold float64 // 收敛后验概率阈值（默认 0.95）
	MinSamplesForPromote int     // 自动选优最小样本数（默认 100，每臂）
	PosteriorSamples     int     // 蒙特卡洛采样次数（默认 1000）
}

// DefaultBanditConfig 默认 Bandit 配置
func DefaultBanditConfig() BanditConfig {
	return BanditConfig{
		MinSamplesForExploit: 30,
		ExplorationFloor:     0.10,
		TrafficCeiling:       0.60,
		ConvergenceThreshold: 0.95,
		MinSamplesForPromote: 100,
		PosteriorSamples:     1000,
	}
}

// ----------------------------------------------------------------------------
// FeedbackCollector 配置
// ----------------------------------------------------------------------------

// FeedbackCollectorConfig 反馈采集器配置
type FeedbackCollectorConfig struct {
	QueueSize     int             // 异步队列长度（默认 1000）
	FlushInterval time.Duration   // 批量刷盘间隔（默认 2s）
	BatchSize     int             // 批量大小阈值（默认 50）
	Weights       SignalWeightMap // 信号权重（默认 DefaultSignalWeights）
}

// DefaultFeedbackCollectorConfig 默认采集器配置
func DefaultFeedbackCollectorConfig() FeedbackCollectorConfig {
	return FeedbackCollectorConfig{
		QueueSize:     1000,
		FlushInterval: 2 * time.Second,
		BatchSize:     50,
		Weights:       DefaultSignalWeights,
	}
}

// ----------------------------------------------------------------------------
// ChampionDialogueAnalyzer 配置
// ----------------------------------------------------------------------------

// ChampionAnalyzerConfig 销冠对话分析器配置
type ChampionAnalyzerConfig struct {
	MinReward           float64 // 最低奖励阈值（默认 1.0）
	ClusterSimThreshold float64 // 聚类相似度阈值（默认 0.85，pgvector cosine similarity）
	MinClusterSize      int     // 最小簇大小（默认 3，<3 视为噪声丢弃）
	TopKPerCluster      int     // 每簇取 Top-K 代表样本（默认 3）
	MaxDialoguesPerRun  int     // 每次运行最大处理对话数（默认 500）
}

// DefaultChampionAnalyzerConfig 默认分析器配置
func DefaultChampionAnalyzerConfig() ChampionAnalyzerConfig {
	return ChampionAnalyzerConfig{
		MinReward:           1.0,
		ClusterSimThreshold: 0.85,
		MinClusterSize:      3,
		TopKPerCluster:      3,
		MaxDialoguesPerRun:  500,
	}
}

// ----------------------------------------------------------------------------
// PromptIterator 配置
// ----------------------------------------------------------------------------

// PromptIteratorConfig Prompt 迭代器配置
type PromptIteratorConfig struct {
	MinSamplesForIteration  int     // 触发迭代的最小样本数（默认 50）
	NegativeRewardThreshold float64 // 负反馈阈值（默认 -0.5）
	CandidatesPerRun        int     // 每次迭代生成候选数（默认 3）
	AutoApprove             bool    // 是否自动审核通过（默认 false）
}

// DefaultPromptIteratorConfig 默认迭代器配置
func DefaultPromptIteratorConfig() PromptIteratorConfig {
	return PromptIteratorConfig{
		MinSamplesForIteration:  50,
		NegativeRewardThreshold: -0.5,
		CandidatesPerRun:        3,
		AutoApprove:             false,
	}
}

// ----------------------------------------------------------------------------
// SOPAutoOptimizer 配置
// ----------------------------------------------------------------------------

// SOPAutoOptimizerConfig SOP 自动优化器配置
type SOPAutoOptimizerConfig struct {
	AutoApplyPriority      int           // 自动应用的最低 priority（默认 2，即只自动应用 priority≥2）
	RollbackDropThreshold  float64       // 转化率下降阈值（默认 0.20，下降 20% 触发回滚）
	RollbackComplaintRatio float64       // 投诉率上升阈值（默认 0.50，上升 50% 触发回滚）
	ABTestDuration         time.Duration // A/B 测试周期（默认 7 天）
}

// DefaultSOPAutoOptimizerConfig 默认优化器配置
func DefaultSOPAutoOptimizerConfig() SOPAutoOptimizerConfig {
	return SOPAutoOptimizerConfig{
		AutoApplyPriority:      2,
		RollbackDropThreshold:  0.20,
		RollbackComplaintRatio: 0.50,
		ABTestDuration:         7 * 24 * time.Hour,
	}
}

// ----------------------------------------------------------------------------
// 通用错误
// ----------------------------------------------------------------------------

var (
	ErrInvalidInput         = errors.New("feedback_loop: invalid input")
	ErrNoArms               = errors.New("feedback_loop: no arms for experiment")
	ErrInsufficientSamples  = errors.New("feedback_loop: insufficient samples for iteration")
	ErrExperimentNotFound   = errors.New("feedback_loop: experiment not found")
	ErrActivePromptNotFound = errors.New("feedback_loop: no active prompt for sop node")
	ErrDispatcherNotConfig  = errors.New("feedback_loop: llm dispatcher not configured")
	ErrEmbedderNotConfig    = errors.New("feedback_loop: embedder not configured")
	ErrQueueFull            = errors.New("feedback_loop: feedback queue full, event dropped")
)
