package service

import (
	"context"
	"errors"
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


// brokenCache 模拟 Redis 故障：Get 始终返回非 redis.Nil 错误
type brokenCache struct {
	cache.Cache
}

func (b *brokenCache) Get(_ context.Context, _ string) (string, error) {
	return "", errors.New("redis connection refused")
}

// TestHumanEscalation_RedisFailure_ExplicitKeyword_FailClosed
// S-1: Redis 故障 + 最近用户消息命中转人工关键词 → fail-closed 返回 true（转人工）
func TestHumanEscalation_RedisFailure_ExplicitKeyword_FailClosed(t *testing.T) {
	ctx := context.Background()
	mgr := NewHumanEscalationManager(&brokenCache{})

	locked := mgr.IsSessionLockedForHuman(ctx, "s_fail_closed", func() string {
		return "这个问题我要转人工处理"
	})
	if !locked {
		t.Fatal("Redis 故障且命中转人工关键词应 fail-closed 返回 true")
	}
}

// TestHumanEscalation_RedisFailure_NoKeyword_FailOpen
// S-1: Redis 故障 + 最近用户消息未命中关键词 → 放行 AI 返回 false
func TestHumanEscalation_RedisFailure_NoKeyword_FailOpen(t *testing.T) {
	ctx := context.Background()
	mgr := NewHumanEscalationManager(&brokenCache{})

	locked := mgr.IsSessionLockedForHuman(ctx, "s_fail_open", func() string {
		return "你好，这个产品多少钱"
	})
	if locked {
		t.Fatal("Redis 故障且无关键词命中应放行 AI 返回 false")
	}
}

// TestHumanEscalation_RedisFailure_NoFetcher_FailOpen
// S-1: Redis 故障且调用方未提供消息获取器 → 保持放行（向后兼容）
func TestHumanEscalation_RedisFailure_NoFetcher_FailOpen(t *testing.T) {
	ctx := context.Background()
	mgr := NewHumanEscalationManager(&brokenCache{})

	if mgr.IsSessionLockedForHuman(ctx, "s_no_fetcher") {
		t.Fatal("无 fetcher 时 Redis 故障应放行 AI")
	}
}

// TestHumanEscalation_KeyMissing_Unlocked key 不存在（redis.Nil）不算故障，直接放行
func TestHumanEscalation_KeyMissing_Unlocked(t *testing.T) {
	ctx := context.Background()
	mgr := NewHumanEscalationManager(cache.NewMemoryCache())

	called := false
	if mgr.IsSessionLockedForHuman(ctx, "s_missing", func() string {
		called = true
		return "转人工"
	}) {
		t.Fatal("key 不存在应返回 false")
	}
	if called {
		t.Fatal("key 不存在时不应触发降级 fetcher")
	}
}

// TestHumanEscalation_LockHasDefaultTTL S-2: 锁默认带 24h TTL 而非永久
func TestHumanEscalation_LockHasDefaultTTL(t *testing.T) {
	mgr := NewHumanEscalationManager(cache.NewMemoryCache())
	if mgr.lockTTL != HumanLockDefaultTTL || HumanLockDefaultTTL != 24*time.Hour {
		t.Fatalf("lockTTL = %s, want 24h", mgr.lockTTL)
	}
	mgr.SetLockTTL(0)
	if mgr.lockTTL != HumanLockDefaultTTL {
		t.Fatal("SetLockTTL(<=0) 应恢复默认值")
	}
}

// TestHumanEscalation_RenewLock S-2: 续期刷新锁与到期登记
func TestHumanEscalation_RenewLock(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)
	sid := "session_renew"

	if err := mgr.RenewSessionHumanLock(ctx, sid, time.Hour); err != nil {
		t.Fatalf("renew failed: %v", err)
	}
	if !mgr.IsSessionLockedForHuman(ctx, sid) {
		t.Fatal("续期后锁应存在")
	}
	deadline, ok := func() (time.Time, bool) {
		mgr.mu.RLock()
		defer mgr.mu.RUnlock()
		d, ok := mgr.lockDeadlines[sid]
		return d, ok
	}()
	if !ok {
		t.Fatal("续期应登记到期时间")
	}
	if time.Until(deadline) <= 55*time.Minute {
		t.Fatalf("续期 deadline 异常: %s", deadline)
	}
}

// TestHumanEscalation_LockExpiryNotifiesMerchant S-2: 锁到期后台释放并推送商户通知
func TestHumanEscalation_LockExpiryNotifiesMerchant(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)
	mgr.SetLockTTL(30 * time.Millisecond)
	sid := "session_expiry"

	if err := mgr.TriggerCensorshipEscalation(ctx, sid, "test_reason"); err != nil {
		t.Fatalf("trigger failed: %v", err)
	}

	time.Sleep(60 * time.Millisecond)
	mgr.checkExpiredLocks(ctx)

	items, err := c.LRange(ctx, MerchantNotifQueue, 0, -1)
	if err != nil {
		t.Fatalf("lrange failed: %v", err)
	}
	found := false
	for _, item := range items {
		if strings.Contains(item, "HUMAN_LOCK_EXPIRED") &&
			strings.Contains(item, sid) &&
			strings.Contains(item, "AI 恢复服务") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("锁到期应推送商户通知，queue=%v", items)
	}

	mgr.mu.RLock()
	_, tracked := mgr.lockDeadlines[sid]
	mgr.mu.RUnlock()
	if tracked {
		t.Fatal("过期锁登记项应被清除")
	}
}

// TestHumanEscalation_StartLockExpiryChecker 后台 goroutine 自动检测过期
func TestHumanEscalation_StartLockExpiryChecker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := cache.NewMemoryCache()
	mgr := NewHumanEscalationManager(c)
	mgr.SetLockTTL(20 * time.Millisecond)
	mgr.StartLockExpiryChecker(ctx, 10*time.Millisecond)

	if err := mgr.TriggerCensorshipEscalation(ctx, "session_bg_expiry", "test"); err != nil {
		t.Fatalf("trigger failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		items, _ := c.LRange(ctx, MerchantNotifQueue, 0, -1)
		for _, item := range items {
			if strings.Contains(item, "session_bg_expiry") && strings.Contains(item, "HUMAN_LOCK_EXPIRED") {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("2s 内后台检查器未推送锁超时通知")
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

