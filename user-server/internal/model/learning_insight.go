package model

import "time"

// LearningInsight 经验沉淀洞察（L-2 Sleep-time 轻量版）
//
// trace_learning 批处理对 Bad(<60) 样本提取「错误模式」一句话后落库；
// SalesEngine 组 prompt 时按行业注入 top3，避免重蹈覆辙。
//
// 多租户说明：当前为本地单商户部署，MerchantID 为预留列（恒 0），
// 行业隔离靠 Industry 列（部署时经 trace_learning.Config.Industry 配置）。
type LearningInsight struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	MerchantID    uint      `gorm:"not null;default:0;index" json:"merchant_id"`
	Industry      string    `gorm:"type:varchar(64);index" json:"industry"`
	InsightText   string    `gorm:"type:text;not null" json:"insight_text"`
	SourceTraceID string    `gorm:"type:varchar(64);uniqueIndex:idx_li_source_trace" json:"source_trace_id"`
	CreatedAt     time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 表名
func (LearningInsight) TableName() string { return "learning_insights" }
