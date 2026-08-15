import { describe, it, expect, beforeEach } from 'vitest';
import { CircuitBreaker, circuitBreaker } from '../src/core/circuit-breaker.js';

describe('CircuitBreaker 三态机（2026-08-14 P0-B）', () => {
  let cb;
  beforeEach(() => {
    cb = new CircuitBreaker({ failureThreshold: 3, openDurationMs: 100, deadManSeconds: 1 });
  });

  it('初始状态：CLOSED、failureCount=0', () => {
    expect(cb.state).toBe('CLOSED');
    expect(cb.failureCount).toBe(0);
    expect(cb.isOpen()).toBe(false);
  });

  it('CLOSED：连续 3 次失败后跳到 OPEN', () => {
    cb.recordFailure(new Error('e1'));
    cb.recordFailure(new Error('e2'));
    expect(cb.isOpen()).toBe(false);
    cb.recordFailure(new Error('e3'));
    expect(cb.state).toBe('OPEN');
    expect(cb.isOpen()).toBe(true);
  });

  it('OPEN：isOpen() 阻断请求', () => {
    cb.failureCount = 3;
    cb.state = 'OPEN';
    cb.openedAt = Date.now();
    expect(cb.isOpen()).toBe(true);
  });

  it('OPEN：openDurationMs 后进入 HALF_OPEN', async () => {
    cb.failureCount = 3;
    cb.state = 'OPEN';
    cb.openedAt = Date.now() - 200; 
    expect(cb.isOpen()).toBe(false);
    expect(cb.state).toBe('HALF_OPEN');
  });

  it('HALF_OPEN：探测成功 → CLOSED，重置 failureCount', () => {
    cb.state = 'HALF_OPEN';
    cb.halfOpenSuccessCount = 0;
    cb.recordSuccess();
    expect(cb.state).toBe('CLOSED');
    expect(cb.failureCount).toBe(0);
  });

  it('HALF_OPEN：探测失败 → 重新 OPEN', () => {
    cb.state = 'HALF_OPEN';
    cb.openedAt = 0;
    cb.recordFailure(new Error('探测失败'));
    expect(cb.state).toBe('OPEN');
    expect(cb.openedAt).toBeGreaterThan(0);
  });

  it('CLOSED：成功重置 failureCount', () => {
    cb.recordFailure(new Error('e1'));
    cb.recordFailure(new Error('e2'));
    expect(cb.failureCount).toBe(2);
    cb.recordSuccess();
    expect(cb.failureCount).toBe(0);
    expect(cb.state).toBe('CLOSED');
  });

  it('snapshot 返回死开关信号', () => {
    const s = cb.snapshot();
    expect(s).toMatchObject({
      state: 'CLOSED',
      failureCount: 0,
      lastSuccessAt: 0,
      lastFailureAt: 0,
      recentReasons: [],
      deadManSeconds: 1,
    });
    expect(s.healthy).toBe(true);
  });

  it('snapshot.healthy：无任何调用时 healthy=true（无失败即健康）', () => {
    const s = cb.snapshot();
    expect(s.healthy).toBe(true);
  });

  it('snapshot.healthy：成功后 5 秒内 healthy=true（默认 deadManSeconds=5）', () => {
    const c = new CircuitBreaker({ deadManSeconds: 5 });
    c.recordSuccess();
    const s = c.snapshot();
    expect(s.healthy).toBe(true);
  });

  it('snapshot.recentReasons：去重后最多保留 5 种', () => {
    for (let i = 0; i < 10; i++) {
      cb.recordFailure(new Error(`reason-${i % 7}`));
    }
    const s = cb.snapshot();
    expect(s.recentReasons.length).toBeLessThanOrEqual(5);
  });

  it('reset() 回到初始状态', () => {
    cb.failureCount = 3;
    cb.state = 'OPEN';
    cb.openedAt = Date.now();
    cb.reset();
    expect(cb.state).toBe('CLOSED');
    expect(cb.failureCount).toBe(0);
    expect(cb.openedAt).toBe(0);
  });

  it('全局单例 circuitBreaker 存在且类型正确', () => {
    expect(circuitBreaker).toBeInstanceOf(CircuitBreaker);
    expect(typeof circuitBreaker.isOpen).toBe('function');
    expect(typeof circuitBreaker.recordSuccess).toBe('function');
    expect(typeof circuitBreaker.recordFailure).toBe('function');
  });
});


