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
