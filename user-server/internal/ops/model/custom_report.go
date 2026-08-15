package model

import (
	"time"
)

// CustomReport 自定义报表模型
type CustomReport struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Description string    `gorm:"type:varchar(500)" json:"description"`
	DataSource  string    `gorm:"type:varchar(50);not null" json:"data_source"` 
	Dimensions  string    `gorm:"type:text" json:"dimensions"`                  
	Metrics     string    `gorm:"type:text" json:"metrics"`                     
	Filters     string    `gorm:"type:text" json:"filters"`                     
	ChartType   string    `gorm:"type:varchar(20)" json:"chart_type"`           
	ChartConfig string    `gorm:"type:text" json:"chart_config"`                
	IsPublic    bool      `gorm:"default:false" json:"is_public"`               
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
	DataSourceSessions ReportDataSource = "sessions" 
	DataSourceMessages ReportDataSource = "messages" 
	DataSourceOrders   ReportDataSource = "orders"   
	DataSourceClues    ReportDataSource = "clues"    
	DataSourceUsers    ReportDataSource = "users"    
	DataSourceRFM      ReportDataSource = "rfm"      
	DataSourceAgents   ReportDataSource = "agents"   
)

// ChartType 图表类型
type ChartType string

const (
	ChartTypeTable ChartType = "table" 
	ChartTypeLine  ChartType = "line"  
	ChartTypeBar   ChartType = "bar"   
	ChartTypePie   ChartType = "pie"   
	ChartTypeArea  ChartType = "area"  
	ChartTypeCard  ChartType = "card"  
)

// ReportDimension 报表维度
type ReportDimension struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	DataType string `json:"data_type"` 
	GroupBy  bool   `json:"group_by"`
}

// ReportMetric 报表指标
type ReportMetric struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	AggFunc  string `json:"agg_func"`  
	DataType string `json:"data_type"` 
}

// ReportFilter 报表筛选条件
type ReportFilter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"` 
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

