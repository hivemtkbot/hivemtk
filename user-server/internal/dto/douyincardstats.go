package dto

// DouyinCardStatsRequest 抖音卡片统计请求
type DouyinCardStatsRequest struct {
	CardID    uint   `json:"cardId" form:"cardId"`       
	StartDate string `json:"startDate" form:"startDate"` 
	EndDate   string `json:"endDate" form:"endDate"`     
	GroupBy   string `json:"groupBy" form:"groupBy"`     
}

// DouyinCardStatsResponse 抖音卡片统计响应
type DouyinCardStatsResponse struct {
	CardID         uint        `json:"cardId"`         
	Title          string      `json:"title"`          
	ViewCount      int         `json:"viewCount"`      
	DailyStats     []DailyStat `json:"dailyStats"`     
	RecentActivity []Activity  `json:"recentActivity"` 
}

// DouyinCardStatsByTime 按时间分组的统计数据
// DailyStat 日统计数据
type DailyStat struct {
	Date string `json:"date"`
	View int    `json:"view"`
}

// DouyinCardOverallStatsResponse 抖音卡片总体统计响应
// DouyinCardOverallStatsRequest 抖音卡片总体统计请求
type DouyinCardOverallStatsRequest struct {
	GroupBy   string `json:"groupBy" form:"groupBy"` 
	StartDate string `json:"startDate" form:"startDate"`
	EndDate   string `json:"endDate" form:"endDate"`
}

// DouyinCardOverallStatsResponse 抖音卡片总体统计响应
type DouyinCardOverallStatsResponse struct {
	TotalCards     int           `json:"totalCards"`     
	ActiveCards    int           `json:"activeCards"`    
	TotalViews     int           `json:"totalViews"`     
	PopularCards   []PopularCard `json:"popularCards"`   
	DailyStats     []DailyStat   `json:"dailyStats"`     
	RecentActivity []Activity    `json:"recentActivity"` 
}

// DouyinCardTopStats 热门卡片统计
// PopularCard 热门卡片
type PopularCard struct {
	ID        uint   `json:"id"`        
	Title     string `json:"title"`     
	ViewCount int    `json:"viewCount"` 
	CreatedAt string `json:"createdAt"` 
}

// DouyinCardActivity 卡片活动记录
// Activity 活动
type Activity struct {
	ID        uint   `json:"id"`        
	CardID    uint   `json:"cardId"`    
	Action    string `json:"action"`    
	Username  string `json:"username"`  
	IPAddress string `json:"ipAddress"` 
	CreatedAt string `json:"createdAt"` 
}

