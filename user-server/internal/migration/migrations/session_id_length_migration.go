package migrations


import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// SessionIDLengthMigration 扩展 customer_sessions 及关联表 session_id 字段长度
// 50 → 120，修复小红书等长渠道名 stable_id 超长导致 SQLSTATE 22001
type SessionIDLengthMigration struct {
	db *gorm.DB
}

// NewSessionIDLengthMigration 创建迁移实例
func NewSessionIDLengthMigration(db *gorm.DB) *SessionIDLengthMigration {
	return &SessionIDLengthMigration{db: db}
}

// Version 返回版本号
func (m *SessionIDLengthMigration) Version() string { return "v3.22.4" }

// Name 返回迁移名称
func (m *SessionIDLengthMigration) Name() string {
	return "session_id 长度扩展至 120"
}

// Description 返回迁移描述
func (m *SessionIDLengthMigration) Description() string {
	return "扩展 customer_sessions 及关联表 session_id varchar(50) → varchar(120)，修复小红书 stable_id 超长 SQLSTATE 22001"
}

// Up 执行升级
func (m *SessionIDLengthMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	tables := []string{
		"customer_sessions",
		"session_messages",
		"feedback_loop",
		"feedback_learning",
		"ai_suggestion",
		"intent_log",
		"user_blacklist",
		"layer_decision_log",
		"sla",
		"ai_sales_champion",
	}
	for _, tbl := range tables {
		if !m.db.Migrator().HasTable(tbl) {
			continue
		}
		if !m.db.Migrator().HasColumn(tbl, "session_id") {
			continue
		}
		sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN session_id TYPE VARCHAR(120)", tbl)
		if err := m.db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("扩展 %s.session_id 失败: %w", tbl, err)
		}
	}
	return nil
}

// Down 执行降级（不收缩列，避免数据丢失）
func (m *SessionIDLengthMigration) Down(ctx context.Context) error {
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*SessionIDLengthMigration)(nil)