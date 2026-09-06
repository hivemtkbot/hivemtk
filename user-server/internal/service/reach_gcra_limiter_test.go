package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func gcraTestClient(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 15})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("本地 Redis 不可达，跳过 GCRA 真实用例: %v", err)
	}
	t.Cleanup(func() { _ = client.FlushDB(context.Background()).Err(); _ = client.Close() })
	return client
}

// TestGCRABasicAllow 单 key 基本放行/拒绝：QPS=5 Burst=5，前 5 个放行、第 6 个拒绝
func TestGCRABasicAllow(t *testing.T) {
	client := gcraTestClient(t)
	l := NewRedisGCRARateLimiter(client, nil)
	ctx := context.Background()

	spec := RateLimitSpec{QPS: 5, Burst: 5}
	allowed := 0
	for i := 0; i < 6; i++ {
		if l.Allow(ctx, fmt.Sprintf("gcra-basic-%d", i), spec) {
			allowed++
		}
	}

	allowedSame := 0
	for i := 0; i < 6; i++ {
		if l.Allow(ctx, "gcra-basic-same", spec) {
			allowedSame++
		}
	}
	if allowedSame != 5 {
		t.Fatalf("同 key 连发 6 次（QPS=5 Burst=5）放行 = %d, want 5", allowedSame)
	}
	_ = allowed
}

func TestGCRASmoothOutput(t *testing.T) {
	client := gcraTestClient(t)
	l := NewRedisGCRARateLimiter(client, nil)
	ctx := context.Background()
	key := "gcra-smooth"
	spec := RateLimitSpec{QPS: 10, Burst: 5}

	for i := 0; i < 5; i++ {
		if !l.Allow(ctx, key, spec) {
			t.Fatalf("burst 阶段第 %d 个被拒（应全放）", i)
		}
	}
	if l.Allow(ctx, key, spec) {
		t.Fatal("burst 耗尽后应拒绝")
	}

	time.Sleep(300 * time.Millisecond)
	refill := 0
	for i := 0; i < 10; i++ {
		if l.Allow(ctx, key, spec) {
			refill++
		}
	}
	if refill < 1 || refill > 4 {
		t.Fatalf("300ms 后补充放行数 = %d, 期望 1~4（平滑输出）", refill)
	}
}

func TestGCRAFallbackOnRedisDown(t *testing.T) {
	gcraTestClient(t)
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond})
	t.Cleanup(func() { _ = dead.Close() })

	l := NewRedisGCRARateLimiter(dead, NewMemorySendRateLimiter())
	l.retryAfter = 300 * time.Millisecond
	ctx := context.Background()
	spec := RateLimitSpec{QPS: 5, Burst: 5}

	ok := 0
	for i := 0; i < 5; i++ {
		if l.Allow(ctx, "gcra-down", spec) {
			ok++
		}
	}
	if !l.degraded.Load() {
		t.Fatal("Redis 不可达应进入降级态")
	}
	if ok != 5 {
		t.Fatalf("降级后进程内限流应按 burst 放行 5 个, got %d", ok)
	}

	_ = l.Allow(ctx, "gcra-down", spec)
	if !l.degraded.Load() {
		t.Fatal("降级窗口内不应提前探测恢复")
	}
}

// TestGCRAZeroLimit QPS/Burst 均为 0 时直接放行（与 Memory 语义一致）
func TestGCRAZeroLimit(t *testing.T) {
	client := gcraTestClient(t)
	l := NewRedisGCRARateLimiter(client, nil)
	if !l.Allow(context.Background(), "gcra-zero", RateLimitSpec{}) {
		t.Fatal("空 spec 应放行")
	}
}

// TestGCRAReset 清 key 后重新放行
func TestGCRAReset(t *testing.T) {
	client := gcraTestClient(t)
	l := NewRedisGCRARateLimiter(client, nil)
	ctx := context.Background()
	key := "gcra-reset"
	spec := RateLimitSpec{QPS: 1, Burst: 1}
	if !l.Allow(ctx, key, spec) {
		t.Fatal("首次应放行")
	}
	if l.Allow(ctx, key, spec) {
		t.Fatal("burst=1 第二次应拒绝")
	}
	l.Reset(ctx, key)
	if !l.Allow(ctx, key, spec) {
		t.Fatal("Reset 后应重新放行")
	}
}
