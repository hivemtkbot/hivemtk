// DOM 交互工具：逐行移植自三个开源仓库的真实交互逻辑。
// - simulateRealClick / humanType：来自 DY-auto（抖音）
// - enhancedClickWithVerification / setValue：来自 XHS-YYDS（小红书）
// - simulateTyping / simulateEnterKey / simulateRealClick：来自 tiktok-auto-plugin
import { createLogger } from './logger.js';

export { createLogger };

const log = createLogger('dom', 'bridge');

export const qs = (sel, root = document) => root.querySelector(sel);
export const qsa = (sel, root = document) => Array.from(root.querySelectorAll(sel));

/** 等待某个 selector 出现（抖音自动回复脚本用的 waitForElement 同款思路） */
export function waitFor(selector, { timeout = 8000, root = document } = {}) {
  return new Promise((resolve) => {
    const el = qs(selector, root);
    if (el) return resolve(el);
    const start = Date.now();
    const timer = setInterval(() => {
      const found = qs(selector, root);
      if (found || Date.now() - start > timeout) {
        clearInterval(timer);
        resolve(found || null);
      }
    }, 200);
  });
}

/** 真实点击：合成完整 Pointer + Mouse 事件序列（DY-auto / tiktok 同款，绕过点击拦截） */
export function simulateRealClick(element) {
  if (!element) return;
  const rect = element.getBoundingClientRect();
  const x = rect.left + rect.width / 2;
  const y = rect.top + rect.height / 2;
  const opts = (type) => ({
    view: window,
    bubbles: true,
    cancelable: true,
    clientX: x,
    clientY: y,
    screenX: x,
    screenY: y,
    button: 0,
    buttons: 1,
    composed: true,
    pointerId: 1,
    pointerType: 'mouse',
    isPrimary: true,
  });
  element.dispatchEvent(new PointerEvent('pointerdown', opts('pointerdown')));
  element.dispatchEvent(new MouseEvent('mousedown', opts('mousedown')));
  element.dispatchEvent(new PointerEvent('pointerup', opts('pointerup')));
  element.dispatchEvent(new MouseEvent('mouseup', opts('mouseup')));
  element.dispatchEvent(new MouseEvent('click', opts('click')));
}

/**
 * 小红书同款点击：先原生 click，再补一层 mousedown/up/click 事件。
 * 用于 .send_btn 这类需要真实事件触发的按钮。
 */
export function enhancedClick(element) {
  if (!element) return;
  try {
    element.click();
  } catch (e) {
    log.debug('native click failed, fallback', e);
  }
  const ev = (type) =>
    new MouseEvent(type, { bubbles: true, cancelable: true, view: window });
  element.dispatchEvent(ev('mousedown'));
  element.dispatchEvent(ev('mouseup'));
  element.dispatchEvent(ev('click'));
}

/** 小红书 setValue：textarea 设值 + 触发 input（domUtils.js 同款） */
export function setValue(el, value) {
  if (!el) return;
  const proto = el.tagName === 'TEXTAREA' ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
  if (setter) setter.call(el, value);
  else el.value = value;
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
}

/**
 * 抖音 fillInputViaPaste：contenteditable 填值（粘贴事件优先，保证框架识别输入）。
 * 参考 DY-auto fillInputViaPaste：触发 paste + 写入 innerText + 派发 input。
 *
 * 2026-08-07 修复（用户诉求③）：增加 clearBefore 选项，下发场景必须先清空输入框旧内容，
 *   避免「用户正在打字 + extension 同时下发」导致旧内容+新内容拼接发出。
 *   - clearBefore: true  → 先清空（innerText=''/value=''）再 insertText（默认 false 兼容历史行为）
 *   - 历史回填场景保持原行为（不清空，仅 append）
 */
export function fillContentEditable(el, text, { clearBefore = false } = {}) {
  if (!el) return;
  el.focus();
  if (clearBefore) {
    // 1) 清空现有内容（DOM 层面）
    try {
      if (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') {
        el.value = '';
      } else {
        // contenteditable：清空所有子节点，再保留或建立一个 <br> 占位（否则某些平台会认为输入框为空不可发）
        while (el.firstChild) el.removeChild(el.firstChild);
        el.dispatchEvent(new InputEvent('input', { bubbles: true }));
      }
    } catch (_) { /* noop */ }
  }
  // 1) 原生 execCommand 输入（最贴近真人，抖音/小红书 contenteditable 均认）
  try {
    document.execCommand('insertText', false, text);
  } catch (e) {
    /* noop */
  }
  // 2) 兜底：直接写 innerText + 派发 input 事件
  if ((el.innerText || el.value || '').toString().trim() !== text.trim()) {
    if (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') {
      el.value = text;
    } else {
      el.innerText = text;
    }
    el.dispatchEvent(new InputEvent('input', { bubbles: true }));
    el.dispatchEvent(new Event('input', { bubbles: true }));
  }
}

/** 抖音 humanType：逐字符 execCommand('insertText')（更拟人，适合作者打字节奏） */
export function humanType(el, text, { interval = 20 } = {}) {
  return new Promise((resolve) => {
    if (!el) return resolve();
    el.focus();
    let i = 0;
    const tick = () => {
      if (i >= text.length) return resolve();
      try {
        document.execCommand('insertText', false, text[i]);
      } catch (e) {
        el.innerText = (el.innerText || '') + text[i];
      }
      i += 1;
      setTimeout(tick, interval);
    };
    tick();
  });
}

/**
 * TikTok simulateTyping：先聚焦，再逐字符写值并派发 input（适配 Draft.js 受控输入）。
 * 对 textarea / contenteditable 均兼容。
 */
export function simulateTyping(el, text, { interval = 25 } = {}) {
  return new Promise((resolve) => {
    if (!el) return resolve();
    el.focus();
    let acc = '';
    let i = 0;
    const tick = () => {
      if (i >= text.length) {
        el.dispatchEvent(new Event('input', { bubbles: true }));
        return resolve();
      }
      acc += text[i];
      if (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') {
        const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value')?.set;
        if (setter) setter.call(el, acc);
        else el.value = acc;
      } else {
        try {
          document.execCommand('insertText', false, text[i]);
        } catch (e) {
          el.innerText = acc;
        }
      }
      el.dispatchEvent(new InputEvent('input', { bubbles: true }));
      i += 1;
      setTimeout(tick, interval);
    };
    tick();
  });
}

/** TikTok simulateEnterKey：在输入框派发回车（Draft.js 靠 Enter 发送） */
export function simulateEnterKey(el) {
  if (!el) return;
  const ev = (type) =>
    new KeyboardEvent(type, { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true, cancelable: true });
  el.dispatchEvent(ev('keydown'));
  el.dispatchEvent(ev('keypress'));
  el.dispatchEvent(ev('keyup'));
}

/** 取纯文本并压缩多余空白 */
export function cleanText(el) {
  if (!el) return '';
  return (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim();
}

/** 包裹 MutationObserver 的便捷 API */
export function observe(root, cb, options = { childList: true, subtree: true }) {
  if (!root) return null;
  const obs = new MutationObserver(cb);
  obs.observe(root, options);
  return obs;
}

// =============================================================
// 平台改版 / 严格选择器失效时的通用容错（findAnyMessageInput / looksLikeMessagePage）
// 场景：抖音/小红书/TikTok 偶尔会改 className / data-e2e / role，导致
//       channels/*.js 里硬编码的 SEL.EDITOR / SEL.INPUT 失效，桥接整体瘫掉。
// 解决：match() 失败时先扫描 DOM 看是否有「看起来像消息输入框」的元素，
//       配合「页面像是私信/消息页」的启发式判断，给出降级匹配。
//       这样能避免「打开私信页但 adapter.match() 永远返回 false」的全量错误。
// =============================================================

/** 元素是否在视觉上可见（offsetParent + 尺寸 + 非 display:none） */
export function isLikelyVisible(el) {
  if (!el) return false;
  // offsetParent === null 时表示 display:none（但 fixed 元素例外）
  if (el.offsetParent === null && getComputedStyle(el).position !== 'fixed') return false;
  const rect = el.getBoundingClientRect();
  if (rect.width === 0 || rect.height === 0) return false;
  return true;
}

/**
 * 通用回退：扫描页面上所有「可能是消息输入框」的元素并按可信度打分。
 * 命中条件（任一即可）：
 *   1) contenteditable="true" / ""  且 尺寸 ≥ 20×10（聊天输入框通常不小于此）
 *   2) [role="textbox"]  且 尺寸 ≥ 20×10
 *   3) textarea 且 placeholder / aria-label 含「消息|留言|回复|reply|message|input|comment|chat」
 *   4) [data-e2e*="message-input" / "input" / "editor" / "chat-input"]
 *   5) [data-testid*="message" / "input" / "editor"]
 *   6) [aria-label*="消息" / "留言" / "回复" / "Send a message" / "Type a message"]
 * 排除条件（任何一条命中即跳过）：
 *   - className 含 comment / Comment / commentEditor （评论框）
 *   - className 含 editor-kit / editorContainer （视频评论编辑器）
 *   - className 含 search / Search （搜索框）
 *   - 父链上有 className 含 comment / editor-kit / video-comment （继承排除）
 * 启发式排序：
 *   - 优先 contenteditable / role=textbox
 *   - 在视口下半部分（top > 40% 视口高度）的元素更像是聊天输入框
 *   - 尺寸过小（评论框、搜索框）不计入
 * @returns {Element|null} 最佳候选元素；无候选时返回 null
 */
export function findAnyMessageInput() {
  const all = (sel) => {
    try { return Array.from(document.querySelectorAll(sel)); }
    catch (_) { return []; }
  };
  const candidates = [];
  const vh = window.innerHeight || 800;

  // 反例关键词（评论框 / 视频评论 / 搜索框）
  // jingxuan 页面就是误中：messageEditorinputArea 在 editor-kit-container 内
  const EXCLUDE_KEYWORDS = /comment|Comment|editor-kit|editorContainer|searchInput|searchBar|searchBox|SearchInput/i;

  /** 元素或其父链是否含反例 class（继承排除：editor-kit-container 套着 messageEditorinputArea） */
  const hasExcludedAncestor = (el) => {
    let cur = el;
    let depth = 0;
    while (cur && depth < 6) {
      const cls = (cur.className && typeof cur.className === 'string') ? cur.className : '';
      if (EXCLUDE_KEYWORDS.test(cls)) return true;
      cur = cur.parentElement;
      depth += 1;
    }
    return false;
  };

  const score = (el) => {
    if (!isLikelyVisible(el)) return -1;
    if (hasExcludedAncestor(el)) return -1; // 评论/搜索框继承排除
    const r = el.getBoundingClientRect();
    if (r.width < 20 || r.height < 10) return -1;
    // 排除 textarea 多行但特别小（评论框、用户名框通常宽 < 100）
    if (el.tagName === 'TEXTAREA' && r.width < 60) return -1;
    // 在视口下半部分加分
    const lowerHalfBonus = r.top > vh * 0.4 ? 2 : 0;
    return lowerHalfBonus;
  };

  const push = (el, base) => {
    const s = score(el);
    if (s < 0) return;
    candidates.push({ el, score: base + s });
  };

  // 1) contenteditable=true / ''  （最高优先级）
  for (const el of all('div[contenteditable="true"], div[contenteditable=""], [contenteditable="true"], [contenteditable=""]')) {
    push(el, 10);
  }
  // 2) role=textbox
  for (const el of all('[role="textbox"]')) {
    push(el, 8);
  }
  // 3) textarea + placeholder/aria-label 命中关键词
  for (const el of all('textarea')) {
    const ph = (el.getAttribute('placeholder') || '').toLowerCase();
    const aria = (el.getAttribute('aria-label') || '').toLowerCase();
    if (/消息|留言|回复|reply|message|input|comment|chat|say|send|content/.test(ph + ' ' + aria)) {
      push(el, 7);
    }
  }
  // 4) data-e2e
  for (const sel of [
    '[data-e2e*="message-input"]',
    '[data-e2e*="chat-input"]',
    '[data-e2e*="input"]',
    '[data-e2e*="editor"]',
  ]) {
    for (const el of all(sel)) push(el, 6);
  }
  // 5) data-testid
  for (const sel of [
    '[data-testid*="message"]',
    '[data-testid*="input"]',
    '[data-testid*="editor"]',
  ]) {
    for (const el of all(sel)) push(el, 5);
  }
  // 6) aria-label 中文/英文
  for (const el of all('[aria-label]')) {
    const aria = (el.getAttribute('aria-label') || '');
    if (/消息|留言|回复|send a message|type a message|new message/.test(aria.toLowerCase() + ' ' + aria)) {
      push(el, 4);
    }
  }

  if (!candidates.length) return null;
  // 按分数降序，相同分数时尺寸大的优先
  candidates.sort((a, b) => {
    if (b.score !== a.score) return b.score - a.score;
    const ar = a.el.getBoundingClientRect();
    const br = b.el.getBoundingClientRect();
    return (br.width * br.height) - (ar.width * ar.height);
  });
  return candidates[0].el;
}

/**
 * 页面上下文是否看起来像「私信/消息/聊天」页。
 *
 * 判定策略（按可靠性排序）：
 *   1) URL 必须包含私信/IM 关键词：message / chat / im / inbox / direct / dm / private / msg
 *   2) URL 不能包含反例关键词：jingxuan / discover / explore / search / hot / follow / feed / recommend
 *   3) DOM 启发式只作为辅助，需「消息列表」+「会话容器」类名同时存在（多特征投票）
 *      单个 class 命中不算（避免 jingxuan 页面有 conversationConversation* 元素就误判）
 *
 * 设计原则：宁可漏判让 match() 返回 false 由用户报告，也不要误判让桥接在评论区乱跑。
 *
 * @returns {boolean}
 */
export function looksLikeMessagePage() {
  try {
    const url = (location.href || '').toLowerCase();

    // ---- 1. URL 反例黑名单（首页/feed/精选/搜索/个人主页/帖子详情 等） ----
    // 即便 URL 包含聊天关键词，若同时包含反例词，仍按反例处理
    const EXCLUDE_URL_PATTERNS = [
      /\/jingxuan\b/i,           // 抖音精选 https://www.douyin.com/jingxuan
      /\/discover\b/i,           // 抖音/小红书 发现页
      /\/explore\b/i,            // 小红书 explore
      /\/search\b/i,             // 搜索结果页
      /\/hot\b/i,                // 热门榜
      /\/follow\b/i,             // 关注列表（小红书 follow 不是 IM）
      /\/recommend\b/i,          // 推荐流
      /\/feed\b/i,               // 信息流
      /\/trending\b/i,           // 趋势
      /\/user\/[^/?#]+/,         // 个人主页 https://www.douyin.com/user/xxx（兼容 ?query/#hash）
      /\/video\/\d+/,            // 单个视频详情页 /video/7123456789
      /\/note\//,                // 小红书笔记详情 /note/xxx
      /\/explore\?/,             // 显式搜索 explore query
    ];
    for (const re of EXCLUDE_URL_PATTERNS) {
      if (re.test(url)) return false;
    }

    // ---- 2. URL 正向匹配（最权威） ----
    if (/\/(messages?|chats?|msg|direct|im|inbox|conversation|private|dm)(\/|\?|$|#)/.test(url)) return true;
    if (/[?&](conversation_id|message_id|session_id|chat_id|user_id)=/.test(url)) return true;
    if (/\/messages\/@?[\w.]+/.test(url)) return true;            // TikTok /messages/@user
    if (/\/im\b/.test(url)) return true;                          // 小红书 /im
    if (/\/im\/chat\b/.test(url)) return true;                    // 抖音 /im/chat

    // ---- 3. DOM 启发式（URL 不匹配时的兜底，必须多特征同时命中） ----
    // 要求「消息列表根」+「会话容器」类名同时存在
    // 单凭 message- / conversation 这种宽泛匹配会误中 jingxuan 推荐列表
    const hasMessageList = !!document.querySelector(
      '[class*="MessageList"], [class*="message-list"], [class*="messageList"], ' +
      '[class*="MessageContainer"], [class*="message-container"]'
    );
    const hasChatContainer = !!document.querySelector(
      '[class*="ChatWindow"], [class*="chat-window"], [class*="chatWindow"], ' +
      '[class*="ImChat"], [class*="im-chat"], [class*="IMChat"]'
    );
    // 至少 2 个特征才算（防单点误判）
    if (hasMessageList && hasChatContainer) return true;

    // 强特征：data-e2e 含 chat-item / message-item 列表项（不算 comment）
    const hasChatItems = document.querySelectorAll(
      '[data-e2e*="chat-item"], [data-e2e*="message-item"]'
    ).length >= 2;
    if (hasChatItems) return true;

    return false;
  } catch (_) {
    return false;
  }
}
