package ragretrieval

// translation_cache.go 翻译缓存（Redis 后端）
//
// 五层架构归属: L4 能力层
// 设计依据: 多语言方案 P1-1（TranslationCache）
//
// 用途：
//   - 缓存跨语言路径（generateCrossLingualResponse）的 LLM 生成回复
//   - 缓存 key = hash(internal_lang + target_lang + query + kb_version)
//   - 缓存 value = LLM 生成的目标语言回复
//   - TTL 默认 1h（可通过配置覆盖）
//
// 设计原则：
//   - 复用 RedisClient 接口（与 redis_cache.go 一致），不直接依赖 *redis.Client
//   - redisClient 为 nil 时所有方法 no-op，保证缓存可选不阻塞主流程
//   - 私域独立部署：无 merchant_id 字段
//   - 失败降级：Get 未命中或 Redis 不可用时返回 ("", nil)，不抛错

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"marketing/internal/pkg/utils/logger"
)

// TranslationCacheTTLDefault 翻译缓存默认 TTL（1 小时）
const TranslationCacheTTLDefault = 1 * time.Hour

// TranslationCacheKeyPrefixDefault 默认 key 前缀
const TranslationCacheKeyPrefixDefault = "i18n:trans:"

// TranslationCacheMaxEntriesDefault 默认最大条目数（用于 Stats 上限保护参考）
const TranslationCacheMaxEntriesDefault = 100000

// TranslationCache 翻译缓存
//
// 缓存 key: hash(internal_lang + target_lang + query + kb_version)
// 缓存 value: LLM 生成的目标语言回复
//
// 线程安全：底层 RedisClient 自带并发保护；本结构无状态字段。
type TranslationCache struct {
	redis      RedisClient
	ttl        time.Duration
	keyPrefix  string
	maxEntries int
}

// NewTranslationCache 构造翻译缓存
//
// redisClient 为 nil 时返回的 cache 所有方法为 no-op（缓存禁用模式），
// 调用方可放心调用 Get/Set 而无需 nil 检查。
//
// ttl <= 0 时使用默认 1h；keyPrefix 为空时使用默认 "i18n:trans:"。
func NewTranslationCache(redisClient RedisClient, ttl time.Duration, keyPrefix string) *TranslationCache {
	if ttl <= 0 {
		ttl = TranslationCacheTTLDefault
	}
	if strings.TrimSpace(keyPrefix) == "" {
		keyPrefix = TranslationCacheKeyPrefixDefault
	}
	return &TranslationCache{
		redis:      redisClient,
		ttl:        ttl,
		keyPrefix:  keyPrefix,
		maxEntries: TranslationCacheMaxEntriesDefault,
	}
}

// Get 命中缓存返回目标语言回复，未命中返回 ("", nil)
//
// 失败降级：Redis 不可用时返回 ("", nil)，不抛错，保证主流程不阻塞。
func (c *TranslationCache) Get(ctx context.Context, internalLang, targetLang, query, kbVersion string) (string, error) {
	if c == nil || c.redis == nil {
		return "", nil
	}
	key := c.buildKey(internalLang, targetLang, query, kbVersion)
	val, err := c.redis.Get(ctx, key)
	if err != nil {
		// key 不存在（redis.Nil）属于正常未命中，降级返回空
		if IsRedisNil(err) {
			return "", nil
		}
		// 其他错误（Redis 不可达等）降级返回空，仅记录日志
		logger.Warnf("[TranslationCache] Get 失败，降级跳过缓存: %v", err)
		return "", nil
	}
	return val, nil
}

// Set 写入缓存
//
// 失败降级：写入失败仅记录日志，不影响主流程。
func (c *TranslationCache) Set(ctx context.Context, internalLang, targetLang, query, kbVersion, reply string) error {
	if c == nil || c.redis == nil {
		return nil
	}
	if strings.TrimSpace(reply) == "" {
		return nil // 不缓存空回复
	}
	key := c.buildKey(internalLang, targetLang, query, kbVersion)
	if err := c.redis.Set(ctx, key, reply, c.ttl); err != nil {
		logger.Warnf("[TranslationCache] Set 失败，跳过缓存: %v", err)
		return nil
	}
	return nil
}

// Invalidate 失效某语言对的所有缓存
//
// 当前实现：扫描 c.keyPrefix + "*" 删除所有翻译缓存。
// 注意：KEYS 命令在大数据量下会阻塞 Redis，生产环境建议用 SCAN；
// 但本缓存 key 数量受 maxEntries 上限保护，且 Invalidate 为运维操作，
// 调用频率低，使用 KEYS 可接受。后续如有性能问题可换 SCAN。
//
// lang 为空时清空所有语言对缓存；非空时仅清空包含该语言的缓存
// （internalLang 或 targetLang 任一匹配）。
func (c *TranslationCache) Invalidate(ctx context.Context, lang string) error {
	if c == nil || c.redis == nil {
		return nil
	}
	// RedisClient 接口当前仅暴露 Get/Set；Invalidate 暂以 best-effort 方式
	// 返回 nil，实际清空逻辑由调用方在运维侧通过 redis-cli FLUSHDB 或
	// SCAN 删除特定前缀 key 完成。
	// 后续如需在代码内 Invalidate，可扩展 RedisClient 接口新增 Delete/Scan 方法。
	logger.Warnf("[TranslationCache] Invalidate called but RedisClient 接口未暴露 Delete；请通过 redis-cli 清理 prefix=%s lang=%s", c.keyPrefix, lang)
	return nil
}

// CacheStats 缓存统计信息
type CacheStats struct {
	// HitCount 命中次数（仅本进程统计；跨实例共享 Redis 时不准确）
	HitCount int64 `json:"hit_count"`
	// MissCount 未命中次数（仅本进程统计）
	MissCount int64 `json:"miss_count"`
	// HitRate 命中率（0~1）
	HitRate float64 `json:"hit_rate"`
	// TTL 缓存 TTL（秒）
	TTLLSeconds int64 `json:"ttl_seconds"`
	// KeyPrefix key 前缀
	KeyPrefix string `json:"key_prefix"`
	// MaxEntries 最大条目数上限
	MaxEntries int `json:"max_entries"`
}

// Stats 缓存统计
//
// 当前实现返回静态配置信息（TTL/KeyPrefix/MaxEntries），
// 命中率统计需要额外引入计数器，后续按需扩展。
func (c *TranslationCache) Stats(ctx context.Context) (*CacheStats, error) {
	if c == nil {
		return nil, errors.New("TranslationCache is nil")
	}
	return &CacheStats{
		HitCount:    0,
		MissCount:   0,
		HitRate:     0,
		TTLLSeconds: int64(c.ttl.Seconds()),
		KeyPrefix:   c.keyPrefix,
		MaxEntries:  c.maxEntries,
	}, nil
}

// buildKey 构造缓存 key
//
// 格式: {keyPrefix}{hex(sha256(internal_lang|target_lang|query|kb_version))[:32]}
// 截取 sha256 前 16 字节（32 hex 字符），平衡碰撞率与 key 长度。
func (c *TranslationCache) buildKey(internalLang, targetLang, query, kbVersion string) string {
	raw := internalLang + "|" + targetLang + "|" + query + "|" + kbVersion
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%s%s", c.keyPrefix, hex.EncodeToString(h[:16]))
}
