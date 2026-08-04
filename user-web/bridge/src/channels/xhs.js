// 小红书网页私信适配器 —— 真实交互逻辑移植自 XHS-YYDS。
// 参考：.research/XHS-YYDS/src/domUtils.js、src/messageDetector.js、src/autoReply.js
//   - 输入框 #jarvis-reply-textarea（textarea，setValue 设值 + input 事件）
//   - 发送 .send_btn（enhancedClickWithVerification / simulateClick）
//   - 消息 .im-msg-item + .left(对方)/.right(自己) + .text-message + [data-msg-type]
//   - MutationObserver 监听消息列表（.vue-recycle-scroller / chat-content）
//   - 会话项 .sx-contact-item（data-key / data-contactusemid / data-id）
//
// 架构对齐抖音适配器（src/channels/douyin.js）：
//   - AI 选择器合并（mergeSelectors：LLM 生成产物优先、规则兜底）——平台改版无需发版
//   - 弹性多候选 SEL（改版/换肤时任一命中即用）
//   - 结构页面识别 isXhsMessagePage（不依赖 URL 路径，SPA 浮层/面板均可命中）
//   - 账号/会话/会话列表多级兜底（localStorage 缓存 + 稳定 unknown，杜绝空串导致 WS 401）
//   - 消息类型提取（卡片/图片/语音/视频/链接/撤回/聚光系统消息）
//   - 噪音过滤（会话列表项/联系人卡片绝不当作聊天内容上行）
//   - 发送多级兜底（合并选择器 → 规则 → 通用 DOM 扫描）
import { BaseAdapter } from '../core/channel-adapter.js';
import { CHANNELS, SENDER } from '../core/types.js';
import { mergeSelectors, runExtractor, customConversationListSelectors } from '../core/selector-ai.js';
import { SelectorEngine } from '../core/selector-engine.js';
import { qs, qsa, cleanText, setValue, fillContentEditable, enhancedClick, createLogger, findAnyMessageInput, looksLikeMessagePage } from '../core/dom.js';
import { isSelfMessage } from '../core/fallback.js';

const log = createLogger('xhs', CHANNELS.XHS);

// —— AI 选择器 ——
// 运行时优先使用 selector-ai 缓存的「云端 LLM 生成选择器」，仅在未命中/未配置时
// 回退到本文件的 SEL 规则候选（mergeSelectors 已处理优先级：AI 在前、规则在后）。
// 兜底对象在调用时构建（避免早于 SEL 声明求值导致的 TDZ 问题）。
function mergedSelectors() {
  const fb = {
    itemSelectors: SEL.MSG_ITEM.split(',').map((s) => s.trim()).filter(Boolean),
    listSelectors: SEL.MSG_LIST.split(',').map((s) => s.trim()).filter(Boolean),
    textSelectors: SEL.TEXT.split(',').map((s) => s.trim()).filter(Boolean),
    inputSelectors: SEL.INPUT.split(',').map((s) => s.trim()).filter(Boolean),
    sendSelectors: SEL.SEND.split(',').map((s) => s.trim()).filter(Boolean),
    selfMarkers: SEL.SELF_ITEM.split(',').map((s) => s.trim()).filter(Boolean),
    otherMarkers: SEL.OTHER_ITEM.split(',').map((s) => s.trim()).filter(Boolean),
  };
  return mergeSelectors(CHANNELS.XHS, location.host, fb);
}

// —— 真实选择器（XHS-YYDS 已校验 + 改版兜底候选）——
// 小红书网页 IM 结构：
//   - 会话列表 .sx-contact-item（虚拟滚动 .vue-recycle-scroller）
//   - 消息项 .im-msg-item（内含 .left/.right + .text-message + [data-msg-type]）
//   - 输入框 #jarvis-reply-textarea（textarea）
//   - 发送 .send_btn
// 多候选：改版/换肤时旧 class 失效，任一候选命中即用。
export const SEL = {
  // 左侧会话列表：新版 .xhs-im-conv-item（data-conv-id）；旧版 .sx-contact-item
  CHAT_LIST: '.xhs-im-conv-item, .sx-contact-item, .chat-list-box [class*="contact-item"], [class*="xhs-im-conv-list"] [class*="conv-item"]',
  // 消息线程容器（新版 .xhs-im-msg-list-wrap/.xhs-im-msg-list；旧版 im-chat-window/vue-recycle）
  MSG_LIST: '.xhs-im-msg-list-wrap, .xhs-im-msg-list, .vue-recycle-scroller.ready, [class*="chat-content"], [class*="message-list"], [class*="MessageList"], [class*="chat-window"], [class*="ChatWindow"], [class*="im-chat"], [class*="xhs-im-chat-window"]',
  // 消息气泡：真实为 .chat-item（data-message-id/data-content-type，对方 --left/--other）
  // 兼容旧版 .im-msg-item 与多版变体
  MSG_ITEM: '.chat-item, [class*="chat-item"], [class*="chatItem"], [class*="ChatItem"], .im-msg-item, [class*="msg-item"], [class*="MsgItem"], [class*="message-item"], [class*="MessageItem"], [class*="im-msg"], [class*="chatMsg"], [class*="bubble"], [class*="Bubble"]',
  // 自/他标记：新版 --right(自己)/--left(对方)、--self/--other；旧版 .right/.left
  SELF_ITEM: '.chat-item__content--right, .chat-item__bubble--self, .chat-item__bubble--mine, [class*="chat-item"][class*="--right"], [class*="--self"], .right, [class*="self"], [class*="outgoing"], [class*="right-message"]',
  OTHER_ITEM: '.chat-item__content--left, .chat-item__bubble--other, [class*="chat-item"][class*="--left"], [class*="--other"], .left, [class*="other"], [class*="incoming"], [class*="left-message"]',
  // 未读新消息标记（巡检用：左侧会话列表项上的红点/未读 badge）。弹性多候选，任一命中即视为有新消息。
  UNREAD: '[class*="unread" i], [class*="Unread" i], [class*="red-dot" i], [class*="reddot" i], [class*="redDot" i], [class*="new-msg" i], [class*="newMsg" i], [class*="new_message" i], [data-unread="1"], [data-unread="true"], [class*="badge" i][class*="msg" i]',
  // 气泡文本：新版 .xhs-im-bubble__text（p 标签）；旧版 .text-message
  TEXT: '.xhs-im-bubble__text, .text-message, [class*="text-message"], [class*="textMessage"], [class*="msg-content"], [class*="message-content"]',
  // 严格输入框：真实小红书新版为 div.xhs-im-input-bar-editor[contenteditable]
  // （旧版 #jarvis-reply-textarea 已废弃，须兼容两者）
  INPUT: '#jarvis-reply-textarea, div.xhs-im-input-bar-editor[contenteditable="true"], [class*="xhs-im-input-bar-editor"]',
  // 发送：新版 .xhs-im-input-bar-actions 内按钮 + 回车兜底；旧版 .send_btn
  SEND: '.xhs-im-input-bar-actions [class*="action-btn"][class*="send"], [class*="xhs-im-send"], [class*="xhs-im-msg-send"], [class*="xhs-im-input-bar-send"], .send_btn, [class*="send-btn"], [class*="sendBtn"], [class*="send-button"], button[class*="send"], [aria-label*="发送"]',
  // 消息类型：小红书气泡上的 data-msg-type（TEXT/IMAGE/CARD/SPOTLIGHT/VOICE...）
  MSG_TYPE: '[data-msg-type], [data-content-type]',
  // 笔记卡片消息（XHS-YYDS card_container / card_bottom_title / card_bottom_info）
  CARD: '.card_container, [class*="card-container"], [class*="CardContainer"]',
  CARD_TITLE: '.card_bottom_title, [class*="card-title"], [class*="CardTitle"]',
  CARD_INFO: '.card_bottom_info, [class*="card-info"], [class*="CardInfo"]',
  // 聚光进线（系统消息）：XHS-YYDS source-tip
  SPOTLIGHT: '.source-tip',
  // 聊天对象昵称（右侧聊天页顶部 header 内的对方/群名称元素）。
  // 需求③/修复：1v1 私信时把对方昵称作为「发件人」（sender_name）上报，而非空串/字符串 hash。
  // 新版小红书 /chat 页聊天 header 顶部标题元素（.xhs-im-chat-title / .chat-title / .title）；
  // 旧版 .im-chat-window 内 [class*="name"]。多候选弹性，任一命中即视为对方昵称。
  PEER_NAME: '.xhs-im-chat-title, .xhs-im-chat-header__title, [class*="xhs-im-chat-title"], [class*="xhs-im-chat-header"] [class*="title"], [class*="chat-window"] [class*="header"] [class*="title"], [class*="chat-window"] [class*="header"] [class*="name"], [class*="chat-content"] [class*="header"] [class*="name"], .im-chat-window [class*="header"] [class*="name"], .im-chat-window [class*="title"], [class*="chat-header"] [class*="name"]',
};

// —— 输入框候选 ——
// STRICT_INPUT_SELECTORS：仅平台严格特征（用于 matchMode strict 判定 / 页面识别）。
// INPUT_FALLBACKS：发送路径的弹性候选（改版/换肤兜底），不参与 matchMode 判定，
// 避免「任意 textarea 命中就误判为 strict」导致 fallback 降级失效。
// 新版小红书输入框为 div.xhs-im-input-bar-editor[contenteditable]（非 textarea）。
const STRICT_INPUT_SELECTORS = [
  '#jarvis-reply-textarea',
  'div.xhs-im-input-bar-editor[contenteditable="true"]',
  '[class*="xhs-im-input-bar-editor"]',
];
const INPUT_FALLBACKS = [
  '#jarvis-reply-textarea',
  'div.xhs-im-input-bar-editor[contenteditable="true"]',
  '[class*="xhs-im-input-bar-editor"]',
  'textarea[placeholder*="发消息"]',
  'textarea[placeholder*="回复"]',
  'textarea[placeholder*="消息"]',
  'textarea[aria-label*="回复"], textarea[aria-label*="发消息"]',
  '[role="textbox"]',
];

function strictInputEl() {
  for (const sel of STRICT_INPUT_SELECTORS) {
    try {
      const el = qs(sel);
      if (el) return el;
    } catch (_) { /* 非法选择器跳过 */ }
  }
  return null;
}

// 小红书聊天页 URL：https://www.xiaohongshu.com/chat（用户确认的真实私信页入口）。
// SPA 面板未渲染时结构命中可能为 false，URL 命中最先判定，避免等待轮询。
function isXhsChatUrl() {
  const path = (location.pathname || '');
  return path === '/chat' || path.startsWith('/chat/');
}

// 小红书联系人 id 规范化：去除常见前缀、清理非法字符（XHS-YYDS normalizeContactId 同款）。
function normalizeContactId(id) {
  if (id == null) return '';
  let s = String(id).trim();
  if (!s) return '';
  const prefixes = ['Total-', 'Active-', 'User-', 'Contact-', 'User_', 'Contact_'];
  for (const prefix of prefixes) {
    if (s.startsWith(prefix)) {
      s = s.substring(prefix.length);
      break; // 只移除一个前缀
    }
  }
  s = s.replace(/[^a-zA-Z0-9_-]/g, '');
  return s;
}

// 当前登录的小红书账号 id。
// 多层兜底：侧边栏/header「我的」主页链接 → 任意 profile 链接 → 任意 /user/ 链接 →
// localStorage 缓存（同一账号浮层/改版取不到也可恢复）→ 稳定 unknown（绝不空串）。
// 空串会导致 WS 握手持空 account_id → 服务端 401 → 历史/实时全不上行。
function getAccountId() {
  const candidates = [];
  // 1) 「我的」主页链接（侧边栏/header/nav，最可靠）
  qsa('aside a[href*="/user/profile/"], header a[href*="/user/profile/"], nav a[href*="/user/profile/"]').forEach((a) => {
    const m = a.getAttribute('href')?.match(/\/user\/profile\/([\w.-]+)/);
    if (m && m[1] && m[1] !== 'self') candidates.push(m[1]);
  });
  // 2) 任意 profile 链接（私信页 header 常含当前账号头像链接）
  qsa('a[href*="/user/profile/"]').forEach((a) => {
    const m = a.getAttribute('href')?.match(/\/user\/profile\/([\w.-]+)/);
    if (m && m[1] && m[1] !== 'self') candidates.push(m[1]);
  });
  // 3) 任意 /user/ 链接（最后手段）
  qsa('a[href*="/user/"]').forEach((a) => {
    const m = a.getAttribute('href')?.match(/\/user\/(?:profile\/)?([\w.-]+)/);
    if (m && m[1] && m[1] !== 'self') candidates.push(m[1]);
  });
  for (const c of candidates) {
    if (c && c.trim()) {
      // 同步写 localStorage（页面内同步可用，刷新后仍可兜底；不依赖异步 chrome.storage）
      try { localStorage.setItem(`hivebridge:account:${CHANNELS.XHS}`, c.trim()); } catch (e) { /* noop */ }
      return c.trim();
    }
  }
  // 4) 同步兜底：localStorage 缓存（页面结构临时取不到真实 id 时复用本页曾取到的值）
  try {
    const ls = localStorage.getItem(`hivebridge:account:${CHANNELS.XHS}`);
    if (ls) return ls;
  } catch (e) { /* noop */ }
  // 5) 稳定 unknown（绝不返回空串，避免 WS 握手持空 account_id → 服务端 401 → 全链路断）
  return `${CHANNELS.XHS}-unknown`;
}

// 当前会话 id（小红书的会话 id 多为联系人的 user id）。
// 兜底链：活动会话项 data-conv-id（新版最权威）→ URL /chat/{id} 路径 →
//        ?conversation_id → 旧版活动项 data-* → header 对方链接（排除自己）→ 昵称派生。
function getConversationId() {
  // 1) 活动会话项：新版 .xhs-im-conv-item--active[data-conv-id]（真实 DOM 实测），
  //    旧版 .active / [aria-selected="true"]。data-conv-id 是会话聚合键，最权威。
  const active =
    qsa(SEL.CHAT_LIST).find(
      (el) =>
        el.classList.contains('xhs-im-conv-item--active') ||
        el.classList.contains('active') ||
        el.getAttribute('aria-selected') === 'true'
    );
  if (active) {
    const convId =
      active.getAttribute('data-conv-id') ||
      active.getAttribute('data-key') ||
      active.getAttribute('data-id') ||
      active.getAttribute('data-contactusemid') ||
      active.id ||
      active.getAttribute('data-uid');
    const norm = normalizeContactId(convId);
    if (norm) return norm;
  }
  // 2) 新版小红书聊天页 URL 形如 https://www.xiaohongshu.com/chat/{conversationId}
  //    （实测：https://www.xiaohongshu.com/chat/5e4f75e30000000001001cd8）。
  //    会话 id 直接编码在路径里——不解析它 getConversationId() 返回 null，适配器守卫
  //    会拦截全部消息（表现：会话(空) + 消息零上行）。故路径解析兜底必须存在。
  const pathMatch = (location.pathname || '').match(/\/chat\/([^/?#]+)/);
  if (pathMatch && pathMatch[1]) {
    const norm = normalizeContactId(pathMatch[1]);
    if (norm) return norm;
  }
  if (/[?&]conversation_id=/.test(location.href)) {
    return new URLSearchParams(location.search).get('conversation_id');
  }
  // 聊天 header 用户链接（对方主页链接即会话标识）。
  // 关键：新版 /chat 页 header 第一个 /user/profile/ 链接常是「我」自己（账号线索），
  // 不能当会话 id；须跳过与当前登录账号相同的链接，取「对方」链接。
  const myAccount = getAccountId();
  const headerLinks = qsa('[class*="chat-window"] [class*="header"] a[href*="/user/"], [class*="chat-content"] [class*="header"] a[href*="/user/"], .im-chat-window [class*="header"] a[href*="/user/"], [class*="chat-header"] a[href*="/user/"]');
  for (const headerLink of headerLinks) {
    const href = headerLink.getAttribute('href') || '';
    const m = href.match(/\/user\/profile\/([\w.-]+)/) || href.match(/\/user\/([\w.-]+)/);
    if (!m || !m[1]) continue;
    if (myAccount && m[1] === myAccount) continue; // 跳过「我」自己
    return m[1];
  }
  // 消息容器 data-id
  const container = qs('.im-chat-window [data-id], .im-chat-container [data-id], [class*="chat-content"] [data-id]');
  if (container) {
    const norm = normalizeContactId(container.getAttribute('data-id'));
    if (norm) return norm;
  }
  // 兜底（关键）：小红书 /chat 页的活动会话项（.sx-contact-item.active）在部分版本下
  // 无 data-* 属性、header 无 /user/ 链接时，若返回 null，适配器 `if (!getConversationId()) return`
  // 守卫会拦截全部消息 →「打开私信页一条消息都捕获不到」。用会话项昵称文本派生稳定 id。
  if (active) {
    const nameEl = active.querySelector(
      '[class*="nickname" i], [class*="nick-name" i], [class*="name" i], [class*="title" i], [class*="Title" i]'
    );
    const name = nameEl ? cleanText(nameEl) : cleanText(active);
    if (name) return 'conv:' + name.slice(0, 80);
  }
  return null;
}

// 小红书输入框（容错版）：先 INPUT_FALLBACKS 弹性候选 → findAnyMessageInput 通用扫描
// 用途：sendText 在小红书改版后仍能找到输入框（matchMode 判定用 strictInputEl，与此区分）
function findInputEl() {
  for (const sel of INPUT_FALLBACKS) {
    try {
      const el = qs(sel);
      if (el) return el;
    } catch (_) { /* 非法选择器跳过 */ }
  }
  return findAnyMessageInput();
}

// 小红书发送按钮：优先 merged selectors → SEL.SEND → 通用扫描
function findSendButton() {
  const m = mergedSelectors();
  for (const sel of m.sendSelectors) {
    try {
      const el = qs(sel);
      if (el) return el;
    } catch (_) { /* 非法选择器跳过 */ }
  }
  return qs(SEL.SEND) || qs('button[class*="send"], [class*="send"], [aria-label*="发送"]') || null;
}

// —— 非文字消息提取 ——
// 小红书气泡类型：text | card(笔记卡片) | image | voice | video | link | recall | system(聚光进线)
// data-msg-type 属性为主信号；缺失时按 DOM 结构兜底（头像图绝不当作消息图片）。
function extractMessageContent(item) {
  const typeAttr = (item.getAttribute('data-msg-type') || item.querySelector(SEL.MSG_TYPE)?.getAttribute('data-msg-type') || '').toUpperCase();
  const text = cleanText(item.querySelector(SEL.TEXT) || item);
  // 纯时间戳（如「09:56」「昨天 12:30」）是消息间隔标记，不是消息内容——直接跳过。
  // 注意：空文本（纯图片/语音消息）不能命中此正则——必须有时间戳形态才过滤。
  if (text && /^(\d{1,2}:\d{2}|(昨天|前天)?\s*\d{1,2}月\d{1,2}日?\s*\d{1,2}:\d{2}|\d{4}-\d{2}-\d{2})$/.test(text)) {
    return { msgType: 'text', mediaUrl: '', text: '' };
  }
  // 笔记卡片消息（data-msg-type=CARD 或含 card_container）
  const card = item.querySelector(SEL.CARD);
  if (card || typeAttr === 'CARD') {
    const title = cleanText(card?.querySelector(SEL.CARD_TITLE));
    const info = cleanText(card?.querySelector(SEL.CARD_INFO));
    const img = card?.querySelector('img');
    return { msgType: 'card', mediaUrl: img?.getAttribute('src') || '', text: info || title || '笔记卡片' };
  }
  // 聚光进线（系统消息，带 source-tip 来源提示）
  if (item.querySelector(SEL.SPOTLIGHT) || typeAttr === 'SPOTLIGHT') {
    return { msgType: 'system', mediaUrl: '', text };
  }
  // 撤回
  if (/撤回了?一条消息|撤回了一条|recalled a message/i.test(text)) {
    return { msgType: 'recall', mediaUrl: '', text };
  }
  // 媒体：排除头像图（[class*="avatar"]）——私信每条消息都带发送者头像，不能当消息图片
  const imgs = Array.from(item.querySelectorAll('img')).filter(
    (img) => !(img.closest && img.closest('[class*="avatar"], [class*="Avatar"]'))
  );
  const vids = item.querySelectorAll('video, [class*="video"], [class*="Video"]');
  const audio = item.querySelectorAll('[class*="voice"], [class*="Voice"], [class*="audio"], [class*="Audio"]');
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

// —— 聊天对象昵称（发件人 / 对方昵称）——
// 需求③/修复：1v1 私信「一个会话=一条消息」需把对方昵称作为系统客户名称/发件人。
// 从右侧聊天页上方 header 内的标题元素（含对方昵称或群名）抽取文本，多层兜底。
// 排除「小红书 Web IM」等通用标题词、排除仅是产品名（如「私信」），保证拿到真正昵称或群名。
function getPeerName() {
  // 1) SEL.PEER_NAME 命中（最可靠，通过 class）
  for (const sel of String(SEL.PEER_NAME || '').split(',').map((s) => s.trim()).filter(Boolean)) {
    try {
      const el = qs(sel);
      if (el) {
        const t = cleanText(el);
        if (t && !/私信|消息|小红书|聊天|contact|会话/i.test(t)) return t;
      }
    } catch (_) { /* 非法选择器忽略 */ }
  }
  // 2) 聊天 header 内任意带 /user/profile/ 的链接（对方主页链接的可见文字=昵称），排除「我」
  const myAccount = getAccountId();
  const headerLinks = qsa(
    '[class*="chat-window"] [class*="header"] a[href*="/user/"], [class*="chat-content"] [class*="header"] a[href*="/user/"], .im-chat-window [class*="header"] a[href*="/user/"], [class*="chat-header"] a[href*="/user/"]'
  );
  for (const link of headerLinks) {
    const href = link.getAttribute('href') || '';
    const m = href.match(/\/user\/(?:profile\/)?([\w.-]+)/);
    if (!m || !m[1]) continue;
    if (myAccount && m[1] === myAccount) continue; // 跳过「我」自己
    const t = cleanText(link);
    if (t && !/私信|消息|小红书|聊天/i.test(t)) return t;
  }
  // 3) 活动会话列表项里的昵称元素（与 getConversationId 兜底同源，适合 header 取不到场景）
  const active =
    qsa(SEL.CHAT_LIST).find(
      (el) =>
        el.classList.contains('xhs-im-conv-item--active') ||
        el.classList.contains('active') ||
        el.getAttribute('aria-selected') === 'true'
    );
  if (active) {
    const nameEl = active.querySelector(
      '[class*="nickname" i], [class*="nick-name" i], [class*="name" i], [class*="title" i], [class*="Title" i]'
    );
    const t = nameEl ? cleanText(nameEl) : cleanText(active);
    if (t && !/私信|消息|小红书|聊天/i.test(t)) return t;
  }
  return '';
}

// —— 群聊识别 ——
// 小红书群聊特征：① 聊天 header 标题含「群」；② 消息内「@昵称」前缀；③ URL 群 id。
function detectGroup(item) {
  let isGroup = false;
  let groupId = '';
  let groupName = '';
  let senderName = '';
  // 【修复】chat header 文本优先用 getPeerName()（已抽好 header 标题），避免重复合并 selector
  const headerText = getPeerName() || '';
  if (headerText && /群|团队|家族|小组|team|group/i.test(headerText)) {
    isGroup = true;
    groupName = headerText;
  }
  const inner = cleanText(item);
  const atMatch = inner.match(/^@([^\s,，：:]+)[\s,，：:]/);
  if (atMatch) {
    isGroup = true;
    senderName = atMatch[1];
  }
  const gid = (location.href.match(/[?&](group|conversation)_id=([^&]+)/) || [])[2];
  if (gid) {
    groupId = gid;
    isGroup = true;
  }
  return { isGroup, groupId, groupName, senderName };
}

// —— 噪音过滤 ——
// 会话列表项（新版 .xhs-im-conv-item / 旧版 .sx-contact-item）及其内部节点、
// 聊天侧栏卡片绝不应当作消息内容上行。
function isListNoise(item) {
  if (!item || !item.closest) return false;
  if (item.closest('.xhs-im-conv-item')) return true;   // 新版会话项
  if (item.closest('.sx-contact-item')) return true;     // 旧版会话项
  if (item.closest('.xhs-im-conv-list')) return true;    // 新版会话列表容器
  if (item.closest('.chat-list-box')) return true;
  if (item.closest(SEL.CHAT_LIST)) {
    if (item.matches('[class*="conv-item"], [class*="contact"]') || item.querySelector('[class*="conv-item"], [class*="contact"]')) return true;
  }
  return false;
}

// 小红书 IM 结构特征（新版 .chat-item 气泡 / 旧版 .im-msg-item）：
//   - 输入框 .xhs-im-input-bar-editor / #jarvis-reply-textarea
//   - 消息项 .chat-item / .im-msg-item；会话项 .xhs-im-conv-item / .sx-contact-item
// 私信常为 SPA 面板/浮层，URL 可能不切 /message，故以结构命中为主。
function isXhsMessagePage() {
  const hasInput = !!strictInputEl();
  const hasMsg = !!document.querySelector(
    '.chat-item, .im-msg-item, .im-chat-window, [class*="msg-list"], [class*="chat-content"], [class*="message-list"], [class*="chat-list"]'
  );
  if (hasInput && hasMsg) return true;
  const hasContact = !!document.querySelector('.sx-contact-item, .xhs-im-conv-item');
  if (hasContact && hasInput) return true;
  return false;
}

// 会话列表枚举（供适配器「遍历所有私信」全量同步）：
// 返回 [{ id, name, el }]，id 取自会话项的 data-key/data-id/data-contactusemid/id/data-uid（规范化后），
// el 为可点击打开该会话的列表项 DOM。遍历器逐个点击 el → 打开线程 → 回填历史。
function getConversationList() {
  // 用户自定义「会话列表项」选择器优先（人工识别 HTML 填写即生效）；无则用默认 SEL.CHAT_LIST
  const customSels = customConversationListSelectors(CHANNELS.XHS);
  let items = [];
  if (customSels.length) {
    for (const sel of customSels) {
      try {
        items = items.concat(qsa(sel));
      } catch (_) { /* 非法选择器跳过 */ }
    }
  }
  if (!items.length) items = qsa(SEL.CHAT_LIST);
  if (!items.length) return [];
  const out = [];
  const ids = new Set();
  for (const item of items) {
    if (!item || !item.offsetParent) continue; // 跳过不可见（虚拟列表回收节点）
    // 新版 .xhs-im-conv-item 用 data-conv-id（真实 DOM 实测）；旧版 .sx-contact-item 用 data-key/data-id
    const raw =
      item.getAttribute('data-conv-id') ||
      item.getAttribute('data-key') ||
      item.getAttribute('data-id') ||
      item.getAttribute('data-contactusemid') ||
      item.id ||
      item.getAttribute('data-uid');
    let id = normalizeContactId(raw);
    // 昵称：会话项内昵称/标题元素
    const nameEl = item.querySelector(
      '[class*="nickname" i], [class*="nick-name" i], [class*="name" i], [class*="title" i], [class*="Title" i]'
    );
    const name = nameEl ? cleanText(nameEl) : '';
    // 无 data 属性时用昵称文本派生稳定 id（/chat 页会话项可能无 data-*）
    if (!id) {
      if (name) id = 'conv:' + name.slice(0, 80);
      else continue; // 无稳定 id 的节点不计入（避免重复/乱序）
    }
    if (ids.has(id)) continue;
    ids.add(id);
    // 巡检用：是否标记为有未读新消息（红点/未读 badge）。用于 patrol 优先巡检有新消息的会话。
    const unread = detectUnread(item);
    out.push({ id, name, el: item, unread });
  }
  return out;
}

// 未读检测：会话项自身带未读 class，或其内部含未读红点/badge，即视为有新消息。
function detectUnread(item) {
  if (!item) return false;
  try {
    if (item.matches && item.matches(SEL.UNREAD)) return true;
    if (item.querySelector && item.querySelector(SEL.UNREAD)) return true;
  } catch (_) { /* 非法选择器忽略 */ }
  return false;
}

const hooks = {
  match() {
    if (!location.hostname.includes('xiaohongshu.com')) return false;
    if (isXhsChatUrl()) return true;         // https://www.xiaohongshu.com/chat 聊天页
    if (isXhsMessagePage()) return true;     // 结构命中（含 SPA 浮层/面板）
    if (qs(SEL.INPUT)) return true;          // XHS-YYDS 严格同款兜底
    if (findAnyMessageInput() && looksLikeMessagePage()) {
      log.warn('match() 走 fallback 模式：小红书 IM 严格选择器已失效，使用通用 DOM 扫描');
      return true;
    }
    return false;
  },
  matchMode() {
    if (!location.hostname.includes('xiaohongshu.com')) return null;
    if (isXhsChatUrl() || isXhsMessagePage() || qs(SEL.INPUT)) return 'strict';
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
    return qs(SEL.MSG_LIST) || qs('.im-chat-window') || qs('[class*="chat-content"]') || qs('[class*="message-list"]') || null;
  },
  getMessageItems() {
    // 主路径：优先用云端 LLM 生成的选择器（缓存），回退 SEL 多候选；
    // 再由 SelectorEngine 结构启发式定位消息列表与消息项。
    const m = mergedSelectors();
    const { items } = SelectorEngine.locateMessages({
      root: document,
      itemSelectors: m.itemSelectors,
      listSelectors: m.listSelectors,
    });
    const filtered = items.filter((it) => !isListNoise(it));
    if (filtered.length) return filtered;
    // 兜底：小红书消息气泡 class 多变时，用「消息线程容器内的文本叶子」推断气泡
    const thread = this.getMessageListRoot();
    if (!thread) return [];
    const leafText = [];
    const walk = (el, depth) => {
      if (depth > 3) return;
      for (const child of el.children) {
        const txt = cleanText(child);
        if (txt && child.offsetParent !== null && !child.querySelector('.sx-contact-item') && !child.querySelector(SEL.INPUT)) {
          leafText.push(child);
        } else {
          walk(child, depth + 1);
        }
      }
    };
    walk(thread, 0);
    return leafText.filter((el) => !isListNoise(el));
  },
  parseMessageItem(item) {
    // 剔除会话列表节点 / 联系人卡片（它们可能被宽泛选择器命中）
    if (isListNoise(item)) return null;
    // 先判定消息类型（卡片/图片/语音/视频/撤回/系统）
    const { msgType, mediaUrl, text } = extractMessageContent(item);
    if (!text && msgType === 'text') return null; // 纯文本且无内容则跳过
    // 自/他判定：.left/.right class 为主，AI 生成的 self/other 标记兜底修正
    let sender_type = isSelfMessage(item, SEL.SELF_ITEM) ? SENDER.SELF : SENDER.CUSTOMER;
    const m = mergedSelectors();
    const matchesSelf = m.selfMarkers.some((sel) => {
      try { return (item.matches && item.matches(sel)) || (item.closest && item.closest(sel)); } catch (_) { return false; }
    });
    const matchesOther = m.otherMarkers.some((sel) => {
      try { return (item.matches && item.matches(sel)) || (item.closest && item.closest(sel)); } catch (_) { return false; }
    });
    if (sender_type === SENDER.CUSTOMER && matchesSelf) sender_type = SENDER.SELF;
    if (sender_type === SENDER.SELF && matchesOther) sender_type = SENDER.CUSTOMER;
    // 群聊识别
    const groupInfo = detectGroup(item);
    // 发件人/对方昵称（需求③，修复 1v1 私信发件人为空/字符串 hash）：
    //   - 群聊里有 @  前缀的成员昵称 → 用它（groupInfo.senderName）
    //   - 群聊无 @ 时 → sender_name 留空（成员名通常显示在每条气泡上方，本字段不强行抓取）
    //   - 1v1 私信客户消息 → 对方昵称（getPeerName() 抓自聊天 header class）
    //   - 1v1 私信自己消息 → 不需要 sender_name（AI/客服）→ 留空（服务端按 account_id 落地）
    let senderName = groupInfo.senderName || '';
    if (!senderName && !groupInfo.isGroup && sender_type === SENDER.CUSTOMER) {
      senderName = getPeerName();
    }
    // 消息 id：新版 .chat-item 用 data-message-id（唯一幂等键）；旧版 data-id/data-msg-id
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
  // 读取主路径：运行 LLM 生成的可执行 JS 抽取器（彻底不依赖固定选择器）。
  extractMessages() { return runExtractor(CHANNELS.XHS, location.host); },
  async sendText(text) {
    const input = findInputEl();
    if (!input) {
      log.error('未找到小红书输入框（merged + strict + fallback 均失败）');
      throw new Error('xhs input not found');
    }
    // 新版输入框为 div.xhs-im-input-bar-editor[contenteditable]（非 textarea）——
    // 用 execCommand insertText + input 事件（抖音同款 fillContentEditable），
    // 兼容 contenteditable 受控组件；旧版 textarea 走 setValue 兜底。
    if (input.isContentEditable || input.getAttribute('contenteditable') === 'true' || input.tagName !== 'TEXTAREA') {
      fillContentEditable(input, text);
    } else {
      setValue(input, text);
    }
    await new Promise((r) => setTimeout(r, 180));
    const sendBtn = findSendButton();
    if (sendBtn) {
      // XHS-YYDS enhancedClickWithVerification 同款：native click + 补充鼠标事件
      enhancedClick(sendBtn);
      return;
    }
    // 新版 /chat 页发送按钮常为输入后出现的图标或回车发送：无按钮时按 Enter 兜底
    log.warn('未找到小红书发送按钮，改用回车发送');
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true, cancelable: true }));
    input.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true, cancelable: true }));
  },
};

// 导出供单测验证账号/会话列表/规范化/页面识别/对方昵称
export { getAccountId, getConversationId, getConversationList, normalizeContactId, isXhsMessagePage, isXhsChatUrl, getPeerName };

export function buildXhsAdapter() {
  return new BaseAdapter({
    name: 'xhs',
    channel: CHANNELS.XHS,
    SEL,
    hooks,
    rateLimiter: undefined,
  });
}
