package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/dto"
)

// D07/T07: agg 未注入时行为 = 纯启发式（原 G4 前行为回归）
func TestT07_ExtractConfidenceNilAgg(t *testing.T) {
	o := NewSmartCSOrchestrator(nil, nil, nil)
	resp := &SalesResponse{Reply: "您好", Polished: true, Audited: true}
	got := o.extractConfidence(context.Background(), resp, "s1", "在吗")
	// 启发式：0.5+0.1(Reply)+0.05(Polished)+0.1(Audited)=0.75
	if got < 0.74 || got > 0.76 {
		t.Errorf("nil agg 应回退启发式 0.75, got %v", got)
	}
}

// 启发式保留：Intent 透传分支不变
func TestT07_FallbackPassthroughIntent(t *testing.T) {
	o := NewSmartCSOrchestrator(nil, nil, nil)
	resp := &SalesResponse{Intent: &dto.RecognizeResult{IntentType: "price_inquiry", Confidence: 0.83}}
	if got := o.fallbackConfidence(resp); got != 0.83 {
		t.Errorf("Intent.Confidence>0 应透传 0.83, got %v", got)
	}
}
