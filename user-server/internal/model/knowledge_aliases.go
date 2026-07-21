package model

// ============================================================================
// 知识库模型类型别名(过渡层)
// ----------------------------------------------------------------------------
// 知识库相关 model 已迁移到 marketing/internal/aiagent/knowledge/model
// 此文件为旧 model 包提供类型别名,保持向后兼容
// 新代码应直接引用 marketing/internal/aiagent/knowledge/model
// ============================================================================

import (
	knowledgemodel "marketing/internal/aiagent/knowledge/model"
)

// 知识库核心模型
type (
	KBDocument             = knowledgemodel.KBDocument
	KBDocumentStatus       = knowledgemodel.KBDocumentStatus
	RagSession             = knowledgemodel.RagSession
	RagMessage             = knowledgemodel.RagMessage
	KnowledgeDocument      = knowledgemodel.KnowledgeDocument
	KnowledgeChunk         = knowledgemodel.KnowledgeChunk
	KnowledgeImportLog     = knowledgemodel.KnowledgeImportLog
	KnowledgeSearchLog     = knowledgemodel.KnowledgeSearchLog
	KnowledgeOpenAPISource = knowledgemodel.KnowledgeOpenAPISource
	KnowledgeAPIToken      = knowledgemodel.KnowledgeAPIToken
	KnowledgeFeedback      = knowledgemodel.KnowledgeFeedback
	// 知识库扩展类型
	ExternalImportJob = knowledgemodel.ExternalImportJob
)
