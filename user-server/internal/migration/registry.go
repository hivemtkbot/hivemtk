package migration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
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

// Register 注册迁移（版本冲突时返回错误）
func (r *MigrationRegistry) Register(migration Migration) error {
	v := migration.Version()
	if _, exists := r.migrations[v]; exists {
		return fmt.Errorf("duplicate migration version: %s (%s)", v, migration.Name())
	}
	r.migrations[v] = migration
	return nil
}

// Get 获取迁移
func (r *MigrationRegistry) Get(version string) (Migration, bool) {
	m, ok := r.migrations[version]
	return m, ok
}

// GetAll 获取所有迁移（按版本号排序）
func (r *MigrationRegistry) GetAll() []Migration {
	result := make([]Migration, 0, len(r.migrations))
	for _, m := range r.migrations {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		return compareVersions(result[i].Version(), result[j].Version()) < 0
	})
	return result
}

// GetPending 获取待执行的迁移（按版本号排序）
func (r *MigrationRegistry) GetPending(executedVersions []string) []Migration {
	execSet := make(map[string]bool, len(executedVersions))
	for _, v := range executedVersions {
		execSet[v] = true
	}
	var pending []Migration
	for _, m := range r.migrations {
		if !execSet[m.Version()] {
			pending = append(pending, m)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		return compareVersions(pending[i].Version(), pending[j].Version()) < 0
	})
	return pending
}

// Validate 验证迁移（版本唯一性）
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

// compareVersions 比较两个语义化版本号 (v1.2.3 < v1.2.4)
func compareVersions(a, b string) int {
	aParts := parseVersion(a)
	bParts := parseVersion(b)
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	if len(aParts) < len(bParts) {
		return -1
	}
	if len(aParts) > len(bParts) {
		return 1
	}
	return 0
}

// parseVersion 解析版本号字符串 "v1.2.3" -> [1, 2, 3]
func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			result = append(result, 0)
		} else {
			result = append(result, n)
		}
	}
	return result
}

