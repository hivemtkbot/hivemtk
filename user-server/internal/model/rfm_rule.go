package model

import (
	"time"
)

// RFMRule RFM 规则模型
type RFMRule struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(100)" json:"name"`
	RDays1    int       `gorm:"default:7" json:"r_days_1"`                     
	RDays2    int       `gorm:"default:14" json:"r_days_2"`                    
	RDays3    int       `gorm:"default:30" json:"r_days_3"`                    
	RDays4    int       `gorm:"default:60" json:"r_days_4"`                    
	RDays5    int       `gorm:"default:90" json:"r_days_5"`                    
	FCount1   int       `gorm:"default:1" json:"f_count_1"`                    
	FCount2   int       `gorm:"default:3" json:"f_count_2"`                    
	FCount3   int       `gorm:"default:5" json:"f_count_3"`                    
	FCount4   int       `gorm:"default:10" json:"f_count_4"`                   
	FCount5   int       `gorm:"default:20" json:"f_count_5"`                   
	MAmount1  int64     `gorm:"type:bigint;default:10000" json:"m_amount_1"`   
	MAmount2  int64     `gorm:"type:bigint;default:50000" json:"m_amount_2"`   
	MAmount3  int64     `gorm:"type:bigint;default:100000" json:"m_amount_3"`  
	MAmount4  int64     `gorm:"type:bigint;default:500000" json:"m_amount_4"`  
	MAmount5  int64     `gorm:"type:bigint;default:1000000" json:"m_amount_5"` 
	IsActive  bool      `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (RFMRule) TableName() string {
	return "rfm_rules"
}

// UserRFM 用户 RFM 模型
type UserRFM struct {
	ID                uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID            uint       `gorm:"index;not null" json:"user_id"`
	RScore            int        `gorm:"default:0" json:"r_score"`                  
	FScore            int        `gorm:"default:0" json:"f_score"`                  
	MScore            int        `gorm:"default:0" json:"m_score"`                  
	TotalScore        int        `gorm:"default:0" json:"total_score"`              
	Layer             string     `gorm:"type:varchar(20)" json:"layer"`             
	LastTransactionAt *time.Time `json:"last_transaction_at"`                       
	TransactionCount  int        `gorm:"default:0" json:"transaction_count"`        
	TotalAmount       int64      `gorm:"type:bigint;default:0" json:"total_amount"` 
	AvgAmount         int64      `gorm:"type:bigint;default:0" json:"avg_amount"`   
	UpdatedAt         time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (UserRFM) TableName() string {
	return "user_rfms"
}

// RFMLayer 用户分层类型
type RFMLayer string

const (
	RFMLayerImportantValue   RFMLayer = "important_value"   
	RFMLayerImportantKeep    RFMLayer = "important_keep"    
	RFMLayerImportantDevelop RFMLayer = "important_develop" 
	RFMLayerImportantStay    RFMLayer = "important_stay"    
	RFMLayerGeneralValue     RFMLayer = "general_value"     
	RFMLayerGeneralKeep      RFMLayer = "general_keep"      
	RFMLayerGeneralDevelop   RFMLayer = "general_develop"   
	RFMLayerGeneralStay      RFMLayer = "general_stay"      
	RFMLayerNew              RFMLayer = "new"               
	RFMLayerSleep            RFMLayer = "sleep"             
	RFMLayerLost             RFMLayer = "lost"              
)

// GetLayerDescription 获取分层描述
func GetLayerDescription(layer RFMLayer) string {
	descriptions := map[RFMLayer]string{
		RFMLayerImportantValue:   "重要价值用户 - 最近消费、消费频次高、消费金额高",
		RFMLayerImportantKeep:    "重要保持用户 - 很久未消费、消费频次高、消费金额高",
		RFMLayerImportantDevelop: "重要发展用户 - 最近消费、消费频次低、消费金额高",
		RFMLayerImportantStay:    "重要挽留用户 - 很久未消费、消费频次低、消费金额高",
		RFMLayerGeneralValue:     "一般价值用户 - 最近消费、消费频次高、消费金额低",
		RFMLayerGeneralKeep:      "一般保持用户 - 很久未消费、消费频次高、消费金额低",
		RFMLayerGeneralDevelop:   "一般发展用户 - 最近消费、消费频次低、消费金额低",
		RFMLayerGeneralStay:      "一般挽留用户 - 很久未消费、消费频次低、消费金额低",
		RFMLayerNew:              "新用户 - 首次消费",
		RFMLayerSleep:            "沉睡用户 - 超过 60 天未消费",
		RFMLayerLost:             "流失用户 - 超过 90 天未消费",
	}
	return descriptions[layer]
}

