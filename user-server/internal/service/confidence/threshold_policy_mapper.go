package confidence

import (
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

// ToThresholdPolicyModel DTO → Model 转换(供 Service/Repository 使用)
// 转换属业务层职责（原位于 dto 包，P0-7 下沉至 service/confidence，dto 保持纯数据结构）
func ToThresholdPolicyModel(r *dto.ThresholdPolicyRequest) *model.ThresholdPolicy {
	if r == nil {
		return nil
	}
	now := time.Now()
	return &model.ThresholdPolicy{
		PolicyID:                r.PolicyID,
		IntentType:              r.IntentType,
		BaseThreshold:           r.BaseThreshold,
		CustomerLevelWeight:     r.CustomerLevelWeight,
		TimeslotWeight:          r.TimeslotWeight,
		AgentAvailabilityWeight: r.AgentAvailabilityWeight,
		BandHandoffUpper:        r.BandHandoffUpper,
		BandFallbackUpper:       r.BandFallbackUpper,
		BandReviewUpper:         r.BandReviewUpper,
		ReviewSLASeconds:        r.ReviewSLASeconds,
		Version:                 r.Version,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
}

