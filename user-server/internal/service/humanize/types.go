package humanize

// types.go 拟人度评估器类型与接口定义
//
// 五层架构归属: L3 业务层 / L4 能力层
// 设计依据: docs/核心链路优化.md 第十六章 §16.4.1
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"errors"

	"marketing/internal/dto"
	"marketing/internal/model"
)

// HumanizeEvaluator 单次评估器接口（规则与 LLM 共同实现）
type HumanizeEvaluator interface {
	// Evaluate 单次评估
	// input 不允许 nil 且 AIReply 不允许空
	Evaluate(ctx context.Context, input *dto.HumanizeEvalInput) (*dto.HumanizeEvalResult, error)
}

// HumanizeRegenerateFn 重生成回调
//
// 调用方提供：根据上次评估结果重新生成回复
type HumanizeRegenerateFn func(ctx context.Context, input *dto.HumanizeEvalInput, feedback *dto.HumanizeEvalResult) (string, error)

// HumanizeScoreRepository 评分持久化仓储接口
//
// service/humanize 包只依赖 Save 方法；查询方法在 repository 包中独立提供，
// 避免 service 层反向依赖 model 类型
type HumanizeScoreRepository interface {
	Save(ctx context.Context, score *model.HumanizeScore, dimensions []model.HumanizeDimensionRecord) error
}

// ChampionBaselineRepository 销冠基线仓储接口
type ChampionBaselineRepository interface {
	// FindByPersonaIndustryIntent 查找启用的基线（取最新版本）
	FindByPersonaIndustryIntent(ctx context.Context, persona, industry, intent string) (*model.ChampionBaseline, error)
	// Save 保存新版本基线
	Save(ctx context.Context, b *model.ChampionBaseline) (uint64, error)
	// ListEnabled 列出所有启用的基线
	ListEnabled(ctx context.Context) ([]model.ChampionBaseline, error)
	// RefreshPhrases 刷新短语库（异步）
	RefreshPhrases(ctx context.Context, baselineID uint64, phrases []model.ChampionPhrase) error
}

// ChampionMessage 销冠对话消息（用于基线刷新与短语提取）
type ChampionMessage struct {
	Content             string
	PrevCustomerMessage string
	Persona             string
	Industry            string
	Intent              string
}

// LowQualitySampleCollector 低质样本收集器接口
//
// 复用 已有 DBLowQualitySampleCollector，但 HumanizeEvalService 通过此抽象接口调用
type LowQualitySampleCollector interface {
	Collect(ctx context.Context, sample *model.LowQualitySample) error
}

// LLMDispatcher LLM 调度器接口（抽象 llm.Dispatcher，便于测试）
type LLMDispatcher interface {
	// ChatSend 发送聊天请求，返回 content 与 model
	ChatSend(ctx context.Context, prompt string) (content, model string, err error)
}

// ErrInvalidInput 输入非法
var ErrInvalidInput = errors.New("humanize: invalid input")

// ErrInsufficientSamples 样本不足
var ErrInsufficientSamples = errors.New("humanize: insufficient samples")

// ErrAggregatorNotInitialized 聚合器未初始化
var ErrAggregatorNotInitialized = errors.New("humanize: aggregator not initialized")

// clampScore 将分数 clip 到 [0, 1]
func clampScore(s float64) float64 {
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return s
}

// computeHumanizeWeightedScore 计算加权综合分
//
// total = Σ w_i * s_i，权重来自 dto.HumanizeDimensionWeight
func computeHumanizeWeightedScore(scores []dto.HumanizeDimensionScore) float64 {
	if len(scores) == 0 {
		return 0
	}
	m := make(map[dto.HumanizeDimension]float64, len(scores))
	for _, s := range scores {
		m[s.Dimension] = s.Score
	}
	total := 0.0
	for _, dim := range dto.AllHumanizeDimensions {
		total += dto.HumanizeDimensionWeight[dim] * m[dim]
	}
	return total
}
