// 历史消息同步回归测试（需求：打开私信只读历史、同步到系统，不自动回复）
//   - getMessageRoot 缺失时回退 getMessageListRoot（否则历史回填/Observer 永不生效）
//   - _backfill：存量消息一律走 onHistory（仅落库，不触发 AI）
//   - 历史宽限期：挂载/切换后窗口内，新出现的客户消息仅落库(onHistory)，不触发 AI(onInbound)
//   - 宽限期后：客户新消息才是实时 INBOUND（触发 AI）
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { BaseAdapter } from '../src/core/channel-adapter.js';
import { SENDER, DIRECTION } from '../src/core/types.js';

function msgEl(text, st, id, ts) {
  const el = document.createElement('div');
  el.textContent = text;
  el.__st = st;
  el.__id = id;
  el.__ts = ts;
  return el;
}

function makeAdapter(items) {
  const root = document.createElement('div');
  document.body.appendChild(root);
  const hooks = {
    match: () => true,
    // 注意：故意不提供 getMessageRoot，验证回退到 getMessageListRoot
    getMessageListRoot: () => root,
    getMessageItems: () => items,
    getAccountId: () => 'acc1',
    getConversationId: () => 'conv1',
    parseMessageItem: (it) => ({
      text: it.textContent,
      sender_type: it.__st,
      message_id: it.__id,
      timestamp: it.__ts || Date.now(),
    }),
  };
  const a = new BaseAdapter({ name: 't', channel: 'douyin', SEL: { MSG_ITEM: '[data-x]' }, hooks });
  return { a, root };
}

let current = null;
beforeEach(() => { document.body.innerHTML = ''; });
afterEach(() => { if (current) { current.stop(); current = null; } document.body.innerHTML = ''; });

describe('getMessageRoot 回退', () => {
  it('缺失 getMessageRoot 时回退到 getMessageListRoot（非空）', () => {
    const { a, root } = makeAdapter([]);
    expect(a.getMessageRoot()).toBe(root);
  });
});

describe('历史回填 _backfill', () => {
  it('存量消息一律走 onHistory，绝不触发 onInbound', () => {
    const items = [
      msgEl('客户历史消息A', SENDER.CUSTOMER, 'm1', 1700000000000),
      msgEl('我的历史回复B', SENDER.AGENT, 'm2', 1700000001000),
    ];
    const { a } = makeAdapter(items);
    current = a;
    const calls = { inbound: [], history: [] };
    a.start({
      onInbound: (m) => calls.inbound.push(m),
      onHistory: (m, d) => calls.history.push({ m, d }),
    });
    expect(calls.inbound.length).toBe(0); // 关键：历史不触发 AI
    expect(calls.history.length).toBe(2);
    const cust = calls.history.find((h) => h.m.content === '客户历史消息A');
    const self = calls.history.find((h) => h.m.content === '我的历史回复B');
    expect(cust.d).toBe(DIRECTION.INBOUND);
    expect(self.d).toBe(DIRECTION.OUTBOUND);
  });
});

describe('历史宽限期', () => {
  it('宽限期内新出现的客户消息仅落库(onHistory)，不触发 AI(onInbound)', () => {
    const { a } = makeAdapter([]);
    current = a;
    const calls = { inbound: [], history: [] };
    a.start({
      onInbound: (m) => calls.inbound.push(m),
      onHistory: (m, d) => calls.history.push({ m, d }),
    });
    // 仍在宽限期（默认 8s）内
    a._handleIncremental(msgEl('宽限期内客户新消息', SENDER.CUSTOMER, 'm3', Date.now()));
    expect(calls.inbound.length).toBe(0);
    expect(calls.history.length).toBe(1);
    expect(calls.history[0].d).toBe(DIRECTION.INBOUND);
  });

  it('宽限期后新出现的客户消息才走实时 INBOUND（触发 AI）', () => {
    const { a } = makeAdapter([]);
    current = a;
    const calls = { inbound: [], history: [] };
    a.start({
      onInbound: (m) => calls.inbound.push(m),
      onHistory: (m, d) => calls.history.push({ m, d }),
    });
    a.historyGraceUntil = Date.now() - 1; // 模拟宽限期已过
    a._handleIncremental(msgEl('实时客户新消息', SENDER.CUSTOMER, 'm4', Date.now()));
    expect(calls.inbound.length).toBe(1);
    expect(calls.inbound[0].content).toBe('实时客户新消息');
    expect(calls.history.length).toBe(0);
  });

  it('切换会话后宽限期重置，新会话历史再次仅落库', () => {
    const { a } = makeAdapter([]);
    current = a;
    const calls = { inbound: [], history: [] };
    a.start({
      onInbound: (m) => calls.inbound.push(m),
      onHistory: (m, d) => calls.history.push({ m, d }),
    });
    a.historyGraceUntil = Date.now() - 1; // 先让宽限期过期
    // 模拟切换会话：会话 id 变化 → 重新挂载并开启宽限期（复刻 _startConvPolling 的切换逻辑）
    a.conversationId = 'conv1';
    a.hooks.getConversationId = () => 'conv2';
    a.conversationId = 'conv2';
    a._attachConversation();
    a._handleIncremental(msgEl('切回会话后的客户消息', SENDER.CUSTOMER, 'm5', Date.now()));
    expect(calls.inbound.length).toBe(0); // 切换后仍处宽限期，不触发 AI
    expect(calls.history.length).toBe(1);
  });
});
