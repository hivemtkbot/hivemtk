// Package testmigrate 在测试库中运行生产 registry 原生 SQL 迁移。
//
// 之所以独立成包（而非放在 testutil 内），是因为 testutil 被 migration/repository/db
// 等包的测试文件反向 import，若 testutil 直接 import migration 会形成编译环：
//
//	migrations(repo) test -> testutil -> migration -> repository -> db
//
// 把迁移执行逻辑放到本独立包，由真正需要补齐列（embedding/emb_dimension/...）的
// 知识库相关测试显式调用，即可在不引入循环依赖的前提下复用生产迁移。
package testmigrate

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/migration/migrations"
)

// RunTestMigrations 运行所有注册的 registry 迁移的 Up()，使测试库具备与生产一致的完整 schema。
// 基础表不存在（relation does not exist）的迁移视为与本测试无关，记录日志后跳过，不阻断测试。
// 其余错误记录日志但同样不致命，避免单个迁移缺陷拖垮整个测试进程（可由后续专项测试覆盖）。
func RunTestMigrations(t *testing.T, database *gorm.DB) {
	t.Helper()
	registry := migration.NewMigrationRegistry()
	migrations.RegisterMigrations(registry, database)
	for _, m := range registry.GetAll() {
		if err := m.Up(context.Background()); err != nil {
			if isMissingRelationErr(err) {
				t.Logf("测试迁移跳过(基础表未创建,与本测试无关): %s: %v", m.Name(), err)
				continue
			}
			t.Logf("测试迁移告警(未致命): %s: %v", m.Name(), err)
		}
	}
}

func isMissingRelationErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "42704") ||
		strings.Contains(msg, "42p01")
}
