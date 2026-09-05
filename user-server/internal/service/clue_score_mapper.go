package service

import (
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

func FromClueScoreModel(s *model.ClueScore) *dto.ClueScoreResponse {
	if s == nil {
		return nil
	}
	return &dto.ClueScoreResponse{
		ClueID:          s.ClueID,
		Account:         s.Account,
		TotalScore:      s.TotalScore,
		Grade:           s.Grade,
		Confidence:      s.Confidence,
		ChannelScore:    s.ChannelScore,
		VerifyScore:     s.VerifyScore,
		ProfileScore:    s.ProfileScore,
		EngagementScore: s.EngagementScore,
		RecencyScore:    s.RecencyScore,
		ModelVersion:    s.ModelVersion,
		ScoredAt:        s.ScoredAt.UTC().Format("2006-01-02T15:04:05Z"),
		FactorsJSON:     s.FactorsJSON,
	}
}
