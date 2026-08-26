package ragretrieval

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/utils/logger"
)

// ─────────────────────────────────────────────────────────────────────────────
// M6 R-7 pgvector 索引运维（MASTER_COMPETITIVE_DECISIONS.md）
//
// 决策要点：
//   - chunk 行数 < 50,000 时无需建向量索引（顺序扫描更快，避免维护开销）
//   - KB chunk 数 > 50,000 时建 HNSW 索引：
//
//	CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_knowledge_chunks_embedding_hnsw
//	    ON knowledge_chunks USING hnsw (embedding vector_cosine_ops)
//	    WITH (m = 16, ef_construction = 200);
//
//   - 查询会话调参：SET hnsw.ef_search = 100
//   - 过滤型查询（带 product_id 等条件）开启迭代扫描：
//
//	SET hnsw.iterative_scan = relaxed_order;
//	SET hnsw.relax_iterative_scan_limit = 2;  -- 可选
//
//   - 记忆表（dialogue_memory 相关向量列）达到同量级时同样处理
//
// 运维手册：
//  1. 检查行数：SELECT count(*) FROM knowledge_chunks;
//  2. 检查索引：SELECT indexname FROM pg_indexes
//               WHERE tablename='knowledge_chunks' AND indexdef LIKE '%hnsw%';
//  3. 达到阈值后调用 EnsureHNSWIndexIfNeeded（或手工执行上面 DDL；
//     大表务必用 CONCURRENTLY 避免锁写）。
//  4. 建索引后观察 EXPLAIN ANALYZE 确认走 Index Scan。
// ─────────────────────────────────────────────────────────────────────────────

const (
	// HNSWMinRowsForIndex 建议建 HNSW 索引的最小行数（R-7：<5 万不建）
	HNSWMinRowsForIndex int64 = 50000

	// HNSWM HNSW 图的最大连接数（R-7 落地值 m16）
	HNSWM = 16

	// HNSWEfConstruction 建索引时的候选队列长度（R-7 落地值 ef200）
	HNSWEfConstruction = 200

	// HNSWEfSearch 查询时候选队列长度（R-7 落地值 ef_search=100）
	HNSWEfSearch = 100
)

// EnsureHNSWIndexIfNeeded R-7 迁移辅助：行数超过阈值且索引缺失时创建 HNSW 索引。
//
// 参数：
//   - db: 数据库句柄
//   - table: 表名（如 knowledge_chunks）
//   - column: 向量列名（如 embedding）
//   - indexName: 索引名（如 idx_knowledge_chunks_embedding_hnsw）
//
// 返回 created=true 表示本次实际创建了索引。行数未达阈值或索引已存在时不做任何事。
// 注意：本函数使用普通 CREATE INDEX（迁移框架内执行）；生产大表建议运维手工
// 用 CREATE INDEX CONCURRENTLY 执行同等 DDL 以避免锁写。
func EnsureHNSWIndexIfNeeded(ctx context.Context, db *gorm.DB, table, column, indexName string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("db 未初始化")
	}
	table = sanitizeIdent(table)
	column = sanitizeIdent(column)

	var cnt int64
	if err := db.WithContext(ctx).
		Raw(fmt.Sprintf(`SELECT count(*) FROM %s`, table)).
		Scan(&cnt).Error; err != nil {
		return false, fmt.Errorf("统计行数失败: %w", err)
	}
	if cnt < HNSWMinRowsForIndex {
		logger.Infof("[R-7] %s rows=%d < %d, skip hnsw index", table, cnt, HNSWMinRowsForIndex)
		return false, nil
	}

	var exists bool
	if err := db.WithContext(ctx).Raw(`
		SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?)
	`, indexName).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("检查索引存在性失败: %w", err)
	}
	if exists {
		return false, nil
	}

	ddl := fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw ((%s) vector_cosine_ops) WITH (m = %d, ef_construction = %d)`,
		sanitizeIdent(indexName), table, column, HNSWM, HNSWEfConstruction,
	)
	if err := db.WithContext(ctx).Exec(ddl).Error; err != nil {
		return false, fmt.Errorf("创建 HNSW 索引失败: %w", err)
	}
	logger.Infof("[R-7] created hnsw index %s on %s(%s), rows=%d", indexName, table, column, cnt)
	return true, nil
}

// ApplyHNSWSessionParams 会话级查询调参（R-7：ef_search=100 + iterative_scan）。
// 在执行向量检索的连接/事务内调用；iterative_scan 用于带过滤条件的近邻查询，
// 避免 HNSW 在过滤后候选不足时召回塌陷。pgvector 版本不支持 iterative_scan 时静默忽略。
func ApplyHNSWSessionParams(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db 未初始化")
	}
	if err := db.WithContext(ctx).Exec(fmt.Sprintf(`SET hnsw.ef_search = %d`, HNSWEfSearch)).Error; err != nil {
		return err
	}
	// 老版本 pgvector 无该 GUC，忽略错误
	_ = db.WithContext(ctx).Exec(`SET hnsw.iterative_scan = relaxed_order`).Error
	return nil
}

// sanitizeIdent 白名单校验标识符（表/列/索引名），防 SQL 注入（拼接 DDL 场景）
func sanitizeIdent(s string) string {
	const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_."
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(allowed, r) {
			return r
		}
		return -1
	}, s)
	if clean == "" || clean != s {
		panic(fmt.Sprintf("非法标识符: %q", s))
	}
	return clean
}
