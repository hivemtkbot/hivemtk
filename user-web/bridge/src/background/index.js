// background service worker：content script ↔ 服务端 的桥接中继。
// - 每个 (channel, account) 一条 WS（经 registry）
// - content 上行私信 → registry 的 WS → 服务端（触发 AI）
// - 服务端下行 AI 回复 → 路由到对应 content → 回写网页
import { registry } from './registry.js';
import { CHANNELS, FRAME } from '../core/types.js';
import { createLogger } from '../core/logger.js';

const log = createLogger('bg', 'bg');

// key -> Set<port>
const routes = new Map();
// key -> 最近上报的元信息（供状态查询）
const metaStore = new Map();

function keyOf(channel, account) {
  return `${channel}:${account || 'unknown'}`;
}

function getConfig() {
  return new Promise((resolve) => {
    chrome.storage.local.get('bridgeConfig', (res) => {
      resolve(res.bridgeConfig || { serverUrl: 'http://localhost:8080', token: '', autoConnect: true });
    });
  });
}

// 服务端下行 → 路由到对应 content 端口
// B3 修复：无精确会话匹配时**不**fallback 到任意端口（避免多会话串台）。
// 若该账号没有任何端口在线，则丢弃并告警；若有端口但会话不匹配，也告警（content 端应主动 register 当前会话）。
function routeOutbound(reply) {
  const k = keyOf(reply.channel, reply.account_id);
  const set = routes.get(k);
  if (!set || set.size === 0) {
    log.warn('无对应 content 端口，丢弃下行回复', k);
    return;
  }
  // 必须精确会话匹配；找不到时记 warn 但不误投
  const targets = Array.from(set).filter((p) => p.meta && p.meta.conversationId && p.meta.conversationId === reply.conversation_id);
  if (targets.length === 0) {
    log.warn(`下行回复无精确会话匹配（account=${reply.account_id} conv=${reply.conversation_id}），已忽略避免错投`);
    return;
  }
  for (const p of targets) {
    try {
      p.postMessage({ type: FRAME.OUTBOUND, reply });
    } catch (e) {
      log.error('下发失败', e);
    }
  }
}

function ensureConnection(meta) {
  return getConfig().then((cfg) => {
    return registry.ensure({
      serverUrl: cfg.serverUrl,
      token: cfg.token,
      channel: meta.channel,
      accountId: meta.accountId,
      conversationId: meta.conversationId,
      onOutbound: routeOutbound,
      onOpen: () => metaStore.set(keyOf(meta.channel, meta.accountId), { ...meta, online: true }),
      onClose: () => metaStore.set(keyOf(meta.channel, meta.accountId), { ...meta, online: false }),
    });
  });
}

chrome.runtime.onConnect.addListener((port) => {
  if (port.name !== 'bridge') return;
  log.info('content 端口已连接');

  port.onMessage.addListener((msg) => {
    if (!msg || !msg.type) return;
    if (msg.type === FRAME.REGISTER) {
      port.meta = { channel: msg.channel, accountId: msg.accountId, conversationId: msg.conversationId };
      const k = keyOf(msg.channel, msg.accountId);
      if (!routes.has(k)) routes.set(k, new Set());
      routes.get(k).add(port);
      metaStore.set(k, { ...port.meta, online: true });
      ensureConnection(port.meta);
    } else if (msg.type === FRAME.INBOUND) {
      if (msg.message && msg.message.conversation_id) {
        port.meta.conversationId = msg.message.conversation_id;
      }
      const conn = registry.get(port.meta.channel, port.meta.accountId);
      if (conn) conn.sendInbound(msg.message);
      else log.warn('收到 inbound 但无连接', port.meta);
    } else if (msg.type === FRAME.HISTORY) {
      if (msg.message && msg.message.conversation_id) {
        port.meta.conversationId = msg.message.conversation_id;
      }
      const conn = registry.get(port.meta.channel, port.meta.accountId);
      if (conn) conn.sendHistory(msg.message);
      else log.warn('收到 history 但无连接', port.meta);
    } else if (msg.type === 'meta') {
      if (port.meta) {
        port.meta.conversationId = msg.conversationId;
        metaStore.set(keyOf(port.meta.channel, port.meta.accountId), { ...port.meta, online: true });
      }
    }
  });

  port.onDisconnect.addListener(() => {
    if (!port.meta) return;
    const k = keyOf(port.meta.channel, port.meta.accountId);
    const set = routes.get(k);
    if (set) {
      set.delete(port);
      if (set.size === 0) {
        routes.delete(k);
        registry.remove(port.meta.channel, port.meta.accountId);
      }
    }
    log.info('content 端口断开', k);
  });
});

// popup 查询状态 / 配置
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg && msg.type === 'getStatus') {
    const statuses = {};
    for (const [k, m] of metaStore.entries()) statuses[k] = m;
    sendResponse({ statuses, routes: routes.size });
    return true;
  }
  if (msg && msg.type === 'setConfig') {
    chrome.storage.local.set({ bridgeConfig: msg.config }, () => sendResponse({ ok: true }));
    return true;
  }
  return false;
});

log.info('background 已就绪');
