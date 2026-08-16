package llm

import (
	"testing"
)

func newTestDispatcher() *Dispatcher {
	return &Dispatcher{
		providers:  make(map[string]*ProviderConfig),
		routes:     make(map[DispatchScenario]*ScenarioRoute),
		rpmCounter: make(map[string]*rpmBucket),
	}
}

func TestDispatchCacheSetGet(t *testing.T) {
	d := newTestDispatcher()
	ctx := context.Background()
	d.setCache(ctx, "k1", 10, "v1")
	if c, ok := d.getCache(ctx, "k1"); !ok || c != "v1" {
		t.Fatalf("expected v1, got %q ok=%v", c, ok)
	}
}

func TestDispatchCacheExpiry(t *testing.T) {
	d := newTestDispatcher()
	ctx := context.Background()
	d.setCache(ctx, "k1", 60, "v1")
	if v, ok := d.getCache(ctx, "k1"); !ok || v != "v1" {
		t.Fatal("expected cache hit for k1")
	}
	if _, ok := d.getCache(ctx, "missing"); ok {
		t.Fatal("expected miss for missing key")
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

