package migrations

// rag_hybrid_migration.go RAG 混合检索 SQL 迁移 v2.7.0
//
// 五层架构归属: L5 数据层
// 设计依据: docs/核心链路优化.md 第十四章 §14.3 表结构设计
// 私域独立部署: 无 merchant_id 字段
//
// 本迁移补充 RAG 混合检索所需的数据库基础设施：
//  1. knowledge_chunks 表增强（tsvector 列 + 索引 + 触发器）
//  2. query_rewrite_cache 表（查询改写缓存，HyDE/Multi-Query 结果持久化）
//  3. embedding_cache 表（embedding 结果持久化缓存，避免重复 TEI 调用）
//  4. knowledge_search_logs 表增强（混合检索各阶段延迟/计数）
//  5. zhparser 扩展（中文 BM25 分词）
//
// 幂等性: 所有 DDL 使用 IF NOT EXISTS / IF NOT EXISTS，可重入
// 依赖: v2.6.0（KnowledgeVectorMigration，knowledge_chunks.embedding 列已存在）

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// RagHybridMigration RAG 混合检索迁移 v2.7.0
type RagHybridMigration struct {
	db *gorm.DB
}

// NewRagHybridMigration 创建迁移实例
func NewRagHybridMigration(db *gorm.DB) *RagHybridMigration {
	return &RagHybridMigration{db: db}
}

// Version 返回版本号
func (m *RagHybridMigration) Version() string { return "v2.7.0" }

// Name 返回迁移名称
func (m *RagHybridMigration) Name() string { return "RAG 混合检索（tsvector + 缓存表）" }

// Description 返回迁移描述
func (m *RagHybridMigration) Description() string {
	return "为 knowledge_chunks 增加 tsvector 列/触发器，新建 query_rewrite_cache / embedding_cache 表，增强 knowledge_search_logs 监控字段"
}

// Up 执行升级
func (m *RagHybridMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 0) 安装 pgcrypto 扩展（触发器中 digest() 函数依赖，必须先于 enhanceKnowledgeChunks 安装）
	if err := m.installPgcrypto(ctx); err != nil {
		logger.Infof("[RagHybridMigration] pgcrypto 扩展安装提示（content_hash 自动维护将不可用）: %v", err)
	}

	// 1) 安装 zhparser 扩展（中文分词）
	if err := m.installZhparser(ctx); err != nil {
		// zhparser 安装失败不阻断迁移（生产环境镜像已预装，开发环境可用 simple 配置兜底）
		logger.Infof("[RagHybridMigration] zhparser 扩展安装提示（可忽略，将用 simple 兜底）: %v", err)
	}

	// 2) 创建 zh_rag 文本搜索配置（基于 zhparser）
	if err := m.createZhRagConfig(ctx); err != nil {
		logger.Infof("[RagHybridMigration] zh_rag 配置创建提示: %v", err)
	}

	// 3) knowledge_chunks 表增强
	if err := m.enhanceKnowledgeChunks(ctx); err != nil {
		return fmt.Errorf("enhance knowledge_chunks 失败: %w", err)
	}

	// 4) query_rewrite_cache 表
	if err := m.createQueryRewriteCache(ctx); err != nil {
		return fmt.Errorf("create query_rewrite_cache 失败: %w", err)
	}

	// 5) embedding_cache 表
	if err := m.createEmbeddingCache(ctx); err != nil {
		return fmt.Errorf("create embedding_cache 失败: %w", err)
	}

	// 6) knowledge_search_logs 表增强
	if err := m.enhanceKnowledgeSearchLogs(ctx); err != nil {
		return fmt.Errorf("enhance knowledge_search_logs 失败: %w", err)
	}

	return nil
}

// installPgcrypto 安装 pgcrypto 扩展（content_hash 自动维护依赖 digest() 函数）
//
// 必须在 enhanceKnowledgeChunks 之前调用，否则触发器在 INSERT 时会因 digest() 不存在而失败
func (m *RagHybridMigration) installPgcrypto(ctx context.Context) error {
	var installed bool
	if err := m.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pgcrypto')`,
	).Scan(&installed).Error; err != nil {
		return err
	}
	if installed {
		return nil
	}
	return m.db.WithContext(ctx).Exec(`CREATE EXTENSION IF NOT EXISTS pgcrypto`).Error
}

// installZhparser 安装 zhparser 扩展
//
// 前提: postgres 镜像已安装 zhparser.so（Dockerfile 中预装）
// 失败时不阻断迁移（生产环境必须安装，开发环境可用 simple 配置兜底）
func (m *RagHybridMigration) installZhparser(ctx context.Context) error {
	// 检查是否已安装
	var installed bool
	if err := m.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'zhparser')`,
	).Scan(&installed).Error; err != nil {
		return err
	}
	if installed {
		return nil
	}
	return m.db.WithContext(ctx).Exec(`CREATE EXTENSION IF NOT EXISTS zhparser`).Error
}

// createZhRagConfig 创建 zh_rag 文本搜索配置
//
// 基于 zhparser 分词器，配置同义词等可在后续维护
func (m *RagHybridMigration) createZhRagConfig(ctx context.Context) error {
	// 检查 zhparser 是否已安装
	var installed bool
	if err := m.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'zhparser')`,
	).Scan(&installed).Error; err != nil || !installed {
		return nil // zhparser 未安装时跳过，BM25Retriever 会用 simple 兜底
	}

	// 创建配置（幂等：CREATE TEXT SEARCH CONFIGURATION 不支持 IF NOT EXISTS，用 DO 块）
	stmt := `
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_ts_config WHERE cfgname = 'zh_rag'
			) THEN
				CREATE TEXT SEARCH CONFIGURATION zh_rag (PARSER = zhparser);
				ALTER TEXT SEARCH CONFIGURATION zh_rag ADD MAPPING FOR n,v,a,i,e,l WITH simple;
			END IF;
		END $$;
	`
	return m.db.WithContext(ctx).Exec(stmt).Error
}

// enhanceKnowledgeChunks 增强 knowledge_chunks 表
//
// 新增列:
//   - content_tsv tsvector（content 列的 tsvector，BM25 召回兜底路径）
//   - contextual_context text（Anthropic Contextual Retrieval 生成的 50-100 token 上下文）
//   - contextual_tsv tsvector（contextual_context + content 的 tsvector，BM25 召回优先路径）
//   - content_hash varchar(64)（content SHA256，用于 chunk 去重）
//   - embed_status varchar(20)（向量状态：pending/indexed/failed）
//
// 新增索引:
//   - idx_knowledge_chunks_content_hash（content_hash）
//   - idx_knowledge_chunks_embedding_id（embedding_id）
//   - idx_knowledge_chunks_embed_status（embed_status）
//   - idx_knowledge_chunks_content_tsv（GIN tsvector 索引）
//   - idx_knowledge_chunks_contextual_tsv（GIN tsvector 索引）
//
// 新增触发器:
//   - knowledge_chunks_tsv_update（INSERT/UPDATE 时自动维护 content_tsv / contextual_tsv）
func (m *RagHybridMigration) enhanceKnowledgeChunks(ctx context.Context) error {
	// 新增列（幂等）
	stmts := []string{
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS content_tsv tsvector`,
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS contextual_context text`,
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS contextual_tsv tsvector`,
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS content_hash varchar(64)`,
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS embed_status varchar(20) DEFAULT 'pending'`,
	}
	for _, sql := range stmts {
		if err := m.db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("add column failed (%s): %w", sql, err)
		}
	}

	// 新增索引（幂等）
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_content_hash ON knowledge_chunks(content_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_embedding_id ON knowledge_chunks(embedding_id)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_embed_status ON knowledge_chunks(embed_status)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_content_tsv ON knowledge_chunks USING GIN (content_tsv)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_contextual_tsv ON knowledge_chunks USING GIN (contextual_tsv)`,
	}
	for _, sql := range indexes {
		if err := m.db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("create index failed (%s): %w", sql, err)
		}
	}

	// 触发器（幂等：DROP IF EXISTS + CREATE）
	// content_tsv: 优先用 zh_rag 配置（若存在），否则用 simple
	// contextual_tsv: 同上，但基于 contextual_context || ' ' || content
	triggerStmt := `
		DROP TRIGGER IF EXISTS knowledge_chunks_tsv_update ON knowledge_chunks;

		CREATE OR REPLACE FUNCTION knowledge_chunks_tsv_trigger() RETURNS trigger AS $$
		BEGIN
			-- 优先 zh_rag，不存在则 simple
			IF EXISTS (SELECT 1 FROM pg_ts_config WHERE cfgname = 'zh_rag') THEN
				NEW.content_tsv := to_tsvector('zh_rag', coalesce(NEW.content, ''));
				NEW.contextual_tsv := to_tsvector('zh_rag', coalesce(NEW.contextual_context, '') || ' ' || coalesce(NEW.content, ''));
			ELSE
				NEW.content_tsv := to_tsvector('simple', coalesce(NEW.content, ''));
				NEW.contextual_tsv := to_tsvector('simple', coalesce(NEW.contextual_context, '') || ' ' || coalesce(NEW.content, ''));
			END IF;
			-- 自动维护 content_hash（若未设置）
			IF NEW.content_hash IS NULL AND NEW.content IS NOT NULL THEN
				NEW.content_hash := encode(digest(coalesce(NEW.content, ''), 'sha256'), 'hex');
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		CREATE TRIGGER knowledge_chunks_tsv_update
			BEFORE INSERT OR UPDATE ON knowledge_chunks
			FOR EACH ROW EXECUTE FUNCTION knowledge_chunks_tsv_trigger();
	`
	if err := m.db.WithContext(ctx).Exec(triggerStmt).Error; err != nil {
		return fmt.Errorf("create trigger failed: %w", err)
	}

	// 一次性回填存量 chunk 的 tsvector（不阻塞迁移完成；大表可异步执行）
	backfill := `
		UPDATE knowledge_chunks
		SET content_tsv = CASE
			WHEN EXISTS (SELECT 1 FROM pg_ts_config WHERE cfgname = 'zh_rag')
			THEN to_tsvector('zh_rag', coalesce(content, ''))
			ELSE to_tsvector('simple', coalesce(content, ''))
		END,
		contextual_tsv = CASE
			WHEN EXISTS (SELECT 1 FROM pg_ts_config WHERE cfgname = 'zh_rag')
			THEN to_tsvector('zh_rag', coalesce(contextual_context, '') || ' ' || coalesce(content, ''))
			ELSE to_tsvector('simple', coalesce(contextual_context, '') || ' ' || coalesce(content, ''))
		END
		WHERE content_tsv IS NULL
	`
	_ = m.db.WithContext(ctx).Exec(backfill).Error // best-effort

	return nil
}

// createQueryRewriteCache 创建 query_rewrite_cache 表
//
// 用途: 持久化 HyDE 假设文档 + Multi-Query 变体，避免重复 LLM 调用
// TTL: 30 天（expires_at 字段控制）
func (m *RagHybridMigration) createQueryRewriteCache(ctx context.Context) error {
	stmt := `
		CREATE TABLE IF NOT EXISTS query_rewrite_cache (
			id              BIGSERIAL PRIMARY KEY,
			query_hash      VARCHAR(64)  NOT NULL UNIQUE,
			original_query  TEXT         NOT NULL,
			hyde_doc        TEXT,
			multi_queries   JSONB,
			rewrite_model   VARCHAR(100),
			rewrite_type    VARCHAR(50),
			hit_count       BIGINT       DEFAULT 0,
			last_used_at    TIMESTAMP,
			expires_at      TIMESTAMP,
			created_at      TIMESTAMP    DEFAULT NOW(),
			updated_at      TIMESTAMP    DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_query_rewrite_cache_expires ON query_rewrite_cache(expires_at);
		CREATE INDEX IF NOT EXISTS idx_query_rewrite_cache_last_used ON query_rewrite_cache(last_used_at);
	`
	return m.db.WithContext(ctx).Exec(stmt).Error
}

// createEmbeddingCache 创建 embedding_cache 表
//
// 用途: 持久化 query embedding 结果，避免重复 TEI 调用
// TTL: 30 天（expires_at 字段控制）
// 注意: embedding 列类型 vector(1024)（与 knowledge_chunks.embedding 一致）
func (m *RagHybridMigration) createEmbeddingCache(ctx context.Context) error {
	// 确认 pgvector 扩展可用
	var extCount int64
	if err := m.db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM pg_extension WHERE extname = 'vector'`,
	).Scan(&extCount).Error; err != nil {
		return fmt.Errorf("查询 pgvector 扩展失败: %w", err)
	}
	if extCount == 0 {
		if err := m.db.WithContext(ctx).Exec(`CREATE EXTENSION IF NOT EXISTS vector`).Error; err != nil {
			return fmt.Errorf("pgvector 扩展未安装且创建失败: %w", err)
		}
	}

	// pgcrypto 已在 Up() 开头通过 installPgcrypto 安装（content_hash 触发器依赖）
	// 此处不再重复安装

	stmt := `
		CREATE TABLE IF NOT EXISTS embedding_cache (
			id              BIGSERIAL PRIMARY KEY,
			text_hash       VARCHAR(64)  NOT NULL,
			text_content    TEXT         NOT NULL,
			model           VARCHAR(100) NOT NULL,
			dimension       INT          NOT NULL DEFAULT 1024,
			embedding       vector(1024) NOT NULL,
			hit_count       BIGINT       DEFAULT 0,
			last_used_at    TIMESTAMP,
			expires_at      TIMESTAMP,
			created_at      TIMESTAMP    DEFAULT NOW(),
			updated_at      TIMESTAMP    DEFAULT NOW(),
			CONSTRAINT uk_embedding_cache_hash_model UNIQUE (text_hash, model)
		);
		CREATE INDEX IF NOT EXISTS idx_embedding_cache_expires ON embedding_cache(expires_at);
		CREATE INDEX IF NOT EXISTS idx_embedding_cache_last_used ON embedding_cache(last_used_at);
	`
	return m.db.WithContext(ctx).Exec(stmt).Error
}

// enhanceKnowledgeSearchLogs 增强 knowledge_search_logs 表
//
// 新增字段（混合检索各阶段延迟/计数，用于监控）:
//   - vector_count / bm25_count / fused_count / rerank_count
//   - vector_latency_ms / bm25_latency_ms / rewrite_latency_ms / rerank_latency_ms
//   - rewrite_used / cache_hit
func (m *RagHybridMigration) enhanceKnowledgeSearchLogs(ctx context.Context) error {
	// 检查表是否存在
	var tableExists bool
	if err := m.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'knowledge_search_logs')`,
	).Scan(&tableExists).Error; err != nil {
		return err
	}
	if !tableExists {
		// 表不存在时创建（首次部署）
		stmt := `
			CREATE TABLE knowledge_search_logs (
				id                  BIGSERIAL PRIMARY KEY,
				query               TEXT,
				product_id          BIGINT,
				top_k               INT,
				vector_count        INT      DEFAULT 0,
				bm25_count          INT      DEFAULT 0,
				fused_count         INT      DEFAULT 0,
				rerank_count        INT      DEFAULT 0,
				vector_latency_ms   BIGINT   DEFAULT 0,
				bm25_latency_ms     BIGINT   DEFAULT 0,
				rewrite_latency_ms  BIGINT   DEFAULT 0,
				rerank_latency_ms   BIGINT   DEFAULT 0,
				rewrite_used        VARCHAR(50),
				cache_hit           BOOLEAN  DEFAULT false,
				created_at          TIMESTAMP DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_knowledge_search_logs_created ON knowledge_search_logs(created_at);
			CREATE INDEX IF NOT EXISTS idx_knowledge_search_logs_product ON knowledge_search_logs(product_id);
		`
		return m.db.WithContext(ctx).Exec(stmt).Error
	}

	// 表已存在时增加列（幂等）
	stmts := []string{
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS vector_count INT DEFAULT 0`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS bm25_count INT DEFAULT 0`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS fused_count INT DEFAULT 0`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS rerank_count INT DEFAULT 0`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS vector_latency_ms BIGINT DEFAULT 0`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS bm25_latency_ms BIGINT DEFAULT 0`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS rewrite_latency_ms BIGINT DEFAULT 0`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS rerank_latency_ms BIGINT DEFAULT 0`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS rewrite_used VARCHAR(50)`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS cache_hit BOOLEAN DEFAULT false`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS product_id BIGINT`,
		`ALTER TABLE knowledge_search_logs ADD COLUMN IF NOT EXISTS top_k INT`,
	}
	for _, sql := range stmts {
		_ = m.db.WithContext(ctx).Exec(sql).Error
	}
	// 新增索引（幂等）
	_ = m.db.WithContext(ctx).Exec(`CREATE INDEX IF NOT EXISTS idx_knowledge_search_logs_created ON knowledge_search_logs(created_at)`).Error
	_ = m.db.WithContext(ctx).Exec(`CREATE INDEX IF NOT EXISTS idx_knowledge_search_logs_product ON knowledge_search_logs(product_id)`).Error
	return nil
}

// Down 执行降级
//
// 注意:
//   - 不删除 knowledge_chunks 表（业务数据，仅删除新增列）
//   - 删除 query_rewrite_cache / embedding_cache 缓存表（仅缓存数据，可重建）
//   - 不删除 knowledge_search_logs 表（历史监控数据）
func (m *RagHybridMigration) Down(ctx context.Context) error {
	// 删除触发器
	_ = m.db.WithContext(ctx).Exec(`DROP TRIGGER IF EXISTS knowledge_chunks_tsv_update ON knowledge_chunks`).Error
	_ = m.db.WithContext(ctx).Exec(`DROP FUNCTION IF EXISTS knowledge_chunks_tsv_trigger()`).Error

	// 删除索引（按名称幂等删除）
	indexes := []string{
		`DROP INDEX IF EXISTS idx_knowledge_chunks_content_hash`,
		`DROP INDEX IF EXISTS idx_knowledge_chunks_embedding_id`,
		`DROP INDEX IF EXISTS idx_knowledge_chunks_embed_status`,
		`DROP INDEX IF EXISTS idx_knowledge_chunks_content_tsv`,
		`DROP INDEX IF EXISTS idx_knowledge_chunks_contextual_tsv`,
	}
	for _, sql := range indexes {
		_ = m.db.WithContext(ctx).Exec(sql).Error
	}

	// 删除 knowledge_chunks 新增列
	cols := []string{
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS content_tsv`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS contextual_context`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS contextual_tsv`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS content_hash`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS embed_status`,
	}
	for _, sql := range cols {
		_ = m.db.WithContext(ctx).Exec(sql).Error
	}

	// 删除缓存表
	_ = m.db.WithContext(ctx).Exec(`DROP TABLE IF EXISTS query_rewrite_cache`).Error
	_ = m.db.WithContext(ctx).Exec(`DROP TABLE IF EXISTS embedding_cache`).Error

	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*RagHybridMigration)(nil)
