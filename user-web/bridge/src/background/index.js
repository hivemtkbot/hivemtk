import { DEFAULT_USER_SERVER } from '../core/constants.js';
import { createLogger } from '../core/logger.js';
import { autoInjectOnCommit, injectContentScript, scriptingAvailable } from './injector.js';
import './sse-client.js';

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

// USR-BR-01: 健康度上报增强
// 后端聚合：channel 错误率告警 / popup 实时卡片 / 异常自动暂停
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg && msg.type === 'getStatus') {
    chrome.storage.local.get([KEY_ACTIVE, KEY_HEALTH], (res) => {
      const statuses = res[KEY_ACTIVE] || {};
      const healthByChannel = res[KEY_HEALTH] || {};
      // 统计异常 channel（健康度超时 / 错误率 > 50%）
      const unhealthy = [];
      const now = Date.now();
      for (const ch of Object.keys(healthByChannel)) {
        const h = healthByChannel[ch];
        if (h.errorRate && h.errorRate > 0.5) {
          unhealthy.push({ channel: ch, errorRate: h.errorRate, lastError: h.lastError });
        }
        if (now - (h.reportedAt || 0) > 60000) {
          // 健康度 60s 未更新，视为异常
        }
      }
      sendResponse({ statuses, health: healthByChannel, unhealthy, routes: 0, connStats: null });
    });
    return true;
  }
  if (msg && msg.type === 'setConfig') {
    setConfig(msg.config).then((r) => sendResponse(r));
    return true;
  }
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
        // USR-BR-01: 错误率过高自动暂停
        if (snapshot.errorRate > 0.7) {
          console.warn(`[bg] channel ${channel} 错误率 ${(snapshot.errorRate * 100).toFixed(0)}%，自动暂停`);
          broadcastToAllTabs({ type: 'emergencyStop', reason: 'high_error_rate', channel });
        }
      });
    });
    return true;
  }
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
  if (msg && msg.type === 'getAccounts') {
    chrome.storage.local.get([KEY_ACTIVE, KEY_HEALTH], (res) => {
      sendResponse({
        accounts: res[KEY_ACTIVE] || {},
        health: res[KEY_HEALTH] || {},
      });
    });
    return true;
  }
  // P1-P6：账号暂停/启用 —— 持久化到 config.accounts 并广播 accountsUpdated（content 立即生效）
  if (msg && msg.type === 'bridge:setAccountEnabled') {
    const { channel, accountId, enabled } = msg;
    if (!channel || !accountId) {
      sendResponse({ ok: false, error: 'bridge:setAccountEnabled 缺少 channel/accountId' });
      return false;
    }
    getConfig().then((cfg) => {
      const accounts = cfg.accounts && typeof cfg.accounts === 'object' && !Array.isArray(cfg.accounts)
        ? cfg.accounts : {};
      const ch = accounts[channel] && typeof accounts[channel] === 'object' && !Array.isArray(accounts[channel])
        ? accounts[channel] : {};
      ch[accountId] = { enabled: !!enabled, pausedAt: enabled ? null : Date.now() };
      accounts[channel] = ch;
      return setConfig({ ...cfg, accounts });
    }).then((r) => {
      if (!r.ok) { sendResponse({ ok: false, error: r.error || '持久化失败' }); return; }
      broadcastToAllTabs({ type: 'bridge:accountsUpdated', channel, accountId, enabled: !!enabled });
      sendResponse({ ok: true });
    });
    return true;
  }
  // P1-P6：账号切换 —— 更新 channel/accountId/conversationId 并广播 accountsUpdated
  if (msg && msg.type === 'bridge:switchAccount') {
    const { channel, accountId, conversationId } = msg;
    if (!channel || !accountId) {
      sendResponse({ ok: false, error: 'bridge:switchAccount 缺少 channel/accountId' });
      return false;
    }
    getConfig().then((cfg) =>
      setConfig({ ...cfg, channel, accountId, conversationId: conversationId || '' })
    ).then((r) => {
      if (!r.ok) { sendResponse({ ok: false, error: r.error || '持久化失败' }); return; }
      broadcastToAllTabs({ type: 'bridge:accountsUpdated', channel, accountId });
      sendResponse({ ok: true });
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
        } catch (_) {  }
      }
    });
  } catch (e) {
    log.warn('broadcastToAllTabs 失败：' + (e && e.message ? e.message : String(e)));
  }
}

setInterval(() => {
  chrome.storage.local.get(KEY_HEALTH, (res) => {
    const cur = res[KEY_HEALTH] || {};
    const now = Date.now();
    let changed = false;
    for (const ch of Object.keys(cur)) {
      if (now - (cur[ch].reportedAt || 0) > 600000) { 
        delete cur[ch];
        changed = true;
      }
    }
    if (changed) chrome.storage.local.set({ [KEY_HEALTH]: cur });
  });
}, 60000); 

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

