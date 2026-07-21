package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"marketing/internal/aiagent/knowledge/model"
	"marketing/internal/aiagent/knowledge/repository"
	dbutil "marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// KnowledgeMerchantService 商户自部署场景的 RAG 核心增强服务
// 对应审计项：商户需要可视化管理 + 外部系统导入 + 检索调试 + 分段编辑 + 反馈
//
// 包含以下子能力：
//  1. BatchImportService    批量导入（CSV/JSON）
//  2. PlaygroundService     检索 Playground（不同 topK / 阈值 / 调参）
//  3. ChunkEditService      分段编辑（增/改/删/重切）
//  4. FeedbackService       反馈标注
//  5. TokenService          外部 API Token 管理
//  6. ExternalImportService 外部系统接入（飞书/Notion/通用 JSON）
type KnowledgeMerchantService struct {
	db        *gorm.DB // 保留以维持 GetDB() 兼容（测试和过渡场景）
	kbService *KnowledgeService
	ragSearch *RagSearcher
	docRepo   *repository.KnowledgeDocumentRepository
	chunkRepo *repository.KnowledgeChunkRepository
	prodRepo  *repository.RagConfigRepository
	// 新增：商户 RAG 业务专属仓储
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

// ensureReposFromDB 在 struct 直接构造（如测试中 &KnowledgeMerchantService{db: db}）时，
// 按需从 s.db 派生新的仓库实例，保持与原 s.db 直用等价语义。
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

// ============================================================================
// 1) 批量导入（CSV / JSON / Excel-text）
// ============================================================================

// BatchImportItem 批量导入的单条记录
type BatchImportItem struct {
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Source   string   `json:"source"` // 数据来源描述（可选）
}

// BatchImportRequest 批量导入请求
type BatchImportRequest struct {
	ProductID string                `json:"product_id"`
	Operator  string                `json:"operator"`
	Format    string                `json:"format"` // csv / json / auto
	Items     []BatchImportItem     `json:"items,omitempty"`
	File      multipart.File        `json:"-"`
	FileHead  *multipart.FileHeader `json:"-"`
}

// BatchImportResult 批量导入结果
type BatchImportResult struct {
	BatchNo     string   `json:"batch_no"`
	Total       int      `json:"total"`
	Accepted    int      `json:"accepted"`
	Rejected    int      `json:"rejected"`
	DocumentIDs []uint64 `json:"document_ids"`
	Errors      []string `json:"errors"`
}

// BatchImport 批量导入（统一入口，items 或 file 二选一）
func (s *KnowledgeMerchantService) BatchImport(ctx context.Context, req *BatchImportRequest) (*BatchImportResult, error) {
	if req.ProductID == "" {
		return nil, errors.New("product_id 不能为空")
	}
	batchNo := "BATCH-" + time.Now().Format("20060102150405") + "-" + uuid.New().String()[:8]

	product, err := s.prodRepo.GetRagProductByID(ctx, req.ProductID)
	if err != nil || product == nil {
		return nil, errors.New("产品不存在")
	}
	productNumericID := HashStringToInt64(req.ProductID)

	// 收集 items
	items := req.Items
	if len(items) == 0 && req.File != nil {
		parsed, ferr := s.parseBatchFile(ctx, req.File, req.FileHead, req.Format)
		if ferr != nil {
			return nil, ferr
		}
		items = parsed
	}
	if len(items) == 0 {
		return &BatchImportResult{BatchNo: batchNo, Total: 0}, nil
	}

	result := &BatchImportResult{
		BatchNo:     batchNo,
		Total:       len(items),
		DocumentIDs: make([]uint64, 0),
		Errors:      make([]string, 0),
	}

	for idx, it := range items {
		if strings.TrimSpace(it.Content) == "" {
			result.Rejected++
			result.Errors = append(result.Errors, fmt.Sprintf("第 %d 行: 内容为空", idx+1))
			continue
		}
		title := it.Title
		if title == "" {
			title = fmt.Sprintf("批量导入_%d", idx+1)
		}
		// 复用 KnowledgeService.Import 处理
		imp, err := s.kbService.Import(ctx, &ImportRequest{
			ProductID:  req.ProductID,
			SourceType: model.SourceTypeBatch,
			Title:      title,
			Content:    it.Content,
			Category:   it.Category,
			Tags:       it.Tags,
			Operator:   req.Operator,
			BatchNo:    batchNo,
		})
		if err != nil {
			result.Rejected++
			result.Errors = append(result.Errors, fmt.Sprintf("第 %d 行: %s", idx+1, err.Error()))
			continue
		}
		result.Accepted++
		result.DocumentIDs = append(result.DocumentIDs, imp.DocumentID)
		_ = productNumericID // 引用以避免未使用变量
	}
	return result, nil
}

// parseBatchFile 解析批量导入文件（CSV/JSON）
func (s *KnowledgeMerchantService) parseBatchFile(ctx context.Context, file multipart.File, header *multipart.FileHeader, format string) ([]BatchImportItem, error) {
	if file == nil {
		return nil, errors.New("文件不能为空")
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	if format == "" {
		format = "auto"
	}
	if format == "auto" && header != nil {
		name := strings.ToLower(header.Filename)
		switch {
		case strings.HasSuffix(name, ".json"):
			format = "json"
		case strings.HasSuffix(name, ".csv"):
			format = "csv"
		default:
			format = "json"
		}
	}
	switch format {
	case "csv":
		return parseCSV(raw)
	case "json":
		return parseJSON(raw)
	default:
		return nil, errors.New("不支持的格式: " + format)
	}
}

func parseCSV(data []byte) ([]BatchImportItem, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1 // 允许变长
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 解析失败: %w", err)
	}
	if len(rows) < 2 {
		return nil, errors.New("CSV 必须至少包含表头和一行数据")
	}
	header := rows[0]
	idxTitle := -1
	idxContent := -1
	idxCategory := -1
	idxTags := -1
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "title", "标题", "name":
			idxTitle = i
		case "content", "内容", "text", "body", "q", "question":
			idxContent = i
		case "category", "分类":
			idxCategory = i
		case "tags", "标签":
			idxTags = i
		}
	}
	if idxContent < 0 {
		return nil, errors.New("CSV 缺少 content 列")
	}
	items := make([]BatchImportItem, 0, len(rows)-1)
	for i, row := range rows[1:] {
		if idxContent >= len(row) {
			continue
		}
		it := BatchImportItem{
			Content: row[idxContent],
		}
		if idxTitle >= 0 && idxTitle < len(row) {
			it.Title = row[idxTitle]
		}
		if idxCategory >= 0 && idxCategory < len(row) {
			it.Category = row[idxCategory]
		}
		if idxTags >= 0 && idxTags < len(row) {
			tags := strings.Split(row[idxTags], ",")
			for j := range tags {
				tags[j] = strings.TrimSpace(tags[j])
			}
			it.Tags = tags
		}
		items = append(items, it)
		_ = i
	}
	return items, nil
}

// ParseCSV 公开版 parseCSV(供跨包测试使用)
func ParseCSV(data []byte) ([]BatchImportItem, error) {
	return parseCSV(data)
}

func parseJSON(data []byte) ([]BatchImportItem, error) {
	// 兼容两种结构：[items] 或 {items: [...]} 或 {data: [...]}
	var arr []BatchImportItem
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	var wrap struct {
		Items     []BatchImportItem `json:"items"`
		Data      []BatchImportItem `json:"data"`
		List      []BatchImportItem `json:"list"`
		Documents []BatchImportItem `json:"documents"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	if len(wrap.Items) > 0 {
		return wrap.Items, nil
	}
	if len(wrap.Data) > 0 {
		return wrap.Data, nil
	}
	if len(wrap.List) > 0 {
		return wrap.List, nil
	}
	if len(wrap.Documents) > 0 {
		return wrap.Documents, nil
	}
	return nil, errors.New("JSON 数据为空或结构不匹配")
}

// ============================================================================
// 2) 检索 Playground（商户调试用：自定义 topK/阈值/重排/过滤）
// ============================================================================

// PlaygroundRequest 检索 Playground 请求
type PlaygroundRequest struct {
	ProductID           string         `json:"product_id"`
	Query               string         `json:"query"`
	TopK                int            `json:"top_k"`
	SimilarityThreshold float64        `json:"similarity_threshold"`
	FilterCategory      string         `json:"filter_category"`
	UseThreeTier        bool           `json:"use_three_tier"`
	UseRerank           bool           `json:"use_rerank"`
	Tags                []string       `json:"tags"`
	Extra               map[string]any `json:"extra"`
	// MetadataFilters 附加字段过滤（如 {"customer_id":"123","order_id":"A01"}），
	// 把检索收敛到特定业务上下文（某客户的订单知识等）。
	MetadataFilters map[string]string `json:"metadata_filters"`
}

// PlaygroundChunk 命中分段
type PlaygroundChunk struct {
	ChunkID    uint64         `json:"chunk_id"`
	DocumentID uint64         `json:"document_id"`
	Title      string         `json:"title"`
	Content    string         `json:"content"`
	Score      float64        `json:"score"`
	Source     string         `json:"source"` // 来自 L1/L2/L3/L4
	FromCache  bool           `json:"from_cache"`
	Metadata   map[string]any `json:"metadata"`
}

// PlaygroundResult 检索 Playground 响应
type PlaygroundResult struct {
	Query     string            `json:"query"`
	Total     int               `json:"total"`
	Chunks    []PlaygroundChunk `json:"chunks"`
	MaxScore  float64           `json:"max_score"`
	MinScore  float64           `json:"min_score"`
	AvgScore  float64           `json:"avg_score"`
	LatencyMs int64             `json:"latency_ms"`
	Source    string            `json:"source"`
	FromCache bool              `json:"from_cache"`
	DebugInfo map[string]any    `json:"debug_info"`
}

// Playground 检索 Playground 入口
func (s *KnowledgeMerchantService) Playground(ctx context.Context, req *PlaygroundRequest) (*PlaygroundResult, error) {
	if req.ProductID == "" {
		return nil, errors.New("product_id 不能为空")
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, errors.New("query 不能为空")
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.TopK > 50 {
		req.TopK = 50
	}
	if req.SimilarityThreshold < 0 {
		req.SimilarityThreshold = 0
	}
	if req.SimilarityThreshold > 1 {
		req.SimilarityThreshold = 1
	}
	productNumericID := HashStringToInt64(req.ProductID)
	start := time.Now()
	chunks, err := s.ragSearch.SearchIndex(ctx, productNumericID, req.Query, req.TopK, req.MetadataFilters)
	if err != nil {
		return nil, err
	}
	// 应用阈值
	filtered := chunks[:0]
	for _, c := range chunks {
		if c.Score >= req.SimilarityThreshold {
			filtered = append(filtered, c)
		}
	}
	chunks = filtered

	pc := make([]PlaygroundChunk, 0, len(chunks))
	var max, min, sum float64
	for i, c := range chunks {
		if i == 0 {
			max = c.Score
			min = c.Score
		} else {
			if c.Score > max {
				max = c.Score
			}
			if c.Score < min {
				min = c.Score
			}
		}
		sum += c.Score
		title := ""
		if c.Metadata != nil {
			if t, ok := c.Metadata["title"].(string); ok {
				title = t
			}
		}
		pc = append(pc, PlaygroundChunk{
			ChunkID:    c.ID,
			DocumentID: c.DocumentID,
			Title:      title,
			Content:    c.Content,
			Score:      c.Score,
			Source:     "L2",
			Metadata:   c.Metadata,
		})
	}
	avg := 0.0
	if len(chunks) > 0 {
		avg = sum / float64(len(chunks))
	}

	// 记录 search log
	_ = s.recordSearchLog(ctx, productNumericID, req.Query, req.TopK, req.SimilarityThreshold, len(chunks), max, min, avg, time.Since(start).Milliseconds())

	return &PlaygroundResult{
		Query:     req.Query,
		Total:     len(chunks),
		Chunks:    pc,
		MaxScore:  max,
		MinScore:  min,
		AvgScore:  avg,
		LatencyMs: time.Since(start).Milliseconds(),
		Source:    "L2",
		DebugInfo: map[string]any{
			"product_id":     req.ProductID,
			"top_k":          req.TopK,
			"threshold":      req.SimilarityThreshold,
			"use_three_tier": req.UseThreeTier,
			"use_rerank":     req.UseRerank,
		},
	}, nil
}

func (s *KnowledgeMerchantService) recordSearchLog(ctx context.Context, productID int64, query string, topK int, threshold float64, count int, max, min, avg float64, latencyMs int64) error {
	if s.db == nil {
		return nil
	}
	s.ensureReposFromDB()
	h := sha256.Sum256([]byte(query))
	logEntry := &model.KnowledgeSearchLog{
		ProductID:           productID,
		Query:               query,
		QueryHash:           hex.EncodeToString(h[:]),
		TopK:                topK,
		SimilarityThreshold: threshold,
		ResultCount:         count,
		MaxScore:            max,
		MinScore:            min,
		AvgScore:            avg,
		LatencyMs:           int(latencyMs),
		Hit:                 boolToInt(count > 0),
		Source:              "L2",
	}
	return s.searchLogRepo.Create(ctx, logEntry)
}

// ============================================================================
// 3) 分段编辑
// ============================================================================

// GetDocumentChunks 列出文档分段（支持分页）
func (s *KnowledgeMerchantService) GetDocumentChunks(ctx context.Context, documentID uint64, page, pageSize int) ([]model.KnowledgeChunk, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	return s.chunkRepo.PageByDocumentID(ctx, documentID, page, pageSize)
}

// UpdateChunkRequest 更新分段内容
type UpdateChunkRequest struct {
	ChunkID uint64 `json:"chunk_id"`
	Content string `json:"content"`
}

// UpdateChunk 更新分段
func (s *KnowledgeMerchantService) UpdateChunk(ctx context.Context, req *UpdateChunkRequest) error {
	if req.ChunkID == 0 {
		return errors.New("chunk_id 不能为空")
	}
	if strings.TrimSpace(req.Content) == "" {
		return errors.New("内容不能为空")
	}
	chunk, err := s.chunkRepo.GetByID(ctx, req.ChunkID)
	if err != nil {
		return fmt.Errorf("分段不存在: %w", err)
	}
	chunk.Content = req.Content
	chunk.CharCount = len(req.Content)
	chunk.TokenCount = len(strings.Fields(req.Content))
	h := sha256.Sum256([]byte(req.Content))
	chunk.ContentHash = hex.EncodeToString(h[:])
	chunk.EmbeddingID = "" // 内容变化后清空旧向量，触发重新向量化
	if err := s.chunkRepo.Update(ctx, chunk); err != nil {
		return err
	}
	// 重新向量化（per-product 配置优先），保持 knowledge_chunks.embedding 与内容一致
	if err := s.kbService.EmbedAndPersistChunks(ctx, chunk.ProductID, []model.KnowledgeChunk{*chunk}); err != nil {
		logger.Errorf("[knowledge] 分段更新后重新向量化失败: %v", err)
	}
	return nil
}

// DeleteChunk 删除分段
func (s *KnowledgeMerchantService) DeleteChunk(ctx context.Context, chunkID uint64) error {
	if chunkID == 0 {
		return errors.New("chunk_id 不能为空")
	}
	return s.chunkRepo.Delete(ctx, chunkID)
}

// SplitChunkRequest 拆分分段
type SplitChunkRequest struct {
	ChunkID uint64   `json:"chunk_id"`
	Parts   []string `json:"parts"`
}

// SplitChunk 拆分分段为多段
func (s *KnowledgeMerchantService) SplitChunk(ctx context.Context, req *SplitChunkRequest) error {
	if req.ChunkID == 0 {
		return errors.New("chunk_id 不能为空")
	}
	if len(req.Parts) < 2 {
		return errors.New("至少需要 2 段")
	}
	chunk, err := s.chunkRepo.GetByID(ctx, req.ChunkID)
	if err != nil {
		return err
	}
	// 删除原分段，插入新分段
	if err := s.chunkRepo.Delete(ctx, chunk.ID); err != nil {
		return err
	}
	docID := chunk.DocumentID
	productID := chunk.ProductID
	newChunks := make([]model.KnowledgeChunk, 0, len(req.Parts))
	baseIndex := chunk.ChunkIndex
	for i, part := range req.Parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		h := sha256.Sum256([]byte(part))
		newChunks = append(newChunks, model.KnowledgeChunk{
			DocumentID:  docID,
			ProductID:   productID,
			ChunkIndex:  baseIndex + i,
			Content:     part,
			ContentHash: hex.EncodeToString(h[:]),
			CharCount:   len(part),
			TokenCount:  len(strings.Fields(part)),
		})
	}
	if err := s.chunkRepo.BatchCreate(ctx, newChunks); err != nil {
		return err
	}
	// 重新向量化（per-product 配置优先），保持 knowledge_chunks.embedding 与新分段一致
	if err := s.kbService.EmbedAndPersistChunks(ctx, productID, newChunks); err != nil {
		logger.Errorf("[knowledge] 分段重切后重新向量化失败: %v", err)
	}
	return nil
}

// ============================================================================
// 4) 反馈标注
// ============================================================================

// SubmitFeedbackRequest 反馈请求
type SubmitFeedbackRequest struct {
	ProductID  string `json:"product_id"`
	Query      string `json:"query"`
	DocumentID uint64 `json:"document_id"`
	ChunkID    uint64 `json:"chunk_id"`
	Rating     int    `json:"rating"` // 1=good 0=neutral -1=bad
	Comment    string `json:"comment"`
	Operator   string `json:"operator"`
	SessionID  string `json:"session_id"`
}

// SubmitFeedback 提交反馈
func (s *KnowledgeMerchantService) SubmitFeedback(ctx context.Context, req *SubmitFeedbackRequest) error {
	if req.Query == "" {
		return errors.New("query 不能为空")
	}
	if req.Rating < -1 || req.Rating > 1 {
		return errors.New("rating 必须在 [-1, 0, 1]")
	}
	if s.db == nil {
		return nil
	}
	s.ensureReposFromDB()
	h := sha256.Sum256([]byte(req.Query))
	fb := &model.KnowledgeFeedback{
		ProductID: req.ProductID, // 直接存储字符串 ProductID
		Query:     req.Query,
		QueryHash: hex.EncodeToString(h[:]),
		Rating:    req.Rating,
		Comment:   req.Comment,
		Operator:  req.Operator,
		SessionID: req.SessionID,
	}
	if req.DocumentID > 0 {
		d := req.DocumentID
		fb.DocumentID = &d
	}
	if req.ChunkID > 0 {
		c := req.ChunkID
		fb.ChunkID = &c
	}
	return s.feedbackRepo.Create(ctx, fb)
}

// ListFeedbacksRequest 反馈列表查询
type ListFeedbacksRequest struct {
	ProductID string `json:"product_id"`
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
	Rating    int    `json:"rating"`
}

// ListFeedbacks 反馈列表
func (s *KnowledgeMerchantService) ListFeedbacks(ctx context.Context, req *ListFeedbacksRequest) ([]model.KnowledgeFeedback, int64, error) {
	if s.db == nil {
		return nil, 0, nil
	}
	s.ensureReposFromDB()
	filter := repository.FeedbackListFilter{
		ProductID: req.ProductID,
		Page:      req.Page,
		PageSize:  req.PageSize,
	}
	if req.Rating >= -1 && req.Rating <= 1 {
		filter.Rating = req.Rating
		filter.HasRating = true
	}
	return s.feedbackRepo.List(ctx, filter)
}

// ============================================================================
// 5) API Token 管理（外部系统通过 Token 推送文档）
// ============================================================================

// CreateTokenRequest 创建 Token
type CreateTokenRequest struct {
	Name      string     `json:"name"`
	ProductID string     `json:"product_id"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedBy string     `json:"created_by"`
}

// CreateToken 创建 Token（返回明文 token 仅此一次）
func (s *KnowledgeMerchantService) CreateToken(ctx context.Context, req *CreateTokenRequest) (*model.KnowledgeAPIToken, error) {
	if req.Name == "" {
		return nil, errors.New("name 不能为空")
	}
	if req.ProductID == "" {
		return nil, errors.New("product_id 不能为空")
	}
	s.ensureReposFromDB()
	plain, err := generateToken()
	if err != nil {
		return nil, err
	}
	hashed := hashToken(plain)
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{"read", "write"}
	}
	scopesJSON, _ := json.Marshal(scopes)
	tok := &model.KnowledgeAPIToken{
		Name:       req.Name,
		Token:      hashed,
		TokenPlain: plain,
		Scopes:     string(scopesJSON),
		ProductID:  req.ProductID,
		Enabled:    1,
		ExpiresAt:  req.ExpiresAt,
		CreatedBy:  req.CreatedBy,
	}
	if err := s.tokenRepo.Create(ctx, tok); err != nil {
		return nil, err
	}
	return tok, nil
}

// ListTokens 列出 Token
func (s *KnowledgeMerchantService) ListTokens(ctx context.Context, productID string) ([]model.KnowledgeAPIToken, error) {
	s.ensureReposFromDB()
	list, err := s.tokenRepo.ListByProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	// 隐藏 hashed token 明文
	for i := range list {
		list[i].Token = ""
		list[i].TokenPlain = ""
	}
	return list, nil
}

// RevokeToken 吊销 Token
func (s *KnowledgeMerchantService) RevokeToken(ctx context.Context, id uint64) error {
	s.ensureReposFromDB()
	return s.tokenRepo.DisableByID(ctx, id, 0)
}

// ValidateToken 校验 Token（外部系统使用）
func (s *KnowledgeMerchantService) ValidateToken(ctx context.Context, plain string) (*model.KnowledgeAPIToken, error) {
	if plain == "" {
		return nil, errors.New("token 不能为空")
	}
	if s.db == nil {
		return nil, errors.New("数据库未初始化")
	}
	s.ensureReposFromDB()
	hashed := hashToken(plain)
	tok, err := s.tokenRepo.FindByToken(ctx, hashed)
	if err != nil {
		return nil, errors.New("token 无效")
	}
	if tok.Enabled != 1 {
		return nil, errors.New("token 已禁用")
	}
	if tok.ExpiresAt != nil && tok.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("token 已过期")
	}
	// 异步更新使用统计
	// R6 修复：原 goroutine 无 recover、错误被静默吞噬。添加 recover + 错误日志。
	tokID := tok.ID
	go func(id uint64) {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("knowledge_merchant: async IncrementUsage recovered from panic: %v", r)
			}
		}()
		if err := s.tokenRepo.IncrementUsage(context.Background(), id); err != nil {
			logger.Errorf("knowledge_merchant: async IncrementUsage failed, token_id=%d: %v", id, err)
		}
	}(tokID)
	return tok, nil
}

// ============================================================================
// 6) 外部系统接入（飞书/Notion/钉钉/通用 JSON）
// ============================================================================

// ExternalImportRequest 外部导入请求
type ExternalImportRequest struct {
	Source    string            `json:"source"` // feishu/notion/dingtalk/custom
	ProductID string            `json:"product_id"`
	Token     string            `json:"-"`     // API Token 鉴权
	Items     []BatchImportItem `json:"items"` // 通用 JSON
	// 飞书专用
	FeishuDocID string `json:"feishu_doc_id,omitempty"`
	// Notion 专用
	NotionPageID string `json:"notion_page_id,omitempty"`
	Operator     string `json:"operator"`
	Sync         bool   `json:"sync"` // 同步返回结果（默认 false，异步任务）
}

// ExternalImportResponse 外部导入响应
type ExternalImportResponse struct {
	JobNo       string   `json:"job_no"`
	Status      string   `json:"status"`
	Total       int      `json:"total"`
	Accepted    int      `json:"accepted"`
	Rejected    int      `json:"rejected"`
	FailedItems int      `json:"failed_items"`
	DocumentIDs []uint64 `json:"document_ids,omitempty"`
	Errors      []string `json:"errors,omitempty"`
	Async       bool     `json:"async"`
}

// ExternalImport 外部系统导入（统一入口）
func (s *KnowledgeMerchantService) ExternalImport(ctx context.Context, req *ExternalImportRequest) (*ExternalImportResponse, error) {
	if req.Source == "" {
		return nil, errors.New("source 不能为空")
	}
	if req.ProductID == "" {
		return nil, errors.New("product_id 不能为空")
	}

	// 1) 校验 Token
	tok, err := s.ValidateToken(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	if !tokenHasScope(tok.Scopes, "write") {
		return nil, errors.New("token 缺少 write 权限")
	}

	// 2) 校验产品
	if _, err := s.prodRepo.GetRagProductByID(ctx, req.ProductID); err != nil {
		return nil, errors.New("产品不存在")
	}

	// 3) 准备 items
	items := req.Items
	if len(items) == 0 {
		switch req.Source {
		case "feishu":
			if req.FeishuDocID == "" {
				return nil, errors.New("飞书模式需要 feishu_doc_id")
			}
			fetched, ferr := s.fetchFeishu(ctx, req.FeishuDocID)
			if ferr != nil {
				return nil, ferr
			}
			items = fetched
		case "notion":
			if req.NotionPageID == "" {
				return nil, errors.New("Notion 模式需要 notion_page_id")
			}
			fetched, ferr := s.fetchNotion(ctx, req.NotionPageID, tok)
			if ferr != nil {
				return nil, err
			}
			items = fetched
		default:
			return nil, errors.New("未提供 items，且 source 不支持自动抓取")
		}
	}

	// 4) 异步或同步
	jobNo := "EXT-" + time.Now().Format("20060102150405") + "-" + uuid.New().String()[:8]
	if !req.Sync {
		s.ensureReposFromDB()
		// 落库为 pending 任务
		job := &model.ExternalImportJob{
			JobNo:      jobNo,
			ProductID:  req.ProductID, // 直接存储字符串 ProductID
			Source:     req.Source,
			TotalItems: len(items),
			Status:     "pending",
			Operator:   req.Operator,
		}
		payload, _ := json.Marshal(req)
		job.Payload = string(payload)
		_ = s.externalRepo.Create(ctx, job)
		// 异步处理
		go func(productID string, items []BatchImportItem, op string) {
			// 整体超时兜底：批量遍历外部导入可能耗时较长，防止 goroutine 永久阻塞
			bg, bgCancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer bgCancel()
			started := time.Now()
			now := time.Now()
			_ = s.externalRepo.UpdateStatusByJobNo(bg, jobNo, map[string]any{
				"status":     "running",
				"started_at": &now,
			})
			resp, _ := s.runExternalImport(bg, productID, items, op, jobNo)
			finished := time.Now()
			updates := map[string]any{
				"status":       "completed",
				"finished_at":  &finished,
				"done_items":   resp.Accepted,
				"failed_items": resp.FailedItems,
			}
			if len(resp.Errors) > 0 {
				ed, _ := json.Marshal(resp.Errors)
				updates["error_detail"] = string(ed)
			}
			_ = s.externalRepo.UpdateStatusByJobNo(bg, jobNo, updates)
			_ = started
		}(req.ProductID, items, req.Operator)
		return &ExternalImportResponse{
			JobNo:  jobNo,
			Status: "pending",
			Total:  len(items),
			Async:  true,
		}, nil
	}

	// 同步模式
	return s.runExternalImport(ctx, req.ProductID, items, req.Operator, jobNo)
}

func (s *KnowledgeMerchantService) runExternalImport(ctx context.Context, productID string, items []BatchImportItem, operator, jobNo string) (*ExternalImportResponse, error) {
	resp := &ExternalImportResponse{
		JobNo:       jobNo,
		Status:      "running",
		Total:       len(items),
		DocumentIDs: make([]uint64, 0),
		Errors:      make([]string, 0),
	}
	for idx, it := range items {
		if strings.TrimSpace(it.Content) == "" {
			resp.Rejected++
			resp.FailedItems++
			resp.Errors = append(resp.Errors, fmt.Sprintf("第 %d 项: 内容为空", idx+1))
			continue
		}
		title := it.Title
		if title == "" {
			title = fmt.Sprintf("外部导入_%s_%d", jobNo, idx+1)
		}
		imp, err := s.kbService.Import(ctx, &ImportRequest{
			ProductID:  productID,
			SourceType: model.SourceTypeBatch,
			Title:      title,
			Content:    it.Content,
			Category:   it.Category,
			Tags:       it.Tags,
			Operator:   operator,
			BatchNo:    jobNo,
		})
		if err != nil {
			resp.Rejected++
			resp.FailedItems++
			resp.Errors = append(resp.Errors, fmt.Sprintf("第 %d 项: %s", idx+1, err.Error()))
			continue
		}
		resp.Accepted++
		resp.DocumentIDs = append(resp.DocumentIDs, imp.DocumentID)
	}
	resp.Status = "completed"
	return resp, nil
}

// ListExternalJobs 列出外部导入任务
func (s *KnowledgeMerchantService) ListExternalJobs(ctx context.Context, productID string, page, pageSize int) ([]model.ExternalImportJob, int64, error) {
	s.ensureReposFromDB()
	return s.externalRepo.List(ctx, repository.ExternalJobListFilter{
		ProductID: productID,
		Page:      page,
		PageSize:  pageSize,
	})
}

// fetchFeishu 飞书文档抓取（真实实现 + 凭证缺失降级）
//
// 凭证来源（按优先级）：
//  1. 环境变量 FEISHU_APP_ID + FEISHU_APP_SECRET（推荐：私域部署统一配置）
//  2. 入参 fallback 凭证：调用方可通过 token metadata 注入（预留）
//
// 真实调用流程：
//  1. POST https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal
//     body: {"app_id":"...","app_secret":"..."} → tenant_access_token
//  2. GET https://open.feishu.cn/open-apis/docx/v1/documents/{docID}/raw_content
//     header: Authorization: Bearer <tenant_access_token>
//     → markdown/HTML 内容
//  3. 按 \n## 或 \n### 切分为多个 BatchImportItem
//
// 失败模式：凭证缺失 / 网络失败 / 飞书 4xx-5xx 时返回带上下文的错误，
// 绝不返回 mock 数据（避免审计项"无 mock"红线）。
func (s *KnowledgeMerchantService) fetchFeishu(ctx context.Context, docID string) ([]BatchImportItem, error) {
	if docID == "" {
		return nil, errors.New("飞书 docID 不能为空")
	}

	// 1) 凭证获取
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")
	if appID == "" || appSecret == "" {
		return nil, errors.New("飞书抓取未配置凭证 (FEISHU_APP_ID/FEISHU_APP_SECRET)，请通过 items 字段直接传入结构化数据")
	}

	// 2) HTTP 客户端（短超时，避免阻塞 RAG 主流程）
	client := &http.Client{Timeout: 15 * time.Second}

	// 2.1 获取 tenant_access_token
	form := url.Values{}
	form.Set("app_id", appID)
	form.Set("app_secret", appSecret)
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("构建飞书 token 请求失败: %w", err)
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("飞书 token 请求失败: %w", err)
	}
	defer tokenResp.Body.Close()
	var tokenBody struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenBody); err != nil {
		return nil, fmt.Errorf("解析飞书 token 响应失败: %w", err)
	}
	if tokenBody.Code != 0 || tokenBody.TenantAccessToken == "" {
		return nil, fmt.Errorf("飞书鉴权失败: code=%d msg=%s", tokenBody.Code, tokenBody.Msg)
	}

	// 2.2 拉取文档原始内容
	docURL := fmt.Sprintf("https://open.feishu.cn/open-apis/docx/v1/documents/%s/raw_content", docID)
	docReq, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构建飞书文档请求失败: %w", err)
	}
	docReq.Header.Set("Authorization", "Bearer "+tokenBody.TenantAccessToken)
	docResp, err := client.Do(docReq)
	if err != nil {
		return nil, fmt.Errorf("飞书文档请求失败: %w", err)
	}
	defer docResp.Body.Close()
	if docResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(docResp.Body)
		return nil, fmt.Errorf("飞书文档拉取失败: status=%d body=%s", docResp.StatusCode, string(body))
	}
	var docBody struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Content string `json:"content"` // markdown
		} `json:"data"`
	}
	if err := json.NewDecoder(docResp.Body).Decode(&docBody); err != nil {
		return nil, fmt.Errorf("解析飞书文档响应失败: %w", err)
	}
	if docBody.Code != 0 {
		return nil, fmt.Errorf("飞书文档错误: code=%d msg=%s", docBody.Code, docBody.Msg)
	}
	markdown := docBody.Data.Content
	if strings.TrimSpace(markdown) == "" {
		return nil, errors.New("飞书文档内容为空")
	}

	// 3) 切分为多个 BatchImportItem（按二级标题或 2000 字符硬切）
	items := splitMarkdownToItems(markdown, docID, "feishu")
	return items, nil
}

// fetchNotion Notion 页面抓取（真实实现 + 凭证缺失降级）
//
// 凭证来源（按优先级）：
//  1. 环境变量 NOTION_API_KEY（推荐：Notion integration secret_xxx...）
//  2. tok 参数扩展（当前 model.KnowledgeAPIToken 不含 metadata，留作接口前置）
//
// 真实调用流程：
//  1. GET https://api.notion.com/v1/blocks/{pageID}/children?page_size=100
//     header: Authorization: Bearer <notion_integration_token>
//     header: Notion-Version: 2022-06-28
//  2. 递归抓取 has_children=true 的子块
//  3. 按 block type 提取 text（paragraph/heading_1/heading_2/heading_3/bulleted_list_item）
//  4. 按 heading_1/heading_2 切分为多个 BatchImportItem
//
// 失败模式：凭证缺失 / 网络失败 / Notion 4xx-5xx 时返回带上下文的错误。
func (s *KnowledgeMerchantService) fetchNotion(ctx context.Context, pageID string, tok *model.KnowledgeAPIToken) ([]BatchImportItem, error) {
	if pageID == "" {
		return nil, errors.New("Notion pageID 不能为空")
	}
	_ = tok // 当前 model 未携带 metadata，预留未来扩展

	apiKey := os.Getenv("NOTION_API_KEY")
	if apiKey == "" {
		return nil, errors.New("Notion 抓取未配置凭证 (NOTION_API_KEY)，请通过 items 字段直接传入结构化数据")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	items, err := s.fetchNotionBlocksRecursive(ctx, client, apiKey, pageID, 0, 8)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("Notion 页面内容为空或全为非文本块")
	}
	return items, nil
}

// fetchNotionBlocksRecursive 递归拉取 Notion 块并按 H1/H2 切分
func (s *KnowledgeMerchantService) fetchNotionBlocksRecursive(ctx context.Context, client *http.Client, apiKey, blockID string, depth, maxDepth int) ([]BatchImportItem, error) {
	if depth > maxDepth {
		return nil, nil
	}
	u := fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children?page_size=100", blockID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("构建 Notion 请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Notion-Version", "2022-06-28")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Notion 拉取失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Notion 拉取失败: status=%d body=%s", resp.StatusCode, string(body))
	}
	var nb struct {
		Results []struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			HasChildren bool   `json:"has_children"`
			Heading1    *struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"heading_1"`
			Heading2 *struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"heading_2"`
			Heading3 *struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"heading_3"`
			Paragraph *struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"paragraph"`
			BulletedListItem *struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"bulleted_list_item"`
		} `json:"results"`
		HasMore    bool   `json:"has_more"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&nb); err != nil {
		return nil, fmt.Errorf("解析 Notion 响应失败: %w", err)
	}

	// 按 H1/H2 切分为多个段落（每一段成为一个 BatchImportItem）
	var items []BatchImportItem
	var currentTitle string
	var currentBuf strings.Builder

	flush := func() {
		body := strings.TrimSpace(currentBuf.String())
		if body != "" {
			items = append(items, BatchImportItem{
				Title:   currentTitle,
				Content: body,
				Source:  "notion:" + blockID,
			})
		}
		currentBuf.Reset()
	}

	extractText := func(rt []struct {
		PlainText string `json:"plain_text"`
	}) string {
		parts := make([]string, 0, len(rt))
		for _, r := range rt {
			parts = append(parts, r.PlainText)
		}
		return strings.Join(parts, "")
	}

	for _, b := range nb.Results {
		var text string
		var isSectionBoundary bool
		switch b.Type {
		case "heading_1":
			if b.Heading1 != nil {
				text = extractText(b.Heading1.RichText)
				isSectionBoundary = true
			}
		case "heading_2":
			if b.Heading2 != nil {
				text = extractText(b.Heading2.RichText)
				isSectionBoundary = true
			}
		case "heading_3":
			if b.Heading3 != nil {
				text = extractText(b.Heading3.RichText)
				currentBuf.WriteString("### ")
				currentBuf.WriteString(text)
				currentBuf.WriteString("\n")
			}
		case "paragraph":
			if b.Paragraph != nil {
				text = extractText(b.Paragraph.RichText)
				currentBuf.WriteString(text)
				currentBuf.WriteString("\n\n")
			}
		case "bulleted_list_item":
			if b.BulletedListItem != nil {
				text = extractText(b.BulletedListItem.RichText)
				currentBuf.WriteString("- ")
				currentBuf.WriteString(text)
				currentBuf.WriteString("\n")
			}
		}
		_ = text

		// H1/H2: flush 旧段落，开启新段落
		if isSectionBoundary {
			flush()
			currentTitle = strings.TrimSpace(text)
			if currentTitle == "" {
				currentTitle = "Untitled"
			}
		}

		// 递归拉取子块
		if b.HasChildren {
			childItems, err := s.fetchNotionBlocksRecursive(ctx, client, apiKey, b.ID, depth+1, maxDepth)
			if err != nil {
				return nil, err
			}
			for _, ci := range childItems {
				currentBuf.WriteString(ci.Content)
				currentBuf.WriteString("\n")
			}
		}
	}
	flush()
	return items, nil
}

// splitMarkdownToItems 按二级标题切分 Markdown 文档
//
// 切分规则：
//   - 遇到 "## 标题" 时开启新段
//   - 单段超过 2000 字符时按 \n\n 软切
//   - 没有 "## 标题" 的整篇作为单一段
func splitMarkdownToItems(markdown, sourceID, source string) []BatchImportItem {
	lines := strings.Split(markdown, "\n")
	var items []BatchImportItem
	var currentTitle = "Main"
	var currentBuf strings.Builder

	flush := func() {
		body := strings.TrimSpace(currentBuf.String())
		if body == "" {
			return
		}
		// 软切：超过 2000 字符按段落切
		const maxLen = 2000
		if len([]rune(body)) > maxLen {
			chunks := softSplitParagraphs(body, maxLen)
			for i, c := range chunks {
				items = append(items, BatchImportItem{
					Title:   fmt.Sprintf("%s (part %d)", currentTitle, i+1),
					Content: c,
					Source:  fmt.Sprintf("%s:%s", source, sourceID),
				})
			}
		} else {
			items = append(items, BatchImportItem{
				Title:   currentTitle,
				Content: body,
				Source:  fmt.Sprintf("%s:%s", source, sourceID),
			})
		}
		currentBuf.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			flush()
			currentTitle = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			continue
		}
		currentBuf.WriteString(line)
		currentBuf.WriteString("\n")
	}
	flush()
	return items
}

// softSplitParagraphs 按段落软切长文本
func softSplitParagraphs(body string, maxLen int) []string {
	paragraphs := strings.Split(body, "\n\n")
	var chunks []string
	var buf strings.Builder
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// 单段超过 maxLen 硬切
		if len([]rune(p)) > maxLen {
			if buf.Len() > 0 {
				chunks = append(chunks, strings.TrimSpace(buf.String()))
				buf.Reset()
			}
			runes := []rune(p)
			for i := 0; i < len(runes); i += maxLen {
				end := i + maxLen
				if end > len(runes) {
					end = len(runes)
				}
				chunks = append(chunks, string(runes[i:end]))
			}
			continue
		}
		if buf.Len()+len(p)+2 > maxLen {
			chunks = append(chunks, strings.TrimSpace(buf.String()))
			buf.Reset()
		}
		buf.WriteString(p)
		buf.WriteString("\n\n")
	}
	if buf.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(buf.String()))
	}
	return chunks
}

// ============================================================================
// 辅助函数
// ============================================================================

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "kbg_" + hex.EncodeToString(b), nil
}

func hashToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

// ParseJSON 公开版 parseJSON(供跨包测试使用)
func ParseJSON(data []byte) ([]BatchImportItem, error) {
	return parseJSON(data)
}

// GenerateToken 公开版 generateToken(供跨包测试使用)
func GenerateToken() (string, error) {
	return generateToken()
}

// HashToken 公开版 hashToken(供跨包测试使用)
func HashToken(plain string) string {
	return hashToken(plain)
}

func tokenHasScope(scopes string, target string) bool {
	var arr []string
	if err := json.Unmarshal([]byte(scopes), &arr); err != nil {
		return false
	}
	for _, s := range arr {
		if s == target || s == "*" {
			return true
		}
	}
	return false
}

// TokenHasScope 公开版 tokenHasScope(供跨包测试使用)
func TokenHasScope(scopes string, target string) bool {
	return tokenHasScope(scopes, target)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// BoolToInt 公开版 boolToInt(供跨包测试使用)
func BoolToInt(b bool) int {
	return boolToInt(b)
}

// 避免 unused import 警告
var _ = http.StatusOK
