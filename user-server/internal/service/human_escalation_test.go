package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/cache"
)


// TestHumanEscalation_BasicFlow 完整流程
func TestHumanEscalation_BasicFlow(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)
	sessionID := "session_basic"

	if mgr.IsSessionLockedForHuman(ctx, sessionID) {
		t.Fatal("session should not be locked initially")
	}

	if err := mgr.TriggerCensorshipEscalation(ctx, sessionID, "crisis_high:骗子"); err != nil {
		t.Fatalf("trigger failed: %v", err)
	}

	if !mgr.IsSessionLockedForHuman(ctx, sessionID) {
		t.Error("session should be locked after trigger")
	}

	reason, _ := mgr.GetEscalationReason(ctx, sessionID)
	if reason != "crisis_high:骗子" {
		t.Errorf("reason = %s, want 'crisis_high:骗子'", reason)
	}

	if err := mgr.ReleaseHumanLock(ctx, sessionID); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	if mgr.IsSessionLockedForHuman(ctx, sessionID) {
		t.Error("session should not be locked after release")
	}
}

// TestHumanEscalation_MultipleSessions 多会话并发测试
func TestHumanEscalation_MultipleSessions(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)

	sessions := []string{"s1", "s2", "s3", "s4", "s5"}
	for _, sid := range sessions {
		if err := mgr.TriggerCensorshipEscalation(ctx, sid, "test_reason"); err != nil {
			t.Fatalf("trigger %s failed: %v", sid, err)
		}
	}

	for _, sid := range sessions {
		if !mgr.IsSessionLockedForHuman(ctx, sid) {
			t.Errorf("session %s should be locked", sid)
		}
	}
}

// TestHumanEscalation_EmptySessionID 边界：空 sessionID
func TestHumanEscalation_EmptySessionID(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)

	if err := mgr.TriggerCensorshipEscalation(ctx, "", "reason"); err == nil {
		t.Error("expected error for empty sessionID")
	}

	if mgr.IsSessionLockedForHuman(ctx, "") {
		t.Error("empty sessionID should return false")
	}
}

// TestHumanEscalation_NotificationPushed 验证通知已入队
func TestHumanEscalation_NotificationPushed(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)
	sessionID := "session_notif"

	if err := mgr.TriggerCensorshipEscalation(ctx, sessionID, "crisis_high:骗子"); err != nil {
		t.Fatalf("trigger failed: %v", err)
	}

	items, err := c.LRange(ctx, MerchantNotifQueue, 0, -1)
	if err != nil {
		t.Fatalf("lrange failed: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one notification in queue")
	}

	notif := items[0]
	if !strings.Contains(notif, sessionID) {
		t.Errorf("notification should contain sessionID, got: %s", notif)
	}
	if !strings.Contains(notif, "TRANSFER_TO_HUMAN") {
		t.Errorf("notification should contain event type, got: %s", notif)
	}
	if !strings.Contains(notif, "high") {
		t.Errorf("notification severity should be high for crisis_high reason, got: %s", notif)
	}
}

// TestHumanEscalation_ReleaseClearsReason 释放会清除原因
func TestHumanEscalation_ReleaseClearsReason(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)
	sessionID := "session_release"

	_ = mgr.TriggerCensorshipEscalation(ctx, sessionID, "test_reason")
	reason, _ := mgr.GetEscalationReason(ctx, sessionID)
	if reason == "" {
		t.Fatal("expected reason to be stored")
	}

	_ = mgr.ReleaseHumanLock(ctx, sessionID)
	reason, _ = mgr.GetEscalationReason(ctx, sessionID)
	if reason != "" {
		t.Errorf("reason should be cleared after release, got: %s", reason)
	}
}

// TestHumanEscalation_Stats 统计验证
func TestHumanEscalation_Stats(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)

	_ = mgr.TriggerCensorshipEscalation(ctx, "s1", "reason_a")
	_ = mgr.TriggerCensorshipEscalation(ctx, "s2", "reason_a")
	_ = mgr.TriggerCensorshipEscalation(ctx, "s3", "reason_b")

	stats := mgr.GetStats(ctx)
	if stats.TotalTriggers != 3 {
		t.Errorf("TotalTriggers = %d, want 3", stats.TotalTriggers)
	}
	if stats.LastSessionID != "s3" {
		t.Errorf("LastSessionID = %s, want s3", stats.LastSessionID)
	}
}

// TestHumanEscalation_Concurrent 100 并发触发
func TestHumanEscalation_Concurrent(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sid := "concurrent_" + itoa(idx)
			_ = mgr.TriggerCensorshipEscalation(ctx, sid, "concurrent")
		}(i)
	}
	wg.Wait()

	locked := 0
	for i := 0; i < 100; i++ {
		sid := "concurrent_" + itoa(i)
		if mgr.IsSessionLockedForHuman(ctx, sid) {
			locked++
		}
	}
	if locked != 100 {
		t.Errorf("locked = %d, want 100", locked)
	}
}

// TestHumanEscalation_CustomNotifier 自定义通知器
func TestHumanEscalation_CustomNotifier(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)

	received := make(chan *EscalationEvent, 1)
	mgr.SetNotifier(ctx, &captureNotifier{ch: received})

	_ = mgr.TriggerCensorshipEscalation(ctx, "session_capture", "test_reason")

	select {
	case e := <-received:
		if e.SessionID != "session_capture" {
			t.Errorf("sessionID = %s, want session_capture", e.SessionID)
		}
		if e.Event != "TRANSFER_TO_HUMAN" {
			t.Errorf("event = %s, want TRANSFER_TO_HUMAN", e.Event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notifier did not receive event within 2s")
	}
}

// TestHumanEscalation_GatekeeperIntegration 与方向6 门禁阶段集成
func TestHumanEscalation_GatekeeperIntegration(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)
	sessionID := "session_integration"

	if err := mgr.TriggerCensorshipEscalation(ctx, sessionID, "high_risk_keyword:骗子"); err != nil {
		t.Fatalf("trigger failed: %v", err)
	}

	if !mgr.IsSessionLockedForHuman(ctx, sessionID) {
		t.Error("session should be locked for AI bypass")
	}

	reason, _ := mgr.GetEscalationReason(ctx, sessionID)
	if !strings.Contains(reason, "high_risk_keyword") {
		t.Errorf("reason should contain high_risk_keyword, got: %s", reason)
	}
}


type captureNotifier struct {
	ch chan *EscalationEvent
}

func (n *captureNotifier) Notify(_ context.Context, e *EscalationEvent) error {
	select {
	case n.ch <- e:
	default:
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

