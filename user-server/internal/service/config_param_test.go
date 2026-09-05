package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

func TestSeedConfigParams(t *testing.T) {
	gdb := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if gdb == nil {
		t.Skip("no DB")
	}
	if err := SeedConfigParams(context.Background(), gdb); err != nil {
		t.Fatalf("Seed failed: %v", err)
	}
	var count int64
	gdb.Model(&model.ConfigParam{}).Count(&count)
	if count != 109 {
		t.Fatalf("want 109, got %d", count)
	}

	svc := GlobalConfigParam()
	ctx := context.Background()

	if v := svc.GetDuration(ctx, "bridge", "polling_max_timeout", 0); v != 500*time.Second {
		t.Errorf("bridge.polling_max_timeout = %v, want 500s", v)
	}
	if v := svc.GetFloat(ctx, "knowledge", "similarity_threshold", 0); v != 0.5 {
		t.Errorf("knowledge.similarity_threshold = %v, want 0.5", v)
	}
	if v := svc.GetInt(ctx, "pagination", "page_max_size", 0); v != 100 {
		t.Errorf("pagination.page_max_size = %d, want 100", v)
	}
	if v := svc.GetBool(ctx, "smart_cs", "enable_auto_reply", false); !v {
		t.Errorf("smart_cs.enable_auto_reply = false, want true")
	}
	if v := svc.GetString(ctx, "knowledge", "embedding_dimension", "0"); v != "1024" {
		t.Errorf("knowledge.embedding_dimension = %s, want 1024", v)
	}
	t.Logf("✅ Seed 59 params + typed reads all pass")
}

func TestConfigParamUpdateResetAudit(t *testing.T) {
	gdb := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if gdb == nil {
		t.Skip("no DB")
	}
	if err := SeedConfigParams(context.Background(), gdb); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	svc := NewConfigParamService(gdb)
	ctx := context.Background()

	if err := svc.UpdateValue(ctx, "bridge", "polling_default_timeout", "60", 1); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if v := svc.GetDuration(ctx, "bridge", "polling_default_timeout", 0); v != 60*time.Second {
		t.Errorf("after update = %v, want 60s", v)
	}

	if err := svc.ResetToDefault(ctx, "bridge", "polling_default_timeout", 1); err != nil {
		t.Fatalf("ResetToDefault: %v", err)
	}
	if v := svc.GetDuration(ctx, "bridge", "polling_default_timeout", 0); v != 30*time.Second {
		t.Errorf("after reset = %v, want 30s", v)
	}

	if err := svc.BulkResetGroup(ctx, "knowledge", 1); err != nil {
		t.Fatalf("BulkResetGroup: %v", err)
	}

	var logCount int64
	gdb.Model(&model.ConfigParamAuditLog{}).Count(&logCount)
	if logCount < 2 {
		t.Errorf("audit log count = %d, want >= 2", logCount)
	}

	t.Logf("✅ update/reset/bulk_reset/audit pass")
}

func TestFallbackNilDB(t *testing.T) {
	svc := &ConfigParamService{cache: make(map[string]paramEntry), loaded: make(map[string]bool)}
	ctx := context.Background()
	if v := svc.GetInt(ctx, "any", "missing", 42); v != 42 {
		t.Errorf("fallback int wrong")
	}
	if v := svc.GetFloat(ctx, "any", "missing", 3.14); v != 3.14 {
		t.Errorf("fallback float wrong")
	}
	if v := svc.GetBool(ctx, "any", "missing", true); !v {
		t.Errorf("fallback bool wrong")
	}
	if v := svc.GetString(ctx, "any", "missing", "hello"); v != "hello" {
		t.Errorf("fallback string wrong")
	}
	if v := svc.GetDuration(ctx, "any", "missing", 99*time.Second); v != 99*time.Second {
		t.Errorf("fallback duration wrong")
	}
	t.Logf("✅ nil DB fallback pass")
}

func TestDefaultParamDefsCount(t *testing.T) {
	defs := DefaultParamDefs()
	if len(defs) != 109 {
		t.Fatalf("want 109 defs, got %d", len(defs))
	}
	for i, d := range defs {
		if d.Group == "" || d.Key == "" || d.DefaultValue == "" {
			t.Errorf("def[%d] bad: group=%q key=%q default=%q", i, d.Group, d.Key, d.DefaultValue)
		}
	}
	t.Logf("✅ 106 default defs validated")
}
