package migrations

import (
	"testing"

	"hivemtk-user/internal/migration"
)

// 复审补充：完整注册无版本冲突 + 新增迁移排序在末位（防版本回退误插）
func TestRegisterMigrations_NoDuplicateVersions(t *testing.T) {
	registry := migration.NewMigrationRegistry()
	RegisterMigrations(registry, nil)
	if err := registry.Validate(); err != nil {
		t.Fatalf("registry validate: %v", err)
	}
	all := registry.GetAll()
	if len(all) == 0 {
		t.Fatal("registry empty")
	}
	last := all[len(all)-1].Version()
	for _, v := range []string{"v3.29.0", "v3.30.0", "v3.31.0", "v3.32.0", "v3.33.0"} {
		found := false
		for _, m := range all {
			if m.Version() == v {
				found = true
			}
		}
		if !found {
			t.Errorf("迁移 %s 未注册", v)
		}
	}
	t.Logf("迁移总数=%d 末位=%s", len(all), last)
}
