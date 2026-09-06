package service

import (
	"context"
	"testing"

	agent_runtime "hivemtk-user/internal/aiagent/agent/runtime"
	"hivemtk-user/internal/cache"
)

// 本文件提供 -count=N 跨测试状态隔离的共享 helper。
//
// 背景：进程级单例 cache.GetGlobalCache() 与 agent_runtime 的 reply guard
//（internal/aiagent/agent/runtime/idempotency.go）在 go test -count=2 时会把
// 上一轮测试残留的键/认领带到下一轮，导致去重命中（skip duplicate outbound）、
// AI 排他锁静默跳过、旅程 Transition 返回 (nil, nil) 等假失败。
// 统一修法：测试内用独立内存缓存 + 测试前后释放 reply 认领。

// newIsolatedCacheForTest 返回与本测试绑定的独立内存缓存（测试结束自动 Close）。
// 需要传给 NewInboxIngressServiceWithDB / NewCustomerJourneyServiceWithCache 等
// 构造器，替代 nil（nil 会回退到进程级全局单例，造成 -count=N 串扰）。
func newIsolatedCacheForTest(t *testing.T) cache.Cache {
	t.Helper()
	memCache := cache.NewMemoryCache()
	t.Cleanup(memCache.Close)
	return memCache
}

// releaseReplyClaimForTest 释放指定 EventID 的出站回复认领（claim-before-confirm
// 幂等守卫）。固定 EventID 的测试在 -count=2 时，第二轮会被上一轮认领拦截
// （"skip duplicate outbound"），因此测试前先清一次，Cleanup 里再清一次。
func releaseReplyClaimForTest(t *testing.T, eventIDs ...string) {
	t.Helper()
	release := func() {
		for _, id := range eventIDs {
			if id != "" {
				agent_runtime.ReleaseReply(id)
			}
		}
	}
	release()
	t.Cleanup(release)
}

// cleanupWebhookDedupKeysForTest 删除指定 EventID 的 webhook 去重键
// （mtk:webhook:dedup:<eventID>，TTL 5 分钟，落在全局缓存上）。
// 供直接依赖全局缓存的 isDuplicate 测试在 -count=2 时做前后清理。
func cleanupWebhookDedupKeysForTest(t *testing.T, eventIDs ...string) {
	t.Helper()
	ctx := context.Background()
	cleanup := func() {
		for _, id := range eventIDs {
			if id == "" {
				continue
			}
			_ = cache.GetGlobalCache().Delete(ctx, "mtk:webhook:dedup:"+id)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
}

// cleanupTgOutreachKeysForTest 清理 Telegram 触达双层冷却键（TTL 30 分钟，
// 落在全局缓存上）：mtk:tg:outreach:<acc>:<chat>:<sender> 与
// mtk:tg:dm_outreach:<acc>:<uid>:<group>。
// -count=2 第二轮会因上一轮残留的冷却键被静默拦截，导致断言失败。
func cleanupTgOutreachKeysForTest(t *testing.T, accountID, chatID string, senderIDs ...string) {
	t.Helper()
	ctx := context.Background()
	cleanup := func() {
		for _, sender := range senderIDs {
			if sender == "" {
				continue
			}
			_ = cache.GetGlobalCache().Delete(ctx, "mtk:tg:outreach:"+accountID+":"+chatID+":"+sender)
			_ = cache.GetGlobalCache().Delete(ctx, "mtk:tg:outreach:"+accountID+":dm:"+sender)
			_ = cache.GetGlobalCache().Delete(ctx, "mtk:tg:dm_outreach:"+accountID+":"+sender+":"+chatID)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
}

// newIsolatedJourneyService 创建使用独立内存缓存的客户旅程服务。
// NewCustomerJourneyService() 默认注入进程级全局缓存，旅程状态键
// journey:state:* TTL 长，-count=2 第二轮读到旧阶段会让 Transition 返回
// (nil, nil)（同阶段幂等短路），调用方断言 event 非 nil 即 panic。
func newIsolatedJourneyService(t *testing.T) *CustomerJourneyService {
	t.Helper()
	return NewCustomerJourneyServiceWithCache(newIsolatedCacheForTest(t))
}
