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
function handleSelfcheck(adapter, sendResponse) {
  try {
    const meta = adapter.snapshotMeta();
    const items = adapter.getMessageItems();
    const parsed = items.slice(-5).map((it) => adapter.parseMessageItem(it)).filter(Boolean);
    sendResponse({
      channel: adapter.channel,
      matched: adapter.match(),
      accountId: meta.accountId,
      conversationId: meta.conversationId,
      msgItemCount: items.length,
      selectors: adapter.SEL || null,
      sample: parsed.map((p) => ({ sender: p.sender_type, text: (p.text || '').slice(0, 60) })),
    });
  } catch (e) {
    sendResponse({ error: String(e) });
  }
}

export function startBridge(channel, buildAdapter) {
  const adapter = buildAdapter();

  // P0-S2-X 修复：ping/selfcheck 监听器必须在 match() 检查之前注册。
  // 早期版本在 adapter.match() 失败时直接 return，导致用户在抖音首页
  // （不是私信/消息页）时 popup 永远拿不到任何响应，显示 "undefined"。
  // 现在统一处理：未匹配页只回 ping/selfcheck 诊断，不启动 background 端口/observer。
  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    if (!msg || !msg.type) return false;
    if (msg.type === 'ping') {
      // ping 始终应答（探活用），让 popup 能区分"未注入" vs "未匹配页"
      sendResponse({ pong: true, channel, matched: adapter.match() });
      return false;
    }
    if (msg.type === 'selfcheck') {
      if (!adapter.match()) {
        // 已注入但不是私信/消息页：返回 matched=false + 引导，不报错
        sendResponse({
          channel,
          matched: false,
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
      // P0-S1-7 扩展端 XSS 防护：先经过 sanitizeForDisplay 净化（控制长度、去掉控制字符）
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

  // R4 修复：保存 timer 句柄便于清理；停止时清除避免泄漏
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
