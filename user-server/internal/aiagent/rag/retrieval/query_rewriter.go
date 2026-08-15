package ragretrieval


import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/utils/logger"
)

// QueryRewriteStrategy 改写策略标签
type QueryRewriteStrategy string

const (
	StrategyNone       QueryRewriteStrategy = "none"            
	StrategyHyDE       QueryRewriteStrategy = "hyde"            
	StrategyMultiQuery QueryRewriteStrategy = "multiquery"      
	StrategyHyDEMulti  QueryRewriteStrategy = "hyde_multiquery" 
)

// RewrittenQuery 改写结果
type RewrittenQuery struct {
	Original     string               `json:"original"`      
	Rewritten    string               `json:"rewritten"`     
	MultiQueries []string             `json:"multi_queries"` 
	UsedStrategy QueryRewriteStrategy `json:"used_strategy"` 
	CacheHit     bool                 `json:"cache_hit"`     
}

// QueryRewriter 查询改写器
type QueryRewriter struct {
	hydeGen     *HyDEGenerator
	multiGen    *MultiQueryGenerator
	redisClient RedisClient
	db          *gorm.DB
	redisTTL    time.Duration 
}

// QueryRewriterConfig 查询改写配置
type QueryRewriterConfig struct {
	RedisTTL time.Duration
}

// DefaultQueryRewriterConfig 默认配置
func DefaultQueryRewriterConfig() *QueryRewriterConfig {
	return &QueryRewriterConfig{
		RedisTTL: 24 * time.Hour,
	}
}

// NewQueryRewriter 创建查询改写器
//
// db 可为 nil（仅用 Redis 缓存，跳过 DB 缓存层）
// redisClient 可为 nil（仅用 DB 缓存，跳过 Redis 缓存层）
// hydeGen / multiGen 可为 nil（自动视为禁用该策略）
func NewQueryRewriter(
	hydeGen *HyDEGenerator,
	multiGen *MultiQueryGenerator,
	redisClient RedisClient,
	db *gorm.DB,
	cfg *QueryRewriterConfig,
) *QueryRewriter {
	if cfg == nil {
		cfg = DefaultQueryRewriterConfig()
	}
	return &QueryRewriter{
		hydeGen:     hydeGen,
		multiGen:    multiGen,
		redisClient: redisClient,
		db:          db,
		redisTTL:    cfg.RedisTTL,
	}
}

// Rewrite 执行查询改写
//
// 流程:
//  1. 空 query 直接返回（UsedStrategy=none）
//  2. 计算 query_hash = SHA256(normalized_query)
//  3. 查 Redis 缓存（key: rag:rewrite:{query_hash}），命中直接返回
//  4. 未命中查 DB query_rewrite_cache 表
//  5. 仍未命中则并行调用 HyDE + Multi-Query 生成
//  6. 写入 Redis (TTL 24h) + DB（异步）
//
// 容错:
//   - Redis / DB 失败不阻断，直接走生成路径
//   - HyDE / Multi-Query 失败不阻断对方，独立决策
//   - 二者均失败时 UsedStrategy=none, Rewritten=原 query
func (q *QueryRewriter) Rewrite(ctx context.Context, query string) (*RewrittenQuery, error) {
	if query == "" {
		return &RewrittenQuery{
			Original:     query,
			Rewritten:    query,
			UsedStrategy: StrategyNone,
		}, nil
	}

	hash := sha256Hex(normalizeQuery(query))
	redisKey := fmt.Sprintf("rag:rewrite:%s", hash)

	if q.redisClient != nil {
		if cached, err := q.redisClient.Get(ctx, redisKey); err == nil && cached != "" {
			var rw RewrittenQuery
			if err := json.Unmarshal([]byte(cached), &rw); err == nil {
				rw.CacheHit = true
				rw.Original = query
				go q.updateCacheStats(context.Background(), hash)
				return &rw, nil
			}
		}
	}

	if q.db != nil {
		if dbCached, err := q.queryDB(ctx, hash); err == nil && dbCached != nil {
			if q.redisClient != nil {
				if data, err := json.Marshal(dbCached); err == nil {
					_ = q.redisClient.Set(ctx, redisKey, string(data), q.redisTTL)
				}
			}
			dbCached.CacheHit = true
			dbCached.Original = query
			return dbCached, nil
		}
	}

	// 3) 并行生成 HyDE + Multi-Query
	var (
		hydeDoc      string
		multiQueries []string
		hydeErr      error
		multiErr     error
		strategy     QueryRewriteStrategy = StrategyNone
	)
	var wg sync.WaitGroup
	if q.hydeGen != nil && q.hydeGen.IsEnabled() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hydeDoc, hydeErr = q.hydeGen.Generate(ctx, query)
		}()
	}
	if q.multiGen != nil && q.multiGen.IsEnabled() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			multiQueries, multiErr = q.multiGen.Generate(ctx, query)
		}()
	}
	wg.Wait()

	rw := &RewrittenQuery{Original: query}
	if hydeErr == nil && hydeDoc != "" {
		rw.Rewritten = hydeDoc
		strategy = StrategyHyDE
	} else {
		rw.Rewritten = query
	}
	if multiErr == nil && len(multiQueries) > 0 {
		rw.MultiQueries = multiQueries
		if strategy == StrategyHyDE {
			strategy = StrategyHyDEMulti
		} else {
			strategy = StrategyMultiQuery
		}
	}
	rw.UsedStrategy = strategy

	if q.redisClient != nil {
		if data, err := json.Marshal(rw); err == nil {
			_ = q.redisClient.Set(ctx, redisKey, string(data), q.redisTTL)
		}
	}
	if q.db != nil {
		go q.persistToDB(context.Background(), hash, query, rw)
	}

	return rw, nil
}

// queryDB 查 DB query_rewrite_cache 表
//
// SQL: SELECT hyde_doc, multi_queries, rewrite_type FROM query_rewrite_cache
//
//	WHERE query_hash = $1 AND expires_at > NOW()
//
// 表不存在时返回 (nil, nil)，不阻断主流程
func (q *QueryRewriter) queryDB(ctx context.Context, hash string) (*RewrittenQuery, error) {
	// 检查表是否存在（迁移未执行时跳过）
	var tableExists bool
	if err := q.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'query_rewrite_cache')`,
	).Scan(&tableExists).Error; err != nil || !tableExists {
		return nil, nil
	}

	var row struct {
		HyDEDoc      string `gorm:"column:hyde_doc"`
		MultiQueries string `gorm:"column:multi_queries"`
		RewriteType  string `gorm:"column:rewrite_type"`
	}
	err := q.db.WithContext(ctx).Raw(
		`SELECT hyde_doc, multi_queries, rewrite_type FROM query_rewrite_cache
		 WHERE query_hash = ? AND (expires_at IS NULL OR expires_at > NOW())`,
		hash,
	).Scan(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	if row.HyDEDoc == "" && row.MultiQueries == "" {
		return nil, nil
	}
	rw := &RewrittenQuery{
		Rewritten:    row.HyDEDoc,
		UsedStrategy: QueryRewriteStrategy(row.RewriteType),
	}
	if row.MultiQueries != "" {
		_ = json.Unmarshal([]byte(row.MultiQueries), &rw.MultiQueries)
	}
	if rw.Rewritten == "" {
		rw.Rewritten = row.HyDEDoc 
	}
	return rw, nil
}

// persistToDB 异步写入 DB query_rewrite_cache 表
//
// best-effort: 失败仅记录日志，不影响主流程
func (q *QueryRewriter) persistToDB(ctx context.Context, hash, query string, rw *RewrittenQuery) {
	if q.db == nil {
		return
	}
	// 检查表是否存在
	var tableExists bool
	if err := q.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'query_rewrite_cache')`,
	).Scan(&tableExists).Error; err != nil || !tableExists {
		return
	}
	multiJSON, _ := json.Marshal(rw.MultiQueries)
	err := q.db.WithContext(ctx).Exec(`
		INSERT INTO query_rewrite_cache (query_hash, original_query, hyde_doc, multi_queries, rewrite_model, rewrite_type, hit_count, last_used_at, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, NOW(), NOW() + INTERVAL '30 days', NOW(), NOW())
		ON CONFLICT (query_hash) DO UPDATE SET
			hyde_doc = EXCLUDED.hyde_doc,
			multi_queries = EXCLUDED.multi_queries,
			rewrite_type = EXCLUDED.rewrite_type,
			updated_at = NOW()
	`,
		hash, query, rw.Rewritten, string(multiJSON), "local", string(rw.UsedStrategy),
	).Error
	if err != nil {
		logger.Errorf("query_rewriter: persist rewrite cache failed, hash=%s: %v", hash, err)
	}
}

// updateCacheStats 异步更新 hit_count
func (q *QueryRewriter) updateCacheStats(ctx context.Context, hash string) {
	if q.db == nil {
		return
	}
	var tableExists bool
	if err := q.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'query_rewrite_cache')`,
	).Scan(&tableExists).Error; err != nil || !tableExists {
		return
	}
	_ = q.db.WithContext(ctx).Exec(
		`UPDATE query_rewrite_cache SET hit_count = hit_count + 1, last_used_at = NOW() WHERE query_hash = ?`,
		hash,
	).Error
}

