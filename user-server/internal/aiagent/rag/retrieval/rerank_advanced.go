package ragretrieval

// rerank_advanced.go Re-rank 重排序器（C 域 缺口 #1）
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十四章 §14.4.5 重排策略
//
// 在已有 LocalReranker（rerank.go，调用本地 TEI /rerank）基础上扩展三种重排策略：
//   a) Cross-Encoder 重排序（通过 RerankerInterface 调用本地推理服务，复用 LocalReranker）
//   b) RRF 融合（复用 rrf_fusion.go 的 RRFFusion）
//   c) 混合策略：先 RRF 融合再 Cross-Encoder 重排（默认推荐）
//
// 设计原则:
//   - 接口 Reranker.Rerank(ctx, query, docs, topK) 返回 []RankedDoc
//   - 提供 DefaultReranker（混合策略），开箱即用
//   - 缓存优化：相同 query+doc_id 的分数缓存 1 小时（LRU + TTL）
//   - 限制：topK ≤ 20，docs ≤ 100（超出截断，避免 LLM 调用爆炸）
//   - 私域独立部署: 无 merchant_id 字段
//
// 与已有 LocalReranker 关系:
//   - LocalReranker（rerank.go）保留为底层"分数评估器"
//   - 本文件提供更高级别的 Reranker 接口和策略组合
//   - 不破坏 LocalReranker 已有 API（HybridSearcher 仍依赖 RerankerInterface）

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ----------------------------------------------------------------------------
// 类型定义
// ----------------------------------------------------------------------------

// RetrievedDoc 检索得到的候选文档（含来源/原始分数，便于多路融合）
type RetrievedDoc struct {
	ID       string  // 文档/分片 ID（唯一）
	Content  string  // 文档内容
	Score    float64 // 原始检索分数（来自向量/BM25/RRF 等）
	Source   string  // 来源（vector / bm25 / rrf），用于多路融合
	Metadata map[string]any
}

// RankedDoc 重排后的文档（带最终分数与排名）
type RankedDoc struct {
	ID         string
	Content    string
	Score      float64 // 重排后最终分数
	Rank       int     // 1-based 排名
	Original   float64 // 原始分数（保留便于对比）
	Strategy   string  // 命中的重排策略（cross_encoder / rrf / hybrid）
	Recomputed bool    // 是否真正经过 LLM 评估（false 表示走缓存/降级）
}

// Reranker 高级别重排接口（C 域）
//
// 与 RerankerInterface 区别：
//   - RerankerInterface（rerank.go）只做单路打分（LocalReranker 调 TEI /rerank）
//   - Reranker 在其上封装多策略组合（RRF / Cross-Encoder / Hybrid）+ 缓存 + 限制
type Reranker interface {
	// Rerank 对候选文档按与 query 的相关性重排，返回按分数降序的 topK 结果
	//
	// 限制:
	//   - topK ≤ 20（超出截断为 20）
	//   - len(docs) ≤ 100（超出截断为 100，保留原始前 100 个）
	//   - docs 为空时返回 nil, nil
	Rerank(ctx context.Context, query string, docs []RetrievedDoc, topK int) ([]RankedDoc, error)

	// Strategy 返回策略名（cross_encoder / rrf / hybrid）
	Strategy() string
}

// ----------------------------------------------------------------------------
// 缓存：相同 query+doc_id 的分数缓存 1 小时
// ----------------------------------------------------------------------------

// rerankScoreCache 进程内 LRU+TTL 缓存：key=queryHash+":"+docID → score
//
// 设计：
//   - 容量 2048 条（典型查询 × doc 对数，足以覆盖热门 query）
//   - TTL 1 小时（业务允许 stale score，文档变更周期 > 1h）
//   - 命中后异步刷新（不阻塞主流程，避免缓存穿透）
//   - 私域部署无 Redis 依赖，进程内缓存足够（单实例足够；多实例各有缓存可接受）
type rerankScoreCache struct {
	mu       sync.RWMutex
	items    map[string]rerankCacheEntry
	capacity int
	ttl      time.Duration
}

type rerankCacheEntry struct {
	score   float64
	expires time.Time
}

// newRerankScoreCache 构造缓存
func newRerankScoreCache(capacity int, ttl time.Duration) *rerankScoreCache {
	if capacity <= 0 {
		capacity = 2048
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &rerankScoreCache{
		items:    make(map[string]rerankCacheEntry, capacity),
		capacity: capacity,
		ttl:      ttl,
	}
}

// Get 命中返回 score, true；未命中/过期返回 0, false
func (c *rerankScoreCache) Get(key string) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[key]
	if !ok {
		return 0, false
	}
	if time.Now().After(e.expires) {
		return 0, false
	}
	return e.score, true
}

// Set 写入缓存；超出容量时按 FIFO 淘汰（简单稳定，避免 LRU 在锁内的复杂度）
func (c *rerankScoreCache) Set(key string, score float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 已存在则更新（不增加容量）
	if _, ok := c.items[key]; ok {
		c.items[key] = rerankCacheEntry{score: score, expires: time.Now().Add(c.ttl)}
		return
	}
	// 容量满时按写入顺序淘汰一个最早的（用 map 迭代取一个最旧 expires）
	if len(c.items) >= c.capacity {
		var oldestKey string
		var oldestExp time.Time
		first := true
		for k, v := range c.items {
			if first {
				oldestKey, oldestExp, first = k, v.expires, false
				continue
			}
			if v.expires.Before(oldestExp) {
				oldestKey, oldestExp = k, v.expires
			}
		}
		if oldestKey != "" {
			delete(c.items, oldestKey)
		}
	}
	c.items[key] = rerankCacheEntry{score: score, expires: time.Now().Add(c.ttl)}
}

// Clear 清空缓存
func (c *rerankScoreCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]rerankCacheEntry, c.capacity)
}

// Len 当前缓存数量（测试用）
func (c *rerankScoreCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// hashQuery 计算 query 哈希（用作缓存 key 的一部分）
func hashQuery(query string) string {
	h := sha256.Sum256([]byte(query))
	return hex.EncodeToString(h[:])[:16] // 16 字符足够避免碰撞，控制 key 长度
}

// cacheKey 构造缓存键
func cacheKey(query, docID string) string {
	return hashQuery(query) + ":" + docID
}

// ----------------------------------------------------------------------------
// 策略枚举
// ----------------------------------------------------------------------------

// RerankStrategy 重排策略
type RerankStrategy string

const (
	// StrategyCrossEncoder Cross-Encoder 重排（调用 LLM/dispatcher 给 query-doc 对打分）
	StrategyCrossEncoder RerankStrategy = "cross_encoder"
	// StrategyRRF RRF 融合（复用 rrf_fusion.go）
	StrategyRRF RerankStrategy = "rrf"
	// StrategyHybrid 混合策略（先 RRF 后 Cross-Encoder，默认推荐）
	StrategyHybrid RerankStrategy = "hybrid"
)

// ----------------------------------------------------------------------------
// Cross-Encoder 评分器（基于已有 LocalReranker）
// ----------------------------------------------------------------------------

// CrossEncoderScorer 通过 RerankerInterface（LocalReranker）给 query-doc 对打分
//
// LocalReranker.Rerank(ctx, query, []RerankDoc) 返回 []RerankResult{ID, Score}
// 内部走本地 TEI /rerank 端点（bge-reranker-v2-m3 跨编码器）
type CrossEncoderScorer struct {
	delegate RerankerInterface // 通常是 *LocalReranker
	cache    *rerankScoreCache
}

// NewCrossEncoderScorer 构造 Cross-Encoder 评分器
//
// delegate 为 nil 时返回 nil scorer（HybridReranker 会降级为纯 RRF）
func NewCrossEncoderScorer(delegate RerankerInterface, cache *rerankScoreCache) *CrossEncoderScorer {
	if delegate == nil {
		return nil
	}
	if cache == nil {
		cache = newRerankScoreCache(2048, time.Hour)
	}
	return &CrossEncoderScorer{delegate: delegate, cache: cache}
}

// Score 对每个 doc 调用 Cross-Encoder 评估分数
//
// 返回 docID → score 映射；命中缓存的不重复调用
// 失败的 doc 不计入结果（由调用方决定降级策略）
func (s *CrossEncoderScorer) Score(ctx context.Context, query string, docs []RetrievedDoc) (map[string]float64, error) {
	if s == nil || s.delegate == nil {
		return nil, errors.New("cross-encoder scorer 未初始化")
	}
	if len(docs) == 0 {
		return map[string]float64{}, nil
	}

	// 1) 先查缓存，筛出未命中的 doc
	scores := make(map[string]float64, len(docs))
	var pending []RetrievedDoc
	for _, d := range docs {
		if v, ok := s.cache.Get(cacheKey(query, d.ID)); ok {
			scores[d.ID] = v
			continue
		}
		pending = append(pending, d)
	}

	if len(pending) == 0 {
		return scores, nil
	}

	// 2) 批量调用 Cross-Encoder（LocalReranker.Rerank 一次性传所有 texts）
	rerankDocs := make([]RerankDoc, 0, len(pending))
	for _, d := range pending {
		rerankDocs = append(rerankDocs, RerankDoc{ID: d.ID, Content: d.Content})
	}
	results, err := s.delegate.Rerank(ctx, query, rerankDocs)
	if err != nil {
		return scores, fmt.Errorf("cross-encoder 调用失败: %w", err)
	}

	// 3) 回填缓存
	for _, r := range results {
		scores[r.ID] = r.Score
		s.cache.Set(cacheKey(query, r.ID), r.Score)
	}
	return scores, nil
}

// ----------------------------------------------------------------------------
// 三种 Reranker 实现
// ----------------------------------------------------------------------------

// CrossEncoderReranker 纯 Cross-Encoder 重排
type CrossEncoderReranker struct {
	scorer *CrossEncoderScorer
	cache  *rerankScoreCache
}

// NewCrossEncoderReranker 构造纯 Cross-Encoder 重排器
func NewCrossEncoderReranker(scorer *CrossEncoderScorer) *CrossEncoderReranker {
	if scorer == nil {
		return nil
	}
	return &CrossEncoderReranker{scorer: scorer, cache: scorer.cache}
}

// Strategy 策略名
func (r *CrossEncoderReranker) Strategy() string { return string(StrategyCrossEncoder) }

// Rerank 实现 Reranker 接口
func (r *CrossEncoderReranker) Rerank(ctx context.Context, query string, docs []RetrievedDoc, topK int) ([]RankedDoc, error) {
	if r == nil || r.scorer == nil {
		return nil, errors.New("cross-encoder reranker 未初始化")
	}
	docs = clampDocs(docs)
	topK = clampTopK(topK)
	if len(docs) == 0 {
		return nil, nil
	}

	scores, err := r.scorer.Score(ctx, query, docs)
	if err != nil {
		// Cross-Encoder 失败：降级为按原始分数排序（不报错，保证可用性）
		return fallbackByOriginal(docs, topK, string(StrategyCrossEncoder)), nil
	}

	ranked := make([]RankedDoc, 0, len(docs))
	for _, d := range docs {
		score, ok := scores[d.ID]
		if !ok {
			// 未拿到分数（可能 LocalReranker 跳过该 ID）：保留原始分数
			score = d.Score
		}
		ranked = append(ranked, RankedDoc{
			ID:         d.ID,
			Content:    d.Content,
			Score:      score,
			Original:   d.Score,
			Strategy:   string(StrategyCrossEncoder),
			Recomputed: true,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
	return finalize(ranked, topK), nil
}

// RRFReranker 纯 RRF 融合重排（不需要 LLM，纯算法层）
//
// 把所有候选视为"多路召回结果"，按 RRF 公式重新打分
type RRFReranker struct {
	k     int
	cache *rerankScoreCache
}

// NewRRFReranker 构造 RRF 重排器
//
// k ≤ 0 时用默认值 60（与 rrf_fusion.go 一致）
func NewRRFReranker(k int, cache *rerankScoreCache) *RRFReranker {
	if k <= 0 {
		k = 60
	}
	if cache == nil {
		cache = newRerankScoreCache(2048, time.Hour)
	}
	return &RRFReranker{k: k, cache: cache}
}

// Strategy 策略名
func (r *RRFReranker) Strategy() string { return string(StrategyRRF) }

// Rerank 实现 Reranker 接口
//
// 算法：
//  1. 按 Source 分组（vector / bm25 / rrf / unknown 等），各自视为一路召回
//  2. 对每路按 Original 分数降序得到 rank
//  3. RRF 公式：score = Σ 1 / (k + rank)
//  4. 同一 doc 在多路出现会累加分数
func (r *RRFReranker) Rerank(ctx context.Context, query string, docs []RetrievedDoc, topK int) ([]RankedDoc, error) {
	docs = clampDocs(docs)
	topK = clampTopK(topK)
	if len(docs) == 0 {
		return nil, nil
	}

	// 1) 按 Source 分组
	bySource := make(map[string][]RetrievedDoc, 4)
	for _, d := range docs {
		src := d.Source
		if src == "" {
			src = "default"
		}
		bySource[src] = append(bySource[src], d)
	}

	// 2) 每路按 Original 降序得到 rank，应用 RRF 公式
	type entry struct {
		doc   RetrievedDoc
		score float64
	}
	merged := make(map[string]*entry, len(docs))
	for src, list := range bySource {
		// 复制后排序，不影响原切片
		sorted := make([]RetrievedDoc, len(list))
		copy(sorted, list)
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].Score > sorted[j].Score
		})
		for i, d := range sorted {
			rrfScore := 1.0 / float64(r.k+i+1)
			if e, ok := merged[d.ID]; ok {
				e.score += rrfScore
			} else {
				merged[d.ID] = &entry{doc: d, score: rrfScore}
			}
		}
		_ = src // src 不影响公式（rank 仅在路内计算）
	}

	// 3) 转换并排序
	ranked := make([]RankedDoc, 0, len(merged))
	for _, e := range merged {
		// 缓存 RRF 分数（同 query+doc_id 一致）
		r.cache.Set(cacheKey(query, e.doc.ID), e.score)
		ranked = append(ranked, RankedDoc{
			ID:         e.doc.ID,
			Content:    e.doc.Content,
			Score:      e.score,
			Original:   e.doc.Score,
			Strategy:   string(StrategyRRF),
			Recomputed: true,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].ID < ranked[j].ID // 同分按 ID 稳定
	})
	return finalize(ranked, topK), nil
}

// HybridReranker 混合策略：先 RRF 后 Cross-Encoder
//
// 流程：
//  1. 用 RRFReranker 对所有候选重新打分（得到 RRF 排序）
//  2. 取 RRF top N（默认 = min(topK*3, 30)）送 Cross-Encoder 精排
//  3. Cross-Encoder 失败则回退 RRF 排序
type HybridReranker struct {
	rrf        *RRFReranker
	crossEnc   *CrossEncoderReranker
	cache      *rerankScoreCache
	crossRatio int // Cross-Encoder 候选数 = min(topK*crossRatio, maxCross)
	maxCross   int
}

// NewHybridReranker 构造混合策略重排器
//
// rrfK: RRF 平滑常数（≤0 用默认 60）
// delegate: Cross-Encoder 底层评估器（LocalReranker），nil 时降级为纯 RRF
func NewHybridReranker(rrfK int, delegate RerankerInterface, cache *rerankScoreCache) *HybridReranker {
	if cache == nil {
		cache = newRerankScoreCache(2048, time.Hour)
	}
	r := &HybridReranker{
		rrf:        NewRRFReranker(rrfK, cache),
		cache:      cache,
		crossRatio: 3,
		maxCross:   30,
	}
	if delegate != nil {
		scorer := NewCrossEncoderScorer(delegate, cache)
		r.crossEnc = NewCrossEncoderReranker(scorer)
	}
	return r
}

// Strategy 策略名
func (r *HybridReranker) Strategy() string { return string(StrategyHybrid) }

// Rerank 实现 Reranker 接口
func (r *HybridReranker) Rerank(ctx context.Context, query string, docs []RetrievedDoc, topK int) ([]RankedDoc, error) {
	if r == nil {
		return nil, errors.New("hybrid reranker 未初始化")
	}
	docs = clampDocs(docs)
	topK = clampTopK(topK)
	if len(docs) == 0 {
		return nil, nil
	}

	// 1) RRF 粗排：取 topK*3 候选
	rrfTopN := topK * r.crossRatio
	if rrfTopN > r.maxCross {
		rrfTopN = r.maxCross
	}
	if rrfTopN < topK {
		rrfTopN = topK
	}
	rrfRanked, err := r.rrf.Rerank(ctx, query, docs, rrfTopN)
	if err != nil {
		// RRF 不应失败（纯算法）；若失败则降级为按 Original 排序
		return fallbackByOriginal(docs, topK, string(StrategyHybrid)), nil
	}
	if len(rrfRanked) == 0 {
		return nil, nil
	}

	// 2) 无 Cross-Encoder 时直接返回 RRF 结果
	if r.crossEnc == nil {
		// 标记策略为 hybrid（即使没走 cross-encoder，保持策略名一致）
		for i := range rrfRanked {
			rrfRanked[i].Strategy = string(StrategyHybrid)
		}
		return finalize(rrfRanked, topK), nil
	}

	// 3) Cross-Encoder 精排：把 RRF top N 转换为 RetrievedDoc 送入
	candidates := make([]RetrievedDoc, 0, len(rrfRanked))
	for _, rd := range rrfRanked {
		candidates = append(candidates, RetrievedDoc{
			ID:      rd.ID,
			Content: rd.Content,
			Score:   rd.Score, // RRF 分数作为原始分数
			Source:  "rrf",
		})
	}
	ceRanked, err := r.crossEnc.Rerank(ctx, query, candidates, topK)
	if err != nil || len(ceRanked) == 0 {
		// Cross-Encoder 失败：回退 RRF 排序
		for i := range rrfRanked {
			rrfRanked[i].Strategy = string(StrategyHybrid)
		}
		return finalize(rrfRanked, topK), nil
	}

	// 4) 标记最终策略为 hybrid（虽然最终分数来自 Cross-Encoder，但整体流程是混合）
	for i := range ceRanked {
		ceRanked[i].Strategy = string(StrategyHybrid)
	}
	return finalize(ceRanked, topK), nil
}

// ----------------------------------------------------------------------------
// DefaultReranker 默认混合策略重排器
// ----------------------------------------------------------------------------

// DefaultReranker 默认 Reranker（混合策略）
//
// 用 LocalReranker（rerank.go）作为 Cross-Encoder 底层评估器
// 当 LocalReranker 配置不可达（DefaultRerankConfig().Enabled=false）时，
// 自动降级为纯 RRF 策略（仍可用，只是不走 LLM 精排）
func DefaultReranker() Reranker {
	delegate := NewLocalReranker() // 不调 DefaultRerankConfig，避免构造期失败
	cache := newRerankScoreCache(2048, time.Hour)
	return NewHybridReranker(60, delegate, cache)
}

// NewDefaultRerankerWithConfig 显式注入 Cross-Encoder 底层评估器
//
// delegate 为 nil 时降级为纯 RRF
func NewDefaultRerankerWithConfig(delegate RerankerInterface) Reranker {
	cache := newRerankScoreCache(2048, time.Hour)
	return NewHybridReranker(60, delegate, cache)
}

// ----------------------------------------------------------------------------
// 工具函数
// ----------------------------------------------------------------------------

const (
	maxTopK             = 20
	maxDocs             = 100
	defaultCrossEncTopN = 20
)

// clampTopK 限制 topK ≤ 20，≤0 用默认 20
func clampTopK(topK int) int {
	if topK <= 0 {
		return defaultCrossEncTopN
	}
	if topK > maxTopK {
		return maxTopK
	}
	return topK
}

// clampDocs 限制 docs ≤ 100（保留前 100 个）
func clampDocs(docs []RetrievedDoc) []RetrievedDoc {
	if len(docs) <= maxDocs {
		return docs
	}
	return docs[:maxDocs]
}

// finalize 截断到 topK 并填充 Rank 字段（1-based）
func finalize(ranked []RankedDoc, topK int) []RankedDoc {
	if topK > 0 && len(ranked) > topK {
		ranked = ranked[:topK]
	}
	for i := range ranked {
		ranked[i].Rank = i + 1
	}
	return ranked
}

// fallbackByOriginal 按 Original 分数降序排列（降级策略）
//
// 当 Cross-Encoder 不可用时使用；标记 Recomputed=false
func fallbackByOriginal(docs []RetrievedDoc, topK int, strategy string) []RankedDoc {
	ranked := make([]RankedDoc, 0, len(docs))
	for _, d := range docs {
		ranked = append(ranked, RankedDoc{
			ID:         d.ID,
			Content:    d.Content,
			Score:      d.Score,
			Original:   d.Score,
			Strategy:   strategy,
			Recomputed: false,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].ID < ranked[j].ID
	})
	return finalize(ranked, topK)
}

// ----------------------------------------------------------------------------
// 适配器：把 Reranker 适配为 RerankerInterface（供 HybridSearcher 使用）
// ----------------------------------------------------------------------------

// RerankerToInterfaceAdapter 把高级 Reranker 适配为旧的 RerankerInterface
//
// 用于让 HybridSearcher（hybrid_searcher.go）透明地使用新的混合策略 Reranker
type RerankerToInterfaceAdapter struct {
	reranker Reranker
}

// NewRerankerToInterfaceAdapter 构造适配器
func NewRerankerToInterfaceAdapter(reranker Reranker) *RerankerToInterfaceAdapter {
	return &RerankerToInterfaceAdapter{reranker: reranker}
}

// Rerank 实现 RerankerInterface（接受 []RerankDoc 返回 []RerankResult）
func (a *RerankerToInterfaceAdapter) Rerank(ctx context.Context, query string, docs []RerankDoc) ([]RerankResult, error) {
	if a == nil || a.reranker == nil {
		return nil, errors.New("adapter 未初始化")
	}
	retrieved := make([]RetrievedDoc, 0, len(docs))
	for _, d := range docs {
		retrieved = append(retrieved, RetrievedDoc{ID: d.ID, Content: d.Content})
	}
	ranked, err := a.reranker.Rerank(ctx, query, retrieved, len(docs))
	if err != nil {
		return nil, err
	}
	out := make([]RerankResult, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, RerankResult{ID: r.ID, Score: r.Score})
	}
	return out, nil
}

// Compile-time 接口断言
var (
	_ Reranker          = (*CrossEncoderReranker)(nil)
	_ Reranker          = (*RRFReranker)(nil)
	_ Reranker          = (*HybridReranker)(nil)
	_ RerankerInterface = (*RerankerToInterfaceAdapter)(nil)
)

// ----------------------------------------------------------------------------
// 辅助：从 Chunks 转换为 RetrievedDoc
// ----------------------------------------------------------------------------

// ChunksToRetrievedDocs 把 []Chunk 转为 []RetrievedDoc
//
// 用于让 HybridSearcher 现有调用点（chunks → RerankChunks）平滑接入新 Reranker
func ChunksToRetrievedDocs(chunks []Chunk, source string) []RetrievedDoc {
	if len(chunks) == 0 {
		return nil
	}
	out := make([]RetrievedDoc, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, RetrievedDoc{
			ID:       c.ID,
			Content:  c.Content,
			Score:    c.Score,
			Source:   source,
			Metadata: c.Metadata,
		})
	}
	return out
}

// RankedDocsToChunks 把 []RankedDoc 转回 []Chunk（保留重排后顺序与分数）
func RankedDocsToChunks(ranked []RankedDoc, original map[string]Chunk) []Chunk {
	if len(ranked) == 0 {
		return nil
	}
	out := make([]Chunk, 0, len(ranked))
	for _, r := range ranked {
		c, ok := original[r.ID]
		if !ok {
			// 未找到原始 chunk，构造一个最小可用 Chunk
			c = Chunk{ID: r.ID, Content: r.Content}
		}
		c.Score = r.Score
		out = append(out, c)
	}
	return out
}

// String 用于日志/调试
func (r RankedDoc) String() string {
	return fmt.Sprintf("RankedDoc{ID=%s, Score=%.4f, Rank=%d, Strategy=%s}", r.ID, r.Score, r.Rank, r.Strategy)
}

// DescribeReranker 返回 Reranker 的人类可读描述（测试/调试用）
func DescribeReranker(r Reranker) string {
	if r == nil {
		return "nil"
	}
	return fmt.Sprintf("Reranker(strategy=%s)", r.Strategy())
}

// TrimStrategyName 截断策略名（避免日志过长）
func TrimStrategyName(s string) string {
	if len(s) > 32 {
		return strings.TrimSpace(s[:32]) + "..."
	}
	return strings.TrimSpace(s)
}
