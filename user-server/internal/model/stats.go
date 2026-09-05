package model

import (
	"time"
)

// APILog API调用日志
type APILog struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	LicenseID  string    `gorm:"type:varchar(36);index;comment:许可证ID" json:"license_id"`
	Endpoint   string    `gorm:"type:varchar(255);comment:API端点" json:"endpoint"`
	Method     string    `gorm:"type:varchar(10);comment:HTTP方法" json:"method"`
	StatusCode int       `gorm:"comment:响应状态码" json:"status_code"`
	Duration   int64     `gorm:"comment:响应时间(毫秒)" json:"duration"`
	IPAddress  string    `gorm:"type:varchar(45);comment:IP地址" json:"ip_address"`
	UserAgent  string    `gorm:"type:text;comment:用户代理" json:"user_agent"`
}

func (APILog) TableName() string {
	return "api_logs"
}

// VisitLog 访问日志
type VisitLog struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	LicenseID string    `gorm:"type:varchar(36);index;comment:许可证ID" json:"license_id"`
	Path      string    `gorm:"type:varchar(255);comment:访问路径" json:"path"`
	IPAddress string    `gorm:"type:varchar(45);comment:IP地址" json:"ip_address"`
	UserAgent string    `gorm:"type:text;comment:用户代理" json:"user_agent"`
	Referer   string    `gorm:"type:varchar(255);comment:来源页面" json:"referer"`
}

func (VisitLog) TableName() string {
	return "visit_logs"
}

// DailyStats 每日统计
type DailyStats struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Date            string    `gorm:"type:varchar(10);uniqueIndex;comment:日期(YYYY-MM-DD)" json:"date"`
	LicenseID       string    `gorm:"type:varchar(36);index;comment:许可证ID" json:"license_id"`
	APICalls        int64     `gorm:"default:0;comment:API调用次数" json:"api_calls"`
	Visits          int64     `gorm:"default:0;comment:访问次数" json:"visits"`
	UniqueVisitors  int64     `gorm:"default:0;comment:独立访客数" json:"unique_visitors"`
	ErrorCount      int64     `gorm:"default:0;comment:错误次数" json:"error_count"`
	AvgResponseTime int64     `gorm:"default:0;comment:平均响应时间(毫秒)" json:"avg_response_time"`
}

func (DailyStats) TableName() string {
	return "daily_stats"
}

// SystemMetrics 系统指标
type SystemMetrics struct {
	ID                uint      `gorm:"primarykey" json:"id"`
	CreatedAt         time.Time `json:"created_at"`
	CPUUsage          float64   `gorm:"comment:CPU使用率" json:"cpu_usage"`
	MemoryUsage       float64   `gorm:"comment:内存使用率" json:"memory_usage"`
	DiskUsage         float64   `gorm:"comment:磁盘使用率" json:"disk_usage"`
	NetworkIn         int64     `gorm:"comment:网络入流量(bytes)" json:"network_in"`
	NetworkOut        int64     `gorm:"comment:网络出流量(bytes)" json:"network_out"`
	ActiveConnections int64     `gorm:"comment:活跃连接数" json:"active_connections"`
	ErrorCount        int64     `gorm:"default:0;comment:错误次数" json:"error_count"`
}

func (SystemMetrics) TableName() string {
	return "system_metrics"
}
