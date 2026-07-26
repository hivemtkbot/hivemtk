package agent_runtime

import (
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestReplyGuard_ExactlyOnce 验证同一 EventID 仅首条链路可认领出站（双触发地雷核心防线）。
func TestReplyGuard_ExactlyOnce(t *testing.T) {
	if !ClaimReply("evt-1") {
		t.Fatal("first claim should succeed")
	}
	if ClaimReply("evt-1") {
		t.Fatal("second claim for same event must be denied")
	}
	if !HasReplied("evt-1") {
		t.Fatal("evt-1 should be marked replied")
	}
	// 不同事件仍可认领，互不干扰
	if !ClaimReply("evt-2") {
		t.Fatal("different event should still be claimable")
	}
}

// TestReplyGuard_EmptyEventID 空 EventID 退化为允许，避免误拦截正常消息。
func TestReplyGuard_EmptyEventID(t *testing.T) {
	if !ClaimReply("") {
		t.Fatal("empty eventID should be allowed (degraded mode)")
	}
	if HasReplied("") {
		t.Fatal("empty eventID must not be treated as replied")
	}
}

// TestReplyGuard_TTLSweep 大量不同事件不应 panic，且 TTL 惰性清理可正常运行。
func TestReplyGuard_TTLSweep(t *testing.T) {
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("sweep-%d", i)
		if !ClaimReply(id) {
			t.Fatalf("event %s should be claimable", id)
		}
	}
}

// TestReplyGuard_Release 出站失败后释放认领，允许平台重投重新认领。
func TestReplyGuard_Release(t *testing.T) {
	if !ClaimReply("rel-1") {
		t.Fatal("first claim should succeed")
	}
	if ClaimReply("rel-1") {
		t.Fatal("second claim must be denied")
	}
	ReleaseReply("rel-1")
	if !ClaimReply("rel-1") {
		t.Fatal("after release, claim should succeed again (enables retry on failed outbound)")
	}
	// 空 EventID 释放为 no-op，不应 panic
	ReleaseReply("")
}

// TestReplyGuard_RedisNilStaysLocal SetReplyGuardRedis(nil) 不改变默认进程内守卫。
func TestReplyGuard_RedisNilStaysLocal(t *testing.T) {
	SetReplyGuardRedis(nil)
	if !ClaimReply("nil-1") {
		t.Fatal("after SetReplyGuardRedis(nil), local claim should still work")
	}
	if ClaimReply("nil-1") {
		t.Fatal("second claim must be denied")
	}
	ReleaseReply("nil-1")
}

// TestReplyGuard_RedisUnavailableFallsBackToLocal Redis 不可达时降级到进程内守卫，
// 保证单实例下消息不丢、出站不被阻断（R7 优雅降级核心语义）。
func TestReplyGuard_RedisUnavailableFallsBackToLocal(t *testing.T) {
	bad := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1", // 必然拒绝连接
		DialTimeout: 500 * time.Millisecond,
	})
	defer bad.Close()
	SetReplyGuardRedis(bad) // Ping 失败 → 保持进程内守卫
	// 降级后行为应与纯进程内一致
	if !ClaimReply("deg-1") {
		t.Fatal("degraded mode should still claim successfully")
	}
	if ClaimReply("deg-1") {
		t.Fatal("degraded mode should deny second claim")
	}
	ReleaseReply("deg-1")
	if !ClaimReply("deg-1") {
		t.Fatal("after release in degraded mode, re-claim should succeed")
	}
	ReleaseReply("deg-1")
}
