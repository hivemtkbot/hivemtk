// background service worker：content script ↔ 服务端 的桥接中继。
// - 每个 (channel, account) 一条 WS（经 registry）
// - content 上行私信 → registry 的 WS → 服务端（触发 AI）
// - 服务端下行 AI 回复 → 路由到对应 content → 回写网页
// - popup 配置 / 状态查询 ↔ storage 双向同步
//
// 设计要点：
//   - 所有默认值从 ../core/constants.js 单源导入，禁止就地写死
//   - chrome.runtime.onMessage / onConnect 全部 lastError 兜底
//   - 配置 storage key 集中常量（KEY）管理，禁止字面量散布
import { registry } from './registry.js';
import { FRAME } from '../core/types.js';
import { DEFAULT_USER_SERVER } from '../core/constants.js';
import { createLogger } from '../core/logger.js';
import { autoInjectOnCommit, injectContentScript, scriptingAvailable } from './injector.js';

const log = createLogger('bg', 'bg');

// 存储 key：popup 与 background 共享，禁止字面量散布
const KEY = 'bridgeConfig';

// key -> Set<port>
const routes = new Map();
// key -> 最近上报的元信息（供状态查询）
const metaStore = new Map();

// ---- chrome API 兜底 ----
function lastError() {
  try {
    if (!chrome.runtime.lastError) return null;
    return chrome.runtime.lastError.message || '(无错误详情)';
  } catch (_) { return null; }
}

// ---- 配置存储 ----
function getConfig() {
  return new Promise((resolve) => {
    chrome.storage.local.get(KEY, (res) => {
      // 缺失时返回基于 constants.js 的默认配置（注意：不是 fallback，
      // 是首次安装的合理初始值；用户可随时在 popup 修改）
      resolve(
        res[KEY] || {
          serverUrl: DEFAULT_USER_SERVER.baseUrl,
          token: '',
          autoConnect: true,
        }
      );
    });
  });
}

function setConfig(cfg) {
  return new Promise((resolve) => {
    chrome.storage.local.set({ [KEY]: cfg }, () => {
      const err = lastError();
      if (err) {
        resolve({ ok: false, error: err });
        return;
      }
      resolve({ ok: true });
    });
  });
}

// ---- 服务端下行 → 路由到对应 content 端口 ----
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

function keyOf(channel, account) {
  return `${channel}:${account || 'unknown'}`;
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

// ---- popup 双向通信：状态查询 / 配置写入 ----
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg && msg.type === 'getStatus') {
    const statuses = {};
    for (const [k, m] of metaStore.entries()) statuses[k] = m;
    sendResponse({ statuses, routes: routes.size });
    return true;
  }
  if (msg && msg.type === 'setConfig') {
    // 走 setConfig helper 统一错误兜底，避免 storage 写入失败时静默
    setConfig(msg.config).then((r) => sendResponse(r));
    return true; // 保持 channel 开启，等待异步 sendResponse
  }
  // ---- 自愈协议：popup 在 ping 失败时请求后台执行 programmatic 注入 ----
  // 解决"标签页在扩展加载/更新前已打开"场景（接收端不存在）
  if (msg && msg.type === 'injectContentScript') {
    const { tabId, url, allFrames } = msg;
    if (typeof tabId !== 'number') {
      sendResponse({ ok: false, reason: 'invalid_tabId' });
      return false;
    }
    injectContentScript(tabId, url, { allFrames: !!allFrames }, (r) => {
      sendResponse(r);
    });
    return true; // 异步 sendResponse
  }
  return false;
});

// ---- webNavigation.onCommitted：自动注入兜底 ----
// 当用户在抖音/小红书/TikTok 站内做 SPA 路由切换、或者新开标签时，
// 由 content_scripts 自动注入（manifest 静态配置）。
// 但若用户**先开标签页后启用扩展**（或扩展刚刚更新），
// 该标签页的 content script 不会自动加载——这是 "Receiving end does not exist"
// 的最常见根因。这里通过 webNavigation.onDOMContentLoaded 主动注入兜底。
//
// 注意：onCommitted 会比 onDOMContentLoaded 更早，对 SPA 路由切换也更敏感。
// 配合 injector.js 的 1.5s 去重，短时间重复路由不会触发重复注入。
if (chrome.webNavigation && chrome.webNavigation.onCommitted) {
  chrome.webNavigation.onCommitted.addListener(autoInjectOnCommit);
  log.info('webNavigation.onCommitted 已接入：自动注入兜底');
} else {
  log.warn('webNavigation API 不可用，依赖 manifest content_scripts 静态注入');
}

if (scriptingAvailable()) {
  log.info('chrome.scripting 可用：popup 侧可触发自愈注入');
}

log.info('background 已就绪');
