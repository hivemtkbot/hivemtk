// 协议对齐测试：证明扩展端帧/字段与服务端 frames.go 严格一致（需求③ 论证）。
import { describe, it, expect } from 'vitest';
import { FRAME, makeUnifiedMessage, parseUnifiedReply, SENDER, DIRECTION, RATE_LIMIT_DEFAULTS } from '../src/core/types.js';

describe('协议常量对齐服务端', () => {
  it('帧类型与服务端 frames.go 一致', () => {
    expect(FRAME.INBOUND).toBe('inbound_message');
    expect(FRAME.OUTBOUND).toBe('outbound_reply');
    expect(FRAME.HISTORY).toBe('history');
    expect(FRAME.REGISTER).toBe('register');
    expect(FRAME.PONG).toBe('pong');
    expect(FRAME.ACK).toBe('ack');
    expect(FRAME.PING).toBe('ping');
  });
});

describe('makeUnifiedMessage 字段对齐 UnifiedMessage', () => {
  it('输出服务端字段名（content/event_id/sender_id 而非 text/message_id）', () => {
    const m = makeUnifiedMessage({
      channel: 'douyin_web', account_id: 'A1', conversation_id: 'C1',
      event_id: 'M1', sender_type: SENDER.CUSTOMER, content: '你好',
      timestamp: 1700000000000,
    });
    expect(m.content).toBe('你好');      // 服务端读 Content
    expect(m.event_id).toBe('M1');        // 服务端读 EventID
    expect(m.sender_id).toBe('C1');       // 客户消息 sender_id = conversation_id
    expect(m.channel).toBe('douyin_web');
    expect(m.account_id).toBe('A1');
    expect(m.conversation_id).toBe('C1');
    expect(m.timestamp).toBe(1700000000000);
    expect(m.text).toBeUndefined();       // 不应出现旧字段
    expect(m.message_id).toBeUndefined();
  });

  it('自己/AI 消息 sender_id 回退为 account_id', () => {
    const m = makeUnifiedMessage({ channel: 'xhs_web', account_id: 'A2', conversation_id: 'C2', sender_type: SENDER.AGENT, content: '回复' });
    expect(m.sender_id).toBe('A2');
    expect(m.direction).toBeUndefined();
  });

  it('history 帧携带 direction', () => {
    const m = makeUnifiedMessage({ channel: 'tiktok_web', account_id: 'A3', conversation_id: 'C3', sender_type: SENDER.CUSTOMER, content: 'x', direction: DIRECTION.INBOUND });
    expect(m.direction).toBe('inbound');
  });
});

describe('parseUnifiedReply 读取服务端 content', () => {
  it('读取 content（不是 text）', () => {
    const r = parseUnifiedReply({ channel: 'xhs_web', account_id: 'A', conversation_id: 'C', content: '你好', msg_type: 'text', media_url: '', reply_to_event_id: 'M1' });
    expect(r.content).toBe('你好');
    expect(r.conversation_id).toBe('C');
    expect(r.reply_to_event_id).toBe('M1');
  });
});

describe('默认风控参数存在', () => {
  it('RATE_LIMIT_DEFAULTS 各字段存在', () => {
    for (const k of ['accountCapacity', 'accountRefillPerMin', 'minIntervalMs', 'conversationCooldownMs', 'conversationPerHour', 'jitterMinMs', 'jitterMaxMs', 'dedupWindowMs']) {
      expect(RATE_LIMIT_DEFAULTS[k]).toBeGreaterThan(0);
    }
  });
});
