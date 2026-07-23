package confidence

// handoff_decision.go 转人工决策服务
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十五章 §15.4.11
//
// 职责：
//   1. 写入 handoff_decisions 表（决策记录）
//   2. 触发座席分配（SeatAssignmentService.AutoAssign）
//   3. 更新分配结果（assigned_agent_id / assigned_at）

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// SeatAssignmentService 座席分配服务接口
//
// 已有 SessionAssignmentService 满足，这里抽象为接口解耦
type SeatAssignmentService interface {
	// AutoAssign 自动分配座席
	// 返回分配的座席 ID（0 表示无可用座席，进入 waiting 队列）
	AutoAssign(ctx context.Context, sessionID, reason string) (agentID uint, err error)
}

// HandoffDecisionService 转人工决策服务
type HandoffDecisionService struct {
	repo	*repository.HandoffDecisionRepository
	seatSvc	SeatAssignmentService
}

// NewHandoffDecisionService 创建转人工决策服务
func NewHandoffDecisionService(repo *repository.HandoffDecisionRepository, seatSvc SeatAssignmentService) *HandoffDecisionService {
	return &HandoffDecisionService{repo: repo, seatSvc: seatSvc}
}

// Execute 执行转人工
//
// 流程：
//  1. 写入 handoff_decisions 表
//  2. 调用 SeatAssignmentService.AutoAssign 分配座席
//  3. 更新分配结果
//
// 返回分配的座席 ID（0 表示无可用座席，进入 waiting 队列）
func (h *HandoffDecisionService) Execute(
	ctx context.Context,
	dec *dto.ConfidenceDecision,
	sessionID, customerID, intentType string,
	customerLevel string,
) (uint, error) {
	if dec == nil {
		return 0, fmt.Errorf("decision is nil")
	}

	// 1. 写入决策记录
	reason := h.reasonOf(ctx, dec)
	record := &model.HandoffDecisionRecord{
		DecisionID:	uuid.New().String(),
		SessionID:	sessionID,
		CustomerID:	customerID,
		SignalID:	dec.SignalID,
		Reason:		reason,
		ReasonDetail:	fmt.Sprintf("conf=%.4f threshold=%.4f band=%s", dec.AggregatedConf, dec.DynamicThreshold, dec.DecisionBand),
		Confidence:	dec.AggregatedConf,
		Threshold:	dec.DynamicThreshold,
		IntentType:	intentType,
		CustomerLevel:	customerLevel,
		Timeslot:	timeslotLabel(time.Now()),
		SLABreached:	false,
	}
	if err := h.repo.Create(ctx, record); err != nil {
		return 0, fmt.Errorf("save handoff decision: %w", err)
	}

	// 2. 触发座席分配
	if h.seatSvc == nil {
		return 0, nil
	}
	agentID, err := h.seatSvc.AutoAssign(ctx, sessionID, record.ReasonDetail)
	if err != nil {
		return 0, err
	}
	if agentID > 0 {
		now := time.Now()
		record.AssignedAgentID = int64(agentID)
		record.AssignedAt = &now
		_ = h.repo.Update(ctx, record)
	}
	return agentID, nil
}

// MarkAccepted 标记座席已接受
func (h *HandoffDecisionService) MarkAccepted(ctx context.Context, decisionID string, agentID int64) error {
	return h.repo.MarkAccepted(ctx, decisionID, agentID)
}

// MarkResolved 标记已解决
func (h *HandoffDecisionService) MarkResolved(ctx context.Context, decisionID string, customerAccepted bool) error {
	return h.repo.MarkResolved(ctx, decisionID, customerAccepted)
}

// reasonOf 推断转人工原因
//
// 优先级：
//  1. veto_triggered（否决原因）
//  2. low_confidence（conf < 0.4）
//  3. band_handoff（其他）
func (h *HandoffDecisionService) reasonOf(ctx context.Context, dec *dto.ConfidenceDecision) string {
	if dec.VetoTriggered != "" {
		return dec.VetoTriggered
	}
	if dec.AggregatedConf < 0.4 {
		return "low_confidence"
	}
	return "band_handoff"
}

// timeslotLabel 时段标签（用于记录）
func timeslotLabel(t time.Time) string {
	hour := t.Hour()
	switch {
	case (hour >= 10 && hour < 12) || (hour >= 14 && hour < 16):
		return "peak"
	case hour >= 0 && hour < 7:
		return "low"
	default:
		return "normal"
	}
}
