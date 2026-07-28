// injector.js 单测
//
// 覆盖：
//   1) URL → content script 文件 路由（纯函数，无副作用）
//   2) scriptingAvailable 探测
//   3) injectContentScript 主流程（含错误兜底、dedup、透传参数）
//
// 文档源：user-server/docs/dev/DEVELOPMENT.md 端口对照表 + bridge/bridge.md §17
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import {
  pickContentScriptFile,
  injectContentScript,
  scriptingAvailable,
  __resetInjectState,
} from '../src/background/injector.js';

// =============================================================
// 1) pickContentScriptFile —— 纯函数路由表
// =============================================================
describe('pickContentScriptFile / URL 路由', () => {
  it('抖音主站 → content-douyin.js', () => {
    expect(pickContentScriptFile('https://www.douyin.com/follow')).toBe('content-douyin.js');
  });

  it('抖音创作者中心（子域）→ content-douyin.js', () => {
    expect(pickContentScriptFile('https://creator.douyin.com/')).toBe('content-douyin.js');
  });

  it('小红书 → content-xhs.js', () => {
    expect(pickContentScriptFile('https://www.xiaohongshu.com/message')).toBe('content-xhs.js');
  });

  it('TikTok → content-tiktok.js', () => {
    expect(pickContentScriptFile('https://www.tiktok.com/messages')).toBe('content-tiktok.js');
  });

  it('非支持域名 → null', () => {
    expect(pickContentScriptFile('https://example.com/foo')).toBe(null);
    expect(pickContentScriptFile('https://baidu.com/')).toBe(null);
  });

  it('无效输入 → null', () => {
    expect(pickContentScriptFile(null)).toBe(null);
    expect(pickContentScriptFile(undefined)).toBe(null);
    expect(pickContentScriptFile('')).toBe(null);
    expect(pickContentScriptFile('not a url')).toBe(null);
  });

  it('大小写不敏感', () => {
    expect(pickContentScriptFile('HTTPS://WWW.DOUYIN.COM/')).toBe('content-douyin.js');
  });
});

// =============================================================
// 2) scriptingAvailable —— 探测 chrome API 存在性
// =============================================================
describe('scriptingAvailable / 探测', () => {
  it('chrome.scripting.executeScript 存在 → true', () => {
    const saved = globalThis.chrome;
    globalThis.chrome = { scripting: { executeScript: () => {} } };
    expect(scriptingAvailable()).toBe(true);
    globalThis.chrome = saved;
  });

  it('chrome.scripting 缺失 → false', () => {
    const saved = globalThis.chrome;
    globalThis.chrome = { runtime: {} };
    expect(scriptingAvailable()).toBe(false);
    globalThis.chrome = saved;
  });

  it('chrome 完全缺失 → false', () => {
    const saved = globalThis.chrome;
    delete globalThis.chrome;
    expect(scriptingAvailable()).toBe(false);
    globalThis.chrome = saved;
  });
});

// =============================================================
// 3) injectContentScript —— 主流程
// =============================================================
// 关键设计：chrome.scripting.executeScript 第二个参数 callback 是异步触发的，
// 真实 Chrome 实现里 callback 在下一个 microtask 才会触发。
// 测试用 setTimeout(..., 0) 模拟这个延迟。
describe('injectContentScript / 主流程', () => {
  let executeScriptMock;

  beforeEach(() => {
    executeScriptMock = vi.fn();
    globalThis.chrome = {
      scripting: { executeScript: executeScriptMock },
      runtime: { lastError: null },
    };
    __resetInjectState();
  });

  afterEach(() => {
    delete globalThis.chrome;
  });

  // 通用 mock 工厂：成功路径
  function mockSuccess() {
    executeScriptMock.mockImplementation((opts, cb) => {
      setTimeout(() => cb(), 0);
    });
  }

  // 通用 mock 工厂：lastError 失败路径
  function mockLastError(message) {
    executeScriptMock.mockImplementation((opts, cb) => {
      setTimeout(() => {
        globalThis.chrome.runtime.lastError = { message };
        cb();
      }, 0);
    });
  }

  it('成功路径 → { ok: true, file }', async () => {
    mockSuccess();
    const r = await new Promise((resolve) => {
      injectContentScript(42, 'https://www.douyin.com/follow', resolve);
    });
    expect(r.ok).toBe(true);
    expect(r.file).toBe('content-douyin.js');
    expect(executeScriptMock).toHaveBeenCalledTimes(1);
    const [arg] = executeScriptMock.mock.calls[0];
    expect(arg).toEqual({
      target: { tabId: 42, allFrames: false },
      files: ['content-douyin.js'],
    });
  });

  it('lastError 失败 → { ok: false, reason: execute_failed }', async () => {
    mockLastError('permission denied');
    const r = await new Promise((resolve) => {
      injectContentScript(1, 'https://www.douyin.com/', resolve);
    });
    expect(r.ok).toBe(false);
    expect(r.reason).toBe('execute_failed');
    expect(r.error).toContain('permission denied');
  });

  it('executeScript 抛异常 → { ok: false, reason: execute_threw }', async () => {
    executeScriptMock.mockImplementation(() => {
      throw new Error('boom');
    });
    const r = await new Promise((resolve) => {
      injectContentScript(1, 'https://www.douyin.com/', resolve);
    });
    expect(r.ok).toBe(false);
    expect(r.reason).toBe('execute_threw');
    expect(r.error).toContain('boom');
  });

  it('非支持 host → { ok: false, reason: unsupported_host }，不调用 executeScript', async () => {
    const r = await new Promise((resolve) => {
      injectContentScript(1, 'https://example.com/', resolve);
    });
    expect(r.ok).toBe(false);
    expect(r.reason).toBe('unsupported_host');
    expect(executeScriptMock).not.toHaveBeenCalled();
  });

  it('allFrames=true 透传到 target.allFrames', async () => {
    mockSuccess();
    await new Promise((resolve) => {
      injectContentScript(1, 'https://www.douyin.com/', { allFrames: true }, resolve);
    });
    const [arg] = executeScriptMock.mock.calls[0];
    expect(arg.target).toEqual({ tabId: 1, allFrames: true });
  });

  it('dedup：同 (tabId, file) 1.5s 内重复 → 第二次 dedup=true', async () => {
    mockSuccess();
    const r1 = await new Promise((resolve) => {
      injectContentScript(99, 'https://www.douyin.com/', resolve);
    });
    const r2 = await new Promise((resolve) => {
      injectContentScript(99, 'https://www.douyin.com/', resolve);
    });
    expect(r1.ok).toBe(true);
    expect(r1.dedup).toBeUndefined();
    expect(r2.ok).toBe(true);
    expect(r2.dedup).toBe(true);
    expect(executeScriptMock).toHaveBeenCalledTimes(1);
  });

  it('不同 tabId 各自独立（不被 dedup）', async () => {
    mockSuccess();
    const r1 = await new Promise((resolve) => {
      injectContentScript(1, 'https://www.douyin.com/', resolve);
    });
    const r2 = await new Promise((resolve) => {
      injectContentScript(2, 'https://www.douyin.com/', resolve);
    });
    expect(r1.ok).toBe(true);
    expect(r2.ok).toBe(true);
    expect(r1.dedup).toBeUndefined();
    expect(r2.dedup).toBeUndefined();
    expect(executeScriptMock).toHaveBeenCalledTimes(2);
  });

  it('opts 缺省时 allFrames=false', async () => {
    mockSuccess();
    await new Promise((resolve) => {
      injectContentScript(1, 'https://www.douyin.com/', resolve);
    });
    const [arg] = executeScriptMock.mock.calls[0];
    expect(arg.target.allFrames).toBe(false);
  });

  it('opts 位置传函数（兼容旧调用）→ opts 视为空', async () => {
    mockSuccess();
    await new Promise((resolve) => {
      // 旧式调用：injectContentScript(tabId, url, cb)
      injectContentScript(1, 'https://www.douyin.com/', resolve);
    });
    expect(executeScriptMock).toHaveBeenCalledTimes(1);
  });
});
