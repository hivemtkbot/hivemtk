package migrations

// knowledge_weight_migration.go 知识库自学习权重列
//
// 背景：
//   自学习/自进化机制基于追踪系统对每条 trace 打分，动态调整该 trace 涉及的知识库
//   chunk 权重（knowledge_chunks.weight）。权重作为检索排名的第二依据（相关性 score
//   为主序，weight 为调制因子），让系统越用越聪明。
//
// 设计要点：
//   - ADD COLUMN IF NOT EXISTS（PG 幂等可重入）
//   - 私域独立部署：无 merchant_id
//   - 默认 1.0：不影响存量召回；仅自学习模块调整后偏离 1.0

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// KnowledgeWeightMigration 知识库权重迁移
type KnowledgeWeightMigration struct {
	db *gorm.DB
}

// NewKnowledgeWeightMigration 创建迁移实例
func NewKnowledgeWeightMigration(db *gorm.DB) *KnowledgeWeightMigration {
	return &KnowledgeWeightMigration{db: db}
}

// Version 返回版本号
func (m *KnowledgeWeightMigration) Version() string { return "v3.13.0" }

// Name 返回迁移名称
func (m *KnowledgeWeightMigration) Name() string {
	return "知识库自学习 - knowledge_chunks.weight 列"
}

// Description 返回迁移描述
func (m *KnowledgeWeightMigration) Description() string {
	return "v3.13.0：为 knowledge_chunks 新增 weight 双精度列（默认 1.0），作为检索排名第二依据，支持自学习权重调整"
}

// Up 执行升级
func (m *KnowledgeWeightMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS weight double precision NOT NULL DEFAULT 1`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("knowledge_weight 执行失败 (%s): %w", truncate(s, 60), err)
		}
	}
	return nil
}

// Down 回滚
func (m *KnowledgeWeightMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS weight`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("knowledge_weight 回滚失败 (%s): %w", truncate(s, 60), err)
		}
	}
	return nil
}

// 编译期接口断言
var _ migration.Migration = (*KnowledgeWeightMigration)(nil)
