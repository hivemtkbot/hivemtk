package dto

import "time"

// VerifyRequest 品牌验证请求
type VerifyRequest struct {
	ArticleID string   `json:"article_id"`
	Query     string   `json:"query"`
	BrandName string   `json:"brand_name"`
	Models    []string `json:"models"`
}

// VerifyResultResponse 验证结果响应
type VerifyResultResponse struct {
	ID             string    `json:"id"`
	ArticleID      string    `json:"article_id"`
	Model          string    `json:"model"`
	Query          string    `json:"query"`
	BrandMentioned bool      `json:"brand_mentioned"`
	MentionCount   int       `json:"mention_count"`
	Sentiment      string    `json:"sentiment"`
	Position       string    `json:"position"`
	CreatedAt      time.Time `json:"created_at"`
}
