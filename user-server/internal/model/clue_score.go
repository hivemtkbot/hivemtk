// Package model 数据层模型 - 线索评分
//
// 五层架构归属: L3 数据模型层
// 设计依据: 缺口修复 - 线索评分模型
// 私域独立部署: 无 merchant_id 字段
package model

import (
	"time"
)

// ClueScore 线索评分
// 评分维度（每项 0-100，最终加权后映射到 0-100）：
//  1. 渠道质量（25%）：来源类型（QQ/微信/电话/Telegram/Whatsapp/twitter）→ 触达成功率
//  2. 验证状态（20%）：is_verify 是否已核实
//  3. 资料完整度（20%）：name + city + address + desc 填充度
//  4. 行为参与度（25%）：互动次数、最近活跃、停留时长
//  5. 时效性（10%）：create_time 距今越近分数越高（24h 内满分，超 30 天归零）
type ClueScore struct {
	ID      uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ClueID  string `gorm:"type:varchar(36);not null;uniqueIndex" json:"clue_id"`
	Account string `gorm:"type:varchar(255);default:'';index" json:"account"`

	TotalScore int    `gorm:"type:int;not null;default:0" json:"total_score"`
	Grade      string `gorm:"type:varchar(8);not null;default:'D'" json:"grade"` 
	Confidence int    `gorm:"type:int;not null;default:0" json:"confidence"`     

	ChannelScore    int `gorm:"type:int;not null;default:0" json:"channel_score"`
	VerifyScore     int `gorm:"type:int;not null;default:0" json:"verify_score"`
	ProfileScore    int `gorm:"type:int;not null;default:0" json:"profile_score"`
	EngagementScore int `gorm:"type:int;not null;default:0" json:"engagement_score"`
	RecencyScore    int `gorm:"type:int;not null;default:0" json:"recency_score"`

	FactorsJSON string `gorm:"type:text;not null;default:'{}'" json:"factors_json"`

	ModelVersion string    `gorm:"type:varchar(16);not null;default:'h-score-1'" json:"model_version"`
	ScoredAt     time.Time `gorm:"not null;default:NOW()" json:"scored_at"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (ClueScore) TableName() string { return "clue_scores" }

// ClueEngagementEvent 线索互动事件
// 用于统计"行为参与度"维度；不强制要求
type ClueEngagementEvent struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClueID    string    `gorm:"type:varchar(36);not null;index" json:"clue_id"`
	EventType string    `gorm:"type:varchar(32);not null" json:"event_type"` 
	Channel   string    `gorm:"type:varchar(32);not null;default:''" json:"channel"`
	Payload   string    `gorm:"type:text;default:'{}'" json:"payload"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 表名
func (ClueEngagementEvent) TableName() string { return "clue_engagement_events" }

// Grade 总分到等级的映射阈值
const (
	ClueGradeS = "S" 
	ClueGradeA = "A" 
	ClueGradeB = "B" 
	ClueGradeC = "C" 
	ClueGradeD = "D" 
)

// CalcGradeFromScore 根据总分计算等级
func CalcGradeFromScore(score int) string {
	switch {
	case score >= 90:
		return ClueGradeS
	case score >= 75:
		return ClueGradeA
	case score >= 60:
		return ClueGradeB
	case score >= 40:
		return ClueGradeC
	default:
		return ClueGradeD
	}
}

