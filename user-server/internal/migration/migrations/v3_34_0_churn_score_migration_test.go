package migrations

import (
	"context"
	"testing"

	"hivemtk-user/internal/pkg/testutil"
)

// TestChurnScoreMigration_Version 元信息
func TestChurnScoreMigration_Version(t *testing.T) {
	m := NewChurnScoreMigration(nil)
	if m.Version() != "v3.34.0" {
		t.Errorf("Version()=%q want=v3.34.0", m.Version())
	}
	if m.Name() == "" || m.Description() == "" {
		t.Error("Name/Description should not be empty")
	}
}

// TestChurnScoreMigration_NilDB nil db 返回错误
func TestChurnScoreMigration_NilDB(t *testing.T) {
	m := NewChurnScoreMigration(nil)
	if err := m.Up(context.Background()); err == nil {
		t.Error("nil db Up() 应返回错误")
	}
	if err := m.Down(context.Background()); err == nil {
		t.Error("nil db Down() 应返回错误")
	}
}

// TestChurnScoreMigration_UpAndIdempotent 真 PG：建表 + 幂等重跑 + Down
func TestChurnScoreMigration_UpAndIdempotent(t *testing.T) {
	db := testutil.NewTestDB(t)
	m := NewChurnScoreMigration(db)
	ctx := context.Background()

	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}
	// 幂等
	if err := m.Up(ctx); err != nil {
		t.Fatalf("二次 Up() 应幂等: %v", err)
	}
	var exists bool
	if err := db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name='churn_scores')`).Scan(&exists).Error; err != nil || !exists {
		t.Fatalf("churn_scores 表应存在: %v", err)
	}
	// 唯一约束在位
	if err := db.Exec(`INSERT INTO churn_scores (customer_key, p_alive) VALUES ('k1', 0.5) ON CONFLICT (customer_key) DO NOTHING`).Error; err != nil {
		t.Fatalf("customer_key 唯一约束应可用: %v", err)
	}
	if err := m.Down(ctx); err != nil {
		t.Fatalf("Down() failed: %v", err)
	}
	// Down 幂等
	if err := m.Down(ctx); err != nil {
		t.Fatalf("二次 Down() 应幂等: %v", err)
	}
}
