package model


import "time"

// RagQueryLog RAG 查询日志（每次检索一条）
//
// 写入时机：检索完成后异步写入（service.RecordQuery）
// 用途：
//   - 计算召回率/精确率均值
//   - 查询低召回率样本（用于调优）
//
// 延迟计算
// Top-K / Top-1 命中率、平均相似度（新增于）
type RagQueryLog struct {
	ID              int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Query           string    `gorm:"column:query;type:text;not null" json:"query"`
	QueryHash       string    `gorm:"column:query_hash;size:64;index" json:"query_hash"`
	SessionID       string    `gorm:"column:session_id;size:128;index" json:"session_id"`
	ProductID       string    `gorm:"column:product_id;index;default:''" json:"product_id"`
	RetrievedDocIDs string    `gorm:"column:retrieved_doc_ids;type:text" json:"retrieved_doc_ids"` 
	RelevantDocIDs  string    `gorm:"column:relevant_doc_ids;type:text" json:"relevant_doc_ids"`   
	RetrievedCount  int       `gorm:"column:retrieved_count;default:0" json:"retrieved_count"`
	RelevantCount   int       `gorm:"column:relevant_count;default:0" json:"relevant_count"`
	HitCount        int       `gorm:"column:hit_count;default:0" json:"hit_count"`                                                                    
	Top1DocID       string    `gorm:"column:top1_doc_id;size:128;index" json:"top1_doc_id"`                                                           
	HitInTop1       bool      `gorm:"column:hit_in_top1;default:false;index:idx_rag_query_logs_hit_top1,where:hit_in_top1 = TRUE" json:"hit_in_top1"` 
	TopSimilarity   float64   `gorm:"column:top_similarity;type:decimal(10,6);default:0;index:idx_rag_query_logs_top_sim" json:"top_similarity"`      
	Precision       float64   `gorm:"column:precision;type:decimal(6,4);default:0" json:"precision"`                                                  
	Recall          float64   `gorm:"column:recall;type:decimal(6,4);default:0" json:"recall"`                                                        
	LatencyMs       int64     `gorm:"column:latency_ms;default:0" json:"latency_ms"`
	TopK            int       `gorm:"column:top_k;default:5" json:"top_k"`
	Source          string    `gorm:"column:source;size:32;default:'hybrid'" json:"source"` 
	CreatedAt       time.Time `gorm:"column:created_at;not null;default:now();index:idx_rag_query_logs_created" json:"created_at"`
}

// TableName 表名
func (RagQueryLog) TableName() string { return "rag_query_logs" }

// RagMetricsDaily RAG 召回指标聚合（按 5 分钟窗口）
//
// 由后台 cron 任务每 5 分钟聚合 rag_query_logs 写入
// 用途：
//   - 召回率趋势可视化
//   - 触发风控预警（recall < 0.3 等）
type RagMetricsDaily struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	WindowStart    time.Time `gorm:"column:window_start;not null;index:idx_rag_metrics_daily_window,priority:1" json:"window_start"`
	WindowEnd      time.Time `gorm:"column:window_end;not null" json:"window_end"`
	TotalQueries   int64     `gorm:"column:total_queries;default:0" json:"total_queries"`
	AvgRecall      float64   `gorm:"column:avg_recall;type:decimal(6,4);default:0" json:"avg_recall"`
	AvgPrecision   float64   `gorm:"column:avg_precision;type:decimal(6,4);default:0" json:"avg_precision"`
	AvgLatencyMs   float64   `gorm:"column:avg_latency_ms;type:decimal(10,2);default:0" json:"avg_latency_ms"`
	P99LatencyMs   int64     `gorm:"column:p99_latency_ms;default:0" json:"p99_latency_ms"`
	ZeroHitCount   int64     `gorm:"column:zero_hit_count;default:0" json:"zero_hit_count"`     
	LowRecallCount int64     `gorm:"column:low_recall_count;default:0" json:"low_recall_count"` 
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:now();index:idx_rag_metrics_daily_window,priority:2" json:"created_at"`
}

// TableName 表名
func (RagMetricsDaily) TableName() string { return "rag_metrics_daily" }

