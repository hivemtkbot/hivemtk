// content script 公共引导：连接 background 端口，桥接适配器 ↔ 服务端。
// 协议常量/字段与服务端 frames.go 严格对齐（详见 bridge.md §17）。
import { FRAME, parseUnifiedReply } from '../core/types.js';
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
  if (!adapter.match()) {
    log.info('当前页面不匹配', channel, '，不启动监听');
    return;
  }

  const port = chrome.runtime.connect({ name: 'bridge' });

  // B6 修复：同步响应 selfcheck；不返回 true（异步）。
  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    if (msg && msg.type === 'selfcheck') {
      handleSelfcheck(adapter, sendResponse);
    }
    // 未命中 selfcheck 时不要返回 true（否则 chrome 等待异步响应泄漏 port）
    return false;
  });

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
  const reportTimer = setInterval(report, 5000);

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
