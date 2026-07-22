package agent_runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// InferenceCycle 单次推理闭环编排器
// ----------------------------------------------------------------------------
// 文档依据：方向4 认知决策大脑层
//
// 核心职责：
//  1. 串联 6 个阶段（感知 → 对齐 → 门禁 → 规划 → 转人工/执行）
//  2. 错误隔离：单阶段失败不阻塞其他阶段
//  3. 超时控制：每阶段独立超时
//  4. 可观测：每个阶段的决策/耗时/成功与否都记录
//  5. 决策聚合：最终汇总为 InferenceDecision
//
// 与 defaultAgentRuntime 的关系：
//  - InferenceCycle 是 AgentRuntime 的内部组件
//  - 编排器在 HandleCustomerMessage 中被调用
//  - 老路径（sales/cs bridge）保留作为 fallback
// ============================================================================

// EpisodicMemoryProvider 跨会话情境记忆提供者（E1 补全）
// 由外部注入，InferenceCycle 在 RunOnce 开头调用 LoadEpisodicMemory 读取
// 跨会话上下文（L1/L2/L3/L4 汇总），填入 InferenceContext.EpisodicMemory，
// 供 PlannerStage 在 ReplyHint 中引用，使情境记忆影响决策。
// 典型实现：包装 service.MemorySystem.BuildFullContext / Recall。
type EpisodicMemoryProvider interface {
	LoadEpisodicMemory(ctx context.Context, sessionID, customerID string) (string, error)
}

// InferenceCycle 推理闭环
type InferenceCycle struct {
	// 阶段（按顺序执行）
	PerceptionStage InferenceStage
	AlignmentStage  InferenceStage
	GatekeeperStage InferenceStage
	PlannerStage    InferenceStage

	// 阶段超时（单阶段最大耗时）
	StageTimeout time.Duration

	// 总超时
	TotalTimeout time.Duration

	// 跨会话情境记忆提供者（可选，nil 时跳过记忆读取，行为不变）
	memoryProvider EpisodicMemoryProvider

	// 内部状态
	mu        sync.RWMutex
	stopped   bool
	lastStats CycleStats
}

// SetMemoryProvider 注入跨会话情境记忆提供者（E1 补全）
// 非并发安全，应在初始化阶段调用
func (c *InferenceCycle) SetMemoryProvider(p EpisodicMemoryProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memoryProvider = p
}

// CycleStats 闭环统计
type CycleStats struct {
	TotalRuns      int64
	SuccessRuns    int64
	EscalationRuns int64
	FailureRuns    int64
	AvgDurationMs  int64
}

// InferenceCycleConfig 配置
type InferenceCycleConfig struct {
	StageTimeout time.Duration
	TotalTimeout time.Duration
}

// NewInferenceCycle 默认推理闭环
func NewInferenceCycle() *InferenceCycle {
	return &InferenceCycle{
		PerceptionStage: NewDefaultPerceptionStage(),
		AlignmentStage:  NewDefaultAlignmentScorer(),
		GatekeeperStage: NewDefaultCrisisDetector(),
		PlannerStage:    NewDefaultTaskPlanner(),
		StageTimeout:    2 * time.Second,
		TotalTimeout:    8 * time.Second,
	}
}

// NewInferenceCycleWithStages 自定义阶段
func NewInferenceCycleWithStages(
	perception, alignment, gatekeeper, planner InferenceStage,
) *InferenceCycle {
	return &InferenceCycle{
		PerceptionStage: perception,
		AlignmentStage:  alignment,
		GatekeeperStage: gatekeeper,
		PlannerStage:    planner,
		StageTimeout:    2 * time.Second,
		TotalTimeout:    8 * time.Second,
	}
}

// NewInferenceCycleWithConfig 完整自定义
func NewInferenceCycleWithConfig(cfg InferenceCycleConfig, perception, alignment, gatekeeper, planner InferenceStage) *InferenceCycle {
	cycle := NewInferenceCycleWithStages(perception, alignment, gatekeeper, planner)
	if cfg.StageTimeout > 0 {
		cycle.StageTimeout = cfg.StageTimeout
	}
	if cfg.TotalTimeout > 0 {
		cycle.TotalTimeout = cfg.TotalTimeout
	}
	return cycle
}

// ============================================================================
// 核心方法
// ============================================================================

// RunOnce 执行一次完整推理闭环
//
// 入参：
//   - payload: 客户消息载荷
//   - agentCtx: 智能体上下文（可为空，将用默认占位）
//
// 返回：
//   - InferenceDecision: 最终决策
//   - error: 仅在编排器本身失败时返回错误
//
// 行为：
//  1. 构造 InferenceContext
//  2. 顺序执行 4 个阶段（感知 → 对齐 → 门禁 → 规划）
//  3. 每阶段都设独立超时
//  4. 收集所有阶段决策
//  5. 汇总到 InferenceDecision
func (c *InferenceCycle) RunOnce(ctx context.Context, payload CustomerMessagePayload, agentCtx *AgentContext) (*InferenceDecision, error) {
	if c.stopped {
		return nil, ErrRuntimeStopped
	}

	start := time.Now()

	// 默认 agentCtx
	if agentCtx == nil {
		agentCtx = &AgentContext{
			AgentID:   0,
			AgentCode: "default",
			Name:      "默认智能体",
			AgentType: "customer_service",
			LoadedAt:  time.Now(),
		}
	}

	// 构造总超时 ctx
	tctx, cancel := context.WithTimeout(ctx, c.TotalTimeout)
	defer cancel()

	ic := &InferenceContext{
		Payload:   payload,
		AgentCtx:  agentCtx,
		StartTime: start,
		Stages:    []StageDecision{},
		Decision: InferenceDecision{
			ReplyType:  "text",
			Confidence: 0,
		},
	}

	// E1 补全：读取跨会话情境记忆，填入 InferenceContext 供阶段消费。
	// provider 为 nil（未注入）时跳过，保持原行为；读取失败仅告警不阻塞推理。
	c.mu.RLock()
	provider := c.memoryProvider
	c.mu.RUnlock()
	if provider != nil {
		mem, err := provider.LoadEpisodicMemory(tctx, payload.SessionID, payload.CustomerID)
		if err != nil {
			logger.Warnf("[inference_cycle] load episodic memory failed session=%s customer=%s err=%v",
				payload.SessionID, payload.CustomerID, err)
		} else if mem != "" {
			ic.EpisodicMemory = mem
		}
	}

	// 阶段执行列表
	stages := []InferenceStage{
		c.PerceptionStage,
		c.AlignmentStage,
		c.GatekeeperStage,
		c.PlannerStage,
	}

	// 顺序执行
	for _, stage := range stages {
		if stage == nil {
			continue
		}

		// 阶段超时
		sctx, scancel := context.WithTimeout(tctx, c.StageTimeout)
		result := stage.Execute(sctx, ic)
		scancel()

		// 错误处理
		if result.Error != nil {
			logger.Warnf("[inference_cycle] stage=%s error=%v", stage.Name(), result.Error)
		}

		// 早退判定
		if result.EarlyReturn {
			if result.Decision != nil {
				// 合并阶段决策
				ic.Decision = mergeDecision(ic.Decision, *result.Decision)
			}
			ic.Decision.TotalDuration = time.Since(start)
			// 方向6：保留 ic 的诊断快照，便于审计与可观测
			ic.Decision.Crisis = ic.Crisis
			ic.Decision.Sentiment = ic.Sentiment
			ic.Decision.Intent = ic.Intent
			ic.Decision.Alignment = ic.Alignment
			c.recordStats(ic.Decision)
			logger.Infof("[inference_cycle] early return at stage=%s handoff=%v reason=%s duration=%s",
				stage.Name(), ic.Decision.HandoffToHuman, ic.Decision.StopReason, ic.Decision.TotalDuration)
			return &ic.Decision, nil
		}
	}

	// 全部阶段完成：聚合最终决策
	ic.Decision.TotalDuration = time.Since(start)
	if ic.Plan != nil {
		ic.Decision.ReplyType = "text"
		ic.Decision.Confidence = ic.Plan.Confidence
		if ic.Plan.SkipLLM {
			ic.Decision.Reply = ic.Plan.ReplyHint
			ic.Decision.ReplyType = "text"
			ic.Decision.StopReason = "faq_skip_llm"
		} else {
			ic.Decision.StopReason = "plan_ready"
		}
	} else {
		ic.Decision.StopReason = "no_plan"
	}
	// 方向6：保留 ic 的诊断快照
	ic.Decision.Crisis = ic.Crisis
	ic.Decision.Sentiment = ic.Sentiment
	ic.Decision.Intent = ic.Intent
	ic.Decision.Alignment = ic.Alignment

	c.recordStats(ic.Decision)
	logger.Infof("[inference_cycle] completed trace=%s stages=%d plan_type=%s confidence=%.2f duration=%s",
		payload.TraceID, len(ic.Stages),
		planTypeOf(ic.Plan), ic.Decision.Confidence, ic.Decision.TotalDuration)

	return &ic.Decision, nil
}

// ============================================================================
// 辅助方法
// ============================================================================

// mergeDecision 合并两个决策（早退决策优先）
func mergeDecision(base, override InferenceDecision) InferenceDecision {
	merged := base
	if override.HandoffToHuman {
		merged.HandoffToHuman = true
	}
	if override.HandoffReason != "" {
		merged.HandoffReason = override.HandoffReason
	}
	if override.Plan != nil {
		merged.Plan = override.Plan
	}
	if override.Reply != "" {
		merged.Reply = override.Reply
	}
	if override.ReplyType != "" {
		merged.ReplyType = override.ReplyType
	}
	if override.Confidence > 0 {
		merged.Confidence = override.Confidence
	}
	if override.StopReason != "" {
		merged.StopReason = override.StopReason
	}
	// 方向6：保留诊断快照（base 优先于 override，因为 base 已有最新阶段产出）
	if merged.Crisis.Level != CrisisNone || merged.Crisis.Reason != "" {
		// 已有保留
	} else if override.Crisis.Level != CrisisNone || override.Crisis.Reason != "" {
		merged.Crisis = override.Crisis
	}
	return merged
}

// planTypeOf 安全获取 PlanType
func planTypeOf(p *ActionPlan) string {
	if p == nil {
		return "none"
	}
	return p.PlanType
}

// recordStats 记录统计
func (c *InferenceCycle) recordStats(d InferenceDecision) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastStats.TotalRuns++
	if d.HandoffToHuman {
		c.lastStats.EscalationRuns++
	} else if d.Reply != "" || d.Plan != nil {
		c.lastStats.SuccessRuns++
	} else {
		c.lastStats.FailureRuns++
	}
	// 移动平均
	durMs := d.TotalDuration.Milliseconds()
	if c.lastStats.AvgDurationMs == 0 {
		c.lastStats.AvgDurationMs = durMs
	} else {
		c.lastStats.AvgDurationMs = (c.lastStats.AvgDurationMs*9 + durMs) / 10
	}
}

// GetStats 获取统计
func (c *InferenceCycle) GetStats() CycleStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastStats
}

// Stop 停止推理闭环
func (c *InferenceCycle) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return errors.New("inference cycle already stopped")
	}
	c.stopped = true
	return nil
}

// Reset 重置（用于测试）
func (c *InferenceCycle) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = false
	c.lastStats = CycleStats{}
}

// ============================================================================
// 错误定义
// ============================================================================

// ErrInferenceTimeout 推理超时
var ErrInferenceTimeout = errors.New("inference cycle timeout")

// ErrInvalidPayload 无效载荷
var ErrInvalidPayload = errors.New("invalid inference payload")

// ValidatePayload 校验载荷
func ValidatePayload(p CustomerMessagePayload) error {
	if p.ChannelType == "" {
		return fmt.Errorf("%w: channel_type required", ErrInvalidPayload)
	}
	if p.CustomerID == "" {
		return fmt.Errorf("%w: customer_id required", ErrInvalidPayload)
	}
	return nil
}
