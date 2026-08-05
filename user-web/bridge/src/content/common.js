// content script 公共引导：把适配器捕获的消息通过 HTTP 长轮询上报到 user-server。
// 协议常量/字段与服务端 handler_http.go 严格对齐（详见 bridge.md §17）。
//
// 2026-08-05 HTTP-only 重构：彻底移除 WebSocket 长连接。
//   - onInbound 回调 → postIngest(expect_reply: true) 调 /api/bridge/ingest
//   - onHistory 回调 → postIngest(expect_reply: false) 仅落库
//   - 服务端 AI 回复随 HTTP 响应直接返回，content 端拿到后调 adapter.sendOutbound
//   - 完整 URL + 解析后的 query 参数 + body 预览由 http-ingest._logRequest 统一打印
//     （与 user-server 侧 collectHTTPRequestInfo 输出格式严格对齐）
import { parseUnifiedReply } from '../core/types.js';
import { DEFAULT_USER_SERVER } from '../core/constants.js';
import { createLogger } from '../core/logger.js';
import { sanitizeForDisplay } from '../core/sanitize.js';
import { hydrateSelectors, SELECTOR_UPDATE_MSG } from '../core/selector-ai.js';
import { postIngest, HTTP_INGEST_DEFAULTS } from '../core/http-ingest.js';

const log = createLogger('content', 'bridge');

// B6 修复：popup -> content 的 selfcheck 用 chrome.tabs.sendMessage 触发，
// 监听器必须在事件循环 turn 内同步调用 sendResponse，否则 chrome 会报
// "message port closed" 且 popup 永远收不到响应。
// 若需异步，必须在监听器内 return true 且最终在 .then 内调用 sendResponse；
// 此处同步处理即可。
//
// 关键修复：原版用 adapter.snapshotMeta() 取 accountId/conversationId，
// 但三渠道 adapter 都没实现 snapshotMeta hook（仅实现 getAccountId/getConversationId），
// 导致自检结果账号/会话永远空，无法定位"哪条会话/哪个账号"在跑。
// 改为走 getter 拿到真实值。
function handleSelfcheck(adapter, sendResponse, diag) {
  try {
    const items = adapter.getMessageItems();
    const parsed = items.slice(-5).map((it) => adapter.parseMessageItem(it)).filter(Boolean);
    sendResponse({
      channel: adapter.channel,
      matched: adapter.match(),
      matchMode: adapter.matchMode(), // 'strict' | 'fallback' | null
      accountId: adapter.getAccountId() || '',
      conversationId: adapter.getConversationId() || null,
      msgItemCount: items.length,
      selectors: adapter.SEL || null,
      sample: parsed.map((p) => ({ sender: p.sender_type, text: (p.text || '').slice(0, 60) })),
      // 上报链路诊断：桥接是否激活 / 各计数（定位「解析到了但不上报」）
      // 2026-08-05 HTTP-only 重构：移除 portAlive（无 port 概念）。
      bridgeActive: diag ? diag.active : null,
      stats: diag ? diag.stats : null,
    });
  } catch (e) {
    sendResponse({ error: String(e) });
  }
}

// 深度自检：即便 match() 返回 false 也能扫描 DOM，给出真实页面快照
// 用于「打开私信页但平台改版导致 match() 失败」的诊断场景
// 不读 adapter 状态，纯 DOM 扫描，IO 轻量
//
// 关键修复：早期版本用 `return false` 同步模式，但 scanDomSnapshot 在抖音/小红书/
// TikTok 等复杂 SPA 上做 4 次重量级 querySelectorAll，同步执行可能超过 Chrome
// 端口有效响应窗口（约 5s），导致 "The message port closed before a response
// was received" 错误，popup 永远收不到响应。
// 改为异步模式：return true 保持 port 打开，把 scan 放进 microtask 让出事件循环，
// sendResponse 包 try/catch 防止 port 已关闭时二次抛错。
function handleDeepSelfcheck(sendResponse) {
  // 走微任务而不是直接同步执行；这样 chrome.tabs.sendMessage 收到消息后
  // 立刻 return true 拿到的 port 在下一次事件循环 turn 之前不会被关闭。
  Promise.resolve()
    .then(() => scanDomSnapshot())
    .then((snapshot) => {
      try {
        sendResponse({ ok: true, ...snapshot });
      } catch (e) {
        // port 可能在扫描过程中被关闭（页面刷新 / 标签关闭 / 长时间无响应）
        // 此处无法把错误传回 popup，仅记录
        log.warn('deepSelfcheck sendResponse 失败（port 已关闭？）', e);
      }
    })
    .catch((e) => {
      try {
        sendResponse({ ok: false, error: String(e && e.message || e) });
      } catch (sendErr) {
        log.warn('deepSelfcheck 错误回传失败（port 已关闭）', sendErr);
      }
    });
}

// 通用 DOM 快照：扫描页面上所有可能的「消息输入/输出/会话」元素
// 不依赖任何平台特定选择器，给出真实可观测数据
export function scanDomSnapshot() {
  // 1) 可能的输入框：contenteditable / textarea / role=textbox
  const inputs = collectElements((root) => {
    const sels = [
      'div[contenteditable="true"]',
      'textarea',
      '[role="textbox"]',
      '[contenteditable=""]',
      '.sendbox textarea',
      'textarea.ant-input',
    ];
    return uniqueQueryAll(root, sels);
  });
  const visibleInputs = inputs.filter((el) => isVisible(el));

  // 2) 可能的发送按钮：button / [role=button] / aria-label 含"发" / 含 Send
  //    抖音真实发送按钮为 svg.messageMsgInputpublishBtn.e2e-send-msg-btn，需显式纳入
  const sendBtns = collectElements((root) => {
    const sels = [
      'button[type="button"]',
      'button[type="submit"]',
      '[role="button"]',
      '[class*="e2e-send-msg-btn"]',
      '[class*="send-msg"]',
      'svg[class*="send"]',
      '.sendbox button',
      '[class*="sendbox" i] button',
    ];
    return uniqueQueryAll(root, sels);
  });
  const visibleSendBtns = sendBtns
    .filter((el) => isVisible(el))
    .filter((el) => isLikelySendButton(el))
    .slice(0, 10);

  // 3) 可能的会话列表：含 list / chat / message / scroll 类的容器
  const listRoots = collectElements((root) => {
    const sels = [
      '[class*="chat-"]',
      '[class*="message-"]',
      '[class*="msg-"]',
      '[class*="conversation"]',
      '[class*="contact"]',
      '[class*="list"]',
      '[class*="recycle"]',
      '[class*="sidebar"]',
      '[class*="conv-list"]',
      '[id*="conv-list"]',
      '[id*="message-list"]',
      '[data-e2e*="message"]',
      '[data-e2e*="chat"]',
    ];
    return uniqueQueryAll(root, sels);
  });
  // 优先：conversation-list / conv-list / sider 排除 chat-main（右侧面板）
  const chatMainRegex = /chat-main|chat-main|ChatMain|ant-layout-content/i;
  const prioritized = listRoots.filter((el) => {
    const cls = el.className || '';
    return /conversation-list|conv-list|sider|chat-list/i.test(cls) && !chatMainRegex.test(cls);
  });
  const visibleListRoots = (prioritized.length ? prioritized : listRoots)
    .filter((el) => isVisible(el))
    .slice(0, 8);

  // 4) 截图 href（个人主页链接 → 推断账号 id）
  const accountLinks = uniqueQueryAll(document, ['a[href*="/user/"]', 'a[href*="/@"]', 'a[href*="profile"]', 'a[href*="personal?userId="]'])
    .filter((el) => isVisible(el))
    .slice(0, 5);
  const accountHints = accountLinks.map((a) => ({
    text: (a.textContent || '').trim().slice(0, 30),
    href: a.getAttribute('href') || '',
  }));

  // 5) 消息气泡候选：覆盖多版 DOM 变体，用于定位「为何没捕获到聊天消息」
  const msgItemCandidates = [
    'div[data-e2e="msg-item-content"]',
    '[data-e2e*="msg-item"]',
    '[class*="msg-item-content"]',
    '[class*="msg-content"]',
    '[class*="bubble"]',
    '[class*="Bubble"]',
    '[class*="messageText"]',
    '[class*="MessageText"]',
    '[class*="chatMsgItem"]',
    '[class*="MessageItem"]',
    '[class*="messageItem"]',
    '[class*="msgBubble"]',
    '[class*="MsgBubble"]',
    '[class*="imMessage"]',
    '[class*="ImMessage"]',
    '[class*="dialogItem"]',
    '[class*="chatItem"]',
    '[class*="ChatItem"]',
    '[class*="messageBubble"]',
    '[class*="message-row"]',
    '[class*="MessageRow"]',
    '[class*="message-text"]',
  ];
  const msgItems = uniqueQueryAll(document, msgItemCandidates).filter((el) => isVisible(el));
  const msgItemSample = msgItems.slice(0, 5).map((el) => elementSummary(el));
  // 命中哪个候选选择器（用于推荐精确 SEL）
  const hitMsgSelector = msgItemCandidates.find((s) => document.querySelector(s)) || null;

  // 6) 消息线程容器深度结构扫描（关键诊断）：
  //    新版 /chat 页消息气泡 class 未知时，预置候选全 miss → 从已知容器
  //    （im-chat-window / xhs-im-chat-window / chat-window 等）向下钻取，
  //    列出每层可见子节点的 class + 文本，精确定位真实气泡 class。
  const threadRoot = document.querySelector(
    '#message-list-scrollable, .im-chat-window, [class*="xhs-im-chat-window"], [class*="chat-window"], [class*="ChatWindow"], [class*="chat-content"], [class*="ChatContent"], [class*="im-chat"], [class*="ImChat"], [class*="chat-main"], [class*="ChatMain"], [class*="message-list-reverse"]'
  );
  let threadTree = [];
  if (threadRoot) {
    const walk = (el, depth) => {
      if (!el || depth > 4 || threadTree.length >= 12) return;
      const cls = (el.className && typeof el.className === 'string') ? el.className.trim().slice(0, 60) : '';
      const txt = (el.textContent || '').trim().slice(0, 24);
      // 只记录「有 class 且含文本」的节点，跳过纯容器骨架
      if (cls && txt) {
        threadTree.push({ depth, cls, txt });
      }
      for (const child of el.children) walk(child, depth + 1);
    };
    for (const child of threadRoot.children) walk(child, 1);
  }

  return {
    url: location.href,
    hostname: location.hostname,
    title: document.title,
    inputCount: visibleInputs.length,
    inputSample: visibleInputs.slice(0, 5).map((el) => elementSummary(el)),
    sendBtnCount: visibleSendBtns.length,
    sendBtnSample: visibleSendBtns.map((el) => elementSummary(el)),
    listRootCount: visibleListRoots.length,
    listRootSample: visibleListRoots.map((el) => elementSummary(el)),
    accountHints,
    // 推荐操作：让用户能根据真实 DOM 修正 SEL
    recommendedSelector: pickRecommendedSelector(visibleInputs),
    msgItemCount: msgItems.length,
    msgItemSample,
    recommendedMsgSelector: hitMsgSelector || '（未匹配到消息气泡，请把本快照 msgItemSample 发到 issue 校准 SEL.MSG_ITEM）',
    threadRootFound: !!threadRoot,
    threadTree,
  };
}

function uniqueQueryAll(root, sels) {
  const out = new Set();
  for (const sel of sels) {
    try {
      const list = root.querySelectorAll(sel);
      for (const n of list) out.add(n);
    } catch (_) {
      // 忽略非法选择器
    }
  }
  return Array.from(out);
}

function collectElements(producer) {
  return producer(document.body || document);
}

export function isVisible(el) {
  if (!el) return false;
  const style = getComputedStyle(el);
  if (style.display === 'none' || style.visibility === 'hidden') return false;
  // SVG 元素的 offsetParent 恒为 null（不在 HTML offset 链里），
  // 不能据此判隐藏；改为仅用 getBoundingClientRect 判断尺寸。
  // 早期版本 `el.offsetParent === null && position !== 'fixed'` 会把抖音
  // svg.messageMsgInputpublishBtn 发送按钮一律判成不可见，导致自检报「发送按钮 0 个」。
  const isSvg = typeof SVGElement !== 'undefined' && el instanceof SVGElement;
  if (!isSvg && el.offsetParent === null && style.position !== 'fixed') return false;
  const rect = el.getBoundingClientRect();
  if (rect.width === 0 || rect.height === 0) return false;
  return true;
}

function isLikelySendButton(el) {
  const cls = (el.getAttribute('class') || '').toLowerCase();
  if (/e2e-send-msg-btn|send-msg|sendmsg/.test(cls)) return true; // 抖音 svg 发送按钮
  const aria = (el.getAttribute('aria-label') || '').toLowerCase();
  const text = (el.textContent || '').trim().toLowerCase();
  if (/发送|send|发 送|發送/.test(aria) || /发送|send|发 送/.test(text)) return true;
  if (el.querySelector('svg[fill*="FE2C55"], svg[fill*="fe2c55"]')) return true;
  return false;
}

function elementSummary(el) {
  const tag = el.tagName.toLowerCase();
  const id = el.id ? `#${el.id}` : '';
  const cls = (el.getAttribute('class') || '').trim().slice(0, 80);
  const clsPart = cls ? `.${cls.split(/\s+/).slice(0, 3).join('.')}` : '';
  const dataE2E = el.getAttribute('data-e2e');
  const dataE2EPart = dataE2E ? `[data-e2e="${dataE2E}"]` : '';
  const role = el.getAttribute('role');
  const rolePart = role ? `[role="${role}"]` : '';
  const placeholder = el.getAttribute('placeholder') || '';
  const ce = el.getAttribute('contenteditable');
  const cePart = ce !== null ? `[contenteditable="${ce || 'true'}"]` : '';
  const text = (el.textContent || '').trim().slice(0, 40);
  return {
    tag,
    selectorHint: `${tag}${id}${clsPart}${dataE2EPart}${rolePart}${cePart}`.slice(0, 200),
    placeholder: placeholder.slice(0, 50),
    text: text,
  };
}

function pickRecommendedSelector(visibleInputs) {
  if (!visibleInputs.length) return null;
  // 优先级：textarea > 带 placeholder > role=textbox > contenteditable
  for (const el of visibleInputs) {
    if (el.tagName.toLowerCase() === 'textarea') {
      const ph = el.getAttribute('placeholder');
      if (ph) return `textarea[placeholder*="${ph.slice(0, 12)}"]`;
    }
  }
  for (const el of visibleInputs) {
    const ce = el.getAttribute('contenteditable');
    if (ce === 'true') {
      const dataE2E = el.getAttribute('data-e2e');
      if (dataE2E) return `[data-e2e="${dataE2E}"]`;
    }
  }
  for (const el of visibleInputs) {
    const role = el.getAttribute('role');
    if (role === 'textbox') {
      const cls = (el.getAttribute('class') || '').split(/\s+/).filter(Boolean).slice(0, 2).join('.');
      if (cls) return `.${cls}`;
    }
  }
  // 兜底：第一个 contenteditable
  for (const el of visibleInputs) {
    if (el.getAttribute('contenteditable') === 'true') {
      return 'div[contenteditable="true"]';
    }
  }
  return null;
}

export function startBridge(channel, buildAdapter) {
  const adapter = buildAdapter();
  // 上报链路诊断状态（自检时返回给 popup，定位「解析到了但不上报」）。
  // 2026-08-05 HTTP-only 重构：移除 portAlive/connectError（无 port 概念），
  // 移除 register 计数（HTTP 模式无 REGISTER 帧）。
  const diag = { active: false, stats: { inbound: 0, history: 0, outbound: 0, dropped: 0 } };
  // 注入即打印（任何页面都会出现）——用户可据此判断 content script 是否真的运行，
  // 区分「脚本没注入」vs「注入了但选择器没匹配」。
  try { log.info('已注入 content script', channel, location.host, 'match=' + adapter.match()); } catch (_) { /* noop */ }

  // 启动时从 chrome.storage 水合选择器配置到本页 localStorage 镜像（popup 保存后立即生效的基础）
  try { hydrateSelectors(); } catch (_) { /* noop */ }

  // 配置读取：每次 submitIngest 提交前调一次，从 chrome.storage 拿 serverUrl/token。
  // chrome.storage 在 content script 不可用（需要 background 转发）— 但实测可用，
  // 与 background 共享同一 storage.local 命名空间。失败兜底走 DEFAULT_USER_SERVER。
  const getConfig = () => new Promise((resolve) => {
    if (!chrome || !chrome.storage || !chrome.storage.local) {
      resolve({ serverUrl: DEFAULT_USER_SERVER.baseUrl, token: '' });
      return;
    }
    try {
      chrome.storage.local.get('bridgeConfig', (res) => {
        const err = (chrome.runtime && chrome.runtime.lastError) ? chrome.runtime.lastError : null;
        if (err) {
          log.warn('getConfig 失败（fallback 默认）', err.message);
          resolve({ serverUrl: DEFAULT_USER_SERVER.baseUrl, token: '' });
          return;
        }
        const cfg = (res && res.bridgeConfig) || {};
        resolve({
          serverUrl: cfg.serverUrl || DEFAULT_USER_SERVER.baseUrl,
          token: cfg.token || '',
        });
      });
    } catch (e) {
      log.warn('getConfig 异常（fallback 默认）', e && e.message);
      resolve({ serverUrl: DEFAULT_USER_SERVER.baseUrl, token: '' });
    }
  });

  // X 修复：ping/selfcheck 监听器必须在 match 检查之前注册。
  // 早期版本在 adapter.match() 失败时直接 return，导致用户在抖音首页
  // （不是私信/消息页）时 popup 永远拿不到任何响应，显示 "undefined"。
  // 现在统一处理：未匹配页只回 ping/selfcheck 诊断，不启动 background 端口/observer。
  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    if (!msg || !msg.type) return false;
    // match() 安全包装：任何异常都视为「未匹配」并带错误信息返回，不中断监听器
    const safeMatch = () => {
      try { return adapter.match(); } catch (e) { return { error: String(e && e.message || e) }; }
    };
    if (msg.type === 'ping') {
      // ping 始终应答（探活用），让 popup 能区分"未注入" vs "未匹配页"
      let m = safeMatch();
      const isObj = m && typeof m === 'object' && !Array.isArray(m);
      sendResponse({ pong: true, channel, matched: m === true, matchError: isObj ? m.error : null, matchMode: adapter.matchMode() });
      return false;
    }
    if (msg.type === 'selfcheck') {
      let m = safeMatch();
      const isObj = m && typeof m === 'object' && !Array.isArray(m);
      if (m !== true) {
        // 已注入但不是私信/消息页：返回 matched=false + 引导，不报错
        sendResponse({
          channel,
          matched: false,
          matchMode: null,
          matchError: isObj ? m.error : null,
          accountId: '',
          conversationId: null,
          msgItemCount: 0,
          selectors: adapter.SEL || null,
          sample: [],
          hint: '当前页面不是私信/消息页；请打开目标会话的【私信/聊天】界面（不是首页/feed）后再次校准',
        });
        return false;
      }
      handleSelfcheck(adapter, sendResponse, diag);
      return false;
    }
    // 深度自检：无论 match() 成功与否都可触发，给出真实 DOM 快照
    // 用于诊断"match() 失败但页面看着像私信页"（平台改版 / 选择器过期）
    //
    // 必须 return true：handleDeepSelfcheck 内部走 Promise 微任务，扫描是异步的。
    // 若 return false，Chrome 会在当前 tick 结束就关闭 port，popup 收到
    // "The message port closed before a response was received" 错误。
    if (msg.type === 'deepSelfcheck') {
      handleDeepSelfcheck(sendResponse);
      return true;
    }
    // 全量遍历同步所有私信会话（一个私信=一个会话=统一收信中心一条记录；遍历所有私信上报）。
    // 仅在已匹配私信页时有效；异步返回 { ok, synced, total, failures }。
    if (msg.type === 'syncAllConversations') {
      if (!adapter.match()) {
        sendResponse({ ok: false, error: '当前页面不是私信页，无法遍历' });
        return false;
      }
      adapter.syncAllConversations({ throttleMs: 1000 }).then((r) => {
        try { sendResponse({ ok: true, ...r }); } catch (_) { /* noop */ }
      }).catch((e) => {
        try { sendResponse({ ok: false, error: String((e && e.message) || e) }); } catch (_) { /* noop */ }
      });
      return true;
    }
    // popup 保存选择器后广播 → content script 重新从 chrome.storage 水合到本页 localStorage 镜像
    if (msg.type === SELECTOR_UPDATE_MSG) {
      hydrateSelectors().then(() => { try { sendResponse({ ok: true }); } catch (_) { /* noop */ } });
      return true;
    }
    // 巡检制度（需求上行②）：启动/停止/立即巡检一轮/查状态
    if (msg.type === 'patrolStart') {
      if (!adapter.startPatrol) { sendResponse({ ok: false, error: '适配器不支持巡检' }); return false; }
      const r = adapter.startPatrol({ intervalMs: msg.intervalMs });
      sendResponse(r);
      return false;
    }
    if (msg.type === 'patrolStop') {
      if (!adapter.stopPatrol) { sendResponse({ ok: false, error: '适配器不支持巡检' }); return false; }
      sendResponse(adapter.stopPatrol());
      return false;
    }
    if (msg.type === 'patrolNow') {
      if (!adapter.patrol) { sendResponse({ ok: false, error: '适配器不支持巡检' }); return false; }
      adapter.patrol().then((r) => {
        try { sendResponse({ ok: true, ...r }); } catch (_) { /* noop */ }
      }).catch((e) => {
        try { sendResponse({ ok: false, error: String((e && e.message) || e) }); } catch (_) { /* noop */ }
      });
      return true;
    }
    if (msg.type === 'patrolStatus') {
      sendResponse(adapter.patrolStatus ? adapter.patrolStatus() : { ok: false, error: '适配器不支持巡检' });
      return false;
    }
    return false;
  });

  // K2 桥接统计（跨重连累计，便于排查「通通没数据」类问题）：
  // inbound=实时上行、history=历史上行、outbound=下行回写、dropped=端口断开丢弃、register=注册帧次数。
  const stats = diag.stats;

  // 抖音/小红书/TikTok 多为 SPA：content script 注入时私信面板（常作浮层/overlay）
  // 未必已打开，match() 此刻为 false；用户后续点开私信，面板才渲染。
  // 早期版本在 match() 为 false 时直接 return，导致「打开私信页但桥接从未启动」。
  // 改为：先立即尝试激活；不匹配则轮询，待私信面板出现再激活；面板关闭则自动停用。
  let active = false;
  let deactivateBridge = null;
  let pollTimer = null;

  // 2026-08-05 HTTP-only 重构：移除所有 WebSocket 相关状态机。
  //   原来「sync 限速 + onDisconnect 退避 + 5 次连续失败」全是 WS 时代的产物，
  //   HTTP 长轮询无状态、每次请求独立，无需重连计数。保留 sync() 限速用于
  //   「SPA 浮层打开/关闭」时频繁调用 activateBridge 的去抖。
  let lastSyncAt = 0;
  const SYNC_THROTTLE_MS = 3000;      // sync 节流：至少 3 秒间隔

  const activateBridge = () => {
    let closed = false;       // 主动清理（页面卸载）标记

    // 下行分发：HTTP 响应中携带的 outbound_replies 解析后调 adapter.sendOutbound。
    // 抽出独立函数避免内联过长，调试 / 单测也方便。
    const dispatchOutboundReply = async (reply) => {
      const r = parseUnifiedReply(reply);
      if (!r.content) { log.warn('下行回复内容为空，忽略'); return; }
      // 7 扩展端 XSS 防护：先经过 sanitizeForDisplay 净化（控制长度、去掉控制字符）
      const safeContent = sanitizeForDisplay(r.content);
      // 截断标记提示：服务端截断到 4KB 时置 truncated=true，
      // 扩展端在内容尾部追加可见提示，避免用户看到半截消息不知情。
      const finalContent = r.truncated
        ? safeContent + '\n[消息被截断，请联系客服获取完整内容]'
        : safeContent;
      try {
        // 目标会话 ≠ 当前打开的会话时，sendOutbound 会先在左侧列表找到目标用户
        // → 点击进入右侧聊天页 → 再模拟输入发送（用户诉求：按用户找会话再发）。
        await adapter.sendOutbound(finalContent, r.conversation_id);
        stats.outbound++;
        log.info('[下行 outbound] #' + stats.outbound, {
          conv: r.conversation_id,
          len: (r.content || '').length,
          truncated: r.truncated,
        });
      } catch (e) {
        log.error('回写回复失败', e);
      }
    };

    // 通过 postIngest 上报单条消息。统一调用入口：onInbound 走 expect_reply=true
    // 等待 AI 回复；onHistory 走 expect_reply=false 仅落库。
    //
    // 关键设计：URL + 完整 query + body 预览由 http-ingest._logRequest 统一打印
    // （所有渠道 douyin/xhs/tiktok/xianyu 共用），用户可从 console 直接看到上行地址。
    const submitIngest = async (message, opts) => {
      if (!message) return;
      const cfg = await getConfig();
      const meta = adapter.snapshotMeta();
      const accountId = (opts && opts.accountId) || meta.accountId || '';
      const conversationId = (opts && opts.conversationId) || message.conversation_id || meta.conversationId || '';
      if (!accountId || !conversationId) {
        log.warn('ingest 跳过：accountId/conversationId 缺失', { accountId, conversationId });
        return;
      }
      const expectReply = !!(opts && opts.expectReply);
      const label = expectReply ? '[上行 inbound ingest]' : '[上行 history ingest]';
      // 构造 ingest body：与 user-server handler_http.go HTTPIngestRequest 字段对齐。
      // 单条消息包成 messages[]（保持与多消息批量上报统一，server 端解析逻辑一致）。
      const body = {
        v: 2,
        channel: message.channel || channel,
        account_id: accountId,
        conversation_id: conversationId,
        account_name: '',
        agent_id: 0,
        messages: [
          {
            event_id: message.event_id || '',
            channel: message.channel || channel,
            account_id: accountId,
            conversation_id: conversationId,
            sender_id: message.sender_id || '',
            sender_name: message.sender_name || '',
            sender_type: message.sender_type || 'customer',
            msg_type: message.msg_type || 'text',
            content: message.content || '',
            media_url: message.media_url || '',
            timestamp: message.timestamp || Date.now(),
            is_group: !!message.is_group,
            group_id: message.group_id || '',
            group_name: message.group_name || '',
            // 多轮历史：服务端按 messages[].history 逐条落库
            history: Array.isArray(message.history) ? message.history : [],
          },
        ],
        expect_reply: expectReply,
        timeout_ms: HTTP_INGEST_DEFAULTS.longPollTimeoutMs,
      };
      try {
        const resp = await postIngest(
          {
            serverUrl: cfg.serverUrl || DEFAULT_USER_SERVER.baseUrl,
            channel: message.channel || channel,
            accountId,
            conversationId,
            token: cfg.token || '',
          },
          body,
          {
            label,
            timeoutMs: HTTP_INGEST_DEFAULTS.longPollTimeoutMs + 5000,
          }
        );
        // 响应解析：服务端可能附带 outbound_replies（仅当 expect_reply=true 时有内容）
        if (resp && Array.isArray(resp.outbound_replies) && resp.outbound_replies.length) {
          for (const reply of resp.outbound_replies) {
            await dispatchOutboundReply(reply);
          }
        }
      } catch (e) {
        // 失败不抛：避免污染调用栈；上层已用 try/catch 包裹
        log.warn(`${label} 提交失败`, { error: String(e && e.message || e) });
      }
    };

    // 上行：实时客户私信 → AI 路径；存量/自己消息 → 仅落库历史
    adapter.start({
      onInbound: (message) => {
        stats.inbound++;
        const len = (message && message.content ? message.content.length : 0);
        log.info('[上行 inbound] #' + stats.inbound, {
          conv: message && message.conversation_id,
          sender: message && message.sender_type,
          len,
        });
        // fire-and-forget：postIngest 自带退避重试 + 完整 URL/参数日志
        submitIngest(message, { expectReply: true });
      },
      onHistory: (message) => {
        stats.history++;
        const len = (message && message.content ? message.content.length : 0);
        log.info('[上行 history] #' + stats.history, {
          conv: message && message.conversation_id,
          sender: message && message.sender_type,
          len,
        });
        submitIngest(message, { expectReply: false });
      },
      onRateLimited: (decision) => log.warn('下行被风控拦截:', decision.reason),
    });

    // 页面卸载 / SPA 路由切换时清理
    const cleanup = () => {
      closed = true;
      adapter.stop();
    };
    window.addEventListener('beforeunload', cleanup, { once: true });
    window.addEventListener('pagehide', cleanup, { once: true });

    window.__bridgeAdapter = adapter;
    log.info('桥接已启动（HTTP-only）', channel);
    return cleanup;
  };

  const sync = () => {
    // 2026-08-05 修复：sync 节流，防止 SPA 浮层打开/关闭切换时被频繁激活。
    //   至少 3 秒间隔，上次 sync 不足 3 秒则跳过。
    const now = Date.now();
    if (now - lastSyncAt < SYNC_THROTTLE_MS) return;
    lastSyncAt = now;
    // match() 若抛异常（平台早期 DOM 结构不完整 / 某选择器报错），不得中断整个桥接。
    let matched = false;
    try { matched = !!adapter.match(); } catch (e) { log.warn('match() 异常，跳过本轮', e && e.message); }
    if (!active && matched) {
      log.info('私信面板已匹配', channel, '，启动桥接');
      deactivateBridge = activateBridge();
      active = true;
      diag.active = true;
    } else if (active && !matched) {
      log.info('私信面板已关闭', channel, '，停止桥接');
      if (deactivateBridge) { deactivateBridge(); deactivateBridge = null; }
      active = false;
      diag.active = false;
    }
  };

  // 立即尝试一次；不匹配则轮询，覆盖「面板稍后打开 / 关闭」两种场景（抖音 SPA 浮层）
  sync();
  if (!active) {
    log.info('当前页面不匹配', channel, '，启动轮询等待私信面板打开…');
  }
  pollTimer = setInterval(sync, 1500);

  // K2 统计汇总日志：每 15s 打印一次累计条数，便于现场排查数据是否上行/下行/丢弃。
  // 2026-08-05 HTTP-only 重构：移除 register 字段（HTTP 模式无 REGISTER 帧）。
  let statsTimer = setInterval(() => {
    log.info('桥接统计', {
      channel,
      active,
      inbound: stats.inbound,
      history: stats.history,
      outbound: stats.outbound,
      dropped: stats.dropped,
    });
  }, 15000);

  const stopPoll = () => {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
    if (statsTimer) { clearInterval(statsTimer); statsTimer = null; }
  };
  window.addEventListener('beforeunload', stopPoll, { once: true });
  window.addEventListener('pagehide', stopPoll, { once: true });
}
