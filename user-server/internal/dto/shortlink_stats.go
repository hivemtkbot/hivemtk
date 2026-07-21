package dto

import "time"

// ShortLinkAccessResponse 短链访问统计响应
type ShortLinkAccessResponse struct {
	ID          uint      `json:"id"`
	ShortLinkID uint      `json:"short_link_id"`
	IP          string    `json:"ip"`
	UserAgent   string    `json:"user_agent"`
	Referer     string    `json:"referer"`
	DeviceType  string    `json:"device_type"`
	Browser     string    `json:"browser"`
	OS          string    `json:"os"`
	Location    string    `json:"location"`
	AccessTime  time.Time `json:"access_time"`
}

// ShortLinkStatsRequest 短链统计请求
type ShortLinkStatsRequest struct {
	ID        uint   `json:"id" binding:"required"`        // 短链ID
	StartDate string `json:"start_date" form:"start_date"` // 开始日期 YYYY-MM-DD
	EndDate   string `json:"end_date" form:"end_date"`     // 结束日期 YYYY-MM-DD
}

// ShortLinkStatsResponse 短链统计响应
type ShortLinkStatsResponse struct {
	ShortLinkID     uint              `json:"short_link_id"`
	ShortCode       string            `json:"short_code"`
	OriginalURL     string            `json:"original_url"`
	Title           string            `json:"title"`
	TotalAccess     int64             `json:"total_access"`      // 总访问量
	TodayAccess     int64             `json:"today_access"`      // 今日访问量
	DeviceTypeStats []DeviceTypeStats `json:"device_type_stats"` // 设备类型统计
	DailyStats      []DailyStats      `json:"daily_stats"`       // 每日访问统计
}

// AllShortLinksStatsRequest 所有短链统计请求
type AllShortLinksStatsRequest struct {
	StartDate string `json:"start_date" form:"start_date"` // 开始日期 YYYY-MM-DD
	EndDate   string `json:"end_date" form:"end_date"`     // 结束日期 YYYY-MM-DD
}

// AllShortLinksStatsResponse 所有短链统计响应
type AllShortLinksStatsResponse struct {
	TotalAccess     int64                 `json:"total_access"`      // 总访问量
	TodayAccess     int64                 `json:"today_access"`      // 今日访问量
	DeviceTypeStats []DeviceTypeStats     `json:"device_type_stats"` // 设备类型统计
	DailyStats      []DailyStats          `json:"daily_stats"`       // 每日访问统计
	ShortLinkStats  []ShortLinkBasicStats `json:"short_link_stats"`  // 各短链访问统计
}

// DeviceTypeStats 设备类型统计
type DeviceTypeStats struct {
	DeviceType string  `json:"device_type"` // 设备类型
	Count      int64   `json:"count"`       // 数量
	Percentage float64 `json:"percentage"`  // 百分比
}

// DailyStats 每日统计
type DailyStats struct {
	Date  string `json:"date"`  // 日期 YYYY-MM-DD
	Count int64  `json:"count"` // 访问量
}

// ShortLinkBasicStats 短链基本统计
type ShortLinkBasicStats struct {
	ID          uint   `json:"id"`
	ShortCode   string `json:"short_code"`
	Title       string `json:"title"`
	AccessCount int64  `json:"access_count"`
}

// ShareShortLinkRequest 分享短链请求
type ShareShortLinkRequest struct {
	ID uint `json:"id" binding:"required"` // 短链ID
}

// ShareShortLinkResponse 分享短链响应
type ShareShortLinkResponse struct {
	ShortURL string `json:"short_url"` // 短链URL
	QRCode   string `json:"qr_code"`   // 二维码Base64
}
