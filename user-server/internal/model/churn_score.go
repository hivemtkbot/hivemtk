package model

import "time"

// ChurnScore D22b：BG/NBD 流失评分（周批产出，customer_key 唯一 upsert）
//
// 输入统计量 (x, tx, T) 与输出 (p_alive, expected_purchases_30d) 同行落库，
// params 快照保证分数可复现。表结构见 migration v3.34.0。
type ChurnScore struct {
	ID                   int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CustomerKey          string    `gorm:"type:varchar(120);uniqueIndex:uk_churn_customer" json:"customer_key"`
	X                    int       `gorm:"not null;default:0" json:"x"`
	Tx                   float64   `gorm:"not null;default:0" json:"tx"`
	TObs                 float64   `gorm:"column:t_obs;not null;default:0" json:"t_obs"`
	PAlive               float64   `gorm:"not null;default:0" json:"p_alive"`
	ExpectedPurchases30d float64   `gorm:"column:expected_purchases_30d;not null;default:0" json:"expected_purchases_30d"`
	Params               string    `gorm:"type:jsonb;not null;default:'{}'" json:"params"`
	StatsCount           int       `gorm:"not null;default:0" json:"stats_count"`
	ComputedAt           time.Time `gorm:"not null" json:"computed_at"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (ChurnScore) TableName() string { return "churn_scores" }
