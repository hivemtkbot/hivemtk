package service

import (
	dbutil "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/knowledge/repository"
)

// KnowledgeMerchantService 商户自部署场景的 RAG 核心增强服务
// 对应审计项：商户需要可视化管理 + 外部系统导入 + 检索调试 + 分段编辑 + 反馈
//
// 包含以下子能力（按子能力拆分至独立文件）：
//  1. BatchImportService    批量导入（CSV/JSON）             → knowledge_merchant_batch.go
//  2. PlaygroundService     检索 Playground（不同 topK / 阈值 / 调参） → knowledge_merchant_playground.go
//  3. ChunkEditService      分段编辑（增/改/删/重切）         → knowledge_merchant_chunk.go
//  4. FeedbackService       反馈标注                         → knowledge_merchant_feedback.go
//  5. TokenService          外部 API Token 管理              → knowledge_merchant_token.go
//  6. ExternalImportService 外部系统接入（飞书/Notion/通用 JSON） → knowledge_merchant_external.go
type KnowledgeMerchantService struct {
	db            *gorm.DB
	kbService     *KnowledgeService
	ragSearch     *RagSearcher
	docRepo       *repository.KnowledgeDocumentRepository
	chunkRepo     *repository.KnowledgeChunkRepository
	prodRepo      *repository.RagConfigRepository
	searchLogRepo *repository.KnowledgeSearchLogRepository
	feedbackRepo  *repository.KnowledgeFeedbackRepository
	tokenRepo     *repository.KnowledgeAPITokenRepository
	externalRepo  *repository.ExternalImportJobRepository
}

// GetDB 暴露内部 DB（仅用于测试和特殊场景，生产代码不应直接访问）
func (s *KnowledgeMerchantService) GetDB() *gorm.DB {
	return s.db
}

// NewKnowledgeMerchantService 创建商户视角 RAG 服务
func NewKnowledgeMerchantService() *KnowledgeMerchantService {
	db := dbutil.GetDB()
	return &KnowledgeMerchantService{
		db:            db,
		kbService:     NewKnowledgeService(),
		ragSearch:     NewRagSearcher(),
		docRepo:       repository.NewKnowledgeDocumentRepository(db),
		chunkRepo:     repository.NewKnowledgeChunkRepository(db),
		prodRepo:      repository.NewRagConfigRepository(db),
		searchLogRepo: repository.NewKnowledgeSearchLogRepository(db),
		feedbackRepo:  repository.NewKnowledgeFeedbackRepository(db),
		tokenRepo:     repository.NewKnowledgeAPITokenRepository(db),
		externalRepo:  repository.NewExternalImportJobRepository(db),
	}
}

// NewKnowledgeMerchantServiceWithDB 带 DB 的版本（用于测试）
func NewKnowledgeMerchantServiceWithDB(gdb *gorm.DB) *KnowledgeMerchantService {
	return &KnowledgeMerchantService{
		db:            gdb,
		kbService:     NewKnowledgeServiceWithDB(gdb),
		ragSearch:     NewRagSearcherWithDB(gdb),
		docRepo:       repository.NewKnowledgeDocumentRepository(gdb),
		chunkRepo:     repository.NewKnowledgeChunkRepository(gdb),
		prodRepo:      repository.NewRagConfigRepository(gdb),
		searchLogRepo: repository.NewKnowledgeSearchLogRepository(gdb),
		feedbackRepo:  repository.NewKnowledgeFeedbackRepository(gdb),
		tokenRepo:     repository.NewKnowledgeAPITokenRepository(gdb),
		externalRepo:  repository.NewExternalImportJobRepository(gdb),
	}
}

func (s *KnowledgeMerchantService) ensureReposFromDB() {
	if s.db == nil {
		return
	}
	if s.searchLogRepo == nil {
		s.searchLogRepo = repository.NewKnowledgeSearchLogRepository(s.db)
	}
	if s.feedbackRepo == nil {
		s.feedbackRepo = repository.NewKnowledgeFeedbackRepository(s.db)
	}
	if s.tokenRepo == nil {
		s.tokenRepo = repository.NewKnowledgeAPITokenRepository(s.db)
	}
	if s.externalRepo == nil {
		s.externalRepo = repository.NewExternalImportJobRepository(s.db)
	}
}
