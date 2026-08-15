package dto

// XiaohongshuCardStatsRequest 小红书卡片统计请求
type XiaohongshuCardStatsRequest struct {
	CardID    uint   `json:"cardId" form:"cardId"`       
	StartDate string `json:"startDate" form:"startDate"` 
	EndDate   string `json:"endDate" form:"endDate"`     
	GroupBy   string `json:"groupBy" form:"groupBy"`     
}

// XiaohongshuCardStatsResponse 小红书卡片统计响应
type XiaohongshuCardStatsResponse struct {
	CardID         uint        `json:"cardId"`         
	Title          string      `json:"title"`          
	ViewCount      int         `json:"viewCount"`      
	DailyStats     []DailyStat `json:"dailyStats"`     
	RecentActivity []Activity  `json:"recentActivity"` 
}

// XiaohongshuCardOverallStatsRequest 小红书卡片总体统计请求
type XiaohongshuCardOverallStatsRequest struct {
	GroupBy   string `json:"groupBy" form:"groupBy"` 
	StartDate string `json:"startDate" form:"startDate"`
	EndDate   string `json:"endDate" form:"endDate"`
}

// XiaohongshuCardOverallStatsResponse 小红书卡片总体统计响应
type XiaohongshuCardOverallStatsResponse struct {
	TotalCards     int           `json:"totalCards"`     
	ActiveCards    int           `json:"activeCards"`    
	TotalViews     int           `json:"totalViews"`     
	PopularCards   []PopularCard `json:"popularCards"`   
	DailyStats     []DailyStat   `json:"dailyStats"`     
	RecentActivity []Activity    `json:"recentActivity"` 
}

