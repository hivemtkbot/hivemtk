package migrations

import (
	"context"
	"testing"
)

// TestSOPHeatmapIndexMigration_Version 验证元信息
func TestSOPHeatmapIndexMigration_Version(t *testing.T) {
	m := NewSOPHeatmapIndexMigration(nil)
	if m.Version() != "v3.27.1" {
		t.Errorf("Version()=%q want=v3.27.1", m.Version())
	}
	if m.Name() == "" {
		t.Error("Name() should not be empty")
	}
	if m.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

// TestSOPHeatmapIndexMigration_NilDB nil db 返回错误
func TestSOPHeatmapIndexMigration_NilDB(t *testing.T) {
	m := NewSOPHeatmapIndexMigration(nil)
	if err := m.Up(context.Background()); err == nil {
		t.Errorf("nil db Up() 应返回错误")
	}
}
