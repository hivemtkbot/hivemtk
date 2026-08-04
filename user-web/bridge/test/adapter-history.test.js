// 历史消息同步回归测试（需求：打开私信只读历史、同步到系统，不自动回复）
//   - getMessageRoot 缺失时回退 getMessageListRoot（否则历史回填/Observer 永不生效）
//   - _backfill：存量消息一律走 onHistory（仅落库，不触发 AI）
//   - 历史宽限期：挂载/切换后窗口内，新出现的客户消息仅落库(onHistory)，不触发 AI(onInbound)
//   - 宽限期后：客户新消息才是实时 INBOUND（触发 AI）
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { BaseAdapter } from '../src/core/channel-adapter.js';
import { SENDER, DIRECTION } from '../src/core/types.js';
import * as selectorAi from '../src/core/selector-ai.js';

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
  it('一个会话 = 一条会话级消息，内含全部多轮历史，绝不触发 onInbound', () => {
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
    // 点3：回填只发一条会话级帧，多轮历史内嵌 history[]
    expect(calls.history.length).toBe(1);
    const frame = calls.history[0];
    expect(frame.m.conversation_id).toBe('conv1');
    expect(frame.m.history).toBeDefined();
    expect(frame.m.history.length).toBe(2);
    const cust = frame.m.history.find((h) => h.content === '客户历史消息A');
    const self = frame.m.history.find((h) => h.content === '我的历史回复B');
    expect(cust.direction).toBe(DIRECTION.INBOUND);
    expect(self.direction).toBe(DIRECTION.OUTBOUND);
    // 有客户消息 → 帧级摘要方向为 inbound（保持 onHistory 第二参语义）
    expect(frame.d).toBe(DIRECTION.INBOUND);
  });

  it('回填再次触发（宽限延迟 1.5s）不重复上报：seen 去重 → 不再发帧', () => {
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
    expect(calls.history.length).toBe(1); // 首次回填：1 条会话级帧
    a._backfill(); // 二次回填：同批消息已被 seen 去重 → 不发帧
    expect(calls.history.length).toBe(1);
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

describe('群聊 + 实时上下文窗口（点3）', () => {
  function groupMsgEl(text, st, id, senderName) {
    const el = msgEl(text, st, id, Date.now());
    el.__senderName = senderName;
    el.__isGroup = true;
    el.__groupId = 'group-1';
    el.__groupName = '产品交流群';
    return el;
  }

  function makeGroupAdapter(items) {
    const root = document.createElement('div');
    document.body.appendChild(root);
    const hooks = {
      match: () => true,
      getMessageListRoot: () => root,
      getMessageItems: () => items,
      getAccountId: () => 'acc1',
      getConversationId: () => 'group-1',
      parseMessageItem: (it) => ({
        text: it.textContent,
        sender_type: it.__st,
        message_id: it.__id,
        timestamp: it.__ts || Date.now(),
        sender_name: it.__senderName || '',
        is_group: !!it.__isGroup,
        group_id: it.__groupId || '',
        group_name: it.__groupName || '',
      }),
    };
    return new BaseAdapter({ name: 't', channel: 'xhs', SEL: { MSG_ITEM: '[data-x]' }, hooks });
  }

  it('群聊会话级回填：1 条帧，is_group/group_id/group_name 与逐轮 sender_name 上送', () => {
    const items = [
      groupMsgEl('有人在吗', SENDER.CUSTOMER, 'g1', '张三'),
      groupMsgEl('在的', SENDER.AGENT, 'g2', '客服小王'),
      groupMsgEl('@客服小王 帮我查下订单', SENDER.CUSTOMER, 'g3', '李四'),
    ];
    const a = makeGroupAdapter(items);
    current = a;
    const calls = { history: [] };
    a.start({ onHistory: (m, d) => calls.history.push({ m, d }) });
    expect(calls.history.length).toBe(1); // 一个会话 = 一条消息
    const frame = calls.history[0].m;
    expect(frame.is_group).toBe(true);
    expect(frame.group_id).toBe('group-1');
    expect(frame.group_name).toBe('产品交流群');
    expect(frame.history.length).toBe(3);
    expect(frame.history.map((h) => h.sender_name)).toEqual(['张三', '客服小王', '李四']);
    // 群聊消息 sender_id 聚合到群 id
    expect(frame.sender_id).toBe('group-1');
  });

  function groupParsed(text, st, id, senderName) {
    return {
      text,
      sender_type: st,
      message_id: id,
      timestamp: Date.now(),
      sender_name: senderName,
      is_group: true,
      group_id: 'group-1',
      group_name: '产品交流群',
    };
  }

  it('群聊去重按成员区分：不同成员发相同文本不被误删（需求3 多轮完整性）', () => {
    const a = makeGroupAdapter([]);
    current = a;
    const calls = { inbound: [] };
    a.start({ onInbound: (m) => calls.inbound.push(m) });
    a.historyGraceUntil = Date.now() - 1;
    // 两个不同成员都发「好的」：dedup key 含 sender_name，两条都保留
    a._ingest(groupParsed('好的', SENDER.CUSTOMER, 'g1', '张三'));
    a._ingest(groupParsed('好的', SENDER.CUSTOMER, 'g2', '李四'));
    expect(calls.inbound.length).toBe(2);
    expect(calls.inbound[0].content).toBe('好的');
    expect(calls.inbound[1].content).toBe('好的');
    expect(calls.inbound[0].history).toBeDefined();
  });

  it('同一成员重复发相同文本仍被去重（防回环/防刷屏不变）', () => {
    const a = makeGroupAdapter([]);
    current = a;
    const calls = { inbound: [] };
    a.start({ onInbound: (m) => calls.inbound.push(m) });
    a.historyGraceUntil = Date.now() - 1;
    a._ingest(groupParsed('好的', SENDER.CUSTOMER, 'g1', '张三'));
    a._ingest(groupParsed('好的', SENDER.CUSTOMER, 'g2', '张三'));
    expect(calls.inbound.length).toBe(1); // 同成员同文本 → 只上行一次
  });

  it('宽限期后实时 inbound 携带该会话最近多轮上下文窗口', () => {
    const a = makeGroupAdapter([]);
    current = a;
    const calls = { inbound: [] };
    a.start({ onInbound: (m) => calls.inbound.push(m) });
    a.historyGraceUntil = Date.now() - 1; // 宽限期已过
    // 先来 3 轮（进窗口），再来触发 AI 的新消息；每条客户消息均实时 inbound
    a._ingest(groupParsed('第一轮', SENDER.CUSTOMER, 'w1', '张三'));
    a._ingest(groupParsed('我方回复', SENDER.AGENT, 'w2', '客服小王'));
    a._ingest(groupParsed('第二轮', SENDER.CUSTOMER, 'w3', '李四'));
    a._ingest(groupParsed('第三轮请回复', SENDER.CUSTOMER, 'w4', '王五'));
    // 客户消息各触发一次 inbound（w1/w3/w4），AI 回复（w2）仅落库不触发
    expect(calls.inbound.length).toBe(3);
    const frame = calls.inbound[2];
    expect(frame.content).toBe('第三轮请回复');
    // 最新一条 inbound 帧内含该会话最近多轮（含刚触发消息）
    expect(frame.history.length).toBe(4);
    expect(frame.history[0].content).toBe('第一轮');
    expect(frame.history[0].direction).toBe(DIRECTION.INBOUND);
    expect(frame.history[1].direction).toBe(DIRECTION.OUTBOUND); // 我方回复方向 outbound
    expect(frame.history[3].direction).toBe(DIRECTION.INBOUND);
  });
});

describe('历史项 sender_id 解析（统一收信中心聚合键）', () => {
  function makeCidAdapter(getCid) {
    const hooks = {
      match: () => true,
      getAccountId: () => 'acc1',
      getConversationId: () => getCid(),
      getMessageRoot: () => null,
    };
    return new BaseAdapter({ name: 't', channel: 'xhs', SEL: {}, hooks });
  }

  it('1:1 客户消息 → sender_id = 会话 id（对方），非空', () => {
    const a = makeCidAdapter(() => 'conv-xhs-1');
    const item = a._historyItem(
      { sender_type: SENDER.CUSTOMER, text: '在吗', message_id: 'm1', timestamp: 1 },
      DIRECTION.INBOUND
    );
    expect(item.sender_id).toBe('conv-xhs-1');
    expect(item.receiver_id).toBe('');
  });

  it('自己/AI 回复 → sender_id = account_id，receiver_id = 会话 id', () => {
    const a = makeCidAdapter(() => 'conv-xhs-1');
    const item = a._historyItem(
      { sender_type: SENDER.AGENT, text: '在的', message_id: 'm2', timestamp: 2 },
      DIRECTION.OUTBOUND
    );
    expect(item.sender_id).toBe('acc1');
    expect(item.receiver_id).toBe('conv-xhs-1');
  });

  it('群聊消息 → sender_id = 群 id（group_id 优先）', () => {
    const a = makeCidAdapter(() => 'group-1');
    const item = a._historyItem(
      { sender_type: SENDER.CUSTOMER, text: '@所有人 在吗', message_id: 'g1', timestamp: 1, is_group: true, group_id: 'group-1', sender_name: '张三' },
      DIRECTION.INBOUND
    );
    expect(item.sender_id).toBe('group-1');
    expect(item.sender_name).toBe('张三');
  });
});

describe('纯规则架构（无 LLM 抽取器）', () => {
  function makeCidRootAdapter(getCid, root, items) {
    const hooks = {
      match: () => true,
      getAccountId: () => 'acc1',
      getConversationId: () => getCid(),
      getMessageRoot: () => root,
      getMessageItems: () => items || [],
      parseMessageItem: (it) => ({
        sender_type: SENDER.CUSTOMER,
        text: it.textContent,
        message_id: 'm-' + (it.__id || Math.random()),
        timestamp: Date.now(),
      }),
      extractMessages: () => [{ text: 'should-be-ignored', sender_type: 'customer' }], // 残留 hook：纯规则架构下忽略
    };
    return new BaseAdapter({ name: 't', channel: 'xhs', SEL: {}, hooks });
  }

  it('_scanIncremental 走 selector 路径（getMessageItems），忽略 extractMessages 残留', () => {
    const root = document.createElement('div');
    document.body.appendChild(root);
    const a = makeCidRootAdapter(() => 'conv1', root, []);
    current = a;
    const inbound = [];
    a.start({ onInbound: (m) => inbound.push(m), onHistory: () => {} });
    a.historyGraceUntil = Date.now() - 1; // 过宽限期 → 实时 inbound
    // 新增一条消息（模拟 MutationObserver/新消息到达），_scanIncremental 走 selector 路径
    const msgEl = document.createElement('div');
    msgEl.textContent = '来自选择器路径的消息';
    a.hooks.getMessageItems = () => [msgEl];
    a._scanIncremental();
    // extractMessages 返回的假数据不参与；真实消息来自 getMessageItems
    expect(inbound.length).toBe(1);
    expect(inbound[0].content).toBe('来自选择器路径的消息');
  });

  it('_backfill 不依赖 extractMessages（纯 getMessageItems 回填）', () => {
    const root = document.createElement('div');
    document.body.appendChild(root);
    const msgEl = document.createElement('div');
    msgEl.textContent = '回填历史消息';
    const a = makeCidRootAdapter(() => 'conv1', root, [msgEl]);
    current = a;
    const history = [];
    a.start({ onInbound: () => {}, onHistory: (m) => history.push(m) });
    // start() 已回填一次；再手动触发一次验证走 selector
    a._backfill();
    expect(history.length).toBeGreaterThan(0);
  });
});
