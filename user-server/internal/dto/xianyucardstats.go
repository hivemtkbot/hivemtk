package dto

// XianyuCardStatsRequest 闲鱼卡片统计请求
type XianyuCardStatsRequest struct {
	CardID    uint   `json:"cardId" form:"cardId"`
	StartDate string `json:"startDate" form:"startDate"`
	EndDate   string `json:"endDate" form:"endDate"`
	GroupBy   string `json:"groupBy" form:"groupBy"`
}

// XianyuCardStatsResponse 闲鱼卡片统计响应
type XianyuCardStatsResponse struct {
	CardID         uint        `json:"cardId"`
	Title          string      `json:"title"`
	ViewCount      int         `json:"viewCount"`
	ClickCount     int         `json:"clickCount"`
	ShareCount     int         `json:"shareCount"`
	DailyStats     []DailyStat `json:"dailyStats"`
	RecentActivity []Activity  `json:"recentActivity"`
}

// XianyuCardOverallStatsRequest 闲鱼卡片总体统计请求
type XianyuCardOverallStatsRequest struct {
	GroupBy   string `json:"groupBy" form:"groupBy"`
	StartDate string `json:"startDate" form:"startDate"`
	EndDate   string `json:"endDate" form:"endDate"`
}

// XianyuCardOverallStatsResponse 闲鱼卡片总体统计响应
type XianyuCardOverallStatsResponse struct {
	TotalCards     int           `json:"totalCards"`
	ActiveCards    int           `json:"activeCards"`
	TotalViews     int           `json:"totalViews"`
	TotalClicks    int           `json:"totalClicks"`
	TotalShares    int           `json:"totalShares"`
	PopularCards   []PopularCard `json:"popularCards"`
	DailyStats     []DailyStat   `json:"dailyStats"`
	RecentActivity []Activity    `json:"recentActivity"`
}

// CardStatsData 卡片统计数据
type CardStatsData struct {
	CardID       uint          `json:"cardId"`
	Title        string        `json:"title"`
	Views        int           `json:"views"`
	Clicks       int           `json:"clicks"`
	Shares       int           `json:"shares"`
	LastActivity string        `json:"lastActivity"`
	StatsByDate  []StatsByDate `json:"statsByDate"`
}

// OverallStatsData 总体统计数据
type OverallStatsData struct {
	TotalViewCount  int           `json:"totalViewCount"`
	TotalClickCount int           `json:"totalClickCount"`
	TotalShareCount int           `json:"totalShareCount"`
	TotalCards      int64         `json:"totalCards"`
	ActiveCards     int64         `json:"activeCards"`
	StatsByDate     []StatsByDate `json:"statsByDate"`
	TopCards        []TopCard     `json:"topCards"`
}

// StatsByDate 按日期统计数据
type StatsByDate struct {
	Date   string `json:"date"`
	Views  int    `json:"views"`
	Clicks int    `json:"clicks"`
	Shares int    `json:"shares"`
}

// TopCard 热门卡片
type TopCard struct {
	ID        uint   `json:"id"`
	Title     string `json:"title"`
	ViewCount int    `json:"viewCount"`
	CreatedAt string `json:"createdAt"`
}
