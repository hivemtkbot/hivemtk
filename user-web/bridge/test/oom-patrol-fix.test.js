// 验证 2026-08-05 OOM 巡检 + 巡检机制优化 + 抖音自他识别 修复
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { BaseAdapter } from '../src/core/channel-adapter.js';
import { PATROL_DEFAULTS } from '../src/core/types.js';

// mock DOM
global.document = {
  body: { tagName: 'BODY' },
  querySelector: () => null,
  querySelectorAll: () => [],
  createElement: () => ({ tagName: 'DIV', children: [], classList: { add: () => {}, contains: () => false } }),
};

describe('OOM 巡检 修复', () => {
  it('MutationObserver 不再监听 characterData（防抖屏 OOM）', () => {
    // 验证 _observe 不会传 characterData:true
    const observed = {};
    const OrigMO = global.MutationObserver;
    global.MutationObserver = class {
      observe(_root, opts) { Object.assign(observed, opts); }
      disconnect() {}
    };
    try {
      const fakeRoot = document.createElement('div');
      const adapter = new BaseAdapter({ name: 'test', channel: 'xiaohongshu', hooks: { getMessageListRoot: () => fakeRoot } });
      adapter._observe(fakeRoot);
      expect('characterData' in observed).toBe(false);
      expect(observed.childList).toBe(true);
      expect(observed.subtree).toBe(true);
    } finally {
      global.MutationObserver = OrigMO;
    }
  });

  it('内容指纹去重已移至后端（纯桥接：adapter 不再维护 seen Set）', () => {
    // 2026-08-05 架构重构：bridge 只做桥接，内容去重交给后端统一收信中心。
    // adapter.seen Set 已移除，仅保留 seenNodes WeakSet（防 DOM 重复扫描的技术手段）。
    const adapter = new BaseAdapter({ name: 'test', channel: 'xiaohongshu' });
    expect(adapter.seen).toBeUndefined();
    expect(adapter.seenNodes).toBeInstanceOf(WeakSet);
  });

  it('_collectUnseenText 单次封顶 80 条（防 OOM）', async () => {
    // 构造 200 个未 seen 的消息项
    const items = [];
    for (let i = 0; i < 200; i++) {
      items.push({
        textContent: `msg${i}`,
        matches: () => false,
        closest: () => null,
        getAttribute: () => null,
        querySelector: () => null,
        outerHTML: `<div>msg${i}</div>`,
      });
    }
    const adapter = new BaseAdapter({
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
        getConversationId: () => 'cid',
      },
    });
    const batch = adapter._collectUnseenText();
    expect(batch.length).toBe(80);
    expect(batch[0].text).toBe('msg0');
    expect(batch[79].text).toBe('msg79');
    // 剩下的 seenNodes 已标记，下轮扫描跳过
  });
});

describe('巡检机制优化', () => {
  let adapter;
  beforeEach(() => {
    adapter = new BaseAdapter({
      name: 'test',
      channel: 'xiaohongshu',
      hooks: {
        getConversationList: () => [
          { id: 'c1', name: '客户1', unread: true },
          { id: 'c2', name: '客户2', unread: true },
          { id: 'c3', name: '客户3', unread: false },
          { id: 'c4', name: '客户4', unread: true },
          { id: 'c5', name: '客户5', unread: false },
        ],
        getConversationId: () => 'c0', // 用户当前在别的会话
        getMessageItems: () => [],
        parseMessageItem: () => null,
        openConversation: vi.fn(async (id) => ({ id })),
        _findListScroller: () => null,
      },
    });
    adapter._patrolOpts = { intervalMs: 60000, throttleMs: 100, waitActiveMs: 100, maxPerRound: 0, scrollLoadMs: 100, maxPasses: 1, visitAllWhenNoUnread: false };
  });

  it('scanOnly 模式：仅扫描，不点开任何会话', async () => {
    const result = await adapter.patrol({ ...adapter._patrolOpts, scanOnly: true });
    expect(result.scannedTotal).toBe(5);
    expect(result.visited).toBe(0);
    expect(adapter.hooks.openConversation).not.toHaveBeenCalled();
  });

  it('默认模式：遍历所有会话（2026-08-05 修复：不再过滤未读，直接遍历）', async () => {
    // 拦截 _patrolVisit：模拟无新消息
    adapter._patrolVisit = vi.fn(async (conv) => ({ ok: true, newCount: 0 }));
    const result = await adapter.patrol(adapter._patrolOpts);
    expect(result.scannedTotal).toBe(5);
    // 巡检不再过滤未读，直接遍历所有会话 = 5 个
    expect(adapter._patrolVisit).toHaveBeenCalledTimes(5);
  });

  it('用户已停留的会话：自动跳过（不重复点开）', async () => {
    // 用户已停留在 c2
    adapter.hooks.getConversationId = () => 'c2';
    adapter._patrolVisit = vi.fn(async (conv) => ({ ok: true, newCount: 0 }));
    await adapter.patrol(adapter._patrolOpts);
    // 巡检遍历所有会话，c2 被跳过（用户已停留）
    const calledIds = adapter._patrolVisit.mock.calls.map((c) => c[0].id);
    expect(calledIds).toContain('c1');
    expect(calledIds).toContain('c4');
    expect(calledIds).not.toContain('c2');
  });

  it('无 unread 时也遍历所有会话（2026-08-05 修复：不再跳过本轮）', async () => {
    adapter.hooks.getConversationList = () => [
      { id: 'c1', unread: false },
      { id: 'c2', unread: false },
    ];
    adapter._patrolVisit = vi.fn(async () => ({ ok: true, newCount: 0 }));
    const result = await adapter.patrol(adapter._patrolOpts);
    // 巡检不再判断未读，直接遍历所有会话 = 2 个
    expect(result.visited).toBe(2);
    expect(adapter._patrolVisit).toHaveBeenCalledTimes(2);
  });
});

describe('抖音自他识别（修复 2026-08-05）', () => {
  it('新版显式 class 应被识别', async () => {
    const { SEL } = await import('../src/channels/douyin.js');
    expect(SEL.SELF_ITEM).toContain('chatMessageItemSelf');
    expect(SEL.OTHER_ITEM).toContain('chatMessageItemOther');
    expect(SEL.MSG_ITEM).toContain('chatMessageItemSelf');
    expect(SEL.MSG_ITEM).toContain('chatMessageItemOther');
  });
});
