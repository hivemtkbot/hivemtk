package confidence


import (
	"math"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

// DynamicThresholdCalculator 4 因子动态阈值计算器
type DynamicThresholdCalculator struct {
	policyEngine *ThresholdPolicyEngine
}

// NewDynamicThresholdCalculator 创建计算器
func NewDynamicThresholdCalculator(engine *ThresholdPolicyEngine) *DynamicThresholdCalculator {
	return &DynamicThresholdCalculator{policyEngine: engine}
}

// ThresholdInput 计算入参
type ThresholdInput struct {
	IntentType        string    
	CustomerLevel     string    
	AgentAvailability float64   
	Now               time.Time 
}

// Calculate 计算 4 因子动态阈值
//
// T = base[intent] + α*customer_level + β*timeslot + γ*agent_availability
// 最终 clip 到 [0.4, 0.95]
//
// 因子说明：
//   - customer_level: vip +1, low -1, normal 0；权重 α = 0.05
//   - timeslot:       高峰(10-12, 14-16) -1（放宽让更多进人工）, 低谷(0-7) +1（加严避免无人值守）, 其他 0；权重 β = 0.05
//   - agent_availability: 空闲>50% -1（多转人工）, 空闲<10% +1（加严避免堆积）, 其他 0；权重 γ = 0.10
func (c *DynamicThresholdCalculator) Calculate(in *ThresholdInput) float64 {
	if c.policyEngine == nil {
		return 0.70 
	}
	policy := c.policyEngine.GetPolicy(in.IntentType)
	if policy == nil {
		return 0.70
	}

	t := policy.BaseThreshold

	// 2. customer_level_factor
	var clFactor float64
	switch in.CustomerLevel {
	case "vip":
		clFactor = 1.0
	case "low":
		clFactor = -1.0
	default:
		clFactor = 0
	}
	t += policy.CustomerLevelWeight * clFactor

	hour := in.Now.Hour()
	var tsFactor float64
	switch {
	case (hour >= 10 && hour < 12) || (hour >= 14 && hour < 16):
		tsFactor = -1.0 
	case hour >= 0 && hour < 7:
		tsFactor = 1.0 
	default:
		tsFactor = 0
	}
	t += policy.TimeslotWeight * tsFactor

	// 4. agent_availability_factor
	var avFactor float64
	switch {
	case in.AgentAvailability > 0.5:
		avFactor = -1.0 
	case in.AgentAvailability < 0.1:
		avFactor = 1.0 
	default:
		avFactor = 0
	}
	t += policy.AgentAvailabilityWeight * avFactor

	t = math.Max(0.40, math.Min(0.95, t))
	return t
}

// DetermineBand 根据 conf 和 threshold 决定决策区间
//
// 注意：band 边界由策略的 BandHandoffUpper / BandFallbackUpper / BandReviewUpper 决定
// 这样允许不同意图有不同的区间边界（运营可调）
//
// 但 conf 与 threshold 的关系也需考虑：
//   - 当 conf < threshold 且 conf < BandHandoffUpper → handoff
//   - 当 conf < threshold 但 conf ∈ [BandHandoffUpper, BandFallbackUpper) → llm_fallback
//   - 当 conf >= threshold 但 conf < BandReviewUpper → review（边界审核）
//   - 当 conf >= BandReviewUpper → auto
//
// 简化实现（按设计文档 §15.2.6）：
//
//	[0, BandHandoffUpper)        → handoff
//	[BandHandoffUpper, BandFallbackUpper) → llm_fallback
//	[BandFallbackUpper, BandReviewUpper)  → review
//	[BandReviewUpper, 1.0]               → auto
//
// threshold 用于触发条件判定，band 边界用 policy 配置
func (c *DynamicThresholdCalculator) DetermineBand(conf, threshold float64, policy *model.ThresholdPolicy) string {
	if policy == nil {
		if conf < 0.40 {
			return dto.BandHandoff
		}
		if conf < 0.60 {
			return dto.BandLLMFallback
		}
		if conf < 0.75 {
			return dto.BandReview
		}
		return dto.BandAuto
	}
	if conf < policy.BandHandoffUpper {
		return dto.BandHandoff
	}
	if conf < policy.BandFallbackUpper {
		return dto.BandLLMFallback
	}
	if conf < policy.BandReviewUpper {
		return dto.BandReview
	}
	return dto.BandAuto
}

