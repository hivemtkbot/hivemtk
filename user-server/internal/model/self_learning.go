package model

// self_learning.go 对话驱动自我学习三位一体机制 Model 定义
//
// 五层架构归属: L4 Model 层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1)
// 私域独立部署: 无 merchant_id / tenant_id 多租户字段
//
// 本文件包含 3 个实体：
//   1. SelfLearningLog        - 自我学习日志表（幂等保证 + 全链路追踪）
//   2. AssetBundleCandidate   - 资产包候选表（销冠对话 → 候选 → A/B → 上线）
//   3. AssetBundleABTest      - 资产包 A/B 实验表（baseline vs candidate）
//
// 同时扩展 KnowledgeChunk（已在 knowledge_workspace.go 中定义）增加 6 个质量字段，
// 因 GORM AutoMigrate 不可靠，扩展字段通过 SelfLearningMigration 维护，
// 此处仅在 KnowledgeChunk 结构体追加字段映射，便于代码层透明访问。

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
)

// ============================================================================
// SelfLearningLog 自我学习日志表
// ============================================================================

// SelfLearningScenario 自我学习场景类型
type SelfLearningScenario string

const (
	// ScenarioRagWarmup RAG 预热（dialogue.started 触发）
	ScenarioRagWarmup SelfLearningScenario = "rag_warmup"
	// ScenarioRagReflect RAG 反思（dialogue.ended 触发）
	ScenarioRagReflect SelfLearningScenario = "rag_reflect"
	// ScenarioAssetGenerate 资产包候选生成
	ScenarioAssetGenerate SelfLearningScenario = "asset_generate"
	// ScenarioAssetABTest 资产包 A/B 测试
	ScenarioAssetABTest SelfLearningScenario = "asset_ab_test"
	// ScenarioAssetDegrade 资产包降级
	ScenarioAssetDegrade SelfLearningScenario = "asset_degrade"
	// ScenarioRagSupervision RAG 自我监督（5 维指标采集）
	ScenarioRagSupervision SelfLearningScenario = "rag_supervision"
	// ScenarioAssetSupervision 资产包自我监督
	ScenarioAssetSupervision SelfLearningScenario = "asset_supervision"
	// ScenarioLLMCorrection LLM 自我矫正（幻觉/跑题修复）
	ScenarioLLMCorrection SelfLearningScenario = "llm_correction"
)

// SelfLearningStatus 自我学习日志状态
type SelfLearningStatus string

const (
	SelfLearningStatusPending SelfLearningStatus = "pending"
	SelfLearningStatusRunning SelfLearningStatus = "running"
	SelfLearningStatusSuccess SelfLearningStatus = "success"
	SelfLearningStatusFailed  SelfLearningStatus = "failed"
	SelfLearningStatusSkipped SelfLearningStatus = "skipped"
)

// SelfLearningTriggerEvent 触发事件类型
type SelfLearningTriggerEvent string

const (
	TriggerEventDialogueStarted   SelfLearningTriggerEvent = "dialogue.started"
	TriggerEventDialogueEnded     SelfLearningTriggerEvent = "dialogue.ended"
	TriggerEventAssetDegraded     SelfLearningTriggerEvent = "asset.degraded"
	TriggerEventRagCorpusUpdated  SelfLearningTriggerEvent = "rag.corpus.updated"
	TriggerEventCronHourly        SelfLearningTriggerEvent = "cron.hourly"
	TriggerEventCronDaily         SelfLearningTriggerEvent = "cron.daily"
	TriggerEventCronSixHours      SelfLearningTriggerEvent = "cron.6h"
	TriggerEventSupervisionSignal SelfLearningTriggerEvent = "supervision.signal"
)

// SelfLearningLog 自我学习日志实体
//
// 设计要点：
//   - log_id 全局唯一（sha256 去重键，便于跨实例追溯）
//   - UNIQUE(session_id, scenario) 保证幂等：相同 session + 场景仅处理一次
//   - input_summary/output_summary 用 JSONB 灵活存储场景特定数据
//   - trace_id 串联全链路（与 LLM 调用日志 trace_id 对齐）
type SelfLearningLog struct {
	ID            uint64                   `gorm:"primaryKey;autoIncrement" json:"id"`
	LogID         string                   `gorm:"type:varchar(64);uniqueIndex;not null" json:"log_id"`
	SessionID     string                   `gorm:"type:varchar(64);index;not null" json:"session_id"`
	TraceID       string                   `gorm:"type:varchar(64);index" json:"trace_id"`
	Scenario      SelfLearningScenario     `gorm:"type:varchar(32);index;not null" json:"scenario"`
	TriggerEvent  SelfLearningTriggerEvent `gorm:"type:varchar(32);not null" json:"trigger_event"`
	Status        SelfLearningStatus       `gorm:"type:varchar(16);index;not null;default:'pending'" json:"status"`
	InputSummary  JSONMap                  `gorm:"type:jsonb;default:'{}'" json:"input_summary"`
	OutputSummary JSONMap                  `gorm:"type:jsonb;default:'{}'" json:"output_summary"`
	ErrorMsg      string                   `gorm:"type:text" json:"error_msg"`
	DurationMs    int64                    `gorm:"default:0" json:"duration_ms"`
	StartedAt     time.Time                `gorm:"not null" json:"started_at"`
	FinishedAt    *time.Time               `json:"finished_at"`
	CreatedAt     time.Time                `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 表名
func (SelfLearningLog) TableName() string { return "self_learning_logs" }

// ============================================================================
// AssetBundleCandidate 资产包候选表
// ============================================================================

// AssetBundleCandidateStatus 候选状态
type AssetBundleCandidateStatus string

const (
	// CandidateStatusCandidate 候选中（待聚类升级）
	CandidateStatusCandidate AssetBundleCandidateStatus = "candidate"
	// CandidateStatusABTesting A/B 实验中
	CandidateStatusABTesting AssetBundleCandidateStatus = "ab_testing"
	// CandidateStatusPromoted 已升级为 active 资产包
	CandidateStatusPromoted AssetBundleCandidateStatus = "promoted"
	// CandidateStatusRejected 已拒绝（A/B 失败或人工拒绝）
	CandidateStatusRejected AssetBundleCandidateStatus = "rejected"
)

// AssetBundleCandidate 资产包候选实体
//
// 来源链路：
//
//	dialogue.ended (reward ≥ 2.0 & outcome=converted)
//	     → ChampionDialogueAnalyzer.AnalyzePipeline 提取话术
//	     → scriptsToMessages 打包为 OpenAI ChatML messages
//	     → 写入本表（status=candidate）
//	     → 定时聚类（pgvector similarity ≥ 0.85）
//	     → 簇大小 ≥ 3 → 升级为 A/B 实验
//	     → BanditAllocator 收敛 → promoted_asset_id 写入 asset_bundles
type AssetBundleCandidate struct {
	ID               uint64                     `gorm:"primaryKey;autoIncrement" json:"id"`
	CandidateID      string                     `gorm:"type:varchar(64);uniqueIndex;not null" json:"candidate_id"`
	SourceSessionIDs pq.StringArray             `gorm:"type:text[];not null;default:'{}'" json:"source_session_ids"`
	ExtractedScripts JSONMap                    `gorm:"type:jsonb;not null;default:'[]'" json:"extracted_scripts"`
	ProposedMessages AssetBundleMessages        `gorm:"type:jsonb;not null;default:'[]'" json:"proposed_messages"`
	Industry         string                     `gorm:"type:varchar(32);index" json:"industry"`
	Language         string                     `gorm:"type:varchar(8);not null;default:'zh'" json:"language"`
	Scenario         string                     `gorm:"type:varchar(32);index" json:"scenario"`
	ClusterCount     int                        `gorm:"not null;default:0" json:"cluster_count"`
	RewardSum        float64                    `gorm:"type:decimal(10,3);not null;default:0" json:"reward_sum"`
	Status           AssetBundleCandidateStatus `gorm:"type:varchar(16);index;not null;default:'candidate'" json:"status"`
	ABTestID         string                     `gorm:"type:varchar(64);index" json:"ab_test_id"`
	PromotedAssetID  string                     `gorm:"type:varchar(64)" json:"promoted_asset_id"`
	CreatedAt        time.Time                  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time                  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (AssetBundleCandidate) TableName() string { return "asset_bundle_candidates" }

// ============================================================================
// AssetBundleABTest 资产包 A/B 实验表
// ============================================================================

// AssetBundleABTestStatus A/B 实验状态
type AssetBundleABTestStatus string

const (
	// ABTestStatusRunning 实验进行中
	ABTestStatusRunning AssetBundleABTestStatus = "running"
	// ABTestStatusConverged 已收敛（BanditAllocator 判定）
	ABTestStatusConverged AssetBundleABTestStatus = "converged"
	// ABTestStatusCompleted 已完成（promote 或 rollback）
	ABTestStatusCompleted AssetBundleABTestStatus = "completed"
	// ABTestStatusRolledBack 已回滚（A/B 失败、候选不收敛）
	ABTestStatusRolledBack AssetBundleABTestStatus = "rolled_back"
)

// ABTestWinnerArm 胜出臂
type ABTestWinnerArm string

const (
	WinnerArmBaseline  ABTestWinnerArm = "baseline"
	WinnerArmCandidate ABTestWinnerArm = "candidate"
)

// TrafficSplit 流量分配（baseline vs candidate）
//
// 实现 driver.Valuer / sql.Scanner 让 GORM 透明序列化到 JSONB
type TrafficSplit struct {
	Baseline  float64 `json:"baseline"`
	Candidate float64 `json:"candidate"`
}

// Value 序列化到 JSONB
func (t TrafficSplit) Value() (driver.Value, error) {
	return json.Marshal(t)
}

// Scan 从 JSONB 反序列化
func (t *TrafficSplit) Scan(src any) error {
	if src == nil {
		*t = TrafficSplit{Baseline: 0.5, Candidate: 0.5}
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.New("TrafficSplit.Scan: unsupported type")
	}
	return json.Unmarshal(data, t)
}

// AssetBundleABTest 资产包 A/B 实验实体
//
// 设计要点：
//   - experiment_id 全局唯一（与 BanditAllocator 的 experimentID 对齐）
//   - baseline_asset_id 是当前 active 资产包
//   - candidate_id 关联 asset_bundle_candidates.candidate_id
//   - BanditAllocator 每 6 小时收敛检查，winner_arm 写入后 completed
type AssetBundleABTest struct {
	ID               uint64                  `gorm:"primaryKey;autoIncrement" json:"id"`
	ExperimentID     string                  `gorm:"type:varchar(64);uniqueIndex;not null" json:"experiment_id"`
	BaselineAssetID  string                  `gorm:"type:varchar(64);index;not null" json:"baseline_asset_id"`
	CandidateID      string                  `gorm:"type:varchar(64);index;not null" json:"candidate_id"`
	Scenario         string                  `gorm:"type:varchar(32);index;not null;default:''" json:"scenario"`
	TrafficSplit     TrafficSplit            `gorm:"type:jsonb;not null" json:"traffic_split"`
	Status           AssetBundleABTestStatus `gorm:"type:varchar(16);index;not null;default:'running'" json:"status"`
	WinnerArm        ABTestWinnerArm         `gorm:"type:varchar(16)" json:"winner_arm"`
	BaselineSamples  int                     `gorm:"not null;default:0" json:"baseline_samples"`
	CandidateSamples int                     `gorm:"not null;default:0" json:"candidate_samples"`
	BaselineReward   float64                 `gorm:"type:decimal(12,3);not null;default:0" json:"baseline_reward"`
	CandidateReward  float64                 `gorm:"type:decimal(12,3);not null;default:0" json:"candidate_reward"`
	StartedAt        time.Time               `gorm:"not null" json:"started_at"`
	ConvergedAt      *time.Time              `json:"converged_at"`
	CompletedAt      *time.Time              `json:"completed_at"`
	CreatedAt        time.Time               `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time               `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (AssetBundleABTest) TableName() string { return "asset_bundle_ab_tests" }

// ============================================================================
// KnowledgeChunk 扩展（在原 model/knowledge_workspace.go 中定义的基础上，
// 通过 GORM 字段映射追加 6 个质量字段）
// ============================================================================

// KnowledgeChunkQualityLabel 知识库语料质量标签
type KnowledgeChunkQualityLabel string

const (
	// QualityLabelNormal 正常语料（默认）
	QualityLabelNormal KnowledgeChunkQualityLabel = "normal"
	// QualityLabelChampion 销冠话术补录（aggregated_reward ≥ 1.5）
	QualityLabelChampion KnowledgeChunkQualityLabel = "champion"
	// QualityLabelLowQuality 低质语料（low_quality_hits ≥ 3）
	QualityLabelLowQuality KnowledgeChunkQualityLabel = "low_quality"
	// QualityLabelArchived 已归档（low_quality_hits ≥ 10，从召回池剔除）
	QualityLabelArchived KnowledgeChunkQualityLabel = "archived"
)

// KnowledgeChunkExt 知识库语料扩展字段（用于自我矫正）
//
// 注意：此处不重新定义 KnowledgeChunk，而是通过 GORM 字段更新机制，
// 在原有 KnowledgeChunk 上追加这些字段。为了避免破坏现有代码，
// 通过额外的 struct 暴露字段，Repository 层会用 raw SQL 操作这些字段。
//
// 字段含义：
//   - QualityScore      累计 reward（销冠补录 +reward，低质标记 -reward）
//   - QualityLabel      质量标签（normal/champion/low_quality/archived）
//   - LowQualityHits    低质命中次数（累计 3 → low_quality，10 → archived）
//   - ChampionHits      销冠命中次数（用于排序）
//   - SourceSessionIDs  来源会话 ID 列表（销冠补录时记录来源）
//   - LastRewardAt      最后一次奖励更新时间
type KnowledgeChunkExt struct {
	QualityScore     float64                    `gorm:"column:quality_score;not null;default:0"`
	QualityLabel     KnowledgeChunkQualityLabel `gorm:"column:quality_label;type:varchar(16);not null;default:'normal'"`
	LowQualityHits   int                        `gorm:"column:low_quality_hits;not null;default:0"`
	ChampionHits     int                        `gorm:"column:champion_hits;not null;default:0"`
	SourceSessionIDs pq.StringArray             `gorm:"column:source_session_ids;type:text[];not null;default:'{}'"`
	LastRewardAt     *time.Time                 `gorm:"column:last_reward_at"`
}

// TableName 同 KnowledgeChunk（仅用于 GORM 字段映射）
func (KnowledgeChunkExt) TableName() string { return "knowledge_chunks" }

// 注意：KnowledgeChunkExt 与 KnowledgeChunk 共享同一张表，
// Repository 层通过 Raw SQL 或 Select 指定字段操作扩展字段，
// 不直接用 GORM AutoMigrate（避免破坏现有索引）。

// ============================================================================
// SelfLearningSwitch 自我学习三位一体统一开关
// ============================================================================
//
// 设计依据：v1.1 §7.4 用户开启即全自动执行
// 三位一体：RAG + 资产包 + LLM 共享一个开关
// 三级自治：manual / supervised / autonomous
//
// 表行数固定为 1（单例），通过 id=1 强制约束
// 单例模式：private 部署场景下，全实例共享同一开关状态

// AutonomyLevel 自治等级
type AutonomyLevel string

const (
	// AutonomyLevelManual 仅采集，不自动执行（人工审核每个动作）
	AutonomyLevelManual AutonomyLevel = "manual"
	// AutonomyLevelSupervised 自动执行 + 人工审核关键决策（promote/rollback 需人工）
	AutonomyLevelSupervised AutonomyLevel = "supervised"
	// AutonomyLevelAutonomous 全自动执行（含 promote/rollback，仅告警通知）
	AutonomyLevelAutonomous AutonomyLevel = "autonomous"
)

// SelfLearningSwitch 自我学习统一开关（单例）
type SelfLearningSwitch struct {
	ID                      uint64        `gorm:"primaryKey" json:"id"` // 固定为 1
	AutonomyLevel           AutonomyLevel `gorm:"type:varchar(16);not null;default:'manual'" json:"autonomy_level"`
	EnableRAG               bool          `gorm:"not null;default:false" json:"enable_rag"`
	EnableAsset             bool          `gorm:"not null;default:false" json:"enable_asset"`
	EnableLLM               bool          `gorm:"not null;default:false" json:"enable_llm"`
	MaxDailyCorrections     int           `gorm:"not null;default:100" json:"max_daily_corrections"`
	MaxDailyPromotions      int           `gorm:"not null;default:5" json:"max_daily_promotions"`
	LowQualityThreshold     float64       `gorm:"type:decimal(8,3);not null;default:3.0" json:"low_quality_threshold"`
	ChampionRewardThreshold float64       `gorm:"type:decimal(8,3);not null;default:1.5" json:"champion_reward_threshold"`
	ABTestMinSamples        int           `gorm:"not null;default:100" json:"ab_test_min_samples"`
	CircuitBreakerThreshold float64       `gorm:"type:decimal(5,3);not null;default:0.300" json:"circuit_breaker_threshold"`
	CircuitBreakerWindowMin int           `gorm:"not null;default:30" json:"circuit_breaker_window_min"`
	// 运行时状态（不持久化计算字段，由 service 实时更新）
	CircuitOpen      bool       `gorm:"not null;default:false" json:"circuit_open"`
	TodayCorrections int        `gorm:"not null;default:0" json:"today_corrections"`
	TodayPromotions  int        `gorm:"not null;default:0" json:"today_promotions"`
	TodayResetAt     *time.Time `json:"today_reset_at"`
	LastTriggeredAt  *time.Time `json:"last_triggered_at"`
	UpdatedBy        uint       `json:"updated_by"`
	CreatedAt        time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (SelfLearningSwitch) TableName() string { return "self_learning_switch" }

// ============================================================================
// SelfSupervisionSignal 自我监督信号表
// ============================================================================
//
// 设计依据：v1.1 §7.2 5 维监督指标
// 同时覆盖 RAG 与资产包的 5 维监督指标
// 每个指标按 (target_type, metric_name, bucket_hour) 聚合

// SupervisionTargetType 监督目标类型
type SupervisionTargetType string

const (
	SupervisionTargetRAG    SupervisionTargetType = "rag"    // RAG 监督
	SupervisionTargetAsset  SupervisionTargetType = "asset"  // 资产包监督
	SupervisionTargetLLM    SupervisionTargetType = "llm"    // LLM 监督
	SupervisionTargetHybrid SupervisionTargetType = "hybrid" // 混合监督
)

// SupervisionMetricName 监督指标名常量
//
// 指标维度划分（v1.1 §7.2 三位一体监督）：
//
//	RAG 4 维：
//	  1. recall_precision     召回精度（基于反馈信号）
//	  2. recall_coverage      召回覆盖（关键词覆盖率）
//	  3. generation_fidelity  生成忠实度（LLM-as-Judge 采样）
//	  4. answer_relevance     答案相关性（LLM-as-Judge 采样）
//	AssetBundle 5 维（资产包专属监督）：
//	  5. asset_effectiveness  资产包效能（综合反馈奖励）
//	  6. asset_adoption       资产包采纳率（销冠采用比例）
//	  7. asset_conversion     资产包转化率（转化成功比例）
//	  8. asset_complaint      资产包投诉率（负反馈比例，越低越好）
//	  9. asset_freshness      资产包新鲜度（最近使用时间衰减）
//	 10. asset_ab_converge    A/B 实验收敛度（实验健康度）
const (
	SupervisionMetricRecallPrecision    = "recall_precision"    // RAG 召回精度
	SupervisionMetricRecallCoverage     = "recall_coverage"     // RAG 召回覆盖
	SupervisionMetricGenerationFidelity = "generation_fidelity" // RAG 生成忠实度
	SupervisionMetricAnswerRelevance    = "answer_relevance"    // RAG 答案相关性
	SupervisionMetricAssetEffectiveness = "asset_effectiveness" // 资产包效能（综合反馈）
	SupervisionMetricAssetAdoption      = "asset_adoption"      // 资产包采纳率
	SupervisionMetricAssetConversion    = "asset_conversion"    // 资产包转化率
	SupervisionMetricAssetComplaint     = "asset_complaint"     // 资产包投诉率（越低越好）
	SupervisionMetricAssetFreshness     = "asset_freshness"     // 资产包新鲜度
	SupervisionMetricAssetABConverge    = "asset_ab_converge"   // A/B 实验收敛度
)

// SupervisionSignalStatus 监督信号状态
type SupervisionSignalStatus string

const (
	SupervisionStatusNormal  SupervisionSignalStatus = "normal"
	SupervisionStatusWarning SupervisionSignalStatus = "warning"
	SupervisionStatusAlert   SupervisionSignalStatus = "alert"
)

// SelfSupervisionSignal 自我监督信号实体
//
// 按 (target_type, metric_name, bucket_hour) 聚合
// bucket_hour: 按小时分桶，便于时序看板与告警
type SelfSupervisionSignal struct {
	ID          uint64                  `gorm:"primaryKey;autoIncrement" json:"id"`
	SignalID    string                  `gorm:"type:varchar(64);uniqueIndex;not null" json:"signal_id"`
	TargetType  SupervisionTargetType   `gorm:"type:varchar(16);index;not null" json:"target_type"`
	TargetID    string                  `gorm:"type:varchar(64);index;default:''" json:"target_id"` // asset_id / scenario / 空（全局）
	MetricName  string                  `gorm:"type:varchar(32);index;not null" json:"metric_name"`
	BucketHour  time.Time               `gorm:"type:timestamptz;index;not null" json:"bucket_hour"` // 按小时分桶
	Value       float64                 `gorm:"type:decimal(10,4);not null;default:0" json:"value"`
	Baseline    float64                 `gorm:"type:decimal(10,4);not null;default:0" json:"baseline"`
	Threshold   float64                 `gorm:"type:decimal(10,4);not null;default:0" json:"threshold"`
	SampleCount int64                   `gorm:"not null;default:0" json:"sample_count"`
	Status      SupervisionSignalStatus `gorm:"type:varchar(16);index;not null;default:'normal'" json:"status"`
	TraceIDs    pq.StringArray          `gorm:"type:text[];not null;default:'{}'" json:"trace_ids"` // 触发 trace 列表（前 10）
	Detail      JSONMap                 `gorm:"type:jsonb;default:'{}'" json:"detail"`              // 详细分位数/P50/P95 等
	CreatedAt   time.Time               `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt   time.Time               `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (SelfSupervisionSignal) TableName() string { return "self_supervision_signals" }

// ============================================================================
// SelfCorrectionAction 自我矫正动作审计表
// ============================================================================
//
// 设计依据：v1.1 §7.3 失败矩阵驱动的 7 类修复策略
// 所有矫正动作落库审计，supervised 模式下需人工确认，
// autonomous 模式下自动执行（仍记录供回滚）

// CorrectionActionType 矫正动作类型
type CorrectionActionType string

const (
	CorrectionRetrieveRetry  CorrectionActionType = "retrieve_retry"        // 检索重试
	CorrectionQueryRewrite   CorrectionActionType = "query_rewrite"         // 查询改写
	CorrectionChunkArchive   CorrectionActionType = "chunk_archive"         // 语料归档
	CorrectionChampionUpsert CorrectionActionType = "chunk_champion_upsert" // 销冠补录
	CorrectionAssetPromote   CorrectionActionType = "asset_promote"         // 资产包晋升
	CorrectionAssetRollback  CorrectionActionType = "asset_rollback"        // 资产包回滚
	CorrectionLLMCorrection  CorrectionActionType = "llm_correction"        // LLM 自我矫正
)

// CorrectionActionStatus 矫正动作状态
type CorrectionActionStatus string

const (
	CorrectionStatusPending    CorrectionActionStatus = "pending"     // 待执行（supervised 模式待人工确认）
	CorrectionStatusApplied    CorrectionActionStatus = "applied"     // 已执行
	CorrectionStatusRolledBack CorrectionActionStatus = "rolled_back" // 已回滚
	CorrectionStatusFailed     CorrectionActionStatus = "failed"      // 执行失败
	CorrectionStatusSkipped    CorrectionActionStatus = "skipped"     // 跳过（护栏拦截）
)

// SelfCorrectionAction 自我矫正动作实体
type SelfCorrectionAction struct {
	ID            uint64                 `gorm:"primaryKey;autoIncrement" json:"id"`
	ActionID      string                 `gorm:"type:varchar(64);uniqueIndex;not null" json:"action_id"`
	TriggerLogID  string                 `gorm:"type:varchar(64);index" json:"trigger_log_id"` // self_learning_logs.log_id
	ActionType    CorrectionActionType   `gorm:"type:varchar(32);index;not null" json:"action_type"`
	Scenario      string                 `gorm:"type:varchar(32);index" json:"scenario"`
	TargetType    string                 `gorm:"type:varchar(16);index" json:"target_type"` // rag_chunk/asset_bundle/llm_reply
	TargetID      string                 `gorm:"type:varchar(64);index" json:"target_id"`
	Before        JSONMap                `gorm:"type:jsonb;default:'{}'" json:"before"`
	After         JSONMap                `gorm:"type:jsonb;default:'{}'" json:"after"`
	AutonomyLevel AutonomyLevel          `gorm:"type:varchar(16)" json:"autonomy_level"`
	Operator      string                 `gorm:"type:varchar(64);not null;default:'auto'" json:"operator"` // auto / 用户名
	OperatorID    uint                   `json:"operator_id"`
	Reason        string                 `gorm:"type:text" json:"reason"`
	Status        CorrectionActionStatus `gorm:"type:varchar(16);index;not null;default:'pending'" json:"status"`
	AppliedAt     *time.Time             `json:"applied_at"`
	RolledBackAt  *time.Time             `json:"rolled_back_at"`
	ErrorMsg      string                 `gorm:"type:text" json:"error_msg"`
	CreatedAt     time.Time              `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt     time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (SelfCorrectionAction) TableName() string { return "self_correction_actions" }
