package repository

// system_config_kv_repository.go 通用 KV 配置仓储
//
// 五层架构归属：L4 数据访问层
// 表：system_config_kv
//
// 用途：密码策略、UI 偏好等"以单条 JSON 形式存整体配置"的场景。
// 区别于 system_config（业务字段表），KV 表任意 key 由调用方自定义。

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
)

// SystemConfigKVRepository KV 配置仓储接口
type SystemConfigKVRepository interface {
	// Get 按 key 取单条 value
	// 未找到时返回 ("", nil)，由调用方决定是否走默认配置
	Get(ctx context.Context, key string) (string, error)

	// Upsert 不存在则插入、存在则更新 value
	// 返回写入的最终值（用于回显等场景）
	Upsert(ctx context.Context, key, value string) (string, error)

	// EnsureTable 表不存在时自动创建（与原有 system_config_kv 表结构一致）
	// 用于服务启动早期没有运行 migration 的场景（如离线脚本 / 单元测试）
	EnsureTable(ctx context.Context) error
}

type systemConfigKVRepo struct {
	db *gorm.DB
}

// NewSystemConfigKVRepository 构造
func NewSystemConfigKVRepository() SystemConfigKVRepository {
	return &systemConfigKVRepo{db: db.GetDB()}
}

// Get 按 key 查询
func (r *systemConfigKVRepo) Get(ctx context.Context, key string) (string, error) {
	var row model.SystemConfigKV
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return row.Value, nil
}

// Upsert 写入或更新
func (r *systemConfigKVRepo) Upsert(ctx context.Context, key, value string) (string, error) {
	now := time.Now()
	row := model.SystemConfigKV{
		Key:       key,
		Value:     value,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"value":      value,
			"updated_at": now,
		}),
	}).Create(&row).Error
	if err != nil {
		// 表可能不存在，触发 EnsureTable 后重试一次
		if ensureErr := r.EnsureTable(ctx); ensureErr != nil {
			return "", err
		}
		err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "key"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"value":      value,
				"updated_at": now,
			}),
		}).Create(&row).Error
		if err != nil {
			return "", err
		}
	}
	return value, nil
}

// EnsureTable 建表（IF NOT EXISTS）
func (r *systemConfigKVRepo) EnsureTable(ctx context.Context) error {
	stmt := `CREATE TABLE IF NOT EXISTS system_config_kv (
		key VARCHAR(100) PRIMARY KEY,
		value TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`
	return r.db.WithContext(ctx).Exec(stmt).Error
}

var _ SystemConfigKVRepository = (*systemConfigKVRepo)(nil)
