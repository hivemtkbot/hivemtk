import { describe, it, expect, beforeEach } from 'vitest';
import { RateLimiter } from '../src/core/rate-limiter.js';

describe('RateLimiter LRU + TTL（2026-08-14 P1-F）', () => {
  let rl;
  beforeEach(() => {
    rl = new RateLimiter({
      accountCapacity: 10,
      accountRefillPerMin: 60,
      conversationPerHour: 100,
      conversationCooldownMs: 1000,
      dedupWindowMs: 5000,
      minIntervalMs: 100,
      jitterMinMs: 10,
      jitterMaxMs: 20,
    });
  });

  it('accountBuckets 触顶 200 时驱逐最久未访问', () => {
    for (let i = 0; i < 201; i++) {
      rl.tryAcquire('douyin', `acc-${i}`, 'conv-1', '');
    }
    const stats = rl.bucketStats();
    expect(stats.accountBuckets).toBe(200);
    expect(stats.accountBucketsMax).toBe(200);
  });

  it('accountBuckets LRU：访问后保持不淘汰', () => {
    for (let i = 0; i < 200; i++) rl.tryAcquire('douyin', `acc-${i}`, 'c1', '');
    rl.tryAcquire('douyin', 'acc-0', 'c1', '');
    rl.tryAcquire('douyin', 'acc-200', 'c1', '');
    rl.__reset();
    const rl2 = new RateLimiter({ accountCapacity: 1, accountRefillPerMin: 60, conversationPerHour: 100, conversationCooldownMs: 1000, dedupWindowMs: 5000, minIntervalMs: 0, jitterMinMs: 0, jitterMaxMs: 0 });
    for (let i = 0; i < 300; i++) {
      rl2.tryAcquire('douyin', `acc-${i}`, 'c1', '');
    }
    expect(rl2.bucketStats().accountBuckets).toBeLessThanOrEqual(200);
  });

  it('convBuckets 触顶 1000 时先按 TTL 淘汰再 LRU', () => {
    for (let i = 0; i < 1500; i++) {
      rl.tryAcquire('douyin', 'acc-1', `conv-${i}`, '');
    }
    const stats = rl.bucketStats();
    expect(stats.convBuckets).toBeLessThanOrEqual(1000);
  });

  it('bucketStats 暴露所有配置', () => {
    const s = rl.bucketStats();
    expect(s).toEqual({
      accountBuckets: 0,
      accountBucketsMax: 200,
      convBuckets: 0,
      convBucketsMax: 1000,
      convBucketsTtlMs: 30 * 24 * 3600 * 1000,
    });
  });

  it('__reset 清空所有状态', () => {
    rl.tryAcquire('douyin', 'acc-1', 'conv-1', '');
    expect(rl.bucketStats().accountBuckets).toBe(1);
    rl.__reset();
    expect(rl.bucketStats().accountBuckets).toBe(0);
    expect(rl.bucketStats().convBuckets).toBe(0);
  });

  it('LRU 命中：delete + re-set 不丢桶状态', () => {
    rl.tryAcquire('douyin', 'acc-1', 'conv-1', '');
    rl.markSent('douyin', 'acc-1', 'conv-1', 'hello');
    // 第二次访问同一会话 → LRU 命中，cooldownMs=1000 期间再次请求仍被 cooldown 拦截
    // （防回环比 dedup 优先级高——同会话短时间内不重复回）。
    const r1 = rl.tryAcquire('douyin', 'acc-1', 'conv-1', 'hello2');
    expect(r1.allowed).toBe(false);
    expect(r1.reason).toMatch(/cooldown/);
    expect(rl.bucketStats().convBuckets).toBe(1);
  });
});

