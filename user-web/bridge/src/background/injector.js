// 内容脚本自动注入器（auto-injector）
//
// 解决"Could not establish connection. Receiving end does not exist."的 4 类根因：
//   1. 扩展被禁用 → 由用户在 chrome://extensions 处理，本模块无能为力
//   2. content script 尚未执行 → 本模块通过 programmatic 注入兜底
//   3. 标签页在扩展加载前已打开 → 本模块在 webNavigation.onCommitted 时主动注入
//   4. 页面在 iframe 中 → 注入时传 allFrames: true 覆盖（仅对已支持 host）
//
// 设计要点：
//   - URL → 注入文件 的映射集中管理，禁止字面量散落
//   - 注入函数带幂等保护：同 (tabId, file) 短时间内重复请求不重复注入
//   - 注入结果通过 callback 回调，符合现有 popup 异步风格
//
// 文档源：user-server/docs/dev/DEVELOPMENT.md 端口对照表
//         + bridge/bridge.md §17 协议规范

// URL → content script 文件 的路由
// 顺序：先匹配具体平台域，最后 fallback 到 'common'（仅 ping/selfcheck）
// 调整需同步 manifest.json content_scripts[*].js 与 build.mjs entries
//
// 2026-08-05 审计 P0 修复（H5 域名白名单）：
//   原实现漏了 xianyu/goofish 域名 → popup 自检自动注入 xianyu 永远走 unsupported_host 失败路径。
//   补全 xianyu.com / goofish.com（咸鱼两个域名共用 content-xianyu.js）。
//   并集中到 SUPPORTED_HOSTS 常量，供各处运行时校验复用（manifest matches + popup 域名判断）。
export const SUPPORTED_HOSTS = [
  'douyin.com',
  'xiaohongshu.com',
  'tiktok.com',
  'goofish.com',
  'xianyu.com',
];

// isSupportedHost：运行时域名校验。manifest.matches 是入口校验，
// 这里是代码层兜底（programmatic 注入 / popup 自检 / background 路由）。
// 双校验确保即使 manifest 误配也不会注入到非支持站点。
export function isSupportedHost(url) {
  if (!url) return false;
  try {
    const u = new URL(url);
    const host = u.hostname.toLowerCase();
    return SUPPORTED_HOSTS.some((d) => host === d || host.endsWith('.' + d));
  } catch (_) {
    return false;
  }
}

function pickContentScriptFile(url) {
  if (!url) return null;
  try {
    const u = new URL(url);
    const host = u.hostname.toLowerCase();
    if (/(^|\.)douyin\.com$/.test(host)) return 'content-douyin.js';
    if (/(^|\.)xiaohongshu\.com$/.test(host)) return 'content-xhs.js';
    if (/(^|\.)tiktok\.com$/.test(host)) return 'content-tiktok.js';
    // 2026-08-05 修复：闲鱼两个域名（xianyu.com + goofish.com）共用 content-xianyu.js
    if (/(^|\.)goofish\.com$/.test(host)) return 'content-xianyu.js';
    if (/(^|\.)xianyu\.com$/.test(host)) return 'content-xianyu.js';
    return null; // 非支持 host
  } catch (_) {
    return null;
  }
}

// 注入幂等保护：同 (tabId, file) 在 1500ms 内只注入一次
const RECENT_INJECTS = new Map(); // key -> timestamp
const INJECT_DEDUP_MS = 1500;

// 测试专用：清空 dedup 状态，避免用例间状态污染。
// 不在生产路径调用，仅供 vitest 用。
export function __resetInjectState() {
  RECENT_INJECTS.clear();
}

function _key(tabId, file) {
  return `${tabId}:${file}`;
}

function _isRecent(tabId, file) {
  const k = _key(tabId, file);
  const t = RECENT_INJECTS.get(k);
  if (!t) return false;
  if (Date.now() - t > INJECT_DEDUP_MS) {
    RECENT_INJECTS.delete(k);
    return false;
  }
  return true;
}

function _markRecent(tabId, file) {
  RECENT_INJECTS.set(_key(tabId, file), Date.now());
  // 定期清理避免内存泄漏
  if (RECENT_INJECTS.size > 500) {
    const now = Date.now();
    for (const [k, t] of RECENT_INJECTS.entries()) {
      if (now - t > INJECT_DEDUP_MS) RECENT_INJECTS.delete(k);
    }
  }
}

// ---- chrome API 兜底 ----
function lastError() {
  try {
    if (!chrome.runtime.lastError) return null;
    return chrome.runtime.lastError.message || '(无错误详情)';
  } catch (_) {
    return null;
  }
}

// 检查当前环境是否支持 chrome.scripting
export function scriptingAvailable() {
  return Boolean(
    typeof chrome !== 'undefined' &&
      chrome &&
      chrome.scripting &&
      typeof chrome.scripting.executeScript === 'function'
  );
}

// 主动注入 content script
//
// 参数：
//   tabId: 目标标签页
//   url:   标签页 URL（用于选择文件）
//   opts:  { allFrames?: boolean } 可选
//   cb:    (result) => void  回调
//     result = { ok, file?, reason?, error? }
export function injectContentScript(tabId, url, opts, cb) {
  // opts 可选，兼容旧调用
  if (typeof opts === 'function') {
    cb = opts;
    opts = {};
  }
  opts = opts || {};
  cb = cb || (() => {});

  if (!scriptingAvailable()) {
    cb({ ok: false, reason: 'scripting_api_unavailable' });
    return;
  }
  const file = pickContentScriptFile(url);
  if (!file) {
    cb({ ok: false, reason: 'unsupported_host' });
    return;
  }
  if (_isRecent(tabId, file)) {
    cb({ ok: true, file, dedup: true });
    return;
  }
  try {
    chrome.scripting.executeScript(
      {
        target: { tabId, allFrames: !!opts.allFrames },
        files: [file],
      },
      () => {
        const err = lastError();
        if (err) {
          cb({ ok: false, reason: 'execute_failed', error: err, file });
          return;
        }
        _markRecent(tabId, file);
        cb({ ok: true, file });
      }
    );
  } catch (e) {
    cb({ ok: false, reason: 'execute_threw', error: String(e), file });
  }
}

// 给 background 的 webNavigation.onCommitted 用的注入
// 与 popup 用同一份逻辑；自动注入后 content script 自身会建立端口
export function autoInjectOnCommit(details) {
  // 只处理主 frame 切换（tabId 一致 + frameId===0），避免对 iframe 重复注入
  if (!details || details.frameId !== 0) return;
  if (details.tabId < 0) return;
  if (!details.url) return;
  if (!pickContentScriptFile(details.url)) return;
  injectContentScript(details.tabId, details.url, { allFrames: false }, (r) => {
    // 静默：成功不打印避免日志噪音，失败记录 warn 便于排错
    if (!r.ok && r.reason !== 'unsupported_host') {
      // 注入失败不影响主流程，但需要可观测
      // background 的 log 在 background/index.js 注入，此处直接 console 即可
      // （MV3 SW console 会进 chrome://extensions 的 Service Worker inspect）
      // eslint-disable-next-line no-console
      console.warn('[auto-inject]', r.reason, r.error || '');
    }
  });
}

export { pickContentScriptFile };
// 说明：SUPPORTED_HOSTS / isSupportedHost 在声明处已用 `export const` / `export function` 导出，
// 不在此重复 re-export，避免 esbuild "Multiple exports with the same name" 错误。
