// Package service 知识库子域 —— 核心服务入口
//
// 本文件仅保留：
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
	agent_runtime "hivemtk-user/internal/aiagent/agent/runtime"
	"hivemtk-user/internal/aiagent/knowledge/model"
	"hivemtk-user/internal/aiagent/knowledge/repository"
	"hivemtk-user/internal/aiagent/llm"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
	"hivemtk-user/internal/etl"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/async"
	"hivemtk-user/internal/pkg/utils/logger"
	"mime/multipart"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// KnowledgeService 知识库服务(V2.0 统一入口)
type KnowledgeService struct {
	db            *gorm.DB // R-3 Contextual Retrieval enhancer 需要原生句柄
	processor     *etl.DocumentProcessor
	vectorizer    *ragretrieval.Vectorizer
	indexer       ragretrieval.IndexManagerInterface
	ragRepo       *repository.RagConfigRepository
	docRepo       *repository.KnowledgeDocumentRepository
	chunkRepo     *repository.KnowledgeChunkRepository
	importLogRepo *repository.KnowledgeImportLogRepository
	searchLogRepo *repository.KnowledgeSearchLogRepository
	embeddingSvc  *llm.EmbeddingService
	llmSvc        *llm.LLMService
	ragSearcher   *RagSearcher
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
		db:            gdb,
		processor:     etl.NewDocumentProcessor(nil),
		vectorizer:    ragretrieval.NewVectorizer(EmbeddingDim(), nil),
		indexer:       nil,
		ragRepo:       repository.NewRagConfigRepository(gdb),
		docRepo:       repository.NewKnowledgeDocumentRepository(gdb),
		chunkRepo:     repository.NewKnowledgeChunkRepository(gdb),
		importLogRepo: repository.NewKnowledgeImportLogRepository(gdb),
		searchLogRepo: repository.NewKnowledgeSearchLogRepository(gdb),
		embeddingSvc:  llm.NewEmbeddingService(),
		llmSvc:        llm.NewLLMService(),
		ragSearcher:   NewRagSearcherWithDB(gdb),
	}
}

// ImportRequest 统一导入请求
type ImportRequest struct {
	ProductID  string
	SourceType model.SourceType
	Title      string
	Content    string
	SourceRef  string
	Category   string
	Tags       []string
	File       multipart.File
	FileHeader *multipart.FileHeader
	Operator   string
	IP         string
	UserAgent  string
	BatchNo    string
	Metadata   map[string]any `json:"metadata"`
}

// ImportResult 统一导入结果(知识库专用)
type KnowledgeImportResult struct {
	DocumentID uint64    `json:"document_id"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	SourceType string    `json:"source_type"`
	CreatedAt  time.Time `json:"created_at"`
}

// Import 统一导入入口
func (s *KnowledgeService) Import(ctx context.Context, req *ImportRequest) (*KnowledgeImportResult, error) {
	if req.ProductID == "" {
		return nil, errors.New("product_id 不能为空")
	}

	product, err := s.ragRepo.GetRagProductByID(ctx, req.ProductID)
	if err != nil {
		return nil, fmt.Errorf("产品不存在: %w", err)
	}
	if product == nil {
		return nil, errors.New("产品不存在")
	}
	productNumericID := product.ID

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
		doc, err2 = s.importText(ctx, req, product, productNumericID)
	case model.SourceTypeBatch:
		doc, err2 = s.importText(ctx, req, product, productNumericID)
	default:
		return nil, fmt.Errorf("不支持的来源类型: %s", req.SourceType)
	}

	if err2 != nil {
		_ = s.logImport(ctx, req, 0, "failed", int(time.Since(start).Milliseconds()), err2.Error())
		return nil, err2
	}

	_ = s.logImport(ctx, req, doc.ID, "success", int(time.Since(start).Milliseconds()), "")

	async.RunWithTimeout(ctx, AsyncProcessingTimeout(), func(procCtx context.Context) {
		s.processDocumentAsync(procCtx, doc.ID, productNumericID, doc.FilePath, doc.FileName, req.Content, doc.MimeType, doc.Title, req.SourceType, req.Metadata)
	})

	return &KnowledgeImportResult{
		DocumentID: doc.ID,
		Title:      doc.Title,
		Status:     string(doc.EmbedStatus),
		SourceType: string(doc.SourceType),
		CreatedAt:  doc.CreatedAt,
	}, nil
}

// List 列出文档
func (s *KnowledgeService) List(ctx context.Context, filter repository.ListFilter) ([]*model.KnowledgeDocument, int64, error) {
	return s.docRepo.List(ctx, filter)
}

// Get 获取文档
//
// KnowledgeDocument.ProductID 是 string（与 RAG 产品 RagProduct.ID 同为 UUID），前端直接传入，无需 HashStringToInt64 转换。
func (s *KnowledgeService) Get(ctx context.Context, productID string, id int64) (*model.KnowledgeDocument, error) {
	return s.docRepo.GetByProductAndID(ctx, productID, id)
}

// GetProgress 获取处理进度
func (s *KnowledgeService) GetProgress(ctx context.Context, documentID uint64) (map[string]any, error) {
	doc, err := s.docRepo.GetByID(ctx, documentID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":             doc.ID,
		"title":          doc.Title,
		"embed_status":   doc.EmbedStatus,
		"embed_progress": doc.EmbedProgress,
		"chunk_count":    doc.ChunkCount,
		"total_tokens":   doc.TotalTokens,
		"error_msg":      doc.ErrorMsg,
		"last_index_at":  doc.LastIndexAt,
	}, nil
}

// GetChunks 获取分段
func (s *KnowledgeService) GetChunks(ctx context.Context, documentID uint64) ([]model.KnowledgeChunk, error) {
	return s.chunkRepo.GetByDocumentID(ctx, documentID)
}

// Delete 删除文档(级联清理 chunks + pgvector)
func (s *KnowledgeService) Delete(ctx context.Context, productID string, id int64) error {
	doc, err := s.docRepo.GetByProductAndID(ctx, productID, id)
	if err != nil {
		return err
	}
	uid := uint64(id)
	if err := s.chunkRepo.DeleteByDocumentID(ctx, uid); err != nil {
		logger.Errorf("[knowledge] 删除分段失败: %v", err)
	}
	if doc.FilePath != "" {
		_ = os.Remove(doc.FilePath)
	}
	if err := s.docRepo.Delete(ctx, uid); err != nil {
		return err
	}

	agent_runtime.PublishKnowledgeDocumentDelete(productID, uint(id), 0)

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

	agent_runtime.PublishKnowledgeDocumentUpdate(doc.ProductID, uint(doc.ID), "", 0)

	return nil
}

// ListImportLogs 列出导入日志
func (s *KnowledgeService) ListImportLogs(ctx context.Context, filter repository.ImportLogListFilter) ([]model.KnowledgeImportLog, int64, error) {
	return s.importLogRepo.List(ctx, filter)
}

// resolveProductByID 通过 RAG 产品 UUID(string) 反查知识库。
// 知识库 product_id 现已统一为 RagProduct.ID(string UUID)，与 RAG 产品直接对应。
// 用于 ingestion / 分段编辑阶段读取 per-product embedding 配置，避免与检索侧向量空间不一致。
func (s *KnowledgeService) resolveProductByID(ctx context.Context, productID string) *model.RagProduct {
	if s.ragRepo == nil {
		return nil
	}
	products, err := s.ragRepo.ListRagProducts(ctx)
	if err != nil || len(products) == 0 {
		return nil
	}
	for _, p := range products {
		if p.ID == productID {
			return p
		}
	}
	return nil
}

// resolveEmbeddingConfig 返回用于指定知识库向量化的 embedding 服务与配置。
// per-product EmbeddingProviderConfig 优先；否则回退全局默认配置（与检索侧 QueryKnowledgeBase 一致）。
func (s *KnowledgeService) resolveEmbeddingConfig(ctx context.Context, numericProductID string) (*llm.EmbeddingService, *llm.EmbeddingConfig) {
	if prod := s.resolveProductByID(ctx, numericProductID); prod != nil && prod.EmbeddingProviderConfig.BaseURL != "" {
		dim := prod.EmbeddingProviderConfig.Dimension
		if dim == 0 {
                    dim = EmbeddingDim()		}
		cfg := &llm.EmbeddingConfig{
			APIType:        prod.EmbeddingProviderConfig.APIType,
			BaseURL:        prod.EmbeddingProviderConfig.BaseURL,
			Model:          prod.EmbeddingProviderConfig.Model,
			APIKey:         prod.EmbeddingProviderConfig.APIKey,
			Dimension:      dim,
			AllowFallback:  false,
			RequestTimeout: DefaultRequestTimeoutSeconds(),
			MaxRetries:     2,
		}
		return llm.NewEmbeddingServiceWithConfig(cfg), cfg
	}
	return s.embeddingSvc, nil
}

// persistChunkEmbeddings 委托给 chunkRepo 持久化 embedding（默认 tei 来源）
func (s *KnowledgeService) persistChunkEmbeddings(ctx context.Context, chunks []model.KnowledgeChunk, embeddings [][]float32) error {
	return s.persistChunkEmbeddingsWithSource(ctx, chunks, embeddings, llm.EmbedSourceTEI)
}

// persistChunkEmbeddingsWithSource D16：带来源标记持久化
func (s *KnowledgeService) persistChunkEmbeddingsWithSource(ctx context.Context, chunks []model.KnowledgeChunk, embeddings [][]float32, source string) error {
	return s.chunkRepo.UpdateEmbeddingsBatchWithSource(ctx, chunks, embeddings, source)
}

// EmbedAndPersistChunks 向量化指定分片并写入 knowledge_chunks.embedding（per-product 配置优先）。
// 供分段编辑（更新/重切）后重新向量化使用，确保向量与内容一致。
func (s *KnowledgeService) EmbedAndPersistChunks(ctx context.Context, numericProductID string, chunks []model.KnowledgeChunk) error {
	embService, embCfg := s.resolveEmbeddingConfig(ctx, numericProductID)
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embeddings, source, err := embService.EmbedWithSource(ctx, embCfg, texts)
	if err != nil {
		return fmt.Errorf("向量化失败: %w", err)
	}
	// N-4 维度守卫：preset 不符条目剔除（log+跳过），不中断批量写入
	chunks, embeddings = filterValidEmbeddings(embService, embCfg, chunks, embeddings)
	return s.persistChunkEmbeddingsWithSource(ctx, chunks, embeddings, source)
}

// Search 检索知识库
func (s *KnowledgeService) Search(ctx context.Context, productID string, query string, topK int, threshold float64) ([]model.KnowledgeChunk, error) {
	// R43 修复：原为半成品桩（_ = queryVec; return nil,nil）恒空，检索 API 全线失效。
	// 接线到 RagSearcher 的 HybridSearcher（向量+BM25 混合），命中分片按阈值过滤。
	if s.ragSearcher == nil {
		return nil, errors.New("rag searcher 未初始化")
	}
	// RagSearcher.Search 内部：hybrid(向量+BM25+RRF+重排) → legacy 降级链全托管
	ragChunks, err := s.ragSearcher.Search(ctx, query, topK)
	if err != nil {
		return nil, fmt.Errorf("混合检索失败: %w", err)
	}
	out := make([]model.KnowledgeChunk, 0, len(ragChunks))
	for _, c := range ragChunks {
		// 注意：RankRAGChunks 后的 Score 是加权排序分（含 chunk 权重），不是
		// 0-1 余弦相似度，不能用绝对阈值截断。阈值语义由 RagSearcher 内部的
		// 相似度门控承担，这里只做归属映射。
		docID, _ := strconv.ParseUint(strings.TrimPrefix(c.DocID, "kb_doc_"), 10, 64)
		out = append(out, model.KnowledgeChunk{
			DocumentID: docID,
			ProductID:  productID,
			Content:    c.Content,
		})
	}
	return out, nil
}
