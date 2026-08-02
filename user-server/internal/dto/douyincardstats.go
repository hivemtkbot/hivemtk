package dto

// DouyinCardStatsRequest 抖音卡片统计请求
type DouyinCardStatsRequest struct {
	CardID    uint   `json:"cardId" form:"cardId"`       // 卡片ID
	StartDate string `json:"startDate" form:"startDate"` // 开始日期，格式：
	EndDate   string `json:"endDate" form:"endDate"`     // 结束日期，格式：
	GroupBy   string `json:"groupBy" form:"groupBy"`     // 分组方式：day, week, month
}

// DouyinCardStatsResponse 抖音卡片统计响应
type DouyinCardStatsResponse struct {
	CardID         uint        `json:"cardId"`         // 卡片ID
	Title          string      `json:"title"`          // 卡片标题
	ViewCount      int         `json:"viewCount"`      // 总浏览数
	DailyStats     []DailyStat `json:"dailyStats"`     // 按时间分组的统计数据
	RecentActivity []Activity  `json:"recentActivity"` // 最近活动
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
	GroupBy   string `json:"groupBy" form:"groupBy"` // day, week, month
	StartDate string `json:"startDate" form:"startDate"`
	EndDate   string `json:"endDate" form:"endDate"`
}

// DouyinCardOverallStatsResponse 抖音卡片总体统计响应
type DouyinCardOverallStatsResponse struct {
	TotalCards     int           `json:"totalCards"`     // 总卡片数
	ActiveCards    int           `json:"activeCards"`    // 激活卡片数
	TotalViews     int           `json:"totalViews"`     // 总浏览数
	PopularCards   []PopularCard `json:"popularCards"`   // 热门卡片
	DailyStats     []DailyStat   `json:"dailyStats"`     // 按时间分组的统计数据
	RecentActivity []Activity    `json:"recentActivity"` // 最近活动
}

// DouyinCardTopStats 热门卡片统计
// PopularCard 热门卡片
type PopularCard struct {
	ID        uint   `json:"id"`        // 卡片ID
	Title     string `json:"title"`     // 卡片标题
	ViewCount int    `json:"viewCount"` // 浏览数
	CreatedAt string `json:"createdAt"` // 创建时间
}

// DouyinCardActivity 卡片活动记录
// Activity 活动
type Activity struct {
	ID        uint   `json:"id"`        // 活动ID
	CardID    uint   `json:"cardId"`    // 卡片ID
	Action    string `json:"action"`    // 操作类型：view
	Username  string `json:"username"`  // 用户名
	IPAddress string `json:"ipAddress"` // IP地址
	CreatedAt string `json:"createdAt"` // 创建时间
}
