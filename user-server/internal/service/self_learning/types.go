package selflearning

// types.go 对话驱动自我学习三位一体机制 - 类型与接口定义
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1)
//
// 私域独立部署: 无 merchant_id 字段
//
// 本包是知识库(RAG) + 资产包(AssetBundle) + LLM 三位一体的自我学习机制
// 用户开启即全自动执行（v1.1 §7.4）
//
// 本包包含 7 个组件：
//   P0 核心服务（v1.1 §2-§6）:
//     1. SwitchService         - 三位一体统一开关（三级自治 + 6 道护栏 + 熔断）
//     2. DialogueEventPublisher - 对话事件发布（dialogue.started/ended）
//     3. RAGSelfCorrector      - RAG 自我矫正（预热 + 反思 + 低质标记 + 销冠补录）
//     4. AssetBundleLearner    - 资产包自我学习（候选生成 + 聚类升级 + 降级）
//     5. Orchestrator          - 主调度器（订阅事件 + 协程调度 + 信号量 + 幂等）
//   P1 三位一体扩展（v1.1 §7）:
//     6. RAGSelfSupervisor     - RAG 5 维监督指标 + LLM-as-Judge 采样
//     7. SelfCorrectionDispatcher - 失败矩阵派发 7 类修复策略
//
// 依赖抽象（依赖倒置，便于单元测试）：
//   - DB: 通过 Repository 接口注入
//   - LLM: LLMDispatcher 接口
//   - Embedding: Embedder 接口
//   - EventBus: EventBus 接口
//   - RAGEngine: RAGEngine 接口（用于 Warmup）

import (
	"context"
	"errors"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
)

// ============================================================================
// 抽象接口（依赖倒置）
// ============================================================================

// LLMDispatcher LLM 调度器接口
//
// 抽象 llm.Dispatcher，使本包不直接依赖 llm 包
type LLMDispatcher interface {
	// Dispatch 发送调度请求
	Dispatch(ctx context.Context, scenario string, prompt, systemPrompt string, jsonMode bool, maxTokens int) (content, model string, err error)
}

// Embedder 文本向量化接口
type Embedder interface {
	Embed(text string) []float32
	Dimension() int
}

// EventBus 事件总线接口
type EventBus interface {
	Publish(topic string, payload any) error
	Subscribe(topic string, handler any) error
}

// RAGEngine RAG 引擎接口（用于预热）
//
// 抽象 rag/core.RAGEngine，避免循环依赖
type RAGEngine interface {
	// Warmup 预热：缓存 Top-K 召回结果
	Warmup(ctx context.Context, sessionID, query string, ttl time.Duration) error
	// Retrieve 召回（供 reflect 阶段重用）
	Retrieve(ctx context.Context, query string, topK int) (chunks []RAGChunk, err error)
}

// RAGChunk RAG 召回的语料块
type RAGChunk struct {
	ChunkID      uint64
	DocumentID   uint64
	Content      string
	Score        float64
	QualityLabel model.KnowledgeChunkQualityLabel // 从 KnowledgeChunkExt 联表
}

// ChampionAnalyzer 销冠对话分析器接口（复用 feedback_loop 现有实现）
type ChampionAnalyzer interface {
	// AnalyzePipeline 分析销冠对话并提取话术
	AnalyzePipeline(ctx context.Context, since time.Time) (scripts []ExtractedScript, err error)
}

// ExtractedScript 提取的话术（来自 ChampionAnalyzer）
type ExtractedScript struct {
	Title              string
	Content            string
	Scenario           string
	TriggerKeywords    []string
	JourneyStage       string
	EffectivenessScore float64
	ClusterID          uint
}

// BanditAllocator Bandit 分配器接口（复用 feedback_loop 现有实现）
type BanditAllocator interface {
	// SelectArm 选择 arm
	SelectArm(ctx context.Context, experimentID string) (armKey string, err error)
	// UpdateReward 更新奖励
	UpdateReward(ctx context.Context, experimentID, armKey string, reward float64) error
	// CheckConvergence 检查收敛
	CheckConvergence(ctx context.Context, experimentID string) (converged bool, winnerArm string, posteriorProb float64, err error)
}

// AssetBundleRepository 资产包仓储接口（抽象 repository.AssetBundleRepository）
//
// 注意：与 repository.AssetBundleRepository 完全兼容，可直接传入
type AssetBundleRepository interface {
	// FindByAssetID 按 asset_id 查询资产包
	FindByAssetID(ctx context.Context, assetID string) (*model.AssetBundle, error)
	// Update 更新资产包（用于 status 变更等）
	Update(ctx context.Context, m *model.AssetBundle) error
	// ListActive 列出 active 资产包
	ListActive(ctx context.Context, limit int) ([]*model.AssetBundle, error)
	// IncrementUseCount 累加使用次数
	IncrementUseCount(ctx context.Context, assetID string) error
}

// FeedbackSignalRepository 反馈信号仓储接口（用于读取会话聚合奖励）
type FeedbackSignalRepository interface {
	GetBySessionID(ctx context.Context, sessionID string) (*model.FeedbackSignal, error)
	ListChampionSince(ctx context.Context, since time.Time, limit int) ([]*model.FeedbackSignal, error)
}

// ============================================================================
// 统一返回类型
// ============================================================================

// CorrectionResult 矫正结果
type CorrectionResult struct {
	ActionID    string                       `json:"action_id"`
	ActionType  model.CorrectionActionType   `json:"action_type"`
	TargetID    string                       `json:"target_id"`
	Status      model.CorrectionActionStatus `json:"status"` // applied / skipped / failed
	Reason      string                       `json:"reason"`
	Before      map[string]any               `json:"before"`
	After       map[string]any               `json:"after"`
	AppliedAt   time.Time                    `json:"applied_at"`
}

// CandidateGenerationResult 候选生成结果
type CandidateGenerationResult struct {
	CandidateID  string    `json:"candidate_id"`
	SourceCount  int       `json:"source_count"`
	RewardSum    float64   `json:"reward_sum"`
	ClusterCount int       `json:"cluster_count"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason,omitempty"`
	GeneratedAt  time.Time `json:"generated_at"`
}

// ConvergenceCheckResult 收敛检查结果
type ConvergenceCheckResult struct {
	ExperimentID string  `json:"experiment_id"`
	Converged    bool    `json:"converged"`
	WinnerArm    string  `json:"winner_arm"`
	PosteriorProb float64 `json:"posterior_prob"`
	TotalSamples int64   `json:"total_samples"`
	ShouldPromote bool   `json:"should_promote"` // autonomous 模式下是否自动晋升
}

// GuardrailCheckResult 护栏检查结果（v1.1 §7.4.5）
type GuardrailCheckResult struct {
	Passed            bool     `json:"passed"`
	BlockedReasons    []string `json:"blocked_reasons"`
	DailyQuotaUsed    int      `json:"daily_quota_used"`
	DailyQuotaLimit   int      `json:"daily_quota_limit"`
	CircuitOpen       bool     `json:"circuit_open"`
	AutonomyLevel     model.AutonomyLevel `json:"autonomy_level"`
}

// ============================================================================
// 统一开关决策门面（SwitchService 暴露给其他组件的接口）
// ============================================================================

// SwitchSnapshot 开关快照（只读视图，供其他组件并发读取）
//
// 其他组件（RAGSelfCorrector/AssetBundleLearner/Orchestrator）通过此快照决策：
//   - 是否执行（enable_rag/enable_asset/enable_llm）
//   - 执行等级（autonomy_level: manual/supervised/autonomous）
//   - 是否熔断中（circuit_open）
//   - 今日配额（today_corrections/today_promotions vs max_*）
type SwitchSnapshot struct {
	AutonomyLevel          model.AutonomyLevel
	EnableRAG              bool
	EnableAsset            bool
	EnableLLM              bool
	CircuitOpen            bool
	MaxDailyCorrections    int
	MaxDailyPromotions     int
	TodayCorrections       int
	TodayPromotions        int
	LowQualityThreshold    float64
	ChampionRewardThreshold float64
	ABTestMinSamples       int
	CircuitBreakerThreshold float64
	CircuitBreakerWindowMin int
	LastTriggeredAt        *time.Time
	UpdatedAt              time.Time
}

// ToDTO 转换为 DTO 响应
func (s *SwitchSnapshot) ToDTO() *dto.SwitchStatusResponse {
	if s == nil {
		return nil
	}
	return &dto.SwitchStatusResponse{
		AutonomyLevel:           s.AutonomyLevel,
		EnableRAG:               s.EnableRAG,
		EnableAsset:             s.EnableAsset,
		EnableLLM:               s.EnableLLM,
		MaxDailyCorrections:     s.MaxDailyCorrections,
		MaxDailyPromotions:      s.MaxDailyPromotions,
		LowQualityThreshold:     s.LowQualityThreshold,
		ChampionRewardThreshold: s.ChampionRewardThreshold,
		ABTestMinSamples:        s.ABTestMinSamples,
		CircuitBreakerThreshold: s.CircuitBreakerThreshold,
		CircuitBreakerWindowMin: s.CircuitBreakerWindowMin,
		CircuitOpen:             s.CircuitOpen,
		TodayCorrections:        s.TodayCorrections,
		TodayPromotions:         s.TodayPromotions,
		LastTriggeredAt:         s.LastTriggeredAt,
		UpdatedAt:               s.UpdatedAt,
	}
}

// ============================================================================
// 统一错误定义
// ============================================================================

var (
	ErrSwitchDisabled         = errors.New("self_learning: switch is disabled")
	ErrCircuitOpen            = errors.New("self_learning: circuit breaker is open")
	ErrDailyQuotaExceeded     = errors.New("self_learning: daily quota exceeded")
	ErrAutonomyLevelForbidden = errors.New("self_learning: action forbidden under current autonomy level")
	ErrRAGEngineNil           = errors.New("self_learning: rag engine is nil")
	ErrChampionAnalyzerNil    = errors.New("self_learning: champion analyzer is nil")
	ErrBanditAllocatorNil     = errors.New("self_learning: bandit allocator is nil")
	ErrLLMDispatcherNil       = errors.New("self_learning: llm dispatcher is nil")
	ErrEmbedderNil            = errors.New("self_learning: embedder is nil")
	ErrEventBusNil            = errors.New("self_learning: event bus is nil")
	ErrOrchestratorNotRunning = errors.New("self_learning: orchestrator is not running")
)
