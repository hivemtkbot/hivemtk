package model

import "time"

// BatchOperationHistory 批量操作历史记录
type BatchOperationHistory struct {
	ID            uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	OperationType string     `gorm:"type:varchar(50);not null" json:"operation_type"` 
	DataType      string     `gorm:"type:varchar(50)" json:"data_type"`               
	Status        string     `gorm:"type:varchar(20);default:pending" json:"status"`  
	TotalCount    int        `gorm:"default:0" json:"total_count"`
	SuccessCount  int        `gorm:"default:0" json:"success_count"`
	FailedCount   int        `gorm:"default:0" json:"failed_count"`
	OperatorID    uint       `json:"operator_id"`
	ErrorMessage  string     `gorm:"type:text" json:"error_message"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (BatchOperationHistory) TableName() string {
	return "batch_operation_histories"
}

