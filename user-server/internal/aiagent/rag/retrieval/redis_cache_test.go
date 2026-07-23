package ragretrieval

import (
	"context"
	"testing"
	"time"

	"marketing/internal/cache"
)

// TestRedisBackedCacheSearchResultRoundTrip 验证适配器通过 JSON 信封保留具体 Go 类型，
// 使 RAG 检索热路径中的类型断言 cached.([]SearchResult) 仍然成立。
func TestRedisBackedCacheSearchResultRoundTrip(t *testing.T) {
	backend := cache.NewMemoryCache()
	defer backend.Close()
	c := NewRedisBackedCache(backend)

	in := []SearchResult{
		{DocumentID: "1", Content: "hello", Score: 0.9, Metadata: map[string]any{"k": "v"}},
	}
	c.Set("k1", in, time.Minute)

	got, ok := c.Get("k1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	out, ok := got.([]SearchResult)
	if !ok {
		t.Fatalf("type assertion failed, got %T", got)
	}
	if len(out) != 1 || out[0].DocumentID != "1" || out[0].Content != "hello" {
		t.Fatalf("unexpected value: %+v", out)
	}
	if out[0].Metadata["k"] != "v" {
		t.Fatalf("metadata lost: %+v", out[0].Metadata)
	}

	// 未命中
	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss for missing key")
	}

	// 删除
	c.Delete("k1")
	if _, ok := c.Get("k1"); ok {
		t.Fatal("expected miss after delete")
	}
}
