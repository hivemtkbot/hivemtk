// content script 公共引导：连接 background 端口，桥接适配器 ↔ 服务端。
// 协议常量/字段与服务端 frames.go 严格对齐（详见 bridge.md §17）。
import { FRAME, parseUnifiedReply } from '../core/types.js';
import { UI_DEFAULTS } from '../core/constants.js';
import { createLogger } from '../core/logger.js';
import { sanitizeForDisplay } from '../core/sanitize.js';

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
function handleSelfcheck(adapter, sendResponse) {
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
function scanDomSnapshot() {
  // 1) 可能的输入框：contenteditable / textarea / role=textbox
  const inputs = collectElements((root) => {
    const sels = [
      'div[contenteditable="true"]',
      'textarea',
      '[role="textbox"]',
      '[contenteditable=""]',
    ];
    return uniqueQueryAll(root, sels);
  });
  const visibleInputs = inputs.filter((el) => isVisible(el));

  // 2) 可能的发送按钮：button / [role=button] / aria-label 含"发" / 含 Send
  const sendBtns = collectElements((root) => {
    const sels = [
      'button[type="button"]',
      'button[type="submit"]',
      '[role="button"]',
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
      '[data-e2e*="message"]',
      '[data-e2e*="chat"]',
    ];
    return uniqueQueryAll(root, sels);
  });
  const visibleListRoots = listRoots.filter((el) => isVisible(el)).slice(0, 8);

  // 4) 截图 href（个人主页链接 → 推断账号 id）
  const accountLinks = uniqueQueryAll(document, ['a[href*="/user/"]', 'a[href*="/@"]', 'a[href*="profile"]'])
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
  ];
  const msgItems = uniqueQueryAll(document, msgItemCandidates).filter((el) => isVisible(el));
  const msgItemSample = msgItems.slice(0, 5).map((el) => elementSummary(el));
  // 命中哪个候选选择器（用于推荐精确 SEL）
  const hitMsgSelector = msgItemCandidates.find((s) => document.querySelector(s)) || null;

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

function isVisible(el) {
  if (!el) return false;
  // offsetParent === null 时表示 display:none（但 fixed 元素例外）
  if (el.offsetParent === null && getComputedStyle(el).position !== 'fixed') return false;
  const rect = el.getBoundingClientRect();
  if (rect.width === 0 || rect.height === 0) return false;
  return true;
}

function isLikelySendButton(el) {
  const aria = (el.getAttribute('aria-label') || '').toLowerCase();
  const text = (el.textContent || '').trim().toLowerCase();
  if (/发送|send|发 送|发 送|發送/.test(aria) || /发送|send|发 送/.test(text)) return true;
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

  // X 修复：ping/selfcheck 监听器必须在 match 检查之前注册。
  // 早期版本在 adapter.match() 失败时直接 return，导致用户在抖音首页
  // （不是私信/消息页）时 popup 永远拿不到任何响应，显示 "undefined"。
  // 现在统一处理：未匹配页只回 ping/selfcheck 诊断，不启动 background 端口/observer。
  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    if (!msg || !msg.type) return false;
    if (msg.type === 'ping') {
      // ping 始终应答（探活用），让 popup 能区分"未注入" vs "未匹配页"
      sendResponse({ pong: true, channel, matched: adapter.match(), matchMode: adapter.matchMode() });
      return false;
    }
    if (msg.type === 'selfcheck') {
      if (!adapter.match()) {
        // 已注入但不是私信/消息页：返回 matched=false + 引导，不报错
        sendResponse({
          channel,
          matched: false,
          matchMode: null,
          accountId: '',
          conversationId: null,
          msgItemCount: 0,
          selectors: adapter.SEL || null,
          sample: [],
          hint: '当前页面不是私信/消息页；请打开目标会话的【私信/聊天】界面（不是首页/feed）后再次校准',
        });
        return false;
      }
      handleSelfcheck(adapter, sendResponse);
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
    return false;
  });

  if (!adapter.match()) {
    log.info('当前页面不匹配', channel, '，不启动监听（已注册 ping/selfcheck 用于诊断）');
    return;
  }

  const port = chrome.runtime.connect({ name: 'bridge' });

  // 下行：服务端 AI 回复经 background 路由到此处
  port.onMessage.addListener(async (msg) => {
    if (msg && msg.type === FRAME.OUTBOUND && msg.reply) {
      const r = parseUnifiedReply(msg.reply);
      if (!r.content) { log.warn('下行回复内容为空，忽略'); return; }
      // 7 扩展端 XSS 防护：先经过 sanitizeForDisplay 净化（控制长度、去掉控制字符）
      const safeContent = sanitizeForDisplay(r.content);
      // 只回写匹配当前会话的回复（多用户场景：避免串台）
      if (r.conversation_id && adapter.getConversationId() && r.conversation_id !== adapter.getConversationId()) {
        log.warn('下行回复会话不匹配，忽略', r.conversation_id, adapter.getConversationId());
        return;
      }
      try {
        await adapter.sendOutbound(safeContent);
      } catch (e) {
        log.error('回写回复失败', e);
      }
    }
  });

  // 上行：实时客户私信 → AI 路径；存量/自己消息 → 仅落库历史
  adapter.start({
    onInbound: (message) => port && port.postMessage({ type: FRAME.INBOUND, message }),
    onHistory: (message) => port && port.postMessage({ type: FRAME.HISTORY, message }),
    onRateLimited: (decision) => log.warn('下行被风控拦截:', decision.reason),
  });

  // B4 修复：仅当 accountId / conversationId 变化时才发 REGISTER（避免无意义重发）
  let lastMeta = { accountId: '', conversationId: '' };
  const report = (force = false) => {
    const meta = adapter.snapshotMeta();
    if (!force && meta.accountId === lastMeta.accountId && meta.conversationId === lastMeta.conversationId) {
      return;
    }
    lastMeta = { accountId: meta.accountId || '', conversationId: meta.conversationId || '' };
    try {
      port.postMessage({ type: FRAME.REGISTER, channel, accountId: meta.accountId, conversationId: meta.conversationId });
    } catch (e) {
      // port 可能已断开
      log.warn('report 失败', e);
    }
  };
  report(true);

  // 修复：保存 timer 句柄便于清理；停止时清除避免泄漏
  // 周期由 UI_DEFAULTS.metaReportIntervalMs 单源管理（见 constants.js / DEFAULTS.md）
  const reportTimer = setInterval(report, UI_DEFAULTS.metaReportIntervalMs);

  // 页面卸载 / SPA 路由切换时清理
  const cleanup = () => {
    if (reportTimer) clearInterval(reportTimer);
    adapter.stop();
    try { port.disconnect(); } catch (_) { /* noop */ }
  };
  window.addEventListener('beforeunload', cleanup, { once: true });
  window.addEventListener('pagehide', cleanup, { once: true });

  window.__bridgeAdapter = adapter;
  log.info('桥接已启动', channel);
}
