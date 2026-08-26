package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// CustomerOwnerAgentMigration customers 表增加 owner_agent_id 列（S-4 专属坐席定向路由）
//
// MASTER_COMPETITIVE_DECISIONS.md M19/S-4：assignment 优先匹配 customer.owner_agent_id
// 在线者，否则走现有分配算法。列可空（未绑定专属坐席的客户行为不变）。
type CustomerOwnerAgentMigration struct {
	db *gorm.DB
}

// NewCustomerOwnerAgentMigration 创建迁移实例
func NewCustomerOwnerAgentMigration(db *gorm.DB) *CustomerOwnerAgentMigration {
	return &CustomerOwnerAgentMigration{db: db}
}

// Version 返回版本号
func (m *CustomerOwnerAgentMigration) Version() string { return "v3.25.0" }

// Name 返回迁移名称
func (m *CustomerOwnerAgentMigration) Name() string {
	return "customers 增加 owner_agent_id 专属坐席列"
}

// Description 返回迁移描述
func (m *CustomerOwnerAgentMigration) Description() string {
	return "M19/S-4 专属坐席定向路由：customers 表新增可空列 owner_agent_id（指向 agent_statuses.agent_id）+ 索引"
}

// Up 执行升级
func (m *CustomerOwnerAgentMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if !m.db.Migrator().HasTable("customers") {
		return nil
	}
	if m.db.Migrator().HasColumn("customers", "owner_agent_id") {
		return nil // 幂等
	}
	stmts := []string{
		`ALTER TABLE customers ADD COLUMN IF NOT EXISTS owner_agent_id BIGINT`,
		`CREATE INDEX IF NOT EXISTS idx_customers_owner_agent ON customers (owner_agent_id)`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("customers.owner_agent_id 迁移失败: %w", err)
		}
	}
	return nil
}

// Down 执行降级（不删列，避免误删绑定数据；如需回滚请手动处理）
func (m *CustomerOwnerAgentMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*CustomerOwnerAgentMigration)(nil)
