package migrations

// self_learning_migration.go 对话驱动自我学习三位一体机制 v3.11.0
//
// 五层架构归属: L5 数据层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1)
// 私域独立部署: 无 merchant_id / tenant_id 多租户字段
//
// 本迁移创建 自我学习三位一体机制所需的 6 张新表 + 1 张现有表扩展字段：
//  新表 1. self_learning_logs       - 自我学习日志表（幂等保证 + 全链路追踪）
//  新表 2. asset_bundle_candidates  - 资产包候选表（销冠对话 → 候选 → A/B → 上线）
//  新表 3. asset_bundle_ab_tests    - 资产包 A/B 实验表（baseline vs candidate）
//  新表 4. self_learning_switch     - 三位一体统一开关（单例：manual/supervised/autonomous）
//  新表 5. self_supervision_signals - 自我监督信号表（5 维指标按小时分桶）
//  新表 6. self_correction_actions  - 自我矫正动作审计表（7 类修复策略）
//  扩展 1. knowledge_chunks         - 新增 6 个质量字段（quality_score/quality_label/
//                                      low_quality_hits/champion_hits/source_session_ids/last_reward_at）
//
// 幂等性: 所有 DDL 使用 IF NOT EXISTS / DO $$ BEGIN ... END $$ 模式，可重入
// 依赖: v1.x（knowledge_chunks 已创建）/ v3.0.0（feedback_signals 已创建）

import (
	"context"
	"fmt"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// SelfLearningMigration 对话驱动自我学习三位一体机制迁移 v3.11.0
type SelfLearningMigration struct {
	db *gorm.DB
}

// NewSelfLearningMigration 创建迁移实例
func NewSelfLearningMigration(db *gorm.DB) *SelfLearningMigration {
	return &SelfLearningMigration{db: db}
}

// Version 返回版本号
func (m *SelfLearningMigration) Version() string { return "v3.11.0" }

// Name 返回迁移名称
func (m *SelfLearningMigration) Name() string {
	return "对话驱动自我学习三位一体机制（3 张新表 + 1 张表扩展）"
}

// Description 返回迁移描述
func (m *SelfLearningMigration) Description() string {
	return "创建 self_learning_logs/asset_bundle_candidates/asset_bundle_ab_tests 3 张表，并扩展 knowledge_chunks 表 6 个质量字段"
}

// Up 执行升级
func (m *SelfLearningMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 1. self_learning_logs 表
	if err := m.createSelfLearningLogs(ctx); err != nil {
		return fmt.Errorf("create self_learning_logs 失败: %w", err)
	}

	// 2. asset_bundle_candidates 表
	if err := m.createAssetBundleCandidates(ctx); err != nil {
		return fmt.Errorf("create asset_bundle_candidates 失败: %w", err)
	}

	// 3. asset_bundle_ab_tests 表
	if err := m.createAssetBundleABTests(ctx); err != nil {
		return fmt.Errorf("create asset_bundle_ab_tests 失败: %w", err)
	}

	// 4. 扩展 knowledge_chunks 表：6 个质量字段
	if err := m.alterKnowledgeChunks(ctx); err != nil {
		return fmt.Errorf("alter knowledge_chunks 失败: %w", err)
	}

	// 5. self_learning_switch 表（三位一体统一开关）
	if err := m.createSelfLearningSwitch(ctx); err != nil {
		return fmt.Errorf("create self_learning_switch 失败: %w", err)
	}

	// 6. self_supervision_signals 表（5 维监督指标）
	if err := m.createSelfSupervisionSignals(ctx); err != nil {
		return fmt.Errorf("create self_supervision_signals 失败: %w", err)
	}

	// 7. self_correction_actions 表（7 类矫正动作审计）
	if err := m.createSelfCorrectionActions(ctx); err != nil {
		return fmt.Errorf("create self_correction_actions 失败: %w", err)
	}

	return nil
}

// createSelfLearningLogs 创建 self_learning_logs 表
//
// 设计依据：v1.1 §2.3.1
// 用途：每次自我学习动作落库，UNIQUE(session_id, scenario) 保证幂等
func (m *SelfLearningMigration) createSelfLearningLogs(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS self_learning_logs (
			id              BIGSERIAL PRIMARY KEY,
			log_id          VARCHAR(64) NOT NULL UNIQUE,
			session_id      VARCHAR(64) NOT NULL,
			trace_id        VARCHAR(64) NOT NULL DEFAULT '',
			scenario        VARCHAR(32) NOT NULL,
			trigger_event   VARCHAR(32) NOT NULL,
			status          VARCHAR(16) NOT NULL DEFAULT 'pending',
			input_summary   JSONB DEFAULT '{}',
			output_summary  JSONB DEFAULT '{}',
			error_msg       TEXT DEFAULT '',
			duration_ms     INT DEFAULT 0,
			started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			finished_at     TIMESTAMPTZ,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(session_id, scenario)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_self_learning_logs_status ON self_learning_logs(status, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_self_learning_logs_scenario ON self_learning_logs(scenario, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_self_learning_logs_trace ON self_learning_logs(trace_id)`,
	}
	return execAllSelfLearning(ctx, m.db, stmts)
}

// createAssetBundleCandidates 创建 asset_bundle_candidates 表
//
// 设计依据：v1.1 §2.3.2
// 用途：销冠对话 → ChampionAnalyzer 提取话术 → 打包为候选 → A/B 测试 → 上线
func (m *SelfLearningMigration) createAssetBundleCandidates(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS asset_bundle_candidates (
			id                  BIGSERIAL PRIMARY KEY,
			candidate_id        VARCHAR(64) NOT NULL UNIQUE,
			source_session_ids  TEXT[] NOT NULL DEFAULT '{}',
			extracted_scripts   JSONB NOT NULL DEFAULT '[]',
			proposed_messages   JSONB NOT NULL DEFAULT '[]',
			industry            VARCHAR(32) DEFAULT '',
			language            VARCHAR(8) NOT NULL DEFAULT 'zh',
			scenario            VARCHAR(32) DEFAULT '',
			cluster_count       INT NOT NULL DEFAULT 0,
			reward_sum          DECIMAL(10,3) NOT NULL DEFAULT 0,
			status              VARCHAR(16) NOT NULL DEFAULT 'candidate',
			ab_test_id          VARCHAR(64) DEFAULT '',
			promoted_asset_id   VARCHAR(64) DEFAULT '',
			created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_bundle_candidates_status ON asset_bundle_candidates(status, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_bundle_candidates_scenario ON asset_bundle_candidates(scenario, status)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_bundle_candidates_ab_test ON asset_bundle_candidates(ab_test_id)`,
	}
	return execAllSelfLearning(ctx, m.db, stmts)
}

// createAssetBundleABTests 创建 asset_bundle_ab_tests 表
//
// 设计依据：v1.1 §2.3.3
// 用途：现役资产包 vs 候选资产包的流量对比实验
func (m *SelfLearningMigration) createAssetBundleABTests(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS asset_bundle_ab_tests (
			id                BIGSERIAL PRIMARY KEY,
			experiment_id     VARCHAR(64) NOT NULL UNIQUE,
			baseline_asset_id VARCHAR(64) NOT NULL,
			candidate_id      VARCHAR(64) NOT NULL,
			scenario          VARCHAR(32) NOT NULL DEFAULT '',
			traffic_split     JSONB NOT NULL DEFAULT '{"baseline":0.5,"candidate":0.5}',
			status            VARCHAR(16) NOT NULL DEFAULT 'running',
			winner_arm        VARCHAR(16) DEFAULT '',
			baseline_samples  INT NOT NULL DEFAULT 0,
			candidate_samples INT NOT NULL DEFAULT 0,
			baseline_reward   DECIMAL(12,3) NOT NULL DEFAULT 0,
			candidate_reward  DECIMAL(12,3) NOT NULL DEFAULT 0,
			started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			converged_at      TIMESTAMPTZ,
			completed_at      TIMESTAMPTZ,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_bundle_ab_tests_status ON asset_bundle_ab_tests(status, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_bundle_ab_tests_scenario ON asset_bundle_ab_tests(scenario, status)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_bundle_ab_tests_baseline ON asset_bundle_ab_tests(baseline_asset_id)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_bundle_ab_tests_candidate ON asset_bundle_ab_tests(candidate_id)`,
	}
	return execAllSelfLearning(ctx, m.db, stmts)
}

// alterKnowledgeChunks 扩展 knowledge_chunks 表：6 个质量字段
//
// 设计依据：v1.1 §2.3.4（适配项目实际表名 knowledge_chunks 而非 knowledge_corpus）
// 用途：RAG 自我矫正的核心载体——记录语料质量分、低质/销冠命中次数、来源会话
func (m *SelfLearningMigration) alterKnowledgeChunks(ctx context.Context) error {
	stmt := `DO $$
BEGIN
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'knowledge_chunks' AND column_name = 'quality_score') THEN
		ALTER TABLE knowledge_chunks ADD COLUMN quality_score DECIMAL(8,3) NOT NULL DEFAULT 0;
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'knowledge_chunks' AND column_name = 'quality_label') THEN
		ALTER TABLE knowledge_chunks ADD COLUMN quality_label VARCHAR(16) NOT NULL DEFAULT 'normal';
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'knowledge_chunks' AND column_name = 'low_quality_hits') THEN
		ALTER TABLE knowledge_chunks ADD COLUMN low_quality_hits INT NOT NULL DEFAULT 0;
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'knowledge_chunks' AND column_name = 'champion_hits') THEN
		ALTER TABLE knowledge_chunks ADD COLUMN champion_hits INT NOT NULL DEFAULT 0;
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'knowledge_chunks' AND column_name = 'source_session_ids') THEN
		ALTER TABLE knowledge_chunks ADD COLUMN source_session_ids TEXT[] NOT NULL DEFAULT '{}';
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'knowledge_chunks' AND column_name = 'last_reward_at') THEN
		ALTER TABLE knowledge_chunks ADD COLUMN last_reward_at TIMESTAMPTZ;
	END IF;
END $$`
	stmts := []string{
		stmt,
		// 质量标签 + 奖励时间索引，用于自我矫正扫描
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_quality ON knowledge_chunks(quality_label, low_quality_hits DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_champion ON knowledge_chunks(quality_label, champion_hits DESC) WHERE quality_label = 'champion'`,
	}
	return execAllSelfLearning(ctx, m.db, stmts)
}

// createSelfLearningSwitch 创建 self_learning_switch 表（三位一体统一开关单例）
//
// 设计依据：v1.1 §7.4 用户开启即全自动执行
// 单例：固定 id=1 的单行数据，由 service 层 GetOrCreate 初始化
// 三级自治：manual / supervised / autonomous
func (m *SelfLearningMigration) createSelfLearningSwitch(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS self_learning_switch (
			id                          BIGSERIAL PRIMARY KEY,
			autonomy_level              VARCHAR(16) NOT NULL DEFAULT 'manual',
			enable_rag                  BOOLEAN NOT NULL DEFAULT FALSE,
			enable_asset                BOOLEAN NOT NULL DEFAULT FALSE,
			enable_llm                  BOOLEAN NOT NULL DEFAULT FALSE,
			max_daily_corrections       INT NOT NULL DEFAULT 100,
			max_daily_promotions        INT NOT NULL DEFAULT 5,
			low_quality_threshold       DECIMAL(8,3) NOT NULL DEFAULT 3.0,
			champion_reward_threshold   DECIMAL(8,3) NOT NULL DEFAULT 1.5,
			ab_test_min_samples         INT NOT NULL DEFAULT 100,
			circuit_breaker_threshold   DECIMAL(5,3) NOT NULL DEFAULT 0.300,
			circuit_breaker_window_min  INT NOT NULL DEFAULT 30,
			circuit_open                BOOLEAN NOT NULL DEFAULT FALSE,
			today_corrections           INT NOT NULL DEFAULT 0,
			today_promotions            INT NOT NULL DEFAULT 0,
			today_reset_at              TIMESTAMPTZ,
			last_triggered_at           TIMESTAMPTZ,
			updated_by                  BIGINT NOT NULL DEFAULT 0,
			created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		// 单例约束：只允许 id=1 的一行
		`INSERT INTO self_learning_switch (id, autonomy_level) VALUES (1, 'manual') ON CONFLICT DO NOTHING`,
	}
	return execAllSelfLearning(ctx, m.db, stmts)
}

// createSelfSupervisionSignals 创建 self_supervision_signals 表
//
// 设计依据：v1.1 §7.2 5 维监督指标
// 按 (target_type, metric_name, bucket_hour) 聚合
func (m *SelfLearningMigration) createSelfSupervisionSignals(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS self_supervision_signals (
			id            BIGSERIAL PRIMARY KEY,
			signal_id     VARCHAR(64) NOT NULL UNIQUE,
			target_type   VARCHAR(16) NOT NULL,
			target_id     VARCHAR(64) NOT NULL DEFAULT '',
			metric_name   VARCHAR(32) NOT NULL,
			bucket_hour   TIMESTAMPTZ NOT NULL,
			value         DECIMAL(10,4) NOT NULL DEFAULT 0,
			baseline      DECIMAL(10,4) NOT NULL DEFAULT 0,
			threshold     DECIMAL(10,4) NOT NULL DEFAULT 0,
			sample_count  BIGINT NOT NULL DEFAULT 0,
			status        VARCHAR(16) NOT NULL DEFAULT 'normal',
			trace_ids     TEXT[] NOT NULL DEFAULT '{}',
			detail        JSONB DEFAULT '{}',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_self_supervision_signals_target_metric ON self_supervision_signals(target_type, metric_name, bucket_hour)`,
		`CREATE INDEX IF NOT EXISTS idx_self_supervision_signals_target_id ON self_supervision_signals(target_id, bucket_hour)`,
		`CREATE INDEX IF NOT EXISTS idx_self_supervision_signals_status ON self_supervision_signals(status, bucket_hour) WHERE status != 'normal'`,
		// unique 约束：同一 target+metric+bucket 仅一条
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_self_supervision_signals_bucket ON self_supervision_signals(target_type, target_id, metric_name, bucket_hour)`,
	}
	return execAllSelfLearning(ctx, m.db, stmts)
}

// createSelfCorrectionActions 创建 self_correction_actions 表（7 类矫正动作审计）
//
// 设计依据：v1.1 §7.3 失败矩阵驱动的 7 类修复策略
// 所有矫正动作落库审计，supervised 模式下需人工确认，
// autonomous 模式下自动执行（仍记录供回滚）
func (m *SelfLearningMigration) createSelfCorrectionActions(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS self_correction_actions (
			id             BIGSERIAL PRIMARY KEY,
			action_id      VARCHAR(64) NOT NULL UNIQUE,
			trigger_log_id VARCHAR(64) NOT NULL DEFAULT '',
			action_type    VARCHAR(32) NOT NULL,
			scenario       VARCHAR(32) NOT NULL DEFAULT '',
			target_type    VARCHAR(16) NOT NULL DEFAULT '',
			target_id      VARCHAR(64) NOT NULL DEFAULT '',
			before         JSONB DEFAULT '{}',
			after          JSONB DEFAULT '{}',
			autonomy_level VARCHAR(16) NOT NULL DEFAULT 'manual',
			operator       VARCHAR(64) NOT NULL DEFAULT 'auto',
			operator_id    BIGINT NOT NULL DEFAULT 0,
			reason         TEXT DEFAULT '',
			status         VARCHAR(16) NOT NULL DEFAULT 'pending',
			applied_at     TIMESTAMPTZ,
			rolled_back_at TIMESTAMPTZ,
			error_msg      TEXT DEFAULT '',
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_self_correction_actions_action_type ON self_correction_actions(action_type, status, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_self_correction_actions_target ON self_correction_actions(target_type, target_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_self_correction_actions_trigger ON self_correction_actions(trigger_log_id)`,
		`CREATE INDEX IF NOT EXISTS idx_self_correction_actions_status ON self_correction_actions(status, created_at)`,
	}
	return execAllSelfLearning(ctx, m.db, stmts)
}

// Down 回滚（删除新表 + 扩展字段）
//
// 注意：
//   - 6 张新表可安全删除
//   - knowledge_chunks 的扩展字段删除（不丢业务数据，仅删除质量元信息）
func (m *SelfLearningMigration) Down(ctx context.Context) error {
	stmts := []string{
		`DROP TABLE IF EXISTS self_correction_actions`,
		`DROP TABLE IF EXISTS self_supervision_signals`,
		`DROP TABLE IF EXISTS self_learning_switch`,
		`DROP INDEX IF EXISTS idx_knowledge_chunks_champion`,
		`DROP INDEX IF EXISTS idx_knowledge_chunks_quality`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS last_reward_at`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS source_session_ids`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS champion_hits`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS low_quality_hits`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS quality_label`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS quality_score`,
		`DROP TABLE IF EXISTS asset_bundle_ab_tests`,
		`DROP TABLE IF EXISTS asset_bundle_candidates`,
		`DROP TABLE IF EXISTS self_learning_logs`,
	}
	return execAllSelfLearning(ctx, m.db, stmts)
}

// execAllSelfLearning 批量执行 SQL（出错即返回）
//
// 命名区别于 execAllFeedbackLoop，避免冲突
func execAllSelfLearning(ctx context.Context, db *gorm.DB, stmts []string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	for _, sql := range stmts {
		if err := db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("exec failed (%s): %w", sql, err)
		}
	}
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*SelfLearningMigration)(nil)
