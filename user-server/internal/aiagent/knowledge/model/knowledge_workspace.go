package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// JSONMap 通用 JSON 字段类型（用于 knowledge_chunks.translated_versions 等 JSONB 字段）
//
// 注：本类型在 knowledge/model 包内本地定义，避免反向依赖 marketing/internal/model
// 造成循环引用。语义与 model.JSONMap 完全一致。
type JSONMap map[string]any

// Value 实现 driver.Valuer 接口，写库时序列化为 JSON 字符串。
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	return json.Marshal(j)
}

// Scan 实现 sql.Scanner 接口，读库时反序列化 JSONB 为 map。
func (j *JSONMap) Scan(value any) error {
	if value == nil {
		*j = JSONMap{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte("{}")
	}
	if len(data) == 0 {
		*j = JSONMap{}
		return nil
	}
	return json.Unmarshal(data, j)
}

// EmbedStatus 文档嵌入状态
type EmbedStatus string

const (
	EmbedStatusPending    EmbedStatus = "pending"
	EmbedStatusProcessing EmbedStatus = "processing"
	EmbedStatusIndexed    EmbedStatus = "indexed"
	EmbedStatusFailed     EmbedStatus = "failed"
)

// SourceType 文档来源类型
type SourceType string

const (
	SourceTypeUpload  SourceType = "upload"
	SourceTypeText    SourceType = "text"
	SourceTypeURL     SourceType = "url"
	SourceTypeBatch   SourceType = "batch"
	SourceTypeOpenAPI SourceType = "openapi"
)

// KnowledgeDocument 知识库文档(产品维度)
type KnowledgeDocument struct {
	ID            uint64      `gorm:"primaryKey;autoIncrement" json:"id"`
	ProductID string       `gorm:"index;not null;default:''" json:"product_id"`
	SourceType    SourceType  `gorm:"size:16;default:'upload';index" json:"source_type"`
	SourceRef     string      `gorm:"size:512" json:"source_ref"`
	Title         string      `gorm:"size:256;not null" json:"title"`
	FileName      string      `gorm:"size:256" json:"file_name"`
	FilePath      string      `gorm:"size:512" json:"file_path"`
	FileURL       string      `gorm:"size:512" json:"file_url"`
	FileType      string      `gorm:"size:16" json:"file_type"`
	FileSize      int64       `gorm:"default:''" json:"file_size"`
	MimeType      string      `gorm:"size:64" json:"mime_type"`
	ChunkCount    int         `gorm:"default:''" json:"chunk_count"`
	TotalTokens   int         `gorm:"default:''" json:"total_tokens"`
	EmbedStatus   EmbedStatus `gorm:"size:16;default:'pending';index" json:"embed_status"`
	EmbedProgress int         `gorm:"default:''" json:"embed_progress"`
	ErrorMsg      string      `gorm:"type:text" json:"error_msg"`
	Tags          string      `gorm:"type:jsonb;default:'[]'" json:"tags"` // JSON 字符串
	Category      string      `gorm:"size:64;index" json:"category"`
	Priority      int         `gorm:"default:''" json:"priority"`
	Metadata      string      `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	ImportedBy    string      `gorm:"size:64" json:"imported_by"`
	// 2026-07-31 P0-B: 按智能体隔离字段
	//   nil  = 共享 (默认, 向后兼容旧数据)
	//   &X   = 仅 X 智能体可见
	// 索引: idx_knowledge_doc_agent_id (按智能体过滤, ListByAgent)
	AgentID     *uint      `gorm:"index" json:"agent_id,omitempty"`
	LastIndexAt *time.Time `json:"last_index_at"`
	SearchCount int64      `gorm:"default:''" json:"search_count"`
	HitCount    int64      `gorm:"default:''" json:"hit_count"`
	Status      int        `gorm:"default:1" json:"status"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (KnowledgeDocument) TableName() string {
	return "knowledge_documents"
}

// KnowledgeChunk 知识库分段详情
type KnowledgeChunk struct {
	ID              uint64  `gorm:"primaryKey;autoIncrement" json:"id"`
	DocumentID      uint64  `gorm:"index;not null" json:"document_id"`
	ProductID string   `gorm:"index;not null;default:''" json:"product_id"`
	ChunkIndex      int     `gorm:"not null" json:"chunk_index"`
	Content         string  `gorm:"type:text;not null" json:"content"`
	ContentHash     string  `gorm:"size:64;index" json:"content_hash"`
	TokenCount      int     `gorm:"default:''" json:"token_count"`
	CharCount       int     `gorm:"default:''" json:"char_count"`
	EmbeddingID     string  `gorm:"size:64" json:"embedding_id"`
	SimilarityScore float64 `gorm:"default:''" json:"similarity_score"`
	HitCount        int     `gorm:"default:''" json:"hit_count"`
	Metadata        string  `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	SourceLanguage  string  `gorm:"type:varchar(8);default:'zh'" json:"source_language"` // 知识库源语言（v1.2 出海多语言方案）
	// TranslatedVersions 预翻译版本（知识库预翻译支持）
	// 格式：{"en": "translated content", "ja": "..."}
	// 命中目标语言时返回翻译版本，未命中返回原文 Content。
	// 默认关闭预翻译，仅高频条目按需翻译后回填此字段。
	TranslatedVersions JSONMap   `gorm:"type:jsonb;column:translated_versions" json:"translated_versions,omitempty"`
	CreatedAt          time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 表名
func (KnowledgeChunk) TableName() string {
	return "knowledge_chunks"
}

// KnowledgeImportLog 知识库导入审计日志
type KnowledgeImportLog struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProductID string     `gorm:"index;not null;default:''" json:"product_id"`
	DocumentID  *uint64   `gorm:"index" json:"document_id"`
	SourceType  string    `gorm:"size:16" json:"source_type"`
	BatchNo     string    `gorm:"size:32;index" json:"batch_no"`
	Status      string    `gorm:"size:16;default:'success'" json:"status"`
	Operator    string    `gorm:"size:64" json:"operator"`
	IP          string    `gorm:"size:64" json:"ip"`
	UserAgent   string    `gorm:"size:256" json:"user_agent"`
	DurationMs  int       `json:"duration_ms"`
	ErrorDetail string    `gorm:"type:jsonb" json:"error_detail"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 表名
func (KnowledgeImportLog) TableName() string {
	return "knowledge_import_logs"
}

// KnowledgeSearchLog 知识库检索统计日志
type KnowledgeSearchLog struct {
	ID                  uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProductID string     `gorm:"index;not null;default:''" json:"product_id"`
	Query               string    `gorm:"type:text" json:"query"`
	QueryHash           string    `gorm:"size:64;index" json:"query_hash"`
	TopK                int       `json:"top_k"`
	SimilarityThreshold float64   `json:"similarity_threshold"`
	ResultCount         int       `json:"result_count"`
	MaxScore            float64   `json:"max_score"`
	MinScore            float64   `json:"min_score"`
	AvgScore            float64   `json:"avg_score"`
	LatencyMs           int       `json:"latency_ms"`
	Hit                 int       `gorm:"default:''" json:"hit"`
	Source              string    `gorm:"size:32" json:"source"`
	SessionID           string    `gorm:"size:64" json:"session_id"`
	CreatedAt           time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 表名
func (KnowledgeSearchLog) TableName() string {
	return "knowledge_search_logs"
}

// KnowledgeOpenAPISource 知识库 OpenAPI 数据源
type KnowledgeOpenAPISource struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ProductID string      `gorm:"index;not null;default:''" json:"product_id"`
	Name            string     `gorm:"size:128;not null" json:"name"`
	Type            string     `gorm:"size:16;not null;default:'rest'" json:"type"`
	Endpoint        string     `gorm:"size:512;not null" json:"endpoint"`
	Method          string     `gorm:"size:8;default:'GET'" json:"method"`
	AuthType        string     `gorm:"size:16;default:'none'" json:"auth_type"`
	AuthConfig      string     `gorm:"type:jsonb" json:"auth_config"` // 加密 JSON
	RequestTemplate string     `gorm:"type:text" json:"request_template"`
	ResponsePath    string     `gorm:"size:256" json:"response_path"`
	FieldMapping    string     `gorm:"type:jsonb;default:'{}'" json:"field_mapping"`
	Schedule        string     `gorm:"size:32" json:"schedule"`
	Enabled         int        `gorm:"default:1" json:"enabled"`
	LastSyncAt      *time.Time `json:"last_sync_at"`
	LastStatus      string     `gorm:"size:16;default:'never'" json:"last_status"`
	LastError       string     `gorm:"type:text" json:"last_error"`
	TotalSynced     int64      `gorm:"default:''" json:"total_synced"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (KnowledgeOpenAPISource) TableName() string {
	return "knowledge_openapi_sources"
}
