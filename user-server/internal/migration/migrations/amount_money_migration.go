package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// AmountMoneyMigration 金额字段 BIGINT 重构迁移
type AmountMoneyMigration struct {
	db *gorm.DB
}

// NewAmountMoneyMigration 创建迁移实例
func NewAmountMoneyMigration(db *gorm.DB) *AmountMoneyMigration {
	return &AmountMoneyMigration{db: db}
}

// Version 返回版本号
func (m *AmountMoneyMigration) Version() string { return "v2.9.0" }

// Name 返回迁移名称
func (m *AmountMoneyMigration) Name() string { return "金额字段 BIGINT 重构（分）" }

// Description 返回迁移描述
func (m *AmountMoneyMigration) Description() string {
	return "将 market_templates.price / rfm_rules.m_amount_* / user_rfms.total_amount / sales_personas.avg_deal_amount 等 decimal 金额字段重构为 bigint（单位：分）"
}

// Up 执行升级
func (m *AmountMoneyMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	if err := m.migrateMarketTemplatePrice(ctx); err != nil {
		return fmt.Errorf("migrate market_templates.price 失败: %w", err)
	}

	if err := m.migrateRFMRuleMAmounts(ctx); err != nil {
		return fmt.Errorf("migrate rfm_rules.m_amount_* 失败: %w", err)
	}

	if err := m.migrateUserRFMAmounts(ctx); err != nil {
		return fmt.Errorf("migrate user_rfms.total_amount / avg_amount 失败: %w", err)
	}

	if err := m.migrateSalesPersonaAmounts(ctx); err != nil {
		return fmt.Errorf("migrate sales_personas.avg_deal_amount / total_revenue 失败: %w", err)
	}

	if err := m.migrateExternalOrderAmounts(ctx); err != nil {
		return fmt.Errorf("migrate external_orders.total_amount / pay_amount / discount_amount 失败: %w", err)
	}

	if err := m.migrateExternalProductAmounts(ctx); err != nil {
		return fmt.Errorf("migrate external_products.price / original_price 失败: %w", err)
	}

	if err := m.migrateDecimalToBigint(ctx, "ab_conversion_events", "event_value"); err != nil {
		return fmt.Errorf("migrate ab_conversion_events.event_value 失败: %w", err)
	}

	if err := m.migrateDecimalToBigint(ctx, "ab_experiment_results", "revenue"); err != nil {
		return fmt.Errorf("migrate ab_experiment_results.revenue 失败: %w", err)
	}

	return nil
}

func (m *AmountMoneyMigration) migrateMarketTemplatePrice(ctx context.Context) error {
	return m.migrateDecimalToBigint(ctx, "market_templates", "price")
}

func (m *AmountMoneyMigration) migrateRFMRuleMAmounts(ctx context.Context) error {
	columns := []string{"m_amount_1", "m_amount_2", "m_amount_3", "m_amount_4", "m_amount_5"}
	for _, col := range columns {
		if err := m.migrateDecimalToBigint(ctx, "rfm_rules", col); err != nil {
			return fmt.Errorf("migrate rfm_rules.%s 失败: %w", col, err)
		}
	}
	return nil
}

func (m *AmountMoneyMigration) migrateUserRFMAmounts(ctx context.Context) error {
	columns := []string{"total_amount", "avg_amount"}
	for _, col := range columns {
		if err := m.migrateDecimalToBigint(ctx, "user_rfms", col); err != nil {
			return fmt.Errorf("migrate user_rfms.%s 失败: %w", col, err)
		}
	}
	return nil
}

func (m *AmountMoneyMigration) migrateSalesPersonaAmounts(ctx context.Context) error {
	columns := []string{"avg_deal_amount", "total_revenue"}
	for _, col := range columns {
		if err := m.migrateDecimalToBigint(ctx, "sales_personas", col); err != nil {
			return fmt.Errorf("migrate sales_personas.%s 失败: %w", col, err)
		}
	}
	return nil
}

func (m *AmountMoneyMigration) migrateExternalOrderAmounts(ctx context.Context) error {
	columns := []string{"total_amount", "pay_amount", "discount_amount"}
	for _, col := range columns {
		if err := m.migrateDecimalToBigint(ctx, "external_orders", col); err != nil {
			return fmt.Errorf("migrate external_orders.%s 失败: %w", col, err)
		}
	}
	return nil
}

func (m *AmountMoneyMigration) migrateExternalProductAmounts(ctx context.Context) error {
	columns := []string{"price", "original_price"}
	for _, col := range columns {
		if err := m.migrateDecimalToBigint(ctx, "external_products", col); err != nil {
			return fmt.Errorf("migrate external_products.%s 失败: %w", col, err)
		}
	}
	return nil
}

func (m *AmountMoneyMigration) migrateDecimalToBigint(ctx context.Context, table, column string) error {

	var dataType string
	err := m.db.WithContext(ctx).
		Raw(`SELECT data_type FROM information_schema.columns
		     WHERE table_name = ? AND column_name = ?`, table, column).
		Scan(&dataType).Error
	if err != nil {
		return fmt.Errorf("查询 %s.%s 列类型失败: %w", table, column, err)
	}
	if dataType == "" {
		return nil
	}
	if dataType != "numeric" && dataType != "decimal" {
		return nil
	}

	newCol := column + "_new"
	stmts := []string{
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s bigint NOT NULL DEFAULT 0`, table, newCol),
		fmt.Sprintf(`UPDATE %s SET %s = ROUND(%s * 100)::bigint`, table, newCol, column),
		fmt.Sprintf(`ALTER TABLE %s DROP COLUMN %s`, table, column),
		fmt.Sprintf(`ALTER TABLE %s RENAME COLUMN %s TO %s`, table, newCol, column),
	}
	return execAll(ctx, m.db, stmts)
}

// Down 回滚（不自动回滚，避免数据精度损失）
//
// 注意：金额字段一旦升级为 bigint（分），不应回滚为 decimal（元），
// 因为回滚需要除以 100，可能引入浮点精度问题。如必须回滚，请手动执行 SQL。
func (m *AmountMoneyMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*AmountMoneyMigration)(nil)
