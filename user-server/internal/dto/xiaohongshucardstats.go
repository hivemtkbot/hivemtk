package dto

// XiaohongshuCardStatsRequest 小红书卡片统计请求
type XiaohongshuCardStatsRequest struct {
	CardID    uint   `json:"cardId" form:"cardId"`       // 卡片ID
	StartDate string `json:"startDate" form:"startDate"` // 开始日期，格式：2023-01-01
	EndDate   string `json:"endDate" form:"endDate"`     // 结束日期，格式：2023-01-31
	GroupBy   string `json:"groupBy" form:"groupBy"`     // 分组方式：day, week, month
}

// XiaohongshuCardStatsResponse 小红书卡片统计响应
type XiaohongshuCardStatsResponse struct {
	CardID         uint        `json:"cardId"`         // 卡片ID
	Title          string      `json:"title"`          // 卡片标题
	ViewCount      int         `json:"viewCount"`      // 总浏览数
	DailyStats     []DailyStat `json:"dailyStats"`     // 按时间分组的统计数据
	RecentActivity []Activity  `json:"recentActivity"` // 最近活动
}

// XiaohongshuCardOverallStatsRequest 小红书卡片总体统计请求
type XiaohongshuCardOverallStatsRequest struct {
	GroupBy   string `json:"groupBy" form:"groupBy"` // day, week, month
	StartDate string `json:"startDate" form:"startDate"`
	EndDate   string `json:"endDate" form:"endDate"`
}

// XiaohongshuCardOverallStatsResponse 小红书卡片总体统计响应
type XiaohongshuCardOverallStatsResponse struct {
	TotalCards     int           `json:"totalCards"`     // 总卡片数
	ActiveCards    int           `json:"activeCards"`    // 激活卡片数
	TotalViews     int           `json:"totalViews"`     // 总浏览数
	PopularCards   []PopularCard `json:"popularCards"`   // 热门卡片
	DailyStats     []DailyStat   `json:"dailyStats"`     // 按时间分组的统计数据
	RecentActivity []Activity    `json:"recentActivity"` // 最近活动
}
