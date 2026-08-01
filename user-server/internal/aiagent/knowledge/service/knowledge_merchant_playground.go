package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"marketing/internal/aiagent/knowledge/model"
)

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
		req.TopK = DefaultTopK
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
	productNumericID := req.ProductID
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

func (s *KnowledgeMerchantService) recordSearchLog(ctx context.Context, productID string, query string, topK int, threshold float64, count int, max, min, avg float64, latencyMs int64) error {
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
