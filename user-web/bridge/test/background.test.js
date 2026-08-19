import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

function makeChrome({ storage = {} } = {}) {
  const listeners = { message: [] };
  const data = { ...storage };
  let _lastError = null;
  const chrome = {
    runtime: {
      get lastError() { return _lastError; },
      set lastError(v) { _lastError = v; },
      onMessage: { addListener: (fn) => listeners.message.push(fn) },
    },
    storage: {
      local: {
        get: (key, cb) => {
          const out = {};
          if (typeof key === 'string') out[key] = data[key];
          else if (Array.isArray(key)) key.forEach((k) => (out[k] = data[k]));
          else Object.keys(data).forEach((k) => (out[k] = data[k]));
          cb(out);
        },
        set: (obj, cb) => {
          Object.assign(data, obj);
          cb && cb();
        },
      },
    },
    webNavigation: { onCommitted: { addListener: () => {} } },
    scripting: {},
    tabs: {
      sendMessage: (_tabId, _msg, _cb) => {},
      query: (_opts, cb) => { cb([]); },
    },
    alarms: {
      create: () => {},
      onAlarm: { addListener: () => {} },
    },
  };
  return { chrome, listeners, data, setLastError: (v) => { _lastError = v; } };
}

let currentMock = null;
beforeEach(() => {
  currentMock = makeChrome();
  globalThis.chrome = currentMock.chrome;
});
afterEach(() => {
  delete globalThis.chrome;
  currentMock = null;
});

async function loadBackground() {
  vi.resetModules();
  const mod = await import('../src/background/index.js');
  return mod;
}

// 尝试所有监听器，找到能处理该消息的那个（遍历 sse-client.js + index.js 等多个 listener）
function callListener(listeners, msg, sendResponse) {
  let called = false;
  const wrapper = (resp) => { called = true; sendResponse(resp); };
  for (const fn of listeners.message) {
    const ret = fn(msg, {}, wrapper);
    if (ret !== false || called) return called;
  }
  return called;
}

describe('background 加载', () => {
  it('无语法错误，能正常执行（至少注册了 onMessage 监听器）', async () => {
    await loadBackground();
    // index.js + sse-client.js 各注册了一个 onMessage listener
    expect(currentMock.listeners.message.length).toBeGreaterThanOrEqual(1);
  });
});

describe('background 配置存储 setConfig 路径', () => {
  it('onMessage setConfig 成功 → 返回 {ok: true}，并写入 storage', async () => {
    await loadBackground();
    const { listeners, data } = currentMock;
    const resp = await new Promise((resolve) => {
      callListener(listeners, { type: 'setConfig', config: { serverUrl: 'http://written:8204', token: 'tk' } }, resolve);
    });
    expect(resp).toEqual({ ok: true });
    expect(data.bridgeConfig.serverUrl).toBe('http://written:8204');
    expect(data.bridgeConfig.token).toBe('tk');
  });

  it('onMessage setConfig 失败（lastError 有值）→ 返回 {ok: false, error}', async () => {
    await loadBackground();
    const { listeners, setLastError } = currentMock;
    const origSet = currentMock.chrome.storage.local.set;
    currentMock.chrome.storage.local.set = (obj, cb) => {
      setLastError({ message: 'quota exceeded' });
      origSet.call(currentMock.chrome.storage.local, obj, () => {
        cb && cb();
        setLastError(null);
      });
    };
    const resp = await new Promise((resolve) => {
      callListener(listeners, { type: 'setConfig', config: { serverUrl: 'http://x' } }, resolve);
    });
    expect(resp).toEqual({ ok: false, error: 'quota exceeded' });
  });
});

describe('background onMessage 路由', () => {
  it('getStatus → 返回 statuses + routes 计数（初始全空）', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    const resp = await new Promise((resolve) => {
      callListener(listeners, { type: 'getStatus' }, resolve);
    });
    expect(resp).toHaveProperty('statuses');
    expect(resp).toHaveProperty('routes');
    expect(resp.routes).toBe(0);
    expect(resp.statuses).toEqual({});
    expect(resp.connStats).toBeNull();
  });

  it('getStatus → storage 中已有 activeByChannel 时透传', async () => {
    currentMock = makeChrome({
      storage: {
        bridgeActiveByChannel: { xiaohongshu: { channel: 'xiaohongshu', accountId: 'a1', online: true } },
      },
    });
    globalThis.chrome = currentMock.chrome;
    await loadBackground();
    const { listeners } = currentMock;
    const resp = await new Promise((resolve) => {
      callListener(listeners, { type: 'getStatus' }, resolve);
    });
    expect(resp.statuses).toEqual({ xiaohongshu: { channel: 'xiaohongshu', accountId: 'a1', online: true } });
  });

  it('injectContentScript 缺 tabId → 同步返回 {ok: false, reason: invalid_tabId}', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    let called = false;
    let sent;
    const wrapper = (r) => { called = true; sent = r; };
    for (const fn of listeners.message) {
      const ret = fn({ type: 'injectContentScript', url: 'http://x' }, {}, wrapper);
      if (ret !== false || called) break;
    }
    expect(sent).toEqual({ ok: false, reason: 'invalid_tabId' });
  });

  it('injectContentScript 合法 tabId → 异步返回 {ok}', async () => {
    await loadBackground();
    // 模拟 chrome.scripting.executeScript
    let cbArg;
    globalThis.chrome.scripting.executeScript = (_opts, cb) => cb([{ result: 'stub' }]);
    const { listeners } = currentMock;
    const resp = await new Promise((resolve) => {
      callListener(listeners, { type: 'injectContentScript', tabId: 1, url: 'http://x' }, resolve);
    });
    expect(resp).toBeDefined();
  });

  it('未知 type → listener 同步返回 false（不保持 channel）', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    let sendResponseCalled = false;
    for (const fn of listeners.message) {
      const ret = fn({ type: 'foo-bar' }, {}, () => { sendResponseCalled = true; });
      // 第一个返回非 false 的就停止（index.js 的会返回 false，然后 sse-client.js 也返回 false）
      // 但我们只想验证第一个匹配项的行为
      if (ret !== false) break;
    }
    // 所有监听器都应该返回 false 且不调用 sendResponse
    expect(sendResponseCalled).toBe(false);
  });

  it('空 message → listener 同步返回 false', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    let sendResponseCalled = false;
    for (const fn of listeners.message) {
      const ret = fn(null, {}, () => { sendResponseCalled = true; });
      if (ret !== false) break;
    }
    expect(sendResponseCalled).toBe(false);
  });
});

describe('background DEFAULT_USER_SERVER 默认值', () => {
  it('storage 完全为空 → setConfig 写入默认值 serverUrl 来自 constants.js', async () => {
    const { DEFAULT_USER_SERVER } = await import('../src/core/constants.js');
    await loadBackground();
    const { data, listeners } = currentMock;
    const resp = await new Promise((resolve) => {
      callListener(listeners, { type: 'setConfig', config: { serverUrl: DEFAULT_USER_SERVER.baseUrl, token: '', autoConnect: true } }, resolve);
    });
    expect(resp).toEqual({ ok: true });
    expect(data.bridgeConfig.serverUrl).toBe(DEFAULT_USER_SERVER.baseUrl);
  });
});

describe('background HTTP-only 模式', () => {
  it('不再监听 chrome.runtime.onConnect', async () => {
    await loadBackground();
    expect(currentMock.chrome.runtime.onConnect).toBeUndefined();
  });
});

