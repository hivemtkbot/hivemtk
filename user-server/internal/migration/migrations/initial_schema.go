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
	registry.Register(NewMarketingFlowSchemaMigration(db))
	registry.Register(NewUnmultitenantSchemaMigration(db))
	// 私域部署:merchant_id 字段可空化(幂等,可重入)
	registry.Register(NewMerchantIDNullableMigration(db))
	// Phase 1: 企微 webhook 字段 + 智能体开关(打通用户全链路)
	registry.Register(NewWecomWebhookFieldsMigration(db))
	// 多 AI 智能体架构:ai_agents / channel_agent_bindings / customer_service_agents
	registry.Register(NewAIAgentSchemaMigration(db))
	// ai_agents 扩展 2 字段(决策策略 / A/B 实验),依据 ADR-008 §2.3
	registry.Register(NewAIAgentExtensionMigration(db))
	// RAG 向量存储(pgvector embedding + HNSW 索引)
	registry.Register(NewKnowledgeVectorMigration(db))
	// P0-1 SOP 节点执行器(sop_exec_events / sop_timers / sop_outbox)
	registry.Register(NewSOPExecutorMigration(db))
	// P0-2 RAG 混合检索(tsvector + query_rewrite_cache + embedding_cache + 监控字段)
	registry.Register(NewRagHybridMigration(db))
	// P0-3 置信度驱动转人工(7+1 张表：信号/校准/转人工/审核/策略/SLA/AB测试)
	registry.Register(NewConfidenceMigration(db))
	// P0-4 拟人度评估器(5 张表：评分/维度明细/销冠基线/销冠短语/AB统计)
	registry.Register(NewHumanizeEvaluatorMigration(db))
	// P0-5 反馈学习闭环(6 张表 + 2 张表扩展：反馈事件/信号/销冠对话/Prompt候选/Bandit臂/AB测试 + sop_agents/script_templates 字段扩展)
	registry.Register(NewFeedbackLoopMigration(db))
	// 金额字段使用 BIGINT（分）— decimal 金额字段升级为 bigint
	registry.Register(NewAmountMoneyMigration(db))
	// D+E 域合规（邮件+短信退订与追踪 6 张表）
	// 依据《互联网电子邮件服务管理办法》第十三条 + 《通信短消息服务管理规定》第十八条
	registry.Register(NewComplianceDEMigration(db))
	// H 域 - 线索评分 + RFM 联动 + 流失挽回(4 张表)
	registry.Register(NewHP1Migration(db))
	// L 域 - 第三方对接模板(integration_templates)
	registry.Register(NewLP1Migration(db))
	// P1-1/P1-2/P1-3/P1-4 认证与安全（user_mfa / login_events / security_alerts / password_history / system_config_kv）
	registry.Register(NewAuthSecurityMigration(db))
	// A 域 — team_users 表新增 data_scope / department_id / team_id / custom_dept_ids 4 字段，支持 P1-4 行级权限
	registry.Register(NewADomainP1Migration(db))
	// short_links 字段补齐 — title 等列缺失会导致短链创建 500
	registry.Register(NewShortLinkColumnsMigration(db))
	// M 域 — provider_health / system_kv_config / intent_logs / trace_events 4 张表
	registry.Register(NewMP1Migration(db))
	// ops AI 生产力统计依赖的 llm_usage_records 表
	registry.Register(NewLLMUsageRecordsMigration(db))
	// 方向9 资产包模式 — asset_bundles / asset_bundle_version_logs
	registry.Register(NewAssetBundleMigration(db))
	// LLM 路由可观测性 — llm_routing_logs / llm_routing_audit 两张表
	registry.Register(NewLLMRoutingLogsMigration(db))
	registry.Register(NewLLMRoutingLogsExtendMigration(db))
	// v1.2 出海多语言方案 - glossaries 表 + ai_agents/chat_channels/llm_routing_logs/knowledge_chunks/asset_bundles 多语言字段
	registry.Register(NewMultilingualI18nMigration(db))
	// v1.2 出海多语言方案 P1-3 - knowledge_chunks.translated_versions 字段（预翻译支持）
	registry.Register(NewMultilingualI18nP13Migration(db))
	// S3-6 Telegram polling 分布式锁（polling_owner + polling_heartbeat_at）
	registry.Register(NewTelegramPollingLockMigration(db))
	// 2026-07-31 AI 智能体性能优化 - FAQ / SOP 知识库 + Layer 决策日志（双层架构 Layer1 命中 SkipLLM）
	registry.Register(NewAIPerfFAQSOPLayerMigration(db))
	// 2026-07-31 AI 智能体知识库绑定 - faq_entry_ids / sop_template_ids 字段（与 rag_product_ids 一致）
	registry.Register(NewAIAgentKBBindingMigration(db))
	// 2026-07-31 P0-B 知识库统一 - knowledge_bases / agent_kb_bindings + 3 表 agent_id
	registry.Register(NewKBUnificationMigration(db))
	// 继续添加新的迁移...
}
