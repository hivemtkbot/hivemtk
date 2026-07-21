package migrations

// confidence_migration_test.go 置信度驱动转人工迁移 v2.8.0 PG 集成测试
//
// 覆盖：
//  1. Version / Name / Description 元信息
//  2. Up() 创建 8 张表 + 11 条默认策略
//  3. Up() 幂等：重复执行不报错
//  4. seedDefaultPolicies 幂等：已有数据不覆盖
//  5. Down() 回滚 6 张表（保留 calibrations / policies）
//  6. Down() 后 calibrations / policies 仍存在
//  7. 各表可正常写入数据（验证字段完整）
//  8. nil db 返回错误
//  9. 默认策略字段值正确

import (
	"context"
	"testing"

	"marketing/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupConfidenceMigrationTestDB 创建迁移测试 DB（空库）
func setupConfidenceMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 项目规则"不允许跳过"：PG 集成测试必须运行，testutil.NewTestDB 在连接失败时 t.Fatal
	return testutil.NewTestDB(t)
}

// TestConfidenceMigration_Version 验证元信息
func TestConfidenceMigration_Version(t *testing.T) {
	m := NewConfidenceMigration(nil)
	if m.Version() != "v2.8.0" {
		t.Errorf("Version()=%q want=v2.8.0", m.Version())
	}
	if m.Name() == "" {
		t.Error("Name() should not be empty")
	}
	if m.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

// TestConfidenceMigration_NilDB nil db 返回错误
func TestConfidenceMigration_NilDB(t *testing.T) {
	m := NewConfidenceMigration(nil)
	if err := m.Up(context.Background()); err == nil {
		t.Errorf("nil db Up() 应返回错误")
	}
}

// TestConfidenceMigration_UpCreatesAllTables 集成测试：Up 创建所有 8 张表
func TestConfidenceMigration_UpCreatesAllTables(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	expectedTables := []string{
		"confidence_signals",
		"confidence_calibrations",
		"handoff_decisions",
		"review_queue",
		"threshold_policies",
		"sla_monitors",
		"ab_tests",
		"ab_test_metrics",
	}
	for _, table := range expectedTables {
		var exists bool
		if err := db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = ?)`, table).Scan(&exists).Error; err != nil || !exists {
			t.Errorf("表 %s 应存在 after Up(): exists=%v err=%v", table, exists, err)
		}
	}
}

// TestConfidenceMigration_UpIdempotent 集成测试：Up 幂等
func TestConfidenceMigration_UpIdempotent(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("first Up() failed: %v", err)
	}
	// 第二次 Up 不应报错
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("second Up() should be idempotent, got: %v", err)
	}
	// 第三次 Up 不应报错
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("third Up() should be idempotent, got: %v", err)
	}
}

// TestConfidenceMigration_SeedDefaultPolicies 集成测试：Up 后插入 11 条默认策略
func TestConfidenceMigration_SeedDefaultPolicies(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM threshold_policies`).Scan(&count).Error; err != nil {
		t.Fatalf("查询策略数失败: %v", err)
	}
	if count != 11 {
		t.Errorf("默认策略应为 11 条, got %d", count)
	}
}

// TestConfidenceMigration_SeedPoliciesIdempotent 集成测试：种子幂等
// 重复 Up 不应增加策略数
func TestConfidenceMigration_SeedPoliciesIdempotent(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("first Up() failed: %v", err)
	}

	var countAfterFirst int64
	_ = db.Raw(`SELECT COUNT(*) FROM threshold_policies`).Scan(&countAfterFirst).Error

	// 再次 Up
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("second Up() failed: %v", err)
	}

	var countAfterSecond int64
	_ = db.Raw(`SELECT COUNT(*) FROM threshold_policies`).Scan(&countAfterSecond).Error

	if countAfterSecond != countAfterFirst {
		t.Errorf("种子应幂等: first=%d second=%d", countAfterFirst, countAfterSecond)
	}
}

// TestConfidenceMigration_DefaultPoliciesFields 集成测试：默认策略字段值正确
func TestConfidenceMigration_DefaultPoliciesFields(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	cases := []struct {
		intent string
		base   float64
	}{
		{"default", 0.70},
		{"complaint", 0.85},
		{"churn", 0.85},
		{"objection", 0.75},
		{"ask_product", 0.70},
		{"ask_service", 0.70},
		{"price_inquiry", 0.65},
		{"purchase", 0.65},
		{"after_sale", 0.80},
		{"social", 0.50},
		{"greeting", 0.50},
	}
	for _, tc := range cases {
		var base float64
		err := db.Raw(
			`SELECT base_threshold FROM threshold_policies WHERE intent_type = ? AND is_active = TRUE`,
			tc.intent,
		).Scan(&base).Error
		if err != nil {
			t.Errorf("查询 intent=%s 策略失败: %v", tc.intent, err)
			continue
		}
		if !approxEqualFloat(base, tc.base) {
			t.Errorf("intent=%s base=%v want %v", tc.intent, base, tc.base)
		}
	}

	// 验证公共字段
	var cw, tw, aw float64
	var bhu, bfu, bru float64
	var slaSec int
	_ = db.Raw(`
		SELECT customer_level_weight, timeslot_weight, agent_availability_weight,
		       band_handoff_upper, band_fallback_upper, band_review_upper,
		       review_sla_seconds
		FROM threshold_policies WHERE intent_type = 'default'
	`).Row().Scan(&cw, &tw, &aw, &bhu, &bfu, &bru, &slaSec)
	if !approxEqualFloat(cw, 0.05) {
		t.Errorf("customer_level_weight=0.05, got %v", cw)
	}
	if !approxEqualFloat(tw, 0.05) {
		t.Errorf("timeslot_weight=0.05, got %v", tw)
	}
	if !approxEqualFloat(aw, 0.10) {
		t.Errorf("agent_availability_weight=0.10, got %v", aw)
	}
	if !approxEqualFloat(bhu, 0.40) {
		t.Errorf("band_handoff_upper=0.40, got %v", bhu)
	}
	if !approxEqualFloat(bfu, 0.60) {
		t.Errorf("band_fallback_upper=0.60, got %v", bfu)
	}
	if !approxEqualFloat(bru, 0.75) {
		t.Errorf("band_review_upper=0.75, got %v", bru)
	}
	if slaSec != 30 {
		t.Errorf("review_sla_seconds=30, got %d", slaSec)
	}
}

// TestConfidenceMigration_Down 集成测试：Down 回滚 6 张表
func TestConfidenceMigration_Down(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}
	if err := m.Down(context.Background()); err != nil {
		t.Fatalf("Down() failed: %v", err)
	}

	// 应被删除的 6 张表
	deletedTables := []string{
		"confidence_signals",
		"handoff_decisions",
		"review_queue",
		"sla_monitors",
		"ab_tests",
		"ab_test_metrics",
	}
	for _, table := range deletedTables {
		var exists bool
		_ = db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = ?)`, table).Scan(&exists)
		if exists {
			t.Errorf("Down() 后表 %s 应被删除", table)
		}
	}
}

// TestConfidenceMigration_DownPreservesCalibrationsAndPolicies
// 集成测试：Down 后 confidence_calibrations / threshold_policies 应保留
func TestConfidenceMigration_DownPreservesCalibrationsAndPolicies(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}
	if err := m.Down(context.Background()); err != nil {
		t.Fatalf("Down() failed: %v", err)
	}

	// 这两张表应保留
	preservedTables := []string{
		"confidence_calibrations",
		"threshold_policies",
	}
	for _, table := range preservedTables {
		var exists bool
		_ = db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = ?)`, table).Scan(&exists)
		if !exists {
			t.Errorf("Down() 后表 %s 应保留", table)
		}
	}

	// threshold_policies 中的 11 条策略应仍存在
	var count int64
	_ = db.Raw(`SELECT COUNT(*) FROM threshold_policies`).Scan(&count)
	if count != 11 {
		t.Errorf("Down() 后 threshold_policies 应有 11 条, got %d", count)
	}
}

// TestConfidenceMigration_InsertConfidenceSignal 集成测试：confidence_signals 可写入
func TestConfidenceMigration_InsertConfidenceSignal(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO confidence_signals
			(signal_id, session_id, customer_id, message_id, intent_type,
			 intent_conf, intent_conf_calibrated, entity_comp, ctx_relev, rag_qual, llm_entropy,
			 aggregated_conf, veto_triggered, dynamic_threshold, decision_band, temperature)
		VALUES ($1, $2, $3, $4, $5, 0.85, 0.88, 1.0, 0.7, 0.6, 0.9, 0.82, '', 0.70, 'auto', 1.0)
	`, "sig-test-1", "sess-1", "cust-1", "msg-1", "ask_product").Error; err != nil {
		t.Errorf("插入 confidence_signals 失败: %v", err)
	}

	var count int64
	_ = db.Raw(`SELECT COUNT(*) FROM confidence_signals WHERE signal_id = 'sig-test-1'`).Scan(&count)
	if count != 1 {
		t.Errorf("插入后应可查询到 1 条, got %d", count)
	}
}

// TestConfidenceMigration_InsertHandoffDecision 集成测试：handoff_decisions 可写入
func TestConfidenceMigration_InsertHandoffDecision(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO handoff_decisions
			(decision_id, session_id, customer_id, signal_id, reason, reason_detail,
			 confidence, threshold, intent_type, customer_level, timeslot)
		VALUES ($1, $2, $3, $4, $5, $6, 0.30, 0.70, 'complaint', 'vip', 'peak')
	`, "dec-1", "sess-1", "cust-1", "sig-1", "veto_complaint", "conf=0.30").Error; err != nil {
		t.Errorf("插入 handoff_decisions 失败: %v", err)
	}
}

// TestConfidenceMigration_InsertReviewQueue 集成测试：review_queue 可写入
func TestConfidenceMigration_InsertReviewQueue(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO review_queue
			(item_id, session_id, customer_id, signal_id, draft_reply,
			 original_confidence, threshold, intent_type, status, sla_deadline)
		VALUES ($1, $2, $3, $4, $5, 0.65, 0.70, 'ask_product', 'pending', NOW() + INTERVAL '30 seconds')
	`, "item-1", "sess-1", "cust-1", "sig-1", "您好，这是草稿回复").Error; err != nil {
		t.Errorf("插入 review_queue 失败: %v", err)
	}
}

// TestConfidenceMigration_InsertCalibration 集成测试：confidence_calibrations 可写入
func TestConfidenceMigration_InsertCalibration(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO confidence_calibrations
			(calibration_id, signal_type, method, temperature,
			 ece_before, ece_after, nll_before, nll_after, sample_size,
			 fit_started_at, fit_finished_at, is_active)
		VALUES ($1, 'intent_conf', 'temperature_scaling', 1.5,
				0.15, 0.05, 0.50, 0.35, 1000,
				NOW() - INTERVAL '1 minute', NOW(), TRUE)
	`, "cal-1").Error; err != nil {
		t.Errorf("插入 confidence_calibrations 失败: %v", err)
	}
}

// TestConfidenceMigration_InsertSLAMonitor 集成测试：sla_monitors 可写入
func TestConfidenceMigration_InsertSLAMonitor(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO sla_monitors
			(monitor_id, bucket_minute, auto_reply_rate, handoff_rate, review_timeout_rate,
			 avg_assignment_seconds, post_handoff_accept_rate, ece, total_messages, alerts_triggered)
		VALUES ($1, NOW(), 0.65, 0.20, 0.05, 25.5, 0.70, 0.04, 100, '')
	`, "mon-1").Error; err != nil {
		t.Errorf("插入 sla_monitors 失败: %v", err)
	}
}

// TestConfidenceMigration_InsertABTest 集成测试：ab_tests + ab_test_metrics 可写入
func TestConfidenceMigration_InsertABTest(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	// ab_tests
	if err := db.Exec(`
		INSERT INTO ab_tests
			(test_id, test_name, description, status, traffic_split, metrics)
		VALUES ($1, $2, $3, 'running',
		        '{"control": 0.5, "treatment": 0.5}'::jsonb,
		        '["ctr", "conversion"]'::jsonb)
	`, "test-1", "测试1", "A/B 测试").Error; err != nil {
		t.Errorf("插入 ab_tests 失败: %v", err)
	}

	// ab_test_metrics
	if err := db.Exec(`
		INSERT INTO ab_test_metrics
			(test_id, group_name, metric_name, value)
		VALUES ($1, $2, $3, $4)
	`, "test-1", "control", "ctr", 0.123456).Error; err != nil {
		t.Errorf("插入 ab_test_metrics 失败: %v", err)
	}
}

// TestConfidenceMigration_IndexesCreated 集成测试：所有索引创建
func TestConfidenceMigration_IndexesCreated(t *testing.T) {
	db := setupConfidenceMigrationTestDB(t)

	m := NewConfidenceMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	expectedIndexes := []string{
		"idx_signals_session",
		"idx_signals_customer",
		"idx_signals_band",
		"idx_calibrations_active",
		"idx_handoff_session",
		"idx_handoff_agent",
		"idx_handoff_sla",
		"idx_review_status",
		"idx_review_session",
		"idx_policies_intent",
		"idx_sla_bucket",
		"idx_ab_status",
		"idx_abm_test",
	}
	for _, idx := range expectedIndexes {
		var exists bool
		err := db.Raw(`
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes WHERE indexname = ?
			)
		`, idx).Scan(&exists).Error
		if err != nil || !exists {
			t.Errorf("索引 %s 应存在: exists=%v err=%v", idx, exists, err)
		}
	}
}

// approxEqualFloat float 近似相等（迁移测试用，独立于 service 包）
func approxEqualFloat(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-6
}
