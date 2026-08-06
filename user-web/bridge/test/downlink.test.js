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
    const adapter = { sendOutbound: vi.fn(async () => true) };
    getOutbox.mockResolvedValue({ status: 'ok', messages: [{ msg_id: 'm-d1', content: 'hi', conversation_id: 'c1' }] });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('douyin', 'acc1', cfg(), { sendOutbound: adapter.sendOutbound });
    const stored = await chrome.storage.local.get(['bridge_sent_douyin']);
    expect(stored['bridge_sent_douyin']).toContain('m-d1');
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
    const adapter = { sendOutbound: vi.fn(async () => true) };
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
    const adapter = { sendOutbound: vi.fn(async () => false) };
    getOutbox.mockResolvedValue({ status: 'ok', messages: [{ msg_id: 'm-x1', content: 'hi', conversation_id: 'c1' }] });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    const c = cfg();
    await pollDownlink('xianyu', 'acc1', c, { sendOutbound: adapter.sendOutbound }); // 失败
    expect(ackOutbox).not.toHaveBeenCalled();
    adapter.sendOutbound.mockResolvedValue(true); // 下一轮：success
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
    const adapter = { sendOutbound: vi.fn(async () => true) };
    getOutbox.mockResolvedValue({ status: 'ok', messages: [{ msg_id: 'm-ackfail', content: 'hi', conversation_id: 'c1' }] });
    // 第一轮：ack 失败
    ackOutbox.mockResolvedValue({ status: 'error' });
    const c = cfg();
    await pollDownlink('douyin', 'acc1', c, { sendOutbound: adapter.sendOutbound });
    // 本地缓存绝不能写入（否则下轮被拦截，消息永久丢失）
    let stored = await chrome.storage.local.get(['bridge_sent_douyin']);
    expect(stored['bridge_sent_douyin'] || []).not.toContain('m-ackfail');
    expect(adapter.sendOutbound).toHaveBeenCalledTimes(1);

    // 第二轮：ack 成功，才写入缓存
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('douyin', 'acc1', c, { sendOutbound: adapter.sendOutbound });
    stored = await chrome.storage.local.get(['bridge_sent_douyin']);
    expect(stored['bridge_sent_douyin']).toContain('m-ackfail');
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
    const adapter = { sendOutbound: vi.fn(async () => true) };
    getOutbox.mockResolvedValue({ status: 'ok', messages: [{ msg_id: 'm-xh1', content: '', conversation_id: 'c1' }] });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('xiaohongshu', 'acc1', cfg(), { sendOutbound: adapter.sendOutbound });
    expect(adapter.sendOutbound).not.toHaveBeenCalled();
    expect(ackOutbox).not.toHaveBeenCalled();
  });
});
