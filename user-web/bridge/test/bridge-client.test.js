// bridge-client 单测：验证客户端帧协议与服务端一致。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { BridgeClient } from '../src/core/bridge-client.js';
import { FRAME } from '../src/core/types.js';

class MockWS {
  constructor(url) {
    this.url = url;
    this.readyState = 0; // CONNECTING
    this.sent = [];
    MockWS.last = this;
    MockWS.all.push(this);
  }
  open() {
    this.readyState = 1;
    this.onopen && this.onopen();
  }
  send(data) {
    this.sent.push(data);
  }
  close() {
    this.readyState = 3;
    this.onclose && this.onclose();
  }
  fireMessage(obj) {
    this.onmessage && this.onmessage({ data: JSON.stringify(obj) });
  }
}
MockWS.all = [];

beforeEach(() => {
  MockWS.all = [];
  global.WebSocket = MockWS;
});

describe('BridgeClient 协议', () => {
  it('连接后发送 register 帧，URL 带 channel/account_id/token', () => {
    const c = new BridgeClient({
      WebSocket: MockWS,
      serverUrl: 'http://localhost:8204',
      channel: 'douyin_web',
      accountId: '123',
      token: 'sek',
    });
    c.connect();
    MockWS.last.open();
    expect(MockWS.last.url).toContain('/api/ws/bridge');
    expect(MockWS.last.url).toContain('channel=douyin_web');
    expect(MockWS.last.url).toContain('account_id=123');
    expect(MockWS.last.url).toContain('token=sek');
    const first = JSON.parse(MockWS.last.sent[0]);
    expect(first.type).toBe(FRAME.REGISTER);
    expect(first.channel).toBe('douyin_web');
  });

  it('收到 outbound 帧 -> 触发 onOutbound(reply)', () => {
    const onOutbound = vi.fn();
    const c = new BridgeClient({ WebSocket: MockWS, serverUrl: 'http://x', channel: 'xhs_web', accountId: 'a' });
    c.onOutbound = onOutbound;
    c.connect();
    MockWS.last.open();
    // I1 修复：用 content 字段（与服务端 UnifiedReply 一致），不再用 text
    const reply = { channel: 'xhs_web', account_id: 'a', conversation_id: 'c1', content: 'hi' };
    MockWS.last.fireMessage({ type: FRAME.OUTBOUND, reply });
    expect(onOutbound).toHaveBeenCalledWith(reply);
  });

  it('收到 pong 帧 -> 标记为 alive（不抛错）', () => {
    const c = new BridgeClient({ WebSocket: MockWS, serverUrl: 'http://x', channel: 'tiktok_web', accountId: 't' });
    c.connect();
    MockWS.last.open();
    expect(() => MockWS.last.fireMessage({ type: FRAME.PONG })).not.toThrow();
  });

  it('sendInbound 发送 inbound 帧', () => {
    const c = new BridgeClient({ WebSocket: MockWS, serverUrl: 'http://x', channel: 'douyin_web', accountId: '1' });
    c.connect();
    MockWS.last.open();
    // I1 修复：用 content 字段（与服务端 UnifiedMessage 一致），不再用 text
    const msg = { channel: 'douyin_web', account_id: '1', conversation_id: 'c', content: 'hello' };
    c.sendInbound(msg);
    const last = JSON.parse(MockWS.last.sent[MockWS.last.sent.length - 1]);
    expect(last.type).toBe(FRAME.INBOUND);
    expect(last.message.content).toBe('hello');
  });
});
