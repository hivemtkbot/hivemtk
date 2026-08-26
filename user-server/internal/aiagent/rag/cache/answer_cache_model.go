// Package ragcache RAG 答案语义缓存（M6 R-2，RT-2 契约约束版）
//
// 三层结构：
//   - Tier1 精确键：同 kb_id + prompt_version + 归一化 query 向量精确相等（~1ms）
//   - Tier2 语义层：pgvector cosine >= 0.95（只调紧不放松）
//   - Tier3 回源：前两层未命中走正常检索+生成链路
//
// RT-2 契约（MASTER_COMPETITIVE_DECISIONS.md）：
//   - 只对 smart_cs FAQ 场景启用；答案必须来自知识库检索且不含客户个性化变量才可入缓存
//   - 缓存 key = (kb_id, prompt_version, 归一化 query 向量)
//   - 命中前校验 kb 更新时间 > 缓存时间则失效
//   - 空/refusal 响应不入缓存
//
// 表结构（需迁移注册，见包内 PGAnswerCacheStore 注释）：
//
//	CREATE TABLE IF NOT EXISTS rag_answer_cache (
//	    id             BIGSERIAL PRIMARY KEY,
//	    kb_id          VARCHAR(64)  NOT NULL,
//	    prompt_version VARCHAR(64)  NOT NULL,
//	    query_vector   vector(1024) NOT NULL,
//	    answer         TEXT         NOT NULL,
//	    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
//	    kb_updated_at  TIMESTAMPTZ  NOT NULL
//	);
//	CREATE INDEX IF NOT EXISTS idx_rag_answer_cache_kb_prompt ON rag_answer_cache (kb_id, prompt_version);
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
	QueryVector   string    `gorm:"column:query_vector;type:vector(1024);not null"` // pgvector 字面量 '[0.1,0.2,...]'
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
	Similarity float64 // Tier2 命中时的 cosine 相似度；其余为 0
}
