package model

import (
	"time"
)

// ABExperiment A/B 实验
type ABExperiment struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Name         string     `gorm:"type:varchar(100);not null" json:"name"`
	Description  string     `gorm:"type:varchar(500)" json:"description"`
	Status       string     `gorm:"type:varchar(20);default:draft" json:"status"` 
	SourceType   string     `gorm:"type:varchar(50)" json:"source_type"`          
	SourceID     string     `gorm:"type:varchar(100)" json:"source_id"`
	TrafficSplit int        `gorm:"default:50" json:"traffic_split"` 
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
	Name            string    `gorm:"type:varchar(50);not null" json:"name"` 
	IsControl       bool      `gorm:"default:false" json:"is_control"`       
	Config          string    `gorm:"type:text" json:"config"`               
	Weight          int       `gorm:"default:50" json:"weight"`              
	TrafficCount    int       `gorm:"default:0" json:"traffic_count"`        
	ConversionCount int       `gorm:"default:0" json:"conversion_count"`     
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
	EventType    string    `gorm:"type:varchar(50)" json:"event_type"`       
	EventValue   int64     `gorm:"type:bigint;default:0" json:"event_value"` 
	UserID       string    `gorm:"type:varchar(100);index" json:"user_id"`
	VariantID    uint      `gorm:"index" json:"variant_id"`
	Metadata     string    `gorm:"type:text" json:"metadata"` 
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
	ConversionRate  float64   `json:"conversion_rate"`                            
	Revenue         int64     `gorm:"type:bigint;default:0" json:"revenue"`       
	AverageValue    int64     `gorm:"type:bigint;default:0" json:"average_value"` 
	ConfidenceLevel float64   `json:"confidence_level"`                           
	IsWinner        bool      `json:"is_winner"`                                  
	UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ABExperimentResult) TableName() string {
	return "ab_experiment_results"
}

