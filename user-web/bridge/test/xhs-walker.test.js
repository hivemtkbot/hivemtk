// xhs 私信「全量遍历同步」单测（需求：一个私信=一个会话=统一收信中心一条记录；遍历所有私信上报）
//
// 覆盖：
//   1. xhs.getConversationList()：从 .sx-contact-item 会话列表 DOM 枚举 [{ id, name, el }]，
//      id 取自 data-key / data-id / data-contactusemid / id / data-uid（规范化后）
//   2. BaseAdapter.syncAllConversations()：逐个点击 → 等待线程渲染 → 回填历史 → 持久化续传
//      - 每个会话触发一次历史回填（统一收信中心一条记录）
//      - 已同步集合生效，二次调用不再重复回填
//      - 无 getConversationList 钩子时安全回退
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { BaseAdapter } from '../src/core/channel-adapter.js';
import { SENDER, CHANNELS } from '../src/core/types.js';
import { getConversationList } from '../src/channels/xhs.js';

beforeEach(() => {
  document.body.innerHTML = '';
});

// 让 jsdom 元素有可见性（真实浏览器 offsetParent 由布局决定；jsdom 恒为 null）
function makeVisible(el) {
  Object.defineProperty(el, 'offsetParent', { configurable: true, get: () => ({}) });
  return el;
}

describe('xhs.getConversationList 枚举会话列表', () => {
  it('从 .sx-contact-item 提取 [{ id, name, el }]，id 取 data-key/data-contactusemid 并规范化', () => {
    const wrapper = makeVisible(document.createElement('div'));
    wrapper.className = 'chat-list-box';
    function addConv(id, dataKey, name) {
      const item = makeVisible(document.createElement('div'));
      item.className = 'sx-contact-item';
      item.setAttribute('data-contactusemid', id);
      if (dataKey) item.setAttribute('data-key', dataKey);
      const nameEl = document.createElement('div');
      nameEl.className = 'nick-name';
      nameEl.textContent = name;
      item.appendChild(nameEl);
      wrapper.appendChild(item);
    }
    addConv('5f8a2b01', 'Total-5f8a2b01', '张三');
    addConv('9c88d777', 'Total-9c88d777', '李四');
    document.body.appendChild(wrapper);

    const list = getConversationList();
    expect(list.length).toBe(2);
    expect(list[0].id).toBe('5f8a2b01');
    expect(list[0].name).toBe('张三');
    expect(list[0].el).toBeTruthy();
    expect(list[1].id).toBe('9c88d777');
    expect(list[1].name).toBe('李四');
  });

  it('无会话列表容器时返回空数组', () => {
    expect(getConversationList()).toEqual([]);
  });

  it('无稳定 id 的节点被过滤（避免重复/乱序）', () => {
    const wrapper = makeVisible(document.createElement('div'));
    wrapper.className = 'chat-list-box';
    const noId = makeVisible(document.createElement('div'));
    noId.className = 'sx-contact-item'; // 无任何 data 属性
    wrapper.appendChild(noId);
    document.body.appendChild(wrapper);
    expect(getConversationList()).toEqual([]);
  });
});

describe('BaseAdapter.syncAllConversations 遍历回填', () => {
  function makeConvItem(id, name) {
    const el = makeVisible(document.createElement('button'));
    el.className = 'sx-contact-item';
    el.setAttribute('data-id', id);
    return { id, name, el };
  }

  // 模拟「点击会话项 → 活动会话变为该会话」的 DOM 行为
  function buildAdapter(convList, clickSpy) {
    let currentActive = null; // 初始不在任何会话
    const hooks = {
      match: () => true,
      getAccountId: () => 'acc1',
      getConversationId: () => currentActive,
      getMessageRoot: () => null,
      getMessageItems: () => [makeVisible(document.createElement('div'))],
      parseMessageItem: () => ({
        sender_type: SENDER.CUSTOMER,
        text: '历史消息',
        message_id: 'm-' + Math.random(),
        timestamp: Date.now(),
      }),
      extractMessages: () => null, // 强制走选择器路径
      getConversationList: () => convList,
    };
    const adapter = new BaseAdapter({ name: 'test', channel: CHANNELS.XHS, SEL: {}, hooks });
    // 点击会话项时切换 currentActive（真实 DOM 中点击会打开线程）
    convList.forEach((conv) => {
      conv.el.addEventListener('click', () => {
        currentActive = conv.id;
        if (clickSpy) clickSpy(conv.id);
      });
    });
    return { adapter, getActive: () => currentActive };
  }

  it('遍历每个会话并各回填一次历史（统一收信中心各一条记录）', async () => {
    const convList = [makeConvItem('5f8a2b01', '张三'), makeConvItem('9c88d76e', '李四'), makeConvItem('1a2b3c4d', '王五')];
    const clickSpy = vi.fn();
    const { adapter } = buildAdapter(convList, clickSpy);
    const onMessage = vi.fn();
    adapter.callbacks.onMessage = onMessage;

    const result = await adapter.syncAllConversations({ throttleMs: 1 });

    expect(result.synced).toBe(3);
    expect(result.failures).toBe(0);
    expect(clickSpy).toHaveBeenCalledTimes(3);
    // 每个会话回填一次历史 → 统一收件箱 3 条记录（纯桥接：统一走 onMessage）
    expect(onMessage).toHaveBeenCalledTimes(3);
    const ids = onMessage.mock.calls.map((c) => c[0].conversation_id);
    expect(ids).toEqual(expect.arrayContaining(['5f8a2b01', '9c88d76e', '1a2b3c4d']));
    expect(onMessage.mock.calls[0][0].direction).toBe('inbound');
  });

  it('已同步集合生效：二次调用不再重复回填', async () => {
    const convList = [makeConvItem('a1', '张三'), makeConvItem('b2', '李四')];
    const { adapter } = buildAdapter(convList);
    const onMessage = vi.fn();
    adapter.callbacks.onMessage = onMessage;

    const r1 = await adapter.syncAllConversations({ throttleMs: 1 });
    expect(r1.synced).toBe(2);
    expect(onMessage).toHaveBeenCalledTimes(2);

    // 二次调用：列表未变，全部已同步 → 跳过
    const r2 = await adapter.syncAllConversations({ throttleMs: 1 });
    expect(r2.synced).toBe(0);
    expect(onMessage).toHaveBeenCalledTimes(2);
  });

  it('点击未打开线程（活动会话未变）→ 该会话跳过且不标记已同步', async () => {
    // 用 button 作为会话项；getConversationId 返回常量 → 点击前后活动会话不变 → 立即走跳过分支（不走 5s 等待）
    const convList = [makeConvItem('a1', '张三')];
    const hooks = {
      match: () => true,
      getAccountId: () => 'acc1',
      getConversationId: () => 'SAME', // 点击前后活动会话不变 → 触发跳过分支
      getMessageRoot: () => null,
      getMessageItems: () => [],
      parseMessageItem: () => ({ sender_type: SENDER.CUSTOMER, text: 'x', message_id: 'm', timestamp: 1 }),
      extractMessages: () => null,
      getConversationList: () => convList,
    };
    const adapter = new BaseAdapter({ name: 'test', channel: CHANNELS.XHS, SEL: {}, hooks });
    const onMessage = vi.fn();
    adapter.callbacks.onMessage = onMessage;

    const r = await adapter.syncAllConversations({ throttleMs: 1, waitActiveMs: 50 });
    expect(r.synced).toBe(0);
    expect(r.failures).toBe(1);
    expect(onMessage).not.toHaveBeenCalled();
  });

  it('无 getConversationList 钩子 → 安全回退（skipped）', async () => {
    const adapter = new BaseAdapter({ name: 'test', channel: CHANNELS.XHS, SEL: {}, hooks: { match: () => true } });
    const r = await adapter.syncAllConversations();
    expect(r.skipped).toBe(true);
    expect(r.reason).toBe('no-hook');
  });
});