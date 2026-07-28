// dom.js 工具测试：findAnyMessageInput / looksLikeMessagePage / isLikelyVisible
//
// 覆盖目标：
//   1. findAnyMessageInput 命中各优先级候选（contenteditable / role=textbox / textarea / data-e2e / aria-label）
//   2. findAnyMessageInput 排除过小、不可见、非消息输入的元素
//   3. findAnyMessageInput 视口下半部分加分
//   4. looksLikeMessagePage 命中 URL 路径 / URL 参数 / DOM 线索
//   5. looksLikeMessagePage 不命中首页 / 帖子详情页
//
// 环境：vitest 默认 happy-dom 提供 window / document / getComputedStyle / innerHeight
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { findAnyMessageInput, looksLikeMessagePage, isLikelyVisible } from '../src/core/dom.js';

// ---- 工具：创建一个带尺寸 / 位置 / 可见性的假元素 ----
function makeEl({
  tag = 'div',
  attrs = {},
  rect = { x: 0, y: 0, width: 200, height: 40, top: 100, left: 0, right: 200, bottom: 140 },
  visible = true,
  position = 'static',
} = {}) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  // happy-dom 不一定实现 offsetParent；用 getComputedStyle stub
  Object.defineProperty(el, 'offsetParent', { configurable: true, get: () => (visible ? {} : null) });
  el.getBoundingClientRect = () => ({
    x: rect.x,
    y: rect.y,
    width: rect.width,
    height: rect.height,
    top: rect.top,
    left: rect.left,
    right: rect.right,
    bottom: rect.bottom,
  });
  // 用 inline style + spy on getComputedStyle
  if (!visible) {
    el.style.display = 'none';
  }
  if (position === 'fixed') {
    el.style.position = 'fixed';
  }
  return el;
}

beforeEach(() => {
  // 每次测试前清空 body
  document.body.innerHTML = '';
  // innerHeight 默认 768（happy-dom 行为）
  if (window.innerHeight === 0) {
    Object.defineProperty(window, 'innerHeight', { value: 800, configurable: true, writable: true });
  }
});

describe('dom isLikelyVisible', () => {
  it('null 元素返回 false', () => {
    expect(isLikelyVisible(null)).toBe(false);
  });

  it('正常尺寸元素返回 true', () => {
    const el = makeEl({ visible: true });
    document.body.appendChild(el);
    expect(isLikelyVisible(el)).toBe(true);
  });

  it('display:none 元素返回 false', () => {
    const el = makeEl({ visible: false });
    document.body.appendChild(el);
    expect(isLikelyVisible(el)).toBe(false);
  });
});

describe('dom findAnyMessageInput', () => {
  it('无任何候选时返回 null', () => {
    document.body.appendChild(makeEl({ tag: 'div', attrs: { class: 'random' } }));
    expect(findAnyMessageInput()).toBeNull();
  });

  it('命中 contenteditable=true', () => {
    const el = makeEl({ tag: 'div', attrs: { contenteditable: 'true' } });
    document.body.appendChild(el);
    expect(findAnyMessageInput()).toBe(el);
  });

  it('命中 contenteditable=""（空串等价于 true）', () => {
    const el = makeEl({ tag: 'div', attrs: { contenteditable: '' } });
    document.body.appendChild(el);
    expect(findAnyMessageInput()).toBe(el);
  });

  it('命中 [role="textbox"]', () => {
    const el = makeEl({ tag: 'div', attrs: { role: 'textbox' } });
    document.body.appendChild(el);
    expect(findAnyMessageInput()).toBe(el);
  });

  it('命中 textarea + placeholder 含"消息"', () => {
    const el = makeEl({ tag: 'textarea', attrs: { placeholder: '请输入消息' } });
    document.body.appendChild(el);
    expect(findAnyMessageInput()).toBe(el);
  });

  it('命中 textarea + aria-label 含 "message"', () => {
    const el = makeEl({ tag: 'textarea', attrs: { 'aria-label': 'Type a message' } });
    document.body.appendChild(el);
    expect(findAnyMessageInput()).toBe(el);
  });

  it('命中 data-e2e="message-input"', () => {
    const el = makeEl({ tag: 'div', attrs: { 'data-e2e': 'message-input' } });
    document.body.appendChild(el);
    expect(findAnyMessageInput()).toBe(el);
  });

  it('命中 data-testid="editor"', () => {
    const el = makeEl({ tag: 'div', attrs: { 'data-testid': 'message-editor' } });
    document.body.appendChild(el);
    expect(findAnyMessageInput()).toBe(el);
  });

  it('命中 aria-label 含"回复"', () => {
    const el = makeEl({ tag: 'div', attrs: { 'aria-label': '回复消息' } });
    document.body.appendChild(el);
    expect(findAnyMessageInput()).toBe(el);
  });

  it('排除尺寸过小的元素（< 20x10）', () => {
    const tiny = makeEl({ tag: 'div', attrs: { contenteditable: 'true' }, rect: { x: 0, y: 0, width: 10, height: 5, top: 100, left: 0, right: 10, bottom: 105 } });
    document.body.appendChild(tiny);
    expect(findAnyMessageInput()).toBeNull();
  });

  it('排除 textarea 宽度 < 60（评论框）', () => {
    const tiny = makeEl({ tag: 'textarea', attrs: { placeholder: '请输入消息' }, rect: { x: 0, y: 0, width: 50, height: 30, top: 100, left: 0, right: 50, bottom: 130 } });
    document.body.appendChild(tiny);
    expect(findAnyMessageInput()).toBeNull();
  });

  it('排除 placeholder 关键词不命中的 textarea（搜索框）', () => {
    const el = makeEl({ tag: 'textarea', attrs: { placeholder: '搜索' } });
    document.body.appendChild(el);
    expect(findAnyMessageInput()).toBeNull();
  });

  it('优先 contenteditable 高于 textarea', () => {
    const ce = makeEl({ tag: 'div', attrs: { contenteditable: 'true' } });
    const ta = makeEl({ tag: 'textarea', attrs: { placeholder: '消息' } });
    document.body.appendChild(ta);
    document.body.appendChild(ce);
    expect(findAnyMessageInput()).toBe(ce);
  });

  it('视口下半部分的输入框优先于上半部分', () => {
    const top = makeEl({ tag: 'div', attrs: { contenteditable: 'true' }, rect: { x: 0, y: 0, width: 200, height: 40, top: 50, left: 0, right: 200, bottom: 90 } });
    const bottom = makeEl({ tag: 'div', attrs: { contenteditable: 'true' }, rect: { x: 0, y: 0, width: 200, height: 40, top: 600, left: 0, right: 200, bottom: 640 } });
    document.body.appendChild(top);
    document.body.appendChild(bottom);
    // 视口 800px，bottom 在 40% (320) 之后应加分
    expect(findAnyMessageInput()).toBe(bottom);
  });

  it('视口 innerHeight 为 0 时不报错', () => {
    Object.defineProperty(window, 'innerHeight', { value: 0, configurable: true, writable: true });
    const el = makeEl({ tag: 'div', attrs: { contenteditable: 'true' } });
    document.body.appendChild(el);
    expect(() => findAnyMessageInput()).not.toThrow();
    // 兜底 vh=800，el 在 100px < 320 → 不加分，但仍能命中
    expect(findAnyMessageInput()).toBe(el);
  });
});

describe('dom looksLikeMessagePage', () => {
  function setUrl(href) {
    Object.defineProperty(window, 'location', {
      value: { ...window.location, href },
      writable: true,
      configurable: true,
    });
  }

  it('默认 location 不命中', () => {
    setUrl('https://www.douyin.com/');
    expect(looksLikeMessagePage()).toBe(false);
  });

  it('命中 /message/ 路径', () => {
    setUrl('https://www.douyin.com/message/detail/');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('命中 /messages/@user 路径（TikTok 私信页）', () => {
    setUrl('https://www.tiktok.com/messages/@someuser');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('命中 /im 路径（小红书 IM）', () => {
    setUrl('https://www.xiaohongshu.com/im/chat');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('命中 URL 查询 conversation_id', () => {
    setUrl('https://www.douyin.com/?conversation_id=123');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('命中 URL 查询 user_id', () => {
    setUrl('https://www.xiaohongshu.com/explore?user_id=abc');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('命中 /chat/ 路径', () => {
    setUrl('https://example.com/chat/room/1');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('命中 DOM 含 chat- 类的元素', () => {
    setUrl('https://www.douyin.com/');
    const el = makeEl({ tag: 'div', attrs: { class: 'chat-container' } });
    document.body.appendChild(el);
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('命中 DOM 含 data-e2e=chat-xxx', () => {
    setUrl('https://www.tiktok.com/');
    const el = makeEl({ tag: 'div', attrs: { 'data-e2e': 'chat-list' } });
    document.body.appendChild(el);
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('首页/feed URL + 干净 DOM 不命中', () => {
    setUrl('https://www.douyin.com/');
    document.body.appendChild(makeEl({ tag: 'div', attrs: { class: 'video-card' } }));
    expect(looksLikeMessagePage()).toBe(false);
  });
});
