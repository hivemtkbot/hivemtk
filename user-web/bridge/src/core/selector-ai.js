// selector-ai.js — 选择器配置中心（纯规则，无 LLM）
//
// 设计决策（用户诉求）：去掉抖音/小红书的 LLM 抽取结构逻辑，改回「人工配置选择器」：
//   1) 平台网页改版 → 开发者/用户从 DevTools 人工识别关键 HTML 结构（class/data 属性）
//   2) 把关键节点选择器填写进 popup 的「选择器配置」（存 chrome.storage.local）
//   3) 扩展运行时读配置 → 合并各渠道默认 SEL → 命中即用，无需发版
//
// 数据流：
//   chrome.storage.local['hivebridge:selectors'] （用户配置，可编辑）
//     → getCustomSelectors(channel)（同步读，会话内缓存）
//     → mergeSelectors(channel, domain, ruleFallbacks)（用户配置优先，SEL 默认兜底）
//
// 移除：LLM 抽取器（extractor）、云端快照、自愈刷新——全部废弃，保持接口兼容。

// 存储 key（与 popup / content 共用）
export const SELECTOR_CONFIG_KEY = 'hivebridge:selectors';

// 选择器配置更新广播消息类型（popup 保存后向 content script 广播，触发重新水合）
export const SELECTOR_UPDATE_MSG = 'selectorsUpdated';

// 各渠道可配置的关键节点字段（供 popup 编辑表单 + mergeSelectors 读取）
export const SELECTOR_FIELDS = [
  'conversationList', // 左侧会话列表项
  'messageList',      // 消息列表容器
  'messageItem',      // 消息气泡
  'text',             // 气泡内文本
  'input',            // 输入框
  'send',             // 发送按钮
  'selfMarker',       // 自方标记
  'otherMarker',      // 对方标记
];

// 会话内配置缓存（跨重载靠 chrome.storage）
const _customCache = new Map();

function _readFromStorage() {
  try {
    if (typeof chrome !== 'undefined' && chrome.storage && chrome.storage.local) {
      // 同步读 storage（content script 里 chrome.storage.local.get 是异步的；
      // 为保持 mergeSelectors 同步契约，先用同步 localStorage 镜像 + 异步 storage 双写）
      if (typeof localStorage !== 'undefined') {
        const raw = localStorage.getItem(SELECTOR_CONFIG_KEY);
        if (raw) return JSON.parse(raw);
      }
    }
    if (typeof localStorage !== 'undefined') {
      const raw = localStorage.getItem(SELECTOR_CONFIG_KEY);
      if (raw) return JSON.parse(raw);
    }
  } catch (_) { /* 存储不可用/损坏，忽略 */ }
  return null;
}

// 异步写配置（popup 调用）：双写 chrome.storage + localStorage 镜像
export function saveSelectors(config) {
  const payload = config || {};
  try {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(SELECTOR_CONFIG_KEY, JSON.stringify(payload));
    }
  } catch (_) { /* noop */ }
  _customCache.clear(); // 配置已更新，丢弃会话内缓存（下一次 mergeSelectors 重读）
  return new Promise((resolve) => {
    try {
      if (typeof chrome !== 'undefined' && chrome.storage && chrome.storage.local) {
        chrome.storage.local.set({ [SELECTOR_CONFIG_KEY]: payload }, () => resolve({ ok: true }));
        return;
      }
    } catch (_) { /* noop */ }
    resolve({ ok: true });
  });
}

// 同步读取某渠道的用户自定义选择器；无配置返回 null（回退渠道默认 SEL）
// 配置结构：{ [channel]: { conversationList: 'sel', messageItem: 'sel', ... } }
export function getCustomSelectors(channel) {
  if (!channel) return null;
  const k = `channel:${channel}`;
  if (_customCache.has(k)) return _customCache.get(k);
  const all = _readFromStorage();
  const cfg = (all && all[channel]) || null;
  if (cfg) _customCache.set(k, cfg);
  return cfg;
}

// 获取某渠道自定义的「会话列表项」选择器（逗号分隔 → 数组）；无配置返回空数组。
// 供渠道 getConversationList 优先使用（用户人工识别 HTML 后填写即生效）。
export function customConversationListSelectors(channel) {
  const cfg = getCustomSelectors(channel);
  if (cfg && cfg.conversationList) {
    return String(cfg.conversationList).split(',').map((s) => s.trim()).filter(Boolean);
  }
  return [];
}

// 清空某渠道自定义选择器缓存（配置更新后调用）
export function _invalidateCustom(channel) {
  if (channel) _customCache.delete(`channel:${channel}`);
  else _customCache.clear();
}

// 合并选择器（纯规则）：用户自定义配置优先，渠道 SEL 默认兜底。
// 返回 { itemSelectors, listSelectors, textSelectors, inputSelectors, sendSelectors, selfMarkers, otherMarkers }
// 人工识别 HTML 后通过 UI 配置面板填写 user 配置；未填的字段自动用渠道默认 SEL。
export function mergeSelectors(channel, ruleFallbacks) {
  const out = {
    itemSelectors: [],
    listSelectors: [],
    textSelectors: [],
    inputSelectors: [],
    sendSelectors: [],
    selfMarkers: [],
    otherMarkers: [],
  };
  const cfg = getCustomSelectors(channel);
  const pick = (field) => {
    if (cfg && cfg[field]) {
      return String(cfg[field]).split(',').map((s) => s.trim()).filter(Boolean);
    }
    return [];
  };
  out.itemSelectors = pick('messageItem');
  out.listSelectors = pick('messageList');
  out.textSelectors = pick('text');
  out.inputSelectors = pick('input');
  out.sendSelectors = pick('send');
  out.selfMarkers = pick('selfMarker');
  out.otherMarkers = pick('otherMarker');
  // 规则兜底追加在后（用户配置在前，SEL 默认在后）
  if (ruleFallbacks) {
    out.itemSelectors = out.itemSelectors.concat(ruleFallbacks.itemSelectors || []);
    out.listSelectors = out.listSelectors.concat(ruleFallbacks.listSelectors || []);
    out.textSelectors = out.textSelectors.concat(ruleFallbacks.textSelectors || []);
    out.inputSelectors = out.inputSelectors.concat(ruleFallbacks.inputSelectors || []);
    out.sendSelectors = out.sendSelectors.concat(ruleFallbacks.sendSelectors || []);
    out.selfMarkers = out.selfMarkers.concat(ruleFallbacks.selfMarkers || []);
    out.otherMarkers = out.otherMarkers.concat(ruleFallbacks.otherMarkers || []);
  }
  // 去重（用户配置与默认可能重复）
  const dedupe = (arr) => Array.from(new Set(arr));
  out.itemSelectors = dedupe(out.itemSelectors);
  out.listSelectors = dedupe(out.listSelectors);
  out.textSelectors = dedupe(out.textSelectors);
  out.inputSelectors = dedupe(out.inputSelectors);
  out.sendSelectors = dedupe(out.sendSelectors);
  out.selfMarkers = dedupe(out.selfMarkers);
  out.otherMarkers = dedupe(out.otherMarkers);
  return out;
}

// 异步水合：从 chrome.storage.local（popup 写入的共享配置）同步到当前页 localStorage 镜像 +
// 清空内存缓存。content script 在初始化 / 收到 selectorsUpdated 广播时调用，确保 popup 配置
// 在抖音/小红书页面内同步生效（content script 的 getCustomSelectors 同步读 localStorage，故需此镜像）。
export async function hydrateSelectors() {
  try {
    if (typeof chrome === 'undefined' || !chrome.storage || !chrome.storage.local) return null;
    const res = await new Promise((resolve) => {
      try { chrome.storage.local.get(SELECTOR_CONFIG_KEY, (r) => resolve(r)); }
      catch (_) { resolve(null); }
    });
    const payload = res && res[SELECTOR_CONFIG_KEY];
    if (payload && typeof payload === 'object') {
      try {
        if (typeof localStorage !== 'undefined') localStorage.setItem(SELECTOR_CONFIG_KEY, JSON.stringify(payload));
      } catch (_) { /* noop */ }
    } else {
      // chrome.storage 无配置：清空 localStorage 镜像，避免旧配置残留
      try { if (typeof localStorage !== 'undefined') localStorage.removeItem(SELECTOR_CONFIG_KEY); } catch (_) { /* noop */ }
    }
    _invalidateCustom(null);
    return payload || null;
  } catch (_) { return null; }
}

// 兼容旧调用：抽取器动作（已无 extractor，返回 null）
export function getExtractorActions(channel, domain) {
  return null;
}

// 兼容旧调用：清空缓存（现在只清自定义选择器缓存）
export function clearExtractorResultCache(channel, domain) {
  _invalidateCustom(channel);
}

// 兼容测试：清空内存缓存
export function _resetMemoryCache() {
  _customCache.clear();
  try {
    if (typeof localStorage !== 'undefined') localStorage.removeItem(SELECTOR_CONFIG_KEY);
  } catch (_) { /* noop */ }
}

export const _internal = { SELECTOR_FIELDS };
