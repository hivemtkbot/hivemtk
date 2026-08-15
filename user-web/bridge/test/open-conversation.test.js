import { describe, it, expect, vi, beforeEach } from 'vitest';
import { BaseAdapter } from '../src/core/channel-adapter.js';

function makeButton(id) {
  const el = document.createElement('button');
  el.addEventListener('click', () => { window.__active = id; });
  return el;
}

class TestAdapter extends BaseAdapter {
  constructor({ channel = 'xiaohongshu', listItems = [], activeId = null } = {}) {
    super({ name: 'test', channel, SEL: {}, hooks: {} });
    this._listItems = listItems;
    this._activeId = activeId;
    this.callbacks = {};
    this.log = { info() {}, warn() {}, error() {} };
    this.getConversationList = vi.fn(() => this._listItems);
    this.getConversationId = vi.fn(() => window.__active || this._activeId);
  }
}

beforeEach(() => { window.__active = null; });

describe('openConversation 兜底（列表未命中）', () => {
  it('conv:<名> 昵称派生 id：按列表项 name 匹配点击打开，返回真实 id', async () => {
    const realId = '63bd5238000000000000abcd';
    const adapter = new TestAdapter({
      channel: 'xiaohongshu',
      listItems: [{ id: realId, name: '雪大王', el: makeButton(realId) }],
      activeId: 'other-conv',
    });
    const opened = await adapter.openConversation('conv:雪大王', { backfill: false, exact: true });
    expect(opened).toBe(realId);
  });

  it('conv:<名> 昵称派生 id：列表里无对应 name 时返回 null（不可达）', async () => {
    const adapter = new TestAdapter({
      channel: 'xiaohongshu',
      listItems: [{ id: 'x', name: '别人', el: makeButton('x') }],
      activeId: 'other-conv',
    });
    const opened = await adapter.openConversation('conv:雪大王', { backfill: false, exact: true });
    expect(opened).toBeNull();
  });

  it('真实 id 屏外：URL 导航（SPA /chat/{id}）打开并同步等待就绪', async () => {
    const offId = 'real-offscreen-id';
    // 列表里没有该会话；pushState 到 /chat/{id} 后 getConversationId 解析出 id
    const adapter = new TestAdapter({ channel: 'xiaohongshu', listItems: [], activeId: 'other-conv' });
    adapter.getConversationId = vi.fn(() => {
      const m = (window.location.pathname || '').match(/\/chat\/([^/?#]+)/);
      return m ? m[1] : adapter._activeId;
    });
    const opened = await adapter.openConversation(offId, { backfill: false, exact: true });
    expect(opened).toBe(offId);
  });

  it('非 xiaohongshu 渠道：真实 id 屏外不触发 URL 导航兜底（避免跨站导航）', async () => {
    const adapter = new TestAdapter({ channel: 'weibo', listItems: [], activeId: 'other-conv' });
    const spy = vi.spyOn(history, 'pushState');
    const opened = await adapter.openConversation('real-offscreen-id', { backfill: false, exact: true });
    expect(opened).toBeNull();
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });
});

