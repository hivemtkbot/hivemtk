package ragcache

import "time"

// CacheTier 缓存命中层级
type CacheTier string

const (
	// TierExact Tier1 精确键命中（query 向量逐位相等）
	TierExact CacheTier = "exact"

	// TierSemantic Tier2 语义层命中（cosine >= 阈值）
	TierSemantic CacheTier = "semantic"

	// TierMiss 未命中（调用方回源检索+生成）
	TierMiss CacheTier = "miss"
)

// DefaultSemanticThreshold Tier2 语义相似度阈值
// RT-2 契约：0.95 起步，只调紧不放松。
const DefaultSemanticThreshold = 0.95

// RAGAnswerCache rag_answer_cache 表模型（L5 数据层，纯数据结构）
type RAGAnswerCache struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	KBID          string    `gorm:"column:kb_id;type:varchar(64);not null"`
	PromptVersion string    `gorm:"column:prompt_version;type:varchar(64);not null"`
	QueryVector   string    `gorm:"column:query_vector;type:vector(1024);not null"`
	Answer        string    `gorm:"column:answer;type:text;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	KBUpdatedAt   time.Time `gorm:"column:kb_updated_at;not null"`
}

// TableName GORM 表名
func (RAGAnswerCache) TableName() string { return "rag_answer_cache" }

// Entry 缓存条目（向量以 []float32 表示，屏蔽 pgvector 序列化细节）
type Entry struct {
	ID            uint64
	KBID          string
	PromptVersion string
	QueryVector   []float32
	Answer        string
	CreatedAt     time.Time
	KBUpdatedAt   time.Time
}

// LookupRequest 缓存查询请求
type LookupRequest struct {
	KBID          string
	PromptVersion string
	QueryVector   []float32
}

// LookupResult 缓存查询结果
type LookupResult struct {
	Tier       CacheTier
	Answer     string
	Similarity float64
}
