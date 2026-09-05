package confidence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// SeatAssignmentService 座席分配服务接口
//
// 已有 SessionAssignmentService 满足，这里抽象为接口解耦
type SeatAssignmentService interface {
	AutoAssign(ctx context.Context, sessionID, reason string) (agentID uint, err error)
}

// HandoffDecisionService 转人工决策服务
type HandoffDecisionService struct {
	repo    *repository.HandoffDecisionRepository
	seatSvc SeatAssignmentService
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

	reason := h.reasonOf(ctx, dec)
	record := &model.HandoffDecisionRecord{
		DecisionID:    uuid.New().String(),
		SessionID:     sessionID,
		CustomerID:    customerID,
		SignalID:      dec.SignalID,
		Reason:        reason,
		ReasonDetail:  fmt.Sprintf("conf=%.4f threshold=%.4f band=%s", dec.AggregatedConf, dec.DynamicThreshold, dec.DecisionBand),
		Confidence:    dec.AggregatedConf,
		Threshold:     dec.DynamicThreshold,
		IntentType:    intentType,
		CustomerLevel: customerLevel,
		Timeslot:      timeslotLabel(time.Now()),
		SLABreached:   false,
	}
	if err := h.repo.Create(ctx, record); err != nil {
		return 0, fmt.Errorf("save handoff decision: %w", err)
	}

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

func (h *HandoffDecisionService) reasonOf(_ context.Context, dec *dto.ConfidenceDecision) string {
	if dec.VetoTriggered != "" {
		return dec.VetoTriggered
	}
	if dec.DecisionBand == dto.BandHandoff {
		return "low_confidence"
	}
	return "band_handoff"
}

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
