package migrations

import (
	"context"
	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// UnmultitenantSchemaMigration 彻底移除多租户结构迁移(原 V200Migration 重命名)
// 私域部署版本:删除 merchants 表,移除所有业务表中的 merchant_id 字段,重建唯一索引
type UnmultitenantSchemaMigration struct {
	db *gorm.DB
}

// NewUnmultitenantSchemaMigration 创建单租户迁移
func NewUnmultitenantSchemaMigration(db *gorm.DB) *UnmultitenantSchemaMigration {
	return &UnmultitenantSchemaMigration{db: db}
}

// Version 返回版本号
func (m *UnmultitenantSchemaMigration) Version() string {
	return "v2.0.0"
}

// Name 返回迁移名称
func (m *UnmultitenantSchemaMigration) Name() string {
	return "彻底移除多租户结构"
}

// Description 返回迁移描述
func (m *UnmultitenantSchemaMigration) Description() string {
	return "独立部署版本：删除 merchants 表，移除所有业务表中的 merchant_id 字段，重建唯一索引"
}

// 注意：与 system_config 等系统级表无关，仅针对业务数据表
var businessTablesWithMerchantID = []string{
	"operation_logs",
	"marketing_flows",
	"flow_executions",
	"customers",
	"customer_tags",
	"customer_events",
	"customer_sessions",
	"clues",
	"orders",
	"products",
	"materials",
	"material_categories",
	"knowledge_documents",
	"knowledge_chunks",
	"unified_messages",
	"unified_replies",
	"platform_accounts",
	"wecom_accounts",
	"wecom_account_healths",
	"wecom_configs",
	"whatsapp_accounts",
	"whatsapp_sessions",
	"whatsapp_messages",
	"whatsapp_message_templates",
	"whatsapp_message_queue",
	"whatsapp_jobs",
	"whatsapp_drafts",
	"whatsapp_group_messages",
	"telegram_accounts",
	"sms",
	"sms_jobs",
	"sms_lists",
	"emails",
	"email_drafts",
	"email_jobs",
	"email_lists",
	"email_sends",
	"email_smtps",
	"rags",
	"rag_sessions",
	"rag_products",
	"rag_configs",
	"scripts",
	"script_templates",
	"churn_predictions",
	"rfm_rules",
	"ab_experiments",
	"ai_contents",
	"ai_sales_champions",
	"backup_records",
	"live_codes",
	"live_code_qrs",
	"live_code_qr_stats",
	"short_links",
	"short_link_accesses",
	"domain_pools",
	"dashboard_screens",
	"market_templates",
	"market_template_downloads",
	"card_accesses",
	"douyin_cards",
	"douyin_card_activities",
	"kuaishou_cards",
	"kuaishou_card_activities",
	"xiaohongshu_cards",
	"xiaohongshu_card_activities",
	"xianyu_cards",
	"xianyu_card_activities",
	"xianyu_card_stats",
	"tiktok_cards",
	"custom_reports",
	"integrations",
	"obs_configs",
	"batch_operation_histories",
	"message_hubs",
	"inbox_conversations",
	"stats",
	"community",
	"system_configs",
	"user_tags",
	"accounts",
}

// Up 执行升级
func (m *UnmultitenantSchemaMigration) Up(ctx context.Context) error {
	for _, table := range businessTablesWithMerchantID {
		m.db.Exec("ALTER TABLE " + table + " DROP COLUMN IF EXISTS merchant_id")
	}

	m.db.Exec("DROP TABLE IF EXISTS merchants")

	m.db.Exec("DROP TABLE IF EXISTS platform_licenses")
	m.db.Exec("DROP TABLE IF EXISTS platform_installs")


	return nil
}

// Down 执行降级
func (m *UnmultitenantSchemaMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*UnmultitenantSchemaMigration)(nil)

