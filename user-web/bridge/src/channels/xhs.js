// 小红书网页私信适配器 —— 真实交互逻辑移植自 XHS-YYDS。
// 参考：.research/XHS-YYDS/src/domUtils.js、src/messageDetector.js、src/autoReply.js
//   - 输入框 #jarvis-reply-textarea（textarea，setValue 设值 + input 事件）
//   - 发送 .send_btn（enhancedClickWithVerification / simulateClick）
//   - 消息 .im-msg-item + .left(对方)/.right(自己) + .text-message
//   - MutationObserver 监听 .vue-recycle-scroller（消息列表）
import { BaseAdapter } from '../core/channel-adapter.js';
import { CHANNELS, SENDER } from '../core/types.js';
import { qs, qsa, cleanText, setValue, enhancedClick, createLogger } from '../core/dom.js';
import { isSelfMessage } from '../core/fallback.js';

const log = createLogger('xhs', CHANNELS.XHS);

export const SEL = {
  CHAT_LIST: '.sx-contact-item, .chat-list-box [class*="contact"]',
  MSG_LIST: '.vue-recycle-scroller.ready.direction-vertical, [class*="message-list"], [class*="chat-content"]',
  MSG_ITEM: '.im-msg-item',
  SELF_ITEM: '.right',
  OTHER_ITEM: '.left',
  TEXT: '.text-message',
  INPUT: '#jarvis-reply-textarea',
  SEND: '.send_btn',
};

function getAccountId() {
  const self = qs('a[href*="/user/profile/"]') || qs('a[href*="/user/"]');
  const m = self?.getAttribute('href')?.match(/\/user\/(?:profile\/)?([\w-]+)/);
  return m ? m[1] : '';
}

function getConversationId() {
  if (/[?&]conversation_id=/.test(location.href)) {
    return new URLSearchParams(location.search).get('conversation_id');
  }
  const active =
    qsa('.sx-contact-item').find((el) => el.classList.contains('active') || el.getAttribute('aria-selected') === 'true') ||
    qs('.im-chat-window [class*="header"]');
  // 小红书会话 id 多挂在 contact item 的 data 属性上
  const id =
    active?.getAttribute('data-key') ||
    active?.getAttribute('data-id') ||
    active?.getAttribute('data-contactusemid') ||
    active?.getAttribute('id') ||
    active?.getAttribute('data-uid');
  return id || null;
}

const hooks = {
  match() {
    return location.hostname.includes('xiaohongshu.com') && !!qs(SEL.INPUT);
  },
  selectors: SEL,
  getMessageListRoot() {
    return qs(SEL.MSG_LIST) || qs('[class*="chat"]');
  },
  getMessageItems() {
    return qsa(SEL.MSG_ITEM);
  },
  parseMessageItem(item) {
    // P2-S2-9 自他消息判定兜底：先按 .right/.left class，再用头像位置兜底
    const isSelf = isSelfMessage(item, '.right') || isSelfMessage(item, SEL.SELF_ITEM);
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
  async sendText(text) {
    const input = qs(SEL.INPUT);
    if (!input) {
      log.error('未找到小红书输入框 #jarvis-reply-textarea');
      throw new Error('xhs input not found');
    }
    // XHS domUtils.setValue 同款：设值 + 派发 input
    setValue(input, text);
    await new Promise((r) => setTimeout(r, 120));
    const sendBtn = qs(SEL.SEND);
    if (!sendBtn) {
      log.error('未找到小红书发送按钮 .send_btn');
      throw new Error('xhs send button not found');
    }
    // XHS autoReply enhancedClickWithVerification 同款
    enhancedClick(sendBtn);
  },
};

export function buildXhsAdapter() {
  return new BaseAdapter({
    name: 'xhs',
    channel: CHANNELS.XHS,
    SEL,
    hooks,
    rateLimiter: undefined,
  });
}
