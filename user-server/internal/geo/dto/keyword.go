package dto

import "time"

// MineKeywordsRequest 关键词挖掘请求
type MineKeywordsRequest struct {
	SeedWords  []string `json:"seed_words"`
	Mode       string   `json:"mode"`
	BrandName  string   `json:"brand_name"`
	Advantages []string `json:"advantages"`
}

// SemanticExpandRequest 语义扩展请求
type SemanticExpandRequest struct {
	Keywords   []string `json:"keywords"`
	BrandName  string   `json:"brand_name"`
	ExpandMode string   `json:"expand_mode"`
}

// TopicClusterRequest 话题聚类请求
type TopicClusterRequest struct {
	Keywords  []string `json:"keywords"`
	BrandName string   `json:"brand_name"`
}

// KeywordListResponse 关键词列表响应
type KeywordListResponse struct {
	Total int64          `json:"total"`
	List  []*KeywordItem `json:"list"`
}

// KeywordItem 关键词条目
type KeywordItem struct {
	ID           string    `json:"id"`
	Keyword      string    `json:"keyword"`
	Category     string    `json:"category"`
	Source       string    `json:"source"`
	SearchVolume int       `json:"search_volume"`
	Difficulty   float64   `json:"difficulty"`
	Intent       string    `json:"intent"`
	Cluster      string    `json:"cluster"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}
