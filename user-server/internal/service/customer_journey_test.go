package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/cache"
)

func newP5JourneyPair() (*CustomerJourneyService, *CustomerJourneyService, *cache.MemoryCache) {
	l2 := cache.NewMemoryCache()
	defer l2.Close()
	svc1 := NewCustomerJourneyServiceWithCache(l2)
	svc2 := NewCustomerJourneyServiceWithCache(l2)
	return svc1, svc2, l2
}

// TestJourneyP5_MultiInstanceRedisAuthority 实例 A 写入后，实例 B 必须能从权威源读到最新阶段
func TestJourneyP5_MultiInstanceRedisAuthority(t *testing.T) {
	ctx := context.Background()
	svc1, svc2, _ := newP5JourneyPair()

	if _, err := svc1.Transition(ctx, "cust_p5_a", StageContact, "test", "op", "init", nil); err != nil {
		t.Fatalf("transition: %v", err)
	}

	got := svc2.GetState(ctx, "cust_p5_a")
	if got.CurrentStage != StageContact {
		t.Errorf("instance B stage = %s, want %s (should read Redis authority)", got.CurrentStage, StageContact)
	}
}

// TestJourneyP5_L1ReadCacheTTL 内存读缓存 60s 内命中本地；过期后惰性淘汰并回源 Redis 权威
func TestJourneyP5_L1ReadCacheTTL(t *testing.T) {
	ctx := context.Background()
	svc, _, l2 := newP5JourneyPair()

	if _, err := svc.Transition(ctx, "cust_p5_b", StageContact, "test", "op", "init", nil); err != nil {
		t.Fatalf("transition: %v", err)
	}

	remote := JourneyState{
		CustomerID:   "cust_p5_b",
		CurrentStage: StageQuoted,
		StageSince:   time.Now(),
		StageHistory: []JourneyEvent{},
		AutoTags:     []string{},
		Metadata:     map[string]string{},
	}
	if err := l2.SetJSON(ctx, journeyStateKey("cust_p5_b"), &remote, time.Hour); err != nil {
		t.Fatalf("seed redis: %v", err)
	}

	cached := svc.GetState(ctx, "cust_p5_b")
	if cached.CurrentStage != StageContact {
		t.Fatalf("L1 hit expected contact, got %s", cached.CurrentStage)
	}

	svc.mu.Lock()
	svc.l1ExpiresAt["cust_p5_b"] = time.Now().Add(-time.Second)
	svc.mu.Unlock()

	fresh := svc.GetState(ctx, "cust_p5_b")
	if fresh.CurrentStage != StageQuoted {
		t.Errorf("after L1 expiry stage = %s, want %s (must fall back to Redis authority)", fresh.CurrentStage, StageQuoted)
	}

	svc.mu.RLock()
	exp, ok := svc.l1ExpiresAt["cust_p5_b"]
	svc.mu.RUnlock()
	if !ok || exp.Before(time.Now()) {
		t.Error("reloaded L1 entry should carry fresh TTL after lazy eviction")
	}
}

// TestJourneyP5_WriteDualWrite 写路径必须双写：L1 与 L2(Redis) 同时可见
func TestJourneyP5_WriteDualWrite(t *testing.T) {
	ctx := context.Background()
	svc, _, l2 := newP5JourneyPair()

	if _, err := svc.Transition(ctx, "cust_p5_c", StageInterested, "test", "op", "init", nil); err != nil {
		t.Fatalf("transition: %v", err)
	}

	var stored JourneyState
	if err := l2.GetJSON(ctx, journeyStateKey("cust_p5_c"), &stored); err != nil || stored.CustomerID == "" {
		t.Fatalf("redis dual-write missing: err=%v state=%+v", err, stored)
	}
	if stored.CurrentStage != StageInterested {
		t.Errorf("redis stage = %s, want %s", stored.CurrentStage, StageInterested)
	}
	if len(stored.StageHistory) != 1 {
		t.Errorf("redis history len = %d, want 1", len(stored.StageHistory))
	}

	svc.Touch(ctx, "cust_p5_c", "test")
	stored2 := JourneyState{}
	if err := l2.GetJSON(ctx, journeyStateKey("cust_p5_c"), &stored2); err != nil {
		t.Fatalf("redis get after touch: %v", err)
	}
	if stored2.TotalTouches != 1 {
		t.Errorf("total_touches in redis = %d, want 1", stored2.TotalTouches)
	}
}

// TestJourneySleepCron_RunOnce 调度执行应复用 AutoDetectSleeping：超阈值客户迁移至 sleeping
func TestJourneySleepCron_RunOnce(t *testing.T) {
	svc, _, l2 := newP5JourneyPair()
	if _, err := svc.Transition(context.Background(), "cust_sleep_a", StageWon, "ai", "ai", "init", nil); err != nil {
		t.Fatalf("transition: %v", err)
	}

	svc.mu.Lock()
	svc.states["cust_sleep_a"].LastTouchAt = time.Now().Add(-200 * 24 * time.Hour)
	svc.mu.Unlock()

	c := NewJourneySleepCron(svc)
	detected := c.runOnce(context.Background())

	found := false
	for _, cid := range detected {
		if cid == "cust_sleep_a" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cust_sleep_a should be detected, got %v", detected)
	}

	var stored JourneyState
	if err := l2.GetJSON(context.Background(), journeyStateKey("cust_sleep_a"), &stored); err != nil {
		t.Fatalf("read redis: %v", err)
	}
	if stored.CurrentStage != StageSleeping {
		t.Errorf("redis stage = %s, want %s", stored.CurrentStage, StageSleeping)
	}
}

// TestJourneySleepCron_RunOnce_PanicRecovered 执行 panic 不应外溢崩溃调度协程
func TestJourneySleepCron_RunOnce_PanicRecovered(t *testing.T) {
	c := NewJourneySleepCron(nil)
	c.svc = nil
	detected := c.runOnce(context.Background())
	if detected != nil {
		t.Errorf("expected nil after recovered panic, got %v", detected)
	}
}

// TestJourneySleepCron_StartStopIdempotent 重复 Start/Stop 应安全（不重复起协程、不死锁）
func TestJourneySleepCron_StartStopIdempotent(t *testing.T) {
	_c := cache.NewMemoryCache()
	defer _c.Close()
	c := NewJourneySleepCron(NewCustomerJourneyServiceWithCache(_c))
	done := make(chan struct{})
	go func() {
		c.Start(context.Background())
		c.Start(context.Background())
		c.Stop(context.Background())
		c.Stop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start/Stop deadlocked")
	}
}
