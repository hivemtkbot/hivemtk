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
//   5) 死开关（2026-08-14 头脑风暴·P0-C）：汇总 content 上报的 circuit-breaker 健康度，
//      暴露给 popup 展示"最后成功时间 / 熔断状态 / 失败原因"
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
// content 端上报的 health snapshot（按 channel 索引；2026-08-14 P0-C 死开关）
const KEY_HEALTH = 'bridgeHealthByChannel';

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
    // content 端把 active meta + health snapshot 写到 storage；popup 启动 / 刷新时直接读
    chrome.storage.local.get([KEY_ACTIVE, KEY_HEALTH], (res) => {
      const statuses = res[KEY_ACTIVE] || {};
      const healthByChannel = res[KEY_HEALTH] || {};
      // 2026-08-14 P0-C：popup 一次 getStatus 返回活跃状态 + 健康度
      //   - statuses: { [channel]: { accountId, accountName, currentConvId, capturedCount, ... } }
      //   - health: { [channel]: { state, lastSuccessAt, lastFailureAt, healthy, recentReasons } }
      sendResponse({ statuses, health: healthByChannel, routes: 0, connStats: null });
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
  // ---- 2026-08-14 P0-C：content 上报健康度快照 ----
  if (msg && msg.type === 'reportHealth') {
    const { channel, snapshot } = msg;
    if (!channel || !snapshot) {
      sendResponse({ ok: false, reason: 'invalid_payload' });
      return false;
    }
    chrome.storage.local.get(KEY_HEALTH, (res) => {
      const cur = res[KEY_HEALTH] || {};
      cur[channel] = { ...snapshot, reportedAt: Date.now() };
      chrome.storage.local.set({ [KEY_HEALTH]: cur }, () => {
        const err = lastError();
        if (err) sendResponse({ ok: false, error: err });
        else sendResponse({ ok: true });
      });
    });
    return true;
  }
  // ---- 2026-08-15 M2-P1-产品3：紧急停止广播 ----
  // popup 写入 bridgeEmergencyStop=true 后，通知 background 广播到所有 content 立即停摆
  // 设计：content 端会监听 storage.onChanged 自行处理（kill switch 不依赖 background 长连接）
  // 此处 broadcast 仅为加速生效（content 端可以立刻知道，不需要等下个 storage 读周期）
  if (msg && msg.type === 'emergencyStop') {
    broadcastToAllTabs({ type: 'emergencyStop', reason: msg.reason || 'manual' });
    sendResponse({ ok: true, broadcast: true });
    return false;
  }
  if (msg && msg.type === 'resumeBridge') {
    broadcastToAllTabs({ type: 'resumeBridge' });
    sendResponse({ ok: true, broadcast: true });
    return false;
  }
  // ---- 2026-08-15 M2-P1-产品4：多账号列表 ----
  // popup 拉取所有 content 上报的 active meta（已通过 KEY_ACTIVE 存储），按 channel 聚合
  if (msg && msg.type === 'getAccounts') {
    chrome.storage.local.get([KEY_ACTIVE, KEY_HEALTH], (res) => {
      sendResponse({
        accounts: res[KEY_ACTIVE] || {},
        health: res[KEY_HEALTH] || {},
      });
    });
    return true;
  }
  return false;
});

// 2026-08-15 M2-P1-产品3：向所有抖音/小红书/TikTok/闲鱼 标签页广播消息
//   用于 kill switch 立即生效（不等 storage.onChanged 轮询）
function broadcastToAllTabs(payload) {
  try {
    if (!chrome.tabs || !chrome.tabs.query) return;
    chrome.tabs.query({}, (tabs) => {
      const list = Array.isArray(tabs) ? tabs : [];
      for (const t of list) {
        if (!t || !t.id || !t.url) continue;
        if (!/douyin\.com|xiaohongshu\.com|tiktok\.com|goofish\.com|xianyu\.com/i.test(t.url)) continue;
        try {
          chrome.tabs.sendMessage(t.id, payload, () => { try { void chrome.runtime.lastError; } catch (_) {} });
        } catch (_) { /* 接收端不存在：忽略 */ }
      }
    });
  } catch (e) {
    log.warn('broadcastToAllTabs 失败：' + (e && e.message ? e.message : String(e)));
  }
}

// 2026-08-14 P0-C：定期清理陈旧 health 记录（防止 popup 看到已关闭页面的"假死"信号）
//   content 关闭/崩溃时不会主动清理 health，故后台做 TTL 清理（10 分钟无上报视为离线）
setInterval(() => {
  chrome.storage.local.get(KEY_HEALTH, (res) => {
    const cur = res[KEY_HEALTH] || {};
    const now = Date.now();
    let changed = false;
    for (const ch of Object.keys(cur)) {
      if (now - (cur[ch].reportedAt || 0) > 600000) { // 10 分钟
        delete cur[ch];
        changed = true;
      }
    }
    if (changed) chrome.storage.local.set({ [KEY_HEALTH]: cur });
  });
}, 60000); // 每分钟检查一次

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
