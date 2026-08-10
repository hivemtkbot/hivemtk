package migrations

// amount_money_migration.go 金额字段 BIGINT 重构迁移 v2.9.0
//
// 五层架构归属: L5 数据层
// 设计依据: docs/standards/MASTER_RULES.md「金额一律用 BIGINT 存「分」，不要用 NUMERIC/FLOAT」
// 私域独立部署: 无 merchant_id 字段
//
// 本迁移将历史遗留的 float64 + decimal(10,2) 金额字段重构为 int64 + bigint（单位：分）：
//  1. market_templates.price             decimal(10,2) → bigint（数据 * 100 转换）
//  2. rfm_rules.m_amount_1 ~ m_amount_5  decimal(10,2) → bigint（数据 * 100 转换）
//  3. user_rfms.total_amount             decimal(10,2) → bigint（数据 * 100 转换）
//  4. user_rfms.avg_amount               decimal(10,2) → bigint（数据 * 100 转换）
//  5. sales_personas.avg_deal_amount     decimal(12,2) → bigint（数据 * 100 转换）
//  6. sales_personas.total_revenue       decimal(14,2) → bigint（数据 * 100 转换）
//  7. external_orders.total_amount       decimal(10,2) → bigint（数据 * 100 转换）
//  8. external_orders.pay_amount         decimal(10,2) → bigint（数据 * 100 转换）
//  9. external_orders.discount_amount    decimal(10,2) → bigint（数据 * 100 转换）
// 10. external_products.price            decimal(10,2) → bigint（数据 * 100 转换）
// 11. external_products.original_price   decimal(10,2) → bigint（数据 * 100 转换）
// 12. ab_conversion_events.event_value   decimal(10,2) → bigint（数据 * 100 转换）
// 13. ab_experiment_results.revenue      decimal(10,2) → bigint（数据 * 100 转换）
// 14. ab_experiment_results.average_value（原无 gorm 类型，AutoMigrate 自动建为 bigint，无需数据迁移）
//
// 保留 decimal 不转换的字段：
//   - ai_sales_logs.cost（decimal(10,4)）：LLM API 调用成本（美元），4 位小数无法用分表示
//   - ops/churn_predictions.average_order_value（decimal(10,2)）：0-100 评分（非金额）
//   - 各类 *_score / *_rate / *_weight（decimal(5,2)）：评分/比率/权重（非金额）
//
// 幂等性: 通过检查当前列类型决定是否执行，可重入
// 依赖: market_templates / rfm_rules / user_rfms / sales_personas / external_orders / external_products 表已存在（由 initial_schema 创建）

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

	// 1. market_templates.price: decimal(10,2) → bigint（数据 * 100 转换）
	if err := m.migrateMarketTemplatePrice(ctx); err != nil {
		return fmt.Errorf("migrate market_templates.price 失败: %w", err)
	}

	// 2. rfm_rules.m_amount_1 ~ m_amount_5: decimal(10,2) → bigint（数据 * 100 转换）
	if err := m.migrateRFMRuleMAmounts(ctx); err != nil {
		return fmt.Errorf("migrate rfm_rules.m_amount_* 失败: %w", err)
	}

	// 3. user_rfms.total_amount & avg_amount: decimal(10,2) → bigint（数据 * 100 转换）
	if err := m.migrateUserRFMAmounts(ctx); err != nil {
		return fmt.Errorf("migrate user_rfms.total_amount / avg_amount 失败: %w", err)
	}

	// 4. sales_personas.avg_deal_amount & total_revenue: decimal → bigint（数据 * 100 转换）
	if err := m.migrateSalesPersonaAmounts(ctx); err != nil {
		return fmt.Errorf("migrate sales_personas.avg_deal_amount / total_revenue 失败: %w", err)
	}

	// 5. external_orders.total_amount / pay_amount / discount_amount: decimal(10,2) → bigint（数据 * 100 转换）
	if err := m.migrateExternalOrderAmounts(ctx); err != nil {
		return fmt.Errorf("migrate external_orders.total_amount / pay_amount / discount_amount 失败: %w", err)
	}

	// 6. external_products.price / original_price: decimal(10,2) → bigint（数据 * 100 转换）
	if err := m.migrateExternalProductAmounts(ctx); err != nil {
		return fmt.Errorf("migrate external_products.price / original_price 失败: %w", err)
	}

	// 7. ab_conversion_events.event_value: decimal(10,2) → bigint（数据 * 100 转换）
	if err := m.migrateDecimalToBigint(ctx, "ab_conversion_events", "event_value"); err != nil {
		return fmt.Errorf("migrate ab_conversion_events.event_value 失败: %w", err)
	}

	// 8. ab_experiment_results.revenue: decimal(10,2) → bigint（数据 * 100 转换）
	//    average_value 原无 gorm 类型，AutoMigrate 自动建为 bigint，无需数据迁移
	if err := m.migrateDecimalToBigint(ctx, "ab_experiment_results", "revenue"); err != nil {
		return fmt.Errorf("migrate ab_experiment_results.revenue 失败: %w", err)
	}

	return nil
}

// migrateMarketTemplatePrice 迁移 market_templates.price 字段
// decimal(10,2) → bigint，数据 * 100 转换为分
func (m *AmountMoneyMigration) migrateMarketTemplatePrice(ctx context.Context) error {
	return m.migrateDecimalToBigint(ctx, "market_templates", "price")
}

// migrateRFMRuleMAmounts 迁移 rfm_rules 表 m_amount_1 ~ m_amount_5 五个字段
// decimal(10,2) → bigint，数据 * 100 转换为分
func (m *AmountMoneyMigration) migrateRFMRuleMAmounts(ctx context.Context) error {
	columns := []string{"m_amount_1", "m_amount_2", "m_amount_3", "m_amount_4", "m_amount_5"}
	for _, col := range columns {
		if err := m.migrateDecimalToBigint(ctx, "rfm_rules", col); err != nil {
			return fmt.Errorf("migrate rfm_rules.%s 失败: %w", col, err)
		}
	}
	return nil
}

// migrateUserRFMAmounts 迁移 user_rfms 表 total_amount & avg_amount 两个字段
// decimal(10,2) → bigint，数据 * 100 转换为分
func (m *AmountMoneyMigration) migrateUserRFMAmounts(ctx context.Context) error {
	columns := []string{"total_amount", "avg_amount"}
	for _, col := range columns {
		if err := m.migrateDecimalToBigint(ctx, "user_rfms", col); err != nil {
			return fmt.Errorf("migrate user_rfms.%s 失败: %w", col, err)
		}
	}
	return nil
}

// migrateSalesPersonaAmounts 迁移 sales_personas 表 avg_deal_amount & total_revenue 两个字段
// decimal(12,2)/decimal(14,2) → bigint，数据 * 100 转换为分
func (m *AmountMoneyMigration) migrateSalesPersonaAmounts(ctx context.Context) error {
	columns := []string{"avg_deal_amount", "total_revenue"}
	for _, col := range columns {
		if err := m.migrateDecimalToBigint(ctx, "sales_personas", col); err != nil {
			return fmt.Errorf("migrate sales_personas.%s 失败: %w", col, err)
		}
	}
	return nil
}

// migrateExternalOrderAmounts 迁移 external_orders 表 total_amount / pay_amount / discount_amount 三个字段
// decimal(10,2) → bigint，数据 * 100 转换为分
func (m *AmountMoneyMigration) migrateExternalOrderAmounts(ctx context.Context) error {
	columns := []string{"total_amount", "pay_amount", "discount_amount"}
	for _, col := range columns {
		if err := m.migrateDecimalToBigint(ctx, "external_orders", col); err != nil {
			return fmt.Errorf("migrate external_orders.%s 失败: %w", col, err)
		}
	}
	return nil
}

// migrateExternalProductAmounts 迁移 external_products 表 price & original_price 两个字段
// decimal(10,2) → bigint，数据 * 100 转换为分
func (m *AmountMoneyMigration) migrateExternalProductAmounts(ctx context.Context) error {
	columns := []string{"price", "original_price"}
	for _, col := range columns {
		if err := m.migrateDecimalToBigint(ctx, "external_products", col); err != nil {
			return fmt.Errorf("migrate external_products.%s 失败: %w", col, err)
		}
	}
	return nil
}

// migrateDecimalToBigint 通用迁移：将指定表的指定列从 decimal/numeric → bigint（数据 * 100）
//
// 流程：
//  1. 查询当前列类型（幂等：若已是 bigint 则跳过）
//  2. 添加临时列 _new bigint
//  3. 数据迁移：decimal * 100 → bigint（四舍五入）
//  4. 删除旧列
//  5. 重命名新列为原列名
func (m *AmountMoneyMigration) migrateDecimalToBigint(ctx context.Context, table, column string) error {
	// 检查当前列类型（幂等：若已是 bigint 则跳过）
	var dataType string
	err := m.db.WithContext(ctx).
		Raw(`SELECT data_type FROM information_schema.columns
		     WHERE table_name = ? AND column_name = ?`, table, column).
		Scan(&dataType).Error
	if err != nil {
		return fmt.Errorf("查询 %s.%s 列类型失败: %w", table, column, err)
	}
	if dataType == "" {
		// 列不存在，由 AutoMigrate 创建，无需迁移
		return nil
	}
	if dataType != "numeric" && dataType != "decimal" {
		// 已是 bigint 或其他类型，跳过
		return nil
	}

	// 将 decimal 转为 bigint（数据 * 100 转换为分）
	newCol := column + "_new"
	stmts := []string{
		// 1. 临时列存储 bigint 值
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s bigint NOT NULL DEFAULT 0`, table, newCol),
		// 2. 数据迁移：decimal * 100 → bigint（四舍五入）
		fmt.Sprintf(`UPDATE %s SET %s = ROUND(%s * 100)::bigint`, table, newCol, column),
		// 3. 删除旧列
		fmt.Sprintf(`ALTER TABLE %s DROP COLUMN %s`, table, column),
		// 4. 重命名新列
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

// compile-time 接口断言
var _ migration.Migration = (*AmountMoneyMigration)(nil)
