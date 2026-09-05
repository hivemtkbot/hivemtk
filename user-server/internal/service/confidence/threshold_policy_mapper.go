package confidence

import (
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

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
