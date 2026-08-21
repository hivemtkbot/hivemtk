package dto

// KeywordPerformance 关键词表现数据
type KeywordPerformance struct {
	Keyword        string  `json:"keyword"`
	QueryCount     int     `json:"query_count"`
	MentionCount   int     `json:"mention_count"`
	MentionRate    float64 `json:"mention_rate"`
	AvgSentiment   string  `json:"avg_sentiment"`
	HighValueScore float64 `json:"high_value_score"`
}

// KeywordEnhanceResponse 关键词数据增强响应
type KeywordEnhanceResponse struct {
	HasData            bool                  `json:"has_data"`
	Message            string                `json:"message,omitempty"`
	TotalKeywords      int                   `json:"total_keywords"`
	HighValueKeywords  []*KeywordPerformance `json:"high_value_keywords"`
	Suggestions        []string              `json:"suggestions"`
	IntentDistribution map[string]int        `json:"intent_distribution"`
}
