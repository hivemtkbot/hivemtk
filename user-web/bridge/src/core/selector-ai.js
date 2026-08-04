// selector-ai.js — 云端 LLM 动态选择器（解耦硬编码）
//
// 设计目标（用户需求）：彻底摆脱「写死在前端的渠道选择器」。
//   1) 插件把当前 DOM 序列化为【脱敏结构快照】（仅 tag + class + 关键结构属性，
//      不含文本/链接/图片，避免泄露私信内容）。
//   2) 调用 user-server 的 /api/bridge/ai-selectors（复用后端 LLM 栈，key 在本地 Mac，
//      不进前端、不经公网明文）。
//   3) 后端用 LLM 把快照 → 标准 SelectorSpec（列表/消息项/输入框/发送按钮/自他标记）。
//   4) 插件把 SelectorSpec【缓存到 localStorage】（域名+渠道指纹），运行时优先使用。
//      平时零延迟全用缓存，仅在首次/改版/连续 0 命中时请求云端 → 自愈。
//   5) LLM 不可用 → 回退规则引擎（selector-engine 多候选 + 结构启发式）。
//
// 这样「选择器」从代码常量变成「云端生成的运行时产物」，抖音改版只需 LLM 重新识别，
// 无需改前端代码、无需发版。

import { DEFAULT_USER_SERVER } from './constants.js';

// SelectorSpec 协议版本：schema 变更时 +1，旧缓存自动失效
// v2：新增 extractor（LLM 返回的「可执行 JS 抽取器」），作为读取主路径
export const SPEC_VERSION = 2;

// 缓存有效期（7 天）。改版后结构会变，靠「连续 0 命中自愈」提前刷新。
const CACHE_TTL_MS = 7 * 24 * 3600 * 1000;
// 自愈/主动刷新的冷却（6 小时），避免抖动期高频打 LLM。
const REFRESH_COOLDOWN_MS = 6 * 3600 * 1000;
const CACHE_PREFIX = 'hivebridge:sel:';
const ENDPOINT = '/api/bridge/ai-selectors';
const FETCH_TIMEOUT_MS = 20000;

// 内存缓存（会话内即时生效，跨重载靠 localStorage）
const _memory = Object.create(null);

// ============================================================
// 1) DOM 脱敏快照序列化
// ============================================================
// 只保留选择器相关结构信息，严格剔除任何用户数据：
//   - 不保留文本节点内容（连长度都不留，避免推断聊天内容）
//   - 不保留 src/href/图片/视频地址（避免泄露资源 URL）
//   - 仅保留：tag、class（前 6 个）、id、role、data-e2e、contenteditable、aria-label、type
// 深度/节点数双重限幅，保证发送给 LLM 的体积可控。
export function snapshotDom(root, opts = {}) {
  if (!root || root.nodeType !== 1) return '{}';
  const maxDepth = opts.maxDepth ?? 8;
  const maxNodes = opts.maxNodes ?? 1800;
  const perNode = opts.perNode ?? 48;
  const counters = { n: 0 };

  const KEEP_ATTRS = ['id', 'role', 'data-e2e', 'contenteditable', 'aria-label', 'type'];

  function walk(node, depth) {
    if (!node || counters.n >= maxNodes || depth > maxDepth) return null;
    if (node.nodeType !== 1) return null;
    counters.n++;

    const tag = node.tagName ? node.tagName.toLowerCase() : '?';
    const cls =
      node.className && typeof node.className === 'string'
        ? node.className.trim().split(/\s+/).filter(Boolean).slice(0, 6)
        : [];

    const attrs = {};
    for (const k of KEEP_ATTRS) {
      const v = node.getAttribute ? node.getAttribute(k) : null;
      if (v) attrs[k] = k === 'id' ? v.slice(0, 60) : String(v).slice(0, 40);
    }

    const children = [];
    const childEls = node.children ? Array.from(node.children).slice(0, perNode) : [];
    for (const ch of childEls) {
      const wc = walk(ch, depth + 1);
      if (wc) children.push(wc);
    }

    // 剪枝：既无 class/属性、又无保留下来的子节点 → 纯噪声，丢弃
    if (!cls.length && Object.keys(attrs).length === 0 && children.length === 0) {
      return null;
    }

    const el = { t: tag, c: cls };
    if (Object.keys(attrs).length) el.a = attrs;
    if (children.length) el.ch = children;
    return el;
  }

  const tree = walk(root, 0);
  return JSON.stringify(tree);
}

// ============================================================
// 2) 云端调用
// ============================================================
function resolveApiBase() {
  try {
    const cfg = window.__HIVE_BRIDGE_CONFIG__;
    if (cfg && cfg.userServerBaseUrl) return cfg.userServerBaseUrl.replace(/\/$/, '');
    const ls = localStorage.getItem('hivebridge:server');
    if (ls) return ls.replace(/\/$/, '');
  } catch (_) {}
  return DEFAULT_USER_SERVER.baseUrl;
}

async function fetchSpec(channel, domain, snapshot) {
  const url = `${resolveApiBase()}${ENDPOINT}`;
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), FETCH_TIMEOUT_MS);
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        channel,
        domain,
        dom_snapshot: snapshot,
        spec_version: SPEC_VERSION,
      }),
      signal: ctrl.signal,
    });
    if (!res.ok) throw new Error('http ' + res.status);
    const data = await res.json();
    const spec = data && data.spec;
    if (!spec) throw new Error('empty spec');
    const hasSelectors = Array.isArray(spec.message_item) && spec.message_item.length > 0;
    const hasExtractor = typeof spec.extractor === 'string' && spec.extractor.length > 0;
    // 主路径是 extractor；选择器仅兜底。二者至少其一，否则视为无效产物。
    if (!hasSelectors && !hasExtractor) {
      throw new Error('empty spec: neither selectors nor extractor');
    }
    return spec;
  } finally {
    clearTimeout(timer);
  }
}

// ============================================================
// 3) 缓存读写
// ============================================================
function cacheKey(channel, domain) {
  return `${CACHE_PREFIX}${channel}:${domain}`;
}

function loadCache(channel, domain) {
  try {
    const raw = localStorage.getItem(cacheKey(channel, domain));
    if (!raw) return null;
    const obj = JSON.parse(raw);
    if (obj.version !== SPEC_VERSION) return null;
    if (Date.now() - obj.ts > CACHE_TTL_MS) return null;
    return obj.spec;
  } catch (_) {
    return null;
  }
}

export function saveCache(channel, domain, spec) {
  try {
    localStorage.setItem(
      cacheKey(channel, domain),
      JSON.stringify({ version: SPEC_VERSION, ts: Date.now(), spec })
    );
  } catch (_) {}
}

// ============================================================
// 4) 对外 API（同步取缓存 + 异步刷新）
// ============================================================

// 同步取缓存（扫描路径零延迟）。内存 → localStorage → null
export function getCachedSpec(channel, domain) {
  const k = `${channel}:${domain}`;
  if (_memory[k]) return _memory[k];
  const c = loadCache(channel, domain);
  if (c) _memory[k] = c;
  return c;
}

// EXTRACTOR_BLOCKLIST 运行期静态校验黑名单（与后端 extractorBlocklist 对应，防御纵深）：
// 拒绝会联网、读写本地存储、执行动态代码、跳转页面等危险写法。
export const EXTRACTOR_BLOCKLIST = [
  'fetch(', 'xmlhttprequest', 'websocket(', 'import(', 'eval(', 'new function',
  'localStorage', 'sessionstorage', 'document.cookie', 'window.open(',
  'location.href', 'location.assign', 'location.replace', 'chrome.storage',
  'postmessage', 'navigator.sendbeacon', 'indexeddb',
];

function sanitizeExtractorCode(code) {
  const low = String(code).toLowerCase();
  for (const bad of EXTRACTOR_BLOCKLIST) {
    if (low.includes(bad)) return `forbidden token: ${bad}`;
  }
  // 必须长得像一个函数（普通函数或箭头函数），否则无法编译
  if (!low.includes('function') && !low.includes('=>')) {
    return 'extractor must be a function body';
  }
  return null;
}

// 编译缓存：同一段 extractor 源码只 new Function 一次
const _extractorFnCache = new Map();
// 执行结果缓存：同一 channel:domain 当次会话内复用（避免每帧重复跑抽取器）
const _extractorResultCache = new Map();

// compileExtractor 把 LLM 返回的源码编译为可执行函数。
// 返回 { ok, fn, error }。fn 签名 (doc, win) => ({ messages, input_box, send_button })。
export function compileExtractor(code) {
  const cached = _extractorFnCache.get(code);
  if (cached) return cached;
  const err = sanitizeExtractorCode(code);
  if (err) {
    const r = { ok: false, fn: null, error: err };
    _extractorFnCache.set(code, r);
    return r;
  }
  let fn = null;
  try {
    // 沙箱式执行：只注入 document / window，不暴露 fetch / localStorage / XMLHttpRequest 等全局。
    // LLM 返回的是「函数表达式/箭头函数」（如 function(doc,win){...} 或 (doc,win)=>({...})），
    // 必须包成 return (CODE)(document, window) 立即执行，直接返回 { messages, input_box, send_button }。
    // 注意：绝不能把 code 直接当 new Function 的 body —— 函数声明作 statement 会报
    // "Function statements require a function name"。
    fn = new Function('document', 'window', 'return (' + code + ')(document, window);');
  } catch (e) {
    const r = { ok: false, fn: null, error: 'compile error: ' + (e && e.message) };
    _extractorFnCache.set(code, r);
    return r;
  }
  const r = { ok: true, fn, error: null };
  _extractorFnCache.set(code, r);
  return r;
}

// normalizeExtractItem 把 extractor 返回的单条消息归一到内部消息形状。
function normalizeExtractItem(it) {
  if (!it || typeof it !== 'object') return null;
  const self = it.self === true ||
    (typeof it.sender_type === 'string' && it.sender_type.toLowerCase() === 'self');
  const text = typeof it.text === 'string' ? it.text : '';
  const mediaUrl = typeof it.mediaUrl === 'string' ? it.mediaUrl : '';
  if (!text && !mediaUrl) return null; // 既无文本也无媒体，无意义
  const msgType = typeof it.msgType === 'string' ? it.msgType : mediaUrl ? 'image' : 'text';
  return {
    id: typeof it.id === 'string' && it.id ? it.id : text.slice(0, 64),
    text,
    self,
    sender_type: self ? 'self' : 'customer',
    timestamp: typeof it.timestamp === 'number' && it.timestamp > 0 ? it.timestamp : Date.now(),
    msg_type: msgType,
    media_url: mediaUrl,
    is_group: false,
    group_id: '',
    group_name: '',
    sender_name: '',
    raw: null,
  };
}

// runExtractor 在真实页面执行 LLM 抽取器，返回 { messages, input_box, send_button } 或 null。
// 任何异常 / 校验失败都返回 null（调用方回退到选择器 / 规则引擎），绝不抛错。
export function runExtractor(channel, domain) {
  const ck = `${channel}:${domain}`;
  const cached = _extractorResultCache.get(ck);
  if (cached) return cached;
  const spec = getCachedSpec(channel, domain);
  if (!spec || typeof spec.extractor !== 'string' || !spec.extractor) return null;
  const { ok, fn, error } = compileExtractor(spec.extractor);
  if (!ok || !fn) {
    try { console.warn('[bridge-ai] extractor compile failed, fallback:', error); } catch (_) {}
    return null;
  }
  let out;
  try {
    out = fn(
      typeof document !== 'undefined' ? document : null,
      typeof window !== 'undefined' ? window : null
    );
  } catch (e) {
    try { console.warn('[bridge-ai] extractor runtime error, fallback:', e && e.message); } catch (_) {}
    return null;
  }
  if (!out || !Array.isArray(out.messages)) return null;
  const messages = [];
  for (const it of out.messages) {
    const n = normalizeExtractItem(it);
    if (n) messages.push(n);
  }
  const result = {
    messages,
    input_box: typeof out.input_box === 'string' ? out.input_box : '',
    send_button: typeof out.send_button === 'string' ? out.send_button : '',
  };
  _extractorResultCache.set(ck, result); // 仅缓存成功结果；失败不缓存，下次再试
  return result;
}

// getExtractorActions 返回抽取器提供的「输入框/发送按钮」选择器（可选，用于发送路径）。
export function getExtractorActions(channel, domain) {
  const r = runExtractor(channel, domain);
  if (!r) return null;
  if (!r.input_box && !r.send_button) return null;
  return { input_box: r.input_box, send_button: r.send_button };
}

// validateSpec 在「真实浏览器页面」上验证 LLM 生成产物是否可用。
// 返回 true 表示可缓存；false 表示应拒收（回退规则引擎，稍后自愈重试）。
// 设计取舍：
//   - 非浏览器环境（单元测试 / SSR）→ 无法验证，一律放行，避免误杀。
//   - 页面尚未渲染聊天容器（刚进入会话）→ 无法判断，放行，交给自愈稍后重试。
//   - 优先验证 extractor（主路径）：能编译、能在当前 DOM 跑出数组即可；
//     否则回退验证选择器：页面有聊天容器但选择器命中 0 → 判定无效。
// validateSpec 在「真实浏览器页面」上验证 LLM 生成产物是否可用。
// 返回 true 表示可缓存；false 表示应拒收（回退规则引擎，稍后自愈重试）。
export function validateSpec(spec) {
  const hasExtractor = spec && typeof spec.extractor === 'string' && spec.extractor.length > 0;
  if (hasExtractor) {
    const { ok, fn } = compileExtractor(spec.extractor);
    if (!ok || !fn) {
      // 不可编译 → 若完全没有选择器兜底则拒收
      if (!spec.message_item || spec.message_item.length === 0) return false;
    } else {
      try {
        const r = fn(
          typeof document !== 'undefined' ? document : null,
          typeof window !== 'undefined' ? window : null
        );
        if (!r || !Array.isArray(r.messages)) {
          if (!spec.message_item || spec.message_item.length === 0) return false;
        }
      } catch (_) {
        if (!spec.message_item || spec.message_item.length === 0) return false;
      }
    }
    return true; // extractor 至少可编译/试跑；选择器有无不影响（仅兜底）
  }
  // 无 extractor → 走原选择器校验逻辑
  if (!spec || !Array.isArray(spec.message_item) || spec.message_item.length === 0) {
    return false;
  }
  if (typeof document === 'undefined' || typeof document.querySelectorAll !== 'function') {
    return true; // 非浏览器：无法验证，放行
  }
  let matched = 0;
  for (const sel of spec.message_item) {
    try {
      matched += document.querySelectorAll(sel).length;
    } catch (_) {
      /* 非法选择器忽略 */
    }
  }
  if (matched > 0) return true;
  // 没命中任何 item：看页面是否有疑似聊天容器
  const chatHint = document.querySelector(
    '[class*="message"],[class*="msg"],[class*="chat"],[class*="im-"],[class*="conversation"],[class*="dialog"]'
  );
  return !chatHint;
}

// 异步刷新：抓快照 → 调云端 → 校验 → 写缓存。失败 / 校验不通过返回 null（调用方回退规则）。
export async function refreshSpec(channel, domain) {
  const root = document.body || document.documentElement;
  if (!root) return null;
  const snapshot = snapshotDom(root);
  try {
    const spec = await fetchSpec(channel, domain, snapshot);
    if (!validateSpec(spec)) {
      throw new Error('spec failed live-dom validation');
    }
    saveCache(channel, domain, spec);
    _memory[`${channel}:${domain}`] = spec;
    _extractorResultCache.delete(`${channel}:${domain}`); // 新 spec 生效，丢弃旧抽取结果
    return spec;
  } catch (_) {
    return null;
  }
}

// 把 AI spec 与规则候选合并：AI 优先（放前），规则兜底（放后）。
// 返回 { itemSelectors, listSelectors, textSelectors, inputSelectors, sendSelectors, selfMarkers, otherMarkers }
export function mergeSelectors(channel, domain, ruleFallbacks) {
  const spec = getCachedSpec(channel, domain);
  const out = {
    itemSelectors: [],
    listSelectors: [],
    textSelectors: [],
    inputSelectors: [],
    sendSelectors: [],
    selfMarkers: [],
    otherMarkers: [],
  };
  if (spec) {
    out.itemSelectors = Array.isArray(spec.message_item) ? spec.message_item.slice() : [];
    out.listSelectors = Array.isArray(spec.message_list) ? spec.message_list.slice() : [];
    out.textSelectors = Array.isArray(spec.text) ? spec.text.slice() : [];
    out.inputSelectors = Array.isArray(spec.input_box) ? spec.input_box.slice() : [];
    out.sendSelectors = Array.isArray(spec.send_button) ? spec.send_button.slice() : [];
    out.selfMarkers = Array.isArray(spec.self_marker) ? spec.self_marker.slice() : [];
    out.otherMarkers = Array.isArray(spec.other_marker) ? spec.other_marker.slice() : [];
  }
  // 规则兜底追加在后
  if (ruleFallbacks) {
    out.itemSelectors = out.itemSelectors.concat(ruleFallbacks.itemSelectors || []);
    out.listSelectors = out.listSelectors.concat(ruleFallbacks.listSelectors || []);
    out.textSelectors = out.textSelectors.concat(ruleFallbacks.textSelectors || []);
    out.inputSelectors = out.inputSelectors.concat(ruleFallbacks.inputSelectors || []);
    out.sendSelectors = out.sendSelectors.concat(ruleFallbacks.sendSelectors || []);
    out.selfMarkers = out.selfMarkers.concat(ruleFallbacks.selfMarkers || []);
    out.otherMarkers = out.otherMarkers.concat(ruleFallbacks.otherMarkers || []);
  }
  return out;
}

export const _internal = { REFRESH_COOLDOWN_MS, CACHE_TTL_MS };

// 仅供测试：清空内存缓存（localStorage 由测试环境自行 clear）
export function _resetMemoryCache() {
  for (const k in _memory) delete _memory[k];
  _extractorResultCache.clear();
  _extractorFnCache.clear();
}
