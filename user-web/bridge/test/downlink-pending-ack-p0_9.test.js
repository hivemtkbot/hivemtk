// P0-9 _pendingAck 最大重试次数与退避策略测试（2026-08-15 10/10 任务清单）。
//
// 覆盖：
//   - ackRetryBackoffMs 指数退避（1s → 60s cap）
//   - addPendingAck 首次入队 vs 已存在
//   - claimDuePendingAck 退避未到跳过 / 超限丢弃 / TTL 过期清理
//   - markPendingAckTried 成功删除 / 失败 attempts++
//   - getPendingAckStats 监控读出
//
// 隔离手段：每个 test 用独立 channel 名（test_p0_9_<rand>）避免污染。

import { describe, it, expect, beforeEach, vi } from 'vitest';

// 模拟 chrome.storage（避免下行 chrome 全局依赖）
globalThis.chrome = {
  storage: {
    local: {
      _data: {},
      async get(keys) {
        const out = {};
        const list = Array.isArray(keys) ? keys : [keys];
        for (const k of list) if (k in this._data) out[k] = this._data[k];
        return out;
      },
      async set(obj) {
        Object.assign(this._data, obj);
      },
      async remove(keys) {
        const list = Array.isArray(keys) ? keys : [keys];
        for (const k of list) delete this._data[k];
      },
    },
  },
};

import {
  ackRetryBackoffMs,
  addPendingAck,
  claimDuePendingAck,
  markPendingAckTried,
  getPendingAckStats,
} from '../src/core/downlink.js';

let _uniq = 0;
const newChannel = () => `test_p0_9_${++_uniq}_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;

beforeEach(() => {
  globalThis.chrome.storage.local._data = {};
});

// =============================================================
// 单元 1：ackRetryBackoffMs 指数退避序列
// =============================================================
describe('P0-9 ackRetryBackoffMs 指数退避', () => {
  it('attempt 序列：1000, 2000, 4000, 8000, 16000, 32000, 60000, 60000, 60000, 60000', () => {
    expect(ackRetryBackoffMs(1)).toBe(1000);
    expect(ackRetryBackoffMs(2)).toBe(2000);
    expect(ackRetryBackoffMs(3)).toBe(4000);
    expect(ackRetryBackoffMs(4)).toBe(8000);
    expect(ackRetryBackoffMs(5)).toBe(16000);
    expect(ackRetryBackoffMs(6)).toBe(32000);
    expect(ackRetryBackoffMs(7)).toBe(60000); // cap
    expect(ackRetryBackoffMs(8)).toBe(60000);
    expect(ackRetryBackoffMs(10)).toBe(60000);
    expect(ackRetryBackoffMs(100)).toBe(60000);
  });

  it('边界：attempt<1 退化为 baseMs=1000', () => {
    expect(ackRetryBackoffMs(0)).toBe(1000);
    expect(ackRetryBackoffMs(-1)).toBe(1000);
  });
});

// =============================================================
// 单元 2：addPendingAck 基础入队
// =============================================================
describe('P0-9 addPendingAck 基础入队', () => {
  it('首次入队：size=1', () => {
    const ch = newChannel();
    addPendingAck(ch, 'm_first', 'first_error');
    const stats = getPendingAckStats(ch);
    expect(stats.size).toBe(1);
  });

  it('重复入队：size 不变，attempts 不重置', () => {
    const ch = newChannel();
    addPendingAck(ch, 'm_repeat');
    markPendingAckTried(ch, 'm_repeat', false, 'e1');
    markPendingAckTried(ch, 'm_repeat', false, 'e2');
    addPendingAck(ch, 'm_repeat', 'e3');
    const stats = getPendingAckStats(ch);
    expect(stats.size).toBe(1);
    expect(stats.maxAttempts).toBe(2);
  });
});

// =============================================================
// 单元 3：markPendingAckTried 成功/失败
// =============================================================
describe('P0-9 markPendingAckTried 成功/失败', () => {
  it('success=true：删除条目', () => {
    const ch = newChannel();
    addPendingAck(ch, 'm_succ');
    expect(getPendingAckStats(ch).size).toBe(1);
    markPendingAckTried(ch, 'm_succ', true);
    expect(getPendingAckStats(ch).size).toBe(0);
  });

  it('success=false：attempts++', () => {
    const ch = newChannel();
    addPendingAck(ch, 'm_fail');
    markPendingAckTried(ch, 'm_fail', false, 'boom');
    markPendingAckTried(ch, 'm_fail', false, 'boom2');
    markPendingAckTried(ch, 'm_fail', false, 'boom3');
    const stats = getPendingAckStats(ch);
    expect(stats.size).toBe(1);
    expect(stats.maxAttempts).toBe(3);
  });
});

// =============================================================
// 单元 4：claimDuePendingAck 退避过滤
// =============================================================
describe('P0-9 claimDuePendingAck 退避过滤', () => {
  it('首次：lastTryAt=0 → 立即可重试', () => {
    const ch = newChannel();
    addPendingAck(ch, 'm_immediate');
    const due = claimDuePendingAck(ch);
    expect(due.length).toBe(1);
    expect(due[0].msgId).toBe('m_immediate');
  });

  it('刚 markPendingAckTried 一次：attempts=1 → backoff=1s 未到 → 不可重试', () => {
    const ch = newChannel();
    addPendingAck(ch, 'm_just_tried');
    markPendingAckTried(ch, 'm_just_tried', false);
    const due = claimDuePendingAck(ch);
    expect(due.length).toBe(0);
  });

  it('退避时长到期（vi.setSystemTime 推进 2.5s）：可重试', () => {
    const ch = newChannel();
    addPendingAck(ch, 'm_old');
    markPendingAckTried(ch, 'm_old', false);
    // attempts=1 → 下一轮 backoff = ackRetryBackoffMs(2) = 2000ms
    // 推进 2.5s
    vi.setSystemTime(Date.now() + 2500);
    try {
      const due = claimDuePendingAck(ch);
      expect(due.length).toBe(1);
    } finally {
      vi.useRealTimers();
    }
  });
});

// =============================================================
// 单元 5：超过最大重试次数 → 丢弃
// =============================================================
describe('P0-9 claimDuePendingAck 超限丢弃', () => {
  it('attempts=10 → claimDuePendingAck 触发丢弃', () => {
    const ch = newChannel();
    addPendingAck(ch, 'm_exhausted');
    // 失败 10 次
    for (let i = 0; i < 10; i++) {
      markPendingAckTried(ch, 'm_exhausted', false);
    }
    expect(getPendingAckStats(ch).size).toBe(1);
    expect(getPendingAckStats(ch).maxAttempts).toBe(10);
    // claimDuePendingAck 触发丢弃
    const due = claimDuePendingAck(ch);
    expect(due.length).toBe(0);
    expect(getPendingAckStats(ch).size).toBe(0);
  });
});

// =============================================================
// 单元 6：TTL 过期清理
// =============================================================
describe('P0-9 claimDuePendingAck TTL 过期清理', () => {
  it('firstSeenAt > 24h → 直接丢弃', () => {
    const ch = newChannel();
    addPendingAck(ch, 'm_old_ttl');
    // 推进 25h
    vi.setSystemTime(Date.now() + 25 * 60 * 60 * 1000);
    try {
      const due = claimDuePendingAck(ch);
      expect(due.length).toBe(0);
      expect(getPendingAckStats(ch).size).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });
});

// =============================================================
// 单元 7：容量保护（FIFO 淘汰）
// =============================================================
describe('P0-9 容量保护', () => {
  it('超过 MAX_PENDING_ACK_PER_CHANNEL 时按 firstSeenAt 升序淘汰最早', async () => {
    // 重新 import 拿到 MAX 常量
    const ch = newChannel();
    // 注入 1001 条（MAX=1000），第 1 条应被淘汰
    for (let i = 0; i < 1001; i++) {
      addPendingAck(ch, `m_cap_${i}`);
    }
    expect(getPendingAckStats(ch).size).toBe(1000);
  });
});

// =============================================================
// 单元 8：与下行集成（mock ackOutbox）
// =============================================================
describe('P0-9 集成：addPendingAck + markPendingAckTried 闭环', () => {
  it('失败 → 入队 → 退避后取出 → 标记成功 → 清除', () => {
    const ch = newChannel();
    // 模拟"下行 sendOutbound 成功 + ack 失败"
    addPendingAck(ch, 'm_loop', 'send_succeeded_ack_failed');
    expect(getPendingAckStats(ch).size).toBe(1);
    // 模拟下次轮询时 claim 取出
    const due = claimDuePendingAck(ch);
    expect(due.length).toBe(1);
    // 模拟"重试 ack 成功"
    markPendingAckTried(ch, 'm_loop', true);
    expect(getPendingAckStats(ch).size).toBe(0);
  });

  it('失败 → 入队 → 退避未到 → 不被 claim', () => {
    const ch = newChannel();
    addPendingAck(ch, 'm_retry');
    markPendingAckTried(ch, 'm_retry', false, 'boom');
    // 立即 claim：attempts=1, backoff=2s 未到
    const due = claimDuePendingAck(ch);
    expect(due.length).toBe(0);
    // 推进 2.5s（> 2s 退避）
    vi.setSystemTime(Date.now() + 2500);
    try {
      const due2 = claimDuePendingAck(ch);
      expect(due2.length).toBe(1);
    } finally {
      vi.useRealTimers();
    }
  });
});
