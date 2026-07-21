package llm

import (
	"sync"
	"testing"
	"time"
)

func newTestDispatcher() *Dispatcher {
	return &Dispatcher{
		providers:  make(map[string]*ProviderConfig),
		routes:     make(map[DispatchScenario]*ScenarioRoute),
		rpmCounter: make(map[string]*rpmBucket),
		cache:      make(map[string]*dispatchCacheEntry),
		cacheMu:    sync.RWMutex{},
	}
}

func TestDispatchCacheSetGet(t *testing.T) {
	d := newTestDispatcher()
	d.setCache("k1", 10, "v1")
	if c, ok := d.getCache("k1"); !ok || c != "v1" {
		t.Fatalf("expected v1, got %q ok=%v", c, ok)
	}
}

func TestDispatchCacheExpiry(t *testing.T) {
	d := newTestDispatcher()
	d.setCache("k2", 60, "v2")
	d.cache["k2"].expireAt = time.Now().Add(-time.Second)
	if _, ok := d.getCache("k2"); ok {
		t.Fatal("expected expired entry to miss")
	}
}

func TestDispatchCacheZeroTTLNoop(t *testing.T) {
	d := newTestDispatcher()
	d.setCache("k3", 0, "v3")
	if _, ok := d.getCache("k3"); ok {
		t.Fatal("zero TTL should not store")
	}
}

func TestEstimateTokens(t *testing.T) {
	if estimateTokens("") != 0 {
		t.Fatal("empty text should be 0 tokens")
	}
	if estimateTokens("hello") <= 0 {
		t.Fatal("ascii text should be > 0 tokens")
	}
	if estimateTokens("你好世界") <= 0 {
		t.Fatal("cjk text should be > 0 tokens")
	}
}

func TestCacheKeyStableAndDistinct(t *testing.T) {
	a := CacheKey(ScenarioSOPReply, "  prompt  ")
	b := CacheKey(ScenarioSOPReply, "prompt")
	if a != b {
		t.Fatal("trim should produce same key")
	}
	if CacheKey(ScenarioSOPReply, "x") == CacheKey(ScenarioIntentRecognize, "x") {
		t.Fatal("different scenarios should produce different keys")
	}
}
