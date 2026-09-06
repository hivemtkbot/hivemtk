package model

import "time"

// MsgHourlySummary message_hub 小时级增量汇总表（M18 表 D-3）。
//
// 决策源：docs/architecture/MASTER_COMPETITIVE_DECISIONS.md M18 表 D-3 / X-8。
// 由 watermark 增量批处理维护（见 service.MessageHubSummaryAggregationService），
// 驾驶舱 SSE 优先读本表（容忍分钟级陈旧），summary 陈旧 >10min 时回源 message_hub
// 原生 SQL 聚合（双读策略，X-8）。
//
// 维度键：hour_bucket + merchant_id + platform（PK，唯一索引 uni_mhs_bucket_dim）。
// merchant_id 为多租户预留列（私域单商户恒 0），与 learning_insight 等模型口径一致。
type MsgHourlySummary struct {
	HourBucket   time.Time `gorm:"type:timestamptz;not null;uniqueIndex:uni_mhs_bucket_dim,priority:1" json:"hour_bucket"`
	MerchantID   uint      `gorm:"not null;default:0;uniqueIndex:uni_mhs_bucket_dim,priority:2" json:"merchant_id"`
	Platform     string    `gorm:"type:varchar(30);not null;default:'';uniqueIndex:uni_mhs_bucket_dim,priority:3" json:"platform"`
	SessionCount int64     `gorm:"not null;default:0" json:"session_count"`
	AICount      int64     `gorm:"not null;default:0" json:"ai_count"`
	HumanCount   int64     `gorm:"not null;default:0" json:"human_count"`
	MessageCount int64     `gorm:"not null;default:0" json:"message_count"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (MsgHourlySummary) TableName() string { return "msg_hourly_summary" }

// AggregationWatermark 增量聚合水位线（D-3，trace_learning 式模式复用）。
// source 为数据源标识（当前仅 "message_hub"）；last_event_id 记录已消费的最大源表 id，
// 与增量 upsert 同事务推进，保证重启/重跑不丢不重。
type AggregationWatermark struct {
	Source      string    `gorm:"type:varchar(60);primaryKey" json:"source"`
	LastEventID int64     `gorm:"not null;default:0" json:"last_event_id"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AggregationWatermark) TableName() string { return "aggregation_watermarks" }

// SummarySourceMessageHub watermark source 常量：message_hub 小时汇总
const SummarySourceMessageHub = "message_hub"
