import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.stubGlobal('chrome', { storage: { local: { get() {}, set() {} } } });

const { getAccountId } = await import('../src/channels/douyin.js');

describe('douyin getAccountId 浮层兜底', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    try { localStorage.clear(); } catch (e) {  }
  });
  afterEach(() => { document.body.innerHTML = ''; });

  it('浮层无左导航个人链接、仅会话项有真实 token id 时，返回真实 id（非空串）', () => {
    document.body.innerHTML = `
      <div class="conversationConversationListwrapper">
        <a href="//www.douyin.com/user/MS4wABC123">会话A</a>
        <a href="//www.douyin.com/user/MS4wDEF456">会话B</a>
      </div>`;
    const id = getAccountId();
    expect(id).toBe('MS4wABC123');
    expect(id).not.toBe('');
    expect(id).not.toContain('self');
    expect(id).not.toContain('unknown');
  });

  it('仅「我的」链接指向 /user/self 占位时，仍返回空串以外的稳定值（不触发 WS 401）', () => {
    document.body.innerHTML = `<a href="//www.douyin.com/user/self?from_nav=1">我的</a>`;
    const id = getAccountId();
    expect(id).not.toBe(''); 
    expect(id).toContain('unknown');
  });

  it('完全无 /user/ 链接时，返回 unknown 兜底（非空串）', () => {
    document.body.innerHTML = `<div class="some-page">无账号线索</div>`;
    expect(getAccountId()).toBe('douyin-unknown');
  });

  it('浮层下取不到真实 id 时，复用 localStorage 缓存的真实 id', () => {
    localStorage.setItem('hivebridge:account:douyin', 'MS4wCachedId');
    document.body.innerHTML = `<a href="//www.douyin.com/user/self">我的</a>`;
    expect(getAccountId()).toBe('MS4wCachedId');
  });
});

