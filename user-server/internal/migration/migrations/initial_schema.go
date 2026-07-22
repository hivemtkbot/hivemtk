package migrations

import (
	"context"
	"marketing/internal/migration"

	"gorm.io/gorm"
)

// InitialSchemaMigration 初始版本迁移(原 V110Migration 重命名,语义化命名)
// 私域部署: 单租户,无 merchant_id 字段
type InitialSchemaMigration struct {
	db *gorm.DB
}

// NewInitialSchemaMigration 创建初始版本迁移
func NewInitialSchemaMigration(db *gorm.DB) *InitialSchemaMigration {
	return &InitialSchemaMigration{db: db}
}

// Version 返回版本号
func (m *InitialSchemaMigration) Version() string {
	return "v1.1.0"
}

// Name 返回迁移名称
func (m *InitialSchemaMigration) Name() string {
	return "初始版本迁移"
}

// Description 返回迁移描述
func (m *InitialSchemaMigration) Description() string {
	return "创建基础表结构"
}

// Up 执行升级
func (m *InitialSchemaMigration) Up(ctx context.Context) error {
	return nil
}

// Down 执行降级
func (m *InitialSchemaMigration) Down(ctx context.Context) error {
	return nil
}

// Ensure InitialSchemaMigration implements Migration interface
var _ migration.Migration = (*InitialSchemaMigration)(nil)

// TeamUserSchemaMigration 团队成员管理迁移(原 V120Migration 重命名)
type TeamUserSchemaMigration struct {
	db *gorm.DB
}

// NewTeamUserSchemaMigration 创建团队成员迁移
func NewTeamUserSchemaMigration(db *gorm.DB) *TeamUserSchemaMigration {
	return &TeamUserSchemaMigration{db: db}
}

// Version 返回版本号
func (m *TeamUserSchemaMigration) Version() string {
	return "v1.2.0"
}

// Name 返回迁移名称
func (m *TeamUserSchemaMigration) Name() string {
	return "团队成员管理"
}

// Description 返回迁移描述
func (m *TeamUserSchemaMigration) Description() string {
	return "添加团队成员、角色和权限表"
}

// Up 执行升级
func (m *TeamUserSchemaMigration) Up(ctx context.Context) error {
	// 创建团队成员表
	m.db.Exec(`
		CREATE TABLE IF NOT EXISTS team_users (
			id BIGSERIAL PRIMARY KEY,
			username VARCHAR(50) NOT NULL UNIQUE,
			password VARCHAR(255) NOT NULL,
			name VARCHAR(50),
			email VARCHAR(100) UNIQUE,
			phone VARCHAR(20),
			role VARCHAR(20) DEFAULT 'viewer',
			status INTEGER DEFAULT 1,
			last_login_at TIMESTAMP,
			last_login_ip VARCHAR(50),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)

	// 创建团队角色表
	m.db.Exec(`
		CREATE TABLE IF NOT EXISTS team_roles (
			id BIGSERIAL PRIMARY KEY,
			code VARCHAR(20) NOT NULL UNIQUE,
			name VARCHAR(50) NOT NULL,
			permissions TEXT,
			is_system BOOLEAN DEFAULT FALSE,
			status INTEGER DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)

	// 创建操作日志表
	m.db.Exec(`
		CREATE TABLE IF NOT EXISTS operation_logs (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL,
			username VARCHAR(50),
			action VARCHAR(20),
			module VARCHAR(50),
			resource VARCHAR(50),
			resource_id VARCHAR(50),
			detail TEXT,
			old_value TEXT,
			new_value TEXT,
			ip VARCHAR(50),
			user_agent VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)

	// 创建索引
	m.db.Exec("CREATE INDEX IF NOT EXISTS idx_operation_logs_user_time ON operation_logs(user_id, created_at)")
	m.db.Exec("CREATE INDEX IF NOT EXISTS idx_operation_logs_user ON operation_logs(user_id)")

	return nil
}

// Down 执行降级
func (m *TeamUserSchemaMigration) Down(ctx context.Context) error {
	m.db.Exec("DROP TABLE IF EXISTS operation_logs")
	m.db.Exec("DROP TABLE IF EXISTS team_roles")
	m.db.Exec("DROP TABLE IF EXISTS team_users")
	return nil
}

var _ migration.Migration = (*TeamUserSchemaMigration)(nil)

// MarketingFlowSchemaMigration 营销自动化迁移(原 V130Migration 重命名)
type MarketingFlowSchemaMigration struct {
	db *gorm.DB
}

// NewMarketingFlowSchemaMigration 创建营销流程迁移
func NewMarketingFlowSchemaMigration(db *gorm.DB) *MarketingFlowSchemaMigration {
	return &MarketingFlowSchemaMigration{db: db}
}

// Version 返回版本号
func (m *MarketingFlowSchemaMigration) Version() string {
	return "v1.3.0"
}

// Name 返回迁移名称
func (m *MarketingFlowSchemaMigration) Name() string {
	return "营销自动化"
}

// Description 返回迁移描述
func (m *MarketingFlowSchemaMigration) Description() string {
	return "添加营销流程、触发器、动作表"
}

// Up 执行升级
func (m *MarketingFlowSchemaMigration) Up(ctx context.Context) error {
	// 创建营销流程表
	m.db.Exec(`
		CREATE TABLE IF NOT EXISTS marketing_flows (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(100),
			description VARCHAR(500),
			status VARCHAR(20) DEFAULT 'draft',
			trigger_type VARCHAR(20),
			trigger_config TEXT,
			flow_data TEXT,
			version INTEGER DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)

	// 创建流程执行记录表
	m.db.Exec(`
		CREATE TABLE IF NOT EXISTS flow_executions (
			id BIGSERIAL PRIMARY KEY,
			flow_id BIGINT,
			trigger_id VARCHAR(50),
			user_id VARCHAR(50),
			status VARCHAR(20),
			current_node VARCHAR(50),
			execution_data TEXT,
			started_at TIMESTAMP,
			completed_at TIMESTAMP,
			error_message TEXT
		)
	`)

	// 创建索引
	m.db.Exec("CREATE INDEX IF NOT EXISTS idx_marketing_flows_status ON marketing_flows(status)")
	m.db.Exec("CREATE INDEX IF NOT EXISTS idx_flow_executions_flow ON flow_executions(flow_id)")
	m.db.Exec("CREATE INDEX IF NOT EXISTS idx_flow_executions_user ON flow_executions(user_id)")

	return nil
}

// Down 执行降级
func (m *MarketingFlowSchemaMigration) Down(ctx context.Context) error {
	m.db.Exec("DROP TABLE IF EXISTS flow_executions")
	m.db.Exec("DROP TABLE IF EXISTS marketing_flows")
	return nil
}

var _ migration.Migration = (*MarketingFlowSchemaMigration)(nil)

// RegisterMigrations 注册所有迁移
func RegisterMigrations(registry *migration.MigrationRegistry, db *gorm.DB) {
	registry.Register(NewInitialSchemaMigration(db))
	registry.Register(NewTeamUserSchemaMigration(db))
	registry.Register(NewMarketingFlowSchemaMigration(db))
	registry.Register(NewUnmultitenantSchemaMigration(db))
	// 私域部署:merchant_id 字段可空化(幂等,可重入)
	registry.Register(NewMerchantIDNullableMigration(db))
	// Phase 1: 企微 webhook 字段 + 智能体开关(打通用户全链路)
	registry.Register(NewWecomWebhookFieldsMigration(db))
	// 多 AI 智能体架构:ai_agents / channel_agent_bindings / customer_service_agents
	registry.Register(NewAIAgentSchemaMigration(db))
	// 2026-07-17:ai_agents 扩展 2 字段(决策策略 / A/B 实验),依据 ADR-008 §2.3
	registry.Register(NewAIAgentExtensionMigration(db))
	// 2026-07-18:RAG 向量存储(pgvector embedding + HNSW 索引)
	registry.Register(NewKnowledgeVectorMigration(db))
	// 2026-07-19:P0-1 SOP 节点执行器(sop_exec_events / sop_timers / sop_outbox)
	registry.Register(NewSOPExecutorMigration(db))
	// 2026-07-19:P0-2 RAG 混合检索(tsvector + query_rewrite_cache + embedding_cache + 监控字段)
	registry.Register(NewRagHybridMigration(db))
	// 2026-07-19:P0-3 置信度驱动转人工(7+1 张表：信号/校准/转人工/审核/策略/SLA/AB测试)
	registry.Register(NewConfidenceMigration(db))
	// 2026-07-19:P0-4 拟人度评估器(5 张表：评分/维度明细/销冠基线/销冠短语/AB统计)
	registry.Register(NewHumanizeEvaluatorMigration(db))
	// 2026-07-19:P0-5 反馈学习闭环(6 张表 + 2 张表扩展：反馈事件/信号/销冠对话/Prompt候选/Bandit臂/AB测试 + sop_agents/script_templates 字段扩展)
	registry.Register(NewFeedbackLoopMigration(db))
	// 2026-07-21:金额字段 BIGINT 重构（分）— 历史遗留 decimal 金额字段升级为 bigint
	registry.Register(NewAmountMoneyMigration(db))
	// 2026-07-21:D+E 域合规（邮件+短信退订与追踪 6 张表）
	// 依据《互联网电子邮件服务管理办法》第十三条 + 《通信短消息服务管理规定》第十八条
	registry.Register(NewComplianceDEMigration(db))
	// 2026-07-21:H 域 P1 缺口修复 - 线索评分 + RFM 联动 + 流失挽回(4 张表)
	registry.Register(NewHP1Migration(db))
	// 2026-07-21:L 域 P1 缺口修复 - 第三方对接模板(integration_templates)
	registry.Register(NewLP1Migration(db))
	// 2026-07-21:P1-1/P1-2/P1-3/P1-4 认证与安全（user_mfa / login_events / security_alerts / password_history / system_config_kv）
	registry.Register(NewAuthSecurityMigration(db))
	// 2026-07-21:A 域 P1 缺口修复 — team_users 表新增 data_scope / department_id / team_id / custom_dept_ids 4 字段，支持 P1-4 行级权限
	registry.Register(NewADomainP1Migration(db))
	// 2026-07-22:short_links 字段补齐 — 修复短链创建 500（column title does not exist）
	registry.Register(NewShortLinkColumnsMigration(db))
	// 2026-07-21:M 域 P1 缺口修复 — provider_health / system_kv_config / intent_logs / trace_events 4 张表
	registry.Register(NewMP1Migration(db))
	// 2026-07-22:ops AI 生产力统计依赖的 llm_usage_records 表（历史漏建）
	registry.Register(NewLLMUsageRecordsMigration(db))
	// 2026-07-22:方向9 资产包模式 — asset_bundles / asset_bundle_version_logs
	registry.Register(NewAssetBundleMigration(db))
	// 继续添加新的迁移...
}
