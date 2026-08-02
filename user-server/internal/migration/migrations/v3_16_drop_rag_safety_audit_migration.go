package migrations

// v3_16_drop_rag_safety_audit_migration.go 清理孤儿表 rag_safety_audit_logs
//
// 背景 (深度审查发现 风险):
//   - rag_safety_audit_logs 表于 018_cde_p1_gap_fixes.sql 创建, 包含
//     tenant_id VARCHAR(64) 字段
//   - 全项目 grep `rag_safety_audit` / `RagSafetyAudit` → 0 匹配
//   - 表无任何 Go 代码引用, 2 个索引 (idx_rag_safety_audit_tenant /
//     idx_rag_safety_audit_blocked) 全是孤儿
//   - commit 1 + commit 2 已完成:
//     - SafetyCheckRequest.AgentID 改为 uint
//     - 移除 HTTP controller, service 仅保留 3 个词检 (纯内存)
//   - 后续无计划重建该表 (越权检测已删除, 词检不需要 DB 落库)
//
// 迁移内容:
//   1. DROP INDEX IF EXISTS idx_rag_safety_audit_tenant
//   2. DROP INDEX IF EXISTS idx_rag_safety_audit_blocked
//   3. DROP TABLE IF EXISTS rag_safety_audit_logs
//
// 幂等: IF EXISTS 保护, 重复执行无副作用。
// 私域部署: 孤儿表清理, 释放存储 + 简化 schema 维护。

import (
	"context"
	"fmt"
	"log"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// RagSafetyAuditDropMigration 清理孤儿表 rag_safety_audit_logs (v3.16)
type RagSafetyAuditDropMigration struct {
	db *gorm.DB
}

// NewRagSafetyAuditDropMigration 创建迁移实例
func NewRagSafetyAuditDropMigration(db *gorm.DB) *RagSafetyAuditDropMigration {
	return &RagSafetyAuditDropMigration{db: db}
}

// Version 返回版本号
func (m *RagSafetyAuditDropMigration) Version() string { return "v3.16.0" }

// Name 返回迁移名称
func (m *RagSafetyAuditDropMigration) Name() string {
	return "清理孤儿表 rag_safety_audit_logs (commit 3 二次审查)"
}

// Description 返回迁移描述
func (m *RagSafetyAuditDropMigration) Description() string {
	return "2026-08-01 二次深度审查: rag_safety_audit_logs 表无任何 Go 引用, tenant_id 字段是历史 DDL 残留。DROP 2 个孤儿索引 + DROP 表。"
}

// Up 执行迁移
func (m *RagSafetyAuditDropMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	log.Println("[v3.16] 开始清理孤儿表 rag_safety_audit_logs ...")

	// ---- 1. DROP 孤儿索引 ----
	log.Println("[v3.16] 1/3 DROP INDEX idx_rag_safety_audit_tenant ...")
	if err := m.db.WithContext(ctx).Exec(
		`DROP INDEX IF EXISTS idx_rag_safety_audit_tenant`,
	).Error; err != nil {
		return fmt.Errorf("v3.16 1/3 DROP 索引 idx_rag_safety_audit_tenant 失败: %w", err)
	}
	log.Println("[v3.16] ✓ idx_rag_safety_audit_tenant 已删除")

	log.Println("[v3.16] 2/3 DROP INDEX idx_rag_safety_audit_blocked ...")
	if err := m.db.WithContext(ctx).Exec(
		`DROP INDEX IF EXISTS idx_rag_safety_audit_blocked`,
	).Error; err != nil {
		return fmt.Errorf("v3.16 2/3 DROP 索引 idx_rag_safety_audit_blocked 失败: %w", err)
	}
	log.Println("[v3.16] ✓ idx_rag_safety_audit_blocked 已删除")

	// ---- 2. DROP 孤儿表 ----
	log.Println("[v3.16] 3/3 DROP TABLE rag_safety_audit_logs ...")
	if err := m.db.WithContext(ctx).Exec(
		`DROP TABLE IF EXISTS rag_safety_audit_logs`,
	).Error; err != nil {
		return fmt.Errorf("v3.16 3/3 DROP 表 rag_safety_audit_logs 失败: %w", err)
	}
	log.Println("[v3.16] ✓ rag_safety_audit_logs 表已删除")

	log.Println("[v3.16] 孤儿表清理完成")
	return nil
}

// Down 执行回滚 (谨慎: 孤儿表数据本来就不存在, 仅重建空表)
func (m *RagSafetyAuditDropMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	log.Println("[v3.16] 回滚: 重建空表 rag_safety_audit_logs ...")
	stmt := `
CREATE TABLE IF NOT EXISTS rag_safety_audit_logs (
    id            BIGSERIAL PRIMARY KEY,
    user_id       VARCHAR(64),
    stage         VARCHAR(16)  NOT NULL,
    content_hash  VARCHAR(64)  NOT NULL,
    blocked       BOOLEAN      NOT NULL DEFAULT FALSE,
    issues        JSONB        NOT NULL DEFAULT '[]',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rag_safety_audit_blocked ON rag_safety_audit_logs (blocked) WHERE blocked = TRUE;
`
	if err := m.db.WithContext(ctx).Exec(stmt).Error; err != nil {
		return fmt.Errorf("v3.16 回滚重建 rag_safety_audit_logs 失败: %w", err)
	}
	log.Println("[v3.16] ✓ 回滚完成 (注意: 不重建 tenant_id 字段和 idx_tenant 索引, 因为二者都是多租户残留)")
	return nil
}

// Ensure RagSafetyAuditDropMigration implements Migration interface
var _ migration.Migration = (*RagSafetyAuditDropMigration)(nil)
