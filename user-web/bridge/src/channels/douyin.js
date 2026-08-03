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
import { SelectorEngine } from '../core/selector-engine.js';

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

// 当前登录的抖音账号 id。
// 多层兜底：浮层（/jingxuan 等）下左导航/header 的个人链接常缺失或指向 /user/self（占位），
// 导致返回空串 → WS 握手持空 account_id → 服务端 401 拒绝 → 历史/实时全不上行。
// 故优先取「会话项里的真实用户链接」（含 MS4w... token 形式 id），其次「我的」链接，
// 再次 chrome.storage 缓存（同一账号即使浮层取不到也可恢复），最后稳定 unknown 而非空串。
function getAccountId() {
  const candidates = [];
  // 1) 会话列表项里的真实用户链接（最可靠，含 token 形式 id）
  qsa(`${SEL.CHAT_LIST} a[href*="/user/"]`).forEach((a) => {
    const m = a.getAttribute('href')?.match(/\/user\/([^/?#]+)/);
    if (m && m[1] && m[1] !== 'self') candidates.push(m[1]);
  });
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
  // 5) 稳定 unknown（绝不返回空串，避免 WS 握手持空 account_id → 服务端 401 → 全链路断）
  return `${CHANNELS.DOUYIN}-unknown`;
}

// 导出供单测验证浮层兜底（避免 /jingxuan 浮层下返回空串导致 WS 401）
export { getAccountId };

// —— 非文字消息提取（问题 3）——
// 抖音气泡可能含图片 / 视频 / 语音 / 表情 / 链接。统一归为 msg_type：
//   text | image | voice | video | emoji | link | recall | system
// 文本优先；含 <img>/视频则记为 image/video（content 留可读描述或 media_url）。
function extractMessageContent(item) {
  const text = cleanText(item.querySelector(SEL.TEXT) || item);
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

// —— 群聊识别（问题 2）——
// 抖音群聊特征：① 会话标题非单一用户（群名常含「群」「家族」「同学」等或含多成员）；
// ② 单条消息含「@昵称 」前缀或「[群成员]」结构；③ 一屏内出现多个不同昵称气泡。
// 这里采用「结构 + 文本」双重判定，避免 1v1 误判为群。
function detectGroup(item) {
  let isGroup = false;
  let groupId = '';
  let groupName = '';
  let senderName = '';
  // 1) 标题栏群名（聊天头部含群标识）
  const header = qs('[class*="chat-header"], [class*="ChatHeader"], [class*="title"], [class*="Title"]');
  const headerText = header ? cleanText(header) : '';
  if (headerText && /群|家族|同学|同事|粉丝|俱乐部|team|group/i.test(headerText)) {
    isGroup = true;
    groupName = headerText;
  }
  // 2) 消息内 @ 前缀 / [群成员昵称] 结构
  const inner = cleanText(item);
  const atMatch = inner.match(/^@([^\s,，：:]+)[\s,，：:]/);
  const bracketMatch = inner.match(/^\[([^\]]+)\]/);
  if (atMatch || bracketMatch) {
    isGroup = true;
    senderName = (atMatch && atMatch[1]) || (bracketMatch && bracketMatch[1]) || '';
  }
  // 3) 群 id：URL 含 group/conversation_id 群串
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
  const active = qsa(
    `${SEL.CHAT_LIST} [class*="curConversation"], ${SEL.CHAT_LIST} [aria-selected="true"], ${SEL.CHAT_LIST} [class*="active"]`
  ).find((el) => el.offsetParent !== null);
  // 群聊：活动项可能含群名而非 /user/ 链接；优先取 data-* 上的会话标识
  const dataConv = active?.getAttribute('data-conversation-id') || active?.getAttribute('data-conv-id') || active?.getAttribute('data-id');
  if (dataConv) return dataConv;
  const link = active?.querySelector('a[href*="/user/"]') || qs('[class*="chat-header"] a[href*="/user/"]');
  // 兼容 /user/<数字id> 与 /user/MS4w...（token 形式）；命中后切换会话会重新回填历史
  const m = link?.getAttribute('href')?.match(/\/user\/([^/?#]+)/);
  return m ? m[1] : null;
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
    // 主路径：用 SelectorEngine 多候选 + 结构启发式定位消息列表与消息项。
    // 即使抖音改版导致 SEL.MSG_ITEM 部分失效，引擎仍能用「像气泡」的结构特征兜底命中，
    // 从架构上解决「单一写死选择器失效 → 只抓到 2 条」。
    const { items } = SelectorEngine.locateMessages({
      root: document,
      itemSelectors: SEL.MSG_ITEM.split(',').map((s) => s.trim()),
      listSelectors: SEL.MSG_LIST.split(',').map((s) => s.trim()).filter(Boolean),
    });
    const filtered = items.filter((it) => !isListNoise(it));
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
    return leafText.filter((el) => !isListNoise(el));
  },
  parseMessageItem(item) {
    // item 已是消息气泡（文本/媒体元素）。先判定消息类型（问题 3：非文字消息）。
    const { msgType, mediaUrl, text } = extractMessageContent(item);
    if (!text && msgType === 'text') return null; // 纯文本且无内容则跳过
    // 自/他判定：对齐检测为主（右=自己，左=客户），class 关键词兜底
    let sender_type = classifyByAlignment(item);
    if (sender_type === SENDER.CUSTOMER && item.closest && (item.closest(SEL.SELF_ITEM) || (item.matches && item.matches(SEL.SELF_ITEM)))) {
      sender_type = SENDER.SELF;
    }
    // 群聊识别（问题 2）：检测群特征（群标题 / @全员 / 多人昵称前缀）
    const groupInfo = detectGroup(item);
    const mid =
      item.getAttribute('data-id') ||
      item.getAttribute('data-msg-id') ||
      item.getAttribute('id') ||
      `${getConversationId()}:${text}:${item.textContent?.length}:${Date.now()}`;
    return {
      message_id: mid,
      sender_type,
      text,
      media_url: mediaUrl,
      msg_type: msgType,
      is_group: groupInfo.isGroup,
      group_id: groupInfo.groupId,
      group_name: groupInfo.groupName,
      sender_name: groupInfo.senderName,
      timestamp: Date.now(),
      raw: item.outerHTML?.slice(0, 500),
    };
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
