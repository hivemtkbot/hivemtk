package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// IntentMergeMigration OPT-DB-10: intent_records 与 intent_logs 整合
//
// 现状：
//   - intent_records（销售意图识别记录）：记录销售场景下的意图识别结果，
//     包含 IntentType/IntentSubtype/Confidence/Entities/Sentiment 等业务字段。
//   - intent_logs（精细意图识别日志）：记录 LLM 层的精细意图识别结果，
//     包含 IntentMajor/IntentMinor/Method/Reasoning/TraceID 等运行时字段。
//
// 两张表功能重叠且数据源相同（同一 LLM 意图识别链路），应当合并为一。
//
// 迁移策略：
//   1. 为 intent_records 添加来自 intent_logs 的缺失字段（method / reasoning / trace_id / timestamp）
//   2. 添加 source 字段标记记录来源（'sales' 来自原 intent_records, 'fine_grained' 来自原 intent_logs）
//   3. 将 intent_logs 数据按字段映射并入 intent_records
//   4. 删除原 intent_logs 表
//
// 幂等安全：每一步先检查目标和前置条件，已执行则跳过。
type IntentMergeMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*IntentMergeMigration)(nil)

func NewIntentMergeMigration(db *gorm.DB) *IntentMergeMigration {
	return &IntentMergeMigration{db: db}
}

func (m *IntentMergeMigration) Version() string { return "v3.22.3" }

func (m *IntentMergeMigration) Name() string { return "intent_records 与 intent_logs 整合" }

func (m *IntentMergeMigration) Description() string {
	return "合并 intent_records 与 intent_logs 两张意图表，统一为 intent_records"
}

func (m *IntentMergeMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 检查 intent_logs 表是否存在
	var intentLogsExists bool
	err := m.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'intent_logs')`,
	).Scan(&intentLogsExists).Error
	if err != nil {
		return fmt.Errorf("检查 intent_logs 表失败: %w", err)
	}

	if !intentLogsExists {
		// intent_logs 不存在，无需合并
		return nil
	}

	// 检查 intent_records 表是否存在
	var intentRecordsExists bool
	err = m.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'intent_records')`,
	).Scan(&intentRecordsExists).Error
	if err != nil {
		return fmt.Errorf("检查 intent_records 表失败: %w", err)
	}

	if !intentRecordsExists {
		return fmt.Errorf("intent_records 表不存在，无法合并")
	}

	// Step 1: 为 intent_records 添加缺失字段
	addColumns := map[string]string{
		"method":    `ADD COLUMN IF NOT EXISTS method varchar(16) NOT NULL DEFAULT 'llm'`,
		"reasoning": `ADD COLUMN IF NOT EXISTS reasoning text`,
		"trace_id":  `ADD COLUMN IF NOT EXISTS trace_id varchar(64)`,
		"timestamp": `ADD COLUMN IF NOT EXISTS timestamp timestamptz`,
		"source":    `ADD COLUMN IF NOT EXISTS source varchar(32) NOT NULL DEFAULT 'sales'`,
	}

	for col, def := range addColumns {
		sql := fmt.Sprintf(`ALTER TABLE intent_records %s`, def)
		if err := m.db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("添加列 intent_records.%s 失败: %w", col, err)
		}
	}

	// 为 trace_id 和 timestamp 添加索引
	_ = m.db.WithContext(ctx).Exec(`CREATE INDEX IF NOT EXISTS idx_intent_records_trace_id ON intent_records (trace_id)`)
	_ = m.db.WithContext(ctx).Exec(`CREATE INDEX IF NOT EXISTS idx_intent_records_timestamp ON intent_records (timestamp)`)
	_ = m.db.WithContext(ctx).Exec(`CREATE INDEX IF NOT EXISTS idx_intent_records_source ON intent_records (source)`)

	// Step 2: 检查是否已合并过（source 列已有非 'sales' 值表示已合并）
	var mergedCount int64
	m.db.WithContext(ctx).Model(&struct{}{}).
		Table("intent_records").
		Where("source = ?", "fine_grained").
		Count(&mergedCount)

	if mergedCount == 0 {
		// Step 3: 将 intent_logs 数据并入 intent_records
		// 字段映射：intent_logs → intent_records
		//   customer_id → customer_id
		//   session_id → session_id
		//   message → raw_text
		//   intent_major → intent_type
		//   intent_minor → intent_subtype
		//   confidence → confidence
		//   method → method
		//   latency_ms → latency_ms
		//   reasoning → reasoning
		//   trace_id → trace_id
		//   timestamp → timestamp
		//   created_at → created_at
		//   source = 'fine_grained' (fixed)
		insertSQL := `
			INSERT INTO intent_records (session_id, customer_id, raw_text, intent_type, intent_subtype,
				confidence, method, latency_ms, reasoning, trace_id, timestamp, created_at, source)
			SELECT
				il.session_id,
				il.customer_id,
				il.message,
				il.intent_major,
				il.intent_minor,
				il.confidence,
				il.method,
				il.latency_ms,
				COALESCE(il.reasoning, ''),
				il.trace_id,
				il.timestamp,
				il.created_at,
				'fine_grained'
			FROM intent_logs il
			WHERE NOT EXISTS (
				SELECT 1 FROM intent_records ir
				WHERE ir.session_id = il.session_id
				  AND ir.created_at = il.created_at
				  AND ir.source = 'fine_grained'
			)
		`
		result := m.db.WithContext(ctx).Exec(insertSQL)
		if result.Error != nil {
			return fmt.Errorf("合并 intent_logs 数据失败: %w", result.Error)
		}
	}

	// Step 4: 删除 intent_logs 表
	_ = m.db.WithContext(ctx).Exec(`DROP TABLE IF EXISTS intent_logs`)

	return nil
}

func (m *IntentMergeMigration) Down(ctx context.Context) error {
	return nil
}