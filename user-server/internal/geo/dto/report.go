package dto

// ReportRequest 报表请求
type ReportRequest struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	GroupBy   string `json:"group_by"`
}

// ROIRequest 成本 ROI 请求
type ROIRequest struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// ReportResponse 报表响应
type ReportResponse struct {
	TotalArticles      int64   `json:"total_articles"`
	TotalKeywords      int64   `json:"total_keywords"`
	TotalOptimizations int64   `json:"total_optimizations"`
	TotalVerifications int64   `json:"total_verifications"`
	TotalCostUSD       float64 `json:"total_cost_usd"`
	TotalCostCNY       float64 `json:"total_cost_cny"`
	TotalAPICalls      int64   `json:"total_api_calls"`
}

// APICostItem API 成本条目
type APICostItem struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	CallCount    int64   `json:"call_count"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	CostCNY      float64 `json:"cost_cny"`
}
