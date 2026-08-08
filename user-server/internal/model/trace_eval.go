package model

import "time"

// TraceEvalLog 追踪自学习打分记录
//
// 每条被评估的 trace 一条记录（trace_id 唯一，重评估覆盖）。记录 LLM 对完整
// 请求-响应链的打分，以及据此调整的涉及知识库 chunk 权重，便于审计与前端展示。
//
// 与 knowledge_chunks.weight 的配合：
//   - 本表是「评估审计日志」（不可变历史）；
//   - knowledge_chunks.weight 是「运行时权重」（被本模块按打分动态调整），
//     作为检索排名的第二依据（见 RagSearcher.rankRAGChunks）。
type TraceEvalLog struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TraceID        string    `gorm:"type:varchar(64);uniqueIndex:idx_tel_trace" json:"trace_id"`
	ConversationID string    `gorm:"type:varchar(100);index" json:"conversation_id"`
	Channel        string    `gorm:"type:varchar(30);index" json:"channel"`
	Score          int       `gorm:"not null;default:0" json:"score"`
	DimensionsJSON string    `gorm:"type:jsonb;default:'{}'" json:"dimensions"`
	Reason         string    `gorm:"type:text" json:"reason"`
	Bad            bool      `gorm:"not null;default:false" json:"bad"`
	// AdjustedChunks 本次调整的 chunk 权重明细：[{"id":,"old":,"new":}]
	AdjustedChunks string `gorm:"type:jsonb;default:'[]'" json:"adjusted_chunks"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 指定审计日志表名
func (TraceEvalLog) TableName() string { return "trace_eval_log" }
