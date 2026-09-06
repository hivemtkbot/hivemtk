package ragretrieval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

// RAGTier 检索层级
type RAGTier string

const (
	TierL1HotCache  RAGTier = "L1_hot_cache"
	TierL2WarmIndex RAGTier = "L2_warm_index"
	TierL3ColdIndex RAGTier = "L3_cold_index"
	TierL4Keyword   RAGTier = "L4_keyword"
)

// RAGThreeTierService 三级 RAG 检索服务
// RAG 三级架构贯通
// 调度策略：L1 → L2 → L3 → L4，按层级下沉，命中即返回
// 适用于客服/销售/营销场景的知识检索
type RAGThreeTierService struct {
	cache   *LRUCache
	storage StorageInterface
	vector  VectorizerInterface
	indexer IndexManagerInterface

	coldIndex IndexManagerInterface

	keyword KeywordSearcher

	mu      sync.RWMutex
	stats   TierStats
	enabled map[RAGTier]bool
}

// TierStats 各级命中统计
type TierStats struct {
	L1Hits int64 `json:"l1_hits"`
	L2Hits int64 `json:"l2_hits"`
	L3Hits int64 `json:"l3_hits"`
	L4Hits int64 `json:"l4_hits"`
	Misses int64 `json:"misses"`
	Total  int64 `json:"total"`
	AvgMs  int64 `json:"avg_ms"`
}

// TierSearchResult 三级检索的统一结果
type TierSearchResult struct {
	Query     string         `json:"query"`
	Answer    string         `json:"answer,omitempty"`
	Chunks    []Chunk        `json:"chunks"`
	Source    RAGTier        `json:"source"`
	Score     float64        `json:"score"`
	LatencyMs int64          `json:"latency_ms"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	FromCache bool           `json:"from_cache"`
}

// NewRAGThreeTierService 构造三级服务
func NewRAGThreeTierService(
	storage StorageInterface,
	vector VectorizerInterface,
	indexer IndexManagerInterface,
	coldIndex IndexManagerInterface,
	keyword KeywordSearcher,
	cacheSize int,
) *RAGThreeTierService {
	if cacheSize <= 0 {
		cacheSize = 1024
	}
	return &RAGThreeTierService{
		cache:     NewLRUCache(cacheSize, 30*time.Minute),
		storage:   storage,
		vector:    vector,
		indexer:   indexer,
		coldIndex: coldIndex,
		keyword:   keyword,
		enabled: map[RAGTier]bool{
			TierL1HotCache:  true,
			TierL2WarmIndex: indexer != nil,
			TierL3ColdIndex: coldIndex != nil,
			TierL4Keyword:   keyword != nil,
		},
	}
}

// Search 三级检索入口
func (s *RAGThreeTierService) Search(ctx context.Context, kbID, query string, topK int) (*TierSearchResult, error) {
	if s == nil {
		return nil, errors.New("nil service")
	}
	if query == "" {
		return nil, errors.New("empty query")
	}
	if topK <= 0 {
		topK = 5
	}
	start := time.Now()
	s.mu.Lock()
	s.stats.Total++
	s.mu.Unlock()

	s.mu.RLock()
	l1Enabled := s.enabled[TierL1HotCache]
	l2Enabled := s.enabled[TierL2WarmIndex]
	l3Enabled := s.enabled[TierL3ColdIndex]
	l4Enabled := s.enabled[TierL4Keyword]
	s.mu.RUnlock()

	if l1Enabled {
		key := s.cacheKey(kbID, query, topK)
		if v, ok := s.cache.Get(key); ok {
			res, ok := v.(*TierSearchResult)
			if !ok {
				goto l2_fallback
			}
			res.FromCache = true
			res.LatencyMs = time.Since(start).Milliseconds()
			s.mu.Lock()
			s.stats.L1Hits++
			s.updateAvg(res.LatencyMs)
			s.mu.Unlock()
			return res, nil
		}
	}

l2_fallback:
	if l2Enabled {
		chunks, score, err := s.searchTier(ctx, s.indexer, kbID, query, topK)
		if err == nil && len(chunks) > 0 {
			res := s.makeResult(query, chunks, TierL2WarmIndex, score, start)
			s.cachePut(s.cacheKey(kbID, query, topK), res, l1Enabled)
			s.mu.Lock()
			s.stats.L2Hits++
			s.updateAvg(res.LatencyMs)
			s.mu.Unlock()
			return res, nil
		}
		if err != nil {
			logger.Errorf("[RAG-3T] L2 检索失败: %v", err)
		}
	}

	if l3Enabled {
		chunks, score, err := s.searchTier(ctx, s.coldIndex, kbID, query, topK)
		if err == nil && len(chunks) > 0 {
			res := s.makeResult(query, chunks, TierL3ColdIndex, score, start)
			s.cachePut(s.cacheKey(kbID, query, topK), res, l1Enabled)
			s.mu.Lock()
			s.stats.L3Hits++
			s.updateAvg(res.LatencyMs)
			s.mu.Unlock()
			return res, nil
		}
	}

	if l4Enabled {
		chunks, err := s.keyword.SearchKeyword(ctx, kbID, query, topK)
		if err == nil && len(chunks) > 0 {
			res := s.makeResult(query, chunks, TierL4Keyword, 0.5, start)
			s.mu.Lock()
			s.stats.L4Hits++
			s.updateAvg(res.LatencyMs)
			s.mu.Unlock()
			return res, nil
		}
	}

	s.mu.Lock()
	s.stats.Misses++
	s.mu.Unlock()
	return &TierSearchResult{
		Query:     query,
		Chunks:    []Chunk{},
		Source:    "",
		LatencyMs: time.Since(start).Milliseconds(),
		Metadata:  map[string]any{"reason": "no_hit"},
	}, nil
}

func (s *RAGThreeTierService) searchTier(ctx context.Context, mgr IndexManagerInterface, kbID, query string, topK int) ([]Chunk, float64, error) {
	if mgr == nil {
		return nil, 0, errors.New("nil indexer")
	}
	if s.vector == nil {
		return nil, 0, errors.New("nil vectorizer")
	}
	vec, err := s.vector.EmbedText(query)
	if err != nil {
		return nil, 0, err
	}
	chunks, err := mgr.SearchIndex(ctx, kbID, vec, topK)
	if err != nil {
		return nil, 0, err
	}
	if len(chunks) == 0 {
		return chunks, 0, nil
	}
	maxScore := chunks[0].Score
	for _, c := range chunks {
		if c.Score > maxScore {
			maxScore = c.Score
		}
	}
	return chunks, maxScore, nil
}

func (s *RAGThreeTierService) makeResult(query string, chunks []Chunk, src RAGTier, score float64, start time.Time) *TierSearchResult {
	return &TierSearchResult{
		Query:     query,
		Chunks:    chunks,
		Source:    src,
		Score:     score,
		LatencyMs: time.Since(start).Milliseconds(),
		Metadata:  map[string]any{"chunks": len(chunks)},
	}
}

func (s *RAGThreeTierService) cacheKey(kbID, query string, topK int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", kbID, strings.ToLower(strings.TrimSpace(query)), topK)))
	return hex.EncodeToString(h[:16])
}

func (s *RAGThreeTierService) cachePut(key string, val *TierSearchResult, l1Enabled bool) {
	if !l1Enabled {
		return
	}
	s.cache.Set(key, val, 30*time.Minute)
}

func (s *RAGThreeTierService) updateAvg(ms int64) {
	n := s.stats.Total
	if n == 0 {
		return
	}
	s.stats.AvgMs = (s.stats.AvgMs*(n-1) + ms) / n
}

// Stats 获取统计
func (s *RAGThreeTierService) Stats() TierStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// EnableTier 动态启用/禁用某层
func (s *RAGThreeTierService) EnableTier(tier RAGTier, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled[tier] = enabled
}

// IsEnabled 查询某层是否启用
func (s *RAGThreeTierService) IsEnabled(tier RAGTier) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled[tier]
}

// ClearCache 清空 L1
func (s *RAGThreeTierService) ClearCache() {
	s.cache.Clear()
}

// WarmupCache 预热缓存（批量 query -> 答案）
func (s *RAGThreeTierService) WarmupCache(ctx context.Context, kbID string, queries []string, topK int) (int, error) {
	count := 0
	for _, q := range queries {
		_, err := s.Search(ctx, kbID, q, topK)
		if err == nil {
			count++
		}
	}
	return count, nil
}

// MergeResults 多层级结果合并（用于同时检索 L2 + L3 提升召回率）
func (s *RAGThreeTierService) MergeResults(results ...[]Chunk) []Chunk {
	seen := make(map[string]bool)
	merged := make([]Chunk, 0, 32)
	for _, list := range results {
		for _, c := range list {
			if c.ID == "" || seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			merged = append(merged, c)
		}
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })
	return merged
}

// EncodeResult 序列化结果（用于跨服务传递）
func (s *RAGThreeTierService) EncodeResult(r *TierSearchResult) ([]byte, error) {
	return json.Marshal(r)
}

// DecodeResult 反序列化
func (s *RAGThreeTierService) DecodeResult(b []byte) (*TierSearchResult, error) {
	var r TierSearchResult
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
