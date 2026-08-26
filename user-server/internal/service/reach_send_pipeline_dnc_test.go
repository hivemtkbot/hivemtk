package service

import (
	"context"
	"testing"
)

// dncStubChecker DoNotContactChecker 测试桩
type dncStubChecker struct {
	blocked map[string]bool
}

func (s *dncStubChecker) IsBlocked(_ context.Context, oneID, channel string) bool {
	return s.blocked[oneID+"|"+channel] || s.blocked[oneID+"|*"]
}

// TestReachSendPipeline_Permission_DoNotContactBlocked 命中全局退订标志位时 permission 步骤拦截发送
func TestReachSendPipeline_Permission_DoNotContactBlocked(t *testing.T) {
	ctx := context.Background()
	adapter := NewFuncChannelAdapter(nil)
	pipeline := NewSendPipeline(SendPipelineConfig{
		PermissionChecker: AllowAllSendPermission{},
		DoNotContact:      &dncStubChecker{blocked: map[string]bool{"one-dnc|sms": true}},
		RateLimiter:       NoOpSendRateLimiter{},
		AuditLogger:       NewMemorySendAuditLogger(10),
		CostTracker:       NoOpSendCostTracker{},
		JourneyTracker:    NoOpSendJourneyTracker{},
		Adapter:           adapter,
	})

	req := &ReachSendRequest{
		Channel:     "sms",
		CustomerID:  "one-dnc",
		RecipientID: "13800138000",
		Content:     "hello",
	}
	resp := pipeline.Send(ctx, req)
	if resp.Success {
		t.Fatal("Expected send blocked by do-not-contact")
	}
	if resp.Error != ErrSendDoNotContact.Error() {
		t.Errorf("Expected error %q, got %q", ErrSendDoNotContact.Error(), resp.Error)
	}
	found := false
	for _, s := range resp.StepResults {
		if s.Step == SendStepPermission && !s.Success && s.Error == ErrSendDoNotContact.Error() {
			found = true
		}
	}
	if !found {
		t.Error("Expected permission step to fail with do-not-contact error")
	}
	if adapter.Count(ctx) != 0 {
		t.Errorf("Expected adapter not called, got %d calls", adapter.Count(ctx))
	}
}

// TestReachSendPipeline_Permission_DoNotContactAllowed 未命中标志位时正常放行
func TestReachSendPipeline_Permission_DoNotContactAllowed(t *testing.T) {
	ctx := context.Background()
	pipeline := NewSendPipeline(SendPipelineConfig{
		PermissionChecker: AllowAllSendPermission{},
		DoNotContact:      &dncStubChecker{blocked: map[string]bool{}},
		RateLimiter:       NoOpSendRateLimiter{},
		AuditLogger:       NewMemorySendAuditLogger(10),
		CostTracker:       NoOpSendCostTracker{},
		JourneyTracker:    NoOpSendJourneyTracker{},
		Adapter:           NewFuncChannelAdapter(nil),
	})

	resp := pipeline.Send(ctx, &ReachSendRequest{
		Channel:     "sms",
		CustomerID:  "one-clean",
		RecipientID: "13700004000",
		Content:     "hello",
	})
	if !resp.Success {
		t.Fatalf("Expected success, got error %q", resp.Error)
	}
}

// TestCountedSendPipeline_DoNotContactSkipped 计数上报：DoNotContactSkipped 递增
func TestCountedSendPipeline_DoNotContactSkipped(t *testing.T) {
	ctx := context.Background()
	inner := NewSendPipeline(SendPipelineConfig{
		PermissionChecker: AllowAllSendPermission{},
		DoNotContact:      &dncStubChecker{blocked: map[string]bool{"one-x|email": true}},
		RateLimiter:       NoOpSendRateLimiter{},
		AuditLogger:       NewMemorySendAuditLogger(10),
		CostTracker:       NoOpSendCostTracker{},
		JourneyTracker:    NoOpSendJourneyTracker{},
		Adapter:           NewFuncChannelAdapter(nil),
	})
	counted := NewCountedSendPipeline(inner)

	counted.Send(ctx, &ReachSendRequest{Channel: "email", CustomerID: "one-x", RecipientID: "a@b.c", Content: "x"})
	cp, ok := counted.(interface {
		Stats(ctx context.Context) SendPipelineStats
	})
	if !ok {
		t.Fatal("Expected counted pipeline to expose Stats")
	}
	stats := cp.Stats(ctx)
	if stats.DoNotContactSkipped != 1 {
		t.Errorf("Expected DoNotContactSkipped=1, got %d", stats.DoNotContactSkipped)
	}
	if stats.FailedSends != 1 {
		t.Errorf("Expected FailedSends=1, got %d", stats.FailedSends)
	}
}
