package migrations

import (
	"context"
	"hivemtk-user/internal/migration"

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
	registry.Register(NewMerchantIDNullableMigration(db))
	registry.Register(NewWecomWebhookFieldsMigration(db))
	registry.Register(NewAIAgentSchemaMigration(db))
	registry.Register(NewAIAgentExtensionMigration(db))
	registry.Register(NewKnowledgeVectorMigration(db))
	registry.Register(NewSOPExecutorMigration(db))
	registry.Register(NewRagHybridMigration(db))
	registry.Register(NewConfidenceMigration(db))
	registry.Register(NewHumanizeEvaluatorMigration(db))
	registry.Register(NewFeedbackLoopMigration(db))
	registry.Register(NewAmountMoneyMigration(db))
	registry.Register(NewComplianceDEMigration(db))
	registry.Register(NewHP1Migration(db))
	registry.Register(NewLP1Migration(db))
	registry.Register(NewAuthSecurityMigration(db))
	registry.Register(NewADomainP1Migration(db))
	registry.Register(NewShortLinkColumnsMigration(db))
	registry.Register(NewMP1Migration(db))
	registry.Register(NewLLMUsageRecordsMigration(db))
	registry.Register(NewAssetBundleMigration(db))
	registry.Register(NewLLMRoutingLogsMigration(db))
	registry.Register(NewLLMRoutingLogsExtendMigration(db))
	registry.Register(NewMultilingualI18nMigration(db))
	registry.Register(NewMultilingualI18nP13Migration(db))
	registry.Register(NewKnowledgeWeightMigration(db))
	registry.Register(NewTelegramPollingLockMigration(db))
	registry.Register(NewAIPerfFAQSOPLayerMigration(db))
	registry.Register(NewAIAgentKBBindingMigration(db))
	registry.Register(NewKBUnificationMigration(db))
	registry.Register(NewRagSafetyAuditDropMigration(db))
	registry.Register(NewRagAlertsDropMigration(db))
	registry.Register(NewBridgeAccountMigration(db))
	registry.Register(NewBridgeChannelNormalizeMigration(db))
	registry.Register(NewBridgeChannelUnifyV2Migration(db))
	registry.Register(NewBridgeChannelUnifyV3_18_1Migration(db))
	registry.Register(NewCustomerSessionUpdatedAtMigration(db))
	registry.Register(NewKnowledgeSearchLogProductOptionalMigration(db))
	registry.Register(NewDropCdpAutoReplyMigration(db))
	registry.Register(NewLiveCodeClickLogsMigration(db))
}

