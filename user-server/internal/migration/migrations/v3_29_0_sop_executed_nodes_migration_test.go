package migrations

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// TestSOPExecutedNodesMigration_Version 验证元信息
func TestSOPExecutedNodesMigration_Version(t *testing.T) {
	m := NewSOPExecutedNodesMigration(nil)
	if m.Version() != "v3.29.0" {
		t.Errorf("Version()=%q want=v3.29.0", m.Version())
	}
	if m.Name() == "" {
		t.Error("Name() should not be empty")
	}
	if m.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

// TestSOPExecutedNodesMigration_NilDB nil db 返回错误
func TestSOPExecutedNodesMigration_NilDB(t *testing.T) {
	m := NewSOPExecutedNodesMigration(nil)
	if err := m.Up(context.Background()); err == nil {
		t.Errorf("nil db Up() 应返回错误")
	}
	if err := m.Down(context.Background()); err == nil {
		t.Errorf("nil db Down() 应返回错误")
	}
}

// TestSOPExecutedNodesMigration_UpAndIdempotent 真实 DB：Up 后列存在且 Save 全字段写可用，重复执行幂等
// 注意：需真实 PG（TEST_DATABASE_URL 或 POSTGRES_*，默认 127.0.0.1:8202），不可达时跳过。
func TestSOPExecutedNodesMigration_UpAndIdempotent(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SOPExecution{})

	m := NewSOPExecutedNodesMigration(db)

	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}

	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up() idempotent failed: %v", err)
	}

	exec := &model.SOPExecution{
		SOPID:         1,
		CustomerID:    "c1",
		SessionID:     "s1",
		Status:        "running",
		ExecutedNodes: model.JSONArray{map[string]any{"node_id": "n1", "status": "completed", "attempt": 1}},
	}
	if err := db.Create(exec).Error; err != nil {
		t.Fatalf("Create with ExecutedNodes failed: %v", err)
	}

	var got model.SOPExecution
	if err := db.First(&got, exec.ID).Error; err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if len(got.ExecutedNodes) != 1 {
		t.Fatalf("ExecutedNodes round-trip mismatch: %+v", got.ExecutedNodes)
	}
	first, ok := got.ExecutedNodes[0].(map[string]any)
	if !ok || first["node_id"] != "n1" {
		t.Errorf("ExecutedNodes[0] mismatch: %T %+v", got.ExecutedNodes[0], got.ExecutedNodes[0])
	}

	if err := m.Down(context.Background()); err != nil {
		t.Fatalf("Down() failed: %v", err)
	}
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("re-Up() after Down failed: %v", err)
	}
}
