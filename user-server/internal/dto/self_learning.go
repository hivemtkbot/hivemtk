package dto

// self_learning.go 对话驱动自我学习三位一体机制 DTO
//
// 五层架构归属: L2 网关 / L3 业务 之间的传输层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1)
//
// 私域独立部署: 无 merchant_id 字段
//
// 本文件定义 self_learning 包对外暴露的 DTO，按职责分组：
//   1. 开关控制           - SwitchConfigRequest / SwitchStatusResponse
//   2. 日志查询           - SelfLearningLogListRequest / SelfLearningLogResponse
//   3. 候选管理           - AssetBundleCandidateResponse / CandidateListRequest
//   4. A/B 实验管理       - ABTestListRequest / ABTestResponse
//   5. 监督指标看板       - SupervisionDashboardResponse / SupervisionMetricItem
//   6. 矫正动作审计       - CorrectionActionItem / CorrectionAuditResponse
//
// 设计原则：
//   - 严禁 DTO 反向引用 Service（五层架构约束 §七）
//   - 校验逻辑下沉至 service 层（五层架构规范 §七：DTO 层禁止含业务逻辑）
//   - 响应 DTO 与 Model 解耦，避免 GORM 字段直接外泄

import (
	"errors"
	"time"

	"marketing/internal/model"
)

// ============================================================================
// 1. 开关控制 DTO（用户开启即全自动执行 - v1.1 §7.4）
// ============================================================================

// SwitchConfigRequest 自我学习开关配置请求
//
// 三位一体统一开关：知识库(RAG) + 资产包(AssetBundle) 共享一个开关
// 三级自治等级：
//   - manual      仅采集，不自动执行（人工审核每个动作）
//   - supervised  自动执行 + 人工审核关键决策（promote/rollback 需人工）
//   - autonomous  全自动执行（含 promote/rollback，仅告警通知）
type SwitchConfigRequest struct {
	AutonomyLevel  model.AutonomyLevel `json:"autonomy_level" binding:"required"` // manual/supervised/autonomous
	EnableRAG      bool                `json:"enable_rag"`                       // 是否启用 RAG 自我学习
	EnableAsset    bool                `json:"enable_asset"`                     // 是否启用资产包自我学习
	EnableLLM      bool                `json:"enable_llm"`                       // 是否启用 LLM 自我矫正
	// 安全护栏参数（v1.1 §7.4.5）
	MaxDailyCorrections    int     `json:"max_daily_corrections"`    // 每日最大矫正动作数（默认 100）
	MaxDailyPromotions     int     `json:"max_daily_promotions"`     // 每日最大资产包晋升数（默认 5）
	LowQualityThreshold    float64 `json:"low_quality_threshold"`    // 低质语料阈值（默认 3.0）
	ChampionRewardThreshold float64 `json:"champion_reward_threshold"` // 销冠补录阈值（默认 1.5）
	ABTestMinSamples       int     `json:"ab_test_min_samples"`      // A/B 最小样本数（默认 100）
	// 熔断参数
	CircuitBreakerThreshold float64 `json:"circuit_breaker_threshold"` // 失败率熔断阈值（默认 0.3）
	CircuitBreakerWindowMin int     `json:"circuit_breaker_window_min"` // 统计窗口（分钟，默认 30）
}

// SwitchStatusResponse 开关状态响应
type SwitchStatusResponse struct {
	AutonomyLevel          model.AutonomyLevel `json:"autonomy_level"`
	EnableRAG              bool                `json:"enable_rag"`
	EnableAsset            bool                `json:"enable_asset"`
	EnableLLM              bool                `json:"enable_llm"`
	MaxDailyCorrections    int                 `json:"max_daily_corrections"`
	MaxDailyPromotions     int                 `json:"max_daily_promotions"`
	LowQualityThreshold    float64             `json:"low_quality_threshold"`
	ChampionRewardThreshold float64            `json:"champion_reward_threshold"`
	ABTestMinSamples       int                 `json:"ab_test_min_samples"`
	CircuitBreakerThreshold float64            `json:"circuit_breaker_threshold"`
	CircuitBreakerWindowMin int                `json:"circuit_breaker_window_min"`
	// 运行时状态
	CircuitOpen         bool  `json:"circuit_open"`          // 当前是否熔断中
	TodayCorrections    int   `json:"today_corrections"`     // 今日已执行矫正数
	TodayPromotions     int   `json:"today_promotions"`      // 今日已晋升资产包数
	LastTriggeredAt     *time.Time `json:"last_triggered_at"` // 最后一次触发时间
	UpdatedAt           time.Time `json:"updated_at"`
}

// ============================================================================
// 2. 日志查询 DTO
// ============================================================================

// SelfLearningLogListRequest 自我学习日志列表查询请求
type SelfLearningLogListRequest struct {
	Scenario     model.SelfLearningScenario `json:"scenario" form:"scenario"`
	Status       model.SelfLearningStatus   `json:"status" form:"status"`
	TriggerEvent string                     `json:"trigger_event" form:"trigger_event"`
	SessionID    string                     `json:"session_id" form:"session_id"`
	TraceID      string                     `json:"trace_id" form:"trace_id"`
	Since        time.Time                  `json:"since" form:"since"`
	Until        time.Time                  `json:"until" form:"until"`
	Page         int                        `json:"page" form:"page"`
	Size         int                        `json:"size" form:"size"`
}

// SelfLearningLogResponse 自我学习日志响应
type SelfLearningLogResponse struct {
	LogID         string                        `json:"log_id"`
	SessionID     string                        `json:"session_id"`
	TraceID       string                        `json:"trace_id"`
	Scenario      model.SelfLearningScenario    `json:"scenario"`
	TriggerEvent  model.SelfLearningTriggerEvent `json:"trigger_event"`
	Status        model.SelfLearningStatus      `json:"status"`
	InputSummary  map[string]any                `json:"input_summary"`
	OutputSummary map[string]any                `json:"output_summary"`
	ErrorMsg      string                        `json:"error_msg"`
	DurationMs    int64                         `json:"duration_ms"`
	StartedAt     time.Time                     `json:"started_at"`
	FinishedAt    *time.Time                    `json:"finished_at"`
	CreatedAt     time.Time                     `json:"created_at"`
}

// SelfLearningLogListResponse 自我学习日志列表响应
type SelfLearningLogListResponse struct {
	List  []*SelfLearningLogResponse `json:"list"`
	Total int64                      `json:"total"`
	Page  int                        `json:"page"`
	Size  int                        `json:"size"`
}

// SelfLearningDashboardResponse 自我学习看板响应
//
// 用于看板首页：今日执行统计 + 失败率 + 熔断状态
type SelfLearningDashboardResponse struct {
	TodayCount        map[model.SelfLearningStatus]int64 `json:"today_count"`
	TodayTotal        int64                              `json:"today_total"`
	TodaySuccess      int64                              `json:"today_success"`
	TodayFailed       int64                              `json:"today_failed"`
	SuccessRate       float64                            `json:"success_rate"`
	FailedRate        float64                            `json:"failed_rate"`
	Switch            *SwitchStatusResponse              `json:"switch"`
	RecentFailedLogs  []*SelfLearningLogResponse         `json:"recent_failed_logs"`
	RecentChampionOps []*CorrectionActionItem            `json:"recent_champion_ops"`
	UpdatedAt         time.Time                          `json:"updated_at"`
}

// ============================================================================
// 3. 候选管理 DTO
// ============================================================================

// CandidateListRequest 资产包候选列表查询请求
type CandidateListRequest struct {
	Scenario  string `json:"scenario" form:"scenario"`
	Status    string `json:"status" form:"status"`
	Industry  string `json:"industry" form:"industry"`
	Language  string `json:"language" form:"language"`
	Since     time.Time `json:"since" form:"since"`
	Page      int    `json:"page" form:"page"`
	Size      int    `json:"size" form:"size"`
}

// AssetBundleCandidateResponse 资产包候选响应
type AssetBundleCandidateResponse struct {
	CandidateID      string                        `json:"candidate_id"`
	SourceSessionIDs []string                      `json:"source_session_ids"`
	ExtractedScripts map[string]any                `json:"extracted_scripts"`
	ProposedMessages []model.AssetBundleMessage    `json:"proposed_messages"`
	Industry         string                        `json:"industry"`
	Language         string                        `json:"language"`
	Scenario         string                        `json:"scenario"`
	ClusterCount     int                           `json:"cluster_count"`
	RewardSum        float64                       `json:"reward_sum"`
	Status           model.AssetBundleCandidateStatus `json:"status"`
	ABTestID         string                        `json:"ab_test_id"`
	PromotedAssetID  string                        `json:"promoted_asset_id"`
	CreatedAt        time.Time                     `json:"created_at"`
	UpdatedAt        time.Time                     `json:"updated_at"`
}

// CandidateListResponse 候选列表响应
type CandidateListResponse struct {
	List  []*AssetBundleCandidateResponse `json:"list"`
	Total int64                           `json:"total"`
	Page  int                             `json:"page"`
	Size  int                             `json:"size"`
}

// ============================================================================
// 4. A/B 实验管理 DTO
// ============================================================================

// ABTestListRequest A/B 实验列表查询请求
type ABTestListRequest struct {
	Scenario       string `json:"scenario" form:"scenario"`
	Status         string `json:"status" form:"status"`
	BaselineAssetID string `json:"baseline_asset_id" form:"baseline_asset_id"`
	Page           int    `json:"page" form:"page"`
	Size           int    `json:"size" form:"size"`
}

// ABTestResponse A/B 实验响应
type ABTestResponse struct {
	ExperimentID     string                       `json:"experiment_id"`
	BaselineAssetID  string                       `json:"baseline_asset_id"`
	CandidateID      string                       `json:"candidate_id"`
	Scenario         string                       `json:"scenario"`
	TrafficSplit     model.TrafficSplit           `json:"traffic_split"`
	Status           model.AssetBundleABTestStatus `json:"status"`
	WinnerArm        model.ABTestWinnerArm        `json:"winner_arm"`
	BaselineSamples  int                          `json:"baseline_samples"`
	CandidateSamples int                          `json:"candidate_samples"`
	BaselineReward   float64                      `json:"baseline_reward"`
	CandidateReward  float64                      `json:"candidate_reward"`
	StartedAt        time.Time                    `json:"started_at"`
	ConvergedAt      *time.Time                   `json:"converged_at"`
	CompletedAt      *time.Time                   `json:"completed_at"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
}

// ABTestListResponse A/B 实验列表响应
type ABTestListResponse struct {
	List  []*ABTestResponse `json:"list"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
}

// ABTestPromoteRequest A/B 实验晋升请求（supervised 模式下人工确认）
type ABTestPromoteRequest struct {
	ExperimentID string `json:"experiment_id" binding:"required"`
	WinnerArm    string `json:"winner_arm" binding:"required"` // baseline / candidate
	OperatorID   uint   `json:"operator_id"`
	Note         string `json:"note"`
}

// ============================================================================
// 5. 监督指标看板 DTO（v1.1 §7.2 5 维监督指标）
// ============================================================================

// SupervisionDashboardResponse 监督指标看板响应
//
// 5 维监督指标同时覆盖 RAG 与资产包（v1.1 §7.2）：
//   RAG:
//     1. recall_precision  召回精度
//     2. recall_coverage   召回覆盖
//     3. generation_fidelity 生成忠实度
//     4. answer_relevance  答案相关性
//   AssetBundle:
//     5. asset_effectiveness 资产包效能
type SupervisionDashboardResponse struct {
	Range       string                  `json:"range"`        // 24h / 7d / 30d
	From        time.Time               `json:"from"`
	To          time.Time               `json:"to"`
	RAGMetrics  []*SupervisionMetricItem `json:"rag_metrics"`
	AssetMetrics []*SupervisionMetricItem `json:"asset_metrics"`
	Alerts      []*SupervisionAlertItem  `json:"alerts"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

// SupervisionMetricItem 单个监督指标项
type SupervisionMetricItem struct {
	Name         string  `json:"name"`          // recall_precision/recall_coverage/...
	DisplayName  string  `json:"display_name"`  // 中文展示名
	Value        float64 `json:"value"`         // 当前值
	Threshold    float64 `json:"threshold"`     // 告警阈值
	Baseline     float64 `json:"baseline"`      // 基线值
	Trend        string  `json:"trend"`         // up/down/stable
	SampleCount  int64   `json:"sample_count"`  // 样本数
	IsAlert      bool    `json:"is_alert"`      // 是否告警中
	LastSampleAt time.Time `json:"last_sample_at"`
}

// SupervisionAlertItem 监督告警项
type SupervisionAlertItem struct {
	AlertID     string    `json:"alert_id"`
	MetricName  string    `json:"metric_name"`
	Severity    string    `json:"severity"`      // info/warning/critical
	Message     string    `json:"message"`
	TriggeredAt time.Time `json:"triggered_at"`
	ResolvedAt  *time.Time `json:"resolved_at"`
}

// ============================================================================
// 5.1 资产包专属监督看板 DTO（v1.1 §7.2 资产包 5 维监督扩展）
// ============================================================================

// AssetSupervisionDashboardResponse 资产包专属监督看板响应
//
// 与 SupervisionDashboardResponse 区别：
//   - SupervisionDashboardResponse 同时包含 RAG 与资产包指标（综合看板）
//   - AssetSupervisionDashboardResponse 仅包含资产包 5 维专属指标 + A/B 实验汇总
type AssetSupervisionDashboardResponse struct {
	Range         string                    `json:"range"`            // 24h / 7d / 30d
	From          time.Time                 `json:"from"`
	To            time.Time                 `json:"to"`
	AssetMetrics  []*SupervisionMetricItem  `json:"asset_metrics"`    // 5 维资产包指标
	Alerts        []*SupervisionAlertItem   `json:"alerts"`           // 资产包告警列表
	ABTestSummary *AssetABTestSummary       `json:"ab_test_summary"`  // A/B 实验状态汇总
	UpdatedAt     time.Time                 `json:"updated_at"`
}

// AssetABTestSummary 资产包 A/B 实验状态汇总
type AssetABTestSummary struct {
	TotalCount      int64   `json:"total_count"`
	RunningCount    int64   `json:"running_count"`     // 进行中
	ConvergedCount  int64   `json:"converged_count"`   // 已收敛待处理
	CompletedCount  int64   `json:"completed_count"`   // 已完成（candidate 胜出）
	RolledBackCount int64   `json:"rolled_back_count"` // 已回滚（baseline 胜出）
	ConvergeRate    float64 `json:"converge_rate"`     // 收敛完成率 = (converged + completed) / total
}

// ============================================================================
// 6. 矫正动作审计 DTO（v1.1 §7.3 失败矩阵驱动的 7 类修复）
// ============================================================================

// CorrectionActionItem 矫正动作项
//
// 7 类修复策略：
//   1. retrieve_retry       检索重试（扩大 Top-K）
//   2. query_rewrite        查询改写（LLM 改写）
//   3. chunk_archive        语料归档（低质标记 ≥10）
//   4. chunk_champion_upsert 销冠补录（reward ≥ 1.5）
//   5. asset_promote        资产包晋升（A/B 胜出）
//   6. asset_rollback       资产包回滚（A/B 失败）
//   7. llm_correction       LLM 自我矫正（幻觉/跑题）
type CorrectionActionItem struct {
	ActionID      string    `json:"action_id"`
	ActionType    string    `json:"action_type"`     // 7 类之一
	Scenario      string    `json:"scenario"`        // 触发场景
	TriggerLogID  string    `json:"trigger_log_id"`  // 触发的 self_learning_logs.log_id
	TargetType    string    `json:"target_type"`     // rag_chunk / asset_bundle / llm_reply
	TargetID      string    `json:"target_id"`       // 目标 ID（chunk_id / asset_id / reply_id）
	Before        map[string]any `json:"before"`     // 变更前状态
	After         map[string]any `json:"after"`      // 变更后状态
	AutonomyLevel model.AutonomyLevel `json:"autonomy_level"` // 执行时的自治等级
	Operator      string    `json:"operator"`        // auto / 用户名
	Reason        string    `json:"reason"`          // 矫正原因
	Status        string    `json:"status"`          // pending/applied/rolled_back/failed
	AppliedAt     *time.Time `json:"applied_at"`
	RolledBackAt  *time.Time `json:"rolled_back_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// CorrectionAuditResponse 矫正动作审计响应
type CorrectionAuditResponse struct {
	List  []*CorrectionActionItem `json:"list"`
	Total int64                   `json:"total"`
	Page  int                     `json:"page"`
	Size  int                     `json:"size"`
}

// CorrectionListRequest 矫正动作列表查询请求
type CorrectionListRequest struct {
	ActionType string `json:"action_type" form:"action_type"`
	TargetType string `json:"target_type" form:"target_type"`
	Status     string `json:"status" form:"status"`
	Since      time.Time `json:"since" form:"since"`
	Until      time.Time `json:"until" form:"until"`
	Page       int    `json:"page" form:"page"`
	Size       int    `json:"size" form:"size"`
}

// CorrectionRollbackRequest 矫正动作回滚请求（人工撤销自动矫正）
type CorrectionRollbackRequest struct {
	ActionID  string `json:"action_id" binding:"required"`
	OperatorID uint  `json:"operator_id"`
	Reason    string `json:"reason"`
}

// ============================================================================
// 错误定义
// ============================================================================

var (
	ErrSelfLearningRequestNil         = errors.New("self_learning: request is nil")
	ErrInvalidAutonomyLevel           = errors.New("self_learning: invalid autonomy_level, must be manual/supervised/autonomous")
	ErrInvalidMaxDailyCorrections     = errors.New("self_learning: max_daily_corrections must be >= 0")
	ErrInvalidMaxDailyPromotions      = errors.New("self_learning: max_daily_promotions must be >= 0")
	ErrInvalidLowQualityThreshold     = errors.New("self_learning: low_quality_threshold must be >= 0")
	ErrInvalidChampionRewardThreshold = errors.New("self_learning: champion_reward_threshold must be >= 0")
	ErrInvalidABTestMinSamples        = errors.New("self_learning: ab_test_min_samples must be >= 0")
	ErrInvalidCircuitBreakerThreshold = errors.New("self_learning: circuit_breaker_threshold must be in [0,1]")
	ErrExperimentIDEmpty              = errors.New("self_learning: experiment_id is empty")
	ErrInvalidWinnerArm               = errors.New("self_learning: winner_arm must be baseline or candidate")
	ErrActionIDEmpty                  = errors.New("self_learning: action_id is empty")
)
