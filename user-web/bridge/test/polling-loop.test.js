// PollingLoop 单测（2026-08-06 三通道重构）
//
// 覆盖：
//   1. 生命周期：start/stop 设置/清除 patrol + downlink 定时器，幂等
//   2. _patrol：空配置 / 无 serverUrl / 无账号 / 无适配器 / 空会话 静默
//   3. _patrol：抓消息后通过 Uplink → POST /api/bridge/ingest（body 字段对齐）
//   4. 纯桥接：每轮都上报（去重交给后端），Uplink 短窗口合并需 flushAll 才真正发出
//   5. _patrol 游标：遍历会话列表、逐会话切换随机 1-2s（由 BRIDGE_THREE_CHANNEL 控制）
//
// 设计：使用 fake timer + mock fetch，绕开真实网络。Uplink 合并窗口用 flushAll 强制刷新。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { PollingLoop } from '../src/core/polling-loop.js';
import { BRIDGE_THREE_CHANNEL } from '../src/core/constants.js';

// 桩适配器
function makeAdapter({ convs, messages }) {
  return {
    getConversationList: vi.fn(async () => convs),
    openConversation: vi.fn(async () => {}),
    getMessages: vi.fn(async () => messages),
    sendText: vi.fn(async () => true),
  };
}

const getMeta = () => ({ accountId: 'acc-1' });
const getConfig = async () => ({ serverUrl: 'http://localhost:8204', token: 'tkn-1' });

describe('polling-loop / 生命周期', () => {
  it('start() 设置 patrol + downlink 定时器，stop() 清除', () => {
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => null, getConfig, getMeta });
    expect(loop._patrolTimer).toBeNull();
    expect(loop._downlinkTimer).toBeNull();
    loop.start();
    expect(loop._patrolTimer).not.toBeNull();
    expect(loop._downlinkTimer).not.toBeNull();
    loop.stop();
    expect(loop._patrolTimer).toBeNull();
    expect(loop._downlinkTimer).toBeNull();
  });

  it('start() 幂等：重复调用不创建多个 timer', () => {
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => null, getConfig, getMeta });
    loop.start();
    const t1 = loop._patrolTimer;
    const t2 = loop._downlinkTimer;
    loop.start();
    expect(loop._patrolTimer).toBe(t1);
    expect(loop._downlinkTimer).toBe(t2);
    loop.stop();
  });

  it('stop() 后 _running 为 false', () => {
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => null, getConfig, getMeta });
    loop.start();
    loop.stop();
    expect(loop._running).toBe(false);
  });
});

describe('polling-loop / _patrol 异常路径', () => {
  it('无 config 时静默', async () => {
    const adapter = makeAdapter({ convs: [], messages: [] });
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => adapter, getConfig: () => null, getMeta });
    await loop._patrol();
    expect(adapter.getConversationList).not.toHaveBeenCalled();
  });

  it('无 serverUrl 时静默', async () => {
    const adapter = makeAdapter({ convs: [], messages: [] });
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => adapter, getConfig: async () => ({}), getMeta });
    await loop._patrol();
    expect(adapter.getConversationList).not.toHaveBeenCalled();
  });

  it('无显式账号时回退 default 并继续巡检', async () => {
    const adapter = makeAdapter({ convs: [{ id: 'c1' }], messages: [{ message_id: 'm1', text: 'hi', sender_id: 'p', sender_name: '小红', sender_type: 'customer', msg_type: 'text', timestamp: 1 }] });
    let captured;
    const realFetch = globalThis.fetch;
    globalThis.fetch = vi.fn(async (url) => { captured = url; return new Response('{"ok":true,"ingested":[],"outbound_replies":[]}', { status: 200 }); });
    try {
      const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => adapter, getConfig: async () => ({ serverUrl: 'http://localhost:8204' }), getMeta: () => ({}) });
      await loop._patrol();
      await loop.uplinks.get('douyin').flushAll();
    } finally {
      globalThis.fetch = realFetch;
    }
    expect(adapter.getConversationList).toHaveBeenCalled();
    expect(captured).toContain('account_id=default');
  });

  it('无适配器时静默', async () => {
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => null, getConfig, getMeta });
    await loop._patrol(); // 不抛
  });

  it('会话列表为空时不上报', async () => {
    const adapter = makeAdapter({ convs: [], messages: [] });
    const loop = new PollingLoop({ channels: ['xiaohongshu'], getAdapter: () => adapter, getConfig, getMeta });
    let fetchCalled = false;
    const realFetch = globalThis.fetch;
    globalThis.fetch = vi.fn(() => { fetchCalled = true; return Promise.resolve(new Response('{}', { status: 200 })); });
    try {
      await loop._patrol();
      await loop.uplinks.get('xiaohongshu').flushAll();
    } finally {
      globalThis.fetch = realFetch;
    }
    expect(fetchCalled).toBe(false);
  });
});

describe('polling-loop / _patrol 抓消息 → 上报（通道A·上报）', () => {
  let realFetch;
  beforeEach(() => { realFetch = globalThis.fetch; });
  afterEach(() => { globalThis.fetch = realFetch; vi.restoreAllMocks(); });

  it('抓到的消息通过 Uplink → POST /api/bridge/ingest，body 字段对齐', async () => {
    let captured;
    globalThis.fetch = vi.fn(async (url, init) => {
      captured = { url, init };
      return new Response(JSON.stringify({ ok: true, ingested: [], outbound_replies: [] }), { status: 200 });
    });
    const adapter = makeAdapter({
      convs: [{ id: 'conv-1' }],
      messages: [{ message_id: 'msg-1', sender_id: 'peer-1', sender_name: '小红', sender_type: 'customer', text: '你好', msg_type: 'text', timestamp: 1700000000 }],
    });
    const loop = new PollingLoop({ channels: ['xiaohongshu'], getAdapter: () => adapter, getConfig, getMeta });
    await loop._patrol();
    await loop.uplinks.get('xiaohongshu').flushAll();
    expect(captured).toBeTruthy();
    expect(captured.url).toContain('channel=xiaohongshu');
    expect(captured.url).toContain('account_id=acc-1');
    expect(captured.url).toContain('conversation_id=conv-1');
    expect(captured.url).toContain('token=tkn-1');
    const body = JSON.parse(captured.init.body);
    expect(body.v).toBe(2);
    expect(body.messages).toHaveLength(1);
    expect(body.messages[0].event_id).toBe('msg-1');
    expect(body.messages[0].sender_name).toBe('小红');
    expect(body.messages[0].content).toBe('你好');
    expect(body.expect_reply).toBeUndefined(); // 纯桥接：不设 expect_reply
  });

  it('无消息时不调用 fetch', async () => {
    const adapter = makeAdapter({ convs: [{ id: 'c' }], messages: [] });
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => adapter, getConfig, getMeta });
    let fetchCalled = false;
    globalThis.fetch = vi.fn(() => { fetchCalled = true; return Promise.resolve(new Response('{}', { status: 200 })); });
    await loop._patrol();
    await loop.uplinks.get('douyin').flushAll();
    expect(fetchCalled).toBe(false);
  });

  it('纯桥接：同一 message_id 每轮都上报（内容去重交给后端）', async () => {
    const adapter = makeAdapter({ convs: [{ id: 'conv-A' }], messages: [{ message_id: 'm1', text: 'hi', sender_id: 'p', msg_type: 'text', timestamp: 1 }] });
    let fetchCount = 0;
    globalThis.fetch = vi.fn(async () => { fetchCount += 1; return new Response('{"ok":true,"ingested":[],"outbound_replies":[]}', { status: 200 }); });
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => adapter, getConfig, getMeta });
    await loop._patrol(); await loop.uplinks.get('douyin').flushAll();
    await loop._patrol(); await loop.uplinks.get('douyin').flushAll();
    expect(fetchCount).toBe(2);
  });

  it('ingest 失败时不崩溃，下一轮仍可上报', async () => {
    const adapter = makeAdapter({ convs: [{ id: 'conv-D' }], messages: [{ message_id: 'm1', text: '1', sender_id: 'p', msg_type: 'text', timestamp: 1 }] });
    let callCount = 0;
    globalThis.fetch = vi.fn(async () => { callCount += 1; if (callCount === 1) return new Response('boom', { status: 500 }); return new Response('{"ok":true,"ingested":[],"outbound_replies":[]}', { status: 200 }); });
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => adapter, getConfig, getMeta, retryOpts: { maxRetries: 0, retryBaseMs: 1 } });
    await loop._patrol(); await loop.uplinks.get('douyin').flushAll();
    await loop._patrol(); await loop.uplinks.get('douyin').flushAll();
    expect(callCount).toBe(2);
  });
});

describe('polling-loop / 巡检配置（要求⑤：3s 一轮，切换随机 1-2s）', () => {
  it('patrolIntervalMs=3000', () => { expect(BRIDGE_THREE_CHANNEL.patrolIntervalMs).toBe(3000); });
  it('switch 区间 [1000,2000]', () => {
    expect(BRIDGE_THREE_CHANNEL.patrolSwitchMinMs).toBe(1000);
    expect(BRIDGE_THREE_CHANNEL.patrolSwitchMaxMs).toBe(2000);
    expect(BRIDGE_THREE_CHANNEL.patrolSwitchMaxMs).toBeGreaterThanOrEqual(BRIDGE_THREE_CHANNEL.patrolSwitchMinMs);
  });
  it('outboxPollIntervalMs=1500（下发轮询独立）', () => { expect(BRIDGE_THREE_CHANNEL.outboxPollIntervalMs).toBe(1500); });
});

describe('polling-loop / _patrolSafe 防护', () => {
  it('stop() 后 _patrolSafe 立即返回', async () => {
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => null, getConfig, getMeta });
    loop.start();
    loop.stop();
    await loop._patrolSafe(); // 不抛
    expect(loop._patrolInFlight).toBe(false);
  });
});
