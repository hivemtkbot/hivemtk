// 回归测试：修复「私信列表视图下把联系人昵称当成聊天消息无限上行」。
// 覆盖两个叠加 bug：
//   1) 去重 key 含 Date.now() → 同一 DOM 节点每次 3s 兜底扫描都算出不同 key，
//      seen 永远去重失败 → 同一批昵称被当成「新消息」无限重复上行。
//   2) 无活动会话（conv:null，私信列表视图）时不应捕获消息——命中多是列表里的联系人昵称。
import { describe, it, expect, beforeEach } from 'vitest';
import { BaseAdapter } from '../src/core/channel-adapter.js';
import { SENDER, DIRECTION } from '../src/core/types.js';

// 极简 DOM 节点模拟（jsdom 下 document 可用，但此处用纯对象避免环境差异）
function fakeNode(text, senderType = SENDER.CUSTOMER) {
  // 模拟 parseMessageItem 需要的字段；节点本身作为 WeakSet key 用真实对象即可。
  return { __text: text, __sender: senderType };
}

function makeAdapter({ items, convId }) {
  const hooks = {
    getMessageItems: () => items,
    parseMessageItem: (node) => ({
      message_id: node.__text,
      sender_type: node.__sender,
      text: node.__text,
      media_url: '',
      timestamp: Date.now(),
      raw: '',
    }),
    getConversationId: () => convId,
    getMessageListRoot: () => ({}),
  };
  const adapter = new BaseAdapter({ name: 't', channel: 'douyin_web', SEL: {}, hooks });
  return adapter;
}

describe('去重 + 列表噪声防护', () => {
  let inbound;
  let history;

  beforeEach(() => {
    inbound = [];
    history = [];
  });

  it('无活动会话（conv:null）时反复扫描也不上行任何消息', () => {
    const names = [fakeNode('钓点王'), fakeNode('小马哥不空军'), fakeNode('吴小小')];
    const adapter = makeAdapter({ items: names, convId: null });
    const cb = { onMessage: (m) => inbound.push(m) };
    adapter.start(cb);
    // 模拟 3s 兜底扫描连续命中同一批节点
    for (let i = 0; i < 5; i++) adapter._scanIncremental();
    expect(inbound).toHaveLength(0);
  });

  it('同一批节点（哪怕 timestamp 每次不同）只上行一次，不会无限重复', () => {
    const msgs = [fakeNode('你好在吗'), fakeNode('怎么收费')];
    const adapter = makeAdapter({ items: msgs, convId: 'MS4w_test' });
    const cb = { onMessage: (m) => inbound.push(m) };
    adapter.start(cb);
    // 10 次兜底扫描，每次 parseMessageItem 的 timestamp=Date.now() 都不同
    for (let i = 0; i < 10; i++) adapter._scanIncremental();
    expect(inbound).toHaveLength(2);
    expect(inbound.map((m) => m.content).sort()).toEqual(['你好在吗', '怎么收费']);
  });

  it('会话切换后新节点正常作为新消息上行', () => {
    const a = fakeNode('第一批');
    const adapter = makeAdapter({ items: [a], convId: 'conv-1' });
    const cb = { onMessage: (m) => inbound.push(m) };
    adapter.start(cb);
    adapter._scanIncremental();
    expect(inbound).toHaveLength(1);

    // 切换会话（convPolling 会调用 _attachConversation；这里直接模拟新节点 + 新 conv）
    const b = fakeNode('第二批');
    adapter.conversationId = 'conv-2';
    adapter.hooks.getConversationId = () => 'conv-2';
    adapter.getMessageItems = () => [b];
    adapter._scanIncremental();
    expect(inbound).toHaveLength(2);
    expect(inbound[1].content).toBe('第二批');
  });
});
