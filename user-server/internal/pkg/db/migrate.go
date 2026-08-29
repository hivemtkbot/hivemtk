package db

import (
	"fmt"
	contentmodel "hivemtk-user/internal/content/model"
	geomodel "hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/model"
	opsmodel "hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"strings"

	knowledgemodel "hivemtk-user/internal/aiagent/knowledge/model"

	"gorm.io/gorm"
)

// allModels 列出所有需要 AutoMigrate 的模型。集中在此便于维护。
// 按"单模型 + 错误容忍 + 跳过"逐个执行。
func allModels() []any {
	return []any{
		&model.Order{},
		&model.AfterSale{},
		&model.User{},
		&model.Account{},
		&model.Message{},
		&model.Smlist{},
		&model.Clue{},
		&model.EmailSmtp{},
		&model.EmailDraft{},
		&model.EmailList{},
		&model.EmailJobs{},
		&model.EmailSend{},
		&model.SystemConfig{},
		&model.DomainPool{},
		&model.DomainBlacklist{},
		&model.DomainHealthLog{},
		&model.ShortLink{},
		&model.ShortLinkAccess{},
		&model.DouyinCard{},
		&model.DouyinCardActivity{},
		&model.XiaohongshuCard{},
		&model.XiaohongshuCardActivity{},
		&model.KuaishouCard{},
		&model.KuaishouCardActivity{},
		&model.XianyuCard{},
		&model.XianyuCardActivity{},
		&model.SmsConfig{},
		&model.SmsAliyunConfig{},
		&model.SmsTencentConfig{},
		&model.SmsHuaweiConfig{},
		&model.SmsRecord{},
		&model.SmsDraft{},
		&model.SmsJob{},
		&model.SmsJobDetail{},
		&model.SystemUser{},
		&model.LiveCode{},
		&model.LiveCodeQR{},
		&model.LiveCodeQRStat{},
		&model.ObsConfig{},
		&contentmodel.Material{},
		&contentmodel.MaterialCategory{},

		&model.WhatsappAccount{},
		&model.WhatsappSession{},
		&model.WhatsappDraft{},
		&model.WhatsappJob{},
		&model.WhatsappJobDetail{},
		&knowledgemodel.RagProduct{},
		&model.PlatformAccountConfig{},
		&model.APILog{},
		&model.WebVitalRecord{},
		&model.RagEvalQuestion{},
		&model.RagEvalRun{},
		&model.VisitLog{},
		&model.DailyStats{},
		&knowledgemodel.KBDocument{},
		&model.TikTokCard{},
		&model.TikTokCardActivity{},
		&model.WhatsappGroupMessage{},
		&model.Backup{},
		&model.RestoreRecord{},
		&knowledgemodel.RagSession{},
		&knowledgemodel.RagMessage{},
		&model.CustomerSession{},
		&model.SessionMessage{},
		&model.AgentStatus{},
		&model.AISuggestion{},
		&model.QuickReply{},
		&model.QuickReplyFolder{},
		&model.CSATSurvey{},
		&model.SessionTag{},
		&model.WeComAccount{},
		&model.WeComCustomer{},
		&model.WeComGroup{},
		&model.WeComGroupMember{},
		&model.WeComMessage{},
		&model.WeComTag{},
		&model.TelegramAccount{},
		&model.FeishuAccount{},
		&model.FeishuCustomer{},
		&model.FeishuMessage{},
		&model.WhatsappMessageTemplate{},
		&model.WhatsAppMessageQueue{},
		&model.WhatsAppQueueStatus{},
		&model.SystemMetrics{},
		&model.Customer{},
		&model.CustomerTag{},
		&model.CustomerTagAssignment{},
		&model.CustomerDoNotContact{},
		&model.CustomerEvent{},
		&model.UserTag{},
		&model.OperationLog{},
		&opsmodel.ABExperiment{},
		&opsmodel.ABVariant{},
		&opsmodel.ABConversionEvent{},
		&opsmodel.ABExperimentResult{},
		&opsmodel.ChurnPrediction{},
		&opsmodel.ChurnWarning{},
		&opsmodel.ChurnModelConfig{},
		&opsmodel.ChurnStatistics{},
		&model.RFMRule{},
		&model.UserRFM{},
		&model.IntegrationAccount{},
		&model.SyncLog{},
		&model.ExternalCustomer{},
		&model.ExternalOrder{},
		&model.ExternalProduct{},
		&model.WebhookEvent{},
		&model.CommunityGroup{},
		&model.CommunityMember{},
		&model.CommunityMessage{},
		&contentmodel.MarketingFlow{},
		&contentmodel.FlowExecution{},
		&opsmodel.CustomReport{},
		&opsmodel.DashboardScreen{},
		&opsmodel.DashboardWidget{},
		&contentmodel.MarketTemplate{},
		&contentmodel.MarketTemplateDownload{},
		&contentmodel.ScriptTemplate{},
		&contentmodel.ScriptCategory{},
		&contentmodel.ScriptRecommend{},
		&contentmodel.AIGenerationRecord{},
		&contentmodel.PromptTemplate{},
		&model.UnifiedMessage{},
		&model.UnifiedReply{},
		&model.PlatformAccount{},
		&model.UpgradeTask{},
		&model.MigrationRecord{},
		&model.MigrationCheckpoint{},
		&contentmodel.BatchOperationHistory{},
		&model.MessageHub{},
		&model.InboxConversation{},
		&model.InboxAssignment{},
		&model.MessageTrace{},
		&model.TraceEvalLog{},
		&model.WeComAccountHealth{},
		&model.IntentRecord{},
		&model.IntentLog{}, // 全端扫描发现：生产 AutoMigrate 漏建 intent_logs（/api/intent/logs 500）
		&model.DialogueMemory{},
		&model.SOPAgent{},
		&model.SOPExecution{},
		&model.SOPExecEvent{},
		&model.SOPTimer{},
		&model.SOPOutbox{},
		&model.SalesIntentScore{},
		&model.AISalesLog{},
		&model.MemoryItem{},
		&model.SOPStateMemory{},
		&model.BusinessMemory{},
		&model.CustomerLongTermMemory{},
		&model.LowQualitySample{},
		&model.SalesChampionProfileSnapshot{},
		&model.SOPNodeTransition{},
		&model.OptimizationSuggestion{},
		&model.ConfidenceSignal{},
		&model.ConfidenceCalibration{},
		&model.HandoffDecisionRecord{},
		&model.ThresholdPolicy{},
		&model.ABTest{},
		&model.ABTestMetric{},
		&model.HumanizeScore{},
		&model.HumanizeDimensionRecord{},
		&model.ChampionBaseline{},
		&model.ChampionPhrase{},
		&model.ABTestStat{},
		&model.FeedbackEvent{},
		&model.FeedbackSignal{},
		&model.ChampionDialogue{},
		&model.PromptCandidate{},
		&model.BanditArm{},
		&model.PromptABTest{},
		&model.ReachPipeline{},
		&model.ReachJob{},
		&model.ScriptLibrary{},
		&model.ScriptVersion{},
		&model.ScriptExposureLog{},
		&model.FeatureFlag{},
		&model.FeatureFlagAuditLog{},
		&model.FeatureFlagEvalLog{},
		&model.ObjectionTemplate{},
		&model.ConversionFunnel{},
		&model.SalesPersona{},
		&model.SalesEvent{}, // H2：销售事件流持久化（替代原内存版 SalesDashboard）
		&opsmodel.PerformanceTestResult{},
		&knowledgemodel.KnowledgeDocument{},
		&knowledgemodel.KnowledgeChunk{},
		&knowledgemodel.KnowledgeImportLog{},
		&knowledgemodel.KnowledgeSearchLog{},
		&knowledgemodel.KnowledgeOpenAPISource{},
		&knowledgemodel.KnowledgeAPIToken{},
		&knowledgemodel.KnowledgeFeedback{},
		&knowledgemodel.ExternalImportJob{},
		&model.AIAgent{},
		&model.ChannelAgentBinding{},
		&model.CustomerServiceAgent{},
		&model.LLMProvider{},
		&model.LLMRoutingRule{},
		&model.FAQEntry{},
		&model.SOPTemplate{},
		&model.LayerDecisionLog{},
		&model.ChatChannel{},
		&model.Notification{},
		&model.AssetBundle{},
		&model.AssetBundleVersionLog{},
		&model.LocalAsset{},
		&model.LocalAssetData{},
		&model.LocalAssetSyncLog{},
		&model.DingTalkAppAccount{},
		&model.WhatsAppCloudAccount{},
		&model.AIToolConfig{},
		&model.AIToolAccountBinding{},
		&model.EmailUnsubscribe{},
		&model.SmsUnsubscribe{},
		&model.SmsDeliveryStatus{},
		&model.SmsNumberPortabilityRecord{},
		&model.RagQueryLog{},
		&model.RagRecallMonitorSnapshot{},
		&model.FeedbackRecordORM{},
		&model.LeadMiningConfig{},
		&model.SecurityAudit{},
		&model.SecurityAuditItem{},
		&model.UserBlacklist{},
		&model.BridgeAccount{},
		&model.PasswordResetToken{},

		// GEO 模块
		&geomodel.GeoKeyword{},
		&geomodel.GeoKeywordGroup{},
		&geomodel.GeoArticle{},
		&geomodel.GeoOptimization{},
		&geomodel.GeoVerifyResult{},
		&geomodel.GeoAPICall{},
		&geomodel.GeoConfig{},
		&geomodel.GeoPlatformAccount{},
		&geomodel.GeoPublishRecord{},
		&geomodel.GeoKnowledgeDocument{},
		&geomodel.GeoWorkflow{},
		&geomodel.GeoWorkflowExecution{},
		&geomodel.GeoWorkflowTemplate{},
		// v3 GEO 决策链化
		&geomodel.GeoQueryChain{},
		&geomodel.GeoContentTask{},
	}
}

// ensureExtensions 在 AutoMigrate 前确保依赖的 PG 扩展已就绪（幂等）。
//
// 业务模型含 pgvector 向量列（champion_dialogues / knowledge_chunks 等），
// 若扩展缺失，GORM AutoMigrate 会报 `type "vector" does not exist`(42704) 中断迁移。
// 生产由 init-user-db.sql 的 CREATE EXTENSION 预建；此处再于 Go 路径主动预建，
// 使 AutoMigrate 自洽（测试 / 私有化镜像未含 vector 包时也不会因扩展缺失而 panic）。
// 失败仅告警不 panic：扩展可能在外部 SQL 已建好，或镜像未含 vector 包（由后续建表错误暴露）。
//
// 安全说明：PostgreSQL CREATE EXTENSION 不支持参数化占位符，必须拼接扩展名。
// 因此采用白名单严格校验：仅允许 vector / uuid-ossp 两个已知扩展名，杜绝 SQL 注入。
func ensureExtensions() {
	allowedExts := map[string]bool{
		"vector":    true,
		"uuid-ossp": true,
	}
	exts := []string{"vector", `"uuid-ossp"`}
	for _, ext := range exts {
		bare := strings.Trim(ext, `"`)
		if !allowedExts[bare] {
			logger.Warn(fmt.Sprintf("跳过未知 PG 扩展(白名单拒绝): %s", ext))
			continue
		}
		if err := DB.Exec("CREATE EXTENSION IF NOT EXISTS " + ext).Error; err != nil {
			logger.Warn(fmt.Sprintf("启用 PG 扩展提示(可忽略若已在外部创建): %s, err=%v", ext, err))
		}
	}
}

func AutoMigrate() *gorm.DB {
	ensureExtensions()

	tolerated := 0
	for _, m := range allModels() {
		if err := DB.AutoMigrate(m); err != nil {
			if isTolerableMigrateError(err) {
				if !DB.Migrator().HasTable(m) {
					createTableFallback(m)
				}
				tolerated++
				continue
			}
			panic(err)
		}
	}

	if missing := missingTables(DB, allModels()...); len(missing) > 0 {
		panic(fmt.Sprintf("AutoMigrate 终校验失败：以下模型对应的数据表缺失（可能被可容忍错误静默吞掉，需排查建表失败根因）：%s", strings.Join(missing, ", ")))
	}

	if tolerated > 0 {
		logger.Warn("AutoMigrate 完成，但存在可容忍的迁移漂移（历史约束命名不一致），建议排查并清理遗留历史表，避免长期静默漂移掩盖真实故障")
	} else {
		logger.Info("AutoMigrate 完成，无迁移漂移")
	}

	postMigrateMessageHubUniqueIndex()

	return DB
}

// postMigrateMessageHubUniqueIndex 把 message_hub 的 msg_id 唯一约束迁移为
// (platform, msg_id, conversation_id) 三元组 UNIQUE（v2 四元组去重契约的库内表达：
// 同一 msg_id 允许在不同渠道下共存，仅同渠道同会话内强唯一）。
// 幂等：旧索引不存在时跳过，新索引已存在时跳过。
func postMigrateMessageHubUniqueIndex() {
	if DB == nil {
		return
	}
	if err := DB.Exec(`ALTER TABLE message_hub DROP CONSTRAINT IF EXISTS uni_message_hub_msg_id`).Error; err != nil {
		logger.Warn(fmt.Sprintf("post-migrate: DROP 旧 uni_message_hub_msg_id 失败(可忽略若已不存在): %v", err))
	}
	// 旧二元组索引比新契约更严格，先降级移除（3 列唯一对既有数据恒兼容，重建不会失败）
	if err := DB.Exec(`DROP INDEX IF EXISTS uni_message_hub_msg_id_conv`).Error; err != nil {
		logger.Warn(fmt.Sprintf("post-migrate: DROP 旧 uni_message_hub_msg_id_conv 失败: %v", err))
	}
	if err := DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uni_message_hub_platform_msg_conv ON message_hub (platform, msg_id, conversation_id)`).Error; err != nil {
		logger.Warn(fmt.Sprintf("post-migrate: CREATE uni_message_hub_platform_msg_conv 失败: %v", err))
	} else {
		logger.Info("post-migrate: message_hub (platform, msg_id, conversation_id) 三元组唯一索引已就绪")
	}
}

// isTolerableMigrateError 判断迁移错误是否可容忍（仅限「历史约束命名漂移」类 NOTICE）。
//
// 收窄为明确白名单——只容忍 PG 在老 schema 上尝试 DROP 一个非 GORM 命名
// 的约束（constraint ... does not exist, SQLSTATE 42704）。其余错误（含 relation does
// not exist、真实建表失败等）一律不吞，直接 panic 上报，避免静默漂移掩盖真实故障。
//
// 命中白名单时记录一次 drift 告警，使「本应不存在却仍存在」的历史约束可见，便于后续清理。
func isTolerableMigrateError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "does not exist") && strings.Contains(msg, "constraint") {
		logger.Warn("AutoMigrate 命中历史约束命名漂移(可容忍，建议清理历史表): " + msg)
		return true
	}
	if strings.Contains(msg, "already exists") {
		logger.Warn("AutoMigrate 命中幂等重跑提示(可容忍): " + msg)
		return true
	}
	return false
}

// missingTables 终校验辅助：返回 allModels 中尚未在数据库中落地的模型类型名。
// 仅做存在性探测（HasTable），不触发任何 DDL，用作 AutoMigrate 收尾的兜底检测，
// 确保「被可容忍错误跳过却实际未建表」的模型在启动期暴露，而非运行时 500。
func missingTables(db *gorm.DB, models ...any) []string {
	var missing []string
	for _, m := range models {
		if db == nil {
			missing = append(missing, fmt.Sprintf("%T", m))
			continue
		}
		if !db.Migrator().HasTable(m) {
			missing = append(missing, fmt.Sprintf("%T", m))
		}
	}
	return missing
}

// tableNameOf 通过 GORM 解析模型对应的表名（含自定义 TableName）。
func tableNameOf(db *gorm.DB, m any) string {
	stmt := &gorm.Statement{DB: db, Dest: m}
	stmt.Parse(m)
	return stmt.Table
}

// createTableFallback 在 AutoMigrate 被可容忍错误中断、但本表尚未建成时，
// 仅建本表（CreateTable 不递归迁移关联模型，避开级联约束漂移），并先剔除
// 可能残留的同名 composite type（历史手动建表 / 部分迁移遗留），否则
// CREATE TABLE 会因类型名冲突而静默失败（"type already exists" 被误判可容忍）。
// 建表后二次核对：仍缺失则直接 panic，不留瑕疵。
func createTableFallback(m any) {
	if tn := tableNameOf(DB, m); tn != "" {
		DB.Exec(fmt.Sprintf("DROP TYPE IF EXISTS %s CASCADE", tn))
	}
	if cerr := DB.Migrator().CreateTable(m); cerr != nil && !isTolerableMigrateError(cerr) {
		panic(fmt.Sprintf("AutoMigrate 兜底 CreateTable(%T) 失败: %v", m, cerr))
	}
	if DB.Migrator().HasTable(m) {
		logger.Warn(fmt.Sprintf("AutoMigrate 兜底 CreateTable 已重建缺失表: %s", tableNameOf(DB, m)))
	} else {
		panic(fmt.Sprintf("AutoMigrate 兜底 CreateTable(%T) 后表仍缺失，需排查建表失败根因", m))
	}
}

