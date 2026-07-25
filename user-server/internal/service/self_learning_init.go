package service

// self_learning_init.go 对话驱动自我学习三位一体机制装配入口
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1)
//
// 职责：
//   1. 构造 SwitchService（三位一体统一开关 + 6 道护栏 + 熔断器）
//   2. 构造 DialogueEventPublisher（对话事件发布器）
//   3. 构造 RAGSelfCorrector（RAG 自我矫正器：预热 + 反思 + 低质标记 + 销冠补录）
//   4. 构造 AssetBundleLearner（资产包自我学习器：候选生成 + 聚类升级 + 降级）
//   5. 构造 LLMSelfCorrector（LLM 自我矫正器：幻觉 + 跑题矫正）
//   6. 构造 SelfCorrectionDispatcher（失败矩阵派发 7 类修复策略）
//   7. 构造 RAGSelfSupervisor（RAG 5 维监督指标 + LLM-as-Judge 采样）
//   8. 构造 AssetBundleSelfSupervisor（资产包 5 维专属监督指标）
//   9. 构造 Orchestrator（主调度器：订阅事件 + 协程调度 + 信号量 + 幂等）
//
// 私域独立部署: 无 merchant_id 字段
//
// 与 feedback_loop 的关系：
//   - feedback_loop 提供 FeedbackCollector / ChampionAnalyzer / BanditAllocator
//   - self_learning 复用 feedback_loop 的 ChampionAnalyzer 和 BanditAllocator
//   - 通过 adapter 接口解耦，避免 import cycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/aiagent/embedding"
	"marketing/internal/aiagent/llm"
	"marketing/internal/event"
	"marketing/internal/repository"
	feedbackloop "marketing/internal/service/feedback_loop"
	selflearning "marketing/internal/service/self_learning"
)

// 全局单例
var (
	selfLearningOnce sync.Once
	selfLearningInst *SelfLearningComponents
)

// selfLearningLLMAdapter 把 llm.Dispatcher 适配为 selflearning.LLMDispatcher 接口
type selfLearningLLMAdapter struct {
	dispatcher *llm.Dispatcher
}

// Dispatch 适配 selflearning.LLMDispatcher
func (a *selfLearningLLMAdapter) Dispatch(ctx context.Context, scenario string, prompt, systemPrompt string, jsonMode bool, maxTokens int) (string, string, error) {
	if a.dispatcher == nil {
		return "", "", nil
	}
	result, err := a.dispatcher.Dispatch(ctx, llm.DispatchRequest{
		Scenario:     llm.DispatchScenario(scenario),
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
		MaxTokens:    maxTokens,
	})
	if err != nil {
		return "", "", err
	}
	return result.Content, result.Model, nil
}

// selfLearningEmbedderAdapter 把 embedding.LocalEmbedding 适配为 selflearning.Embedder 接口
type selfLearningEmbedderAdapter struct {
	embedder *embedding.LocalEmbedding
}

// Embed 单条文本向量化
func (a *selfLearningEmbedderAdapter) Embed(text string) []float32 {
	if a.embedder == nil {
		return make([]float32, 1024)
	}
	return a.embedder.Embed(text)
}

// Dimension 返回向量维度
func (a *selfLearningEmbedderAdapter) Dimension() int {
	if a.embedder == nil {
		return 1024
	}
	return a.embedder.Dimension()
}

// eventBusAdapter 把 *event.EventBus 适配为 selflearning.EventBus 接口
//
// event.EventBus 签名：
//   Publish(evt Event)
//   Subscribe(topic string, handler Handler)
//
// selflearning.EventBus 接口签名：
//   Publish(topic string, payload any) error
//   Subscribe(topic string, handler any) error
//
// 由于事件总线是 best-effort 投递，Publish 失败仅记录日志不返回错误
// 由于 handler 类型不同（event.Handler vs any），Subscribe 需要类型断言转换
type eventBusAdapter struct {
	bus *event.EventBus
}

// Publish 适配：将 (topic, payload) 包装为 event.Event 后发布
func (a *eventBusAdapter) Publish(topic string, payload any) error {
	if a.bus == nil {
		return nil
	}
	a.bus.Publish(event.Event{
		Topic:   topic,
		Payload: payload,
	})
	return nil
}

// Subscribe 适配：将 any 类型的 handler 转换为 event.Handler
//
// 支持的 handler 类型：
//   - func(payload *event.DialogueStartedPayload)
//   - func(payload *event.DialogueEndedPayload)
//   - func(payload *event.AssetDegradedPayload)
//   - event.Handler (func(evt Event) error)
//
// P1-2：新增 default 分支，未匹配的 handler 类型显式返回错误
// 避免调用方误以为订阅成功但实际无 handler 被注册的"静默失败"问题
func (a *eventBusAdapter) Subscribe(topic string, handler any) error {
	if a.bus == nil {
		return nil
	}
	// 尝试匹配已知的 handler 签名
	switch h := handler.(type) {
	case func(*event.DialogueStartedPayload):
		a.bus.Subscribe(topic, func(evt event.Event) error {
			if p, ok := evt.Payload.(*event.DialogueStartedPayload); ok {
				h(p)
			}
			return nil
		})
	case func(*event.DialogueEndedPayload):
		a.bus.Subscribe(topic, func(evt event.Event) error {
			if p, ok := evt.Payload.(*event.DialogueEndedPayload); ok {
				h(p)
			}
			return nil
		})
	case func(*event.AssetDegradedPayload):
		a.bus.Subscribe(topic, func(evt event.Event) error {
			if p, ok := evt.Payload.(*event.AssetDegradedPayload); ok {
				h(p)
			}
			return nil
		})
	case event.Handler:
		a.bus.Subscribe(topic, h)
	default:
		// P1-2：default 分支显式报错
		// 避免调用方误以为订阅成功但实际无 handler 被注册的"静默失败"问题
		// 错误信息包含 handler 的实际类型，便于定位调用方代码
		return fmt.Errorf("eventBusAdapter.Subscribe: unsupported handler type %T for topic %q (supported: func(*DialogueStartedPayload), func(*DialogueEndedPayload), func(*AssetDegradedPayload), event.Handler)", handler, topic)
	}
	return nil
}

// SelfLearningComponents 自我学习三位一体机制组件集合
type SelfLearningComponents struct {
	SwitchSvc          *selflearning.SwitchService
	Publisher          *selflearning.DialogueEventPublisher
	RAGCorrector       *selflearning.RAGSelfCorrector
	AssetLearner       *selflearning.AssetBundleLearner
	LLMCorrector       *selflearning.LLMSelfCorrector
	Dispatcher         *selflearning.SelfCorrectionDispatcher
	RAGSupervisor      *selflearning.RAGSelfSupervisor
	AssetSupervisor    *selflearning.AssetBundleSelfSupervisor
	Orchestrator       *selflearning.Orchestrator
	LLMAdapter         *selfLearningLLMAdapter
	EmbedAdapter       *selfLearningEmbedderAdapter
}

// InitSelfLearningComponents 装配自我学习三位一体机制所有组件
//
// 调用时机：main.go 启动时，DB / EventBus / LLM / Embedding 初始化完成后调用
// 返回：组件集合，包含 Orchestrator（需调用方调用 Orchestrator.Start 启动订阅）
//
// 依赖：
//   - db: 已初始化的 *gorm.DB
//   - bus: 进程内事件总线（*event.EventBus）
//   - dispatcher: LLM 调度器（可为 nil，禁用 LLM 相关功能）
//   - embedder: 文本向量化器（可为 nil，禁用 embedding 相关功能）
//   - feedbackComponents: 反馈学习闭环组件（复用 ChampionAnalyzer / BanditAllocator）
//   - assetBundleRepo: 资产包仓储（用于资产包监督与降级）
func InitSelfLearningComponents(
	db *gorm.DB,
	bus *event.EventBus,
	dispatcher *llm.Dispatcher,
	embedder *embedding.LocalEmbedding,
	feedbackComponents *FeedbackLoopComponents,
	assetBundleRepo repository.AssetBundleRepository,
) *SelfLearningComponents {
	selfLearningOnce.Do(func() {
		// 适配器
		llmAdapter := &selfLearningLLMAdapter{dispatcher: dispatcher}
		embAdapter := &selfLearningEmbedderAdapter{embedder: embedder}
		busAdapter := &eventBusAdapter{bus: bus}

		// 1. Repository 装配
		switchRepo := repository.NewSelfLearningSwitchRepository(db)
		signalRepo := repository.NewSelfSupervisionSignalRepository(db)
		actionRepo := repository.NewSelfCorrectionActionRepository(db)
		logRepo := repository.NewSelfLearningLogRepository(db)
		candidateRepo := repository.NewAssetBundleCandidateRepository(db)
		abTestRepo := repository.NewAssetBundleABTestRepository(db)
		chunkExtRepo := repository.NewKnowledgeChunkExtRepository(db)

		// 2. SwitchService（开关 + 护栏 + 熔断）
		// cacheExp=5s：开关状态内存缓存 5 秒，避免高频 DB 查询
		switchSvc := selflearning.NewSwitchService(switchRepo, logRepo, 5*time.Second)

		// 3. DialogueEventPublisher（事件发布器）
		publisher := selflearning.NewDialogueEventPublisher(busAdapter, nil, nil)

		// 4. RAGSelfCorrector（RAG 自我矫正器）
		ragCorrector := selflearning.NewRAGSelfCorrector(
			switchSvc, chunkExtRepo, logRepo, actionRepo, nil, publisher,
		)

		// 5. AssetBundleLearner（资产包自我学习器）
		var championAdapter selflearning.ChampionAnalyzer
		var banditAdapter selflearning.BanditAllocator
		if feedbackComponents != nil {
			championAdapter = &championAnalyzerShim{analyzer: feedbackComponents.Analyzer}
			banditAdapter = &banditAllocatorShim{bandit: feedbackComponents.Bandit}
		}
		assetLearner := selflearning.NewAssetBundleLearner(
			switchSvc, candidateRepo, abTestRepo, logRepo, actionRepo,
			assetBundleRepo, championAdapter, banditAdapter, publisher,
		)

		// 6. LLMSelfCorrector（LLM 自我矫正器）
		var llmDispatcher selflearning.LLMDispatcher
		if llmAdapter != nil {
			llmDispatcher = llmAdapter
		}
		llmCorrector := selflearning.NewLLMSelfCorrector(
			switchSvc, actionRepo, logRepo, llmDispatcher,
		)

		// 7. SelfCorrectionDispatcher（失败矩阵派发器）
		dispatcherObj := selflearning.NewSelfCorrectionDispatcher(
			switchSvc, actionRepo, logRepo, signalRepo,
			ragCorrector, assetLearner, llmCorrector,
		)

		// 8. RAGSelfSupervisor（RAG 5 维监督器）
		ragSupervisor := selflearning.NewRAGSelfSupervisor(
			switchSvc, signalRepo, logRepo, dispatcherObj, llmDispatcher,
		)

		// 9. AssetBundleSelfSupervisor（资产包 5 维专属监督器）
		assetSupervisor := selflearning.NewAssetBundleSelfSupervisor(
			switchSvc, signalRepo, logRepo, actionRepo, abTestRepo,
			assetBundleRepo, dispatcherObj,
		)

		// 10. Orchestrator（主调度器）
		orchestrator := selflearning.NewOrchestrator(
			switchSvc, ragCorrector, assetLearner, publisher, busAdapter, 50,
			ragSupervisor, assetSupervisor,
		)

		selfLearningInst = &SelfLearningComponents{
			SwitchSvc:       switchSvc,
			Publisher:       publisher,
			RAGCorrector:    ragCorrector,
			AssetLearner:    assetLearner,
			LLMCorrector:    llmCorrector,
			Dispatcher:      dispatcherObj,
			RAGSupervisor:   ragSupervisor,
			AssetSupervisor: assetSupervisor,
			Orchestrator:    orchestrator,
			LLMAdapter:      llmAdapter,
			EmbedAdapter:    embAdapter,
		}
	})
	return selfLearningInst
}

// GetSelfLearningComponents 获取全局自我学习组件实例
func GetSelfLearningComponents() *SelfLearningComponents {
	return selfLearningInst
}

// ============================================================================
// Shim 适配器：将 feedback_loop 组件适配为 self_learning 接口
// ============================================================================

// championAnalyzerShim 把 feedbackloop.ChampionDialogueAnalyzer 适配为 selflearning.ChampionAnalyzer
//
// feedbackloop.ChampionDialogueAnalyzer.AnalyzePipeline 签名：
//   (ctx, since time.Time) (*dto.ChampionAnalysisReport, error)
// selflearning.ChampionAnalyzer.AnalyzePipeline 签名：
//   (ctx, since time.Time) ([]ExtractedScript, error)
// 此 shim 负责调用真实方法并转换 ExtractedScriptDTO → ExtractedScript
type championAnalyzerShim struct {
	analyzer *feedbackloop.ChampionDialogueAnalyzer
}

// AnalyzePipeline 适配方法
func (s *championAnalyzerShim) AnalyzePipeline(ctx context.Context, since time.Time) ([]selflearning.ExtractedScript, error) {
	if s.analyzer == nil {
		return nil, nil
	}
	report, err := s.analyzer.AnalyzePipeline(ctx, since)
	if err != nil {
		return nil, err
	}
	if report == nil || len(report.ExtractedScripts) == 0 {
		return nil, nil
	}
	out := make([]selflearning.ExtractedScript, 0, len(report.ExtractedScripts))
	for _, sc := range report.ExtractedScripts {
		out = append(out, selflearning.ExtractedScript{
			Title:              sc.Title,
			Content:            sc.Content,
			Scenario:           sc.Scenario,
			TriggerKeywords:    sc.TriggerKeywords,
			JourneyStage:       sc.JourneyStage,
			EffectivenessScore: sc.EffectivenessScore,
			ClusterID:          sc.ClusterID,
		})
	}
	return out, nil
}

// banditAllocatorShim 把 feedbackloop.BanditAllocator 适配为 selflearning.BanditAllocator
//
// feedbackloop.BanditAllocator 提供 SelectArm/UpdateReward/CheckConvergence 接口
// 此 shim 做参数/返回值类型转换：
//   - SelectArm: feedbackloop 返回 (armKey, strategy, err) → selflearning 仅需 (armKey, err)
//   - UpdateReward: feedbackloop 需 success bool → selflearning 仅传 reward，success 由 reward>0 推断
//   - CheckConvergence: feedbackloop 返回 (winnerArm, converged) → selflearning 需 (converged, winnerArm, posteriorProb, err)
type banditAllocatorShim struct {
	bandit *feedbackloop.BanditAllocator
}

// SelectArm 选择 arm
func (s *banditAllocatorShim) SelectArm(ctx context.Context, experimentID string) (string, error) {
	if s.bandit == nil {
		return "baseline", nil
	}
	armKey, _, err := s.bandit.SelectArm(ctx, experimentID)
	return armKey, err
}

// UpdateReward 更新奖励（reward > 0 视为 success）
func (s *banditAllocatorShim) UpdateReward(ctx context.Context, experimentID, armKey string, reward float64) error {
	if s.bandit == nil {
		return nil
	}
	return s.bandit.UpdateReward(ctx, experimentID, armKey, reward > 0, reward)
}

// CheckConvergence 检查收敛（posteriorProb 由 selflearning 侧默认 0，feedbackloop 不提供）
func (s *banditAllocatorShim) CheckConvergence(ctx context.Context, experimentID string) (bool, string, float64, error) {
	if s.bandit == nil {
		return false, "", 0, nil
	}
	winnerArm, converged := s.bandit.CheckConvergence(ctx, experimentID)
	return converged, winnerArm, 0, nil
}
