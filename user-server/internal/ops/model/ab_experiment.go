package model

import (
	"time"
)

// ABExperiment A/B 实验
type ABExperiment struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Name         string     `gorm:"type:varchar(100);not null" json:"name"`
	Description  string     `gorm:"type:varchar(500)" json:"description"`
	Status       string     `gorm:"type:varchar(20);default:draft" json:"status"` // draft, running, paused, completed
	SourceType   string     `gorm:"type:varchar(50)" json:"source_type"`          // page, component, message, etc.
	SourceID     string     `gorm:"type:varchar(100)" json:"source_id"`
	TrafficSplit int        `gorm:"default:50" json:"traffic_split"` // 流量分配比例 (0-100)
	StartDate    *time.Time `json:"start_date"`
	EndDate      *time.Time `json:"end_date"`
	CreatedBy    uint       `json:"created_by"`
	CreatedAt    time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ABExperiment) TableName() string {
	return "ab_experiments"
}

// ABVariant A/B 实验变体
type ABVariant struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ExperimentID    uint      `gorm:"index;not null" json:"experiment_id"`
	Name            string    `gorm:"type:varchar(50);not null" json:"name"` // A, B, C...
	IsControl       bool      `gorm:"default:false" json:"is_control"`       // 是否为对照组
	Config          string    `gorm:"type:text" json:"config"`               // JSON 配置
	Weight          int       `gorm:"default:50" json:"weight"`              // 权重
	TrafficCount    int       `gorm:"default:0" json:"traffic_count"`        // 访问人数
	ConversionCount int       `gorm:"default:0" json:"conversion_count"`     // 转化人数
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ABVariant) TableName() string {
	return "ab_variants"
}

// ABConversionEvent 转化事件
type ABConversionEvent struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ExperimentID uint      `gorm:"index;not null" json:"experiment_id"`
	EventName    string    `gorm:"type:varchar(100);not null" json:"event_name"`
	EventType    string    `gorm:"type:varchar(50)" json:"event_type"`       // click, purchase, signup, etc.
	EventValue   int64     `gorm:"type:bigint;default:0" json:"event_value"` // 事件值（分），如购买金额
	UserID       string    `gorm:"type:varchar(100);index" json:"user_id"`
	VariantID    uint      `gorm:"index" json:"variant_id"`
	Metadata     string    `gorm:"type:text" json:"metadata"` // JSON 元数据
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (ABConversionEvent) TableName() string {
	return "ab_conversion_events"
}

// ABExperimentResult 实验结果统计
type ABExperimentResult struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ExperimentID    uint      `gorm:"uniqueIndex;not null" json:"experiment_id"`
	VariantID       uint      `gorm:"index;not null" json:"variant_id"`
	VariantName     string    `gorm:"type:varchar(50)" json:"variant_name"`
	IsControl       bool      `json:"is_control"`
	TrafficCount    int       `json:"traffic_count"`
	ConversionCount int       `json:"conversion_count"`
	ConversionRate  float64   `json:"conversion_rate"`                            // 转化率
	Revenue         int64     `gorm:"type:bigint;default:0" json:"revenue"`       // 总收入（分）
	AverageValue    int64     `gorm:"type:bigint;default:0" json:"average_value"` // 平均价值（分）
	ConfidenceLevel float64   `json:"confidence_level"`                           // 置信度
	IsWinner        bool      `json:"is_winner"`                                  // 是否为优胜者
	UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ABExperimentResult) TableName() string {
	return "ab_experiment_results"
}
