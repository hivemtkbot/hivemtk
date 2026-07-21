package dto

import "marketing/internal/model"

// ClueScoreRequest 单条评分请求
type ClueScoreRequest struct {
	ClueID string `json:"clue_id" binding:"required"`
}

// ClueScoreResponse 评分响应
type ClueScoreResponse struct {
	ClueID          string `json:"clue_id"`
	Account         string `json:"account"`
	TotalScore      int    `json:"total_score"`
	Grade           string `json:"grade"`
	Confidence      int    `json:"confidence"`
	ChannelScore    int    `json:"channel_score"`
	VerifyScore     int    `json:"verify_score"`
	ProfileScore    int    `json:"profile_score"`
	EngagementScore int    `json:"engagement_score"`
	RecencyScore    int    `json:"recency_score"`
	ModelVersion    string `json:"model_version"`
	ScoredAt        string `json:"scored_at"`
	FactorsJSON     string `json:"factors_json"`
}

// FromModel ClueScore → ClueScoreResponse
func (r *ClueScoreResponse) FromModel(s *model.ClueScore) *ClueScoreResponse {
	if s == nil {
		return nil
	}
	return &ClueScoreResponse{
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

// ClueScoreListResponse 评分列表响应
type ClueScoreListResponse struct {
	List     []*ClueScoreResponse `json:"list"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

// ClueEngagementRequest 互动事件请求
type ClueEngagementRequest struct {
	ClueID    string `json:"clue_id" binding:"required"`
	EventType string `json:"event_type" binding:"required"` // reply/click/call/visit
	Channel   string `json:"channel"`
	Payload   any    `json:"payload"`
}

// ScoreAllRequest 批量评分请求
type ScoreAllRequest struct {
	Limit int `form:"limit" json:"limit"`
}

// ScoreAllResponse 批量评分响应
type ScoreAllResponse struct {
	Scored int `json:"scored"`
	Limit  int `json:"limit"`
}

// ClueGradeStats 等级分布
type ClueGradeStats struct {
	Grade string `json:"grade"`
	Count int64  `json:"count"`
}
