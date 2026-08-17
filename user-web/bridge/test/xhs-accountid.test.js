import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { getAccountId, getConversationId, normalizeContactId, isXhsMessagePage } from '../src/channels/xhs.js';

describe('xhs normalizeContactId', () => {
  it('去除常见前缀并清理非法字符', () => {
    expect(normalizeContactId('Total-5a1f2b3c')).toBe('5a1f2b3c');
    expect(normalizeContactId('User_6182abcd')).toBe('6182abcd');
    expect(normalizeContactId('5a1f-2b3c')).toBe('5a1f-2b3c');
    expect(normalizeContactId(null)).toBe('');
    expect(normalizeContactId(undefined)).toBe('');
    expect(normalizeContactId('')).toBe('');
  });
});

describe('xhs getAccountId 兜底', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    try { localStorage.removeItem('hivebridge:account:xiaohongshu'); } catch (e) {  }
  });
  afterEach(() => { document.body.innerHTML = ''; });

  it('侧边栏「我的」主页链接 → 返回真实 id（非空串）', () => {
    document.body.innerHTML = `<aside><a href="https://www.xiaohongshu.com/user/profile/5f8a2b01">我的</a></aside>`;
    expect(getAccountId()).toBe('5f8a2b01');
  });

  it('header 无 profile 链接时，任意 /user/profile/ 链接兜底（非空串）', () => {
    document.body.innerHTML = `<header><a href="https://www.xiaohongshu.com/user/profile/abc123">头像</a></header>`;
    expect(getAccountId()).toBe('abc123');
  });

  it('仅 /user/（无 profile）链接时仍能兜底', () => {
    document.body.innerHTML = `<a href="https://www.xiaohongshu.com/user/xyz789">A</a>`;
    expect(getAccountId()).toBe('xyz789');
  });

  it('完全无链接时返回空串（DOM 兜底失败，后端用三元组命中）', () => {
    document.body.innerHTML = `<div class="empty">无账号线索</div>`;
    expect(getAccountId()).toBe('');
  });

  it('结构链接取不到真实 id 时，复用 localStorage 缓存', () => {
    localStorage.setItem('hivebridge:account:xiaohongshu', 'cachedUserId');
    document.body.innerHTML = `<div class="empty"></div>`;
    expect(getAccountId()).toBe('cachedUserId');
  });
});

describe('xhs getConversationId 兜底', () => {
  beforeEach(() => { document.body.innerHTML = ''; });

  it('URL 带 conversation_id 时优先读取', () => {
    const fakeLoc = {
      href: 'https://www.xiaohongshu.com/message?conversation_id=conv789',
      search: '?conversation_id=conv789',
      host: 'www.xiaohongshu.com',
      hostname: 'www.xiaohongshu.com',
      pathname: '/message',
    };
    vi.stubGlobal('location', fakeLoc);
    expect(getConversationId()).toBe('conv789');
    vi.unstubAllGlobals();
  });

  it('活动会话项（.active）的 data-key / data-id 兜底', () => {
    document.body.innerHTML = `
      <div class="sx-contact-item" data-key="Total-5a1f">路人a</div>
      <div class="sx-contact-item active" data-id="5f8a2b01" data-contactusemid="9c88d777">客服B</div>
    `;
    expect(getConversationId()).toBe('5f8a2b01');
  });

  it('无活动项且无 header 链接时返回 null（不误报会话）', () => {
    document.body.innerHTML = `<div class="aside"></div>`;
    expect(getConversationId()).toBeNull();
  });
});

describe('xhs isXhsMessagePage 结构识别', () => {
  beforeEach(() => { document.body.innerHTML = ''; });
  afterEach(() => { document.body.innerHTML = ''; });

  it('输入框 + 消息容器同时存在 → true', () => {
    document.body.innerHTML = `
      <textarea id="jarvis-reply-textarea"></textarea>
      <div class="im-chat-window"><div class="im-msg-item">你好</div></div>
    `;
    expect(isXhsMessagePage()).toBe(true);
  });

  it('无输入框 → false', () => {
    document.body.innerHTML = `<div class="im-chat-window"><div class="im-msg-item">你好</div></div>`;
    expect(isXhsMessagePage()).toBe(false);
  });

  it('仅会话列表无输入框 → false', () => {
    document.body.innerHTML = `<div class="sx-contact-item">张三</div>`;
    expect(isXhsMessagePage()).toBe(false);
  });
});