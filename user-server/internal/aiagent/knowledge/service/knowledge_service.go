// Package service 知识库子域 —— 核心服务入口
//
// 2026-07-23 五层架构治理（二轮）：从原 919 行 knowledge_service.go 拆分
// 出 6 个子域文件（import/pipeline/reindex/log/security/helpers），本文件
// 仅保留：
//   - 类型定义（ImportRequest, ImportResult）
//   - 构造函数（NewKnowledgeService / NewKnowledgeServiceWithDB）
//   - Import 统一入口
//   - 文档管理（List / Get / GetProgress / GetChunks / Delete / Update）
//   - ListImportLogs
//   - 检索（Search）
//   - embedding 配置解析（resolveProductByNumericID / resolveEmbeddingConfig）
//   - 向量持久化委托（persistChunkEmbeddings / EmbedAndPersistChunks）
package service

import (
	"context"
	"errors"
	"fmt"
	agent_runtime "marketing/internal/aiagent/agent/runtime"
	"marketing/internal/aiagent/knowledge/model"
	"marketing/internal/aiagent/knowledge/repository"
	"marketing/internal/aiagent/llm"
	ragretrieval "marketing/internal/aiagent/rag/retrieval"
	"marketing/internal/etl"
	"marketing/internal/pkg/utils/async"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"mime/multipart"
	"os"
	"time"

	"gorm.io/gorm"
)

// KnowledgeService 知识库服务(V2.0 统一入口)
//
// 2026-07-23 五层架构治理（二轮）：
// 删除 `db *gorm.DB` 字段。原字段仅在 `persistChunkEmbeddings` 间接使用（已下沉到
// `repository.KnowledgeChunkRepository.UpdateEmbeddingsBatch`），其它路径都不需要
// 直接持有 db。service 不再持有 *gorm.DB。
type KnowledgeService struct {
	processor	*etl.DocumentProcessor
	vectorizer	*ragretrieval.Vectorizer
	indexer		ragretrieval.IndexManagerInterface
	ragRepo		*repository.RagConfigRepository
	docRepo		*repository.KnowledgeDocumentRepository
	chunkRepo	*repository.KnowledgeChunkRepository
	importLogRepo	*repository.KnowledgeImportLogRepository
	searchLogRepo	*repository.KnowledgeSearchLogRepository
	embeddingSvc	*llm.EmbeddingService
	llmSvc		*llm.LLMService
}

// NewKnowledgeService 创建知识库服务
func NewKnowledgeService() *KnowledgeService {
	return newKnowledgeServiceWithDB(db.GetDB())
}

// NewKnowledgeServiceWithDB 创建带 DB 的知识库服务(用于测试)
func NewKnowledgeServiceWithDB(gdb *gorm.DB) *KnowledgeService {
	return newKnowledgeServiceWithDB(gdb)
}

func newKnowledgeServiceWithDB(gdb *gorm.DB) *KnowledgeService {
	return &KnowledgeService{
		processor:	etl.NewDocumentProcessor(nil),
		vectorizer:	ragretrieval.NewVectorizer(EmbeddingDim, nil),
		indexer:	nil,
		ragRepo:	repository.NewRagConfigRepository(gdb),
		docRepo:	repository.NewKnowledgeDocumentRepository(gdb),
		chunkRepo:	repository.NewKnowledgeChunkRepository(gdb),
		importLogRepo:	repository.NewKnowledgeImportLogRepository(gdb),
		searchLogRepo:	repository.NewKnowledgeSearchLogRepository(gdb),
		embeddingSvc:	llm.NewEmbeddingService(),
		llmSvc:		llm.NewLLMService(),
	}
}

// ============================================================================
// 统一导入入口
// ============================================================================

// ImportRequest 统一导入请求
type ImportRequest struct {
	ProductID	string
	SourceType	model.SourceType
	Title		string
	Content		string
	SourceRef	string
	Category	string
	Tags		[]string
	File		multipart.File
	FileHeader	*multipart.FileHeader
	Operator	string
	IP		string
	UserAgent	string
	BatchNo		string
	// Metadata 附加字段：承载业务上下文（订单信息、客户ID、渠道等）。
	// 入库时写入 KnowledgeDocument.Metadata，并逐片复制到 KnowledgeChunk.Metadata，
	// 检索时随分片返回，供智能体使用。
	Metadata	map[string]any	`json:"metadata"`
}

// ImportResult 统一导入结果(知识库专用)
type KnowledgeImportResult struct {
	DocumentID	uint64		`json:"document_id"`
	Title		string		`json:"title"`
	Status		string		`json:"status"`
	SourceType	string		`json:"source_type"`
	CreatedAt	time.Time	`json:"created_at"`
}

// Import 统一导入入口
func (s *KnowledgeService) Import(ctx context.Context, req *ImportRequest) (*KnowledgeImportResult, error) {
	if req.ProductID == "" {
		return nil, errors.New("product_id 不能为空")
	}

	// 解析 ProductID 为 int64(产品 ID 可以是 UUID 字符串,但本系统用 string 存储,需查找 numeric ID)
	// 由于 RagProduct.ID 是 string UUID,这里需要从数据库获取 numeric ID(暂时使用 0 表示通过 UUID 关联)
	product, err := s.ragRepo.GetRagProductByID(ctx, req.ProductID)
	if err != nil {
		return nil, fmt.Errorf("产品不存在: %w", err)
	}
	if product == nil {
		return nil, errors.New("产品不存在")
	}
	productNumericID := HashStringToInt64(product.ID)	// UUID 哈希到 int64,匹配知识库 INTEGER 字段

	start := time.Now()
	var doc *model.KnowledgeDocument
	var err2 error

	switch req.SourceType {
	case model.SourceTypeUpload:
		doc, err2 = s.importUploadedFile(ctx, req, product, productNumericID)
	case model.SourceTypeText:
		doc, err2 = s.importText(ctx, req, product, productNumericID)
	case model.SourceTypeURL:
		doc, err2 = s.importFromURL(ctx, req, product, productNumericID)
	case model.SourceTypeOpenAPI:
		// OpenAPI 模式:直接传入 content(已由 OpenAPIService 预解析)
		doc, err2 = s.importText(ctx, req, product, productNumericID)
	case model.SourceTypeBatch:
		// 批量导入: 走 importText 路径,直接使用 content 字段(批量导入的每行内容已经在 BatchImportItem.Content 中)
		doc, err2 = s.importText(ctx, req, product, productNumericID)
	default:
		return nil, fmt.Errorf("不支持的来源类型: %s", req.SourceType)
	}

	if err2 != nil {
		// 记录失败日志
		_ = s.logImport(ctx, req, 0, "failed", int(time.Since(start).Milliseconds()), err2.Error())
		return nil, err2
	}

	// 记录成功日志
	_ = s.logImport(ctx, req, doc.ID, "success", int(time.Since(start).Milliseconds()), "")

	// 启动异步处理
	// 使用 async.RunWithTimeout 保留原 ctx 的 trace ID 等 Value，但用 Background() 作为 parent，
	// 施加 AsyncProcessingTimeout 超时兜底：embedding/索引服务不可达时，防止文档永久卡在 processing 状态。
	async.RunWithTimeout(ctx, AsyncProcessingTimeout, func(procCtx context.Context) {
		s.processDocumentAsync(procCtx, doc.ID, productNumericID, doc.FilePath, doc.FileName, req.Content, doc.MimeType, doc.Title, req.SourceType, req.Metadata)
	})

	return &KnowledgeImportResult{
		DocumentID:	doc.ID,
		Title:		doc.Title,
		Status:		string(doc.EmbedStatus),
		SourceType:	string(doc.SourceType),
		CreatedAt:	doc.CreatedAt,
	}, nil
}

// ============================================================================
// 文档管理
// ============================================================================

// List 列出文档
func (s *KnowledgeService) List(ctx context.Context, filter repository.ListFilter) ([]*model.KnowledgeDocument, int64, error) {
	return s.docRepo.List(ctx, filter)
}

// Get 获取文档
//
// 2026-07-18 修复：KnowledgeDocument.ProductID 是 int64（迁移 schema 为 INTEGER），
// 调用方需将前端传入的 string UUID 经 HashStringToInt64 映射回 int64。
func (s *KnowledgeService) Get(ctx context.Context, productID, id int64) (*model.KnowledgeDocument, error) {
	return s.docRepo.GetByProductAndID(ctx, productID, id)
}

// GetProgress 获取处理进度
func (s *KnowledgeService) GetProgress(ctx context.Context, documentID uint64) (map[string]any, error) {
	doc, err := s.docRepo.GetByID(ctx, documentID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":			doc.ID,
		"title":		doc.Title,
		"embed_status":		doc.EmbedStatus,
		"embed_progress":	doc.EmbedProgress,
		"chunk_count":		doc.ChunkCount,
		"total_tokens":		doc.TotalTokens,
		"error_msg":		doc.ErrorMsg,
		"last_index_at":	doc.LastIndexAt,
	}, nil
}

// GetChunks 获取分段
func (s *KnowledgeService) GetChunks(ctx context.Context, documentID uint64) ([]model.KnowledgeChunk, error) {
	return s.chunkRepo.GetByDocumentID(ctx, documentID)
}

// Delete 删除文档(级联清理 chunks + pgvector)
func (s *KnowledgeService) Delete(ctx context.Context, productID, id int64) error {
	doc, err := s.docRepo.GetByProductAndID(ctx, productID, id)
	if err != nil {
		return err
	}
	uid := uint64(id)
	// 删除分段
	if err := s.chunkRepo.DeleteByDocumentID(ctx, uid); err != nil {
		logger.Errorf("[knowledge] 删除分段失败: %v", err)
	}
	// 删除物理文件
	if doc.FilePath != "" {
		_ = os.Remove(doc.FilePath)
	}
	// 删除文档记录
	if err := s.docRepo.Delete(ctx, uid); err != nil {
		return err
	}

	// 2026-07-17:发布删除事件(ADR-008 §2.5 子项 2)
	// 触发 rag.IncrementalIndexer 清理内存索引
	agent_runtime.PublishKnowledgeDocumentDelete(uint(productID), uint(id), 0)

	return nil
}

// Update 更新文档元信息
func (s *KnowledgeService) Update(ctx context.Context, doc *model.KnowledgeDocument) error {
	if doc.ID == 0 {
		return errors.New("文档 ID 不能为空")
	}
	if err := s.docRepo.Update(ctx, doc); err != nil {
		return err
	}

	// 2026-07-17:发布更新事件(ADR-008 §2.5 子项 2)
	// KnowledgeDocument 无 Content 字段(分段在 KnowledgeChunk),事件不传 content
	agent_runtime.PublishKnowledgeDocumentUpdate(uint(doc.ProductID), uint(doc.ID), "", 0)

	return nil
}

// ListImportLogs 列出导入日志
func (s *KnowledgeService) ListImportLogs(ctx context.Context, filter repository.ImportLogListFilter) ([]model.KnowledgeImportLog, int64, error) {
	return s.importLogRepo.List(ctx, filter)
}

// resolveProductByNumericID 通过 numeric 哈希反查知识库。
// 知识库主键为 UUID 字符串，而 knowledge_chunks.product_id = HashStringToInt64(UUID)。
// 用于 ingestion / 分段编辑阶段读取 per-product embedding 配置，避免与检索侧向量空间不一致。
func (s *KnowledgeService) resolveProductByNumericID(ctx context.Context, numericID int64) *model.RagProduct {
	if s.ragRepo == nil {
		return nil
	}
	products, err := s.ragRepo.ListRagProducts(ctx)
	if err != nil || len(products) == 0 {
		return nil
	}
	for _, p := range products {
		if HashStringToInt64(p.ID) == numericID {
			return p
		}
	}
	return nil
}

// resolveEmbeddingConfig 返回用于指定知识库向量化的 embedding 服务与配置。
// per-product EmbeddingProviderConfig 优先；否则回退全局默认配置（与检索侧 QueryKnowledgeBase 一致）。
func (s *KnowledgeService) resolveEmbeddingConfig(ctx context.Context, numericProductID int64) (*llm.EmbeddingService, *llm.EmbeddingConfig) {
	if prod := s.resolveProductByNumericID(ctx, numericProductID); prod != nil && prod.EmbeddingProviderConfig.BaseURL != "" {
		dim := prod.EmbeddingProviderConfig.Dimension
		if dim == 0 {
			dim = EmbeddingDim
		}
		cfg := &llm.EmbeddingConfig{
			APIType:	prod.EmbeddingProviderConfig.APIType,
			BaseURL:	prod.EmbeddingProviderConfig.BaseURL,
			Model:		prod.EmbeddingProviderConfig.Model,
			APIKey:		prod.EmbeddingProviderConfig.APIKey,
			Dimension:	dim,
			AllowFallback:	false,
			RequestTimeout:	DefaultRequestTimeoutSeconds,
			MaxRetries:	2,
		}
		return llm.NewEmbeddingServiceWithConfig(cfg), cfg
	}
	return s.embeddingSvc, nil
}

// persistChunkEmbeddings 委托给 chunkRepo 持久化 embedding
//
// 2026-07-23 五层架构治理（二轮）：原实现直接在 service 中 tx.Exec 写 SQL，
// 违反 §3.5"service 不应写 SQL"。已下沉到
// `repository.KnowledgeChunkRepository.UpdateEmbeddingsBatch`。
func (s *KnowledgeService) persistChunkEmbeddings(ctx context.Context, chunks []model.KnowledgeChunk, embeddings [][]float32) error {
	return s.chunkRepo.UpdateEmbeddingsBatch(ctx, chunks, embeddings)
}

// EmbedAndPersistChunks 向量化指定分片并写入 knowledge_chunks.embedding（per-product 配置优先）。
// 供分段编辑（更新/重切）后重新向量化使用，确保向量与内容一致。
func (s *KnowledgeService) EmbedAndPersistChunks(ctx context.Context, numericProductID int64, chunks []model.KnowledgeChunk) error {
	embService, embCfg := s.resolveEmbeddingConfig(ctx, numericProductID)
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embeddings, err := embService.Embed(ctx, embCfg, texts)
	if err != nil {
		return fmt.Errorf("向量化失败: %w", err)
	}
	return s.persistChunkEmbeddings(ctx, chunks, embeddings)
}

// ============================================================================
// 检索
// ============================================================================

// Search 检索知识库
func (s *KnowledgeService) Search(ctx context.Context, productID int64, query string, topK int, threshold float64) ([]model.KnowledgeChunk, error) {
	// 1. 向量化 query
	queryVec, err := s.vectorizer.EmbedText(query)
	if err != nil {
		return nil, fmt.Errorf("向量化查询失败: %w", err)
	}

	// 2. 简化实现:从 knowledge_chunks 获取产品下所有分段,在内存中做相似度匹配
	// (生产环境应该用 pgvector 的 <=> 操作)
	total, _ := s.chunkRepo.CountByProductID(ctx, productID)
	if total == 0 {
		return nil, nil
	}

	// 真实检索应该调用 pgvector_index_manager.SearchIndex
	// 这里返回简化结果,实际由 rag_search_service 处理
	_ = queryVec
	return nil, nil
}
