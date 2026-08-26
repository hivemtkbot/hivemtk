package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/cache"
)

// newP5JourneyPair 构造共享同一 L2(Redis 模拟) 后端的两个服务实例，模拟多实例部署
func newP5JourneyPair() (*CustomerJourneyService, *CustomerJourneyService, *cache.MemoryCache) {
	l2 := cache.NewMemoryCache()
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

	// 直接改写 Redis 权威源为已报价（模拟另一实例的迁移）
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

	// L1 未过期：仍返回本地缓存值（contact），不回源
	cached := svc.GetState(ctx, "cust_p5_b")
	if cached.CurrentStage != StageContact {
		t.Fatalf("L1 hit expected contact, got %s", cached.CurrentStage)
	}

	// 回拨 TTL 触发惰性淘汰：回源 Redis 权威（quoted）
	svc.mu.Lock()
	svc.l1ExpiresAt["cust_p5_b"] = time.Now().Add(-time.Second)
	svc.mu.Unlock()

	fresh := svc.GetState(ctx, "cust_p5_b")
	if fresh.CurrentStage != StageQuoted {
		t.Errorf("after L1 expiry stage = %s, want %s (must fall back to Redis authority)", fresh.CurrentStage, StageQuoted)
	}

	// 惰性淘汰后条目应被重新装载且带新 TTL
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

	// Touch 同样双写
	svc.Touch(ctx, "cust_p5_c", "test")
	stored2 := JourneyState{}
	if err := l2.GetJSON(ctx, journeyStateKey("cust_p5_c"), &stored2); err != nil {
		t.Fatalf("redis get after touch: %v", err)
	}
	if stored2.TotalTouches != 1 {
		t.Errorf("total_touches in redis = %d, want 1", stored2.TotalTouches)
	}
}
