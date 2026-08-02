package db

import (
	"fmt"
	contentmodel "marketing/internal/content/model"
	"marketing/internal/model"
	opsmodel "marketing/internal/ops/model"
	"marketing/internal/pkg/utils/logger"
	"strings"

	knowledgemodel "marketing/internal/aiagent/knowledge/model"

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
		// 短信相关模型
		&model.SmsConfig{},
		&model.SmsAliyunConfig{},
		&model.SmsTencentConfig{},
		&model.SmsHuaweiConfig{},
		&model.SmsRecord{},
		&model.SmsDraft{},
		&model.SmsJob{},
		&model.SmsJobDetail{},
		// 用户管理模型
		&model.SystemUser{},
		// 活码相关模型
		&model.LiveCode{},
		&model.LiveCodeQR{},
		&model.LiveCodeQRStat{},
		// 平台端模型
		&model.ObsConfig{},
		// 素材库模型
		&contentmodel.Material{},
		&contentmodel.MaterialCategory{},
		&model.AutoReplyAccount{},
		&model.AutoReplyRule{},
		&model.AutoReplyLog{},
		// WhatsApp models
		&model.WhatsappAccount{},
		&model.WhatsappSession{},
		&model.WhatsappDraft{},
		&model.WhatsappJob{},
		&model.WhatsappJobDetail{},
		// RAG相关模型
		&knowledgemodel.RagProduct{},
		&model.PlatformAccountConfig{},
		// 统计相关模型
		&model.APILog{},
		&model.VisitLog{},
		&model.DailyStats{},
		// 知识库文档
		&knowledgemodel.KBDocument{},
		&model.TikTokCard{},
		&model.TikTokCardActivity{},
		// WhatsApp 群发消息
		&model.WhatsappGroupMessage{},
		// 备份恢复
		&model.Backup{},
		&model.RestoreRecord{},
		// RAG会话持久化
		&knowledgemodel.RagSession{},
		&knowledgemodel.RagMessage{},
		// 客服会话相关模型
		&model.CustomerSession{},
		&model.SessionMessage{},
		&model.AgentStatus{},
		&model.AISuggestion{},
		&model.QuickReply{},
		&model.SessionTag{},
		// 企业微信相关模型
		&model.WeComAccount{},
		&model.WeComCustomer{},
		&model.WeComGroup{},
		&model.WeComGroupMember{},
		&model.WeComMessage{},
		&model.WeComTag{},
		// Telegram 机器人账号（TG 智能体流程入口）
		&model.TelegramAccount{},
		// 飞书账号（Feishu/Lark 机器人）
		&model.FeishuAccount{},
		&model.FeishuCustomer{},
		&model.FeishuMessage{},
		// WhatsApp 消息模板
		&model.WhatsappMessageTemplate{},
		// WhatsApp 消息队列持久化
		&model.WhatsAppMessageQueue{},
		&model.WhatsAppQueueStatus{},
		// 系统指标
		&model.SystemMetrics{},
		// CDP / 客户数据
		&model.Customer{},
		&model.CustomerTag{},
		&model.CustomerEvent{},
		&model.UserTag{},
		// 系统审计
		&model.OperationLog{},
		// A/B 测试
		&opsmodel.ABExperiment{},
		&opsmodel.ABVariant{},
		&opsmodel.ABConversionEvent{},
		&opsmodel.ABExperimentResult{},
		// 流失预测
		&opsmodel.ChurnPrediction{},
		&opsmodel.ChurnWarning{},
		&opsmodel.ChurnModelConfig{},
		&opsmodel.ChurnStatistics{},
		// RFM
		&model.RFMRule{},
		&model.UserRFM{},
		// 第三方对接
		&model.IntegrationAccount{},
		&model.SyncLog{},
		&model.ExternalCustomer{},
		&model.ExternalOrder{},
		&model.ExternalProduct{},
		&model.WebhookEvent{},
		// 社群
		&model.CommunityGroup{},
		&model.CommunityMember{},
		&model.CommunityMessage{},
		// 营销流程
		&contentmodel.MarketingFlow{},
		&contentmodel.FlowExecution{},
		// 自定义报表
		&opsmodel.CustomReport{},
		// 数据大屏
		&opsmodel.DashboardScreen{},
		&opsmodel.DashboardWidget{},
		// 模板市场
		&contentmodel.MarketTemplate{},
		&contentmodel.MarketTemplateDownload{},
		// 话术模板
		&contentmodel.ScriptTemplate{},
		&contentmodel.ScriptCategory{},
		&contentmodel.ScriptRecommend{},
		// AI 内容
		&contentmodel.AIGenerationRecord{},
		&contentmodel.PromptTemplate{},
		// 统一消息
		&model.UnifiedMessage{},
		&model.UnifiedReply{},
		&model.PlatformAccount{},
		// 升级迁移
		&model.UpgradeTask{},
		&model.MigrationRecord{},
		&model.MigrationCheckpoint{},
		// 批量操作
		&contentmodel.BatchOperationHistory{},
		// AI 私域销冠系统 - 多账号聚合与消息中台
		&model.MessageHub{},
		&model.InboxConversation{},
		&model.InboxAssignment{},
		&model.WeComAccountHealth{},
		// AI 私域销冠系统 - AI 谈单中枢
		&model.IntentRecord{},
		&model.DialogueMemory{},
		&model.SOPAgent{},
		&model.SOPExecution{},
		// SOP 执行器持久化：事件溯源 / 定时器 / Outbox
		&model.SOPExecEvent{},
		&model.SOPTimer{},
		&model.SOPOutbox{},
		&model.SalesIntentScore{},
		&model.AISalesLog{},
		// 4 层记忆系统
		&model.MemoryItem{},
		&model.SOPStateMemory{},
		&model.BusinessMemory{},
		// G5 L2 长期记忆 pgvector 增强
		&model.CustomerLongTermMemory{},
		// G6 拟人度评估器低质样本收集
		&model.LowQualitySample{},
		// G7 反馈学习闭环（销冠画像快照 / SOP 节点流转 / 优化建议）
		&model.SalesChampionProfileSnapshot{},
		&model.SOPNodeTransition{},
		&model.OptimizationSuggestion{},
		// 置信度驱动转人工（8 表）
		&model.ConfidenceSignal{},
		&model.ConfidenceCalibration{},
		&model.HandoffDecisionRecord{},
		&model.ReviewQueue{},
		&model.ThresholdPolicy{},
		&model.SLAMonitor{},
		&model.ABTest{},
		&model.ABTestMetric{},
		// 拟人度评估器（5 表）
		&model.HumanizeScore{},
		&model.HumanizeDimensionRecord{},
		&model.ChampionBaseline{},
		&model.ChampionPhrase{},
		&model.ABTestStat{},
		// 反馈学习闭环（6 表，champion_dialogues 含 pgvector 向量列）
		&model.FeedbackEvent{},
		&model.FeedbackSignal{},
		&model.ChampionDialogue{},
		&model.PromptCandidate{},
		&model.BanditArm{},
		&model.PromptABTest{},
		// AI 私域销冠系统 - 触达 Pipeline
		&model.ReachPipeline{},
		&model.ReachJob{},
		// AI 私域销冠系统 - 知识与话术
		&model.ScriptLibrary{},
		&model.ObjectionTemplate{},
		// AI 私域销冠系统 - 数据报表
		&model.ConversionFunnel{},
		&model.SalesPersona{},
		// / 扩展模型
		&opsmodel.PerformanceTestResult{},
		&model.SecurityAuditResult{},
		// RAG V2.0 增强模型
		&knowledgemodel.KnowledgeDocument{},
		&knowledgemodel.KnowledgeChunk{},
		&knowledgemodel.KnowledgeImportLog{},
		&knowledgemodel.KnowledgeSearchLog{},
		&knowledgemodel.KnowledgeOpenAPISource{},
		// 商户 RAG 增强：API Token、用户反馈、外部导入任务
		&knowledgemodel.KnowledgeAPIToken{},
		&knowledgemodel.KnowledgeFeedback{},
		&knowledgemodel.ExternalImportJob{},
		// 多 AI 智能体架构：智能体主表 + 渠道绑定 + 客服挂载
	&model.AIAgent{},
	&model.ChannelAgentBinding{},
	&model.CustomerServiceAgent{},
	// LLM provider 定义持久化（替代纯内存态 AddProvider，重启不丢）
	&model.LLMProvider{},
	// 场景路由规则持久化（替代纯内存态 ScenarioRoute 种子，重启不丢、多实例一致）
	&model.LLMRoutingRule{},
		// AI 性能优化: FAQ / SOP 模板 + Layer 决策日志 (双层架构)
		&model.FAQEntry{},
		&model.SOPTemplate{},
		&model.LayerDecisionLog{},
		// 客服 Web Widget 嵌入渠道
		&model.ChatChannel{},
		// 商户端通知中心（站内通知 / 顶部铃铛 badge）
		&model.Notification{},
		// 方向9：资产包模式 — OpenAI messages 资产包 CRUD + Weave 织布算法
		&model.AssetBundle{},
		&model.AssetBundleVersionLog{},
		// 资产市场本地资产（local-assets）：列表/详情/同步日志三表，此前遗漏未加入
		// 自动迁移，导致 /api/v1/local-assets 报 relation "local_assets" does not exist。
		&model.LocalAsset{},
		&model.LocalAssetData{},
		&model.LocalAssetSyncLog{},
		// 钉钉企业内部应用（支持回调收消息）
		&model.DingTalkAppAccount{},
		// WhatsApp Cloud API 商业账号
		&model.WhatsAppCloudAccount{},
		// 补全此前遗漏的自动迁移模型：以下表由独立 Go 迁移/合规与监控逻辑创建，
		// 但未登记进 allModels()，导致 AutoMigrate 不建表，相关后台页 API 报
		// "relation does not exist"。统一在此登记，重启即自动建表（与项目其他表一致）。
		&model.AIToolConfig{},
		&model.AIToolAccountBinding{},
		&model.EmailUnsubscribe{},
		&model.SmsUnsubscribe{},
		&model.SmsDeliveryStatus{},
		&model.SmsNumberPortabilityRecord{},
		&model.RagQueryLog{},
		&model.RagRecallMonitorSnapshot{},
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
	// 白名单：仅允许这些已知安全的扩展名（小写，不带引号）。
	allowedExts := map[string]bool{
		"vector":    true,
		"uuid-ossp": true,
	}
	exts := []string{"vector", `"uuid-ossp"`}
	for _, ext := range exts {
		// 去除引号后做白名单校验，防止任何外部输入混入。
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
	// 预建依赖扩展，避免向量列模型在无扩展环境下建表中断。
	ensureExtensions()

	// 逐个模型执行 AutoMigrate：单个模型失败不影响后续模型。
	// 一次性传入所有模型时，PG 在约束命名漂移时
	// 抛错会令 GORM 中断整个调用，导致后续 100+ 表未创建（system_users、
	// sop_agents、customer_sessions、chat_channels 等核心表都缺失）。
	tolerated := 0
	for _, m := range allModels() {
		if err := DB.AutoMigrate(m); err != nil {
			if isTolerableMigrateError(err) {
				// 兜底（缺陷闭环）：可容忍错误（历史约束命名漂移等）可能来自
				// 级联关联模型的迁移——例如 platform_account_configs 的 belongs-to
				// 关联 RagProduct 在迁移时触发 rag_products 的约束漂移错误，导致
				// 整个 AutoMigrate 在「本表尚未创建」时被中断。此时用 CreateTable
				// 仅建本表（不递归迁移关联模型），并先剔除可能残留的同名
				// composite type（历史手动建表/部分迁移遗留），避开建表名冲突，
				// 保证表真实落地。
				if !DB.Migrator().HasTable(m) {
					createTableFallback(m)
				}
				tolerated++
				continue
			}
			// 真正不可恢复的错误仍然 panic 上报
			panic(err)
		}
	}

	// 终校验（缺陷闭环）：兜底捕获「单模型迁移被可容忍错误静默跳过、
	// 但表实际未建成」的情况。isTolerableMigrateError 仅按错误文本宽口径放行，
	// 无法区分「约束命名漂移（表已存在）」与「建表失败（表不存在）」，
	// 历史上曾导致 platform_account_configs 在 DB 重置后漏建、/ 接口运行时
	// 才报 relation does not exist。此处逐模型核对表是否真实落地，任一缺失即
	// 启动期 panic，把隐患暴露在部署阶段而非生产 500。
	if missing := missingTables(DB, allModels()...); len(missing) > 0 {
		panic(fmt.Sprintf("AutoMigrate 终校验失败：以下模型对应的数据表缺失（可能被可容忍错误静默吞掉，需排查建表失败根因）：%s", strings.Join(missing, ", ")))
	}

	if tolerated > 0 {
		// 将「静默忽略」升级为显式漂移告警，使运维可感知长期漂移。
		logger.Warn("AutoMigrate 完成，但存在可容忍的迁移漂移（历史约束命名不一致），建议排查并清理遗留历史表，避免长期静默漂移掩盖真实故障")
	} else {
		logger.Info("AutoMigrate 完成，无迁移漂移")
	}
	return DB
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
	// 历史约束命名漂移：GORM 尝试 DROP 一个非 GORM 命名的旧约束，PG 报 42704。
	if strings.Contains(msg, "does not exist") && strings.Contains(msg, "constraint") {
		logger.Warn("AutoMigrate 命中历史约束命名漂移(可容忍，建议清理历史表): " + msg)
		return true
	}
	// 幂等重跑时 GORM 可能发出 "already exists"（relation/type/constraint/index 已存在），
	// 属正常 no-op，不应触发 panic（/ 仅针对真实建表/列失败，如 relation does not exist）。
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
		// 剔除同名 composite type，避免 CREATE TABLE 名冲突导致静默失败。
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
