// TikTok 网页私信适配器 —— 功能对齐抖音/小红书/闲鱼
//
// 设计依据（基于用户提供的真实 DOM 快照）：
//  1. 域：https://www.tiktok.com
//  2. 入口页：https://www.tiktok.com/messages（私信页）/ /messages/@username（具体会话）
//  3. 真实 DOM 结构（data-e2e 属性体系）：
//     - 左侧会话列表：[data-e2e="dm-new-conversation-list"]
//     - 会话项：[data-e2e="dm-new-conversation-item"]（含 data-conversation-id）
//     - 活动会话：aria-selected="true"
//     - 昵称：[data-e2e="dm-new-conversation-nickname"]
//     - 最后消息：SpanInfoExtract
//     - 时间：SpanInfoTime
//     - 聊天头部：[data-e2e="dm-new-chatbox"] > [data-e2e="dm-new-chat-nickname"] + [data-e2e="chat-uniqueid"]
//     - 消息列表：[data-e2e="dm-new-message-list"]
//     - 消息项：[data-e2e="dm-new-chat-item"]
//     - 消息文本：[data-e2e="dm-new-message-text"]
//     - 时间分隔：[data-e2e="dm-new-time-separator"]
//     - 输入框：DraftEditor [data-e2e="dm-new-input-editor"] contenteditable
//     - 表情按钮：[data-e2e="dm-new-emoji-btn"]
//  4. 自/他判定：outgoing/incoming class（TikTok V3 IM 明确使用）
//  5. 发送：Draft.js 靠 Enter 发送（无显式发送按钮），或 SVG 飞机按钮兜底
import { BaseAdapter } from '../core/channel-adapter.js';
import { CHANNELS, SENDER } from '../core/types.js';
import { mergeSelectors, runExtractor, customConversationListSelectors } from '../core/selector-ai.js';
import { SelectorEngine } from '../core/selector-engine.js';
import {
  qs, qsa, cleanText, setValue, fillContentEditable, enhancedClick,
  simulateRealClick, createLogger, findAnyMessageInput, looksLikeMessagePage,
} from '../core/dom.js';

const log = createLogger('tiktok', CHANNELS.TIKTOK);

// —— AI 选择器合并 ——
function mergedSelectors() {
  const fb = {
    itemSelectors: SEL.MSG_ITEM.split(',').map((s) => s.trim()).filter(Boolean),
    listSelectors: SEL.MSG_LIST.split(',').map((s) => s.trim()).filter(Boolean),
    textSelectors: SEL.TEXT.split(',').map((s) => s.trim()).filter(Boolean),
    inputSelectors: (SEL.INPUT || '').split(',').map((s) => s.trim()).filter(Boolean),
    sendSelectors: (SEL.SEND || SEL.SEND_BTN || '').split(',').map((s) => s.trim()).filter(Boolean),
    selfMarkers: SEL.SELF_ITEM.split(',').map((s) => s.trim()).filter(Boolean),
    otherMarkers: SEL.OTHER_ITEM.split(',').map((s) => s.trim()).filter(Boolean),
  };
  return mergeSelectors(CHANNELS.TIKTOK, location.host, fb);
}

// —— 真实选择器（基于 TikTok V3 IM data-e2e 属性体系）——
export const SEL = {
  // 左侧会话列表容器
  CHAT_LIST: '[data-e2e="dm-new-conversation-list"], [class*="DivConversationListContainer"], [class*="InboxList"]',
  // 会话项
  CONV_ITEM: '[data-e2e="dm-new-conversation-item"], [class*="DivItemWrapper"]',
  // 消息列表容器
  MSG_LIST: '[data-e2e="dm-new-message-list"], [data-e2e="dm-new-chatbox"] [role="log"], [class*="DivChatMain"]',
  // 消息气泡
  MSG_ITEM: '[data-e2e="dm-new-chat-item"], [class*="DivChatItemWrapper"], [data-e2e="message-item"]',
  // 自方消息（TikTok V3: outgoing class）
  SELF_ITEM: '[class*="outgoing"], [class*="Outgoing"], [class*="self"], [class*="Self"]',
  // 对方消息（TikTok V3: incoming class）
  OTHER_ITEM: '[class*="incoming"], [class*="Incoming"], [class*="other"], [class*="Other"]',
  // 消息文本
  TEXT: '[data-e2e="dm-new-message-text"], [class*="DivTextContainer"], [class*="message-text"]',
  // 时间分隔文本
  TIME: '[data-e2e="dm-new-time-separator"], [class*="DivTimeContainer"]',
  // 系统消息 class 关键词
  SYSTEM: '[class*="system-msg" i], [class*="SystemMessage" i], [class*="notice" i], [class*="Notice" i], [class*="divider" i], [class*="Divider" i], [class*="time-separator" i], [class*="TimeSeparator" i]',
  // 未读标记
  UNREAD: '[class*="unread" i], [class*="Unread" i], [class*="red-dot" i], [class*="badge" i][class*="msg" i], [data-unread="1"]',
  // 消息类型 data 属性
  MSG_TYPE: '[data-msg-type], [data-message-type], [data-type]',
  // 卡片（TikTok 商品/链接卡片）
  CARD: '[class*="card-container" i], [class*="CardContainer" i], [class*="share-card" i], [class*="ShareCard" i]',
  // 聊天对象昵称（TikTok V3 header）
  PEER_NAME: '[data-e2e="dm-new-chat-nickname"], [class*="DivNameContainer"] [class*="PNickname"], [data-e2e="chat-uniqueid"]',
  // 输入框（TikTok: DraftEditor contenteditable）
  INPUT: '#main-content-messages div[contenteditable="true"][role="textbox"], [data-e2e="dm-new-input-editor"] div[contenteditable="true"], div.DraftEditor-root div[contenteditable="true"]',
  // 发送按钮（TikTok 多数靠 Enter，但可能有 SVG 飞机按钮）
  SEND: 'button[aria-label*="发送" i], button[aria-label*="Send" i], [data-e2e="message-button"], [class*="send-btn" i], [class*="SendBtn" i]',
};

// —— 严格输入框 / 弹性输入框 ——
const STRICT_INPUT_SELECTORS = [
  '#main-content-messages div[contenteditable="true"]',
  '[data-e2e="dm-new-input-editor"] div[contenteditable="true"]',
  'div.DraftEditor-root div[contenteditable="true"]',
  '[data-e2e="message-input"]',
];
const INPUT_FALLBACKS = [
  ...STRICT_INPUT_SELECTORS,
  'div[contenteditable="true"][role="textbox"]',
  'div[contenteditable="true"]',
  '[role="textbox"]',
  'textarea[placeholder*="Message" i]',
  'textarea[placeholder*="消息" i]',
];

function strictGetInputEl() {
  for (const sel of STRICT_INPUT_SELECTORS) {
    try {
      const el = qs(sel);
      if (el && el.offsetParent !== null) return el;
    } catch (_) { /* 非法选择器跳过 */ }
  }
  return null;
}

function getInputEl() {
  for (const sel of INPUT_FALLBACKS) {
    try {
      const el = qs(sel);
      if (el && el.offsetParent !== null) return el;
    } catch (_) { /* 非法选择器跳过 */ }
  }
  return null;
}

function findInputEl() {
  return getInputEl() || findAnyMessageInput();
}

// —— TikTok 聊天页 URL 判断 ——
function isTiktokChatUrl() {
  const path = location.pathname || '';
  return path === '/messages' || path.startsWith('/messages/');
}

function isTiktokHost() {
  return (location.hostname || '').includes('tiktok.com');
}

// —— 聊天页结构识别 ——
function isTiktokMessagePage() {
  if (!isTiktokHost()) return false;
  const hasInput = !!strictGetInputEl();
  const hasMsg = !!document.querySelector(
    '[data-e2e="dm-new-chat-item"], [class*="DivChatItemWrapper"], [data-e2e="message-item"]',
  );
  if (hasInput && hasMsg) return true;
  const hasConvList = !!document.querySelector(SEL.CHAT_LIST);
  if (hasConvList && hasInput) return true;
  return false;
}

// —— 联系人 ID 规范化 ——
function normalizeContactId(id) {
  if (id == null) return '';
  let s = String(id).trim();
  if (!s) return '';
  const prefixes = ['Total-', 'Active-', 'User-', 'Contact-', 'User_', 'Contact_'];
  for (const prefix of prefixes) {
    if (s.startsWith(prefix)) { s = s.substring(prefix.length); break; }
  }
  s = s.replace(/[^a-zA-Z0-9_:-]/g, '');
  return s;
}

// —— 当前登录账号 ID（永不返回空串）——
// TikTok DM 页面特点：会话列表只显示对方，不显示自己
// 因此必须优先从侧边栏/导航栏提取自己，避免把对方误判为自己
function getAccountId() {
  const candidates = [];
  // 1) 侧边栏/导航栏内的 profile 链接（自己，最高优先级）
  qsa('aside a[href*="/@"], nav a[href*="/@"], [class*="sidebar" i] a[href*="/@"]').forEach((a) => {
    const m = a.getAttribute('href')?.match(/@([\w.]+)/);
    if (m && m[1]) candidates.push(m[1]);
  });
  // 2) 顶部导航栏内非聊天区域的 profile 链接（如头像菜单）
  qsa('[class*="DivHeaderContainer"] a[href*="/@"], [data-e2e="nav-profile"] a[href*="/@"]').forEach((a) => {
    const m = a.getAttribute('href')?.match(/@([\w.]+)/);
    if (m && m[1]) candidates.push(m[1]);
  });
  // 3) localStorage 缓存（之前检测到的）
  try {
    const ls = localStorage.getItem(`hivebridge:account:${CHANNELS.TIKTOK}`);
    if (ls) return ls;
  } catch (e) { /* noop */ }
  // 4) 会话列表内的 /@ 链接（⚠️ 这些是对方，仅作最后手段）
  const convLinkSelectors = [
    '[data-e2e="dm-new-conversation-item"] a[href*="/@"]',
    '[class*="DivItemWrapper"] a[href*="/@"]',
    '[data-e2e="dm-new-conversation-list"] a[href*="/@"]',
  ];
  for (const sel of convLinkSelectors) {
    try {
      qsa(sel).forEach((a) => {
        const m = a.getAttribute('href')?.match(/@([\w.]+)/);
        if (m && m[1]) candidates.push(m[1]);
      });
    } catch (_) { /* 非法选择器跳过 */ }
  }
  // 取第一个有效候选
  for (const c of candidates) {
    if (c && c.trim()) {
      try { localStorage.setItem(`hivebridge:account:${CHANNELS.TIKTOK}`, c.trim()); } catch (e) { /* noop */ }
      return c.trim();
    }
  }
  // 5) 稳定 unknown（绝不返回空串，避免 WS 401）
  return `${CHANNELS.TIKTOK}-unknown`;
}

// —— 当前会话 ID ——
function getConversationIdFromUrl() {
  try {
    const pathMatch = (location.pathname || '').match(/\/messages\/@?([\w.]+)/);
    if (pathMatch && pathMatch[1]) return normalizeContactId(pathMatch[1]);
  } catch (_) { /* noop */ }
  return null;
}

function getConversationId() {
  // 1) URL 路径（最权威：/messages/@username）
  const urlId = getConversationIdFromUrl();
  if (urlId) return urlId;
  // 2) 活动会话项 data-conversation-id
  const activeItem = document.querySelector(
    '[data-e2e="dm-new-conversation-item"][aria-selected="true"], [class*="DivItemWrapper"][aria-selected="true"]',
  );
  if (activeItem) {
    const convId =
      activeItem.getAttribute('data-conversation-id') ||
      activeItem.getAttribute('data-id') ||
      activeItem.id ||
      null;
    const norm = normalizeContactId(convId);
    if (norm) return norm;
  }
  // 3) 聊天 header 对方链接（排除自己）
  const myAccount = getAccountId();
  const headerLinks = qsa('[data-e2e="dm-new-chatbox"] a[href*="/@"]');
  for (const link of headerLinks) {
    const href = link.getAttribute('href') || '';
    const m = href.match(/@([\w.]+)/);
    if (!m || !m[1]) continue;
    if (myAccount && m[1] === myAccount) continue;
    return m[1];
  }
  // 4) 活动会话项昵称派生
  if (activeItem) {
    const nameEl = activeItem.querySelector('[data-e2e="dm-new-conversation-nickname"], [class*="PInfoNickname"]');
    const name = nameEl ? cleanText(nameEl) : cleanText(activeItem);
    if (name) return 'conv:' + name.slice(0, 80);
  }
  return null;
}

// —— 聊天对象昵称 ——
function getPeerName() {
  // 1) SEL.PEER_NAME 候选
  for (const sel of String(SEL.PEER_NAME || '').split(',').map((s) => s.trim()).filter(Boolean)) {
    try {
      const el = qs(sel);
      if (el) {
        const t = cleanText(el);
        if (t && !/messages|message|chat|消息|聊天/i.test(t)) return t;
      }
    } catch (_) { /* 非法选择器忽略 */ }
  }
  // 2) header 内 /@ 链接文字（排除自己）
  const myAccount = getAccountId();
  const headerLinks = qsa('[data-e2e="dm-new-chatbox"] a[href*="/@"]');
  for (const link of headerLinks) {
    const href = link.getAttribute('href') || '';
    const m = href.match(/@([\w.]+)/);
    if (!m || !m[1]) continue;
    if (myAccount && m[1] === myAccount) continue;
    const t = cleanText(link);
    if (t && !/messages|message|chat/i.test(t)) return t;
  }
  // 3) 活动会话列表项昵称
  const activeItem = document.querySelector(
    '[data-e2e="dm-new-conversation-item"][aria-selected="true"]',
  );
  if (activeItem) {
    const nameEl = activeItem.querySelector('[data-e2e="dm-new-conversation-nickname"], [class*="PInfoNickname"], [class*="SpanNicknameText"]');
    const t = nameEl ? cleanText(nameEl) : '';
    if (t && !/messages|message|chat/i.test(t)) return t;
  }
  return '';
}

// —— 发送按钮 ——
function findSendButton() {
  const m = mergedSelectors();
  for (const sel of m.sendSelectors) {
    try {
      const el = qs(sel);
      if (el && el.offsetParent !== null) {
        if (el.tagName === 'SVG') return el.closest('button, [role="button"], div, span') || el;
        return el;
      }
    } catch (_) { /* 非法选择器跳过 */ }
  }
  // 兜底：TikTok 可能有 SVG 飞机发送按钮
  const sendBtns = qsa('[data-e2e="message-button"], [class*="send-btn" i], [class*="SendBtn" i]');
  for (const btn of sendBtns) {
    if (btn.offsetParent !== null) {
      if (btn.tagName === 'SVG') return btn.closest('button, [role="button"], div, span') || btn;
      return btn;
    }
  }
  // 兜底：扫描蓝色/主色调 SVG 路径（TikTok 使用 #FE2C55 / #00F2EA 等品牌色）
  const bluePaths = qsa('svg path').filter((p) => {
    const fill = (p.getAttribute('fill') || '').toUpperCase();
    return fill === '#FE2C55' || fill === '#00F2EA' || fill === '#25F4EE' || fill === '#FF4F24';
  });
  for (const path of bluePaths) {
    const clickable = path.closest('button, [role="button"], div[tabindex], span[tabindex]');
    if (clickable && clickable.offsetParent !== null) return clickable;
  }
  return null;
}

// —— 居中对齐检测（系统消息识别漏斗①）——
function isCenterAligned(el) {
  if (!el) return false;
  try {
    const ta = getComputedStyle(el).textAlign;
    if (ta === 'center' || ta === '-webkit-center') return true;
    const parent = el.parentElement;
    if (parent) {
      const jc = getComputedStyle(parent).justifyContent;
      if (jc === 'center') return true;
    }
  } catch (_) { /* jsdom 等无 getComputedStyle 时跳过 */ }
  return false;
}

// —— 内容模式匹配 ——
const TIME_PATTERNS = [
  /^\d{1,2}:\d{2}$/,
  /^(Today|Yesterday|Last)\s+\d{1,2}:\d{2}$/i,
  /^(今天|昨天|前天)\s*\d{1,2}:\d{2}$/,
  /^\d{1,2}\/\d{1,2}\/\d{2,4}$/,
  /^\d{4}[-/]\d{1,2}[-/]\d{1,2}$/,
  /^(上午|下午|凌晨|AM|PM)\s*\d{1,2}:\d{2}$/i,
  /^(Mon|Tue|Wed|Thu|Fri|Sat|Sun)\s*$/i,
  /^\d{1,2}:\d{2}\s*(AM|PM)$/i,
  // 从抖音补全
  /^\d{1,2}月\d{1,2}日(\s+\d{1,2}:\d{2})?$/,
];

const SYSTEM_TEXT_PATTERNS = [
  /you can now message this user/i,
  /follow each other to start chatting/i,
  /followed you/i,
  /accepted your follow request/i,
  /sent you a (message|friend request|follow)/i,
  /recalled a message/i,
  /撤回了一条消息/,
  /message could not be sent/i,
  /message deleted/i,
  /you are (no longer|not) (friends|following)/i,
  /this account is (private|public)/i,
  /typing/i,
  /以下为新消息/,
  /^={3,}$/,
  /^-{3,}$/,
  /blocked/i,
  /reported/i,
  // 从抖音补全
  /你(已)?添加.*为好友/,
  /已添加.*为好友/,
  /对方正在输入/,
  /互相关注/,
  /对方已开启.*验证/,
  /消息发送失败/,
  /消息已发出.*被对方拒收/,
  /我已收到你的消息/,
  /请等待卖家回复/,
];

function isTimeText(text) {
  if (!text) return false;
  const t = text.trim();
  return TIME_PATTERNS.some((re) => re.test(t));
}

function isSystemText(text) {
  if (!text) return false;
  const t = text.trim();
  return SYSTEM_TEXT_PATTERNS.some((re) => re.test(t));
}

// —— 系统消息识别（漏斗 5 层）——
function isSystemMessage(item) {
  if (!item) return false;
  // ① 居中对齐
  if (isCenterAligned(item)) return true;
  // ② class 关键词
  try {
    if (item.matches && item.matches(SEL.SYSTEM)) return true;
    if (item.querySelector && item.querySelector(SEL.SYSTEM)) return true;
  } catch (_) { /* 非法选择器跳过 */ }
  // ③ role / aria-live
  try {
    if (item.matches && (
      item.matches('[role="status"]') ||
      item.matches('[role="alert"]') ||
      item.matches('[aria-live="polite"]') ||
      item.matches('[aria-live="assertive"]')
    )) return true;
  } catch (_) { /* noop */ }
  // ④ 内容模式
  const text = cleanText(item);
  if (isTimeText(text) || isSystemText(text)) return true;
  // ⑤ 结构特征：无头像 + 无气泡（弱信号）
  if (text && text.length < 60) {
    const hasAvatar = !!item.querySelector('[class*="avatar" i], [class*="Avatar" i]');
    const hasBubble = !!item.querySelector('[class*="bubble" i], [class*="Bubble" i], [class*="text-container" i]');
    if (!hasAvatar && !hasBubble) return true;
  }
  return false;
}

// —— 自/他判定（漏斗 4 层）——
function closestScrollable(el) {
  let cur = el;
  while (cur && cur !== document.body) {
    try {
      const style = getComputedStyle(cur);
      const scrollable = style.overflowY === 'auto' || style.overflowY === 'scroll' || cur.scrollHeight > cur.clientHeight + 50;
      if (scrollable) return cur;
    } catch (_) { /* noop */ }
    cur = cur.parentElement;
  }
  return null;
}

function classifyByAlignment(bubble) {
  try {
    const bRect = bubble.getBoundingClientRect();
    if (bRect.width <= 0) return null;
    const bCenter = bRect.left + bRect.width / 2;
    const container = closestScrollable(bubble) || bubble.parentElement;
    const cRect = container ? container.getBoundingClientRect() : { left: 0, width: window.innerWidth || 1200 };
    const cCenter = cRect.left + cRect.width / 2;
    return bCenter > cCenter ? SENDER.SELF : SENDER.CUSTOMER;
  } catch (_) {
    return null;
  }
}

function classifyByDataAttribute(item) {
  if (!item || !item.dataset) return null;
  const sender = (item.dataset.sender || '').toLowerCase();
  if (sender === 'self' || sender === 'me') return SENDER.SELF;
  if (sender === 'other' || sender === 'them' || sender === 'peer') return SENDER.CUSTOMER;
  const dir = (item.dataset.direction || '').toLowerCase();
  if (dir === 'outgoing') return SENDER.SELF;
  if (dir === 'inbound' || dir === 'incoming') return SENDER.CUSTOMER;
  if (item.dataset.isSelf === 'true') return SENDER.SELF;
  if (item.dataset.isSelf === 'false') return SENDER.CUSTOMER;
  if (item.dir === 'rtl') return SENDER.SELF;
  return null;
}

function classifySender(item) {
  // A) class 关键词（最高优先级）
  const m = mergedSelectors();
  const matchesSelf = m.selfMarkers.some((sel) => {
    try { return (item.matches && item.matches(sel)) || (item.closest && item.closest(sel)); } catch (_) { return false; }
  });
  const matchesOther = m.otherMarkers.some((sel) => {
    try { return (item.matches && item.matches(sel)) || (item.closest && item.closest(sel)); } catch (_) { return false; }
  });
  if (matchesSelf && !matchesOther) return SENDER.SELF;
  if (matchesOther && !matchesSelf) return SENDER.CUSTOMER;
  // B) data-* 属性
  const byData = classifyByDataAttribute(item);
  if (byData) return byData;
  // C) 水平对齐
  const byAlign = classifyByAlignment(item);
  if (byAlign) return byAlign;
  // 默认 CUSTOMER（防回环）
  return SENDER.CUSTOMER;
}

// —— 非文字消息提取 ——
function extractMessageContent(item) {
  const typeAttr = (item.getAttribute('data-msg-type') || item.querySelector(SEL.MSG_TYPE)?.getAttribute('data-msg-type') || '').toUpperCase();
  const text = cleanText(item.querySelector(SEL.TEXT) || item);
  // 时间过滤
  if (text && isTimeText(text)) {
    return { msgType: 'system', mediaUrl: '', text: '' };
  }
  // 系统文本
  if (text && isSystemText(text)) {
    return { msgType: 'system', mediaUrl: '', text };
  }
  // 卡片
  const card = item.querySelector(SEL.CARD);
  if (card || typeAttr === 'CARD' || typeAttr === 'SHARE') {
    const title = cleanText(card?.querySelector('[class*="title" i], [class*="Title" i]'));
    const info = cleanText(card?.querySelector('[class*="info" i], [class*="desc" i], [class*="Desc" i]'));
    const img = card?.querySelector('img');
    return { msgType: 'card', mediaUrl: img?.getAttribute('src') || '', text: info || title || '卡片消息' };
  }
  // 撤回
  if (/recalled? a message|撤回了一?条消息/i.test(text)) {
    return { msgType: 'recall', mediaUrl: '', text };
  }
  // 媒体
  const imgs = Array.from(item.querySelectorAll('img')).filter(
    (img) => !(img.closest && img.closest('[class*="avatar" i], [class*="Avatar" i]')),
  );
  const vids = item.querySelectorAll('video, [class*="video" i], [class*="Video" i]');
  const audio = item.querySelectorAll('[class*="voice" i], [class*="Voice" i], [class*="audio" i], [class*="Audio" i]');
  const links = item.querySelectorAll('a[href*="http"]');
  let mediaUrl = '';
  let msgType = 'text';
  if (typeAttr === 'IMAGE' || (imgs.length && !text)) {
    msgType = 'image';
    mediaUrl = imgs[0]?.getAttribute('src') || '';
  } else if (typeAttr === 'VIDEO' || vids.length) {
    msgType = 'video';
    mediaUrl = vids[0]?.getAttribute('src') || '';
  } else if (typeAttr === 'VOICE' || typeAttr === 'AUDIO' || audio.length) {
    msgType = 'voice';
  } else if (links.length && !text) {
    msgType = 'link';
    mediaUrl = links[0].getAttribute('href') || '';
  }
  return { msgType, mediaUrl, text };
}

// —— 群聊识别 ——
function detectGroup(item) {
  let isGroup = false;
  let groupId = '';
  let groupName = '';
  let senderName = '';
  const headerText = getPeerName() || '';
  if (headerText && /group|team|chat|群|团队/i.test(headerText)) {
    isGroup = true;
    groupName = headerText;
  }
  const inner = cleanText(item);
  const atMatch = inner.match(/^@([^\s,，：:]+)[\s,，：:]/);
  const bracketMatch = inner.match(/^\[([^\]]+)\]/);
  if (atMatch || bracketMatch) {
    isGroup = true;
    senderName = (atMatch && atMatch[1]) || (bracketMatch && bracketMatch[1]) || '';
  }
  const gid = (location.href.match(/[?&](group|conversation)_id=([^&]+)/) || [])[2];
  if (gid) {
    groupId = gid;
    isGroup = true;
  }
  return { isGroup, groupId, groupName, senderName };
}

// —— 噪音过滤 ——
function isListNoise(item) {
  if (!item || !item.closest) return false;
  if (item.closest(SEL.CONV_ITEM)) return true;
  if (item.closest(SEL.CHAT_LIST)) {
    if (item.matches('[data-e2e="dm-new-conversation-item"], [class*="DivItemWrapper"]')) return true;
    if (item.querySelector('[data-e2e="dm-new-conversation-item"], [class*="DivItemWrapper"]')) return true;
  }
  return false;
}

// —— 未读检测 ——
function detectUnread(item) {
  if (!item) return false;
  try {
    if (item.matches && item.matches(SEL.UNREAD)) return true;
    if (item.querySelector && item.querySelector(SEL.UNREAD)) return true;
  } catch (_) { /* 非法选择器忽略 */ }
  // TikTok 未读通常有红色 badge 数字
  const badge = item.querySelector('[class*="badge" i], [class*="Badge" i], [class*="count" i]');
  if (badge && badge.textContent && /\d/.test(badge.textContent)) return true;
  return false;
}

// —— 会话列表枚举 ——
function getConversationList() {
  const customSels = customConversationListSelectors(CHANNELS.TIKTOK);
  let items = [];
  if (customSels.length) {
    for (const sel of customSels) {
      try { items = items.concat(qsa(sel)); } catch (_) { /* 非法选择器跳过 */ }
    }
  }
  if (!items.length) items = qsa(SEL.CONV_ITEM);
  // TikTok 兜底：从 conversation-list 容器内找
  if (!items.length) {
    const container = qs('[data-e2e="dm-new-conversation-list"]');
    if (container) items = Array.from(container.querySelectorAll('[data-e2e="dm-new-conversation-item"], [class*="DivItemWrapper"]'));
  }
  if (!items.length) return [];
  const out = [];
  const ids = new Set();
  for (const item of items) {
    if (!item || !item.offsetParent) continue;
    // ID 提取：data-conversation-id → /@username 链接 → 昵称
    let id = null;
    const raw = item.getAttribute('data-conversation-id') || item.getAttribute('data-id') || item.id || null;
    id = normalizeContactId(raw);
    if (!id) {
      const link = item.querySelector('a[href*="/@"]');
      if (link) {
        const m = link.getAttribute('href')?.match(/@([\w.]+)/);
        if (m && m[1]) id = m[1];
      }
    }
    const nameEl = item.querySelector('[data-e2e="dm-new-conversation-nickname"], [class*="PInfoNickname"], [class*="SpanNicknameText"]');
    const name = nameEl ? cleanText(nameEl) : '';
    if (!id) {
      if (name) id = 'conv:' + name.slice(0, 80);
      else continue;
    }
    if (ids.has(id)) continue;
    ids.add(id);
    const unread = detectUnread(item);
    out.push({ id, name, el: item, unread });
  }
  return out;
}

// —— hooks 对象 ——
const hooks = {
  match() {
    if (!isTiktokHost()) return false;
    if (isTiktokChatUrl()) return true;
    if (isTiktokMessagePage()) return true;
    if (strictGetInputEl()) return true;
    if (findAnyMessageInput() && looksLikeMessagePage()) {
      log.warn('match() 走 fallback 模式：TikTok IM 严格选择器已失效，使用通用 DOM 扫描');
      return true;
    }
    return false;
  },
  matchMode() {
    if (!isTiktokHost()) return null;
    if (isTiktokChatUrl() || isTiktokMessagePage() || strictGetInputEl()) return 'strict';
    return 'fallback';
  },
  selectors: SEL,
  getMessageListRoot() {
    const m = mergedSelectors();
    for (const sel of m.listSelectors) {
      try {
        const el = qs(sel);
        if (el) return el;
      } catch (_) { /* 非法选择器跳过 */ }
    }
    return qs(SEL.MSG_LIST) || qs('[class*="DivChatMain"]') || qs('[role="log"]') || null;
  },
  getMessageItems() {
    const m = mergedSelectors();
    const { items } = SelectorEngine.locateMessages({
      root: document,
      itemSelectors: m.itemSelectors,
      listSelectors: m.listSelectors,
    });
    const filtered = items.filter((it) => !isListNoise(it) && !isSystemMessage(it));
    if (filtered.length) return filtered;
    // 兜底：扫描线程容器内文本叶子
    const thread = this.getMessageListRoot();
    if (!thread) return [];
    const leafText = [];
    const walk = (el, depth) => {
      if (depth > 3) return;
      for (const child of el.children) {
        const txt = cleanText(child);
        if (txt && child.offsetParent !== null && !child.querySelector(SEL.CONV_ITEM) && !child.querySelector(SEL.INPUT)) {
          leafText.push(child);
        } else {
          walk(child, depth + 1);
        }
      }
    };
    walk(thread, 0);
    return leafText.filter((el) => !isListNoise(el) && !isSystemMessage(el));
  },
  parseMessageItem(item) {
    if (isListNoise(item)) return null;
    // ① 系统消息识别
    if (isSystemMessage(item)) {
      const sysText = cleanText(item);
      return {
        message_id: item.getAttribute('data-message-id') || item.getAttribute('data-id') || item.id || `sys:${sysText}:${Date.now()}`,
        sender_type: SENDER.SYSTEM,
        text: sysText,
        media_url: '',
        msg_type: 'system',
        is_group: false,
        group_id: '',
        group_name: '',
        sender_name: '',
        timestamp: Date.now(),
        raw: item.outerHTML?.slice(0, 500),
      };
    }
    // ② 内容提取
    const { msgType, mediaUrl, text } = extractMessageContent(item);
    if (!text && msgType === 'text') return null;
    // ③ 自/他判定
    const sender_type = classifySender(item);
    // ④ 群聊识别
    const groupInfo = detectGroup(item);
    // ⑤ 发件人昵称
    let senderName = groupInfo.senderName || '';
    if (!senderName && !groupInfo.isGroup && sender_type === SENDER.CUSTOMER) {
      senderName = getPeerName();
    }
    // ⑥ 消息 id
    const mid =
      item.getAttribute('data-message-id') ||
      item.getAttribute('data-id') ||
      item.getAttribute('data-msg-id') ||
      item.id ||
      `${getConversationId()}:${text}:${item.textContent?.length}`;
    return {
      message_id: mid,
      sender_type,
      text,
      media_url: mediaUrl,
      msg_type: msgType,
      is_group: groupInfo.isGroup,
      group_id: groupInfo.groupId,
      group_name: groupInfo.groupName,
      sender_name: senderName,
      timestamp: Date.now(),
      raw: item.outerHTML?.slice(0, 500),
    };
  },
  getAccountId,
  getConversationId,
  getConversationList,
  getPeerName,
  extractMessages() { return runExtractor(CHANNELS.TIKTOK, location.host); },
  async sendText(text) {
    const input = findInputEl();
    if (!input) {
      log.error('未找到 TikTok 输入框（strict + fallback 均失败）');
      throw new Error('tiktok input not found');
    }
    // TikTok 使用 Draft.js contenteditable
    if (input.isContentEditable || input.getAttribute('contenteditable') === 'true' || input.tagName !== 'TEXTAREA') {
      fillContentEditable(input, text);
    } else {
      setValue(input, text);
    }
    await new Promise((r) => setTimeout(r, 150));
    const btn = findSendButton();
    if (btn) {
      enhancedClick(btn);
      return;
    }
    // 兜底：回车发送
    log.warn('未找到 TikTok 发送按钮，改用回车发送');
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true, cancelable: true }));
    input.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true, cancelable: true }));
  },
};

// 导出供单测验证
export {
  getAccountId, getConversationId, getConversationList,
  normalizeContactId, isTiktokMessagePage, isTiktokChatUrl, getPeerName,
  isSystemMessage, classifySender, isTimeText, isSystemText,
  isCenterAligned, isListNoise, detectUnread, detectGroup,
  findSendButton, findInputEl,
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
