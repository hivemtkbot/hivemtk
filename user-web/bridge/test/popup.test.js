// popup 单元测试：覆盖 normalizeServerUrl / testConnection / showBanner / lastError
//                / diagnoseUninjected / selfCheck 全流程
//
// 设计要点：
//   - popup 源码含 ESM `import { ... } from '../core/constants.js'`，`new Function`
//     不能解析 import 语句，所以用 esbuild 把入口打成 IIFE，再注入到测试桩
//   - 这样测试跑的是"实际跑在扩展里"的那段代码，避免 mock 偏离实现
//   - 全部 it() 改为 async，因 esbuild.build 是异步；首次调用会缓存打包结果
//   - 注入 chrome / document / window / AbortController / fetch 五个全局桩
//
// 环境约束：
//   - 强制 node 环境而非默认 jsdom：esbuild 内部依赖 `new TextEncoder().encode(...) instanceof Uint8Array`
//     的不变量，jsdom 重写了 TextEncoder 会导致 esbuild 直接抛 "Invariant violation"。
//     popup 本身只用 document/window/chrome 三个全局，本测试在 new Function 中自注入，不依赖 jsdom。
//
// 测试场景（与 popup.js 协议严格对齐）：
//   1) normalizeServerUrl：空 / 空格 / 自动补 http / https / 尾斜杠
//   2) testConnection：空 / 2xx / 5xx 降级 / 404 试下一路径 / 4xx 立即返回 / 网络错
//   3) showBanner / clearBanner：DOM 写入与 className 切换
//   4) lastError：null / 有 message / message=undefined 兜底 / chrome 抛错
//   5) diagnoseUninjected：典型 Chrome 错 + 抖音域名 / 非支持域名 / 非典型错 / null
//   6) selfCheck 两步探活：A. 域名不在白名单 / A. ping 失败 / 极端 lastError 兜底
//                         B. ping OK + matched=false / C. ping OK + matched=true
//
// @vitest-environment node

import { describe, it, expect, vi } from 'vitest';
import { build } from 'esbuild';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const popupEntry = resolve(__dirname, '../src/popup/index.js');

// 用 esbuild 一次性把 popup（含 constants.js）打成单文件 IIFE 并缓存。
// 后续 loadPopup 复用同一份产物，避免每个用例重复打包拖慢测试。
let _bundledPromise = null;
function bundlePopup() {
  if (_bundledPromise) return _bundledPromise;
  _bundledPromise = build({
    entryPoints: [popupEntry],
    bundle: true,
    format: 'iife',
    write: false,
    platform: 'browser',
    target: 'es2020',
    logLevel: 'silent',
  }).then((r) => r.outputFiles[0].text);
  return _bundledPromise;
}

// 加载 popup 模块（暴露 window.__popup）
// globals: { chrome, fetch, AbortController? } —— 不传则用默认桩
async function loadPopup(globals = {}) {
  const popupSrc = await bundlePopup();
  // 构造 jsdom 风格的 window/document
  const elements = {};
  const mk = (id) => {
    if (!elements[id]) {
      elements[id] = {
        id,
        className: '',
        textContent: '',
        innerHTML: '',
        value: '',
        placeholder: '',
        disabled: false,
        children: [],
        addEventListener: () => {},
        appendChild: function (n) {
          // 同步更新 children + textContent（便于断言）
          this.children.push(n);
          if (n && typeof n.textContent === 'string') {
            this.textContent = (this.textContent || '') + n.textContent;
          }
          return n;
        },
        focus: () => {},
        dispatchEvent: () => {},
      };
    }
    return elements[id];
  };
  const mkChild = (tag) => ({
    tagName: tag,
    className: '',
    textContent: '',
    appendChild: (n) => n,
  });
  const document = {
    getElementById: (id) => mk(id),
    createElement: (tag) => mkChild(tag),
    addEventListener: () => {},
  };
  const window = { __popup: null };
  const ctx = {
    document,
    window,
    chrome: globals.chrome || {},
    AbortController: globals.AbortController || AbortController,
    fetch: globals.fetch || (() => Promise.resolve({ ok: false })),
    setTimeout,
    clearTimeout,
    console,
  };
  // 把打包后的 IIFE 注入到 new Function 执行；用对象 keys 顺序传 ctx 保证顺序稳定
  const fn = new Function(
    ...Object.keys(ctx),
    `${popupSrc}\nreturn typeof window !== "undefined" ? window.__popup : null;`
  );
  const exported = fn(...Object.values(ctx));
  return { exported, elements, window, document };
}

describe('popup normalizeServerUrl', () => {
  it('空字符串返回空', async () => {
    const { exported } = await loadPopup({});
    expect(exported.normalizeServerUrl('')).toBe('');
    expect(exported.normalizeServerUrl(null)).toBe('');
    expect(exported.normalizeServerUrl(undefined)).toBe('');
  });

  it('去除首尾空格', async () => {
    const { exported } = await loadPopup({});
    expect(exported.normalizeServerUrl('  http://x:8204  ')).toBe('http://x:8204');
  });

  it('自动补 http://', async () => {
    const { exported } = await loadPopup({});
    expect(exported.normalizeServerUrl('localhost:8204')).toBe('http://localhost:8204');
    expect(exported.normalizeServerUrl('192.168.1.1:8204/path')).toBe('http://192.168.1.1:8204/path');
  });

  it('保留 https', async () => {
    const { exported } = await loadPopup({});
    expect(exported.normalizeServerUrl('https://example.com:8204')).toBe('https://example.com:8204');
  });

  it('去除尾斜杠', async () => {
    const { exported } = await loadPopup({});
    expect(exported.normalizeServerUrl('http://x:8204/')).toBe('http://x:8204');
    expect(exported.normalizeServerUrl('http://x:8204///')).toBe('http://x:8204');
    expect(exported.normalizeServerUrl('http://x:8204/path/')).toBe('http://x:8204/path');
  });
});

describe('popup testConnection', () => {
  it('空 URL 返回 empty', async () => {
    const { exported } = await loadPopup({});
    const r = await exported.testConnection('');
    expect(r.ok).toBe(false);
    expect(r.reason).toBe('empty');
  });

  it('2xx 返回 ok=true', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const { exported } = await loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:8204');
    expect(r.ok).toBe(true);
    expect(r.degraded).toBe(false);
    expect(r.status).toBe(200);
    expect(r.url).toBe('http://localhost:8204/health');
  });

  it('503 返回 ok=true degraded=true', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 503 });
    const { exported } = await loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:8204');
    expect(r.ok).toBe(true);
    expect(r.degraded).toBe(true);
    expect(r.status).toBe(503);
  });

  it('404 跳过当前路径，尝试下一个', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 404 })
      .mockResolvedValueOnce({ ok: true, status: 200 });
    const { exported } = await loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:8204');
    expect(r.ok).toBe(true);
    expect(r.url).toBe('http://localhost:8204/healthz');
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('401/403 立即返回 http_xxx', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 401 });
    const { exported } = await loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:8204');
    expect(r.ok).toBe(false);
    expect(r.reason).toBe('http_401');
    expect(r.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('网络错误：所有路径都失败时返回 unreachable', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'));
    const { exported } = await loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:9999');
    expect(r.ok).toBe(false);
    expect(r.reason).toBe('unreachable');
    expect(r.detail).toContain('Failed to fetch');
    expect(fetchMock).toHaveBeenCalledTimes(4); // 4 个候选
  });

  it('第一个失败第二个成功', async () => {
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(new Error('ECONNREFUSED'))
      .mockResolvedValueOnce({ ok: true, status: 200 });
    const { exported } = await loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:8204');
    expect(r.ok).toBe(true);
    expect(r.url).toBe('http://localhost:8204/healthz');
  });
});

describe('popup showBanner', () => {
  it('设置 className 与内容', async () => {
    const { exported, elements } = await loadPopup({});
    exported.showBanner('success', '标题', '内容');
    const el = elements['banner'];
    expect(el.className).toContain('show');
    expect(el.className).toContain('success');
    // children appended
    expect(el.children.length).toBe(2);
    expect(el.children[0].className).toBe('title');
    expect(el.children[0].textContent).toBe('标题');
    expect(el.children[1].textContent).toBe('内容');
  });

  it('不同 kind 切换 className', async () => {
    const { exported, elements } = await loadPopup({});
    exported.showBanner('error', 'X', 'Y');
    expect(elements['banner'].className).toContain('error');
    exported.showBanner('warn', 'X', 'Y');
    expect(elements['banner'].className).toContain('warn');
    exported.showBanner('info', 'X', 'Y');
    expect(elements['banner'].className).toContain('info');
  });

  it('clearBanner 重置 className', async () => {
    const { exported, elements } = await loadPopup({});
    exported.showBanner('error', 'X', 'Y');
    exported.clearBanner();
    expect(elements['banner'].className).toBe('banner');
  });
});

// ---- 新增：lastError / diagnoseUninjected / selfcheck 两步探活 ----

describe('popup lastError', () => {
  it('无 lastError 时返回 null', async () => {
    const { exported } = await loadPopup({ chrome: { runtime: { lastError: null } } });
    expect(exported.lastError()).toBeNull();
  });

  it('有 lastError.message 时返回字符串（不解包对象）', async () => {
    const { exported } = await loadPopup({
      chrome: { runtime: { lastError: { message: 'Could not establish connection. Receiving end does not exist.' } } },
    });
    expect(exported.lastError()).toBe('Could not establish connection. Receiving end does not exist.');
  });

  // 修复回归：早期 lastError 返回整个对象，调用方又取 .message → undefined。
  it('lastError.message 为 undefined 时返回 "(无错误详情)" 兜底，避免 popup 显示 "undefined"', async () => {
    const { exported } = await loadPopup({
      chrome: { runtime: { lastError: { message: undefined } } },
    });
    const r = exported.lastError();
    expect(r).toBe('(无错误详情)');
    expect(r).not.toBeUndefined();
  });

  it('chrome.runtime 抛错时返回 null', async () => {
    const { exported } = await loadPopup({
      chrome: { runtime: { get lastError() { throw new Error('forbidden'); } } },
    });
    expect(exported.lastError()).toBeNull();
  });
});

describe('popup diagnoseUninjected', () => {
  it('典型 Chrome 错误 + 抖音域名 → 提示扩展禁用/未注入/旧标签页/iframe', async () => {
    const { exported } = await loadPopup({});
    const hint = exported.diagnoseUninjected('Could not establish connection. Receiving end does not exist.', 'https://www.douyin.com/');
    expect(hint).toContain('扩展未加载');
    expect(hint).toContain('Ctrl+Shift+R');
    expect(hint).toContain('iframe');
  });

  it('典型 Chrome 错误 + 非支持域名 → 提示 URL 不在 manifest', async () => {
    const { exported } = await loadPopup({});
    const hint = exported.diagnoseUninjected('Could not establish connection. Receiving end does not exist.', 'https://example.com/');
    expect(hint).toContain('manifest matches');
    expect(hint).not.toContain('iframe');
  });

  it('非典型错误 → 走通用兜底文案', async () => {
    const { exported } = await loadPopup({});
    const hint = exported.diagnoseUninjected('Some weird error', 'https://www.douyin.com/');
    expect(hint).toContain('扩展未加载');
    expect(hint).toContain('content script 尚未执行');
  });

  it('errMsg 为 null 时仍走通用兜底', async () => {
    const { exported } = await loadPopup({});
    const hint = exported.diagnoseUninjected(null, 'https://www.douyin.com/');
    expect(hint).toContain('扩展未加载');
  });
});

describe('popup selfCheck 流程', () => {
  // 工具：构造一个 fake chrome API
  function makeChrome({ tab, pingResp, selfcheckResp, pingError, selfcheckError }) {
    const calls = [];
    const fake = {
      calls,
      tabs: {
        query: (_q, cb) => cb([tab]),
        sendMessage: (id, msg, cb) => {
          calls.push({ id, msg });
          if (msg.type === 'ping') {
            if (pingError) {
              // 模拟 lastError 状态在回调前被设置
              setTimeout(() => {
                Object.defineProperty(fake.runtime, 'lastError', { value: pingError, configurable: true });
                cb(undefined);
                Object.defineProperty(fake.runtime, 'lastError', { value: null, configurable: true });
              }, 0);
            } else {
              cb(pingResp);
            }
          } else if (msg.type === 'selfcheck') {
            if (selfcheckError) {
              setTimeout(() => {
                Object.defineProperty(fake.runtime, 'lastError', { value: selfcheckError, configurable: true });
                cb(undefined);
                Object.defineProperty(fake.runtime, 'lastError', { value: null, configurable: true });
              }, 0);
            } else {
              cb(selfcheckResp);
            }
          } else {
            cb(undefined);
          }
        },
      },
      runtime: { lastError: null },
    };
    return fake;
  }

  // 工具：等 setTimeout 0 触发完成
  const flush = () => new Promise((r) => setTimeout(r, 5));

  it('A. 域名不在白名单：直接提示非支持域名，不发任何消息', async () => {
    const tab = { id: 1, url: 'https://example.com/' };
    const chrome = makeChrome({ tab });
    const { exported, elements } = await loadPopup({ chrome });
    exported.selfCheck();
    expect(chrome.calls).toEqual([]);
    expect(elements['selfOut'].textContent).toContain('example.com');
    expect(elements['selfOut'].textContent).toContain('抖音/小红书/TikTok');
  });

  it('A. ping 失败（典型 Chrome 错误）→ 输出诊断文案，不再显示 "undefined"', async () => {
    const tab = { id: 1, url: 'https://www.douyin.com/' };
    const chrome = makeChrome({
      tab,
      pingError: { message: 'Could not establish connection. Receiving end does not exist.' },
    });
    const { exported, elements } = await loadPopup({ chrome });
    exported.selfCheck();
    await flush();
    expect(elements['selfOut'].textContent).not.toContain('undefined');
    expect(elements['selfOut'].textContent).toContain('未注入桥接');
    expect(elements['selfOut'].textContent).toContain('Could not establish connection');
    expect(elements['selfOut'].textContent).toContain('Ctrl+Shift+R');
  });

  it('A. ping 失败但 lastError.message 为 undefined（极端 MV3 场景）→ 兜底为 "(无错误详情)"', async () => {
    const tab = { id: 1, url: 'https://www.douyin.com/' };
    const chrome = makeChrome({
      tab,
      pingError: { message: undefined },
    });
    const { exported, elements } = await loadPopup({ chrome });
    exported.selfCheck();
    await flush();
    expect(elements['selfOut'].textContent).not.toContain('undefined');
    expect(elements['selfOut'].textContent).toContain('(无错误详情)');
  });

  it('B. ping 成功 + selfcheck matched=false → 提示打开私信/消息页', async () => {
    const tab = { id: 1, url: 'https://www.douyin.com/' };
    const chrome = makeChrome({
      tab,
      pingResp: { pong: true, channel: 'douyin_web', matched: false },
      selfcheckResp: { channel: 'douyin_web', matched: false, hint: '请打开私信页' },
    });
    const { exported, elements } = await loadPopup({ chrome });
    exported.selfCheck();
    await flush();
    expect(elements['selfOut'].textContent).toContain('已注入');
    expect(elements['selfOut'].textContent).toContain('不是私信/消息页');
    expect(elements['selfOut'].textContent).toContain('请打开私信页');
  });

  it('C. ping 成功 + selfcheck matched=true → 显示完整桥接数据', async () => {
    const tab = { id: 1, url: 'https://www.douyin.com/follow' };
    const chrome = makeChrome({
      tab,
      pingResp: { pong: true, channel: 'douyin_web', matched: true },
      selfcheckResp: {
        channel: 'douyin_web',
        matched: true,
        accountId: 'A1',
        conversationId: 'C1',
        msgItemCount: 3,
        selectors: { CHAT_LIST: '#island_b69f5' },
        sample: [{ sender: 'customer', text: '你好' }, { sender: 'self', text: '在的' }],
      },
    });
    const { exported, elements } = await loadPopup({ chrome });
    exported.selfCheck();
    await flush();
    expect(elements['selfOut'].textContent).toContain('频道: douyin_web');
    expect(elements['selfOut'].textContent).toContain('匹配: true');
    expect(elements['selfOut'].textContent).toContain('账号: A1');
    expect(elements['selfOut'].textContent).toContain('会话: C1');
    expect(elements['selfOut'].textContent).toContain('消息条目: 3');
    expect(elements['selfOut'].textContent).toContain('island_b69f5');
  });

  it('无活动标签页：直接提示', async () => {
    const chrome = { tabs: { query: (_q, cb) => cb([]) }, runtime: { lastError: null } };
    const { exported, elements } = await loadPopup({ chrome });
    exported.selfCheck();
    expect(elements['selfOut'].textContent).toBe('无活动标签页');
  });

  it('tab.url 不可读（如 chrome:// 页）→ 提示打开非 chrome:// 页面', async () => {
    const tab = { id: 1, url: 'chrome://settings/' };
    const chrome = makeChrome({ tab });
    const { exported, elements } = await loadPopup({ chrome });
    exported.selfCheck();
    expect(elements['selfOut'].textContent).toContain('chrome://');
  });
});
