package model

import "time"

// BanditRefluxLog 奖励回流台账（D04）
//
// 记录 feedback_events → BanditAllocator.UpdateReward 的每一笔回流，
// 双唯一索引保证跨进程防重复入账：
//   - event_id：同一反馈事件只回流一次（worker 重启/重扫幂等）；
//   - (session_id, sop_id, signal_key)：同会话同 SOP 的 conversion 只入账一次
//     （决策 D04"转化去重"防线；同索引也覆盖 complaint/transfer/champion_mark，
//     语义为"同会话同 SOP 同类信号只采纳首次"）。
type BanditRefluxLog struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	EventID      string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_reflux_event" json:"event_id"`
	ExperimentID string    `gorm:"type:varchar(64);not null;default:'';index:idx_reflux_log_experiment,priority:1" json:"experiment_id"`
	ArmKey       string    `gorm:"type:varchar(100);not null;default:''" json:"arm_key"`
	SignalKey    string    `gorm:"type:varchar(50);not null;default:'';uniqueIndex:uk_reflux_conversion,priority:3" json:"signal_key"`
	SessionID    string    `gorm:"type:varchar(120);not null;default:'';uniqueIndex:uk_reflux_conversion,priority:1" json:"session_id"`
	SOPID        uint      `gorm:"not null;default:0;uniqueIndex:uk_reflux_conversion,priority:2" json:"sop_id"`
	Reward       float64   `gorm:"type:decimal(6,3);not null;default:0" json:"reward"`
	Success      bool      `gorm:"not null;default:false" json:"success"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index:idx_reflux_log_experiment,priority:2" json:"created_at"`
}

// TableName 表名
func (BanditRefluxLog) TableName() string { return "bandit_reflux_log" }
