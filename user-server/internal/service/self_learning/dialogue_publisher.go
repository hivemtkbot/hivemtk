package selflearning

// dialogue_publisher.go 对话事件发布器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §2.4
//
// 职责：
//   - 在对话生命周期关键节点发布事件到 EventBus
//   - 解耦对话服务与自我学习子系统（OpenSession/SendMessage/UpdateSessionStatus 不直接调用自我学习）
//
// 事件清单（v1.1 §2.4）：
//   1. TopicDialogueStarted  对话开始 → 触发 RAG 预热
//   2. TopicDialogueEnded    对话结束 → 触发 RAG 反思 / 资产包候选生成
//
// 集成方式：
//   - 在 chat_visitor_service.go 的 OpenSession/SendMessage 中调用 PublishStarted
//   - 在 customer_session.go 的 UpdateSessionStatus(closed/resolved) 中调用 PublishEnded
//   - 调用为 best-effort：发布失败仅记录日志，不影响主流程
//
// 反查 reward：PublishEnded 时通过 FeedbackSignalRepository 查询 aggregated_reward
// 若 FeedbackSignal 不存在（如对话未被采集反馈），reward=0 仍触发反思，但仅做轻量级处理

import (
	"context"
	"fmt"
	"log"
	"time"

	"marketing/internal/event"
)

// DialogueEventPublisher 对话事件发布器
type DialogueEventPublisher struct {
	bus               EventBus
	feedbackSignalRepo FeedbackSignalRepository
	sessionReader     SessionReader // 用于 PublishEnded 时反查会话信息
}

// SessionReader 会话读取接口（抽象 customer_session.CustomerSessionService）
type SessionReader interface {
	// GetSessionInfo 读取会话基础信息用于事件 payload
	GetSessionInfo(ctx context.Context, sessionID string) (*SessionInfo, error)
}

// SessionInfo 会话基础信息
type SessionInfo struct {
	SessionID      string
	TraceID        string
	VisitorID      string
	CustomerID     string
	ChannelType    string
	AccountID      string
	Scenario       string
	StartedAt      time.Time
	EndedAt        *time.Time
	Status         string
	DurationSec    int64
	LastCustomerMsg string
	LastAIReply    string
	UsedCorpusIDs  []string
	UsedAssetIDs   []string
}

// NewDialogueEventPublisher 创建对话事件发布器
func NewDialogueEventPublisher(
	bus EventBus,
	feedbackSignalRepo FeedbackSignalRepository,
	sessionReader SessionReader,
) *DialogueEventPublisher {
	return &DialogueEventPublisher{
		bus:                bus,
		feedbackSignalRepo: feedbackSignalRepo,
		sessionReader:      sessionReader,
	}
}

// ============================================================================
// PublishStarted 发布对话开始事件
// ============================================================================

// PublishStarted 发布 dialogue.started 事件
//
// 触发时机：
//   - VisitorChatService.OpenSession 成功后（WS 连接建立）
//   - WebhookService 首条访客消息到达时
//
// 订阅者：SelfLearningOrchestrator.onDialogueStarted → RAGSelfCorrector.Warmup
//
// 调用方应使用 best-effort 模式：失败仅 log，不阻断主流程
func (p *DialogueEventPublisher) PublishStarted(ctx context.Context, payload *event.DialogueStartedPayload) error {
	if p == nil || p.bus == nil {
		return nil
	}
	if payload == nil {
		return fmt.Errorf("dialogue started payload is nil")
	}
	if payload.SessionID == "" {
		return fmt.Errorf("session_id is empty")
	}
	if payload.StartedAt.IsZero() {
		payload.StartedAt = time.Now()
	}
	if err := p.bus.Publish(event.TopicDialogueStarted, payload); err != nil {
		// best-effort: 仅记录日志，不阻断主流程
		log.Printf("[self_learning] publish dialogue.started failed: session=%s err=%v", payload.SessionID, err)
		return nil
	}
	return nil
}

// ============================================================================
// PublishEnded 发布对话结束事件
// ============================================================================

// PublishEnded 发布 dialogue.ended 事件
//
// 触发时机：
//   - CustomerSessionService.UpdateSessionStatus(status=closed/resolved) 成功后
//
// 订阅者：
//   - SelfLearningOrchestrator.onDialogueEnded
//   - → RAGSelfCorrector.Reflect（基于 reward 矫正语料质量）
//   - → AssetBundleLearner.GenerateCandidate（基于销冠对话生成资产包候选）
//
// 实现要点：
//   1. 通过 SessionReader 反查会话信息（避免调用方手工填充）
//   2. 通过 FeedbackSignalRepository 反查 aggregated_reward（用于 reward-driven 决策）
//   3. reward=0 或 signal 不存在仍触发反思（轻量级处理）
func (p *DialogueEventPublisher) PublishEnded(ctx context.Context, sessionID string, traceID string) error {
	if p == nil || p.bus == nil {
		return nil
	}
	if sessionID == "" {
		return fmt.Errorf("session_id is empty")
	}
	// 1. 反查会话信息
	var info *SessionInfo
	if p.sessionReader != nil {
		var err error
		info, err = p.sessionReader.GetSessionInfo(ctx, sessionID)
		if err != nil {
			log.Printf("[self_learning] publish dialogue.ended: GetSessionInfo failed: session=%s err=%v", sessionID, err)
			// 仍尝试用 minimal payload 发布
			info = &SessionInfo{SessionID: sessionID, TraceID: traceID}
		}
	} else {
		info = &SessionInfo{SessionID: sessionID, TraceID: traceID}
	}
	if info == nil {
		info = &SessionInfo{SessionID: sessionID}
	}

	// 2. 反查 reward
	var reward float64
	var signalBreakdown map[string]any
	var outcome string
	if p.feedbackSignalRepo != nil {
		if sig, err := p.feedbackSignalRepo.GetBySessionID(ctx, sessionID); err == nil && sig != nil {
			reward = sig.AggregatedReward
			signalBreakdown = sig.SignalBreakdown
			outcome = sig.Outcome
		}
	}

	// 3. 构造 payload
	endedAt := time.Now()
	if info.EndedAt != nil {
		endedAt = *info.EndedAt
	}
	durationSec := info.DurationSec
	if durationSec == 0 && !info.StartedAt.IsZero() {
		durationSec = int64(endedAt.Sub(info.StartedAt).Seconds())
	}
	if outcome == "" {
		outcome = info.Status
	}
	if traceID == "" {
		traceID = info.SessionID // 降级：用 sessionID 作为 trace_id
	}

	payload := &event.DialogueEndedPayload{
		SessionID:        sessionID,
		VisitorID:        info.VisitorID,
		CustomerID:       info.CustomerID,
		DurationSec:      durationSec,
		Outcome:          outcome,
		AggregatedReward: reward,
		SignalBreakdown:  signalBreakdown,
		UsedCorpusIDs:    info.UsedCorpusIDs,
		UsedAssetIDs:     info.UsedAssetIDs,
		LastCustomerMsg:  info.LastCustomerMsg,
		LastAIReply:      info.LastAIReply,
		TraceID:          traceID,
		EndedAt:          endedAt,
	}

	// 4. 发布事件
	if err := p.bus.Publish(event.TopicDialogueEnded, payload); err != nil {
		log.Printf("[self_learning] publish dialogue.ended failed: session=%s err=%v", sessionID, err)
		return nil
	}
	return nil
}

// ============================================================================
// PublishAssetDegraded 发布资产包降级事件
// ============================================================================

// PublishAssetDegraded 发布 asset.degraded 事件
//
// 触发时机：AssetBundleLearner.DegradeInactiveAssets 检测到资产包连续 30 天 use_count=0
// 订阅者：看板告警订阅 / SelfLearningOrchestrator 触发新候选生成
func (p *DialogueEventPublisher) PublishAssetDegraded(ctx context.Context, payload *event.AssetDegradedPayload) error {
	if p == nil || p.bus == nil {
		return nil
	}
	if payload == nil || payload.AssetID == "" {
		return fmt.Errorf("asset degraded payload is invalid")
	}
	if payload.DegradedAt.IsZero() {
		payload.DegradedAt = time.Now()
	}
	if err := p.bus.Publish(event.TopicAssetDegraded, payload); err != nil {
		log.Printf("[self_learning] publish asset.degraded failed: asset=%s err=%v", payload.AssetID, err)
		return nil
	}
	return nil
}

// PublishAssetDegradeWarning 发布资产包降级预警事件（autonomous 模式下 24h 前预警）
func (p *DialogueEventPublisher) PublishAssetDegradeWarning(ctx context.Context, payload *event.AssetDegradeWarningPayload) error {
	if p == nil || p.bus == nil {
		return nil
	}
	if payload == nil || payload.AssetID == "" {
		return fmt.Errorf("asset degrade warning payload is invalid")
	}
	if payload.WarnedAt.IsZero() {
		payload.WarnedAt = time.Now()
	}
	if err := p.bus.Publish(event.TopicAssetDegradeWarning, payload); err != nil {
		log.Printf("[self_learning] publish asset.degrade.warning failed: asset=%s err=%v", payload.AssetID, err)
		return nil
	}
	return nil
}

// ============================================================================
// PublishRagCorpusUpdated 发布 RAG 语料变更事件
// ============================================================================

// PublishRagCorpusUpdated 发布 rag.corpus.updated 事件
//
// 触发时机：RAGSelfCorrector 销冠补录/低质归档/降权
// 订阅者：RAG 缓存失效订阅
func (p *DialogueEventPublisher) PublishRagCorpusUpdated(ctx context.Context, payload *event.RagCorpusUpdatedPayload) error {
	if p == nil || p.bus == nil {
		return nil
	}
	if payload == nil || payload.CorpusID == "" {
		return fmt.Errorf("rag corpus updated payload is invalid")
	}
	if payload.UpdatedAt.IsZero() {
		payload.UpdatedAt = time.Now()
	}
	if err := p.bus.Publish(event.TopicRagCorpusUpdated, payload); err != nil {
		log.Printf("[self_learning] publish rag.corpus.updated failed: corpus=%s err=%v", payload.CorpusID, err)
		return nil
	}
	return nil
}
