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

// reasonOf 推断转人工原因
//
// 优先级：
//  1. veto_triggered（否决原因）
//  2. low_confidence（聚合置信度落入 handoff 区间，即 dec.DecisionBand == BandHandoff）
//  3. band_handoff（其他，防御性兜底）
//
// 必须以 dec.DecisionBand 为准，而非硬编码 0.4，也非 dec.DynamicThreshold：
//   - 真实转人工判据由 DynamicThresholdCalculator.DetermineBand 基于 policy.BandHandoffUpper
//     计算（该值可由运营后台按意图动态调整，默认 0.40）。硬编码 0.4 会在 BandHandoffUpper
//     被调高（如 0.55）后与真实判据脱节，把本应归 low_confidence 的 handoff 误标为 band_handoff。
//   - dec.DynamicThreshold 是 per-intent 计算值（如 0.70），并非 handoff 触发条件，
//     用它判定会恒把真实 handoff 归为 band_handoff，故不可取。
func (h *HandoffDecisionService) reasonOf(_ context.Context, dec *dto.ConfidenceDecision) string {
	if dec.VetoTriggered != "" {
		return dec.VetoTriggered
	}
	if dec.DecisionBand == dto.BandHandoff {
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
