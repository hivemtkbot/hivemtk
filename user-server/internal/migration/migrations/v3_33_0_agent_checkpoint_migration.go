package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// AgentCheckpointMigration D06：agent_checkpoints 表（Agent Runtime 五阶段断点续跑）
//
// LangGraph Checkpointer 三表设计的单表简化（阶段级粒度足够）：
// 每 stage 完成即 upsert 一行 (thread_id, stage, state)；恢复按 updated_at 最新行续跑。
// 恢复纪律：只能从阶段边界恢复（阶段内失败整体重跑该阶段），阶段幂等。
type AgentCheckpointMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*AgentCheckpointMigration)(nil)

func NewAgentCheckpointMigration(db *gorm.DB) *AgentCheckpointMigration {
	return &AgentCheckpointMigration{db: db}
}

func (m *AgentCheckpointMigration) Version() string { return "v3.33.0" }

func (m *AgentCheckpointMigration) Name() string {
	return "agent_checkpoints 表（五阶段断点续跑）"
}

func (m *AgentCheckpointMigration) Description() string {
	return "D06: thread_id+stage+state JSONB，阶段边界 checkpoint 与恢复"
}

func (m *AgentCheckpointMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if err := m.db.WithContext(ctx).Exec(`CREATE TABLE IF NOT EXISTS agent_checkpoints (
		id BIGSERIAL PRIMARY KEY,
		thread_id VARCHAR(120) NOT NULL,
		stage VARCHAR(40) NOT NULL,
		state JSONB NOT NULL DEFAULT '{}',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT uk_agent_ckpt UNIQUE (thread_id, stage)
	)`).Error; err != nil {
		return err
	}
	return m.db.WithContext(ctx).Exec(
		`CREATE INDEX IF NOT EXISTS idx_agent_ckpt_thread ON agent_checkpoints(thread_id, updated_at DESC)`).Error
}

func (m *AgentCheckpointMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	return m.db.WithContext(ctx).Exec(`DROP TABLE IF EXISTS agent_checkpoints`).Error
}
