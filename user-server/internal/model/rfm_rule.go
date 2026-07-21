package model

import (
	"time"
)

// RFMRule RFM 规则模型
type RFMRule struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(100)" json:"name"`
	RDays1    int       `gorm:"default:7" json:"r_days_1"`                     // R 值<=7 天，得 5 分
	RDays2    int       `gorm:"default:14" json:"r_days_2"`                    // R 值<=14 天，得 4 分
	RDays3    int       `gorm:"default:30" json:"r_days_3"`                    // R 值<=30 天，得 3 分
	RDays4    int       `gorm:"default:60" json:"r_days_4"`                    // R 值<=60 天，得 2 分
	RDays5    int       `gorm:"default:90" json:"r_days_5"`                    // R 值<=90 天，得 1 分
	FCount1   int       `gorm:"default:1" json:"f_count_1"`                    // 消费次数>=1，得 1 分
	FCount2   int       `gorm:"default:3" json:"f_count_2"`                    // 消费次数>=3，得 2 分
	FCount3   int       `gorm:"default:5" json:"f_count_3"`                    // 消费次数>=5，得 3 分
	FCount4   int       `gorm:"default:10" json:"f_count_4"`                   // 消费次数>=10，得 4 分
	FCount5   int       `gorm:"default:20" json:"f_count_5"`                   // 消费次数>=20，得 5 分
	MAmount1  int64     `gorm:"type:bigint;default:10000" json:"m_amount_1"`   // 消费金额>=100元（10000分），得 1 分
	MAmount2  int64     `gorm:"type:bigint;default:50000" json:"m_amount_2"`   // 消费金额>=500元（50000分），得 2 分
	MAmount3  int64     `gorm:"type:bigint;default:100000" json:"m_amount_3"`  // 消费金额>=1000元（100000分），得 3 分
	MAmount4  int64     `gorm:"type:bigint;default:500000" json:"m_amount_4"`  // 消费金额>=5000元（500000分），得 4 分
	MAmount5  int64     `gorm:"type:bigint;default:1000000" json:"m_amount_5"` // 消费金额>=10000元（1000000分），得 5 分
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
	RScore            int        `gorm:"default:0" json:"r_score"`                  // R 得分 1-5
	FScore            int        `gorm:"default:0" json:"f_score"`                  // F 得分 1-5
	MScore            int        `gorm:"default:0" json:"m_score"`                  // M 得分 1-5
	TotalScore        int        `gorm:"default:0" json:"total_score"`              // 总分 3-15
	Layer             string     `gorm:"type:varchar(20)" json:"layer"`             // 用户分层
	LastTransactionAt *time.Time `json:"last_transaction_at"`                       // 最后交易时间
	TransactionCount  int        `gorm:"default:0" json:"transaction_count"`        // 交易次数
	TotalAmount       int64      `gorm:"type:bigint;default:0" json:"total_amount"` // 总消费金额（分）
	AvgAmount         int64      `gorm:"type:bigint;default:0" json:"avg_amount"`   // 平均消费金额（分）
	UpdatedAt         time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (UserRFM) TableName() string {
	return "user_rfms"
}

// RFMLayer 用户分层类型
type RFMLayer string

const (
	RFMLayerImportantValue   RFMLayer = "important_value"   // 重要价值用户 (R 高 F 高 M 高)
	RFMLayerImportantKeep    RFMLayer = "important_keep"    // 重要保持用户 (R 低 F 高 M 高)
	RFMLayerImportantDevelop RFMLayer = "important_develop" // 重要发展用户 (R 高 F 低 M 高)
	RFMLayerImportantStay    RFMLayer = "important_stay"    // 重要挽留用户 (R 低 F 低 M 高)
	RFMLayerGeneralValue     RFMLayer = "general_value"     // 一般价值用户 (R 高 F 高 M 低)
	RFMLayerGeneralKeep      RFMLayer = "general_keep"      // 一般保持用户 (R 低 F 高 M 低)
	RFMLayerGeneralDevelop   RFMLayer = "general_develop"   // 一般发展用户 (R 高 F 低 M 低)
	RFMLayerGeneralStay      RFMLayer = "general_stay"      // 一般挽留用户 (R 低 F 低 M 低)
	RFMLayerNew              RFMLayer = "new"               // 新用户
	RFMLayerSleep            RFMLayer = "sleep"             // 沉睡用户
	RFMLayerLost             RFMLayer = "lost"              // 流失用户
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
