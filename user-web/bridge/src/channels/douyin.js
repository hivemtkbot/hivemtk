// 抖音网页私信适配器 —— 纯 CSS 选择器架构。
// 设计原则：所有选择器均基于真实 DOM 验证，不使用 LLM 动态生成。
// 平台改版时通过 UI 配置面板(chrome.storage)更新选择器即可，无需发版。
//
// 参考 DOM 结构（2026-08 验证）：
//   - 私信页入口：https://www.douyin.com/chat
//   - 会话列表：左侧 island #island_b69f5，项含 data-e2e="conversation-item"
//   - 消息列表：线程容器含 class*="messageList" / data-e2e="chat-msg-list"
//   - 消息气泡：div[data-e2e="msg-item-content"]（新版）/ class*="messageMessageItem"（/chat路由）
//   - 输入框：div.messageEditorinputArea.editor-kit-container（contenteditable）
//   - 发送按钮：svg[class*="e2e-send-msg-btn"]
//   - 自/他判定已移交后端（服务端权威），前端只负责抽取文本/系统消息。
import { BaseAdapter } from '../core/channel-adapter.js';
import { CHANNELS, SENDER } from '../core/types.js';
import { qs, qsa, cleanText, simulateRealClick, fillContentEditable, createLogger, findAnyMessageInput, looksLikeMessagePage, sanitizePeerName } from '../core/dom.js';
import { SelectorEngine } from '../core/selector-engine.js';
import { mergeSelectors, customConversationListSelectors } from '../core/selector-ai.js';
import { FRONTEND_DEFAULT_SENDER_TYPE } from '../core/fallback.js';

const log = createLogger('douyin', CHANNELS.DOUYIN);

// 合并选择器：用户配置优先（chrome.storage），SEL 默认兜底。
// UI 选择器配置面板修改后，mergeSelectors 自动优先使用用户配置值。
function mergedSelectors() {
  const fb = {
    itemSelectors: SEL.MSG_ITEM.split(',').map((s) => s.trim()).filter(Boolean),
    listSelectors: SEL.MSG_LIST.split(',').map((s) => s.trim()).filter(Boolean),
    textSelectors: SEL.TEXT.split(',').map((s) => s.trim()).filter(Boolean),
    inputSelectors: SEL.INPUT.split(',').map((s) => s.trim()).filter(Boolean),
    sendSelectors: SEL.SEND.split(',').map((s) => s.trim()).filter(Boolean),
  };
  return mergeSelectors(CHANNELS.DOUYIN, fb);
}

// —— 选择器定义（2026-08 验证可用）——
// 每个字段的候选按优先级排列。⚠️ [class*="..."] 均带 i 标志（大小写不敏感），
// 因为平台常用 CSS module 大写类名（如 Unread / ConversationItem 等）。
// 平台改版导致选择器失效时，通过 UI 配置面板覆盖对应字段即可，无需修改代码。
export const SEL = {
  // 左侧会话列表容器
  CHAT_LIST: '#island_b69f5, [data-e2e="conversation-list"]',
  // 消息线程容器（MutationObserver 根）
  MSG_LIST: '[class*="messageList" i], [data-e2e="chat-msg-list"], [class*="chat-content" i]',
  // 消息气泡（2026-08 验证）
  // 抖音最新结构实测：
  //   - 自方气泡：div[data-e2e="msg-item-content"][class*="chatMessageItemSelf" i]
  //   - 对方气泡：div[data-e2e="msg-item-content"][class*="chatMessageItemOther" i]
  //   - 旧版：[class*="messageMessageItem" i]（已不带自他标识）
  // 注意：去掉旧版 [class*="bubble" i] —— 太宽泛，会命中内层 bubble-body 节点，
  // 被 SelectorEngine.containsAnother 滤掉、且被 isSystemMessage 结构特征误判。
  // 自/他判定已移交后端，前端不依据这些 class 区分自己/对方。
  MSG_ITEM: 'div[data-e2e="msg-item-content"], [class*="chatMessageItemSelf" i], [class*="chatMessageItemOther" i], [class*="messageMessageItem" i], [class*="chatMessageItem" i], [class*="msg-content" i]',
  // 聊天气泡内文本
  TEXT: '[data-e2e="msg-item-content"], [class*="msg-content" i]',
  // 输入框（严格：同时含 messageEditorinputArea + editor-kit-container 两 class）
  INPUT: 'div.messageEditorinputArea.editor-kit-container, div[contenteditable="true"][role="textbox"]',
  // 发送按钮（svg 形式，需回溯到最近可交互祖先）
  SEND: '[class*="e2e-send-msg-btn" i], button[aria-label*="发送"]',
  // 消息类型 data 属性
  MSG_TYPE: '[data-msg-type], [data-message-type]',
  // 卡片消息（商品/作品）
  CARD: '[class*="card-container" i], [class*="goods-card" i]',
  // 会话列表项（遍历用，与 CHAT_LIST 容器区分）
  CONV_ITEM: '[data-e2e="conversation-item"], [class*="conversation-item" i], [class*="conv-item" i]',
  // 系统消息 class 关键词（配合内容模式双重识别）
  SYSTEM: '[class*="system-msg" i], [class*="notice" i], [class*="divider" i], [class*="time-stamp" i], [class*="recalled" i]',
  // 聊天对象昵称（右侧 header 标题元素）
  PEER_NAME: '[data-e2e="chat-header-title"], [class*="chat-header" i] [class*="title" i]',
  // 未读红点标记（巡检用）⚠️ 必须带 i 标志，否则 Unread 失配导致 patrol 全静默
  UNREAD: '[class*="unread" i], [class*="red-dot" i], [data-unread="1"]',
  // 输入框（兼容旧引用）
  EDITOR: 'div.messageEditorinputArea.editor-kit-container, div[contenteditable="true"][role="textbox"]',
};

// 抖音输入框（strict）：优先用户配置选择器 → SEL 默认
function strictEditorBox() {
  const m = mergedSelectors();
  for (const sel of m.inputSelectors) {
    try {
      const el = qs(sel);
      if (el) return el;
    } catch (_) { /* 非法选择器跳过 */ }
  }
  return qs(SEL.INPUT) || qs(SEL.EDITOR);
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

// 抖音发送按钮：优先用户配置选择器 → SEL 默认 → 红色路径兜底
function getRealSendButton() {
  const m = mergedSelectors();
  for (const sel of m.sendSelectors) {
    try {
      const el = qs(sel);
      if (el) {
        if (el.tagName === 'SVG') return el.closest('button, [role="button"], div, span') || el;
        return el;
      }
    } catch (_) { /* 非法选择器跳过 */ }
  }
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

// 自/他判定已移交后端（统一 customer 占位，回环防护由后端承担）。

// 当前登录的抖音账号 id。
// 多层兜底：浮层（/jingxuan 等）下左导航/header 的个人链接常缺失或指向 /user/self（占位），
// 导致返回空串 → WS 握手持空 account_id → 服务端 401 拒绝 → 历史/实时全不上行。
// 故优先取「会话项里的真实用户链接」（含 MS4w... token 形式 id），其次「我的」链接，
// 再次 chrome.storage 缓存（同一账号即使浮层取不到也可恢复），最后稳定 unknown 而非空串。
function getAccountId() {
  const candidates = [];
  // 1) 会话列表项里的真实用户链接（最可靠，含 token 形式 id）
  //    注意：SEL.CHAT_LIST 含逗号，直接拼接后代选择器只有最后一项生效——
  //    故拆成独立候选逐项 query（避免 /chat 页漏取账号）。
  const convLinkSelectors = [
    '#island_b69f5 a[href*="/user/"]',
    '[data-e2e="conversation-list"] a[href*="/user/"]',
    '[class*="conversation-list"] a[href*="/user/"]',
    '[class*="ConversationList"] a[href*="/user/"]',
    '[class*="conversation-item"] a[href*="/user/"]',
    '[class*="ConversationItem"] a[href*="/user/"]',
  ];
  for (const sel of convLinkSelectors) {
    try {
      qsa(sel).forEach((a) => {
        const m = a.getAttribute('href')?.match(/\/user\/([^/?#]+)/);
        if (m && m[1] && m[1] !== 'self') candidates.push(m[1]);
      });
    } catch (_) { /* 非法选择器跳过 */ }
  }
  // 2) 左导航/header 「我的」链接
  const self = qs('aside a[href*="/user/"], header a[href*="/user/"]') || qs('a[href*="/user/"]');
  if (self) {
    const m = self.getAttribute('href')?.match(/\/user\/([^/?#]+)/);
    if (m && m[1] && m[1] !== 'self') candidates.push(m[1]);
  }
  // 3) 任意 /user/ 链接（最后手段）
  qsa('a[href*="/user/"]').forEach((a) => {
    const m = a.getAttribute('href')?.match(/\/user\/([^/?#]+)/);
    if (m && m[1] && m[1] !== 'self') candidates.push(m[1]);
  });
  for (const c of candidates) {
    if (c && c.trim()) {
      // 同步写 localStorage（页面内同步可用，刷新后仍可兜底；不依赖异步 chrome.storage）
      try { localStorage.setItem(`hivebridge:account:${CHANNELS.DOUYIN}`, c.trim()); } catch (e) { /* noop */ }
      return c.trim();
    }
  }
  // 4) 同步兜底：localStorage 缓存（浮层取不到真实 id 时，复用本页曾取到的真实 id）
  try {
    const ls = localStorage.getItem(`hivebridge:account:${CHANNELS.DOUYIN}`);
    if (ls) return ls;
  } catch (e) { /* noop */ }
  // 5) 兜底空串（2026-08-14 治本）：旧逻辑回退 `${CHANNELS.DOUYIN}-unknown` 污染后端入库
  //    与按 account_id 关联 outbound 的查询链路，导致 AI 出站回采识别失败、回环拦截失效。
  //    改为空串后，后端层0 改用 (platform + sender_name + content) 三元组命中 outbound，
  //    完全不依赖 account_id。
  return '';
}

// —— 非文字消息提取（问题 3）——
// 抖音气泡可能含图片 / 视频 / 语音 / 表情 / 链接。统一归为 msg_type：
//   text | image | voice | video | emoji | link | recall | system
// 文本优先；含 <img>/视频则记为 image/video（content 留可读描述或 media_url）。
function extractMessageContent(item) {
  const text = cleanText(item.querySelector(SEL.TEXT) || item);
  // 纯时间戳（如「09:56」「昨天 12:30」）是消息间隔标记，不是消息内容——直接跳过
  if (text && /^(\d{1,2}:\d{2}|(昨天|前天)?\s*\d{1,2}月\d{1,2}日?\s*\d{1,2}:\d{2}|\d{4}-\d{2}-\d{2})$/.test(text)) {
    return { msgType: 'text', mediaUrl: '', text: '' };
  }
  const imgs = item.querySelectorAll('img');
  const vids = item.querySelectorAll('video, [class*="video"], [class*="Video"]');
  const audio = item.querySelectorAll('[class*="voice"], [class*="Voice"], [class*="audio"], [class*="Audio"]');
  const emoji = item.querySelectorAll('[class*="emoji"], [class*="Emoji"], [draggable="false"][alt]');
  const links = item.querySelectorAll('a[href*="http"]');
  // 撤回 / 系统消息（如「撤回了一条消息」「你已添加为好友」）
  const sysTxt = text || '';
  if (/撤回了?一条消息|撤回了一条|recalled a message/i.test(sysTxt)) {
    return { msgType: 'recall', mediaUrl: '', text: sysTxt };
  }
  if (/^((你|他|她|对方)?(已?)?(添加|关注|拍了拍|邀请).*|系统|system)/i.test(sysTxt) && !vids.length && !imgs.length) {
    // 宽松系统消息（仅纯文本且无媒体时）
    return { msgType: 'system', mediaUrl: '', text: sysTxt };
  }
  let mediaUrl = '';
  let msgType = 'text';
  if (vids.length) {
    msgType = 'video';
    mediaUrl = vids[0].getAttribute('src') || vids[0].querySelector('video')?.getAttribute('src') || '';
  } else if (imgs.length) {
    msgType = 'image';
    mediaUrl = imgs[0].getAttribute('src') || '';
  } else if (audio.length) {
    msgType = 'voice';
  } else if (emoji.length && !text) {
    msgType = 'emoji';
  } else if (links.length && !text) {
    msgType = 'link';
    mediaUrl = links[0].getAttribute('href') || '';
  }
  return { msgType, mediaUrl, text };
}

// —— 系统消息识别（漏斗 5 层，与 xianyu 同款；抖音没有专属 class 字段，靠内容模式兜底）——
// 命中任一层即视为系统消息，返回 true。parseMessageItem 优先判定，避免触发 AI 误回复。
const SYSTEM_TEXT_PATTERNS = [
  /你(已)?添加.*为好友/,
  /已添加.*为好友/,
  /撤回了?一条消息/,
  /recalled a message/i,
  /对方正在输入/,
  /typing/i,
  /互相关注/,
  /以下为新消息/,
  /^={3,}$/,
  /^-{3,}$/,
  /对方已开启.*验证/,
  /消息发送失败/,
  /消息已发出.*被对方拒收/,
  /我已收到你的消息/,
  /请等待卖家回复/,
];
function isTimeText(text) {
  if (!text) return false;
  const t = text.trim();
  if (/^\d{1,2}:\d{2}$/.test(t)) return true;
  if (/^(今天|昨天|前天)\s*\d{1,2}:\d{2}$/.test(t)) return true;
  if (/^\d{1,2}月\d{1,2}日(\s+\d{1,2}:\d{2})?$/.test(t)) return true;
  if (/^\d{4}[-/]\d{1,2}[-/]\d{1,2}$/.test(t)) return true;
  if (/^(上午|下午|凌晨)\s*\d{1,2}:\d{2}$/.test(t)) return true;
  return false;
}
function isSystemText(text) {
  if (!text) return false;
  const t = text.trim();
  return SYSTEM_TEXT_PATTERNS.some((re) => re.test(t));
}
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
    const hasAvatar = !!item.querySelector('[class*="avatar" i]');
    const hasBubble = !!item.querySelector('[class*="bubble" i], [class*="Bubble" i], [class*="msg-content" i]');
    if (!hasAvatar && !hasBubble) return true;
  }
  return false;
}

// —— 聊天对象昵称（发件人 / 对方昵称）——
// 需求③/修复：1v1 私信「一个会话=一条消息」需把对方昵称作为系统客户名称/发件人。
// 从右侧聊天页上方 header 内的标题元素（含对方昵称或群名）抽取文本，多层兜底。
function getPeerName() {
  // 1) SEL.PEER_NAME 命中（最可靠，通过 class）
  for (const sel of String(SEL.PEER_NAME || '').split(',').map((s) => s.trim()).filter(Boolean)) {
    try {
      const el = qs(sel);
      if (el) {
        const t = cleanText(el);
        if (t && !/私信|消息|抖音|聊天|会话|contact/i.test(t)) return t;
      }
    } catch (_) { /* 非法选择器忽略 */ }
  }
  // 2) 聊天 header 内带 /user/ 链接的可见文字（对方昵称），排除「我」
  const myAccount = getAccountId();
  const headerLinks = qsa('[class*="chat-header"] a[href*="/user/"], [class*="ChatHeader"] a[href*="/user/"]');
  for (const link of headerLinks) {
    const href = link.getAttribute('href') || '';
    const m = href.match(/\/user\/([^/?#]+)/);
    if (!m || !m[1]) continue;
    if (myAccount && m[1] === myAccount) continue; // 跳过「我」自己
    const t = cleanText(link);
    if (t && !/私信|消息|抖音|聊天/i.test(t)) return t;
  }
  // 3) 活动会话列表项里的昵称元素
  const active = qsa(
    '#island_b69f5 [class*="curConversation"], [data-e2e="conversation-list"] [class*="curConversation"], [class*="conversation-list"] [class*="curConversation"], [class*="ConversationList"] [class*="curConversation"], [aria-selected="true"], [class*="curConversation"]'
  ).find((el) => el.offsetParent !== null);
  if (active) {
    const nameEl = active.querySelector(
      '[class*="title" i], [class*="Title" i], [class*="name" i], [class*="nickname" i], [class*="Nickname" i]'
    );
    // sanitizePeerName 剥离会话项里的时间戳/状态徽章等易变后缀
    // （2026-08-07 第十一轮修复：避免 "好吃嘴辰辰 12:31" 这种带时间的脏昵称污染会话 id）
    const raw = nameEl ? cleanText(nameEl) : cleanText(active);
    const t = sanitizePeerName(raw);
    if (t && t !== 'self' && !/私信|消息|抖音|聊天/i.test(t)) return t;
  }
  return '';
}

// —— 群聊识别（问题 2）——
// 抖音群聊特征：① 会话标题非单一用户（群名常含「群」「家族」「同学」等或含多成员）；
// ② 单条消息含「@昵称 」前缀或「[群成员]」结构；③ 一屏内出现多个不同昵称气泡。
// 这里采用「结构 + 文本」双重判定，避免 1v1 误判为群。
function detectGroup(item) {
  let isGroup = false;
  let groupId = '';
  let groupName = '';
  let senderName = '';
  // 【修复】chat header 文本优先用 getPeerName()（已抽好 header 标题），避免重复合并 selector
  const headerText = getPeerName() || '';
  if (headerText && /群|家族|同学|同事|粉丝|俱乐部|team|group/i.test(headerText)) {
    isGroup = true;
    groupName = headerText;
  }
  // 消息内 @ 前缀 / [群成员昵称] 结构
  const inner = cleanText(item);
  const atMatch = inner.match(/^@([^\s,，：:]+)[\s,，：:]/);
  const bracketMatch = inner.match(/^\[([^\]]+)\]/);
  if (atMatch || bracketMatch) {
    isGroup = true;
    senderName = (atMatch && atMatch[1]) || (bracketMatch && bracketMatch[1]) || '';
  }
  // 群 id：URL 含 group/conversation_id 群串
  const gid = (location.href.match(/[?&](group|conversation)_id=([^&]+)/) || [])[2];
  if (gid) {
    groupId = gid;
    isGroup = true;
  }
  return { isGroup, groupId, groupName, senderName };
}

// 当前会话 id：活动会话（深度自检显示活动项含 curConversation class）
function getConversationId() {
  // 群聊 URL 优先取 group/conversation_id
  const fromUrl = (location.href.match(/[?&](group|conversation)_id=([^&]+)/) || [])[2];
  if (fromUrl) return fromUrl;
  if (/[?&]conversation_id=/.test(location.href)) {
    return new URLSearchParams(location.search).get('conversation_id');
  }
  // /chat/{id} 专用路由路径解析（与小红书对称）：抖音 /chat 也可能带会话 id 路径，
  // 如群聊 /chat/{group_id} 或深链 /chat/{sec_uid}。不解析则返回 null → 守卫拦截全部消息。
  const pathMatch = (location.pathname || '').match(/\/chat\/([^/?#]+)/);
  if (pathMatch && pathMatch[1]) {
    return decodeURIComponent(pathMatch[1]);
  }
  const active = qsa(
    '#island_b69f5 [class*="curConversation"], [data-e2e="conversation-list"] [class*="curConversation"], [class*="conversation-list"] [class*="curConversation"], [class*="ConversationList"] [class*="curConversation"], [class*="conversation-item"][class*="curConversation"], [class*="ConversationItem"][class*="curConversation"], [aria-selected="true"], [class*="curConversation"]'
  ).find((el) => el.offsetParent !== null);
  // 群聊：活动项可能含群名而非 /user/ 链接；优先取 data-* 上的会话标识
  const dataConv = active?.getAttribute('data-conversation-id') || active?.getAttribute('data-conv-id') || active?.getAttribute('data-id');
  if (dataConv) return dataConv;
  const link = active?.querySelector('a[href*="/user/"]') || qs('[class*="chat-header"] a[href*="/user/"]');
  // 兼容 /user/<数字id> 与 /user/MS4w...（token 形式）；命中后切换会话会重新回填历史
  const m = link?.getAttribute('href')?.match(/\/user\/([^/?#]+)/);
  if (m && m[1]) return m[1];
  // 兜底（关键）：抖音 /chat 专用路由的活动会话项常无 /user/ 链接、无 data-id 属性
  // （实测 DOM：conversationConversationItemcurConversation[data-e2e="conversation-item"]）。
  // 若此处返回 null，适配器 `if (!getConversationId()) return` 守卫会拦截全部消息，
  // 表现为「打开私信页却一条消息都捕获不到」。故用活动项标题文本派生稳定会话 id：
  // 昵称/群名在同一会话内恒定，足以作为会话聚合键（重名冲突概率低，且优先于完全不捕获）。
  //
  // ⚠️ 2026-08-07 第十一轮修复：必须 sanitizePeerName 剥离会话项内的时间戳/相对时间/
  // 状态徽章等易变后缀，否则 patrol 每次扫描 conversation_id 都不同 → outbound 永远找不到
  // 目标会话 → 大量 pending 永久堆积（实测 287 条）。例：
  //   "好吃嘴辰辰 12:31" → "好吃嘴辰辰"
  //   "AI 修炼场 5 昨天 18:20" → "AI 修炼场 5"
  if (active) {
    const nameEl = active.querySelector(
      '[class*="title" i], [class*="Title" i], [class*="name" i], [class*="nickname" i], [class*="Nickname" i]'
    );
    const raw = nameEl ? cleanText(nameEl) : cleanText(active);
    const name = sanitizePeerName(raw);
    if (name && name !== 'self') return 'conv:' + name.slice(0, 80);
  }
  return null;
}

// 会话列表项（含联系人昵称）不应被当成消息气泡。
// 命中条件：① 位于会话列表容器内；② 内部含指向 /user/ 的个人主页链接
// （真实私信气泡一般不含这类链接，而会话列表行 / 联系人卡片都含）。
// 旧版缺失此过滤，导致私信列表视图下把联系人昵称当成「聊天内容」无限上行
// （表现：conv:null + 钓点王/小马哥不空军/吴小小 等昵称循环）。
//
// ⚠️ 重要修正（解「几十个只上行 2 个」次因）：原 isListNoise 把「文本命中会话列表」
// 的条目一律丢弃，导致大量历史纯文本消息被误杀。新版仅剔除「会话列表节点」与
// 「含个人主页链接的卡片」，不再因文本命中列表而丢弃真实气泡（气泡内也可能含文本）。
function isConversationListNode(el) {
  return !!(el.closest && el.closest(SEL.CHAT_LIST) && (el.matches('[class*="conversation-item"]') || el.matches('[class*="ConversationItem"]') || el.querySelector('[class*="conversation-item"]')));
}
function hasUserProfileLink(el) {
  // 仅当该节点本身是「联系人卡片」（含头像+昵称+链接）而非消息气泡时才剔除。
  // 消息气泡极少同时含 /user/ 链接且自身就是列表行，故加结构约束避免误删。
  const link = el.querySelector && el.querySelector('a[href*="/user/"]');
  if (!link) return false;
  const isCardLike = !!el.querySelector('[class*="avatar" i]') && !!el.querySelector('[class*="name" i], [class*="Nickname" i], [class*="nickname" i]');
  return isCardLike;
}
// 仅剔除明确的会话列表节点 / 联系人卡片；真实消息气泡一律保留（含非文字消息）。
function isListNoise(item) {
  return isConversationListNode(item) || hasUserProfileLink(item);
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

// 会话列表枚举（供适配器「遍历所有私信」全量同步）：
// 返回 [{ id, name, el }]，其中 id 为私信对方的用户标识（/user/<id> 链接或 data 属性），
// el 为可点击打开该会话的列表项 DOM。遍历器逐个点击 el → 打开线程 → 回填历史。
// 仅计入可见且有稳定 id 的会话项（虚拟列表回收节点会被 offsetParent 过滤）。
function getConversationList() {
  const container = qs(SEL.CHAT_LIST);
  // 用户自定义「会话列表项」选择器优先（人工识别 HTML 填写即生效）；无则用默认特征。
  const customSels = customConversationListSelectors(CHANNELS.DOUYIN);
  let items = [];
  if (customSels.length) {
    for (const sel of customSels) {
      try {
        items = items.concat(qsa(sel));
      } catch (_) { /* 非法选择器跳过 */ }
    }
  }
  if (!items.length) {
    // 不依赖 container 存在：/chat 专用路由无 #island_b69f5 island 时，
    // 仍按全局 conversation-item 特征枚举会话项（见下方 items 选择器）。
    // 直接按会话项特征选择（避免 SEL.CHAT_LIST 含逗号时与后代选择器拼接出错）；
    // 仅会话列表项带 conversation-item 类 / data-e2e，消息气泡（message-item）不会命中。
    items = qsa(
      '#island_b69f5 li, [data-e2e="conversation-item"], [class*="conversation-item"], [data-e2e="conversation-list-item"], [class*="conversationListItem"], [class*="ConversationListItem"]'
    );
  }
  // 容器缺失(如 /chat 专用路由无 #island_b69f5)时，上面选择器仍按全局
  // conversation-item 特征命中，故不依赖 container；无命中则回空。
  if (!items.length) return [];
  const out = [];
  const ids = new Set();
  for (const item of items) {
    if (!item || !item.offsetParent) continue; // 跳过不可见（虚拟列表回收节点）
    // 会话 id：优先会话项内 /user/<id> 链接（最可靠）；其次 data-conversation-id / data-sec_uid；
    // 最后用昵称文本派生（/chat 专用路由的会话项常无链接与 data 属性，实测需此兜底）。
    let id = null;
    const link = item.querySelector('a[href*="/user/"]');
    if (link) {
      const m = link.getAttribute('href')?.match(/\/user\/([^/?#]+)/);
      if (m && m[1] && m[1] !== 'self') id = m[1];
    }
    if (!id) id = item.getAttribute('data-conversation-id') || item.getAttribute('data-sec_uid') || item.getAttribute('data-id') || null;
    // 昵称：会话项内的昵称/名称元素（sanitizePeerName 剥离时间戳/状态后缀，
    // 否则同会话不同时刻的 name 不同 → id 不同 → 重复枚举/下行错配）
    const nameEl = item.querySelector(
      '[class*="name" i], [class*="nickname" i], [class*="Nickname" i], [class*="title" i], [class*="Title" i]'
    );
    const name = sanitizePeerName(nameEl ? cleanText(nameEl) : '');
    if (!id) {
      if (name && name !== 'self') id = 'conv:' + name.slice(0, 80);
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
// 红点可能在会话项内（头像角标）或会话项本身加 --unread class。
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
    if (!location.hostname.includes('douyin.com')) return false;
    // 显式优先 /chat 专用聊天路由（会话列表 + 线程同屏，结构最稳定，
    // 适合全量遍历同步私信）：只要存在会话列表项即判定为私信页。
    if (location.pathname.startsWith('/chat')) {
      const hasConvList = !!document.querySelector('[data-e2e="conversation-item"], [class*="conversation-item"], #island_b69f5 li');
      if (hasConvList) return true;
    }
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
    // /chat 专用路由为双栏（左会话列表 + 右消息线程）：editor 所在可滚动祖先
    // 即消息线程面板；island 浮层用 im/message/chat 容器命中。
    const anchor = editorBox() || qs(SEL.CHAT_LIST);
    if (anchor) {
      const sc = closestScrollable(anchor);
      if (sc && sc !== document.body && sc.scrollHeight > sc.clientHeight) return sc;
    }
    return anchor ? anchor.closest('[class*="im"], [class*="message"], [class*="chat"], [class*="Im"], [class*="Message"], [class*="msg"], [class*="Msg"]') : null;
  },
  getMessageItems() {
    // 主路径：用户配置选择器优先 → SEL 默认候选 → SelectorEngine 结构启发式兜底。
    const m = mergedSelectors();
    const { items } = SelectorEngine.locateMessages({
      root: document,
      itemSelectors: m.itemSelectors,
      listSelectors: m.listSelectors,
    });
    const filtered = items.filter((it) => !isListNoise(it) && !isSystemMessage(it));
    if (filtered.length) return filtered;
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
    return leafText.filter((el) => !isListNoise(el) && !isSystemMessage(el));
  },
  parseMessageItem(item) {
    // item 已是消息气泡（文本/媒体元素）。先判定是否系统消息（漏斗 5 层）—— 优先判定，不消耗 AI 配额
    if (isListNoise(item)) return null;
    if (isSystemMessage(item)) {
      const sysText = cleanText(item);
      // 2026-08-05 修复：去掉 Date.now()——同一系统消息每轮扫描生成不同 msg_id
      //   → 后端无法幂等去重 → 不断当新消息入库。系统消息文本本身稳定，无需时间戳后缀。
      return {
        message_id: item.getAttribute('data-id') || item.getAttribute('data-msg-id') || item.id || `sys:${sysText}`,
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
    // 先判定消息类型（问题 3：非文字消息）。
    const { msgType, mediaUrl, text } = extractMessageContent(item);
    if (!text && msgType === 'text') return null; // 纯文本且无内容则跳过
    // 自/他判定已移交后端（统一 customer 占位，回环防护由后端承担）。
    const sender_type = FRONTEND_DEFAULT_SENDER_TYPE;
    // 群聊识别（问题 2）：检测群特征（群标题 / @全员 / 多人昵称前缀）
    const groupInfo = detectGroup(item);
    // 发件人/对方昵称（需求③，修复 1v1 私信发件人为空/字符串 hash）：
    //   - 群聊里有 @  前缀的成员昵称 → 用它（groupInfo.senderName）
    //   - 群聊无 @ 时 → sender_name 留空（成员名通常显示在每条气泡上方）
    //   - 1v1 私信客户消息 → 对方昵称（getPeerName() 抓自聊天 header class）
    //   - 1v1 私信自己消息 → 不需要 sender_name → 留空
    let senderName = groupInfo.senderName || '';
    if (!senderName && !groupInfo.isGroup && sender_type === SENDER.CUSTOMER) {
      senderName = getPeerName();
    }
    // 兜底 msg_id：优先取 DOM 自带 id，否则按文本内容生成确定性 id。
    // 幂等去重（消息ID / 内容hash）已由后端统一负责，前端不再计算内容 hash。
    const mid =
      item.getAttribute('data-id') ||
      item.getAttribute('data-msg-id') ||
      item.getAttribute('id') ||
      `c:${text}`;
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
  mergedSelectors,
  getPeerName,
  async sendText(text) {
    const editor = findInputEl();
    if (!editor) {
      log.error('未找到抖音输入框（strict + fallback 均失败）');
      throw new Error('douyin input not found');
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

// 导出供单测验证（与其他三渠道对齐命名）
export { getAccountId, getConversationId, getConversationList, getPeerName, isListNoise, detectUnread, isSystemMessage, isTimeText, isSystemText, isCenterAligned, isDouyinMessagePage, isConversationListNode, hasUserProfileLink };

export function buildDouyinAdapter() {
  return new BaseAdapter({
    name: 'douyin',
    channel: CHANNELS.DOUYIN,
    SEL,
    hooks,
    rateLimiter: undefined, // 使用默认 RateLimiter
  });
}
