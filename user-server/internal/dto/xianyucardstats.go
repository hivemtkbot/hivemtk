package dto

// XianyuCardStatsRequest 咸鱼卡片统计请求
type XianyuCardStatsRequest struct {
	CardID    uint   `json:"cardId" form:"cardId"`       // 卡片ID
	StartDate string `json:"startDate" form:"startDate"` // 开始日期，格式：
	EndDate   string `json:"endDate" form:"endDate"`     // 结束日期，格式：
	GroupBy   string `json:"groupBy" form:"groupBy"`     // 分组方式：day, week, month
}

// XianyuCardStatsResponse 咸鱼卡片统计响应
type XianyuCardStatsResponse struct {
	CardID         uint        `json:"cardId"`         // 卡片ID
	Title          string      `json:"title"`          // 卡片标题
	ViewCount      int         `json:"viewCount"`      // 总浏览数
	ClickCount     int         `json:"clickCount"`     // 总点击数
	ShareCount     int         `json:"shareCount"`     // 总分享数
	DailyStats     []DailyStat `json:"dailyStats"`     // 按时间分组的统计数据
	RecentActivity []Activity  `json:"recentActivity"` // 最近活动
}

// XianyuCardOverallStatsRequest 咸鱼卡片总体统计请求
type XianyuCardOverallStatsRequest struct {
	GroupBy   string `json:"groupBy" form:"groupBy"` // day, week, month
	StartDate string `json:"startDate" form:"startDate"`
	EndDate   string `json:"endDate" form:"endDate"`
}

// XianyuCardOverallStatsResponse 咸鱼卡片总体统计响应
type XianyuCardOverallStatsResponse struct {
	TotalCards     int           `json:"totalCards"`     // 总卡片数
	ActiveCards    int           `json:"activeCards"`    // 激活卡片数
	TotalViews     int           `json:"totalViews"`     // 总浏览数
	TotalClicks    int           `json:"totalClicks"`    // 总点击数
	TotalShares    int           `json:"totalShares"`    // 总分享数
	PopularCards   []PopularCard `json:"popularCards"`   // 热门卡片
	DailyStats     []DailyStat   `json:"dailyStats"`     // 按时间分组的统计数据
	RecentActivity []Activity    `json:"recentActivity"` // 最近活动
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
