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
// 修复历史问题：原来单次 DB.AutoMigrate(...) 调用，任一模型失败（如历史约束
// 命名漂移 DROP CONSTRAINT 不存在）会中断整个迁移，导致后续模型未创建。
// 现按"单模型 + 错误容忍 + 跳过"逐个执行。
func allModels() []any {
	return []any{
		&model.Order{},
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
		// 开源版：已移除 &model.License{}（License 模型删除，授权流程下线）
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
		// 支付配置
		&model.PaymentConfig{},
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
		// 团队权限
		&model.TeamUser{},
		&model.TeamRole{},
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
		// 修复历史缺陷：这三张表原仅由 registry 迁移系统(SOPExecutorMigration)创建，
		// 但该 registry 在启动流中从未运行，导致 sop_outbox_dispatcher 每 5s 查询
		// sop_timers 报 "relation does not exist"。改由 AutoMigrate 统一创建。
		&model.SOPExecEvent{},
		&model.SOPTimer{},
		&model.SOPOutbox{},
		&model.SalesIntentScore{},
		&model.AISalesLog{},
		// P0-13 4 层记忆系统
		&model.MemoryItem{},
		&model.SOPStateMemory{},
		&model.BusinessMemory{},
		// P1-1 G5 L2 长期记忆 pgvector 增强
		&model.CustomerLongTermMemory{},
		// P1-2 G6 拟人度评估器低质样本收集
		&model.LowQualitySample{},
		// P1-3 G7 反馈学习闭环（销冠画像快照 / SOP 节点流转 / 优化建议）
		&model.SalesChampionProfileSnapshot{},
		&model.SOPNodeTransition{},
		&model.OptimizationSuggestion{},
		// 修复历史缺陷：以下 19 张表原仅由 registry 迁移系统创建
		// （confidence/humanize_evaluator/feedback_loop migration），
		// 但 registry 在启动流中从未运行（NewMigrationService 无非测试调用），
		// 导致后台任务（ConfidenceAggregator / HumanizeEvalService /
		// FeedbackCollector / FeedbackLoopCron）查询这些表时报
		// "relation does not exist"。改由 AutoMigrate 统一创建，与项目现有 schema 机制一致。
		// P0-3 置信度驱动转人工（8 表）
		&model.ConfidenceSignal{},
		&model.ConfidenceCalibration{},
		&model.HandoffDecisionRecord{},
		&model.ReviewQueue{},
		&model.ThresholdPolicy{},
		&model.SLAMonitor{},
		&model.ABTest{},
		&model.ABTestMetric{},
		// P0-4 拟人度评估器（5 表）
		&model.HumanizeScore{},
		&model.HumanizeDimensionRecord{},
		&model.ChampionBaseline{},
		&model.ChampionPhrase{},
		&model.ABTestStat{},
		// P0-5 反馈学习闭环（6 表，champion_dialogues 含 pgvector 向量列）
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
		// P2/P3 扩展模型
		// 开源版：已移除 &model.OTAVersion{}（OTA 流程下线）
		&opsmodel.PerformanceTestResult{},
		&model.SecurityAuditResult{},
		// RAG V2.0 增强模型
		&knowledgemodel.KnowledgeDocument{},
		&knowledgemodel.KnowledgeChunk{},
		&knowledgemodel.KnowledgeImportLog{},
		&knowledgemodel.KnowledgeSearchLog{},
		&knowledgemodel.KnowledgeOpenAPISource{},
		// P0-14 商户 RAG 增强：API Token、用户反馈、外部导入任务
		&knowledgemodel.KnowledgeAPIToken{},
		&knowledgemodel.KnowledgeFeedback{},
		&knowledgemodel.ExternalImportJob{},
		// 多 AI 智能体架构：智能体主表 + 渠道绑定 + 客服挂载
		&model.AIAgent{},
		&model.ChannelAgentBinding{},
		&model.CustomerServiceAgent{},
		// P0-10 ADR-010: 客服 Web Widget 嵌入渠道
		&model.ChatChannel{},
		// P2-X: 商户端通知中心（站内通知 / 顶部铃铛 badge）
		&model.Notification{},
		// 方向9：资产包模式 — OpenAI messages 资产包 CRUD + Weave 织布算法
		&model.AssetBundle{},
		&model.AssetBundleVersionLog{},
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
	// 这是关键修复：原来一次性传入所有模型，PG 在某个历史约束命名漂移时
	// 抛错后 GORM 会中断整个调用，导致后续 100+ 表未创建（system_users、
	// sop_agents、customer_sessions、chat_channels 等核心表都缺失）。
	tolerated := 0
	for _, m := range allModels() {
		if err := DB.AutoMigrate(m); err != nil {
			if isTolerableMigrateError(err) {
				tolerated++
				continue
			}
			// 真正不可恢复的错误仍然 panic 上报
			panic(err)
		}
	}
	if tolerated > 0 {
		// V4/R5/R6 修复：将「静默忽略」升级为显式漂移告警，使运维可感知长期漂移。
		logger.Warn("AutoMigrate 完成，但存在可容忍的迁移漂移（历史约束命名不一致），建议排查并清理遗留历史表，避免长期静默漂移掩盖真实故障")
	} else {
		logger.Info("AutoMigrate 完成，无迁移漂移")
	}
	return DB
}

// isTolerableMigrateError 判断迁移错误是否可容忍（仅限「历史约束命名漂移」类 NOTICE）。
//
// V4/R5/R6 修复：收窄为明确白名单——只容忍 PG 在老 schema 上尝试 DROP 一个非 GORM 命名
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
	// 属正常 no-op，不应触发 panic（R5/R6 仅针对真实建表/列失败，如 relation does not exist）。
	if strings.Contains(msg, "already exists") {
		logger.Warn("AutoMigrate 命中幂等重跑提示(可容忍): " + msg)
		return true
	}
	return false
}
