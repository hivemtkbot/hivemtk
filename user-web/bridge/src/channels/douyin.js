// 抖音网页私信适配器 —— 真实交互逻辑移植自 DY-auto。
// 参考：.research/DY-auto/douyin-auto-文字/content.js
//   editorBox / getRealSendButton / fillInputViaPaste / simulateRealClick / humanType
//
// 注意：DY-auto 仅做「自动回复固定话术」，不解析消息内容。因此消息读取（自/他区分、
// 文本提取、conversation_id）为基于抖音 IM 常见结构的「首版默认值」，需在真机校准
// （见 popup 自检 / bridge.md §校准清单）。
import { BaseAdapter } from '../core/channel-adapter.js';
import { CHANNELS, SENDER } from '../core/types.js';
import { qs, qsa, cleanText, simulateRealClick, fillContentEditable, humanType, createLogger, findAnyMessageInput, looksLikeMessagePage } from '../core/dom.js';
import { isSelfMessage } from '../core/fallback.js';

const log = createLogger('douyin', CHANNELS.DOUYIN);

// —— 真实选择器（来自 DY-auto，均可经 popup 校准） ——
export const SEL = {
  CHAT_LIST: '#island_b69f5',
  // 消息气泡：抖音 IM 容器（首版默认，待校准）
  MSG_LIST: '[class*="chat-message-list"], [class*="ChatMessageList"], [class*="web-msg-list"]',
  MSG_ITEM: '[class*="chat-message-item"], [class*="ChatMessageItem"], [class*="message-item"]',
  SELF_ITEM: '.right', // 自己消息（待真机校准）
  OTHER_ITEM: '.left', // 对方消息（待真机校准）
  TEXT: '.text-message, .chat-message-text, [class*="text"]',
  // 输入框（DY-auto editorBox 同款）
  EDITOR: 'div[contenteditable="true"][role="textbox"]',
};

// 抖音输入框（strict）：仅匹配 SEL.EDITOR = div[contenteditable="true"][role="textbox"]
// 用途：matchMode() 用它区分 strict / fallback，不能用 editorBox()（后者已有容错会假阳）
function strictEditorBox() {
  return qs(SEL.EDITOR);
}

// 抖音输入框：优先 role=textbox 的 contenteditable（DY-auto editorBox 原样）
function editorBox() {
  let box = strictEditorBox();
  if (!box) {
    const boxes = qsa('div[contenteditable="true"]').filter((b) => b.offsetParent !== null);
    box = boxes[boxes.length - 1] || null;
  }
  return box;
}

// 抖音输入框（容错版）：先 strict editorBox，再 findAnyMessageInput 通用扫描
// 用途：sendText 在 strict 选择器失效时仍能找到输入框，避免「桥接已启动但发送失败」
function findInputEl() {
  return editorBox() || findAnyMessageInput();
}

// 抖音发送按钮：DY-auto getRealSendButton 原样（红色 #FE2C55 填充的 SVG）
function getRealSendButton() {
  let btn = qs('span.PygT7Ced.JnY63Rbk.e2e-send-msg-btn');
  if (btn) return btn;
  const redPaths = qsa('path').filter((p) => (p.getAttribute('fill') || '').toUpperCase() === '#FE2C55');
  for (const path of redPaths) {
    const clickable = path.closest('button, div, span, svg');
    if (clickable && clickable.offsetParent !== null) return clickable;
  }
  return qs('[class*="send"], [aria-label*="发送"]') || null;
}

// 当前登录的抖音账号 id（左导航个人主页链接）
function getAccountId() {
  const self = qs('aside a[href*="/user/"], header a[href*="/user/"]') || qs('a[href*="/user/"]');
  const m = self && self.getAttribute('href')?.match(/\/user\/(\d+)/);
  return m ? m[1] : '';
}

// 当前会话 id：活动会话（抖音 IM 常用 data-uid / 用户链接）
function getConversationId() {
  if (/[?&]conversation_id=/.test(location.href)) {
    return new URLSearchParams(location.search).get('conversation_id');
  }
  const active = qsa(`${SEL.CHAT_LIST} li, ${SEL.CHAT_LIST} [role="listitem"]`).find(
    (el) => el.classList.contains('active') || el.getAttribute('aria-selected') === 'true'
  );
  const link = active?.querySelector('a[href*="/user/"]') || qs('[class*="chat-header"] a[href*="/user/"]');
  const m = link?.getAttribute('href')?.match(/\/user\/(\d+)/);
  return m ? m[1] : null;
}

const hooks = {
  match() {
    if (!location.hostname.includes('douyin.com')) return false;
    // 严格匹配：DY-auto editorBox 同款选择器
    if (strictEditorBox()) return true;
    // 容错匹配：通用 DOM 扫描 + 页面像私信页（避免在首页/feed 误启动）
    if (findAnyMessageInput() && looksLikeMessagePage()) {
      log.warn('match() 走 fallback 模式：抖音 IM 严格选择器已失效，使用通用 DOM 扫描');
      return true;
    }
    return false;
  },
  matchMode() {
    if (!location.hostname.includes('douyin.com')) return null;
    return strictEditorBox() ? 'strict' : 'fallback';
  },
  selectors: SEL,
  getMessageListRoot() {
    return qs(SEL.MSG_LIST) || qs(SEL.CHAT_LIST)?.closest('[class*="im"], [class*="message"]');
  },
  getMessageItems() {
    return qsa(SEL.MSG_ITEM);
  },
  parseMessageItem(item) {
    // 9 自他消息判定兜底：先按 .right/.left class，再用头像位置兜底
    const isSelf = isSelfMessage(item, '.right') || isSelfMessage(item, SEL.SELF_ITEM);
    const isOther = !isSelf && (item.classList.contains('left') || !!item.querySelector(SEL.OTHER_ITEM));
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
    const editor = findInputEl();
    if (!editor) {
      log.error('未找到抖音输入框（strict + fallback 均失败）');
      throw new Error('douyin editor not found');
    }
    // 首版用 fillContentEditable（粘贴+innerText 双写），拟人模式可切 humanType
    fillContentEditable(editor, text);
    // 等待输入生效
    await new Promise((r) => setTimeout(r, 150));
    const btn = getRealSendButton();
    if (!btn) {
      log.error('未找到抖音发送按钮');
      throw new Error('douyin send button not found');
    }
    simulateRealClick(btn);
  },
};

export function buildDouyinAdapter() {
  return new BaseAdapter({
    name: 'douyin',
    channel: CHANNELS.DOUYIN,
    SEL,
    hooks,
    rateLimiter: undefined, // 使用默认 RateLimiter
  });
}
