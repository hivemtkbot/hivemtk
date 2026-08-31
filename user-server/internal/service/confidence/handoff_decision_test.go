package confidence

import (
	"context"
	"testing"

	"hivemtk-user/internal/dto"
)

// newHandoffDecisionServiceForTest 构造无需 DB 的测试实例（repo/seatSvc 均可为 nil，reasonOf 不依赖二者）
func newHandoffDecisionServiceForTest() *HandoffDecisionService {
	return &HandoffDecisionService{}
}

func TestReasonOf_VetoTriggered_Wins(t *testing.T) {
	h := newHandoffDecisionServiceForTest()
	dec := &dto.ConfidenceDecision{
		VetoTriggered:  "user_explicitly_asks_human",
		AggregatedConf: 0.95,
		DecisionBand:   dto.BandHandoff,
	}
	if got := h.reasonOf(context.Background(), dec); got != "user_explicitly_asks_human" {
		t.Fatalf("veto must win; want=user_explicitly_asks_human got=%q", got)
	}
}

func TestReasonOf_LowConfidence_WhenBandHandoff(t *testing.T) {
	h := newHandoffDecisionServiceForTest()
	dec := &dto.ConfidenceDecision{
		AggregatedConf: 0.35,
		DecisionBand:   dto.BandHandoff,
	}
	if got := h.reasonOf(context.Background(), dec); got != "low_confidence" {
		t.Fatalf("band handoff must be low_confidence; want=low_confidence got=%q", got)
	}
}

// TestReasonOf_LowConfidence_ConfAboveOldMagic 锁定关键正确性：
// 当运营把 policy.BandHandoffUpper 调高（如 0.55），conf=0.50 仍落入 handoff 区间，
// 原因必须归 low_confidence。旧硬编码 conf<0.4 实现会在此误标 band_handoff —— 本测试暴露该缺陷。
func TestReasonOf_LowConfidence_ConfAboveOldMagic(t *testing.T) {
	h := newHandoffDecisionServiceForTest()
	dec := &dto.ConfidenceDecision{
		AggregatedConf: 0.50,
		DecisionBand:   dto.BandHandoff,
	}
	if got := h.reasonOf(context.Background(), dec); got != "low_confidence" {
		t.Fatalf("band-driven handoff must be low_confidence even if conf>0.4; want=low_confidence got=%q", got)
	}
}

func TestReasonOf_BandHandoff_WhenBandNotHandoff(t *testing.T) {
	h := newHandoffDecisionServiceForTest()
	dec := &dto.ConfidenceDecision{
		AggregatedConf: 0.80,
		DecisionBand:   dto.BandAuto,
	}
	if got := h.reasonOf(context.Background(), dec); got != "band_handoff" {
		t.Fatalf("non-handoff band with no veto must fall back to band_handoff; want=band_handoff got=%q", got)
	}
}
