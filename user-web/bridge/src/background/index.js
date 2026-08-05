// background service worker（2026-08-05 HTTP-only 重构）
//
// 2026-08-05 之前：每个 (channel, account) 维护一条到服务端的 WebSocket。
//   存在 SW 冻结 / zombie 连接 / 重连紧密循环 / onMessage 风暴等问题。
//
// 2026-08-05 之后：所有上行走 HTTP 长轮询（content 端 PollingLoop → user-server /api/bridge/ingest），
//   下行 reply 同样通过 HTTP 响应直接返回给 content 端。background 不再维护任何 WS 状态。
//
// background 唯一职责：
//   1) 配置存储（chrome.storage.local）：serverUrl / token / transportMode 等
//   2) popup 通信：getStatus / setConfig / injectContentScript
//   3) webNavigation.onCommitted：SPA 路由切换时自动注入 content script
//   4) 跨 content 共享的 active meta 读取（content 写入 storage.local.activeByChannel）
//
// 设计要点：
//   - 不再 import registry / BridgeClient（WS 模块已删除）
//   - 不再监听 chrome.runtime.onConnect（content 端不再 connect）
//   - 默认值从 ../core/constants.js 单源导入
//   - chrome.runtime.onMessage 全部 lastError 兜底
import { DEFAULT_USER_SERVER } from '../core/constants.js';
import { createLogger } from '../core/logger.js';
import { autoInjectOnCommit, injectContentScript, scriptingAvailable } from './injector.js';

const log = createLogger('bg', 'bg');

// 存储 key：popup 与 background 共享，禁止字面量散布
const KEY = 'bridgeConfig';
// content 端写入的 active meta（按 channel 索引；多个 content 可同时存在，popup 取最新）
const KEY_ACTIVE = 'bridgeActiveByChannel';

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

// ---- popup 双向通信：状态查询 / 配置写入 ----
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg && msg.type === 'getStatus') {
    // content 端把 active meta 写到 storage；popup 启动 / 刷新时直接读
    chrome.storage.local.get(KEY_ACTIVE, (res) => {
      const statuses = res[KEY_ACTIVE] || {};
      sendResponse({ statuses, routes: 0, connStats: null });
    });
    return true; // 异步 sendResponse
  }
  if (msg && msg.type === 'setConfig') {
    setConfig(msg.config).then((r) => sendResponse(r));
    return true;
  }
  // ---- 自愈协议：popup 在 ping 失败时请求后台执行 programmatic 注入 ----
  if (msg && msg.type === 'injectContentScript') {
    const { tabId, url, allFrames } = msg;
    if (typeof tabId !== 'number') {
      sendResponse({ ok: false, reason: 'invalid_tabId' });
      return false;
    }
    injectContentScript(tabId, url, { allFrames: !!allFrames }, (r) => {
      sendResponse(r);
    });
    return true;
  }
  return false;
});

// ---- webNavigation.onCommitted：自动注入兜底 ----
if (chrome.webNavigation && chrome.webNavigation.onCommitted) {
  chrome.webNavigation.onCommitted.addListener(autoInjectOnCommit);
  log.info('webNavigation.onCommitted 已接入：自动注入兜底');
} else {
  log.warn('webNavigation API 不可用，依赖 manifest content_scripts 静态注入');
}

if (scriptingAvailable()) {
  log.info('chrome.scripting 可用：popup 侧可触发自愈注入');
}

log.info('background 已就绪（HTTP-only 模式，0 WS）');
