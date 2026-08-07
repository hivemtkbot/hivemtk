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
import { findAnyMessageInput, looksLikeMessagePage, isLikelyVisible, sanitizePeerName } from '../src/core/dom.js';

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
    // 注意：/explore 在 URL 黑名单中，user_id 命中要选非反例 URL
    setUrl('https://www.douyin.com/?user_id=abc');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('命中 /chat/ 路径', () => {
    setUrl('https://example.com/chat/room/1');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('命中 DOM：双特征（chat-window + message-list）同时存在', () => {
    // 新逻辑要求多特征同时存在，避免单点误判
    setUrl('https://www.douyin.com/');
    document.body.appendChild(makeEl({ tag: 'div', attrs: { class: 'chat-window' } }));
    document.body.appendChild(makeEl({ tag: 'div', attrs: { class: 'message-list' } }));
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('命中 DOM：≥2 个 data-e2e="chat-item"', () => {
    setUrl('https://www.tiktok.com/');
    document.body.appendChild(makeEl({ tag: 'div', attrs: { 'data-e2e': 'chat-item' } }));
    document.body.appendChild(makeEl({ tag: 'div', attrs: { 'data-e2e': 'chat-item' } }));
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('首页/feed URL + 干净 DOM 不命中', () => {
    setUrl('https://www.douyin.com/');
    document.body.appendChild(makeEl({ tag: 'div', attrs: { class: 'video-card' } }));
    expect(looksLikeMessagePage()).toBe(false);
  });
});

// =============================================================
// 误判回归测试：jingxuan / 视频评论 / 个人主页 等非私信页
// =============================================================
describe('looksLikeMessagePage 误判回归（jingxuan / 视频详情 / 个人主页）', () => {
  function setUrl(href) {
    Object.defineProperty(window, 'location', {
      value: { ...window.location, href },
      writable: true,
      configurable: true,
    });
  }

  // 模拟抖音精选页 DOM：含 messageEditorinputArea（评论框）+ conversationConversation*（推荐列表）
  function mockJingxuanDOM() {
    // 评论输入框外层：editor-kit-container（继承排除命中）
    const wrap = makeEl({ tag: 'div', attrs: { class: 'editor-kit-container' } });
    const input = makeEl({ tag: 'div', attrs: { class: 'messageEditorinputArea', contenteditable: 'true' } });
    const send = makeEl({ tag: 'svg', attrs: { class: 'messageMsgInputpublishBtn' } });
    wrap.appendChild(input);
    wrap.appendChild(send);
    document.body.appendChild(wrap);
    // 推荐列表（jingxuan 侧边栏）
    const listWrap = makeEl({ tag: 'div', attrs: { class: 'conversationConversationListwrapper' } });
    const item = makeEl({ tag: 'div', attrs: { class: 'conversationConversationItemwrapper', 'data-e2e': 'conversation-item' } });
    listWrap.appendChild(item);
    document.body.appendChild(listWrap);
  }

  it('jingxuan 页面：URL 黑名单优先于 DOM 启发式 → 返回 false', () => {
    setUrl('https://www.douyin.com/jingxuan');
    mockJingxuanDOM();
    expect(looksLikeMessagePage()).toBe(false);
  });

  it('discover / explore 页面：URL 黑名单 → false', () => {
    setUrl('https://www.douyin.com/discover');
    document.body.appendChild(makeEl({ tag: 'div', attrs: { class: 'messageContainer' } }));
    expect(looksLikeMessagePage()).toBe(false);

    setUrl('https://www.xiaohongshu.com/explore');
    expect(looksLikeMessagePage()).toBe(false);
  });

  it('search 页面：URL 黑名单 → false', () => {
    setUrl('https://www.douyin.com/search/something');
    expect(looksLikeMessagePage()).toBe(false);
  });

  it('个人主页：URL 黑名单 → false', () => {
    setUrl('https://www.douyin.com/user/MS4wLjABAAAAxx');
    expect(looksLikeMessagePage()).toBe(false);
  });

  it('视频详情页：URL 黑名单 → false', () => {
    setUrl('https://www.douyin.com/video/7123456789');
    expect(looksLikeMessagePage()).toBe(false);
  });

  it('小红书笔记详情：URL 黑名单 → false', () => {
    setUrl('https://www.xiaohongshu.com/explore/note/abc123');
    expect(looksLikeMessagePage()).toBe(false);
  });

  it('抖音 /im/chat/ 正向 URL 仍命中', () => {
    setUrl('https://www.douyin.com/im/chat/123/');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('小红书 /im 正向 URL 仍命中', () => {
    setUrl('https://www.xiaohongshu.com/im/');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('TikTok /messages/@user 正向 URL 仍命中', () => {
    setUrl('https://www.tiktok.com/messages/@someuser');
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('URL 不对 + 仅有单个 conversationList class → 仍不命中（要求多特征）', () => {
    setUrl('https://www.douyin.com/');
    document.body.appendChild(makeEl({ tag: 'div', attrs: { class: 'conversationList' } }));
    expect(looksLikeMessagePage()).toBe(false);
  });

  it('URL 不对 + 仅有单个 chatWindow class → 仍不命中', () => {
    setUrl('https://www.douyin.com/');
    document.body.appendChild(makeEl({ tag: 'div', attrs: { class: 'chatWindow' } }));
    expect(looksLikeMessagePage()).toBe(false);
  });

  it('URL 不对 + messageList + chatWindow 同时存在 → 命中（多特征投票通过）', () => {
    setUrl('https://www.douyin.com/');
    document.body.appendChild(makeEl({ tag: 'div', attrs: { class: 'messageList' } }));
    document.body.appendChild(makeEl({ tag: 'div', attrs: { class: 'chatWindow' } }));
    expect(looksLikeMessagePage()).toBe(true);
  });

  it('URL 不对 + ≥2 个 data-e2e="chat-item" → 命中（强特征）', () => {
    setUrl('https://www.douyin.com/');
    document.body.appendChild(makeEl({ tag: 'div', attrs: { 'data-e2e': 'chat-item' } }));
    document.body.appendChild(makeEl({ tag: 'div', attrs: { 'data-e2e': 'chat-item' } }));
    expect(looksLikeMessagePage()).toBe(true);
  });
});

describe('findAnyMessageInput 误判回归（评论框 / 搜索框）', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  function setUrl(href) {
    Object.defineProperty(window, 'location', {
      value: { ...window.location, href },
      writable: true,
      configurable: true,
    });
  }

  it('抖音精选：editor-kit-container > messageEditorinputArea → 父链排除命中', () => {
    setUrl('https://www.douyin.com/jingxuan');
    const wrap = document.createElement('div');
    wrap.className = 'editor-kit-container';
    const ce = document.createElement('div');
    ce.setAttribute('contenteditable', 'true');
    ce.className = 'messageEditorinputArea';
    ce.getBoundingClientRect = () => ({ x: 0, y: 100, width: 300, height: 50, top: 100, left: 0, right: 300, bottom: 150 });
    Object.defineProperty(ce, 'offsetParent', { configurable: true, get: () => ({}) });
    wrap.appendChild(ce);
    document.body.appendChild(wrap);
    // 同时放一个干净的 contenteditable 在 chatWindow 上下文
    const chatWrap = document.createElement('div');
    chatWrap.className = 'chatWindow';
    const good = document.createElement('div');
    good.setAttribute('contenteditable', 'true');
    good.className = 'messageInput';
    good.getBoundingClientRect = () => ({ x: 0, y: 100, width: 300, height: 50, top: 100, left: 0, right: 300, bottom: 150 });
    Object.defineProperty(good, 'offsetParent', { configurable: true, get: () => ({}) });
    chatWrap.appendChild(good);
    document.body.appendChild(chatWrap);
    // 应该命中 chatWindow 里的，不命中 editor-kit 里的
    expect(findAnyMessageInput()).toBe(good);
  });

  it('commentEditor className → 排除', () => {
    const ce = document.createElement('div');
    ce.setAttribute('contenteditable', 'true');
    ce.className = 'commentEditor';
    ce.getBoundingClientRect = () => ({ x: 0, y: 100, width: 300, height: 50, top: 100, left: 0, right: 300, bottom: 150 });
    Object.defineProperty(ce, 'offsetParent', { configurable: true, get: () => ({}) });
    document.body.appendChild(ce);
    expect(findAnyMessageInput()).toBeNull();
  });

  it('父链有 commentContainer → 排除', () => {
    const wrap = document.createElement('div');
    wrap.className = 'commentContainer';
    const ce = document.createElement('div');
    ce.setAttribute('contenteditable', 'true');
    ce.getBoundingClientRect = () => ({ x: 0, y: 100, width: 300, height: 50, top: 100, left: 0, right: 300, bottom: 150 });
    Object.defineProperty(ce, 'offsetParent', { configurable: true, get: () => ({}) });
    wrap.appendChild(ce);
    document.body.appendChild(wrap);
    expect(findAnyMessageInput()).toBeNull();
  });

  it('父链 searchInput → 排除', () => {
    const wrap = document.createElement('div');
    wrap.className = 'headerSearchInput';
    const ce = document.createElement('div');
    ce.setAttribute('contenteditable', 'true');
    ce.getBoundingClientRect = () => ({ x: 0, y: 100, width: 300, height: 50, top: 100, left: 0, right: 300, bottom: 150 });
    Object.defineProperty(ce, 'offsetParent', { configurable: true, get: () => ({}) });
    wrap.appendChild(ce);
    document.body.appendChild(wrap);
    expect(findAnyMessageInput()).toBeNull();
  });

  it('无 comment/editor-kit 父链的干净 contenteditable → 命中', () => {
    const ce = document.createElement('div');
    ce.setAttribute('contenteditable', 'true');
    ce.className = 'messageInput';
    ce.getBoundingClientRect = () => ({ x: 0, y: 100, width: 300, height: 50, top: 100, left: 0, right: 300, bottom: 150 });
    Object.defineProperty(ce, 'offsetParent', { configurable: true, get: () => ({}) });
    document.body.appendChild(ce);
    expect(findAnyMessageInput()).toBe(ce);
  });
});

// 2026-08-07 修复（用户诉求③）：fillContentEditable 新增 clearBefore 选项，
//   下发场景必须先清空输入框旧内容再写入新内容。
import { fillContentEditable } from '../src/core/dom.js';

describe('fillContentEditable / clearBefore 下发场景先清空', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    // happy-dom 不一定实现 execCommand，mock 之避免抛错
    if (typeof document.execCommand !== 'function') {
      document.execCommand = () => true;
    }
  });

  it('contenteditable 已存在旧文本 + clearBefore:true → 旧内容清空、最终内容仅新文本', () => {
    const ce = document.createElement('div');
    ce.setAttribute('contenteditable', 'true');
    ce.innerText = '用户正在打的字';
    document.body.appendChild(ce);

    fillContentEditable(ce, 'AI 回复', { clearBefore: true });

    // 旧内容必须清空（不应出现"用户正在打的字AI 回复"或"AI 回复用户正在打的字"）
    expect(ce.innerText).toBe('AI 回复');
    expect(ce.innerText).not.toContain('用户正在打的字');
  });

  it('contenteditable 空 + clearBefore:true → 正常填入', () => {
    const ce = document.createElement('div');
    ce.setAttribute('contenteditable', 'true');
    ce.innerText = '';
    document.body.appendChild(ce);

    fillContentEditable(ce, 'AI 回复', { clearBefore: true });
    expect(ce.innerText).toBe('AI 回复');
  });

  it('contenteditable + clearBefore:false（默认）→ 保留历史行为：append 在旧文本后（兼容历史回填场景）', () => {
    const ce = document.createElement('div');
    ce.setAttribute('contenteditable', 'true');
    ce.innerText = '旧文本';
    document.body.appendChild(ce);

    fillContentEditable(ce, '新文本');
    // 兼容历史行为：不清空 → 新内容追加
    expect(ce.innerText).toContain('新文本');
  });

  it('textarea 已存在旧文本 + clearBefore:true → 旧内容清空、最终内容仅新文本', () => {
    const ta = document.createElement('textarea');
    ta.value = '用户正在打的字';
    document.body.appendChild(ta);

    fillContentEditable(ta, 'AI 回复', { clearBefore: true });

    expect(ta.value).toBe('AI 回复');
    expect(ta.value).not.toContain('用户正在打的字');
  });
});

// =============================================================
// sanitizePeerName：会话列表项文本净化为稳定昵称
// 2026-08-07 第十一轮修复：兜底用 cleanText(activeItem) 派生 conversation_id 时，
// 元素文本常含时间戳/状态徽章/相对时间等易变文本，导致同会话不同时刻 conversation_id 不同 →
// outbound 永远找不到目标会话 → 大量 pending 永久堆积（实测 287 条）。
// 用例数据全部来自 DB 真实脏数据（douyin + xianyu pending 列表）。
// =============================================================
describe('sanitizePeerName 剥离易变后缀（第十一轮修复回归）', () => {
  // 直接对文本调用，模拟 cleanText(activeItem) 的输出
  const cases = [
    // [raw, expected, 来源备注]
    // —— 抖音：纯时间后缀 ——
    ['好吃嘴辰辰 12:31', '好吃嘴辰辰', 'douyin pending 35 条'],
    ['好吃嘴辰辰 06:48', '好吃嘴辰辰', 'douyin pending 9 条'],
    ['jack 09:08', 'jack', 'douyin pending 6 条'],
    ['AI 造价实验室 08:28', 'AI 造价实验室', 'douyin pending 7 条'],
    ['AI 证据链管理 13:15', 'AI 证据链管理', 'douyin pending 4 条'],
    ['防止被割韭菜1群 02:32', '防止被割韭菜1群', 'douyin pending 3 条'],
    ['AI 修炼场 5 01:01', 'AI 修炼场 5', 'douyin pending 2 条'],
    // —— 抖音：相对时间后缀 ——
    ['好吃嘴辰辰 刚刚', '好吃嘴辰辰', 'douyin pending 21 条'],
    ['好吃嘴辰辰 18分钟前', '好吃嘴辰辰', 'douyin pending 18 条'],
    ['好吃嘴辰辰 20分钟前', '好吃嘴辰辰', 'douyin pending 16 条'],
    ['猎洞时刻网络安全群 56分钟前', '猎洞时刻网络安全群', 'douyin pending 14 条'],
    ['AI 造价实验室 41分钟前', 'AI 造价实验室', 'douyin pending 3 条'],
    ['AI 证据链管理 57分钟前', 'AI 证据链管理', 'douyin pending 2 条'],
    ['AI 修炼场 5 刚刚', 'AI 修炼场 5', 'douyin pending 3 条'],
    // —— 抖音：昨天 + HH:MM ——
    ['AI 修炼场 5 昨天 18:20', 'AI 修炼场 5', 'douyin pending 31 条'],
    ['猎洞时刻网络安全群 昨天 22:08', '猎洞时刻网络安全群', 'douyin pending 14 条'],
    ['四川-宝宝巴士2️⃣ 昨天 18:21', '四川-宝宝巴士2️⃣', 'douyin pending 4 条'],
    // —— 抖音：完整日期 ——
    ['kfcfourv40 2025/10/29', 'kfcfourv40', 'douyin pending 4 条'],
    ['AIGC小年 智能体2裙 2025/08/30', 'AIGC小年 智能体2裙', 'douyin pending 4 条'],
    ['广东巨龙(龙华)律师事务所官方号 2025/08/09', '广东巨龙(龙华)律师事务所官方号', 'douyin pending 4 条'],
    ['成都嘉贝乐游乐设备 2025/09/19', '成都嘉贝乐游乐设备', 'douyin pending 3 条'],
    // —— 闲鱼：订单状态徽章 ——
    ['淘淘达人软件商城 交易成功', '淘淘达人软件商城', 'xianyu pending 11 条'],
    ['该用户已注销 有新交易评价', '该用户已注销', 'xianyu pending 6 条'],
    ['小鱼票票龙 有新交易评价', '小鱼票票龙', 'xianyu pending 4 条'],
    ['x***1 交易成功', 'x***1', 'xianyu pending 2 条'],
    ['交易成功', '', 'xianyu pending 1 条（纯状态文本应返回空，调用方放弃派生）'],
    // —— 多层后缀（同会话不同时刻叠加） ——
    ['好吃嘴辰辰 昨天 18:20', '好吃嘴辰辰', '多层后缀：先剥 HH:MM 再剥 昨天'],
    // —— 纯昵称（无后缀，不应误删） ——
    ['炫e小枫', '炫e小枫', 'xianyu pending 8 条，无后缀'],
    ['C0123', 'C0123', 'xianyu pending 5 条，无后缀'],
    ['专业电脑手机数码与电器', '专业电脑手机数码与电器', 'xianyu pending 4 条，无后缀'],
    ['陛下的心旅游小店', '陛下的心旅游小店', 'xianyu pending 1 条，无后缀'],
    // —— 边界：空/纯空白/纯时间 ——
    ['', '', '空输入'],
    [null, '', 'null 输入'],
    [undefined, '', 'undefined 输入'],
    ['   ', '', '纯空白'],
    ['12:31', '', '纯时间应返回空（避免派生 conv:12:31 这种垃圾 id）'],
    ['刚刚', '', '纯相对时间应返回空'],
    ['昨天 18:20', '', '纯昨天+时间应返回空'],
    ['交易成功', '', '纯订单状态应返回空'],
    // —— 未读数字标记 [N] ——
    ['钓点王 [3]', '钓点王', '未读数字标记'],
  ];

  for (const [raw, expected, note] of cases) {
    it(`sanitizePeerName(${JSON.stringify(raw)}) === ${JSON.stringify(expected)}  // ${note}`, () => {
      expect(sanitizePeerName(raw)).toBe(expected);
    });
  }

  it('多轮迭代后稳定（不会无限循环）', () => {
    // 极端多层后缀
    const raw = '某群 昨天 18:20';
    const result = sanitizePeerName(raw);
    expect(result).toBe('某群');
    // 再次调用结果不变（幂等）
    expect(sanitizePeerName(result)).toBe('某群');
  });

  it('不破坏纯数字/字母昵称（不带后缀）', () => {
    expect(sanitizePeerName('12345')).toBe('12345');
    expect(sanitizePeerName('user_2025')).toBe('user_2025');
  });
});
