package dto

// PlatformCardStatsRequest 平台卡片统计请求（统一入口）
//
// LM-：用于替代各平台自有的 *CardStatsRequest 命名差异（douyin / kuaishou / xiaohongshu
// / xianyu / tiktok），统一通过 Platform 字段路由到具体平台 service。
type PlatformCardStatsRequest struct {
	Platform  string `json:"platform" form:"platform"`   
	CardID    uint   `json:"cardId" form:"cardId"`       
	StartDate string `json:"startDate" form:"startDate"` 
	EndDate   string `json:"endDate" form:"endDate"`     
	GroupBy   string `json:"groupBy" form:"groupBy"`     
}

// PlatformCardStatsResponse 平台卡片统计响应（统一入口）
type PlatformCardStatsResponse struct {
	Platform       string      `json:"platform"`       
	CardID         uint        `json:"cardId"`         
	Title          string      `json:"title"`          
	ViewCount      int         `json:"viewCount"`      
	ClickCount     int         `json:"clickCount"`     
	ShareCount     int         `json:"shareCount"`     
	DailyStats     []DailyStat `json:"dailyStats"`     
	RecentActivity []Activity  `json:"recentActivity"` 
}

// PlatformCardOverallStatsRequest 平台卡片总体统计请求（统一入口）
type PlatformCardOverallStatsRequest struct {
	Platform  string `json:"platform" form:"platform"`   
	GroupBy   string `json:"groupBy" form:"groupBy"`     
	StartDate string `json:"startDate" form:"startDate"` 
	EndDate   string `json:"endDate" form:"endDate"`     
	Limit     int    `json:"limit" form:"limit"`         
}

// PlatformCardOverallStatsResponse 平台卡片总体统计响应（统一入口）
type PlatformCardOverallStatsResponse struct {
	Platform       string        `json:"platform"`       
	TotalCards     int           `json:"totalCards"`     
	ActiveCards    int           `json:"activeCards"`    
	TotalViews     int           `json:"totalViews"`     
	TotalClicks    int           `json:"totalClicks"`    
	TotalShares    int           `json:"totalShares"`    
	PopularCards   []PopularCard `json:"popularCards"`   
	DailyStats     []DailyStat   `json:"dailyStats"`     
	RecentActivity []Activity    `json:"recentActivity"` 
}

