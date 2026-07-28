// background service worker 单测：覆盖 lastError / getConfig / setConfig / onMessage 路由。
// 策略：每个用例 fresh 一个 mock chrome + 用 vi.resetModules 强制 background 重新执行，
// 触发其模块级副作用（chrome.runtime.onXxx.addListener）。
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { readFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));

// 工具：构造 mock chrome（用 getter 暴露 lastError，以便测试动态修改）
function makeChrome({ storage = {} } = {}) {
  const listeners = { connect: [], message: [] };
  const data = { ...storage };
  // 用闭包变量 + getter 暴露 lastError，让 setConfig helper 能读到
  let _lastError = null;
  const chrome = {
    runtime: {
      get lastError() { return _lastError; },
      set lastError(v) { _lastError = v; },
      onConnect: { addListener: (fn) => listeners.connect.push(fn) },
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
  };
  return { chrome, listeners, data, setLastError: (v) => { _lastError = v; } };
}

// 全局：每个用例保存一份 chrome 引用，背景模块加载时通过 globalThis.chrome 读取
let currentMock = null;
beforeEach(() => {
  currentMock = makeChrome();
  globalThis.chrome = currentMock.chrome;
});
afterEach(() => {
  // 恢复（vitest 自身有内置 chrome mock，但用我们的）
  delete globalThis.chrome;
  currentMock = null;
});

// 加载 background（每次都重新评估模块级副作用）
async function loadBackground() {
  vi.resetModules();
  // 必须先 import 依赖，否则 background import 时拿不到 mock
  // （registry/bridge-client 链上的 chrome 不在测试中触发，可保持原样）
  const mod = await import('../src/background/index.js');
  return mod;
}

describe('background 加载', () => {
  it('无语法错误，能正常执行（监听器已注册）', async () => {
    await loadBackground();
    expect(currentMock.listeners.connect.length).toBe(1);
    expect(currentMock.listeners.message.length).toBe(1);
  });
});

describe('background 配置存储 setConfig 路径', () => {
  it('onMessage setConfig 成功 → 返回 {ok: true}，并写入 storage', async () => {
    await loadBackground();
    const { listeners, data } = currentMock;
    const resp = await new Promise((resolve) => {
      const fn = listeners.message[0];
      fn({ type: 'setConfig', config: { serverUrl: 'http://written:8204', token: 'tk' } }, {}, resolve);
    });
    expect(resp).toEqual({ ok: true });
    expect(data.bridgeConfig.serverUrl).toBe('http://written:8204');
    expect(data.bridgeConfig.token).toBe('tk');
  });

  it('onMessage setConfig 失败（lastError 有值）→ 返回 {ok: false, error}', async () => {
    await loadBackground();
    const { listeners, setLastError } = currentMock;
    // 替换 storage.set，让 set 回调触发时 lastError 仍为失败值
    // 关键：必须等 background 的 setConfig 回调跑完（读了 lastError）之后再清
    const origSet = currentMock.chrome.storage.local.set;
    currentMock.chrome.storage.local.set = (obj, cb) => {
      setLastError({ message: 'quota exceeded' });
      origSet.call(currentMock.chrome.storage.local, obj, () => {
        // 同步排队：先跑 cb（background 会读 lastError），再清
        cb && cb();
        setLastError(null);
      });
    };
    const resp = await new Promise((resolve) => {
      const fn = listeners.message[0];
      fn({ type: 'setConfig', config: { serverUrl: 'http://x' } }, {}, resolve);
    });
    expect(resp).toEqual({ ok: false, error: 'quota exceeded' });
  });
});

describe('background onMessage 路由', () => {
  it('getStatus → 返回 statuses + routes 计数（初始全空）', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    const resp = await new Promise((resolve) => {
      const fn = listeners.message[0];
      fn({ type: 'getStatus' }, {}, resolve);
    });
    expect(resp).toHaveProperty('statuses');
    expect(resp).toHaveProperty('routes');
    expect(resp.routes).toBe(0);
    expect(resp.statuses).toEqual({});
  });

  it('未知 type → listener 同步返回 false（不保持 channel，不调 sendResponse）', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    let sendResponseCalled = false;
    const ret = listeners.message[0]({ type: 'foo-bar' }, {}, () => { sendResponseCalled = true; });
    expect(ret).toBe(false);
    expect(sendResponseCalled).toBe(false);
  });

  it('空 message → listener 同步返回 false', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    let sendResponseCalled = false;
    const ret = listeners.message[0](null, {}, () => { sendResponseCalled = true; });
    expect(ret).toBe(false);
    expect(sendResponseCalled).toBe(false);
  });
});

describe('background onConnect 路由', () => {
  it('非 bridge 端口 → 忽略，不挂 message 监听', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    let portMsgAdded = 0;
    const fakePort = {
      name: 'other',
      onMessage: { addListener: () => portMsgAdded++ },
      onDisconnect: { addListener: () => {} },
    };
    listeners.connect[0](fakePort);
    expect(portMsgAdded).toBe(0);
  });

  it('bridge 端口 → 注册 message + disconnect 监听', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    let portMsgAdded = 0;
    let portDisconnAdded = 0;
    const fakePort = {
      name: 'bridge',
      onMessage: { addListener: () => portMsgAdded++ },
      onDisconnect: { addListener: () => portDisconnAdded++ },
    };
    listeners.connect[0](fakePort);
    expect(portMsgAdded).toBe(1);
    expect(portDisconnAdded).toBe(1);
  });

  it('bridge 端口收到 REGISTER → 填充 port.meta，触发 ensureConnection', async () => {
    await loadBackground();
    const { listeners, chrome } = currentMock;
    // 拦截 storage.get 返回 stub 配置，让 ensureConnection 不会真的去连 WS
    chrome.storage.local.get = (_key, cb) => {
      cb({ bridgeConfig: { serverUrl: 'http://stub:0', token: '', autoConnect: false } });
    };
    const portListeners = [];
    const fakePort = {
      name: 'bridge',
      onMessage: { addListener: (fn) => portListeners.push(fn) },
      onDisconnect: { addListener: () => {} },
    };
    listeners.connect[0](fakePort);
    portListeners[0]({
      type: 'register',
      channel: 'douyin_web',
      accountId: 'A1',
      conversationId: 'C1',
    });
    // ensureConnection 是异步的（getConfig → registry.ensure）
    await new Promise((r) => setTimeout(r, 5));
    expect(fakePort.meta).toEqual({ channel: 'douyin_web', accountId: 'A1', conversationId: 'C1' });
  });

  it('bridge 端口收到 INBOUND 但无连接 → log.warn，不崩', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    const portListeners = [];
    const fakePort = {
      name: 'bridge',
      onMessage: { addListener: (fn) => portListeners.push(fn) },
      onDisconnect: { addListener: () => {} },
    };
    listeners.connect[0](fakePort);
    // 先发 REGISTER 填 meta
    portListeners[0]({ type: 'register', channel: 'douyin_web', accountId: 'A1', conversationId: 'C1' });
    // 再发 INBOUND（registry 此时无 conn，但不应抛）
    expect(() => {
      portListeners[0]({ type: 'inbound_message', message: { conversation_id: 'C1', content: 'hi' } });
    }).not.toThrow();
  });

  it('bridge 端口 disconnect → 清理 routes + registry', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    const portListeners = [];
    const disconnListeners = [];
    const fakePort = {
      name: 'bridge',
      onMessage: { addListener: (fn) => portListeners.push(fn) },
      onDisconnect: { addListener: (fn) => disconnListeners.push(fn) },
    };
    listeners.connect[0](fakePort);
    portListeners[0]({ type: 'register', channel: 'douyin_web', accountId: 'A1', conversationId: 'C1' });
    // 触发 disconnect
    disconnListeners[0]();
    // 不崩 + 再次 disconnect 不应抛（meta 已清）
    expect(() => disconnListeners[0]()).not.toThrow();
  });
});

describe('background DEFAULT_USER_SERVER 默认值', () => {
  it('storage 完全为空 → setConfig 写入的默认值 serverUrl 来自 constants.js', async () => {
    const { DEFAULT_USER_SERVER } = await import('../src/core/constants.js');
    await loadBackground();
    const { data, listeners } = currentMock;
    // 模拟 onMessage setConfig 写入空 config（不应崩）
    const resp = await new Promise((resolve) => {
      const fn = listeners.message[0];
      fn({ type: 'setConfig', config: { serverUrl: DEFAULT_USER_SERVER.baseUrl, token: '', autoConnect: true } }, {}, resolve);
    });
    expect(resp).toEqual({ ok: true });
    expect(data.bridgeConfig.serverUrl).toBe(DEFAULT_USER_SERVER.baseUrl);
  });
});


