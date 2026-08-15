package ragretrieval


import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/pkg/utils/logger"
)

// CachedEmbeddingClient Embedding 服务的 Redis 缓存装饰器
type CachedEmbeddingClient struct {
	inner        llm.EmbeddingServiceInterface 
	redis        RedisClient
	db           *gorm.DB
	ttlRedis     time.Duration 
	ttlDB        time.Duration 
	disableCache bool          
}

// CachedEmbeddingClientConfig 配置
type CachedEmbeddingClientConfig struct {
	TTLRedis     time.Duration
	TTLDB        time.Duration
	DisableCache bool
}

// DefaultCachedEmbeddingClientConfig 默认配置
func DefaultCachedEmbeddingClientConfig() *CachedEmbeddingClientConfig {
	return &CachedEmbeddingClientConfig{
		TTLRedis:     7 * 24 * time.Hour,
		TTLDB:        30 * 24 * time.Hour,
		DisableCache: false,
	}
}

// NewCachedEmbeddingClient 构造装饰器
//
// inner 必须非 nil（通常为 *llm.EmbeddingService）
// redis / db 可为 nil（禁用对应缓存层）
func NewCachedEmbeddingClient(
	inner llm.EmbeddingServiceInterface,
	redis RedisClient,
	db *gorm.DB,
	cfg *CachedEmbeddingClientConfig,
) *CachedEmbeddingClient {
	if cfg == nil {
		cfg = DefaultCachedEmbeddingClientConfig()
	}
	return &CachedEmbeddingClient{
		inner:        inner,
		redis:        redis,
		db:           db,
		ttlRedis:     cfg.TTLRedis,
		ttlDB:        cfg.TTLDB,
		disableCache: cfg.DisableCache,
	}
}

// Embed 批量 embedding（带缓存）
//
// 批量场景：每个 text 独立查缓存，未命中的批量调 TEI，再回填缓存
func (c *CachedEmbeddingClient) Embed(ctx context.Context, cfg *llm.EmbeddingConfig, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	if cfg == nil {
		cfg = c.inner.DefaultConfig()
	}
	if c.disableCache || (c.redis == nil && c.db == nil) {
		return c.inner.Embed(ctx, cfg, texts)
	}

	model := cfg.Model
	expectDim := cfg.Dimension
	if expectDim <= 0 {
		expectDim = 1024
	}
	results := make([][]float32, len(texts))
	missIndices := make([]int, 0, len(texts))
	missTexts := make([]string, 0, len(texts))

	for i, text := range texts {
		hit := false
		if c.redis != nil {
			key := c.cacheKey(model, text)
			if cached, err := c.redis.Get(ctx, key); err == nil && cached != "" {
				if vec, err := decodeVec(cached); err == nil && len(vec) == expectDim {
					results[i] = vec
					hit = true
				}
			}
		}
		if !hit {
			missIndices = append(missIndices, i)
			missTexts = append(missTexts, text)
		}
	}

	if len(missIndices) > 0 && c.db != nil {
		newMissIndices := make([]int, 0, len(missIndices))
		newMissTexts := make([]string, 0, len(missTexts))
		for i, idx := range missIndices {
			text := missTexts[i]
			vec, hit := c.queryDBCache(ctx, model, text, expectDim)
			if hit {
				results[idx] = vec
				if c.redis != nil {
					_ = c.redis.Set(ctx, c.cacheKey(model, text), encodeVec(vec), c.ttlRedis)
				}
			} else {
				newMissIndices = append(newMissIndices, idx)
				newMissTexts = append(newMissTexts, text)
			}
		}
		missIndices = newMissIndices
		missTexts = newMissTexts
	}

	if len(missIndices) > 0 {
		vectors, err := c.inner.Embed(ctx, cfg, missTexts)
		if err != nil {
			return nil, err
		}
		for i, idx := range missIndices {
			if i >= len(vectors) {
				break
			}
			results[idx] = vectors[i]
			go func(text string, vec []float32) {
				ctxBg := context.Background()
				if c.redis != nil {
					_ = c.redis.Set(ctxBg, c.cacheKey(model, text), encodeVec(vec), c.ttlRedis)
				}
				if c.db != nil {
					c.persistDBCache(ctxBg, model, text, vec)
				}
			}(missTexts[i], vectors[i])
		}
	}
	return results, nil
}

// EmbedOne 单条 embedding（带缓存）
func (c *CachedEmbeddingClient) EmbedOne(ctx context.Context, cfg *llm.EmbeddingConfig, text string) ([]float32, error) {
	vectors, err := c.Embed(ctx, cfg, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedding 返回空")
	}
	return vectors[0], nil
}

// DefaultConfig 透传给 inner
func (c *CachedEmbeddingClient) DefaultConfig() *llm.EmbeddingConfig {
	return c.inner.DefaultConfig()
}

// cacheKey 生成缓存 key
//
// 格式: rag:emb:{model}:{sha256(normalized_text)}
func (c *CachedEmbeddingClient) cacheKey(model, text string) string {
	return fmt.Sprintf("rag:emb:%s:%s", model, sha256Hex(normalizeQuery(text)))
}

// queryDBCache 查 DB embedding_cache 表
//
// 表不存在时返回 (nil, false)，不阻断主流程
func (c *CachedEmbeddingClient) queryDBCache(ctx context.Context, model, text string, expectDim int) ([]float32, bool) {
	var tableExists bool
	if err := c.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'embedding_cache')`,
	).Scan(&tableExists).Error; err != nil || !tableExists {
		return nil, false
	}
	hash := sha256Hex(normalizeQuery(text))
	var vecStr string
	err := c.db.WithContext(ctx).Raw(
		`SELECT embedding::text FROM embedding_cache WHERE text_hash = ? AND model = ? AND (expires_at IS NULL OR expires_at > NOW())`,
		hash, model,
	).Scan(&vecStr).Error
	if err != nil || vecStr == "" {
		return nil, false
	}
	vec, err := parsePGVector(vecStr)
	if err != nil || len(vec) != expectDim {
		return nil, false
	}
	go func() {
		_ = c.db.WithContext(context.Background()).Exec(
			`UPDATE embedding_cache SET hit_count = hit_count + 1, last_used_at = NOW() WHERE text_hash = ? AND model = ?`,
			hash, model,
		).Error
	}()
	return vec, true
}

// persistDBCache 写入 DB embedding_cache 表
//
// best-effort: 失败仅记录日志，不影响主流程
func (c *CachedEmbeddingClient) persistDBCache(ctx context.Context, model, text string, vec []float32) {
	var tableExists bool
	if err := c.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'embedding_cache')`,
	).Scan(&tableExists).Error; err != nil || !tableExists {
		return
	}
	hash := sha256Hex(normalizeQuery(text))
	vecLiteral := vecToPGString(vec)
	expiresAt := time.Now().Add(c.ttlDB)
	err := c.db.WithContext(ctx).Exec(`
		INSERT INTO embedding_cache (text_hash, text_content, model, dimension, embedding, hit_count, last_used_at, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?::vector, 0, NOW(), ?, NOW(), NOW())
		ON CONFLICT (text_hash, model) DO UPDATE SET
			embedding = EXCLUDED.embedding,
			dimension = EXCLUDED.dimension,
			last_used_at = NOW(),
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
	`,
		hash, text, model, len(vec), vecLiteral, expiresAt,
	).Error
	if err != nil {
		logger.Errorf("cached_embedding: persistDBCache failed, hash=%s: %v", hash, err)
	}
}

// Compile-time 接口断言
var _ llm.EmbeddingServiceInterface = (*CachedEmbeddingClient)(nil)

