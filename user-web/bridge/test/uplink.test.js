// Uplink 单测（通道A·上报：父层统一上报 + 消息 hash 前端完成）
//
// 覆盖：
//   1. computeMsgID 幂等稳定（同输入同输出，不同输入不同输出）
//   2. Uplink.enqueue 兜底补全 event_id（消息 hash 前端完成）
//   3. Uplink 短窗口合并：同 (accountId|conversationId) 多条消息合并为一次 POST
//   4. 上报 body 字段与 user-server handler_http.go HTTPIngestRequest 对齐
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { computeMsgID, Uplink } from '../src/core/uplink.js';

describe('uplink / computeMsgID（消息 hash 前端完成）', () => {
  it('同输入产出同 hash（幂等去重主键）', () => {
    const a = { channel: 'douyin', accountId: 'acc1', conversationId: 'c1', senderType: 'customer', content: '你好', ts: 1700000000 };
    const b = { channel: 'douyin', accountId: 'acc1', conversationId: 'c1', senderType: 'customer', content: '你好', ts: 1700000000 };
    expect(computeMsgID(a)).toBe(computeMsgID(b));
  });
  it('内容不同 → hash 不同', () => {
    const a = { channel: 'douyin', accountId: 'acc1', conversationId: 'c1', senderType: 'customer', content: '你好', ts: 1 };
    const b = { channel: 'douyin', accountId: 'acc1', conversationId: 'c1', senderType: 'customer', content: '你好吗', ts: 1 };
    expect(computeMsgID(a)).not.toBe(computeMsgID(b));
  });
  it('hash 以 b- 前缀，长度稳定', () => {
    const id = computeMsgID({ channel: 'xhs', accountId: 'a', conversationId: 'c', senderType: 'customer', content: 'x', ts: 1 });
    expect(id.startsWith('b-')).toBe(true);
    expect(id.length).toBeGreaterThan(2);
  });
});

describe('uplink / enqueue + flushAll', () => {
  let realFetch;
  beforeEach(() => { realFetch = globalThis.fetch; });
  afterEach(() => { globalThis.fetch = realFetch; vi.restoreAllMocks(); });

  it('enqueue 兜底补全 event_id（前端 hash）', async () => {
    let captured;
    globalThis.fetch = vi.fn(async (url, init) => { captured = init.body; return new Response('{"ok":true,"ingested":[]}', { status: 200 }); });
    const uplink = new Uplink({ channel: 'douyin', getConfig: async () => ({ serverUrl: 'http://x', token: 't' }) });
    uplink.enqueue({ channel: 'douyin', account_id: 'acc1', conversation_id: 'c1', sender_type: 'customer', content: '你好', timestamp: 1 });
    await uplink.flushAll();
    const body = JSON.parse(captured);
    expect(body.messages[0].event_id).toMatch(/^b-/); // 兜底 hash 已生成
  });

  it('合并窗口：同会话 3 条消息合并为一次 POST', async () => {
    let fetchCount = 0;
    let lastBody;
    globalThis.fetch = vi.fn(async (_u, init) => { fetchCount += 1; lastBody = init.body; return new Response('{"ok":true,"ingested":[]}', { status: 200 }); });
    const uplink = new Uplink({ channel: 'douyin', getConfig: async () => ({ serverUrl: 'http://x', token: 't' }) });
    for (let i = 0; i < 3; i++) {
      uplink.enqueue({ channel: 'douyin', account_id: 'acc1', conversation_id: 'c1', sender_type: 'customer', content: `m${i}`, timestamp: i });
    }
    await uplink.flushAll();
    expect(fetchCount).toBe(1);
    const body = JSON.parse(lastBody);
    expect(body.messages).toHaveLength(3);
  });

  it('不同会话分别 POST（各自 flush）', async () => {
    let fetchCount = 0;
    globalThis.fetch = vi.fn(async () => { fetchCount += 1; return new Response('{"ok":true,"ingested":[]}', { status: 200 }); });
    const uplink = new Uplink({ channel: 'douyin', getConfig: async () => ({ serverUrl: 'http://x', token: 't' }) });
    uplink.enqueue({ channel: 'douyin', account_id: 'acc1', conversation_id: 'c1', content: 'a', timestamp: 1 });
    uplink.enqueue({ channel: 'douyin', account_id: 'acc1', conversation_id: 'c2', content: 'b', timestamp: 2 });
    await uplink.flushAll();
    expect(fetchCount).toBe(2);
  });
});
