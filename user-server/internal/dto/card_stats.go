package dto

// PlatformCardStatsRequest 平台卡片统计请求（统一入口）
//
// LM-P2：用于替代各平台自有的 *CardStatsRequest 命名差异（douyin / kuaishou / xiaohongshu
// / xianyu / tiktok），统一通过 Platform 字段路由到具体平台 service。
type PlatformCardStatsRequest struct {
	Platform  string `json:"platform" form:"platform"`   // 平台标识：douyin/kuaishou/xiaohongshu/xianyu/tiktok
	CardID    uint   `json:"cardId" form:"cardId"`       // 卡片ID
	StartDate string `json:"startDate" form:"startDate"` // 开始日期，格式：2026-01-01
	EndDate   string `json:"endDate" form:"endDate"`     // 结束日期，格式：2026-01-31
	GroupBy   string `json:"groupBy" form:"groupBy"`     // 分组方式：day/week/month
}

// PlatformCardStatsResponse 平台卡片统计响应（统一入口）
type PlatformCardStatsResponse struct {
	Platform       string      `json:"platform"`       // 平台标识
	CardID         uint        `json:"cardId"`         // 卡片ID
	Title          string      `json:"title"`          // 卡片标题
	ViewCount      int         `json:"viewCount"`      // 总浏览数
	ClickCount     int         `json:"clickCount"`     // 总点击数（部分平台支持）
	ShareCount     int         `json:"shareCount"`     // 总分享数（部分平台支持）
	DailyStats     []DailyStat `json:"dailyStats"`     // 按时间分组的统计数据
	RecentActivity []Activity  `json:"recentActivity"` // 最近活动
}

// PlatformCardOverallStatsRequest 平台卡片总体统计请求（统一入口）
type PlatformCardOverallStatsRequest struct {
	Platform  string `json:"platform" form:"platform"`   // 平台标识
	GroupBy   string `json:"groupBy" form:"groupBy"`     // day/week/month
	StartDate string `json:"startDate" form:"startDate"` // 开始日期
	EndDate   string `json:"endDate" form:"endDate"`     // 结束日期
	Limit     int    `json:"limit" form:"limit"`         // 热门卡片返回数量
}

// PlatformCardOverallStatsResponse 平台卡片总体统计响应（统一入口）
type PlatformCardOverallStatsResponse struct {
	Platform       string        `json:"platform"`       // 平台标识
	TotalCards     int           `json:"totalCards"`     // 总卡片数
	ActiveCards    int           `json:"activeCards"`    // 激活卡片数
	TotalViews     int           `json:"totalViews"`     // 总浏览数
	TotalClicks    int           `json:"totalClicks"`    // 总点击数（部分平台支持）
	TotalShares    int           `json:"totalShares"`    // 总分享数（部分平台支持）
	PopularCards   []PopularCard `json:"popularCards"`   // 热门卡片
	DailyStats     []DailyStat   `json:"dailyStats"`     // 按时间分组的统计数据
	RecentActivity []Activity    `json:"recentActivity"` // 最近活动
}
