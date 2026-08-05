// PollingLoop 单测 (2026-08-05)
//
// 覆盖：
//   1. 启动 / 停止：setInterval 启停，幂等
//   2. _tick：空配置 / 无 active / 无适配器 静默
//   3. _tick：抓消息后调用 postIngest（参数严格对齐）
//   4. 去重：同一 message_id 同一轮只抓一次
//   5. 节流：MIN_PER_CONVERSATION_INTERVAL_MS 之内不重抓
//   6. 失败回滚：ingest 失败时把 message_id 从 seen 中移除，下轮重试
//   7. dispatchOutbound：拿到 outbound_replies 后调用
//
// 设计：使用 fake timer + mock postIngest，绕开真实 fetch。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { PollingLoop, POLL_INTERVAL_MS } from '../src/core/polling-loop.js';

// 桩适配器
function makeAdapter({ convs, messages }) {
  return {
    getConversationList: vi.fn(async () => convs),
    openConversation: vi.fn(async () => {}),
    getMessages: vi.fn(async () => messages),
    sendText: vi.fn(async () => true),
  };
}

// 等待 micro-task
function nextTicks(n = 5) {
  return new Promise((r) => setTimeout(r, 0));
}

describe('polling-loop / 生命周期', () => {
  it('start() 设置定时器，stop() 清除', async () => {
    const loop = new PollingLoop({ config: null, getAdapter: () => null });
    expect(loop._timer).toBeNull();
    loop.start();
    expect(loop._timer).not.toBeNull();
    loop.stop();
    expect(loop._timer).toBeNull();
  });

  it('start() 幂等：重复调用不会创建多个 timer', () => {
    const loop = new PollingLoop({ config: null, getAdapter: () => null });
    loop.start();
    const t1 = loop._timer;
    loop.start();
    expect(loop._timer).toBe(t1);
    loop.stop();
  });

  it('stop() 后 _inFlight 状态保留不抛错', () => {
    const loop = new PollingLoop({ config: null, getAdapter: () => null });
    loop.start();
    loop.stop();
    expect(loop._running).toBe(false);
  });
});

describe('polling-loop / _tick 异常路径', () => {
  it('无 config 时静默', async () => {
    const adapter = makeAdapter({ convs: [], messages: [] });
    const loop = new PollingLoop({
      config: null,
      getAdapter: () => adapter,
      dispatchOutbound: () => true,
      getConfig: () => null,
    });
    await loop._tick();
    expect(adapter.getConversationList).not.toHaveBeenCalled();
  });

  it('无 active 账号时静默', async () => {
    const adapter = makeAdapter({ convs: [], messages: [] });
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: null },
      getAdapter: () => adapter,
      dispatchOutbound: () => true,
    });
    await loop._tick();
    expect(adapter.getConversationList).not.toHaveBeenCalled();
  });

  it('active 缺 channel/accountId 静默', async () => {
    const adapter = makeAdapter({ convs: [], messages: [] });
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: '', accountId: '' } },
      getAdapter: () => adapter,
      dispatchOutbound: () => true,
    });
    await loop._tick();
    expect(adapter.getConversationList).not.toHaveBeenCalled();
  });

  it('无适配器时静默', async () => {
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: 'douyin', accountId: 'a1' } },
      getAdapter: () => null,
      dispatchOutbound: () => true,
    });
    await loop._tick(); // 不抛
  });

  it('会话列表为空时不上报', async () => {
    const adapter = makeAdapter({ convs: [], messages: [] });
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: 'douyin', accountId: 'a1' } },
      getAdapter: () => adapter,
      dispatchOutbound: () => true,
    });
    // postIngest 未被调用 → 通过不发请求验证
    let fetchCalled = false;
    const realFetch = globalThis.fetch;
    globalThis.fetch = vi.fn(() => {
      fetchCalled = true;
      return Promise.resolve(new Response('{}', { status: 200 }));
    });
    try {
      await loop._tick();
    } finally {
      globalThis.fetch = realFetch;
    }
    expect(fetchCalled).toBe(false);
  });
});

describe('polling-loop / _tick 抓消息 → 上报', () => {
  let realFetch;
  beforeEach(() => {
    realFetch = globalThis.fetch;
  });
  afterEach(() => {
    globalThis.fetch = realFetch;
    vi.restoreAllMocks();
  });

  it('抓到的消息通过 postIngest 上报，body 字段对齐', async () => {
    let captured;
    globalThis.fetch = vi.fn(async (url, init) => {
      captured = { url, init };
      return new Response(
        JSON.stringify({ ok: true, ingested: [], outbound_replies: [] }),
        { status: 200 }
      );
    });
    const adapter = makeAdapter({
      convs: [{ id: 'conv-1' }],
      messages: [
        {
          message_id: 'msg-1',
          sender_id: 'peer-1',
          sender_name: '小红',
          sender_type: 'customer',
          text: '你好',
          msg_type: 'text',
          timestamp: 1700000000,
        },
      ],
    });
    const loop = new PollingLoop({
      config: {
        serverUrl: 'http://localhost:8204',
        token: 'tkn-1',
        active: { channel: 'xiaohongshu', accountId: 'acc-1', accountName: '小蜜蜂', agentId: 7 },
      },
      getAdapter: () => adapter,
      dispatchOutbound: () => true,
    });
    await loop._tick();
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
    expect(body.expect_reply).toBe(true);
    expect(body.timeout_ms).toBeGreaterThanOrEqual(500000);
  });

  it('无消息时不调用 postIngest', async () => {
    const adapter = makeAdapter({ convs: [{ id: 'c' }], messages: [] });
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: 'douyin', accountId: 'a' } },
      getAdapter: () => adapter,
      dispatchOutbound: () => true,
    });
    let fetchCalled = false;
    globalThis.fetch = vi.fn(() => {
      fetchCalled = true;
      return Promise.resolve(new Response('{}', { status: 200 }));
    });
    await loop._tick();
    expect(fetchCalled).toBe(false);
  });
});

describe('polling-loop / 同一会话去重 + 节流', () => {
  let realFetch;
  beforeEach(() => {
    realFetch = globalThis.fetch;
  });
  afterEach(() => {
    globalThis.fetch = realFetch;
    vi.restoreAllMocks();
  });

  it('同一 message_id 在同一会话内只抓一次', async () => {
    const adapter = makeAdapter({
      convs: [{ id: 'conv-A' }],
      messages: [{ message_id: 'm1', text: 'hi', sender_id: 'p', msg_type: 'text', timestamp: 1 }],
    });
    let fetchCount = 0;
    globalThis.fetch = vi.fn(async () => {
      fetchCount += 1;
      return new Response('{"ok":true,"ingested":[],"outbound_replies":[]}', { status: 200 });
    });
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: 'douyin', accountId: 'a' } },
      getAdapter: () => adapter,
      dispatchOutbound: () => true,
    });
    await loop._tick();
    await loop._tick(); // 第二轮同一 message_id
    expect(fetchCount).toBe(1);
  });

  it('MIN_PER_CONVERSATION_INTERVAL_MS 之内不重抓', async () => {
    const adapter = makeAdapter({
      convs: [{ id: 'conv-B' }],
      messages: [
        { message_id: 'm1', text: '1', sender_id: 'p', msg_type: 'text', timestamp: 1 },
        { message_id: 'm2', text: '2', sender_id: 'p', msg_type: 'text', timestamp: 2 },
      ],
    });
    const fetchMock = vi.fn(async () =>
      new Response('{"ok":true,"ingested":[],"outbound_replies":[]}', { status: 200 })
    );
    globalThis.fetch = fetchMock;
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: 'douyin', accountId: 'a' } },
      getAdapter: () => adapter,
      dispatchOutbound: () => true,
    });
    await loop._tick(); // 抓 m1, m2
    // 立刻再 tick：节流应阻止（间隔 < MIN_PER_CONVERSATION_INTERVAL_MS = 800ms）
    await loop._tick();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('新消息（不同 message_id）下一轮会被抓', async () => {
    let n = 0;
    const adapter = makeAdapter({
      convs: [{ id: 'conv-C' }],
      messages: [
        { message_id: 'm1', text: '1', sender_id: 'p', msg_type: 'text', timestamp: 1 },
      ],
    });
    const fetchMock = vi.fn(async () => {
      return new Response('{"ok":true,"ingested":[],"outbound_replies":[]}', { status: 200 });
    });
    globalThis.fetch = fetchMock;
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: 'douyin', accountId: 'a' } },
      getAdapter: () => adapter,
      dispatchOutbound: () => true,
    });
    await loop._tick(); // m1
    // 调整 _lastPollByConv 让下一轮可以抓
    loop._lastPollByConv.set('douyin:a:conv-C', 0);
    adapter.getMessages.mockResolvedValueOnce([
      { message_id: 'm2', text: '2', sender_id: 'p', msg_type: 'text', timestamp: 2 },
    ]);
    await loop._tick(); // m2
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe('polling-loop / 失败回滚 + 重试', () => {
  let realFetch;
  beforeEach(() => {
    realFetch = globalThis.fetch;
  });
  afterEach(() => {
    globalThis.fetch = realFetch;
    vi.restoreAllMocks();
  });

  it('ingest 失败时把 message_id 从 seen 中移除', async () => {
    const adapter = makeAdapter({
      convs: [{ id: 'conv-D' }],
      messages: [
        { message_id: 'm1', text: '1', sender_id: 'p', msg_type: 'text', timestamp: 1 },
      ],
    });
    globalThis.fetch = vi.fn(async () => new Response('boom', { status: 500 }));
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: 'douyin', accountId: 'a' } },
      getAdapter: () => adapter,
      dispatchOutbound: () => true,
      // 把 postIngest 内部 retry 关掉，立即抛错 → 走 seen 回滚路径
      retryOpts: { maxRetries: 0, retryBaseMs: 1 },
    });
    // 直接调用 _tick 即可；postIngest 失败 → seen.delete
    await loop._tick();
    const seen = loop._seenEventIds.get('douyin:a:conv-D');
    expect(seen ? seen.has('m1') : true).toBe(false);
  });
});

describe('polling-loop / outbound_replies dispatch', () => {
  let realFetch;
  beforeEach(() => {
    realFetch = globalThis.fetch;
  });
  afterEach(() => {
    globalThis.fetch = realFetch;
    vi.restoreAllMocks();
  });

  it('收到 outbound_replies 时 dispatchOutbound 被调用', async () => {
    const adapter = makeAdapter({
      convs: [{ id: 'conv-E' }],
      messages: [
        { message_id: 'm1', text: '1', sender_id: 'p', msg_type: 'text', timestamp: 1 },
      ],
    });
    globalThis.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify({
          ok: true,
          ingested: [{ event_id: 'm1', accepted: true, ai_handled: true }],
          outbound_replies: [
            {
              channel: 'douyin',
              account_id: 'a',
              conversation_id: 'conv-E',
              content: 'AI 回复',
              msg_type: 'text',
              reply_to_event_id: 'm1',
            },
          ],
        }),
        { status: 200 }
      )
    );
    const dispatchOutbound = vi.fn(() => true);
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: 'douyin', accountId: 'a' } },
      getAdapter: () => adapter,
      dispatchOutbound,
    });
    await loop._tick();
    expect(dispatchOutbound).toHaveBeenCalledTimes(1);
    const arg = dispatchOutbound.mock.calls[0][0];
    expect(arg.channel).toBe('douyin');
    expect(arg.conversation_id).toBe('conv-E');
    expect(arg.content).toBe('AI 回复');
    expect(arg.reply_to_event_id).toBe('m1');
  });

  it('dispatchOutbound 返回 false 也不抛错', async () => {
    const adapter = makeAdapter({
      convs: [{ id: 'conv-F' }],
      messages: [
        { message_id: 'm1', text: '1', sender_id: 'p', msg_type: 'text', timestamp: 1 },
      ],
    });
    globalThis.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify({
          ok: true,
          ingested: [],
          outbound_replies: [
            {
              channel: 'douyin',
              conversation_id: 'conv-F',
              content: 'reply',
              msg_type: 'text',
              reply_to_event_id: 'm1',
            },
          ],
        }),
        { status: 200 }
      )
    );
    const dispatchOutbound = vi.fn(() => false);
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: 'douyin', accountId: 'a' } },
      getAdapter: () => adapter,
      dispatchOutbound,
    });
    await expect(loop._tick()).resolves.toBeUndefined();
    expect(dispatchOutbound).toHaveBeenCalled();
  });

  it('空 outbound_replies 时 dispatchOutbound 不被调用', async () => {
    const adapter = makeAdapter({
      convs: [{ id: 'conv-G' }],
      messages: [
        { message_id: 'm1', text: '1', sender_id: 'p', msg_type: 'text', timestamp: 1 },
      ],
    });
    globalThis.fetch = vi.fn(async () =>
      new Response('{"ok":true,"ingested":[],"outbound_replies":[]}', { status: 200 })
    );
    const dispatchOutbound = vi.fn(() => true);
    const loop = new PollingLoop({
      config: { serverUrl: 'http://x', token: 't', active: { channel: 'douyin', accountId: 'a' } },
      getAdapter: () => adapter,
      dispatchOutbound,
    });
    await loop._tick();
    expect(dispatchOutbound).not.toHaveBeenCalled();
  });
});

describe('polling-loop / POLL_INTERVAL_MS', () => {
  it('常量等于 1000ms（用户诉求 1 秒钟一个）', () => {
    expect(POLL_INTERVAL_MS).toBe(1000);
  });
});

describe('polling-loop / start 后的 _tickSafe 防护', () => {
  it('stop() 后 _tickSafe 立即返回', async () => {
    const loop = new PollingLoop({ config: null, getAdapter: () => null });
    loop.start();
    loop.stop();
    await loop._tickSafe(); // 不抛
    expect(loop._inFlight).toBe(false);
  });
});
