// popup 单元测试：覆盖 normalizeServerUrl / testConnection / showBanner
// 使用 jsdom 模拟 DOM + chrome API + AbortController/fetch

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { readFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const popupSrc = readFileSync(resolve(__dirname, '../src/popup/index.js'), 'utf8');

// 加载 popup 模块（暴露 window.__popup）
function loadPopup(globals) {
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
  // popup.js 顶部用 const/function 声明，最后暴露 window.__popup
  // 直接在 Function 构造中执行（保持模块顶层作用域）
  const fn = new Function(...Object.keys(ctx), popupSrc + '\nreturn typeof window !== "undefined" ? window.__popup : null;');
  const exported = fn(...Object.values(ctx));
  return { exported, elements, window, document };
}

describe('popup normalizeServerUrl', () => {
  it('空字符串返回空', () => {
    const { exported } = loadPopup({});
    expect(exported.normalizeServerUrl('')).toBe('');
    expect(exported.normalizeServerUrl(null)).toBe('');
    expect(exported.normalizeServerUrl(undefined)).toBe('');
  });

  it('去除首尾空格', () => {
    const { exported } = loadPopup({});
    expect(exported.normalizeServerUrl('  http://x:8204  ')).toBe('http://x:8204');
  });

  it('自动补 http://', () => {
    const { exported } = loadPopup({});
    expect(exported.normalizeServerUrl('localhost:8204')).toBe('http://localhost:8204');
    expect(exported.normalizeServerUrl('192.168.1.1:8204/path')).toBe('http://192.168.1.1:8204/path');
  });

  it('保留 https', () => {
    const { exported } = loadPopup({});
    expect(exported.normalizeServerUrl('https://example.com:8204')).toBe('https://example.com:8204');
  });

  it('去除尾斜杠', () => {
    const { exported } = loadPopup({});
    expect(exported.normalizeServerUrl('http://x:8204/')).toBe('http://x:8204');
    expect(exported.normalizeServerUrl('http://x:8204///')).toBe('http://x:8204');
    expect(exported.normalizeServerUrl('http://x:8204/path/')).toBe('http://x:8204/path');
  });
});

describe('popup testConnection', () => {
  it('空 URL 返回 empty', async () => {
    const { exported } = loadPopup({});
    const r = await exported.testConnection('');
    expect(r.ok).toBe(false);
    expect(r.reason).toBe('empty');
  });

  it('2xx 返回 ok=true', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const { exported } = loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:8204');
    expect(r.ok).toBe(true);
    expect(r.degraded).toBe(false);
    expect(r.status).toBe(200);
    expect(r.url).toBe('http://localhost:8204/health');
  });

  it('503 返回 ok=true degraded=true', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 503 });
    const { exported } = loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:8204');
    expect(r.ok).toBe(true);
    expect(r.degraded).toBe(true);
    expect(r.status).toBe(503);
  });

  it('404 跳过当前路径，尝试下一个', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 404 })
      .mockResolvedValueOnce({ ok: true, status: 200 });
    const { exported } = loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:8204');
    expect(r.ok).toBe(true);
    expect(r.url).toBe('http://localhost:8204/healthz');
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('401/403 立即返回 http_xxx', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 401 });
    const { exported } = loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:8204');
    expect(r.ok).toBe(false);
    expect(r.reason).toBe('http_401');
    expect(r.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('网络错误：所有路径都失败时返回 unreachable', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'));
    const { exported } = loadPopup({ fetch: fetchMock });
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
    const { exported } = loadPopup({ fetch: fetchMock });
    const r = await exported.testConnection('http://localhost:8204');
    expect(r.ok).toBe(true);
    expect(r.url).toBe('http://localhost:8204/healthz');
  });
});

describe('popup showBanner', () => {
  it('设置 className 与内容', () => {
    const { exported, elements } = loadPopup({});
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

  it('不同 kind 切换 className', () => {
    const { exported, elements } = loadPopup({});
    exported.showBanner('error', 'X', 'Y');
    expect(elements['banner'].className).toContain('error');
    exported.showBanner('warn', 'X', 'Y');
    expect(elements['banner'].className).toContain('warn');
    exported.showBanner('info', 'X', 'Y');
    expect(elements['banner'].className).toContain('info');
  });

  it('clearBanner 重置 className', () => {
    const { exported, elements } = loadPopup({});
    exported.showBanner('error', 'X', 'Y');
    exported.clearBanner();
    expect(elements['banner'].className).toBe('banner');
  });
});
