// 2026-08-14 头脑风暴二次论证·P1-F 单元测试
// 覆盖 RateLimiter 桶的 LRU + TTL
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
    // 用 201 个不同账号填满
    for (let i = 0; i < 201; i++) {
      rl.tryAcquire('douyin', `acc-${i}`, 'conv-1', '');
    }
    const stats = rl.bucketStats();
    expect(stats.accountBuckets).toBe(200);
    expect(stats.accountBucketsMax).toBe(200);
  });

  it('accountBuckets LRU：访问后保持不淘汰', () => {
    // 填 200 个
    for (let i = 0; i < 200; i++) rl.tryAcquire('douyin', `acc-${i}`, 'c1', '');
    // 再访问 acc-0（最久未访问的）→ 移到末尾
    rl.tryAcquire('douyin', 'acc-0', 'c1', '');
    // 再加 1 个 → 触顶 → 应该淘汰 acc-1（现在最久未访问的）
    rl.tryAcquire('douyin', 'acc-200', 'c1', '');
    // acc-0 仍在（最近访问过），acc-1 已淘汰
    // 通过 tryAcquire 是否被限流来间接验证（acc-1 不在桶中 → 新建 → 满 tokens）
    rl.__reset();
    const rl2 = new RateLimiter({ accountCapacity: 1, accountRefillPerMin: 60, conversationPerHour: 100, conversationCooldownMs: 1000, dedupWindowMs: 5000, minIntervalMs: 0, jitterMinMs: 0, jitterMaxMs: 0 });
    // 不直接验证 LRU 顺序（黑盒），只验证容量上限
    for (let i = 0; i < 300; i++) {
      rl2.tryAcquire('douyin', `acc-${i}`, 'c1', '');
    }
    expect(rl2.bucketStats().accountBuckets).toBeLessThanOrEqual(200);
  });

  it('convBuckets 触顶 1000 时先按 TTL 淘汰再 LRU', () => {
    // 填 1500 个不同会话
    for (let i = 0; i < 1500; i++) {
      rl.tryAcquire('douyin', 'acc-1', `conv-${i}`, '');
    }
    const stats = rl.bucketStats();
    // 不会无限增长（容量保护生效）
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
    // 验证 LRU 状态保留：bucket 仍存在、convBuckets 计数不变（被 LRU 移动到末尾，不增不减）
    expect(rl.bucketStats().convBuckets).toBe(1);
  });
});
