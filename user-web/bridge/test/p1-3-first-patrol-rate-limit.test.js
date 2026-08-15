
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { BaseAdapter } from '../src/core/channel-adapter.js';
import { PATROL_DEFAULTS } from '../src/core/types.js';

// 构造 N 个未 seen 的消息项
function makeItems(n) {
  const items = [];
  for (let i = 0; i < n; i++) {
    items.push({
      textContent: `msg${i}`,
      matches: () => false,
      closest: () => null,
      getAttribute: () => null,
      querySelector: () => null,
      outerHTML: `<div>msg${i}</div>`,
    });
  }
  return items;
}

function makeAdapter(items) {
  return new BaseAdapter({
    name: 'test',
    channel: 'xiaohongshu',
    hooks: {
      getMessageItems: () => items,
      parseMessageItem: (it) => ({
        message_id: `m${it.textContent}`,
        text: it.textContent,
        msg_type: 'text',
        sender_type: 'customer',
        timestamp: Date.now(),
      }),
      getConversationId: () => 'cid-test',
    },
  });
}

describe('P1-3 首次 L1 巡检限速/分批', () => {
  beforeEach(() => {
    if (typeof localStorage !== 'undefined') localStorage.clear();
  });

  it('首次 patrol：单会话单次抓取上限 20（防首次涌入）', () => {
    const adapter = makeAdapter(makeItems(80));
    expect(adapter._lastPatrolAt).toBeUndefined();
    expect(adapter._isFirstPatrolRun()).toBe(true);

    const batch = adapter._collectUnseenText();
    expect(batch.length).toBe(PATROL_DEFAULTS.firstRunMaxBatch);
    expect(batch.length).toBeLessThanOrEqual(20);
    expect(batch.length).toBeLessThan(PATROL_DEFAULTS.maxBatchPerPatrol);
  });

  it('非首次 patrol：单会话单次抓取上限 80（maxBatchPerPatrol）', () => {
    const adapter = makeAdapter(makeItems(100));
    adapter._lastPatrolAt = Date.now() - 30 * 1000;
    expect(adapter._isFirstPatrolRun()).toBe(false);

    const batch = adapter._collectUnseenText();
    expect(batch.length).toBe(PATROL_DEFAULTS.maxBatchPerPatrol);
    expect(batch.length).toBe(80);
  });

  it('patrol 完成后更新 _lastPatrolAt，第二次 patrol 不再视为首次', () => {
    const adapter = makeAdapter(makeItems(10));
    expect(adapter._isFirstPatrolRun()).toBe(true);
    adapter._lastPatrolAt = Date.now();
    expect(adapter._isFirstPatrolRun()).toBe(false);
  });

  it('距上次 patrol > firstRunWindowMs：重新视为首次（popup 关闭重开场景）', () => {
    const adapter = makeAdapter(makeItems(10));
    adapter._lastPatrolAt = Date.now() - 2 * 60 * 1000;
    expect(adapter._isFirstPatrolRun()).toBe(true);
  });

  it('距上次 patrol 接近 firstRunWindowMs 边界：仍视为非首次', () => {
    const adapter = makeAdapter(makeItems(10));
    adapter._lastPatrolAt = Date.now() - 50 * 1000;
    expect(adapter._isFirstPatrolRun()).toBe(false);
  });

  it('首次巡检抓满 20 条后告警：剩余靠下轮扫描补齐', () => {
    const adapter = makeAdapter(makeItems(50));
    const warnSpy = vi.spyOn(adapter.log, 'warn').mockImplementation(() => {});
    const batch = adapter._collectUnseenText();
    expect(batch.length).toBe(20);
    // 应打「[首次] 巡检抓取消息数达上限 20」warn
    const hit = warnSpy.mock.calls.some(([msg]) => /首次.*上限\s*20/.test(String(msg)));
    expect(hit).toBe(true);
    warnSpy.mockRestore();
  });

  it('非首次巡检抓满 80 条后告警：剩余靠下轮扫描补齐', () => {
    const adapter = makeAdapter(makeItems(120));
    adapter._lastPatrolAt = Date.now() - 30 * 1000; 
    const warnSpy = vi.spyOn(adapter.log, 'warn').mockImplementation(() => {});
    const batch = adapter._collectUnseenText();
    expect(batch.length).toBe(80);
    const hit = warnSpy.mock.calls.some(([msg]) => /常规.*上限\s*80/.test(String(msg)));
    expect(hit).toBe(true);
    warnSpy.mockRestore();
  });

  it('firstRunMaxBatch 必须是 maxBatchPerPatrol 的子集（确保分批更激进）', () => {
    expect(PATROL_DEFAULTS.firstRunMaxBatch).toBeLessThanOrEqual(PATROL_DEFAULTS.maxBatchPerPatrol);
  });

  it('firstRunWindowMs 必须为正数（避免除零/始终首次）', () => {
    expect(PATROL_DEFAULTS.firstRunWindowMs).toBeGreaterThan(0);
  });
});


