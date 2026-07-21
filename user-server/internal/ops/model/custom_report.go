package model

import (
	"time"
)

// CustomReport 自定义报表模型
type CustomReport struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Description string    `gorm:"type:varchar(500)" json:"description"`
	DataSource  string    `gorm:"type:varchar(50);not null" json:"data_source"` // 数据源：sessions, messages, orders, clues, etc.
	Dimensions  string    `gorm:"type:text" json:"dimensions"`                  // 维度配置 (JSON)
	Metrics     string    `gorm:"type:text" json:"metrics"`                     // 指标配置 (JSON)
	Filters     string    `gorm:"type:text" json:"filters"`                     // 筛选条件 (JSON)
	ChartType   string    `gorm:"type:varchar(20)" json:"chart_type"`           // 图表类型：table, line, bar, pie, area
	ChartConfig string    `gorm:"type:text" json:"chart_config"`                // 图表配置 (JSON)
	IsPublic    bool      `gorm:"default:false" json:"is_public"`               // 是否公开
	CreatedBy   uint      `json:"created_by"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (CustomReport) TableName() string {
	return "custom_reports"
}

// ReportDataSource 报表数据源类型
type ReportDataSource string

const (
	DataSourceSessions ReportDataSource = "sessions" // 会话数据
	DataSourceMessages ReportDataSource = "messages" // 消息数据
	DataSourceOrders   ReportDataSource = "orders"   // 订单数据
	DataSourceClues    ReportDataSource = "clues"    // 线索数据
	DataSourceUsers    ReportDataSource = "users"    // 用户数据
	DataSourceRFM      ReportDataSource = "rfm"      // RFM 数据
	DataSourceAgents   ReportDataSource = "agents"   // 客服数据
)

// ChartType 图表类型
type ChartType string

const (
	ChartTypeTable ChartType = "table" // 表格
	ChartTypeLine  ChartType = "line"  // 折线图
	ChartTypeBar   ChartType = "bar"   // 柱状图
	ChartTypePie   ChartType = "pie"   // 饼图
	ChartTypeArea  ChartType = "area"  // 面积图
	ChartTypeCard  ChartType = "card"  // 卡片
)

// ReportDimension 报表维度
type ReportDimension struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	DataType string `json:"data_type"` // date, string, number
	GroupBy  bool   `json:"group_by"`
}

// ReportMetric 报表指标
type ReportMetric struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	AggFunc  string `json:"agg_func"`  // count, sum, avg, min, max
	DataType string `json:"data_type"` // number, percent
}

// ReportFilter 报表筛选条件
type ReportFilter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"` // eq, ne, gt, gte, lt, lte, in, like, between
	Value    any    `json:"value"`
}

// ReportData 报表数据
type ReportData struct {
	Dimensions []string         `json:"dimensions"`
	Metrics    []string         `json:"metrics"`
	Data       []map[string]any `json:"data"`
	Total      int64            `json:"total"`
}

// GetDataSourceDescription 获取数据源描述
func GetDataSourceDescription(source ReportDataSource) string {
	descriptions := map[ReportDataSource]string{
		DataSourceSessions: "会话数据 - 客服会话相关统计",
		DataSourceMessages: "消息数据 - 消息发送相关统计",
		DataSourceOrders:   "订单数据 - 订单交易相关统计",
		DataSourceClues:    "线索数据 - 销售线索相关统计",
		DataSourceUsers:    "用户数据 - 用户相关统计",
		DataSourceRFM:      "RFM 数据 - 用户分层相关统计",
		DataSourceAgents:   "客服数据 - 客服绩效相关统计",
	}
	return descriptions[source]
}

// GetChartTypeDescription 获取图表类型描述
func GetChartTypeDescription(chartType ChartType) string {
	descriptions := map[ChartType]string{
		ChartTypeTable: "表格 - 详细数据列表",
		ChartTypeLine:  "折线图 - 趋势分析",
		ChartTypeBar:   "柱状图 - 对比分析",
		ChartTypePie:   "饼图 - 占比分析",
		ChartTypeArea:  "面积图 - 趋势 + 占比",
		ChartTypeCard:  "卡片 - 关键指标",
	}
	return descriptions[chartType]
}
