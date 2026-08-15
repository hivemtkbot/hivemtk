package controller

import (
	"context"

	knowledgemodel "hivemtk-user/internal/aiagent/knowledge/model"
)

// OpenAPISyncResult 数据源同步结果视图
//
// P2-3：与 service.SyncResult 字段同构的本地视图，避免 aiagent 依赖 service 包。
type OpenAPISyncResult struct {
	SourceID    uint64 `json:"source_id"`
	TotalItems  int    `json:"total_items"`
	ImportedNum int    `json:"imported_num"`
	SkippedNum  int    `json:"skipped_num"`
	FailedNum   int    `json:"failed_num"`
	DurationMs  int64  `json:"duration_ms"`
	Status      string `json:"status"`
	ErrorMsg    string `json:"error_msg"`
}

// OpenAPISourcePort OpenAPI 数据源管理端口（窄接口）
//
// P2-3：切断 aiagent→service 反向依赖。
// 实现方由装配层（internal/router）注入，生产实现为 service.OpenAPIService 的适配器。
type OpenAPISourcePort interface {
	ListSources(ctx context.Context, productID string) ([]knowledgemodel.KnowledgeOpenAPISource, error)
	CreateSource(ctx context.Context, src *knowledgemodel.KnowledgeOpenAPISource) error
	GetSource(ctx context.Context, productID string, id int64) (*knowledgemodel.KnowledgeOpenAPISource, error)
	UpdateSource(ctx context.Context, src *knowledgemodel.KnowledgeOpenAPISource) error
	DeleteSource(ctx context.Context, productID string, id int64) error
	ToggleEnabled(ctx context.Context, productID string, id int64, enabled bool) error
	SyncSource(ctx context.Context, productID string, sourceID int64) (*OpenAPISyncResult, error)
	TestConnection(ctx context.Context, src *knowledgemodel.KnowledgeOpenAPISource) (map[string]any, error)
}

