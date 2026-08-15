package dto

import "time"

// KuaishouCardStatsRequest 快手卡片统计数据请求
type KuaishouCardStatsRequest struct {
	CardID    uint      `form:"cardId" json:"cardId" binding:"required"`
	StartDate time.Time `form:"startDate" json:"startDate"`
	EndDate   time.Time `form:"endDate" json:"endDate"`
	GroupBy   string    `form:"groupBy" json:"groupBy"` 
}

// KuaishouCardStatsResponse 快手卡片统计数据响应
type KuaishouCardStatsResponse struct {
	CardID     uint        `json:"cardId"`
	CardTitle  string      `json:"cardTitle"`
	TotalViews int         `json:"totalViews"`
	DailyStats []DailyStat `json:"dailyStats"`
}

// KuaishouCardOverallStatsRequest 快手卡片总体统计数据请求
type KuaishouCardOverallStatsRequest struct {
	StartDate time.Time `form:"startDate" json:"startDate"`
	EndDate   time.Time `form:"endDate" json:"endDate"`
	GroupBy   string    `form:"groupBy" json:"groupBy"` 
	Limit     int       `form:"limit" json:"limit"`
}

// KuaishouCardOverallStatsResponse 快手卡片总体统计数据响应
type KuaishouCardOverallStatsResponse struct {
	TotalCards       int           `json:"totalCards"`
	ActiveCards      int           `json:"activeCards"`
	TotalViews       int           `json:"totalViews"`
	PopularCards     []PopularCard `json:"popularCards"`
	DailyStats       []DailyStat   `json:"dailyStats"`
	RecentActivities []Activity    `json:"recentActivities"`
}

// KuaishouPopularCard 快手热门卡片
type KuaishouPopularCard struct {
	CardID    uint   `json:"cardId"`
	Title     string `json:"title"`
	Views     int    `json:"views"`
	ShortLink string `json:"shortLink"`
}

// KuaishouActivity 快手活动记录
type KuaishouActivity struct {
	ID        uint      `json:"id"`
	CardID    uint      `json:"cardId"`
	CardTitle string    `json:"cardTitle"`
	Action    string    `json:"action"` 
	UserIP    string    `json:"userIp"`
	UserAgent string    `json:"userAgent"`
	ExtraData string    `json:"extraData"`
	CreatedAt time.Time `json:"createdAt"`
}

