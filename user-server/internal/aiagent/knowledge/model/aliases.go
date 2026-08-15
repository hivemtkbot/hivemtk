// Package model 知识库域模型（兼容层）。
//
// 架构整改 P0-2（2026-08-10）：知识库实体的权威定义已下沉到共享叶子层
// hivemtk-user/internal/model（五层架构中 model 为叶子层，不得被反向依赖）。
// 本包保留为纯别名转发层，供既有 35 处 import 平滑过渡；
// 新代码应直接 import hivemtk-user/internal/model，本包后续随域重构移除。
package model

import (
	"hivemtk-user/internal/model"
)


type (
	KBDocument       = model.KBDocument
	KBDocumentStatus = model.KBDocumentStatus
)

const (
	KBDocumentStatusPending    = model.KBDocumentStatusPending
	KBDocumentStatusProcessing = model.KBDocumentStatusProcessing
	KBDocumentStatusIndexed    = model.KBDocumentStatusIndexed
	KBDocumentStatusFailed     = model.KBDocumentStatusFailed
)


type (
	EmbedStatus = model.EmbedStatus
	SourceType  = model.SourceType
	JSONMap     = model.JSONMap

	KnowledgeDocument      = model.KnowledgeDocument
	KnowledgeChunk         = model.KnowledgeChunk
	KnowledgeImportLog     = model.KnowledgeImportLog
	KnowledgeSearchLog     = model.KnowledgeSearchLog
	KnowledgeOpenAPISource = model.KnowledgeOpenAPISource
)

const (
	EmbedStatusPending    = model.EmbedStatusPending
	EmbedStatusProcessing = model.EmbedStatusProcessing
	EmbedStatusIndexed    = model.EmbedStatusIndexed
	EmbedStatusFailed     = model.EmbedStatusFailed

	SourceTypeUpload  = model.SourceTypeUpload
	SourceTypeText    = model.SourceTypeText
	SourceTypeURL     = model.SourceTypeURL
	SourceTypeBatch   = model.SourceTypeBatch
	SourceTypeOpenAPI = model.SourceTypeOpenAPI
)


type (
	KnowledgeAPIToken = model.KnowledgeAPIToken
	KnowledgeFeedback = model.KnowledgeFeedback
	ExternalImportJob = model.ExternalImportJob
)


type (
	LLMProviderConfig       = model.LLMProviderConfig
	EmbeddingProviderConfig = model.EmbeddingProviderConfig
	RerankProviderConfig    = model.RerankProviderConfig
	RagProduct              = model.RagProduct
	IntentConfig            = model.IntentConfig
)

// DefaultIntentConfig 默认意图配置（转发共享层实现）
func DefaultIntentConfig() model.IntentConfig {
	return model.DefaultIntentConfig()
}


type (
	RagSession = model.RagSession
	RagMessage = model.RagMessage
)

