package model

import (
	"time"
)

// ChurnPrediction 流失预测
type ChurnPrediction struct {
	ID                uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID            string     `gorm:"type:varchar(100);index;not null" json:"user_id"`
	ChurnScore        float64    `gorm:"type:decimal(10,2)" json:"churn_score"` // 流失分数 0-100
	ChurnRisk         string     `gorm:"type:varchar(20)" json:"churn_risk"`    // low, medium, high, critical
	RiskFactors       string     `gorm:"type:text" json:"risk_factors"`         // JSON 数组，风险因素
	LastActivityAt    *time.Time `json:"last_activity_at"`                      // 最后活跃时间
	LastPurchaseAt    *time.Time `json:"last_purchase_at"`                      // 最后购买时间
	DaysSinceActive   int        `json:"days_since_active"`                     // 未活跃天数
	PurchaseFreq      float64    `json:"purchase_freq"`                         // 购买频率
	AverageOrderValue float64    `gorm:"type:decimal(10,2)" json:"average_order_value"`
	PredictedAt       time.Time  `gorm:"autoCreateTime" json:"predicted_at"`
	CreatedAt         time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ChurnPrediction) TableName() string {
	return "churn_predictions"
}

// ChurnWarning 流失预警
type ChurnWarning struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       string     `gorm:"type:varchar(100);index;not null" json:"user_id"`
	WarningLevel string     `gorm:"type:varchar(20)" json:"warning_level"` // low, medium, high, critical
	WarningType  string     `gorm:"type:varchar(50)" json:"warning_type"`  // inactive_days, purchase_drop, frequency_drop, etc.
	Description  string     `gorm:"type:varchar(500)" json:"description"`
	Suggestion   string     `gorm:"type:text" json:"suggestion"` // 建议措施
	IsHandled    bool       `gorm:"default:false" json:"is_handled"`
	HandledAt    *time.Time `json:"handled_at"`
	HandledBy    uint       `json:"handled_by"`
	HandledNote  string     `gorm:"type:text" json:"handled_note"`
	CreatedAt    time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ChurnWarning) TableName() string {
	return "churn_warnings"
}

// ChurnModelConfig 流失模型配置
type ChurnModelConfig struct {
	ID                 uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	InactiveDaysWeight float64    `gorm:"type:decimal(5,2);default:0.3" json:"inactive_days_weight"` // 未活跃天数权重
	PurchaseFreqWeight float64    `gorm:"type:decimal(5,2);default:0.3" json:"purchase_freq_weight"` // 购买频率权重
	OrderValueWeight   float64    `gorm:"type:decimal(5,2);default:0.2" json:"order_value_weight"`   // 订单金额权重
	EngagementWeight   float64    `gorm:"type:decimal(5,2);default:0.2" json:"engagement_weight"`    // 互动频率权重
	InactiveThreshold  int        `gorm:"default:30" json:"inactive_threshold"`                      // 未活跃阈值（天）
	PurchaseThreshold  int        `gorm:"default:60" json:"purchase_threshold"`                      // 未购买阈值（天）
	HighRiskScore      float64    `gorm:"type:decimal(5,2);default:70" json:"high_risk_score"`       // 高风险分数
	CriticalRiskScore  float64    `gorm:"type:decimal(5,2);default:85" json:"critical_risk_score"`   // 极高风险分数
	LastCalculatedAt   *time.Time `json:"last_calculated_at"`
	CreatedAt          time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ChurnModelConfig) TableName() string {
	return "churn_model_configs"
}

// ChurnStatistics 流失统计
type ChurnStatistics struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	StatDate       string    `gorm:"type:varchar(20);index;not null" json:"stat_date"` // YYYY-MM-DD
	TotalUsers     int       `json:"total_users"`
	ChurnUsers     int       `json:"churn_users"`
	ChurnRate      float64   `gorm:"type:decimal(10,2)" json:"churn_rate"`
	HighRiskUsers  int       `json:"high_risk_users"`
	CriticalUsers  int       `json:"critical_users"`
	RecoveredUsers int       `json:"recovered_users"` // 挽回用户数
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (ChurnStatistics) TableName() string {
	return "churn_statistics"
}
