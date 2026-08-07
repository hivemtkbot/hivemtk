// Downlink 单测（通道C·下发轮询 + 通道B·状态上报 + 本地已发缓存）
//
// 覆盖：
//   1. SentCache：add/has 去重；持久化到 chrome.storage.local
//   2. pollDownlink：拉取 outbox → 转发对应渠道 → ack delivered
//   3. 本地已发缓存：同 msg_id 绝不重复下发（严重消息不重复发给用户）
//   4. 发送失败：不缓存、不 ack，下轮重试
//   5. 空内容不下发（避免占位空消息）
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// 内存版 chrome.storage.local
function makeChromeStorage() {
  const data = {};
  return {
    storage: {
      local: {
        get: vi.fn((keys) => {
          const out = {};
          const arr = Array.isArray(keys) ? keys : [keys];
          for (const k of arr) if (k in data) out[k] = data[k];
          return Promise.resolve(out);
        }),
        set: vi.fn((obj) => { Object.assign(data, obj); return Promise.resolve(); }),
      },
    },
    _data: data,
  };
}

vi.mock('../src/core/http-ingest.js', () => ({
  getOutbox: vi.fn(),
  ackOutbox: vi.fn(),
}));
vi.mock('../src/core/sanitize.js', () => ({
  sanitizeForDisplay: (t) => t,
}));

import { pollDownlink, initDownlink } from '../src/core/downlink.js';
import { getOutbox, ackOutbox } from '../src/core/http-ingest.js';

// 每个用例用独立 channel，避免模块级 SentCache 单例跨用例污染
function cfg() { return async () => ({ serverUrl: 'http://localhost:8204', token: 't' }); }

describe('downlink / SentCache 去重 + 持久化', () => {
  let chrome;
  beforeEach(() => { chrome = makeChromeStorage(); globalThis.chrome = chrome; vi.clearAllMocks(); });
  afterEach(() => { delete globalThis.chrome; });

  it('add 后 has 为 true；持久化到 storage', async () => {
    await initDownlink(['douyin']);
    const adapter = { sendOutbound: vi.fn(async () => ({ ok: true })) };
    getOutbox.mockResolvedValue({ status: 'ok', messages: [{ msg_id: 'm-d1', content: 'hi', conversation_id: 'c1' }] });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('douyin', 'acc1', cfg(), { sendOutbound: adapter.sendOutbound });
    const stored = await chrome.storage.local.get(['bridge_sent_douyin']);
    expect(stored['bridge_sent_douyin']).toContain('m-d1|c1');
    expect(adapter.sendOutbound).toHaveBeenCalledTimes(1);
    expect(ackOutbox).toHaveBeenCalledWith(expect.any(Object), ['m-d1'], expect.any(Object));
  });
});

describe('downlink / 本地已发缓存拦截重复下发', () => {
  let chrome;
  beforeEach(() => { chrome = makeChromeStorage(); globalThis.chrome = chrome; vi.clearAllMocks(); });
  afterEach(() => { delete globalThis.chrome; });

  it('同 msg_id 不重复下发（本地缓存拦截）+ 不重复 ack', async () => {
    await initDownlink(['tiktok']);
    const adapter = { sendOutbound: vi.fn(async () => ({ ok: true })) };
    getOutbox.mockResolvedValue({ status: 'ok', messages: [{ msg_id: 'm-t1', content: 'hi', conversation_id: 'c1' }] });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    const c = cfg();
    await pollDownlink('tiktok', 'acc1', c, { sendOutbound: adapter.sendOutbound }); // 第一次：发送 + ack
    await pollDownlink('tiktok', 'acc1', c, { sendOutbound: adapter.sendOutbound }); // 第二次：缓存命中，跳过
    expect(adapter.sendOutbound).toHaveBeenCalledTimes(1);
    expect(ackOutbox).toHaveBeenCalledTimes(1);
  });
});

describe('downlink / 发送失败重试', () => {
  let chrome;
  beforeEach(() => { chrome = makeChromeStorage(); globalThis.chrome = chrome; vi.clearAllMocks(); });
  afterEach(() => { delete globalThis.chrome; });

  it('sendOutbound 失败：不缓存、不 ack，下轮重试成功', async () => {
    await initDownlink(['xianyu']);
    const adapter = { sendOutbound: vi.fn(async () => ({ ok: false })) };
    getOutbox.mockResolvedValue({ status: 'ok', messages: [{ msg_id: 'm-x1', content: 'hi', conversation_id: 'c1' }] });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    const c = cfg();
    await pollDownlink('xianyu', 'acc1', c, { sendOutbound: adapter.sendOutbound }); // 失败
    expect(ackOutbox).not.toHaveBeenCalled();
    adapter.sendOutbound.mockResolvedValue({ ok: true }); // 下一轮：success
    await pollDownlink('xianyu', 'acc1', c, { sendOutbound: adapter.sendOutbound });
    expect(adapter.sendOutbound).toHaveBeenCalledTimes(2);
    expect(ackOutbox).toHaveBeenCalledTimes(1);
  });
});

describe('downlink / ack 失败不丢消息（先 ack 后缓存）', () => {
  let chrome;
  beforeEach(() => { chrome = makeChromeStorage(); globalThis.chrome = chrome; vi.clearAllMocks(); });
  afterEach(() => { delete globalThis.chrome; });

  it('转发成功但 ack 失败：不写本地缓存，下轮重新拉取并重试', async () => {
    await initDownlink(['douyin']);
    const adapter = { sendOutbound: vi.fn(async () => ({ ok: true })) };
    getOutbox.mockResolvedValue({ status: 'ok', messages: [{ msg_id: 'm-ackfail', content: 'hi', conversation_id: 'c1' }] });
    // 第一轮：ack 失败
    ackOutbox.mockResolvedValue({ status: 'error' });
    const c = cfg();
    await pollDownlink('douyin', 'acc1', c, { sendOutbound: adapter.sendOutbound });
    // 本地缓存绝不能写入（否则下轮被拦截，消息永久丢失）
    let stored = await chrome.storage.local.get(['bridge_sent_douyin']);
    expect(stored['bridge_sent_douyin'] || []).not.toContain('m-ackfail|c1');
    expect(adapter.sendOutbound).toHaveBeenCalledTimes(1);

    // 第二轮：ack 成功，才写入缓存
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('douyin', 'acc1', c, { sendOutbound: adapter.sendOutbound });
    stored = await chrome.storage.local.get(['bridge_sent_douyin']);
    expect(stored['bridge_sent_douyin']).toContain('m-ackfail|c1');
    expect(adapter.sendOutbound).toHaveBeenCalledTimes(2);
    expect(ackOutbox).toHaveBeenCalledTimes(2);
  });
});

describe('downlink / 空内容不下发', () => {
  let chrome;
  beforeEach(() => { chrome = makeChromeStorage(); globalThis.chrome = chrome; vi.clearAllMocks(); });
  afterEach(() => { delete globalThis.chrome; });

  it('空内容不下发、不 ack', async () => {
    await initDownlink(['xiaohongshu']);
    const adapter = { sendOutbound: vi.fn(async () => ({ ok: true })) };
    getOutbox.mockResolvedValue({ status: 'ok', messages: [{ msg_id: 'm-xh1', content: '', conversation_id: 'c1' }] });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('xiaohongshu', 'acc1', cfg(), { sendOutbound: adapter.sendOutbound });
    expect(adapter.sendOutbound).not.toHaveBeenCalled();
    expect(ackOutbox).not.toHaveBeenCalled();
  });
});

// 2026-08-07 P0 修复回归测试：
//   AI 回复 msg_id = contentHash(channel, content)，不含 conversation_id（patrol 回环去重需要）。
//   跨会话同 content 的 AI 回复 msg_id 相同。若 SentCache 只用 msg_id 做 key，
//   第一会话 ack 后 cache.add(msg_id) → 第二会话 cache.has(msg_id) 命中 → 跳过，永远不下发！
//   修复：SentCache key 改为 msg_id|conversation_id 复合键，不同会话各自独立去重。
describe('downlink / 跨会话同 msg_id 不误去重（P0 回归）', () => {
  let chrome;
  beforeEach(() => { chrome = makeChromeStorage(); globalThis.chrome = chrome; vi.clearAllMocks(); });
  afterEach(() => { delete globalThis.chrome; });

  it('同 msg_id 不同 conversation_id：两条都下发、都 ack', async () => {
    await initDownlink(['xiaohongshu']);
    const adapter = { sendOutbound: vi.fn(async () => ({ ok: true, rateLimited: false, notFound: false })) };
    // 两条 msg_id 相同（模拟 contentHash 不含 conv 的跨会话同 content 场景），但 conversation_id 不同
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [
        { msg_id: 'mh:samehash', content: '你好', conversation_id: 'convA' },
        { msg_id: 'mh:samehash', content: '你好', conversation_id: 'convB' },
      ],
    });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('xiaohongshu', 'acc1', cfg(), { sendOutbound: adapter.sendOutbound });

    // 关键回归：两条都被下发（不因 msg_id 相同而误去重第二条）
    expect(adapter.sendOutbound).toHaveBeenCalledTimes(2);
    expect(adapter.sendOutbound).toHaveBeenCalledWith('你好', 'convA', expect.any(Object));
    expect(adapter.sendOutbound).toHaveBeenCalledWith('你好', 'convB', expect.any(Object));

    // 两条都被 ack（每会话独立 ack）
    expect(ackOutbox).toHaveBeenCalledTimes(2);

    // 缓存写入两个复合键，不含裸 msg_id
    const stored = await chrome.storage.local.get(['bridge_sent_xiaohongshu']);
    const arr = stored['bridge_sent_xiaohongshu'] || [];
    expect(arr).toContain('mh:samehash|convA');
    expect(arr).toContain('mh:samehash|convB');
    expect(arr).not.toContain('mh:samehash');
  });

  it('跨轮次：第一轮 convA 已 ack，第二轮 convB 仍能下发（不被 convA 缓存误拦截）', async () => {
    await initDownlink(['kuaishou']);
    const adapter = { sendOutbound: vi.fn(async () => ({ ok: true, rateLimited: false, notFound: false })) };
    const c = cfg();
    // 第一轮：只返回 convA 的消息
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [{ msg_id: 'mh:samehash', content: '你好', conversation_id: 'convA' }],
    });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('kuaishou', 'acc1', c, { sendOutbound: adapter.sendOutbound });
    expect(adapter.sendOutbound).toHaveBeenCalledTimes(1);

    // 第二轮：返回 convB 的消息（msg_id 与 convA 相同）
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [{ msg_id: 'mh:samehash', content: '你好', conversation_id: 'convB' }],
    });
    await pollDownlink('kuaishou', 'acc1', c, { sendOutbound: adapter.sendOutbound });

    // 关键回归：convB 仍被下发（不被 convA 的缓存命中跳过）
    expect(adapter.sendOutbound).toHaveBeenCalledTimes(2);
    expect(adapter.sendOutbound).toHaveBeenLastCalledWith('你好', 'convB', expect.any(Object));
    expect(ackOutbox).toHaveBeenCalledTimes(2);
  });
});

// 2026-08-07 修复（用户诉求 ①②③）：
//   ① 串行 → 多会话场景下，旧版第 1 条成功发完后 markSent 更新 lastGlobalSendAt，
//       第 2 条因 minIntervalMs(1500ms) 拦截 → 整批 break → 第二个会话永远不发。
//       新版按 conversation_id 分组并行：每个会话独立 tryAcquire/markSent，会话 A 限速
//       不影响会话 B。两个会话的两条消息都被下发 + 都 ack。
describe('downlink / 多会话不串行（2026-08-07 核心修复）', () => {
  let chrome;
  beforeEach(() => { chrome = makeChromeStorage(); globalThis.chrome = chrome; vi.clearAllMocks(); });
  afterEach(() => { delete globalThis.chrome; });

  it('两个会话的 pending outbound 全部下发 + 全部 ack（不被全局 minInterval 互相阻断）', async () => {
    await initDownlink(['xiaohongshu']);
    const adapter = {
      sendOutbound: vi.fn(async (text, convId) => {
        // 模拟「刚发完一个会话，下个调用立刻发」：无 rate-limit 干扰
        return { ok: true, rateLimited: false, notFound: false };
      })
    };
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [
        { msg_id: 'm-convA', content: '回复 A 的消息', conversation_id: 'convA' },
        { msg_id: 'm-convB', content: '回复 B 的消息', conversation_id: 'convB' },
      ],
    });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('xiaohongshu', 'acc1', cfg(), { sendOutbound: adapter.sendOutbound });

    // 关键回归：两个会话都收到了 sendOutbound 调用（不被串行阻断）
    expect(adapter.sendOutbound).toHaveBeenCalledTimes(2);
    expect(adapter.sendOutbound).toHaveBeenCalledWith('回复 A 的消息', 'convA', expect.any(Object));
    expect(adapter.sendOutbound).toHaveBeenCalledWith('回复 B 的消息', 'convB', expect.any(Object));

    // 关键回归：两条消息都被 ack（每会话独立 ack）
    expect(ackOutbox).toHaveBeenCalledTimes(2);
    const ackConvIds = ackOutbox.mock.calls.map((c) => (c[2] && c[2].label) || '');
    expect(ackConvIds.some((l) => l.includes('convA'))).toBe(true);
    expect(ackConvIds.some((l) => l.includes('convB'))).toBe(true);
  });

  it('会话 A 被限速拦截，会话 B 仍正常下发（不跨会话阻断、不污染）', async () => {
    await initDownlink(['xiaohongshu']);
    const adapter = {
      sendOutbound: vi.fn(async (text, convId) => {
        if (convId === 'convA') return { ok: false, rateLimited: true, notFound: false };
        return { ok: true, rateLimited: false, notFound: false };
      })
    };
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [
        { msg_id: 'm-A1', content: 'A1', conversation_id: 'convA' },
        { msg_id: 'm-B1', content: 'B1', conversation_id: 'convB' },
        { msg_id: 'm-B2', content: 'B2', conversation_id: 'convB' },
      ],
    });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    // rateRetryWaitMs=1 仅用于单测提速；生产走 rateRetryBaseMs()（minInterval + 抖动）
    await pollDownlink('xiaohongshu', 'acc1', cfg(), {
      sendOutbound: adapter.sendOutbound,
      rateRetryWaitMs: 1,
    });

    // 会话 A：持续被限速 → 初始 1 次 + 有限重试（MAX_RATE_RETRIES=3）后放弃，不在 A 注入 B
    const convACalls = adapter.sendOutbound.mock.calls.filter((c) => c[1] === 'convA');
    expect(convACalls.length).toBeGreaterThanOrEqual(2); // 至少重试一次，而非仅试一次
    // 会话 B：2 次调用都成功（会话 A 限速不阻断会话 B）
    const convBCalls = adapter.sendOutbound.mock.calls.filter((c) => c[1] === 'convB');
    expect(convBCalls.length).toBe(2);
    // 关键：会话 B 仍 ack（不因会话 A 限速被阻断，也不被并行写到 A 的会话里=无跨会话污染）
    expect(ackOutbox).toHaveBeenCalledTimes(1);
    expect(ackOutbox).toHaveBeenCalledWith(expect.any(Object), ['m-B1', 'm-B2'], expect.any(Object));
  });

  it('串行转发：同一会话文案只进入自己的会话输入框（无跨会话污染）', async () => {
    await initDownlink(['xiaohongshu']);
    // 真实 sendOutbound 签名 (text, convId)；断言每条文案确实按 conversation_id 派发到对应会话
    const adapter = {
      sendOutbound: vi.fn(async (text, convId) => ({ ok: true, rateLimited: false, notFound: false })),
    };
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [
        { msg_id: 'm-A', content: '给A的回复', conversation_id: 'convA' },
        { msg_id: 'm-B', content: '给B的回复', conversation_id: 'convB' },
      ],
    });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('xiaohongshu', 'acc1', cfg(), { sendOutbound: adapter.sendOutbound });
    // 每条消息的 content 必须随其 conversation_id 一起传给 sendOutbound（串行保证不串台）
    expect(adapter.sendOutbound).toHaveBeenCalledWith('给A的回复', 'convA', expect.any(Object));
    expect(adapter.sendOutbound).toHaveBeenCalledWith('给B的回复', 'convB', expect.any(Object));
  });
});
