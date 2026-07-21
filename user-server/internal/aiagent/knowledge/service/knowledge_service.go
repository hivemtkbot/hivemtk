package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	agent_runtime "marketing/internal/aiagent/agent/runtime"
	"marketing/internal/aiagent/knowledge/model"
	"marketing/internal/aiagent/knowledge/repository"
	"marketing/internal/aiagent/llm"
	rag_core "marketing/internal/aiagent/rag/core"
	ragretrieval "marketing/internal/aiagent/rag/retrieval"
	"marketing/internal/etl"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// KnowledgeService 知识库服务(V2.0 统一入口)
type KnowledgeService struct {
	db            *gorm.DB
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
}

// NewKnowledgeService 创建知识库服务
func NewKnowledgeService() *KnowledgeService {
	return &KnowledgeService{
		db:            dbGetDB(),
		processor:     etl.NewDocumentProcessor(nil),
		vectorizer:    ragretrieval.NewVectorizer(1024, nil),
		indexer:       nil,
		ragRepo:       repository.NewRagConfigRepository(dbGetDB()),
		docRepo:       repository.NewKnowledgeDocumentRepository(dbGetDB()),
		chunkRepo:     repository.NewKnowledgeChunkRepository(dbGetDB()),
		importLogRepo: repository.NewKnowledgeImportLogRepository(dbGetDB()),
		searchLogRepo: repository.NewKnowledgeSearchLogRepository(dbGetDB()),
		embeddingSvc:  llm.NewEmbeddingService(),
		llmSvc:        llm.NewLLMService(),
	}
}

// NewKnowledgeServiceWithDB 创建带 DB 的知识库服务(用于测试)
func NewKnowledgeServiceWithDB(gdb *gorm.DB) *KnowledgeService {
	return &KnowledgeService{
		db:            gdb,
		processor:     etl.NewDocumentProcessor(nil),
		vectorizer:    ragretrieval.NewVectorizer(1024, nil),
		ragRepo:       repository.NewRagConfigRepository(gdb),
		docRepo:       repository.NewKnowledgeDocumentRepository(gdb),
		chunkRepo:     repository.NewKnowledgeChunkRepository(gdb),
		importLogRepo: repository.NewKnowledgeImportLogRepository(gdb),
		searchLogRepo: repository.NewKnowledgeSearchLogRepository(gdb),
		embeddingSvc:  llm.NewEmbeddingService(),
		llmSvc:        llm.NewLLMService(),
	}
}

// ============================================================================
// 统一导入入口
// ============================================================================

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
	// Metadata 附加字段：承载业务上下文（订单信息、客户ID、渠道等）。
	// 入库时写入 KnowledgeDocument.Metadata，并逐片复制到 KnowledgeChunk.Metadata，
	// 检索时随分片返回，供智能体使用。
	Metadata map[string]any `json:"metadata"`
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

	// 解析 ProductID 为 int64(产品 ID 可以是 UUID 字符串,但本系统用 string 存储,需查找 numeric ID)
	// 由于 RagProduct.ID 是 string UUID,这里需要从数据库获取 numeric ID(暂时使用 0 表示通过 UUID 关联)
	product, err := s.ragRepo.GetRagProductByID(ctx, req.ProductID)
	if err != nil {
		return nil, fmt.Errorf("产品不存在: %w", err)
	}
	if product == nil {
		return nil, errors.New("产品不存在")
	}
	productNumericID := HashStringToInt64(product.ID) // UUID 哈希到 int64,匹配知识库 INTEGER 字段

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
	go func(documentID uint64, productID int64, filePath, fileName, content, mimeType, title string, source model.SourceType, docMeta map[string]any) {
		// 整体超时兜底：embedding/索引服务不可达时，防止文档永久卡在 processing 状态
		procCtx, procCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer procCancel()
		s.processDocumentAsync(procCtx, documentID, productID, filePath, fileName, content, mimeType, title, source, docMeta)
	}(doc.ID, productNumericID, doc.FilePath, doc.FileName, req.Content, doc.MimeType, doc.Title, req.SourceType, req.Metadata)

	return &KnowledgeImportResult{
		DocumentID: doc.ID,
		Title:      doc.Title,
		Status:     string(doc.EmbedStatus),
		SourceType: string(doc.SourceType),
		CreatedAt:  doc.CreatedAt,
	}, nil
}

// HashStringToInt64 将字符串哈希为 int64(用于 product_id 字段)
func HashStringToInt64(s string) int64 {
	h := sha256.Sum256([]byte(s))
	var n int64
	for i := 0; i < 8; i++ {
		n = (n << 8) | int64(h[i])
	}
	if n < 0 {
		n = -n
	}
	return n
}

// ============================================================================
// 各种来源实现
// ============================================================================

func (s *KnowledgeService) importUploadedFile(ctx context.Context, req *ImportRequest, product *model.RagProduct, productNumericID int64) (*model.KnowledgeDocument, error) {
	if req.File == nil || req.FileHeader == nil {
		return nil, errors.New("文件不能为空")
	}
	ext := strings.ToLower(filepath.Ext(req.FileHeader.Filename))
	allowed := map[string]bool{".pdf": true, ".docx": true, ".doc": true, ".txt": true, ".md": true, ".html": true, ".json": true, ".csv": true}
	if !allowed[ext] {
		return nil, fmt.Errorf("不支持的文件类型: %s", ext)
	}

	// 保存文件
	uploadDir := filepath.Join("uploads", "knowledge", req.ProductID, time.Now().Format("20060102"))
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("创建上传目录失败: %w", err)
	}
	filename := uuid.New().String() + ext
	filePath := filepath.Join(uploadDir, filename)
	dst, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("创建文件失败: %w", err)
	}
	defer dst.Close()
	size, err := io.Copy(dst, req.File)
	if err != nil {
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}
	_ = dst.Close()

	title := req.Title
	if title == "" {
		title = strings.TrimSuffix(req.FileHeader.Filename, ext)
	}

	tagsJSON, _ := json.Marshal(req.Tags)
	doc := &model.KnowledgeDocument{

		ProductID:   productNumericID,
		SourceType:  req.SourceType,
		SourceRef:   req.SourceRef,
		Title:       title,
		FileName:    req.FileHeader.Filename,
		FilePath:    filePath,
		FileType:    ext,
		FileSize:    size,
		MimeType:    getMimeType(ext),
		EmbedStatus: model.EmbedStatusPending,
		Category:    req.Category,
		Tags:        string(tagsJSON),
		Metadata:    metaToJSON(req.Metadata),
		Status:      1,
	}
	if err := s.docRepo.Create(ctx, doc); err != nil {
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("保存文档记录失败: %w", err)
	}
	return doc, nil
}

func (s *KnowledgeService) importText(ctx context.Context, req *ImportRequest, product *model.RagProduct, productNumericID int64) (*model.KnowledgeDocument, error) {
	if strings.TrimSpace(req.Content) == "" {
		return nil, errors.New("内容不能为空")
	}
	if req.Title == "" {
		req.Title = "未命名文档_" + time.Now().Format("20060102150405")
	}
	tagsJSON, _ := json.Marshal(req.Tags)
	// 内容存到 FilePath(临时方案,生产可加 Content 字段到 model)
	tmpDir := filepath.Join("uploads", "knowledge-text", req.ProductID, time.Now().Format("20060102"))
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("创建目录失败: %w", err)
	}
	textFile := filepath.Join(tmpDir, uuid.New().String()+".txt")
	if err := os.WriteFile(textFile, []byte(req.Content), 0644); err != nil {
		return nil, fmt.Errorf("写入文本失败: %w", err)
	}
	doc := &model.KnowledgeDocument{

		ProductID:   productNumericID,
		SourceType:  req.SourceType,
		SourceRef:   req.SourceRef,
		Title:       req.Title,
		FileName:    req.Title + ".txt",
		FilePath:    textFile,
		FileType:    ".txt",
		FileSize:    int64(len(req.Content)),
		MimeType:    "text/plain",
		EmbedStatus: model.EmbedStatusPending,
		Category:    req.Category,
		Tags:        string(tagsJSON),
		Metadata:    metaToJSON(req.Metadata),
		Status:      1,
	}
	if err := s.docRepo.Create(ctx, doc); err != nil {
		_ = os.Remove(textFile)
		return nil, fmt.Errorf("保存文档记录失败: %w", err)
	}
	return doc, nil
}

func (s *KnowledgeService) importFromURL(ctx context.Context, req *ImportRequest, product *model.RagProduct, productNumericID int64) (*model.KnowledgeDocument, error) {
	if req.SourceRef == "" {
		return nil, errors.New("URL 不能为空")
	}
	// SSRF 防护
	if err := validateURL(req.SourceRef); err != nil {
		return nil, err
	}
	// 抓取 URL
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(req.SourceRef)
	if err != nil {
		return nil, fmt.Errorf("抓取 URL 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("URL 返回错误状态: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	content := string(body)
	// 简单 HTML 标签剥离(可后续增强)
	content = stripHTML(content)

	title := req.Title
	if title == "" {
		// 从 URL 提取
		title = filepath.Base(req.SourceRef)
		if idx := strings.Index(title, "?"); idx > 0 {
			title = title[:idx]
		}
	}
	tagsJSON, _ := json.Marshal(req.Tags)
	tmpDir := filepath.Join("uploads", "knowledge-url", req.ProductID, time.Now().Format("20060102"))
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("创建目录失败: %w", err)
	}
	textFile := filepath.Join(tmpDir, uuid.New().String()+".html")
	if err := os.WriteFile(textFile, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("写入URL内容失败: %w", err)
	}
	doc := &model.KnowledgeDocument{

		ProductID:   productNumericID,
		SourceType:  model.SourceTypeURL,
		SourceRef:   req.SourceRef,
		Title:       title,
		FileName:    title + ".html",
		FilePath:    textFile,
		FileType:    ".html",
		FileSize:    int64(len(content)),
		MimeType:    "text/html",
		EmbedStatus: model.EmbedStatusPending,
		Category:    req.Category,
		Tags:        string(tagsJSON),
		Metadata:    metaToJSON(req.Metadata),
		Status:      1,
	}
	if err := s.docRepo.Create(ctx, doc); err != nil {
		_ = os.Remove(textFile)
		return nil, fmt.Errorf("保存文档记录失败: %w", err)
	}
	return doc, nil
}

// ============================================================================
// 异步处理流水线
// ============================================================================

// metaToJSON 将附加字段映射为 jsonb 字符串；空/nil 时返回 "{}"。
func metaToJSON(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (s *KnowledgeService) processDocumentAsync(bgCtx context.Context, documentID uint64, productID int64, filePath, fileName, content, mimeType, title string, source model.SourceType, docMeta map[string]any) {
	// panic 兜底：异步 goroutine 内异常会导致文档永久 pending 且无法重试，必须 recover 并标记失败
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[knowledge] processDocumentAsync panic doc=%d: %v", documentID, r)
			s.markFailed(bgCtx, documentID, fmt.Sprintf("处理异常: %v", r))
		}
	}()
	// 1. 标记为处理中
	if err := s.docRepo.UpdateStatus(bgCtx, documentID, model.EmbedStatusProcessing, 10, ""); err != nil {
		logger.Errorf("[knowledge] 标记处理中失败: %v", err)
		return
	}

	// 2. 读取并提取文本
	//    关键修复：上传的 PDF/DOCX 是二进制，不能直接 string(bytes) 切片（会产出乱码）。
	//    先按文件名扩展名提取纯文本，再交给分片器。
	if content == "" && filePath != "" {
		bytes, err := os.ReadFile(filePath)
		if err != nil {
			s.markFailed(bgCtx, documentID, fmt.Sprintf("读取文件失败: %v", err))
			return
		}
		extracted, extErr := etl.ExtractText(fileName, bytes)
		if extErr != nil {
			// 解析失败(如扫描件PDF)退化为原始字节，避免整篇丢失
			logger.Warnf("[knowledge] 文本提取失败 doc=%d: %v，退化为原始内容", documentID, extErr)
			extracted = string(bytes)
		}
		content = extracted
	}
	if strings.TrimSpace(content) == "" {
		s.markFailed(bgCtx, documentID, "文档内容为空（可能为扫描件或空文件）")
		return
	}

	// 3. 分片
	ragDoc := rag_core.Document{
		ID:      fmt.Sprintf("kb_doc_%d", documentID),
		Content: content,
		Metadata: map[string]any{
			"document_id": documentID,
			"product_id":  productID,
			"title":       title,
			"source":      string(source),
		},
		CreatedAt: time.Now(),
	}
	chunks, err := s.processor.ProcessDocument(bgCtx, ragDoc)
	if err != nil {
		s.markFailed(bgCtx, documentID, fmt.Sprintf("分片失败: %v", err))
		return
	}
	if len(chunks) == 0 {
		s.markFailed(bgCtx, documentID, "分片结果为空")
		return
	}
	if err := s.docRepo.UpdateStatus(bgCtx, documentID, model.EmbedStatusProcessing, 30, ""); err != nil {
		logger.Errorf("[knowledge] 更新进度失败: %v", err)
	}

	// 4. 写入分段表（分片携带文档级基础信息 + 业务附加字段）
	baseMeta := map[string]any{
		"document_id": float64(documentID),
		"product_id":  float64(productID),
		"title":       title,
		"source":      string(source),
	}
	for k, v := range docMeta {
		baseMeta[k] = v
	}
	chunkModels := make([]model.KnowledgeChunk, 0, len(chunks))
	totalTokens := 0
	for idx, c := range chunks {
		hash := sha256.Sum256([]byte(c.Content))
		hashStr := hex.EncodeToString(hash[:])
		chunkMeta := map[string]any{"chunk_index": float64(idx)}
		for k, v := range baseMeta {
			chunkMeta[k] = v
		}
		metaBytes, _ := json.Marshal(chunkMeta)
		chunkModels = append(chunkModels, model.KnowledgeChunk{
			DocumentID: documentID,
			ProductID:  productID,

			ChunkIndex:  idx,
			Content:     c.Content,
			ContentHash: hashStr,
			TokenCount:  c.TokenCount,
			CharCount:   len(c.Content),
			Metadata:    string(metaBytes),
		})
		totalTokens += c.TokenCount
	}
	if err := s.chunkRepo.BatchCreate(bgCtx, chunkModels); err != nil {
		s.markFailed(bgCtx, documentID, fmt.Sprintf("写入分段失败: %v", err))
		return
	}
	if err := s.docRepo.UpdateStatus(bgCtx, documentID, model.EmbedStatusProcessing, 60, ""); err != nil {
		logger.Errorf("[knowledge] 更新进度失败: %v", err)
	}

	// 5. 向量化（per 知识库 embedding 配置优先，否则全局默认）
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embService, embCfg := s.resolveEmbeddingConfig(bgCtx, productID)
	embeddings, err := embService.Embed(bgCtx, embCfg, texts)
	if err != nil {
		s.markFailed(bgCtx, documentID, fmt.Sprintf("向量化失败: %v", err))
		return
	}
	if err := s.docRepo.UpdateStatus(bgCtx, documentID, model.EmbedStatusProcessing, 80, ""); err != nil {
		logger.Errorf("[knowledge] 更新进度失败: %v", err)
	}

	// 5.5 持久化向量到 knowledge_chunks.embedding（pgvector）
	// 关键修复：此前 embeddings 仅用于内存索引（生产 s.indexer==nil 被跳过），
	// 导致 knowledge_chunks.embedding 始终为空，检索侧 vectorSearch 读不到任何向量 → RAG 召回失效。
	if err := s.persistChunkEmbeddings(bgCtx, chunkModels, embeddings); err != nil {
		s.markFailed(bgCtx, documentID, fmt.Sprintf("持久化向量失败: %v", err))
		return
	}

	// 6. 入内存索引（生产 s.indexer==nil 跳过；真实检索走 pgvector，见 rag_searcher.go）
	if s.indexer != nil {
		idxChunks := make([]ragretrieval.Chunk, len(chunks))
		for i, c := range chunks {
			idxChunks[i] = ragretrieval.Chunk{
				ID:         c.ID,
				DocumentID: c.DocumentID,
				Content:    c.Content,
				Metadata:   c.Metadata,
				Embedding:  embeddings[i],
				Score:      c.Score,
				TokenCount: c.TokenCount,
				ChunkIndex: i,
			}
		}
		_ = s.indexer.BuildIndex(bgCtx, fmt.Sprintf("product_%d", productID), idxChunks)
	}

	// 7. 更新文档状态
	if err := s.docRepo.UpdateChunkStats(bgCtx, documentID, len(chunks), totalTokens); err != nil {
		logger.Errorf("[knowledge] 更新分段统计失败: %v", err)
	}
	if err := s.docRepo.UpdateStatus(bgCtx, documentID, model.EmbedStatusIndexed, 100, ""); err != nil {
		logger.Errorf("[knowledge] 更新已索引状态失败: %v", err)
	}

	// 8. 更新产品冗余字段
	docCount, _ := s.docRepo.CountByProduct(bgCtx, productID)
	chunkCount, _ := s.chunkRepo.CountByProductID(bgCtx, productID)
	now := time.Now()
	// 反查 UUID
	var productUUID string
	if s.ragRepo != nil {
		if prod, _ := s.ragRepo.FindRagProductByIDOnly(bgCtx, intToString(productID)); prod != nil {
			productUUID = prod.ID
		}
	}
	if productUUID != "" {
		_ = s.ragRepo.UpdateRagProductStats(bgCtx, productUUID, int(docCount), chunkCount, &now)
		// 注:UpdateRagProductStats 方法名保留以兼容旧逻辑(独立部署模式下不真正使用)
	}
}

func (s *KnowledgeService) markFailed(ctx context.Context, documentID uint64, errMsg string) {
	if err := s.docRepo.UpdateStatus(ctx, documentID, model.EmbedStatusFailed, 0, errMsg); err != nil {
		logger.Errorf("[knowledge] 标记失败状态错误: %v", err)
	}
}

func intToString(n int64) string {
	return fmt.Sprintf("%d", n)
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
			dim = 1024
		}
		cfg := &llm.EmbeddingConfig{
			APIType:        prod.EmbeddingProviderConfig.APIType,
			BaseURL:        prod.EmbeddingProviderConfig.BaseURL,
			Model:          prod.EmbeddingProviderConfig.Model,
			APIKey:         prod.EmbeddingProviderConfig.APIKey,
			Dimension:      dim,
			AllowFallback:  false,
			RequestTimeout: 60,
			MaxRetries:     2,
		}
		return llm.NewEmbeddingServiceWithConfig(cfg), cfg
	}
	return s.embeddingSvc, nil
}

// persistChunkEmbeddings 将向量写入 knowledge_chunks.embedding（pgvector vector(1024)），
// 并标记 embed_status='indexed'。这是检索侧 vectorSearch 能读到向量的前提。
func (s *KnowledgeService) persistChunkEmbeddings(ctx context.Context, chunks []model.KnowledgeChunk, embeddings [][]float32) error {
	if len(chunks) != len(embeddings) {
		return fmt.Errorf("分片数与向量数不一致: %d != %d", len(chunks), len(embeddings))
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, c := range chunks {
			vec := vecToPGString(embeddings[i])
			if err := tx.Exec(
				"UPDATE knowledge_chunks SET embedding = $1::vector, embed_status = 'indexed' WHERE id = $2",
				vec, c.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
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

// Reindex 重建单文档索引
//
// 2026-07-18 修复：KnowledgeDocument.ProductID 是 int64（迁移 schema 为 INTEGER），
// 调用方需将前端传入的 string UUID 经 HashStringToInt64 映射回 int64。
func (s *KnowledgeService) Reindex(ctx context.Context, productID, id int64) error {
	doc, err := s.docRepo.GetByProductAndID(ctx, productID, id)
	if err != nil {
		return err
	}
	uid := uint64(id)
	// 删除现有分段和索引
	if err := s.chunkRepo.DeleteByDocumentID(ctx, uid); err != nil {
		return err
	}
	// 重置状态
	if err := s.docRepo.UpdateStatus(ctx, uid, model.EmbedStatusPending, 0, ""); err != nil {
		return err
	}
	// 重新处理 - 由于 KnowledgeDocument 不存 Content 字段,从 FilePath 读取
	var content string
	if doc.FilePath != "" {
		if data, err := os.ReadFile(doc.FilePath); err == nil {
			content = string(data)
		}
	}
	go func(documentID uint64, prodID int64, filePath, contentStr, mimeType, title string, source model.SourceType) {
		// 整体超时兜底：embedding/索引服务不可达时，防止文档永久卡在 processing 状态
		procCtx, procCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer procCancel()
		docMeta := map[string]any{}
		s.processDocumentAsync(procCtx, documentID, prodID, filePath, doc.FileName, contentStr, mimeType, title, source, docMeta)
	}(doc.ID, doc.ProductID, doc.FilePath, content, doc.MimeType, doc.Title, doc.SourceType)
	return nil
}

// RebuildIndex 重建产品级索引
//
// 2026-07-18 修复：同 Reindex，productID 是 int64（迁移 schema 为 INTEGER）。
// 调用方需将前端传入的 string UUID 经 HashStringToInt64 映射回 int64。
func (s *KnowledgeService) RebuildIndex(ctx context.Context, productID int64) error {
	docs, _, err := s.docRepo.List(ctx, repository.ListFilter{

		ProductID: productID,
		PageSize:  10000,
	})
	if err != nil {
		return err
	}
	for _, d := range docs {
		if err := s.Reindex(ctx, productID, int64(d.ID)); err != nil {
			logger.Errorf("[knowledge] 重建文档 %d 失败: %v", d.ID, err)
		}
	}
	return nil
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

// ============================================================================
// 辅助
// ============================================================================

// logImport 记录导入日志
func (s *KnowledgeService) logImport(ctx context.Context, req *ImportRequest, docID uint64, status string, durationMs int, errMsg string) error {
	var docIDPtr *uint64
	if docID > 0 {
		docIDPtr = &docID
	}
	productNumericID := HashStringToInt64(req.ProductID)
	log := &model.KnowledgeImportLog{

		ProductID:   productNumericID,
		DocumentID:  docIDPtr,
		SourceType:  string(req.SourceType),
		BatchNo:     req.BatchNo,
		Status:      status,
		Operator:    req.Operator,
		IP:          req.IP,
		UserAgent:   req.UserAgent,
		DurationMs:  durationMs,
		ErrorDetail: errMsg,
	}
	return s.importLogRepo.Create(ctx, log)
}

// validateURL URL 校验（含 SSRF 防护）
//
// 防护策略：仅允许 http/https 协议，并对解析后的所有 IP 做内网/保留地址拦截。
// 注：DNS 重绑定（TOCTOU）无法靠单次解析完全消除，生产环境应配合出口防火墙 /
// 专用 egress proxy；此处拦截已覆盖绝大多数 SSRF 利用场景（如 169.254.169.254 元数据）。
func validateURL(rawURL string) error {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return errors.New("URL 必须以 http:// 或 https:// 开头")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL 解析失败: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("URL 缺少主机名")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("DNS 解析失败: %w", err)
	}
	if len(ips) == 0 {
		return errors.New("DNS 解析结果为空")
	}
	for _, ipAddr := range ips {
		ip := ipAddr.IP
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("禁止访问内网/保留地址: %s", ip.String())
		}
	}
	return nil
}

// stripHTML 简单 HTML 标签剥离
func stripHTML(html string) string {
	// 移除 script/style
	html = stripBetween(html, "<script", "</script>")
	html = stripBetween(html, "<style", "</style>")
	// 移除所有标签
	var result strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			result.WriteRune(' ')
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

func stripBetween(s, start, end string) string {
	for {
		si := strings.Index(s, start)
		if si < 0 {
			return s
		}
		ei := strings.Index(s[si:], end)
		if ei < 0 {
			return s
		}
		s = s[:si] + s[si+ei+len(end):]
	}
}

func getMimeType(ext string) string {
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".docx", ".doc":
		return "application/msword"
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".html":
		return "text/html"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	default:
		return "application/octet-stream"
	}
}

// dbGetDB 内部获取 DB(避免循环依赖)
func dbGetDB() *gorm.DB {
	return db.GetDB()
}
