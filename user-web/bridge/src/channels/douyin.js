// 抖音网页私信适配器 —— 真实交互逻辑移植自 DY-auto。
// 参考：.research/DY-auto/douyin-auto-文字/content.js
//   editorBox / getRealSendButton / fillInputViaPaste / simulateRealClick / humanType
//
// 注意：DY-auto 仅做「自动回复固定话术」，不解析消息内容。因此消息读取（自/他区分、
// 文本提取、conversation_id）为基于抖音 IM 常见结构的「首版默认值」，需在真机校准
// （见 popup 自检 / bridge.md §校准清单）。
import { BaseAdapter } from '../core/channel-adapter.js';
import { CHANNELS, SENDER } from '../core/types.js';
import { qs, qsa, cleanText, simulateRealClick, fillContentEditable, createLogger, findAnyMessageInput, looksLikeMessagePage } from '../core/dom.js';

const log = createLogger('douyin', CHANNELS.DOUYIN);

// —— 真实选择器（来自深度自检 DOM 快照 + DY-auto 参考）——
// 抖音网页 IM 已改版：输入框无 role=textbox、私信常以浮层覆盖在 /jingxuan 等页打开、
// 消息气泡存在多版 DOM 变体。故不依赖 URL 路径与 role=textbox，改用「结构特征」匹配 +
// 弹性气泡选择器 + 对齐检测自他判定。
export const SEL = {
  CHAT_LIST: 'div.conversationConversationListwrapper, #island_b69f5',
  // 消息线程容器（弹性候选，命中其一即可作为 MutationObserver 根）
  MSG_LIST: '[class*="messageList"], [class*="MessageList"], [class*="chatMsg"], [class*="msgList"], [data-e2e*="msg-list"]',
  // 消息气泡：优先 data-e2e="msg-item-content"，覆盖多版 DOM 变体
  MSG_ITEM: 'div[data-e2e="msg-item-content"], [data-e2e*="msg-item"], [class*="msg-item-content"], [class*="msg-content"], [class*="bubble"], [class*="Bubble"], [class*="messageText"], [class*="MessageText"], [class*="chatMsgItem"], [class*="MessageItem"], [class*="messageItem"], [class*="msgBubble"], [class*="MsgBubble"], [class*="imMessage"], [class*="ImMessage"], [class*="dialogItem"], [class*="chatItem"], [class*="ChatItem"], [class*="messageBubble"]',
  // 自/他气泡 class 关键词（仅兜底，主判定走对齐检测）
  SELF_ITEM: '[class*="self"], [class*="right"], [class*="outgoing"]',
  OTHER_ITEM: '[class*="other"], [class*="left"], [class*="incoming"]',
  TEXT: '[data-e2e="msg-item-content"], [class*="msg-content"], [class*="text"], [class*="Text"]',
  // 严格输入框：真实抖音为「同一元素同时含 messageEditorinputArea + editor-kit-container」
  // （评论框仅父级含 editor-kit-container，故用组合类区分，避免 jingxuan 误判）
  EDITOR: 'div.messageEditorinputArea.editor-kit-container, div.zone-container.editor-kit-container.messageEditorinputArea, div[contenteditable="true"][role="textbox"]',
};

// 抖音输入框（strict）：真实输入框无 role=textbox，故改为匹配 messageEditorinputArea
function strictEditorBox() {
  return qs(SEL.EDITOR);
}

// 抖音输入框：优先真实 editor，再通用扫描容错
function editorBox() {
  let box = strictEditorBox();
  if (!box) {
    const boxes = qsa('div[contenteditable="true"]').filter((b) => b.offsetParent !== null);
    box = boxes[boxes.length - 1] || null;
  }
  return box;
}

// 抖音输入框（容错版）：先 strict editor，再 findAnyMessageInput 通用扫描
function findInputEl() {
  return editorBox() || findAnyMessageInput();
}

// 抖音发送按钮：真实为 svg.messageMsgInputpublishBtn.e2e-send-msg-btn
function getRealSendButton() {
  let btn = qs('[class*="e2e-send-msg-btn"]');
  if (btn) {
    // 发送按钮为 svg 时，点击须落到最近的可交互祖先（带 click 监听的
    // button/div/span），否则直接 dispatch 在 svg 上的 click 未必冒泡触发发送。
    if (btn.tagName === 'SVG') {
      return btn.closest('button, [role="button"], div, span') || btn;
    }
    return btn;
  }
  const redPaths = qsa('path').filter((p) => (p.getAttribute('fill') || '').toUpperCase() === '#FE2C55');
  for (const path of redPaths) {
    const clickable = path.closest('button, div, span, svg');
    if (clickable && clickable.offsetParent !== null) return clickable;
  }
  return qs('[class*="send"], [aria-label*="发送"]') || null;
}

// 最近的可滚动祖先（用于定位消息线程容器 / 对齐判定容器）
function closestScrollable(el) {
  let cur = el;
  while (cur && cur !== document.body) {
    const style = getComputedStyle(cur);
    const scrollable = style.overflowY === 'auto' || style.overflowY === 'scroll' || cur.scrollHeight > cur.clientHeight + 50;
    if (scrollable) return cur;
    cur = cur.parentElement;
  }
  return null;
}

// 自/他判定：以气泡相对「消息线程容器」的水平对齐为主信号（右=自己，左=客户）。
// 抖音气泡自他 class 命名多变，对齐检测不受其影响；class 关键词仅作兜底。
function classifyByAlignment(bubble) {
  const bRect = bubble.getBoundingClientRect();
  const bCenter = bRect.left + bRect.width / 2;
  const container = closestScrollable(bubble) || bubble.parentElement;
  const cRect = container ? container.getBoundingClientRect() : { left: 0, width: window.innerWidth };
  const cCenter = cRect.left + cRect.width / 2;
  return bCenter > cCenter ? SENDER.SELF : SENDER.CUSTOMER;
}

// 当前登录的抖音账号 id（左导航个人主页链接）
function getAccountId() {
  const self = qs('aside a[href*="/user/"], header a[href*="/user/"]') || qs('a[href*="/user/"]');
  // 兼容 /user/self（当前用户主页）与 /user/MS4w...（token 形式）与纯数字 id
  const m = self && self.getAttribute('href')?.match(/\/user\/([^/?#]+)/);
  return m ? m[1] : '';
}

// 当前会话 id：活动会话（深度自检显示活动项含 curConversation class）
function getConversationId() {
  if (/[?&]conversation_id=/.test(location.href)) {
    return new URLSearchParams(location.search).get('conversation_id');
  }
  const active = qsa(
    `${SEL.CHAT_LIST} [class*="curConversation"], ${SEL.CHAT_LIST} [aria-selected="true"], ${SEL.CHAT_LIST} [class*="active"]`
  ).find((el) => el.offsetParent !== null);
  const link = active?.querySelector('a[href*="/user/"]') || qs('[class*="chat-header"] a[href*="/user/"]');
  // 兼容 /user/<数字id> 与 /user/MS4w...（token 形式）；命中后切换会话会重新回填历史
  const m = link?.getAttribute('href')?.match(/\/user\/([^/?#]+)/);
  return m ? m[1] : null;
}

// 抖音 IM 结构特征（来自深度自检 DOM 快照）：
//   - 输入框 messageEditorinputArea（contenteditable，无 role=textbox）
//   - 会话列表 conversationConversationListwrapper / data-e2e="conversation-item"
//   - 发送 e2e-send-msg-btn
// 私信常作浮层覆盖在 /jingxuan 等页，URL 不切 /message，故以结构命中为主。
function isDouyinMessagePage() {
  // 真实私信输入框：同一元素同时含 messageEditorinputArea + editor-kit-container
  const hasEditor = !!document.querySelector(
    'div.messageEditorinputArea.editor-kit-container, div.zone-container.editor-kit-container.messageEditorinputArea'
  );
  // 真实会话列表：含 conversation-item 行（jingxuan 侧栏推荐列表只有 wrapper、无 item）
  const hasConvList = !!document.querySelector('div[data-e2e="conversation-item"], #island_b69f5 li');
  const hasSend = !!document.querySelector('[class*="e2e-send-msg-btn"]');
  if (hasEditor && hasConvList) return true;
  if (hasEditor && hasSend) return true;
  return false;
}

const hooks = {
  match() {
    if (!location.hostname.includes('douyin.com')) return false;
    if (isDouyinMessagePage()) return true; // 结构命中（含浮层 / 无 role=textbox）
    if (strictEditorBox()) return true; // DY-auto 严格同款兜底
    if (findAnyMessageInput() && looksLikeMessagePage()) {
      log.warn('match() 走 fallback 模式：抖音 IM 严格选择器已失效，使用通用 DOM 扫描');
      return true;
    }
    return false;
  },
  matchMode() {
    if (!location.hostname.includes('douyin.com')) return null;
    if (isDouyinMessagePage() || strictEditorBox()) return 'strict';
    return 'fallback';
  },
  selectors: SEL,
  getMessageListRoot() {
    const root = qs(SEL.MSG_LIST) || closestScrollable(qsa(SEL.MSG_ITEM)[0]);
    if (root) return root;
    // 兜底：IM 主面板（输入框/会话列表的最近公共祖先）
    const anchor = editorBox() || qs(SEL.CHAT_LIST);
    return anchor ? anchor.closest('[class*="im"], [class*="message"], [class*="chat"], [class*="Im"], [class*="Message"]') : null;
  },
  getMessageItems() {
    const items = qsa(SEL.MSG_ITEM);
    if (items.length) return items;
    // 兜底：抖音气泡 class 多变时，用「消息线程容器内的文本叶子」推断气泡。
    // 线程容器 = 最近的可滚动区域（输入框/会话列表之上的公共滚动区）。
    const thread = (() => {
      const anchor = editorBox() || qs(SEL.CHAT_LIST);
      if (!anchor) return null;
      const sc = closestScrollable(anchor);
      if (sc) return sc;
      // 否则取 IM 面板主容器
      return anchor.closest('[class*="im"], [class*="message"], [class*="chat"], [class*="Im"], [class*="Message"]') || null;
    })();
    if (!thread) return [];
    // 取线程容器内、含文本且可见的直接子级（或二级）div 作为气泡候选
    const leafText = [];
    const walk = (el, depth) => {
      if (depth > 3) return;
      for (const child of el.children) {
        const txt = cleanText(child);
        if (txt && child.offsetParent !== null && !child.querySelector(SEL.CHAT_LIST) && !child.querySelector(SEL.EDITOR)) {
          leafText.push(child);
        } else {
          walk(child, depth + 1);
        }
      }
    };
    walk(thread, 0);
    return leafText;
  },
  parseMessageItem(item) {
    // item 已是消息气泡（文本元素）。取文本（自身或其内部 text 元素）
    const textEl = item.querySelector(SEL.TEXT) || item;
    const text = cleanText(textEl);
    if (!text && !item.querySelector('img')) return null;
    // 自/他判定：对齐检测为主（右=自己，左=客户），class 关键词兜底
    let sender_type = classifyByAlignment(item);
    if (sender_type === SENDER.CUSTOMER && item.closest && (item.closest(SEL.SELF_ITEM) || (item.matches && item.matches(SEL.SELF_ITEM)))) {
      sender_type = SENDER.SELF;
    }
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
    fillContentEditable(editor, text);
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
