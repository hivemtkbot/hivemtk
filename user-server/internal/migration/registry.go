package migration

import (
	"context"
	"errors"
)

// Migration 迁移接口
type Migration interface {
	Version() string
	Name() string
	Description() string
	Up(ctx context.Context) error
	Down(ctx context.Context) error
}

// MigrationRegistry 迁移注册表
type MigrationRegistry struct {
	migrations map[string]Migration
}

// NewMigrationRegistry 创建迁移注册表
func NewMigrationRegistry() *MigrationRegistry {
	return &MigrationRegistry{
		migrations: make(map[string]Migration),
	}
}

// Register 注册迁移
func (r *MigrationRegistry) Register(migration Migration) {
	r.migrations[migration.Version()] = migration
}

// Get 获取迁移
func (r *MigrationRegistry) Get(version string) (Migration, bool) {
	m, ok := r.migrations[version]
	return m, ok
}

// GetAll 获取所有迁移
func (r *MigrationRegistry) GetAll() []Migration {
	result := make([]Migration, 0, len(r.migrations))
	for _, m := range r.migrations {
		result = append(result, m)
	}
	return result
}

// GetPending 获取待执行的迁移
func (r *MigrationRegistry) GetPending(executedVersions []string) []Migration {
	var pending []Migration
	for _, m := range r.migrations {
		executed := false
		for _, v := range executedVersions {
			if v == m.Version() {
				executed = true
				break
			}
		}
		if !executed {
			pending = append(pending, m)
		}
	}
	return pending
}

// Validate 验证迁移
func (r *MigrationRegistry) Validate() error {
	versions := make(map[string]bool)
	for version := range r.migrations {
		if versions[version] {
			return errors.New("duplicate migration version: " + version)
		}
		versions[version] = true
	}
	return nil
}

