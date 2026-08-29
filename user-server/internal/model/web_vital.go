package model

import "time"

// WebVitalRecord 前端性能指标（Web Vitals: CLS/FID/LCP/FCP/TTFB）
type WebVitalRecord struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Metric    string    `gorm:"type:varchar(16);index;not null" json:"metric"`
	Value     float64   `json:"value"`
	Rating    string    `gorm:"type:varchar(16)" json:"rating"`
	Page      string    `gorm:"type:varchar(300)" json:"page"`
	SessionID string    `gorm:"type:varchar(64);index" json:"session_id"`
	UserAgent string    `gorm:"type:varchar(300)" json:"user_agent"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (WebVitalRecord) TableName() string { return "web_vital_records" }

// RagEvalQuestion RAG 评测集条目（R44 断链清欠）
type RagEvalQuestion struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ProductID string    `gorm:"type:varchar(64);index" json:"product_id"`
	Question  string    `gorm:"type:text;not null" json:"question"`
	Answer    string    `gorm:"type:text" json:"answer"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (RagEvalQuestion) TableName() string { return "rag_eval_questions" }

// RagEvalRun 一次 RAG 评测运行
type RagEvalRun struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Total       int       `json:"total"`
	Hit         int       `json:"hit"`
	Recall5     float64   `json:"recall5"`
	MRR         float64   `json:"mrr"`
	NDCG5       float64   `json:"ndcg5"`
	EvalSetSize int       `json:"eval_set_size"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (RagEvalRun) TableName() string { return "rag_eval_runs" }
