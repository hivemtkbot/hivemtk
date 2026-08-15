import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { RateLimiter } from '../src/core/rate-limiter.js';
import { SENDER, DIRECTION } from '../src/core/types.js';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe('RateLimiter 风控', () => {
  it('首次获取放行，并计算拟人延迟', () => {
    const rl = new RateLimiter();
    const d = rl.tryAcquire('douyin_web', 'A', 'C', 'hi');
    expect(d.allowed).toBe(true);
    expect(d.waitHintMs).toBeGreaterThanOrEqual(800); 
    rl.markSent('douyin_web', 'A', 'C', 'hi');
  });

  it('最小间隔内再次获取被拦截（minInterval）', () => {
    const rl = new RateLimiter();
    rl.tryAcquire('douyin_web', 'A', 'C', 'hi');
    rl.markSent('douyin_web', 'A', 'C', 'hi');
    vi.advanceTimersByTime(200); 
    const d = rl.tryAcquire('douyin_web', 'A', 'C', 'hi2');
    expect(d.allowed).toBe(false);
    expect(d.reason).toContain('minInterval');
  });

  it('跨过最小间隔但仍在会话冷却内被拦截（cooldown）', () => {
    const rl = new RateLimiter();
    rl.tryAcquire('douyin_web', 'A', 'C', 'hi');
    rl.markSent('douyin_web', 'A', 'C', 'hi');
    vi.advanceTimersByTime(2000); 
    const d = rl.tryAcquire('douyin_web', 'A', 'C', 'hi2');
    expect(d.allowed).toBe(false);
    expect(d.reason).toContain('cooldown');
  });

  it('冷却后但相同文案在去重窗口内被拦截（dedup）', () => {
    const rl = new RateLimiter();
    rl.tryAcquire('douyin_web', 'A', 'C', 'same');
    rl.markSent('douyin_web', 'A', 'C', 'same');
    vi.advanceTimersByTime(3500); 
    const d = rl.tryAcquire('douyin_web', 'A', 'C', 'same');
    expect(d.allowed).toBe(false);
    expect(d.reason).toContain('dedup');
  });

  it('去重窗口之后允许再次发送', () => {
    const rl = new RateLimiter();
    rl.tryAcquire('douyin_web', 'A', 'C', 'same');
    rl.markSent('douyin_web', 'A', 'C', 'same');
    vi.advanceTimersByTime(61000); 
    const d = rl.tryAcquire('douyin_web', 'A', 'C', 'same');
    expect(d.allowed).toBe(true);
  });

  it('超限后令牌桶退款，不污染后续配额', () => {
    const rl = new RateLimiter({ accountCapacity: 1, accountRefillPerMin: 1 });
    const d1 = rl.tryAcquire('x', 'A', 'C', 'm1');
    expect(d1.allowed).toBe(true);
    // 立即再获取：账户桶空 -> 拦截，但应退款（令牌恢复）
    const d2 = rl.tryAcquire('x', 'A', 'C', 'm2');
    expect(d2.allowed).toBe(false);
    vi.advanceTimersByTime(60000);
    const d3 = rl.tryAcquire('x', 'A', 'C', 'm3');
    expect(d3.allowed).toBe(true);
  });
});

