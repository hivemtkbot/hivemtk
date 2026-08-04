// TikTok 网页私信适配器 —— 真实交互逻辑移植自 tiktok-auto-plugin。
// 参考：.research/tiktok-auto-plugin/core/content.js、utils/simulator.js
//   - 输入框：#main-content-messages 下 DraftEditor contenteditable（多兜底）
//   - 发送：simulateTyping（逐字符输入 + input 事件）+ simulateEnterKey；
//          或点发送按钮（svg 飞机按钮 / button[aria-label*="Send"]）
//   - 该插件仅做「自动回复」，不解析消息内容，故消息读取为首版默认值，需真机校准。
import { BaseAdapter } from '../core/channel-adapter.js';
import { CHANNELS, SENDER } from '../core/types.js';
import { runExtractor } from '../core/selector-ai.js';
import { qs, qsa, cleanText, simulateTyping, simulateEnterKey, enhancedClick, createLogger, findAnyMessageInput, looksLikeMessagePage } from '../core/dom.js';
import { isSelfMessage } from '../core/fallback.js';

const log = createLogger('tiktok', CHANNELS.TIKTOK);

const INPUT_FALLBACKS = [
  '#main-content-messages div[contenteditable="true"]',
  '[data-e2e="message-input"]',
  'div[contenteditable="true"][role="textbox"]',
  'div[contenteditable="true"]',
  '[role="textbox"]',
  'textarea[placeholder*="消息"]',
  'textarea[placeholder*="Message"]',
];

export const SEL = {
  CHAT_LIST: '[data-e2e="chat-list"], [class*="InboxList"], [class*="chat-list"]',
  MSG_LIST: '[data-e2e="chat-message-list"], [class*="ChatList"], [class*="message-list"]',
  MSG_ITEM: '[data-e2e="message-item"], [class*="DivChatItem"], [class*="ChatItem"]',
  SELF_ITEM: '[class*="outgoing"], [class*="Outgoing"]',
  OTHER_ITEM: '[class*="incoming"], [class*="Incoming"]',
  TEXT: '[data-e2e="message-text"], [class*="message-text"], p',
  SEND_BTN: 'button[aria-label*="发送"], button[aria-label*="Send"], [data-e2e="message-button"]',
};

// TikTok 输入框（strict）：仅匹配带平台特定属性的选择器
// 用途：matchMode() 区分 strict / fallback 时使用
const STRICT_INPUT_SELECTORS = [
  '#main-content-messages div[contenteditable="true"]',
  '[data-e2e="message-input"]',
  'div[contenteditable="true"][role="textbox"]',
];

function strictGetInputEl() {
  for (const sel of STRICT_INPUT_SELECTORS) {
    const el = qs(sel);
    if (el && el.offsetParent !== null) return el;
  }
  return null;
}

function getInputEl() {
  for (const sel of INPUT_FALLBACKS) {
    const el = qs(sel);
    if (el && el.offsetParent !== null) return el;
  }
  return null;
}

// TikTok 输入框（容错版）：先 strict 多兜底，再 findAnyMessageInput 通用扫描
// 用途：match() 和 sendText 在 TikTok 改版后仍能工作
function findInputEl() {
  return getInputEl() || findAnyMessageInput();
}

function findSendButton() {
  const btn = qs(SEL.SEND_BTN);
  if (btn && btn.offsetParent !== null) return btn;
  // 兜底：DraftEditor 用回车发送
  return null;
}

function getAccountId() {
  const m = location.pathname.match(/^\/@([\w.]+)/);
  if (m) return m[1];
  const self = qs('a[href^="https://www.tiktok.com/@"]');
  const mm = self?.getAttribute('href')?.match(/@([\w.]+)/);
  return mm ? mm[1] : '';
}

function getConversationId() {
  const m = location.pathname.match(/^\/messages\/@?([\w.]+)/) || location.pathname.match(/@([\w.]+)/);
  if (m) return m[1];
  const header = qs('[data-e2e="chat-header"], [class*="ChatHeader"]');
  const link = header?.querySelector('a[href*="@"]');
  const mm = link?.getAttribute('href')?.match(/@([\w.]+)/);
  return mm ? mm[1] : null;
}

const hooks = {
  match() {
    if (!location.hostname.includes('tiktok.com')) return false;
    // 严格匹配：tiktok-auto-plugin 原款多兜底（仅平台特定选择器）
    if (strictGetInputEl()) return true;
    // 容错匹配：通用 DOM 扫描 + 页面像私信页
    if (findAnyMessageInput() && looksLikeMessagePage()) {
      log.warn('match() 走 fallback 模式：TikTok IM 严格选择器已失效，使用通用 DOM 扫描');
      return true;
    }
    return false;
  },
  matchMode() {
    if (!location.hostname.includes('tiktok.com')) return null;
    return strictGetInputEl() ? 'strict' : 'fallback';
  },
  selectors: SEL,
  getMessageListRoot() {
    return qs(SEL.MSG_LIST) || qs('[class*="Chat"]');
  },
  getMessageItems() {
    return qsa(SEL.MSG_ITEM);
  },
  parseMessageItem(item) {
    // 9 自他消息判定兜底：先按 outgoing class，再用头像位置兜底
    const isSelf = isSelfMessage(item, '[class*="outgoing"]') || isSelfMessage(item, SEL.SELF_ITEM);
    const sender_type = isSelf ? SENDER.SELF : SENDER.CUSTOMER;
    const textEl = item.querySelector(SEL.TEXT);
    const text = cleanText(textEl);
    if (!text && !item.querySelector('img')) return null;
    const mid =
      item.getAttribute('data-id') ||
      item.getAttribute('data-msg-id') ||
      item.getAttribute('id') ||
      `${getConversationId()}:${text}:${item.textContent?.length}`;
    return { message_id: mid, sender_type, text, media_url: '', timestamp: Date.now(), raw: item.outerHTML?.slice(0, 500) };
  },
  getAccountId,
  getConversationId,
  // 读取主路径：运行 LLM 生成的可执行 JS 抽取器（彻底不依赖固定选择器）。
  extractMessages() { return runExtractor(CHANNELS.TIKTOK, location.host); },
  async sendText(text) {
    const input = findInputEl();
    if (!input) {
      log.error('未找到 TikTok 输入框（strict + fallback 均失败）');
      throw new Error('tiktok input not found');
    }
    // tiktok-auto-plugin simulateTyping 同款：逐字符输入 + input 事件
    await simulateTyping(input, text, { interval: 25 });
    await new Promise((r) => setTimeout(r, 150));
    const btn = findSendButton();
    if (btn) {
      enhancedClick(btn);
    } else {
      // Draft.js 靠 Enter 发送（simulateEnterKey 同款）
      simulateEnterKey(input);
    }
  },
};

export function buildTiktokAdapter() {
  return new BaseAdapter({
    name: 'tiktok',
    channel: CHANNELS.TIKTOK,
    SEL,
    hooks,
    rateLimiter: undefined,
  });
}
